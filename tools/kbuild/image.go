// SPDX-License-Identifier: BSD-2-Clause

package main

// OS images and update bundles. Read docs/build-system.md and docs/updates.md.

import (
	"bufio"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"debug/elf"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"flag"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// BundleInfo is bundle.json. It must agree with src/klos/bundle.go.
type BundleFile struct {
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type BundleInfo struct {
	Distribution string                `json:"distribution"`
	Flavor       string                `json:"flavor"`
	Version      string                `json:"version"`
	Created      int64                 `json:"created"`
	SlotSize     int64                 `json:"slot_size"`
	RootHash     string                `json:"roothash"`
	HashOffset   int64                 `json:"hashoffset"`
	Files        map[string]BundleFile `json:"files"`
}

// BuildRecord is build.json next to the bundle. "klos lock" reads it.
type BuildRecord struct {
	Flavor        string            `json:"flavor"`
	Version       string            `json:"version"`
	Kernel        string            `json:"kernel"`
	KernelConfig  string            `json:"kernel_config_sha256"`
	RootHash      string            `json:"roothash"`
	RootImage     string            `json:"root_img_sha256"`
	EFI           string            `json:"efi_sha256"`
	EFILive       string            `json:"efi_live_sha256"`
	Packages      []PkgMeta         `json:"packages"`
	HostPackages  map[string]string `json:"host_packages"`
	Cmdline       string            `json:"cmdline"`
	BundleTarball string            `json:"bundle_tarball"`
}

func parseSize(s string) int64 {
	m := regexp.MustCompile(`^([0-9]+)([KMG]?)$`).FindStringSubmatch(s)
	if m == nil {
		die("bad size %q (use for example 512M or 2G)", s)
	}
	n, _ := strconv.ParseInt(m[1], 10, 64)
	switch m[2] {
	case "K":
		n <<= 10
	case "M":
		n <<= 20
	case "G":
		n <<= 30
	}
	return n
}

// det makes deterministic values (UUIDs, salts) from a name.
func det(parts ...string) []byte {
	h := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return h[:]
}

func detUUID(parts ...string) string {
	b := det(parts...)
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func imageDir(flavor string) string { return filepath.Join(WORK, "image", flavor) }

// imageOptions are the settings of one image build.
type imageOptions struct {
	Version    string // the OS version
	SecureBoot bool   // sign the EFI binaries
	// Users and Groups are system accounts in addition to the manifest
	// accounts. The test uses them to check the account merge of updates.
	Users  []User
	Groups []Group
}

// defaultImageOptions are the settings from config/klos.toml (and
// KLOS_VERSION).
func defaultImageOptions() imageOptions {
	return imageOptions{Version: VERSION, SecureBoot: CONFIG.Image.SecureBoot}
}

func outDir(flavor, version string) string { return filepath.Join(OUT, "images", flavor, version) }

// listTarball returns the non-directory paths in a package tarball.
func listTarball(tb string) []string {
	out := output("tar", "--zstd", "-tf", tb)
	var files []string
	for _, l := range strings.Split(out, "\n") {
		l = strings.TrimPrefix(l, "./")
		if l == "" || strings.HasSuffix(l, "/") {
			continue
		}
		files = append(files, l)
	}
	return files
}

func excluded(p string, extra []string) bool {
	for _, pat := range append(append([]string{}, CONFIG.Image.Exclude...), extra...) {
		if ok, _ := path.Match(pat, p); ok {
			return true
		}
		if strings.HasSuffix(pat, "/*") && strings.HasPrefix(p, strings.TrimSuffix(pat, "*")) {
			return true
		}
	}
	return false
}

func assembleRootfs(m *Manifest, rootfs string) []string {
	build(m.Packages.Install, false)
	pkgs := runtimeClosure(m.Packages.Install)
	pkgExcludes := func(p string) []string {
		ex := append([]string{}, m.Rootfs.Exclude...)
		for _, e := range m.Rootfs.PackageExclude {
			name, pat, ok := strings.Cut(e, ":")
			if !ok {
				die("bad package-exclude entry %q (use package:pattern)", e)
			}
			if name == p {
				ex = append(ex, pat)
			}
		}
		return ex
	}
	owners := map[string]string{}
	for _, p := range pkgs {
		meta := pkgMeta(p)
		if meta.Kind != "target" {
			die("package %s is a host package; it cannot be in an image", p)
		}
		for _, f := range listTarball(pkgTarball(p)) {
			if excluded(f, pkgExcludes(p)) {
				continue
			}
			if o, ok := owners[f]; ok {
				die("file conflict: /%s is in %s and %s", f, o, p)
			}
			owners[f] = p
		}
	}
	for _, p := range pkgs {
		extractTarball(pkgTarball(p), rootfs, append(append([]string{}, CONFIG.Image.Exclude...), pkgExcludes(p)...))
	}
	for _, o := range m.Rootfs.Overlays {
		src := filepath.Join(TOP, "rootfs", o)
		if !exists(src) {
			die("no overlay directory rootfs/%s", o)
		}
		run(Cmd{Args: []string{"cp", "-a", "--no-preserve=ownership", src + "/.", rootfs + "/"}})
	}
	for _, d := range m.Rootfs.Dirs {
		mkdirAll(filepath.Join(rootfs, d))
	}
	checkScripts(m, rootfs)
	return pkgs
}

// checkScripts stops the build if the image has an executable script that
// the manifest does not allow, or a setuid or setgid file. KLOS has no
// interpreted programs, except the shell for administrators and the scripts
// in [rootfs] scripts, and no setuid or setgid programs.
func checkScripts(m *Manifest, rootfs string) {
	var bad []string
	check(filepath.Walk(rootfs, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode()&(os.ModeSetuid|os.ModeSetgid) != 0 {
			r, _ := filepath.Rel(rootfs, p)
			bad = append(bad, r+" (setuid or setgid)")
		}
		if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
			return nil
		}
		f, err := os.Open(p)
		check(err)
		head := make([]byte, 2)
		n, _ := io.ReadFull(f, head) // a short file is not a script
		closeQuietly(f)
		if n == 2 && string(head) == "#!" {
			r, _ := filepath.Rel(rootfs, p)
			for _, pat := range m.Rootfs.Scripts {
				if ok, _ := path.Match(pat, r); ok {
					return nil
				}
			}
			bad = append(bad, r)
		}
		return nil
	}))
	if len(bad) > 0 {
		sort.Strings(bad)
		die("the image has files that are not allowed:\n  %s", strings.Join(bad, "\n  "))
	}
}

func writeAccounts(m *Manifest, rootfs string) {
	days := SDE / 86400
	var passwd, group, shadow strings.Builder
	members := map[string][]string{}
	for _, u := range m.Users {
		for _, g := range u.Groups {
			members[g] = append(members[g], u.Name)
		}
		fmt.Fprintf(&passwd, "%s:x:%d:%d:%s:%s:%s\n", u.Name, u.UID, u.GID, u.Gecos, u.Home, u.Shell)
		fmt.Fprintf(&shadow, "%s:!:%d::::::\n", u.Name, days)
	}
	for _, g := range m.Groups {
		fmt.Fprintf(&group, "%s:x:%d:%s\n", g.Name, g.GID, strings.Join(members[g.Name], ","))
	}
	writeFile(filepath.Join(rootfs, "etc/passwd"), passwd.String(), 0o644)
	writeFile(filepath.Join(rootfs, "etc/group"), group.String(), 0o644)
	writeFile(filepath.Join(rootfs, "etc/shadow"), shadow.String(), 0o600)
}

func writeServices(m *Manifest, rootfs string) {
	bootd := filepath.Join(rootfs, "etc/dinit.d/boot.d")
	removeAll(bootd)
	mkdirAll(bootd)
	for _, s := range m.Services.Enable {
		if !exists(filepath.Join(rootfs, "etc/dinit.d", s)) {
			die("service %q is enabled, but etc/dinit.d/%s does not exist", s, s)
		}
		symlink("../"+s, filepath.Join(bootd, s))
	}
}

func writeOSRelease(m *Manifest, rootfs, version string) {
	d := CONFIG.Distribution
	writeFile(filepath.Join(rootfs, "usr/lib/os-release"), fmt.Sprintf(
		"NAME=%q\nID=%s\nVERSION_ID=%s\nVERSION=%q\nPRETTY_NAME=%q\nVARIANT=%q\nVARIANT_ID=%s\n",
		d.Name, d.ID, version, version+" ("+m.Flavor.Name+")",
		d.Name+" "+version+" ("+m.Flavor.Name+")", m.Flavor.Description, m.Flavor.Name), 0o644)
	removeIfExists(filepath.Join(rootfs, "etc/os-release"))
	symlink("../usr/lib/os-release", filepath.Join(rootfs, "etc/os-release"))
}

// Signing keys: secrets/bundle.key (PKCS#8 PEM) and secrets/bundle.pub (PKIX PEM).

func cmdKeygen(args []string) {
	force := len(args) > 0 && (args[0] == "-force" || args[0] == "--force")
	key, pub := filepath.Join(SECRETS, "bundle.key"), filepath.Join(SECRETS, "bundle.pub")
	if exists(key) && !force {
		die("%s exists (use -force to replace it)", rel(key))
	}
	makeKey(key, pub)
}

func makeKey(key, pub string) {
	pk, sk, err := ed25519.GenerateKey(rand.Reader)
	check(err)
	skb, err := x509.MarshalPKCS8PrivateKey(sk)
	check(err)
	pkb, err := x509.MarshalPKIXPublicKey(pk)
	check(err)
	mkdirAll(SECRETS)
	writeFile(key, string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: skb})), 0o600)
	writeFile(pub, string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pkb})), 0o644)
	logf("made %s and %s", rel(key), rel(pub))
	logf("this is a development key without a password; keep release keys offline")
}

