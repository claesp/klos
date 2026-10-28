// SPDX-License-Identifier: BSD-2-Clause

package main

// The KLOS builder. Read docs/build-system.md.
//
// The KLOS builder builds the packages of manifests/builder.toml: the
// toolchain, the build tools and the test tools. "builder-image" puts them
// together in a root file system, and "./klos builder" makes a container
// image from it. KLOS thus builds its own builder. If the new builder is
// the same as the builder that built it, the builder is a fixed point.

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// BuilderManifest is manifests/builder.toml.
type BuilderManifest struct {
	Builder struct {
		// Packages are target packages (with their runtime dependencies).
		Packages []string `toml:"packages"`
		// HostPackages are host packages (for example the Go toolchain).
		HostPackages []string `toml:"host-packages"`
		// Conf is the /etc/klos-builder.conf of the KLOS builder.
		Conf string `toml:"conf"`
	} `toml:"builder"`
}

func loadBuilderManifest() *BuilderManifest {
	bm := &BuilderManifest{}
	loadTOML(filepath.Join(TOP, "manifests", "builder.toml"), bm)
	if _, err := parseBuilderConf(bm.Builder.Conf, "conf"); err != nil {
		die("manifests/builder.toml: conf: %v", err)
	}
	return bm
}

// cmdBuilderImage builds the packages of the KLOS builder and writes
// out/<builder>/builder/klos-builder.tar.
func cmdBuilderImage(args []string) {
	if len(args) != 0 {
		die("usage: builder-image")
	}
	bm := loadBuilderManifest()
	build(append(append([]string{}, bm.Builder.Packages...), bm.Builder.HostPackages...), false)
	names := runtimeClosure(bm.Builder.Packages)
	for _, h := range bm.Builder.HostPackages {
		if pkgMeta(h).Kind != "host" {
			die("builder.toml: %s is not a host package", h)
		}
		names = append(names, h)
	}
	sort.Strings(names)

	rootfs := filepath.Join(WORK, "builder", "rootfs")
	removeAll(rootfs)
	mkdirAll(rootfs)
	owners := map[string]string{}
	var lock strings.Builder
	for _, n := range names {
		for _, f := range listTarball(pkgTarball(n)) {
			if o, ok := owners[f]; ok {
				die("builder: file conflict: /%s is in %s and %s", f, o, n)
			}
			owners[f] = n
		}
		extractTarball(pkgTarball(n), rootfs, nil)
		m := pkgMeta(n)
		fmt.Fprintf(&lock, "%s %s %s\n", m.Name, m.Version, m.SHA256)
	}
	for _, d := range []struct {
		path string
		mode os.FileMode
	}{{"var/roothome", 0o700}, {"var/tmp", 0o1777}, {"klos", 0o755}} {
		p := filepath.Join(rootfs, d.path)
		mkdirAll(p)
		check(os.Chmod(p, d.mode))
	}
	writeFile(filepath.Join(rootfs, "etc/passwd"), "root:x:0:0:root:/var/roothome:/bin/sh\n", 0o644)
	writeFile(filepath.Join(rootfs, "etc/group"), "root:x:0:\n", 0o644)
	writeFile(filepath.Join(rootfs, strings.TrimPrefix(builderConfPath, "/")), bm.Builder.Conf, 0o644)
	writeFile(filepath.Join(rootfs, "etc/klos-builder.lock"), lock.String(), 0o644)

	out := filepath.Join(OUT, "builder", "klos-builder.tar")
	mkdirAll(filepath.Dir(out))
	f, err := os.Create(out)
	check(err)
	bw := bufio.NewWriterSize(f, 1<<20)
	if err := writeLabeledTar(rootfs, nil, bw); err != nil {
		die("builder: %v", err)
	}
	check(bw.Flush())
	check(f.Close())
	removeAll(rootfs)
	logf("builder: %s (%d packages, sha256 %s)", rel(out), len(names), sha256File(out))
}
