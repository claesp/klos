// SPDX-License-Identifier: BSD-2-Clause

package main

// The SELinux policy of an image. Read docs/selinux.md.
//
// The policy is CIL (selinux/*.cil). kbuild adds a generated part: the
// security classes, permissions and initial SIDs of the kernel (from the
// kernel source), the policy capabilities, and the "unconfined" rules. Then
// secilc (package secilc) compiles the policy. kbuild writes these files
// into the image:
//
//	/usr/lib/klos/selinux/policy        the binary policy
//	/usr/lib/klos/selinux/file_contexts the labels of files
//	/usr/lib/klos/selinux/config        mode=enforcing or mode=permissive
//
// The installed system has no SELinux user space: /init (klos) loads the
// policy and labels /run, /dev and /var. kbuild labels the files of the
// OS image (labeledTar).

import (
	"archive/tar"
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const seDir = "usr/lib/klos/selinux"

// sePolicyCaps are the policy capabilities that KLOS enables. Read
// docs/selinux.md for the reasons.
var sePolicyCaps = []string{
	"network_peer_controls", "open_perms", "extended_socket_class",
	"cgroup_seclabel", "nnp_nosuid_transition", "genfs_seclabel_symlinks",
	"ioctl_skip_cloexec", "userspace_initial_context",
}

// seRestricted are the permissions that the unconfined domains do not get.
// No process can change the policy or the mode after the boot, and no
// process can make memory writable and executable at the same time (the
// attribute execmem_domain gives the process permissions back).
var seRestricted = map[string][]string{
	"security": {"load_policy", "setenforce", "setbool", "setsecparam",
		"setcheckreqprot"},
	"process":     {"execmem", "execstack", "execheap"},
	"file":        {"execmod"},
	"capability2": {"mac_admin", "mac_override"},
}

type seClass struct {
	name  string
	perms []string
}

var (
	cComment = regexp.MustCompile(`(?s)/\*.*?\*/`)
	cToken   = regexp.MustCompile(`"[^"]*"|[A-Za-z_][A-Za-z0-9_]*|[{}]`)
)

// parseClassMap reads the security classes and their permissions from
// security/selinux/include/classmap.h of the kernel source.
func parseClassMap(text string) ([]seClass, error) {
	macros := map[string][]string{}
	var body strings.Builder
	lines := strings.Split(strings.ReplaceAll(cComment.ReplaceAllString(text, ""), "\\\n", " "), "\n")
	for _, l := range lines {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "#define ") {
			f := strings.Fields(t)
			if len(f) >= 3 {
				macros[f[1]] = cToken.FindAllString(strings.Join(f[2:], " "), -1)
			}
			continue
		}
		if strings.HasPrefix(t, "#") {
			continue
		}
		body.WriteString(l + "\n")
	}
	b := body.String()
	start := strings.Index(b, "secclass_map[] = {")
	if start < 0 {
		return nil, fmt.Errorf("classmap.h: no secclass_map")
	}
	var expand func(tok string, depth int) ([]string, error)
	expand = func(tok string, depth int) ([]string, error) {
		if strings.HasPrefix(tok, `"`) {
			return []string{strings.Trim(tok, `"`)}, nil
		}
		m, ok := macros[tok]
		if !ok || depth > 10 {
			return nil, fmt.Errorf("classmap.h: unknown name %s", tok)
		}
		var out []string
		for _, t := range m {
			e, err := expand(t, depth+1)
			if err != nil {
				return nil, err
			}
			out = append(out, e...)
		}
		return out, nil
	}
	var classes []seClass
	depth := 0
	var cur *seClass
	for _, tok := range cToken.FindAllString(b[start+len("secclass_map[] = "):], -1) {
		switch tok {
		case "{":
			depth++
			if depth == 2 {
				classes = append(classes, seClass{})
				cur = &classes[len(classes)-1]
			}
			continue
		case "}":
			depth--
			if depth == 0 {
				// The map ends with an empty entry ({ NULL }).
				if len(classes) > 0 && classes[len(classes)-1].name == "" {
					classes = classes[:len(classes)-1]
				}
				return classes, nil
			}
			continue
		case "NULL":
			continue
		}
		if cur == nil {
			continue
		}
		vals, err := expand(tok, 0)
		if err != nil {
			return nil, err
		}
		if depth == 2 && cur.name == "" {
			cur.name = vals[0]
		} else if depth == 3 {
			cur.perms = append(cur.perms, vals...)
		}
	}
	return nil, fmt.Errorf("classmap.h: secclass_map has no end")
}

