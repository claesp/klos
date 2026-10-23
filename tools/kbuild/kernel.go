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
func readConfigFragments(names []string) ([][2]string, string) {
	var opts [][2]string
	var text strings.Builder
	for _, n := range names {
		p := filepath.Join(TOP, "kernel", "config", n+".config")
		f, err := os.Open(p)
		if err != nil {
			die("kernel config fragment %s: %v", rel(p), err)
		}
		s := bufio.NewScanner(f)
		for s.Scan() {
			l := strings.TrimSpace(s.Text())
			if l == "" || strings.HasPrefix(l, "#") {
				continue
			}
			m := configLine.FindStringSubmatch(l)
			if m == nil {
				die("%s: bad line %q", rel(p), l)
			}
			opts = append(opts, [2]string{m[1], m[2]})
			if m[2] == "n" {
				fmt.Fprintf(&text, "# %s is not set\n", m[1])
			} else {
				fmt.Fprintf(&text, "%s=%s\n", m[1], m[2])
			}
		}
		f.Close()
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
	build([]string{"llvm"}, false)
	llvm := filepath.Join(hostRoot("llvm"), toolchainPrefix, "bin")
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
		os.Remove(frag)
		die("kernel config for %s does not agree with the fragments:\n  %s",
			kb.flavor, strings.Join(bad, "\n  "))
	}
}

func (kb *kernelBuild) hasModules() bool {
	return readKconfig(filepath.Join(kb.obj, ".config"))["CONFIG_MODULES"] == "y"
}

// stage1 compiles the kernel and installs the modules into rootfs.
func (kb *kernelBuild) stage1(rootfs string) {
	logf("kernel: compile for %s", kb.flavor)
	kb.make("bzImage")
	if kb.hasModules() {
		kb.make("modules")
		kb.make("INSTALL_MOD_PATH="+filepath.Join(rootfs, "usr"), "INSTALL_MOD_STRIP=1", "modules_install")
	}
}

// link makes the final EFI binary with the initramfs and command line.
func (kb *kernelBuild) link(cpio, cmdline, out string) {
	cfg := filepath.Join(kb.obj, ".config")
	sc := filepath.Join(kb.src, "scripts", "config")
	run(Cmd{Args: []string{sc, "--file", cfg,
		"--enable", "CMDLINE_BOOL", "--set-str", "CMDLINE", cmdline,
		"--enable", "CMDLINE_OVERRIDE",
		"--set-str", "INITRAMFS_SOURCE", cpio}})
	kb.make("olddefconfig")
	kb.make("bzImage")
	copyFile(filepath.Join(kb.obj, "arch", "x86", "boot", "bzImage"), out)
}

func (kb *kernelBuild) release() string {
	b, err := os.ReadFile(filepath.Join(kb.obj, "include", "config", "kernel.release"))
	check(err)
	return strings.TrimSpace(string(b))
}
