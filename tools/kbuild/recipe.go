// SPDX-License-Identifier: BSD-2-Clause

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Source is one upstream file of a recipe.
type Source struct {
	URL           string   `toml:"url"`
	SHA256        string   `toml:"sha256"`
	Filename      string   `toml:"filename"`
	Signature     string   `toml:"signature"`
	SignatureType string   `toml:"signature-type"` // pgp (default), minisign, pgp-checksums or pgp-checksum-file
	Checksums     string   `toml:"checksums"`      // pgp-checksum-file: the checksum list that the signature covers
	SignedContent string   `toml:"signed-content"` // "uncompressed": signature is over the .tar
	Key           []string `toml:"key"`            // names of files in keys/
	Extract       *bool    `toml:"extract"`
	Dest          string   `toml:"dest"`
}

type Options struct {
	KeepDocs        bool   `toml:"keep-docs"`
	NoStrip         bool   `toml:"no-strip"`
	KeepEmptyDirs   bool   `toml:"keep-empty-dirs"`
	AllowUnresolved bool   `toml:"allow-unresolved"`
	CFlags          string `toml:"cflags"`
	LDFlags         string `toml:"ldflags"`
	// RunTools are programs in the sysroot (/usr/bin) that the build runs,
	// for example wayland-scanner. kbuild makes a wrapper for each one.
	RunTools []string `toml:"run-tools"`
}

type Recipe struct {
	Dir     string `toml:"-"`
	Package struct {
		Name         string   `toml:"name"`
		Version      string   `toml:"version"`
		Release      int      `toml:"release"`
		Summary      string   `toml:"summary"`
		License      string   `toml:"license"`
		Homepage     string   `toml:"homepage"`
		Kind         string   `toml:"kind"` // target (default) or host
		ImplicitDeps *bool    `toml:"implicit-deps"`
		Depends      []string `toml:"depends"`
		BuildDepends []string `toml:"build-depends"`
		HostDepends  []string `toml:"host-depends"`
		Options      Options  `toml:"options"`
		// LocalSource is a directory in this repository with the source code.
		LocalSource string `toml:"local-source"`
	} `toml:"package"`
	Source []Source `toml:"source"`
	Build  struct {
		Script string `toml:"script"`
	} `toml:"build"`

	Name, Version, Kind, Summary, License string
	Depends, BuildDepends, HostDepends    []string
	Options                               Options
}

// Packages that every target package build gets in its sysroot.
var implicitTarget = []string{"filesystem", "linux-headers", "musl", "llvm-runtimes"}

func loadRecipe(path string) *Recipe {
	r := &Recipe{Dir: filepath.Dir(path)}
	loadTOML(path, r)
	p := &r.Package
	r.Name, r.Version, r.Kind = p.Name, p.Version, p.Kind
	r.Summary, r.License, r.Options = p.Summary, p.License, p.Options
	if r.Name != filepath.Base(r.Dir) {
		die("%s: name %q does not match the directory", path, r.Name)
	}
	if r.Version == "" {
		die("%s: no version", path)
	}
	if p.Release == 0 {
		p.Release = 1
	}
	if r.Kind == "" {
		r.Kind = "target"
	}
	if r.Kind != "target" && r.Kind != "host" {
		die("%s: kind must be target or host", path)
	}
	r.Depends = append([]string{}, p.Depends...)
	r.BuildDepends = append([]string{}, p.BuildDepends...)
	r.HostDepends = append([]string{}, p.HostDepends...)
	implicit := r.Kind == "target"
	if p.ImplicitDeps != nil {
		implicit = *p.ImplicitDeps
	}
	if implicit {
		for _, d := range implicitTarget {
			if d != r.Name && !contains(r.BuildDepends, d) {
				r.BuildDepends = append(r.BuildDepends, d)
			}
		}
		for _, h := range []string{"llvm", "compiler-rt"} {
			if !contains(r.HostDepends, h) {
				r.HostDepends = append(r.HostDepends, h)
			}
		}
	}
	for _, s := range r.Source {
		if s.URL == "" || len(s.SHA256) != 64 {
			die("%s: each source needs a url and a sha256", path)
		}
	}
	return r
}

func (r *Recipe) FullVersion() string {
	return fmt.Sprintf("%s-r%d", r.Version, r.Package.Release)
}

func (r *Recipe) allDeps() []string {
	var d []string
	d = append(d, r.Depends...)
	d = append(d, r.BuildDepends...)
	return append(d, r.HostDepends...)
}

var recipeCache map[string]*Recipe

func recipes() map[string]*Recipe {
	if recipeCache == nil {
		recipeCache = map[string]*Recipe{}
		paths, _ := filepath.Glob(filepath.Join(TOP, "pkgs", "*", "recipe.toml"))
		for _, p := range paths {
			r := loadRecipe(p)
			recipeCache[r.Name] = r
		}
	}
	return recipeCache
}