var sidLine = regexp.MustCompile(`^\s*(?:NULL|"[a-z_]+"),\s*/\*\s*([a-z_]+)`)

// parseInitialSIDs reads the names of the initial SIDs in the kernel order
// from initial_sid_to_string.h. The names of unused SIDs are in comments.
// Entry 0 is a placeholder; the SIDs start at 1.
func parseInitialSIDs(text string) []string {
	var sids []string
	for _, l := range strings.Split(text, "\n") {
		if m := sidLine.FindStringSubmatch(l); m != nil {
			sids = append(sids, m[1])
		}
	}
	if len(sids) > 0 {
		sids = sids[1:]
	}
	return sids
}

// policyVersion returns POLICYDB_VERSION_MAX of the kernel.
func policyVersion(text string) (int, error) {
	defs := map[string]string{}
	for _, l := range strings.Split(text, "\n") {
		f := strings.Fields(l)
		if len(f) >= 3 && f[0] == "#define" && strings.HasPrefix(f[1], "POLICYDB_VERSION_") {
			defs[f[1]] = f[2]
		}
	}
	v := defs["POLICYDB_VERSION_MAX"]
	for i := 0; i < 5; i++ {
		if n, err := strconv.Atoi(v); err == nil {
			return n, nil
		}
		v = defs[v]
	}
	return 0, fmt.Errorf("security.h: no POLICYDB_VERSION_MAX")
}

// seCategories is the number of MCS categories (c0 to c1023).
const seCategories = 1024

var cilSidContext = regexp.MustCompile(`\(sidcontext\s+([a-z_]+)`)

// kernelCIL makes the generated part of the policy. An initial SID that
// the policy does not give a context gets unlabeled_t.
func kernelCIL(classes []seClass, sids []string, policy string) string {
	var b strings.Builder
	b.WriteString("; Generated by kbuild from the kernel source. Do not edit.\n")
	b.WriteString("(handleunknown deny)\n(mls true)\n")
	b.WriteString("(sensitivity s0)\n(sensitivityorder (s0))\n")
	var cats []string
	for i := 0; i < seCategories; i++ {
		cats = append(cats, fmt.Sprintf("c%d", i))
		fmt.Fprintf(&b, "(category c%d)\n", i)
	}
	fmt.Fprintf(&b, "(categoryorder (%s))\n", strings.Join(cats, " "))
	fmt.Fprintf(&b, "(sensitivitycategory s0 (range c0 c%d))\n", seCategories-1)
	given := map[string]bool{}
	for _, m := range cilSidContext.FindAllStringSubmatch(policy, -1) {
		given[m[1]] = true
	}
	var names []string
	for _, c := range classes {
		fmt.Fprintf(&b, "(class %s (%s))\n", c.name, strings.Join(c.perms, " "))
		names = append(names, c.name)
	}
	fmt.Fprintf(&b, "(classorder (%s))\n", strings.Join(names, " "))
	for _, s := range sids {
		fmt.Fprintf(&b, "(sid %s)\n", s)
	}
	fmt.Fprintf(&b, "(sidorder (%s))\n", strings.Join(sids, " "))
	for _, s := range sids {
		if !given[s] {
			fmt.Fprintf(&b, "(sidcontext %s (system_u object_r unlabeled_t low_low))\n", s)
		}
	}
	for _, p := range sePolicyCaps {
		fmt.Fprintf(&b, "(policycap %s)\n", p)
	}
	b.WriteString("; The unconfined domains get all permissions on all types, except\n; the permissions in seRestricted.\n")
	for _, c := range classes {
		restricted := seRestricted[c.name]
		if len(restricted) == 0 {
			fmt.Fprintf(&b, "(allow unconfined_domain any_type (%s (all)))\n", c.name)
			continue
		}
		var perms []string
		for _, p := range c.perms {
			if !contains(restricted, p) {
				perms = append(perms, p)
			}
		}
		if len(perms) > 0 {
			fmt.Fprintf(&b, "(allow unconfined_domain any_type (%s (%s)))\n", c.name, strings.Join(perms, " "))
		}
	}
	return b.String()
}