func signingKey() (ed25519.PrivateKey, string) {
	key, pub := filepath.Join(SECRETS, "bundle.key"), filepath.Join(SECRETS, "bundle.pub")
	if !exists(key) {
		logf("no signing key: make a development key")
		makeKey(key, pub)
	}
	b, err := os.ReadFile(key)
	check(err)
	blk, _ := pem.Decode(b)
	if blk == nil {
		die("%s: no PEM data", rel(key))
	}
	k, err := x509.ParsePKCS8PrivateKey(blk.Bytes)
	check(err)
	sk, ok := k.(ed25519.PrivateKey)
	if !ok {
		die("%s: not an Ed25519 key", rel(key))
	}
	return sk, pub
}

// addELF adds a file from rootfs to the initramfs, with its symlink targets
// and the shared libraries that it needs.
func addELF(c *cpioWriter, rootfs, rel string, seen map[string]bool) {
	rel = strings.TrimPrefix(path.Clean("/"+rel), "/")
	// Use the real directory (for example usr/lib, not lib): the cpio
	// order is sorted, so a path through a symlink can come too early.
	if real, err := filepath.EvalSymlinks(filepath.Join(rootfs, path.Dir(rel))); err == nil {
		if r, err := filepath.Rel(rootfs, real); err == nil && !strings.HasPrefix(r, "..") {
			rel = path.Join(r, path.Base(rel))
		}
	}
	if seen[rel] {
		return
	}
	seen[rel] = true
	p := filepath.Join(rootfs, rel)
	st, err := os.Lstat(p)
	if err != nil {
		die("initramfs: %s is not in the image", rel)
	}
	addParents(c, rel)
	if st.Mode()&os.ModeSymlink != 0 {
		t, err := os.Readlink(p)
		check(err)
		c.symlink(rel, t)
		if !strings.HasPrefix(t, "/") {
			t = path.Join(path.Dir(rel), t)
		}
		addELF(c, rootfs, t, seen)
		return
	}
	data, err := os.ReadFile(p)
	check(err)
	c.file(rel, uint32(st.Mode().Perm()), data)
	f, err := elf.Open(p)
	if err != nil {
		return // not an ELF file
	}
	defer closeQuietly(f)
	for _, prog := range f.Progs {
		if prog.Type == elf.PT_INTERP {
			b := make([]byte, prog.Filesz)
			if _, err := prog.ReadAt(b, 0); err != nil {
				die("%s: read the ELF interpreter: %v", rel, err)
			}
			addELF(c, rootfs, strings.TrimRight(string(b), "\x00"), seen)
		}
	}
	libs, _ := f.DynString(elf.DT_NEEDED)
	for _, l := range libs {
		addELF(c, rootfs, "usr/lib/"+l, seen)
	}
}

