// SPDX-License-Identifier: BSD-2-Clause

package main

// The kernel build. Read docs/kernel.md.
//
// Stage 1 compiles the kernel (and the modules, if the flavor has them)
// from "allnoconfig" plus the configuration fragments of the flavor.
// Stage 2 links the final EFI binary with the initramfs and the command
// line in it. Stage 2 is quick, because only a few files change.

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type kernelBuild struct {
	flavor string
	src    string // the source tree
	obj    string // the build directory (O=)
	env    []string
	llvm   string
}

func kernelSource() string {
	k := kernelDef()
	dir := filepath.Join(WORK, "kernel", "src-"+k.Version)
	top := filepath.Join(dir, "linux-"+k.Version)
	if exists(filepath.Join(dir, ".done")) {
		return top
	}
	removeAll(dir)
	mkdirAll(dir)
	for _, s := range k.Source {
		run(Cmd{Args: []string{"tar", "-xf", checkSource(s), "-C", dir, "--no-same-owner"}})
	}
	patches, _ := filepath.Glob(filepath.Join(TOP, "kernel", "patches", "*.patch"))
	for _, p := range patches {
		logf("kernel: apply %s", filepath.Base(p))
		run(Cmd{Args: []string{"patch", "-p1", "-N", "-i", p}, Dir: top, Quiet: true})
	}
	writeFile(filepath.Join(dir, ".done"), "", 0o644)
	return top
}

var configLine = regexp.MustCompile(`^(CONFIG_[A-Za-z0-9_]+)=(.*)$`)

// readConfigFragments returns the options of the fragments, in order.
// readFragment returns the options of one configuration fragment, in order.
func readFragment(p string) ([][2]string, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer closeQuietly(f)
	var opts [][2]string
	s := bufio.NewScanner(f)
	for s.Scan() {
		l := strings.TrimSpace(s.Text())
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		m := configLine.FindStringSubmatch(l)
		if m == nil {
			return nil, fmt.Errorf("bad line %q", l)
		}
		opts = append(opts, [2]string{m[1], m[2]})
	}
	return opts, s.Err()
}

// readConfigFragments returns the options of the fragments, in order. A
// later fragment replaces the value of an earlier fragment.
func readConfigFragments(names []string) ([][2]string, string) {
	var opts [][2]string
	index := map[string]int{}
	for _, n := range names {
		p := filepath.Join(TOP, "kernel", "config", n+".config")
		frag, err := readFragment(p)
		if err != nil {
			die("kernel config fragment %s: %v", rel(p), err)
		}
		for _, o := range frag {
			if i, ok := index[o[0]]; ok {
				opts[i][1] = o[1]
				continue
			}
			index[o[0]] = len(opts)
			opts = append(opts, o)
		}
	}
	var text strings.Builder
	for _, o := range opts {
		if o[1] == "n" {
			fmt.Fprintf(&text, "# %s is not set\n", o[0])
		} else {
			fmt.Fprintf(&text, "%s=%s\n", o[0], o[1])
		}
	}
	return opts, text.String()
}

func readKconfig(path string) map[string]string {
	b, err := os.ReadFile(path)
	check(err)
	out := map[string]string{}
	for _, l := range strings.Split(string(b), "\n") {
		if m := configLine.FindStringSubmatch(l); m != nil {
			out[m[1]] = m[2]
		} else if strings.HasPrefix(l, "# CONFIG_") && strings.HasSuffix(l, " is not set") {
			out[strings.Fields(l)[1]] = "n"
		}
	}
	return out
}

func newKernelBuild(flavor string) *kernelBuild {
	build([]string{"llvm", "compiler-rt"}, false)
	llvm := filepath.Join(toolchainRoot(), toolchainPrefix, "bin")
	stamp := time.Unix(SDE, 0).UTC().Format("Mon Jan 2 15:04:05 UTC 2006")
	kb := &kernelBuild{
		flavor: flavor,
		src:    kernelSource(),
		obj:    filepath.Join(WORK, "kernel", flavor),
		llvm:   llvm,
		env: append(os.Environ(),
			"KBUILD_BUILD_TIMESTAMP="+stamp,
			"KBUILD_BUILD_USER=klos",
			"KBUILD_BUILD_HOST=klos",
			"KBUILD_BUILD_VERSION=1",
			"PATH="+llvm+":/usr/bin:/bin",
			"LC_ALL=C"),
	}
	return kb
}

func (kb *kernelBuild) make(args ...string) {
	a := []string{"make", "-C", kb.src, "O=" + kb.obj, "LLVM=" + kb.llvm + "/",
		"HOSTCC=gcc", "HOSTCXX=g++", "-j" + JOBS}
	run(Cmd{Args: append(a, args...), Env: kb.env, Stdout: os.Stderr})
}

