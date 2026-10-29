// SPDX-License-Identifier: BSD-2-Clause

package main

// Rust crates of a source with cargo-lock. Cargo.lock has the SHA-256
// value of each crate of crates.io. "fetch" downloads the crates into
// cache/crates and checks the values. The build unpacks the crates into a
// vendor directory, and Cargo uses it without the network (source
// replacement in CARGO_HOME/config.toml). Read docs/recipes.md.

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const cratesIOIndex = "registry+https://github.com/rust-lang/crates.io-index"

// Crate is a package of Cargo.lock from crates.io.
type Crate struct {
	Name, Version, SHA256 string
}

func (c Crate) file() string {
	return filepath.Join(CACHE, "crates", c.Name+"-"+c.Version+".crate")
}

// parseCargoLock returns the crates.io packages of a Cargo.lock file. A
// package from another source (a Git repository) is an error: kbuild cannot
// check it.
func parseCargoLock(text string) ([]Crate, error) {
	var crates []Crate
	var cur map[string]string
	flush := func() error {
		if cur == nil {
			return nil
		}
		defer func() { cur = nil }()
		src := cur["source"]
		switch {
		case src == "":
			return nil // a package of the workspace
		case src != cratesIOIndex:
			return fmt.Errorf("package %s %s comes from %s (only crates.io is supported)", cur["name"], cur["version"], src)
		case len(cur["checksum"]) != 64:
			return fmt.Errorf("package %s %s has no checksum", cur["name"], cur["version"])
		}
		crates = append(crates, Crate{cur["name"], cur["version"], cur["checksum"]})
		return nil
	}
	sc := bufio.NewScanner(strings.NewReader(text))
	for sc.Scan() {
		l := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(l, "[") {
			if err := flush(); err != nil {
				return nil, err
			}
			if l == "[[package]]" {
				cur = map[string]string{}
			}
			continue
		}
		if cur == nil {
			continue
		}
		if k, v, ok := strings.Cut(l, " = "); ok && strings.HasPrefix(v, `"`) {
			cur[k] = strings.Trim(v, `"`)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if err := flush(); err != nil {
		return nil, err
	}
	return crates, nil
}

// sourceCargoLock reads the Cargo.lock file of a source tarball.
func sourceCargoLock(s Source, tarball string) ([]Crate, error) {
	names, err := tarNames(tarball)
	if err != nil {
		return nil, err
	}
	p := shortestWithSuffix(names, s.CargoLock)
	if p == "" {
		return nil, fmt.Errorf("%s has no %s", s.filename(), s.CargoLock)
	}
	b, err := tarFile(tarball, p)
	if err != nil {
		return nil, err
	}
	return parseCargoLock(string(b))
}

// fetchCrates downloads the crates of a source and checks their SHA-256
// values with Cargo.lock.
func fetchCrates(s Source, tarball string) {
	if s.CargoLock == "" {
		return
	}
	crates, err := sourceCargoLock(s, tarball)
	if err != nil {
		die("%v", err)
	}
	mkdirAll(filepath.Join(CACHE, "crates"))
	n := 0
	for _, c := range crates {
		p := c.file()
		if exists(p) {
			continue
		}
		download(fmt.Sprintf("https://static.crates.io/crates/%s/%s-%s.crate", c.Name, c.Name, c.Version), p)
		if got := sha256File(p); got != c.SHA256 {
			_ = os.Rename(p, p+".bad") // keep the file for inspection; the build stops
			die("SHA-256 mismatch for the crate %s %s\n  Cargo.lock %s\n  file       %s", c.Name, c.Version, c.SHA256, got)
		}
		n++
	}
	if n > 0 {
		logf("%s: downloaded %d crates (%d in Cargo.lock)", s.filename(), n, len(crates))
	}
}

// vendorCrates unpacks the crates of the Cargo.lock file in srcTop into
// vendor, and writes the Cargo configuration that uses them.
func vendorCrates(s Source, srcTop, vendor, cargoHome string) {
	b, err := os.ReadFile(filepath.Join(srcTop, s.CargoLock))
	check(err)
	crates, err := parseCargoLock(string(b))
	if err != nil {
		die("%v", err)
	}
	mkdirAll(vendor)
	for _, c := range crates {
		p := c.file()
		if !exists(p) {
			die("the crate %s %s is not in the cache (run './klos fetch')", c.Name, c.Version)
		}
		if sha256File(p) != c.SHA256 {
			die("cached crate changed: %s", p)
		}
		dir := filepath.Join(vendor, c.Name+"-"+c.Version)
		mkdirAll(dir)
		run(Cmd{Args: []string{"tar", "-xzf", p, "-C", dir, "--strip-components=1", "--no-same-owner", "--warning=no-timestamp"}})
		// Cargo checks the package checksum; an empty file list is valid.
		sum, err := json.Marshal(map[string]any{"files": map[string]string{}, "package": c.SHA256})
		check(err)
		writeFile(filepath.Join(dir, ".cargo-checksum.json"), string(sum), 0o644)
	}
	mkdirAll(cargoHome)
	writeFile(filepath.Join(cargoHome, "config.toml"), fmt.Sprintf(`[source.crates-io]
replace-with = "klos-vendor"

[source.klos-vendor]
directory = %q

[net]
offline = true
`, vendor), 0o644)
	logf("vendored %d crates", len(crates))
}