func addParents(c *cpioWriter, rel string) {
	d := path.Dir(rel)
	for d != "." && d != "/" {
		if _, ok := c.entries[d]; !ok {
			c.dir(d, 0o755)
		}
		d = path.Dir(d)
	}
}

func buildInitramfs(m *Manifest, rootfs, out string) {
	c := newCpio()
	for _, d := range []string{"dev", "proc", "sys", "run", "sysroot", "etc", "tmp", "usr", "usr/bin", "usr/lib"} {
		c.dir(d, 0o755)
	}
	c.symlink("bin", "usr/bin")
	c.symlink("sbin", "usr/bin")
	c.symlink("lib", "usr/lib")
	c.chardev("dev/console", 0o600, 5, 1)
	c.chardev("dev/null", 0o666, 1, 3)
	klos, err := os.ReadFile(filepath.Join(rootfs, "usr/bin/klos"))
	check(err)
	c.file("init", 0o755, klos)
	seen := map[string]bool{}
	for _, f := range m.Initramfs.Files {
		addELF(c, rootfs, f, seen)
	}
	c.write(out)
}

func fileInfo(p string) BundleFile {
	st, err := os.Stat(p)
	check(err)
	return BundleFile{Size: st.Size(), SHA256: sha256File(p)}
}

func buildImage(flavor string, opt imageOptions) *BuildRecord {
	version := opt.Version
	m := flavorManifest(flavor)
	iw := imageDir(flavor)
	rootfs := filepath.Join(iw, "rootfs")
	removeAll(iw)
	mkdirAll(rootfs)
	logf("image %s %s: assemble the root file system", flavor, version)
	pkgs := assembleRootfs(m, rootfs)
	accounts := *m
	accounts.Users = append(slices.Clone(m.Users), opt.Users...)
	accounts.Groups = append(slices.Clone(m.Groups), opt.Groups...)
	writeAccounts(&accounts, rootfs)
	writeServices(m, rootfs)
	writeOSRelease(m, rootfs, version)
	sk, pub := signingKey()
	copyFile(pub, filepath.Join(rootfs, "usr/share/klos/bundle.pub"))
	var sbKeys map[string]*sbKey
	if opt.SecureBoot {
		sbKeys = ensureSecureBootKeys()
		writeSecureBootFiles(sbKeys, filepath.Join(rootfs, "usr/share/klos/secureboot"))
	}
	var list strings.Builder
	rec := &BuildRecord{Flavor: flavor, Version: version, HostPackages: map[string]string{}}
	for _, p := range pkgs {
		meta := pkgMeta(p)
		rec.Packages = append(rec.Packages, *meta)
		fmt.Fprintf(&list, "%s %s %s\n", meta.Name, meta.Version, meta.SHA256)
	}
	// All host tools that the packages of the flavor need (the toolchain,
	// Go, build modules).
	for _, r := range topoClosure(m.Packages.Install) {
		if r.Kind == "host" {
			meta := pkgMeta(r.Name)
			rec.HostPackages[r.Name] = meta.Version + " " + meta.SHA256
		}
	}
	writeFile(filepath.Join(rootfs, "usr/lib/klos/packages.txt"), list.String(), 0o644)

	kb := newKernelBuild(flavor)
	kb.configure(m)
	kb.stage1(rootfs)

	fc := buildSELinux(m, rootfs, kernelSource())

	logf("image %s: make the erofs image", flavor)
	img := filepath.Join(iw, "root.img")
	uuid := detUUID("erofs", flavor, version)
	// The tar archive has the SELinux labels (the builder cannot set
	// security.selinux on the files of rootfs).
	rootTar := filepath.Join(iw, "root.tar")
	tf, err := os.Create(rootTar)
	check(err)
	bw := bufio.NewWriterSize(tf, 1<<20)
	if err := writeLabeledTar(rootfs, fc, bw); err != nil {
		die("image %s: %v", flavor, err)
	}
	check(bw.Flush())
	check(tf.Close())
	run(Cmd{Args: []string{"mkfs.erofs", "--tar=f", "-zlz4hc,12", "-T", strconv.FormatInt(SDE, 10),
		"--ignore-mtime", "--all-root", "-U", uuid, img, rootTar}, Quiet: true})
	check(os.Remove(rootTar))
	st, err := os.Stat(img)
	check(err)
	dataSize := (st.Size() + 4095) / 4096 * 4096
	check(os.Truncate(img, dataSize))
	salt := hex.EncodeToString(det("verity", flavor, version))
	vout := output("veritysetup", "format", img, img,
		"--hash-offset="+strconv.FormatInt(dataSize, 10),
		"--data-blocks="+strconv.FormatInt(dataSize/4096, 10),
		"--salt="+salt, "--uuid="+detUUID("verity", flavor, version))
	roothash := ""
	for _, l := range strings.Split(vout, "\n") {
		if strings.HasPrefix(l, "Root hash:") {
			roothash = strings.TrimSpace(strings.TrimPrefix(l, "Root hash:"))
		}
	}
	if len(roothash) != 64 {
		die("veritysetup did not give a root hash:\n%s", vout)
	}
	slot := parseSize(m.Flavor.SlotSize)
	if st, _ := os.Stat(img); st.Size() > slot {
		die("the OS image (%d MiB) is larger than slot-size %s", st.Size()>>20, m.Flavor.SlotSize)
	}

	logf("image %s: make the initramfs and the EFI binaries", flavor)
	cpio := filepath.Join(iw, "initramfs.cpio")
	buildInitramfs(m, rootfs, cpio)
	args := append([]string{}, m.Kernel.Cmdline...)
	if fc != nil && seMode(m) == "enforcing" {
		// SELinux enforces from the policy load: no process (not even
		// /init) needs the permission to change the mode.
		args = append(args, "enforcing=1")
	}
	cmdline := strings.Join(append(args,
		"klos.version="+version, "klos.flavor="+flavor,
		"klos.verity="+roothash, "klos.hashoffset="+strconv.FormatInt(dataSize, 10)), " ")
	rec.Cmdline = cmdline
	efi := filepath.Join(iw, "klos.efi")
	efiLive := filepath.Join(iw, "klos-live.efi")
	kb.link(cpio, cmdline+" klos.mode=installed", efi)
	kb.link(cpio, cmdline+" klos.mode=live", efiLive)
	if sbKeys != nil {
		signPE(efi, sbKeys["db"])
		signPE(efiLive, sbKeys["db"])
		logf("image %s: signed the EFI binaries for Secure Boot", flavor)
	}
	rec.Kernel = kb.release()
	rec.KernelConfig = sha256File(filepath.Join(kb.obj, ".config"))

	od := outDir(flavor, version)
	removeAll(od)
	mkdirAll(od)
	for _, f := range []string{"root.img", "klos.efi", "klos-live.efi"} {
		moveFile(filepath.Join(iw, f), filepath.Join(od, f))
	}
	copyFile(filepath.Join(kb.obj, ".config"), filepath.Join(od, "kernel.config"))
	writeFile(filepath.Join(od, "packages.txt"), list.String(), 0o644)
	info := BundleInfo{
		Distribution: CONFIG.Distribution.Name, Flavor: flavor, Version: version,
		Created: SDE, SlotSize: slot, RootHash: roothash, HashOffset: dataSize,
		Files: map[string]BundleFile{
			"root.img": fileInfo(filepath.Join(od, "root.img")),
			"klos.efi": fileInfo(filepath.Join(od, "klos.efi")),
		},
	}
	data, _ := json.MarshalIndent(info, "", " ")
	data = append(data, '\n')
	writeFile(filepath.Join(od, "bundle.json"), string(data), 0o644)
	writeFile(filepath.Join(od, "bundle.sig"),
		base64.StdEncoding.EncodeToString(ed25519.Sign(sk, data))+"\n", 0o644)
	tarball := filepath.Join(od, fmt.Sprintf("klos-%s-%s.tar", flavor, version))
	run(Cmd{Args: []string{"tar", "-C", od, "--format=posix",
		"--pax-option=exthdr.name=%d/PaxHeaders/%f,delete=atime,delete=ctime",
		fmt.Sprintf("--mtime=@%d", SDE), "--owner=0", "--group=0", "--numeric-owner",
		"-cf", tarball, "bundle.json", "bundle.sig", "root.img", "klos.efi"}})
	rec.RootHash = roothash
	rec.RootImage = info.Files["root.img"].SHA256
	rec.EFI = info.Files["klos.efi"].SHA256
	rec.EFILive = sha256File(filepath.Join(od, "klos-live.efi"))
	rec.BundleTarball = rel(tarball)
	b, _ := json.MarshalIndent(rec, "", " ")
	writeFile(filepath.Join(od, "build.json"), string(b)+"\n", 0o644)
	logf("image %s %s is ready in %s", flavor, version, rel(od))
	logf("  root hash %s", roothash)
	logf("  root.img  %s (%d MiB)", rec.RootImage, info.Files["root.img"].Size>>20)
	logf("  klos.efi  %s", rec.EFI)
	return rec
}