func recipe(name string) *Recipe {
	r, ok := recipes()[name]
	if !ok {
		die("no recipe for package %q", name)
	}
	return r
}

// topoClosure returns the recipes that are necessary for names, in build order.
func topoClosure(names []string) []*Recipe {
	var order []*Recipe
	state := map[string]int{}
	var visit func(n string, chain []string)
	visit = func(n string, chain []string) {
		switch state[n] {
		case 2:
			return
		case 1:
			die("dependency cycle: %s", strings.Join(append(chain, n), " -> "))
		}
		state[n] = 1
		r := recipe(n)
		deps := r.allDeps()
		sort.Strings(deps)
		for _, d := range deps {
			visit(d, append(chain, n))
		}
		state[n] = 2
		order = append(order, r)
	}
	for _, n := range names {
		visit(n, nil)
	}
	return order
}

var hashCache = map[string]string{}

// builderLock identifies the builder: its package list and its
// configuration. A change of the builder changes all input hashes.
func builderLock() string {
	b, _ := os.ReadFile("/etc/klos-builder.lock")
	c, _ := os.ReadFile(builderConfPath)
	return string(b) + "\x00" + string(c)
}

// inputHash is a Merkle hash of all inputs of a package build: the recipe
// directory, the global flags, the builder and the hashes of the build
// dependencies. A changed hash means that the package must be built again.
func inputHash(r *Recipe) string {
	if h, ok := hashCache[r.Name]; ok {
		return h
	}
	h := sha256.New()
	h.Write([]byte(kbuildVersion + "\x00"))
	var files []string
	walkRoots := []string{r.Dir}
	if r.Package.LocalSource != "" {
		walkRoots = append(walkRoots, filepath.Join(TOP, r.Package.LocalSource))
	}
	for _, root := range walkRoots {
		check(filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.Mode().IsRegular() {
				files = append(files, p)
			}
			return nil
		}))
	}
	sort.Strings(files)
	for _, f := range files {
		b, err := os.ReadFile(f)
		check(err)
		relp, _ := filepath.Rel(TOP, f)
		_, _ = fmt.Fprintf(h, "%s\x00%d\x00", relp, len(b)) // hash.Hash.Write never fails
		h.Write(b)
	}
	// Host packages do not get the target flags or the recipe helper
	// library, so changes to these do not cause a toolchain rebuild.
	common := map[string]any{
		"target":  CONFIG.Distribution.Target,
		"sde":     SDE,
		"builder": builderLock(),
	}
	if r.Kind == "target" {
		lib, err := os.ReadFile(filepath.Join(TOP, "tools", "recipe-lib.sh"))
		check(err)
		common["flags"] = CONFIG.Flags
		common["lib"] = string(lib)
	}
	commonJSON, _ := json.Marshal(common)
	h.Write(commonJSON)
	deps := uniqSorted(append(append([]string{}, r.BuildDepends...), r.HostDepends...))
	for _, d := range deps {
		_, _ = fmt.Fprintf(h, "%s=%s\x00", d, inputHash(recipe(d))) // hash.Hash.Write never fails
	}
	sum := hex.EncodeToString(h.Sum(nil))
	hashCache[r.Name] = sum
	return sum
}

// PkgMeta is the metadata that kbuild writes next to each package tarball.
type PkgMeta struct {
	Name     string   `json:"name"`
	Version  string   `json:"version"`
	Kind     string   `json:"kind"`
	License  string   `json:"license"`
	Summary  string   `json:"summary"`
	Input    string   `json:"input"`
	SHA256   string   `json:"sha256"`
	Depends  []string `json:"depends"`
	Provides []string `json:"provides"`
}

// HOSTPKGOUT is the output directory of host packages. It is the same as
// PKGOUT, except in "verify", which builds the target packages again.
var HOSTPKGOUT string

func pkgDir(name string) string {
	if HOSTPKGOUT != "" {
		if r, ok := recipes()[name]; ok && r.Kind == "host" {
			return HOSTPKGOUT
		}
	}
	return PKGOUT
}

func pkgMeta(name string) *PkgMeta {
	b, err := os.ReadFile(filepath.Join(pkgDir(name), name+".json"))
	if err != nil {
		return nil
	}
	m := &PkgMeta{}
	check(json.Unmarshal(b, m))
	return m
}

func pkgTarball(name string) string {
	return filepath.Join(pkgDir(name), name+".tar.zst")
}

func isCurrent(r *Recipe) bool {
	m := pkgMeta(r.Name)
	return m != nil && m.Input == inputHash(r) && exists(pkgTarball(r.Name))
}

func sortStrings(s []string) { sort.Strings(s) }