var cilType = regexp.MustCompile(`\((?:type|typealias)\s+([A-Za-z0-9_]+)\s*\)`)

// buildSELinux compiles the policy of the flavor and writes the policy
// files into the root file system. It returns the file contexts, or nil
// if the flavor has no SELinux policy.
func buildSELinux(m *Manifest, rootfs, ksrc string) *fileContexts {
	if len(m.SELinux.Policy) == 0 {
		return nil
	}
	mode := seMode(m)
	inc := filepath.Join(ksrc, "security/selinux/include")
	read := func(name string) string {
		b, err := os.ReadFile(filepath.Join(inc, name))
		check(err)
		return string(b)
	}
	classes, err := parseClassMap(read("classmap.h"))
	check(err)
	sids := parseInitialSIDs(read("initial_sid_to_string.h"))
	version, err := policyVersion(read("security.h"))
	check(err)

	work := filepath.Join(WORK, "selinux", m.Flavor.Name)
	removeAll(work)
	mkdirAll(work)
	files := []string{filepath.Join(work, "kernel.cil")}
	types := map[string]bool{}
	var policyText strings.Builder
	for _, p := range m.SELinux.Policy {
		src := filepath.Join(TOP, "selinux", p)
		b, err := os.ReadFile(src)
		check(err)
		policyText.Write(b)
		for _, t := range cilType.FindAllStringSubmatch(string(b), -1) {
			types[t[1]] = true
		}
		files = append(files, src)
	}
	writeFile(files[0], kernelCIL(classes, sids, policyText.String()), 0o644)
	var fcText strings.Builder
	for _, p := range m.SELinux.FileContexts {
		b, err := os.ReadFile(filepath.Join(TOP, "selinux", p))
		check(err)
		fcText.Write(b)
	}
	fcText.WriteString(etcUpperContexts(fcText.String()))
	fc, err := parseFileContexts(fcText.String())
	if err != nil {
		die("selinux: %v", err)
	}
	for _, t := range fc.types() {
		if !types[t] {
			die("selinux: file_contexts uses the type %s, but the policy has no such type", t)
		}
	}
	policy := filepath.Join(work, "policy")
	args := append(targetTool("secilc", "secilc"), "--mls=true", "-c", strconv.Itoa(version),
		"-o", policy, "-f", filepath.Join(work, "secilc-file_contexts"))
	run(Cmd{Args: append(args, files...)})
	dir := filepath.Join(rootfs, seDir)
	mkdirAll(dir)
	copyFile(policy, filepath.Join(dir, "policy"))
	writeFile(filepath.Join(dir, "file_contexts"), fcText.String(), 0o644)
	writeFile(filepath.Join(dir, "config"), "mode="+mode+"\n", 0o644)
	logf("selinux: policy version %d, %d classes, %d types, mode %s", version, len(classes), len(types), mode)
	return fc
}

// etcUpperContexts makes file context lines for the /etc overlay upper
// layer from the /etc lines: a file in the upper layer must have the label
// of the same file in /etc.
func etcUpperContexts(text string) string {
	var b strings.Builder
	b.WriteString("# Generated by kbuild: the /etc lines for the /etc overlay upper layer.\n")
	sc := bufio.NewScanner(strings.NewReader(text))
	for sc.Scan() {
		l := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(l, "/etc") {
			b.WriteString("/var/lib/klos/etc/upper" + strings.TrimPrefix(l, "/etc") + "\n")
		}
	}
	return b.String()
}