func cmdImage(args []string) {
	if len(args) != 1 {
		die("usage: image <flavor>")
	}
	buildImage(args[0], defaultImageOptions())
}

func readBuildRecord(flavor string) *BuildRecord {
	p := filepath.Join(outDir(flavor, VERSION), "build.json")
	b, err := os.ReadFile(p)
	if err != nil {
		die("no image for %s %s (run './klos image %s')", flavor, VERSION, flavor)
	}
	rec := &BuildRecord{}
	check(json.Unmarshal(b, rec))
	return rec
}

func lockText(rec *BuildRecord) string {
	var s strings.Builder
	fmt.Fprintf(&s, "# The lock file of the KLOS flavor %q. \"./klos lock %s\" writes it.\n", rec.Flavor, rec.Flavor)
	fmt.Fprintf(&s, "# \"./klos verify %s\" builds the flavor again and compares the results.\n\n", rec.Flavor)
	fmt.Fprintf(&s, "[image]\nflavor = %q\nversion = %q\nroothash = %q\nroot_img_sha256 = %q\nefi_sha256 = %q\nefi_live_sha256 = %q\n\n",
		rec.Flavor, rec.Version, rec.RootHash, rec.RootImage, rec.EFI, rec.EFILive)
	fmt.Fprintf(&s, "[kernel]\nrelease = %q\nconfig_sha256 = %q\n\n", rec.Kernel, rec.KernelConfig)
	var hosts []string
	for h := range rec.HostPackages {
		hosts = append(hosts, h)
	}
	sort.Strings(hosts)
	for _, h := range hosts {
		f := strings.Fields(rec.HostPackages[h])
		fmt.Fprintf(&s, "[[tool]]\nname = %q\nversion = %q\nsha256 = %q\n\n", h, f[0], f[1])
	}
	for _, p := range rec.Packages {
		fmt.Fprintf(&s, "[[package]]\nname = %q\nversion = %q\nsha256 = %q\n\n",
			p.Name, p.Version, p.SHA256)
	}
	return s.String()
}