// configure writes the .config and checks it against the fragments.
func (kb *kernelBuild) configure(m *Manifest) {
	opts, text := readConfigFragments(m.Kernel.Config)
	for _, o := range opts {
		if o[0] == "CONFIG_MODULE_SIG_KEY" {
			moduleSigningKey(strings.Trim(o[1], `"`))
		}
	}
	frag := filepath.Join(kb.obj, "klos-fragments.config")
	mkdirAll(kb.obj)
	old, _ := os.ReadFile(frag)
	if string(old) == text && exists(filepath.Join(kb.obj, ".config")) {
		return
	}
	writeFile(frag, text, 0o644)
	kb.make("KCONFIG_ALLCONFIG="+frag, "allnoconfig")
	cfg := readKconfig(filepath.Join(kb.obj, ".config"))
	var bad []string
	for _, o := range opts {
		got, ok := cfg[o[0]]
		if !ok {
			got = "n"
		}
		if got != o[1] {
			bad = append(bad, fmt.Sprintf("%s: want %s, got %s", o[0], o[1], got))
		}
	}
	if len(bad) > 0 {
		removeIfExists(frag) // configure again next time
		die("kernel config for %s does not agree with the fragments:\n  %s",
			kb.flavor, strings.Join(bad, "\n  "))
	}
}

func (kb *kernelBuild) hasModules() bool {
	return readKconfig(filepath.Join(kb.obj, ".config"))["CONFIG_MODULES"] == "y"
}

// stage1 compiles the kernel and installs the modules into rootfs. A kernel
// without modules needs no stage 1: the final link compiles everything.
func (kb *kernelBuild) stage1(rootfs string) {
	if !kb.hasModules() {
		return
	}
	logf("kernel: compile for %s", kb.flavor)
	// The .config of an earlier build can name an initramfs file that does
	// not exist now. An empty file is an empty list for the kernel build.
	if src := strings.Trim(readKconfig(filepath.Join(kb.obj, ".config"))["CONFIG_INITRAMFS_SOURCE"], `"`); src != "" && !exists(src) {
		writeFile(src, "", 0o644)
	}
	kb.make("bzImage")
	kb.make("modules")
	// depmod comes from the image (package kmod). It runs with the KLOS
	// dynamic linker, because the builder has no kmod.
	depmod := filepath.Join(kb.obj, "klos-depmod")
	writeExec(depmod, fmt.Sprintf("#!/bin/sh\nexec \"%[1]s/usr/lib/libc.so\" --library-path \"%[1]s/usr/lib\" \"%[1]s/usr/bin/depmod\" \"$@\"\n", rootfs))
	if !exists(filepath.Join(rootfs, "usr/bin/depmod")) {
		die("the flavor has kernel modules, but no kmod package")
	}
	kb.make("INSTALL_MOD_PATH="+rootfs, "KERNEL_MODULE_DIRECTORY=/usr/lib/modules",
		"INSTALL_MOD_STRIP=1", "DEPMOD="+depmod, "modules_install")
	if !exists(filepath.Join(rootfs, "usr/lib/modules", kb.release(), "modules.dep")) {
		die("depmod did not make modules.dep")
	}
}

// link makes the final EFI binary with the initramfs and command line.
func (kb *kernelBuild) link(src, cmdline, out string) {
	cfg := filepath.Join(kb.obj, ".config")
	// A fixed path: the .config of the next build refers to it.
	cpio := filepath.Join(kb.obj, "klos-initramfs.cpio")
	copyFile(src, cpio)
	sc := filepath.Join(kb.src, "scripts", "config")
	run(Cmd{Args: []string{sc, "--file", cfg,
		"--enable", "CMDLINE_BOOL", "--set-str", "CMDLINE", cmdline,
		"--enable", "CMDLINE_OVERRIDE",
		"--set-str", "INITRAMFS_SOURCE", cpio,
		"--enable", "INITRAMFS_COMPRESSION_ZSTD"}})
	kb.make("olddefconfig")
	got := readKconfig(cfg)
	if got["CONFIG_INITRAMFS_COMPRESSION_ZSTD"] != "y" || got["CONFIG_CMDLINE_OVERRIDE"] != "y" {
		die("kernel: the initramfs or command line options did not apply")
	}
	kb.make("bzImage")
	copyFile(filepath.Join(kb.obj, "arch", "x86", "boot", "bzImage"), out)
}

func (kb *kernelBuild) release() string {
	b, err := os.ReadFile(filepath.Join(kb.obj, "include", "config", "kernel.release"))
	check(err)
	return strings.TrimSpace(string(b))
}

// moduleSigningKey makes the kernel module signing key one time. The same
// key gives the same signatures, so the builds stay reproducible. Keep the
// key secret: with it, a person can sign kernel modules for KLOS kernels.
func moduleSigningKey(path string) {
	if exists(path) {
		return
	}
	if !strings.HasPrefix(path, SECRETS+"/") {
		die("CONFIG_MODULE_SIG_KEY must be in %s", SECRETS)
	}
	mkdirAll(SECRETS)
	conf := path + ".genkey"
	writeFile(conf, `[ req ]
default_bits = 4096
distinguished_name = req_distinguished_name
prompt = no
string_mask = utf8only
x509_extensions = myexts

[ req_distinguished_name ]
O = KLOS
CN = KLOS kernel module signing key

[ myexts ]
basicConstraints=critical,CA:FALSE
keyUsage=digitalSignature
subjectKeyIdentifier=hash
authorityKeyIdentifier=keyid
`, 0o600)
	run(Cmd{Args: []string{"openssl", "req", "-new", "-nodes", "-utf8", "-sha512", "-days", "36500",
		"-batch", "-x509", "-config", conf, "-outform", "PEM", "-out", path, "-keyout", path}, Quiet: true})
	check(os.Remove(conf))
	check(os.Chmod(path, 0o600))
	logf("made the kernel module signing key %s", rel(path))
}