// targetTool returns the command that runs a program of a target package
// in the builder: the KLOS dynamic linker runs it with the libraries of the
// package and its runtime dependencies.
func targetTool(pkg, prog string) []string {
	build([]string{pkg}, false)
	names := runtimeClosure([]string{pkg})
	h := sha256.New()
	for _, n := range names {
		_, _ = fmt.Fprintf(h, "%s=%s\x00", n, pkgMeta(n).SHA256) // hash.Hash.Write never fails
	}
	root := filepath.Join(WORK, "tools", hex.EncodeToString(h.Sum(nil))[:16])
	if !exists(filepath.Join(root, ".done")) {
		removeAll(root)
		for _, n := range names {
			extractTarball(pkgTarball(n), root, nil)
		}
		writeFile(filepath.Join(root, ".done"), "", 0o644)
	}
	return []string{filepath.Join(root, "usr/lib/libc.so"), "--library-path",
		filepath.Join(root, "usr/lib"), filepath.Join(root, "usr/bin", prog)}
}

// writeLabeledTar writes the root file system as a tar archive for
// "mkfs.erofs --tar=f". Each entry has its SELinux label in a PAX record.
// All files belong to root, and all entries have the same time.
func writeLabeledTar(rootfs string, fc *fileContexts, out io.Writer) error {
	tw := tar.NewWriter(out)
	links := map[uint64]string{}
	unlabeled := 0
	err := filepath.WalkDir(rootfs, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(rootfs, p)
		if err != nil {
			return err
		}
		path := "/" + rel
		name := rel
		if rel == "." {
			path, name = "/", "."
		}
		h := &tar.Header{Name: name, Mode: tarMode(info.Mode()), ModTime: time.Unix(SDE, 0), Format: tar.FormatPAX}
		var data *os.File
		switch {
		case info.IsDir():
			h.Typeflag, h.Name = tar.TypeDir, name+"/"
		case info.Mode()&fs.ModeSymlink != 0:
			t, err := os.Readlink(p)
			if err != nil {
				return err
			}
			h.Typeflag, h.Linkname = tar.TypeSymlink, t
		case info.Mode().IsRegular():
			st := info.Sys().(*syscall.Stat_t)
			if st.Nlink > 1 {
				if first, ok := links[st.Ino]; ok {
					h.Typeflag, h.Linkname = tar.TypeLink, first
					break
				}
				links[st.Ino] = name
			}
			h.Typeflag, h.Size = tar.TypeReg, info.Size()
			if data, err = os.Open(p); err != nil {
				return err
			}
			defer closeQuietly(data)
		default:
			return fmt.Errorf("%s: the image cannot contain this file type", path)
		}
		if h.Typeflag != tar.TypeLink && fc != nil {
			if ctx := fc.lookup(path, info.Mode()); ctx != "" {
				h.PAXRecords = map[string]string{"SCHILY.xattr.security.selinux": ctx}
			} else {
				unlabeled++
			}
		}
		if err := tw.WriteHeader(h); err != nil {
			return err
		}
		if data != nil {
			if _, err := io.Copy(tw, data); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if unlabeled > 0 {
		return fmt.Errorf("%d files have no SELinux label (no file_contexts line matches)", unlabeled)
	}
	return tw.Close()
}

func tarMode(m fs.FileMode) int64 {
	v := int64(m.Perm())
	if m&fs.ModeSetuid != 0 {
		v |= 0o4000
	}
	if m&fs.ModeSetgid != 0 {
		v |= 0o2000
	}
	if m&fs.ModeSticky != 0 {
		v |= 0o1000
	}
	return v
}

// cmdSELinux compiles the policy of a flavor (for policy development). The
// files go to work/selinux/<flavor>/out.
func cmdSELinux(args []string) {
	if len(args) != 1 {
		die("usage: selinux <flavor>")
	}
	m := flavorManifest(args[0])
	out := filepath.Join(WORK, "selinux", "out-"+args[0])
	removeAll(out)
	if buildSELinux(m, out, kernelSource()) == nil {
		die("flavor %s has no SELinux policy", args[0])
	}
	logf("selinux: the policy files are in %s", rel(filepath.Join(out, seDir)))
}

// seMode returns the SELinux mode of a manifest: enforcing (the default)
// or permissive.
func seMode(m *Manifest) string {
	switch m.SELinux.Mode {
	case "", "enforcing":
		return "enforcing"
	case "permissive":
		return "permissive"
	}
	die("[selinux] mode must be enforcing or permissive, not %q", m.SELinux.Mode)
	return ""
}