// reproducible returns the part of a lock text that must be the same for
// all builders: the image, the kernel and the packages. The [[tool]]
// entries (the toolchain of the builder) can be different.
func reproducible(lock string) string {
	var out []string
	skip := false
	for _, l := range strings.Split(lock, "\n") {
		if strings.HasPrefix(l, "[") {
			skip = l == "[[tool]]"
		}
		if !skip && !strings.HasPrefix(l, "#") {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}

func cmdLock(args []string) {
	if len(args) != 1 {
		die("usage: lock <flavor>")
	}
	rec := readBuildRecord(args[0])
	p := filepath.Join(TOP, "manifests", args[0]+".lock")
	writeFile(p, lockText(rec), 0o644)
	logf("wrote %s", rel(p))
}

// cmdVerify builds all target packages and the image again in a separate
// output directory and compares the results with the lock file. The host
// tools (llvm, go) come from the normal output directory.
func cmdVerify(args []string) {
	if len(args) != 1 {
		die("usage: verify <flavor>")
	}
	flavor := args[0]
	lockPath := filepath.Join(TOP, "manifests", flavor+".lock")
	want, err := os.ReadFile(lockPath)
	if err != nil {
		die("no lock file %s (run './klos lock %s')", rel(lockPath), flavor)
	}
	// The build directories (work/build, work/kernel) stay the same: some
	// programs keep their build paths (for example a configure line). The
	// packages and images go to out/verify. The kernel compiles again from
	// the start.
	HOSTPKGOUT = PKGOUT
	PKGOUT = filepath.Join(OUT, "verify", "pkgs")
	OUT = filepath.Join(OUT, "verify")
	removeAll(PKGOUT)
	removeAll(filepath.Join(OUT, "images", flavor))
	removeAll(filepath.Join(WORK, "kernel", flavor))
	rec := buildImage(flavor, defaultImageOptions())
	got := lockText(rec)
	if reproducible(got) != reproducible(string(want)) {
		diffPath := filepath.Join(OUT, flavor+".lock")
		writeFile(diffPath, got, 0o644)
		// diff exits with 1 when the files are different; show the output only.
		_ = tryRun(Cmd{Args: []string{"diff", "-u", lockPath, diffPath}})
		die("the build is not the same as %s", rel(lockPath))
	}
	if got != string(want) {
		logf("verify %s: the toolchain of this builder is not the same as in %s (this is not an error)", flavor, rel(lockPath))
	}
	logf("verify %s: the build is the same as %s", flavor, rel(lockPath))
}

// cmdCompare compares the images of a flavor from two builders.
func cmdCompare(args []string) {
	fs := flag.NewFlagSet("compare", flag.ContinueOnError)
	a := fs.String("a", "alpine-2", "the first builder")
	b := fs.String("b", "arch-2", "the second builder")
	if err := fs.Parse(args); err != nil {
		die("compare: %v", err)
	}
	if fs.NArg() != 1 {
		die("usage: compare [-a builder] [-b builder] <flavor>")
	}
	flavor := fs.Arg(0)
	read := func(builder string) string {
		p := filepath.Join(TOP, "out", builder, "images", flavor, VERSION, "build.json")
		data, err := os.ReadFile(p)
		if err != nil {
			die("no image of %s from %s (run 'KLOS_BUILDER=%s ./klos image %s')", flavor, builder, builder, flavor)
		}
		rec := &BuildRecord{}
		check(json.Unmarshal(data, rec))
		return lockText(rec)
	}
	ta, tb := read(*a), read(*b)
	dir := filepath.Join(TOP, "out", "compare")
	mkdirAll(dir)
	pa, pb := filepath.Join(dir, flavor+"."+*a+".lock"), filepath.Join(dir, flavor+"."+*b+".lock")
	writeFile(pa, ta, 0o644)
	writeFile(pb, tb, 0o644)
	if reproducible(ta) != reproducible(tb) {
		// diff exits with 1 when the files are different; show the output only.
		_ = tryRun(Cmd{Args: []string{"diff", "-u", pa, pb}})
		die("compare %s: the builds of %s and %s are not the same", flavor, *a, *b)
	}
	if ta != tb {
		_ = tryRun(Cmd{Args: []string{"diff", "-u", pa, pb}})
		logf("compare %s: the toolchains are not the same (this is not an error)", flavor)
	}
	logf("compare %s: %s and %s give the same image and packages", flavor, *a, *b)
}
