// SPDX-License-Identifier: BSD-2-Clause

// Command kbuild is the KLOS build tool.
//
// It runs in the bootstrap builder container (see ./klos). It reads recipes
// (pkgs/*/recipe.toml), flavor manifests (manifests/*.toml) and the kernel
// definition (kernel/linux.toml). It builds packages, OS images, kernels,
// update bundles and installer disks. Read docs/build-system.md.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type command struct {
	usage string
	run   func(args []string)
}

var commands = map[string]command{
	"fetch":         {"fetch <target>...", cmdFetch},
	"build":         {"build [-force] <package>...", cmdBuild},
	"list":          {"list", cmdList},
	"keygen":        {"keygen [-force]", cmdKeygen},
	"image":         {"image <flavor>", cmdImage},
	"installer":     {"installer <flavor>", cmdInstaller},
	"lock":          {"lock <flavor>", cmdLock},
	"verify":        {"verify <flavor>", cmdVerify},
	"test":          {"test [-prebuilt] <flavor>", cmdTest},
	"test-images":   {"test-images <flavor>", cmdTestImages},
	"qemu":          {"qemu <flavor>", cmdQemu},
	"hash":          {"hash <url>", cmdHash},
	"revoke":        {"revoke <klos.efi>...", cmdRevoke},
	"selinux":       {"selinux <flavor>", cmdSELinux},
	"builder-image": {"builder-image", cmdBuilderImage},
	"check-sources": {"check-sources", cmdCheckSources},
	"compare":       {"compare [-a builder] [-b builder] <flavor>", cmdCompare},
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: kbuild <command> [arguments]")
	var names []string
	for n := range commands {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		fmt.Fprintf(os.Stderr, "  %s\n", commands[n].usage)
	}
	os.Exit(2)
}

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	exe, err := os.Executable()
	if err == nil && os.Getenv("KLOS_TOP") == "" {
		// The binary is in work/bin; the repository is two levels up.
		check(os.Setenv("KLOS_TOP", filepath.Dir(filepath.Dir(filepath.Dir(exe)))))
	}
	initPaths()
	c, ok := commands[os.Args[1]]
	if !ok {
		usage()
	}
	c.run(os.Args[2:])
}

func cmdFlags(name string, args []string, force *bool) []string {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	if force != nil {
		fs.BoolVar(force, "force", false, "build again even if the output is current")
	} else {
		// "fetch" gets the arguments of the other commands; it ignores their flags.
		fs.Bool("force", false, "ignored")
		fs.String("config", "", "ignored")
		fs.Bool("prebuilt", false, "ignored")
	}
	if err := fs.Parse(args); err != nil {
		die("%s: %v", name, err)
	}
	if fs.NArg() == 0 && name != "list" {
		die("%s: no targets given", name)
	}
	return fs.Args()
}

func cmdFetch(args []string) {
	targets := cmdFlags("fetch", args, nil)
	var names []string
	var sources []Source
	for _, t := range targets {
		if t == "builder" {
			bm := loadBuilderManifest()
			names = append(append(names, bm.Builder.Packages...), bm.Builder.HostPackages...)
		} else if isFlavor(t) {
			m := flavorManifest(t)
			names = append(names, m.Packages.Install...)
			if len(m.SELinux.Policy) > 0 {
				names = append(names, "secilc")
			}
			sources = append(sources, kernelDef().Source...)
		} else if t == "kernel" {
			sources = append(sources, kernelDef().Source...)
		} else {
			names = append(names, t)
		}
	}
	for _, r := range topoClosure(names) {
		sources = append(sources, r.Source...)
	}
	for _, s := range sources {
		fetchSource(s)
	}
	logf("all %d sources are present and verified", len(sources))
}

func cmdBuild(args []string) {
	var force bool
	targets := cmdFlags("build", args, &force)
	build(targets, force)
}

func cmdList(args []string) {
	rs := recipes()
	var names []string
	for n := range rs {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		r := rs[n]
		state := "-"
		if isCurrent(r) {
			state = "built"
		}
		fmt.Printf("%-24s %-18s %-7s %-6s %s\n", r.Name, r.FullVersion(), r.Kind, state, r.Summary)
	}
}

// cmdHash prints the SHA-256 of a URL. Use it to write new recipes.
func cmdHash(args []string) {
	for _, u := range args {
		tmp := filepath.Join(WORK, "hash.tmp")
		mkdirAll(WORK)
		download(u, tmp)
		fmt.Printf("%s  %s\n", sha256File(tmp), u)
		check(os.Remove(tmp))
	}
}

func rel(p string) string {
	r, err := filepath.Rel(TOP, p)
	if err != nil || strings.HasPrefix(r, "..") {
		return p
	}
	return r
}

// cmdCheckSources checks the SHA-256 and the signature of all sources of all
// recipes and of the kernel again (it downloads the signature files that
// are not in the cache).
func cmdCheckSources(args []string) {
	if len(args) != 0 {
		die("usage: check-sources")
	}
	sources := append([]Source{}, kernelDef().Source...)
	var names []string
	for n := range recipes() {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		sources = append(sources, recipes()[n].Source...)
	}
	for _, s := range sources {
		p := fetchSource(s)
		if s.Signature != "" {
			verifySignature(s, p)
		}
	}
	logf("all %d sources have a correct SHA-256 value and signature", len(sources))
}
