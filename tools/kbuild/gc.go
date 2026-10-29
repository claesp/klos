// SPDX-License-Identifier: BSD-2-Clause

package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// cmdGC removes files that no recipe uses: sources in cache/sources,
// builder tarballs in cache/builder that are not the seed, and the packages,
// host tool directories and logs of packages that have no recipe now.
// With -n it only shows the files.
func cmdGC(args []string) {
	fs := flag.NewFlagSet("gc", flag.ContinueOnError)
	dryRun := fs.Bool("n", false, "show the files, but do not remove them")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 {
		die("usage: gc [-n]")
	}
	var unused []string
	unused = append(unused, unusedSources()...)
	unused = append(unused, unusedSeeds()...)
	unused = append(unused, unusedPackageFiles()...)
	var total int64
	for _, p := range unused {
		size := diskUsage(p)
		total += size
		if *dryRun {
			fmt.Printf("%8s  %s\n", humanSize(size), rel(p))
			continue
		}
		logf("remove %s (%s)", rel(p), humanSize(size))
		removeAll(p)
	}
	verb := "removed"
	if *dryRun {
		verb = "can remove"
	}
	logf("gc: %s %d files and directories (%s)", verb, len(unused), humanSize(total))
}

// usedSourceFiles returns the base names of the cache files of all sources
// of all recipes and of the kernel (with the signature and checksum files).
func usedSourceFiles() map[string]bool {
	sources := append([]Source{}, kernelDef().Source...)
	for _, r := range recipes() {
		sources = append(sources, r.Source...)
	}
	used := map[string]bool{}
	for _, s := range sources {
		n := filepath.Base(s.path())
		used[n] = true
		used[n+".sig"] = true
		used[n+".sums"] = true
	}
	return used
}

func unusedSources() []string {
	used := usedSourceFiles()
	return unusedIn(SRCCACHE, func(name string) bool { return used[name] })
}

// unusedSeeds lists the builder tarballs in cache/builder that are not the
// seed of builder/seed.conf.
func unusedSeeds() []string {
	seed, err := seedSHA256()
	if err != nil {
		logf("warning: %v; gc keeps cache/builder", err)
		return nil
	}
	keep := "klos-builder-" + seed[:16] + ".tar"
	return unusedIn(filepath.Join(CACHE, "builder"), func(name string) bool { return name == keep })
}

// unusedPackageFiles lists the package files, host tool directories and
// build logs of packages that have no recipe.
func unusedPackageFiles() []string {
	rs := recipes()
	isRecipe := func(name string) bool {
		_, ok := rs[name]
		return ok
	}
	var out []string
	out = append(out, unusedIn(PKGOUT, func(name string) bool {
		base := strings.TrimSuffix(strings.TrimSuffix(name, ".json"), ".tar.zst")
		return base != name && isRecipe(base)
	})...)
	out = append(out, unusedIn(filepath.Join(WORK, "host"), func(name string) bool {
		return name == "toolchain" || isRecipe(name)
	})...)
	out = append(out, unusedIn(filepath.Join(WORK, "logs"), func(name string) bool {
		base := strings.TrimSuffix(name, ".log")
		return base == name || isRecipe(base)
	})...)
	return out
}

// unusedIn lists the entries of dir for which keep is false. A directory
// that does not exist has no entries.
func unusedIn(dir string, keep func(name string) bool) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if !os.IsNotExist(err) {
			die("gc: %v", err)
		}
		return nil
	}
	var out []string
	for _, e := range entries {
		if !keep(e.Name()) {
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(out)
	return out
}

// seedSHA256 reads SEED_SHA256 from builder/seed.conf.
func seedSHA256() (string, error) {
	p := filepath.Join(TOP, "builder", "seed.conf")
	b, err := os.ReadFile(p)
	if err != nil {
		return "", err
	}
	for _, l := range strings.Split(string(b), "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(l), "SEED_SHA256="); ok && len(v) == 64 {
			return v, nil
		}
	}
	return "", fmt.Errorf("%s: no SEED_SHA256", p)
}

// diskUsage returns the size of the regular files in path.
func diskUsage(path string) int64 {
	var n int64
	_ = filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err == nil && info.Mode().IsRegular() {
			n += info.Size()
		}
		return nil // a file that cannot be read counts as 0 bytes
	})
	return n
}

func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
