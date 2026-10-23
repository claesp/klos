// SPDX-License-Identifier: BSD-2-Clause

package main

import (
	"debug/elf"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

func extractTarball(tarball, dest string, excludes []string) {
	mkdirAll(dest)
	args := []string{"tar", "-xf", tarball, "-C", dest, "--no-same-owner",
		"--delay-directory-restore", "--warning=no-timestamp"}
	if strings.HasSuffix(tarball, ".zst") {
		args = append(args, "--zstd")
	}
	for _, e := range excludes {
		args = append(args, "--exclude=./"+e)
	}
	run(Cmd{Args: args})
}

// hostRoot extracts a host package one time and returns its root directory.
func hostRoot(name string) string {
	m := pkgMeta(name)
	if m == nil {
		die("host package %q is not built", name)
	}
	root := filepath.Join(WORK, "host", name, m.Input[:16])
	if !exists(filepath.Join(root, ".done")) {
		removeAll(filepath.Dir(root))
		extractTarball(pkgTarball(name), root, nil)
		writeFile(filepath.Join(root, ".done"), "", 0o644)
	}
	return root
}

// toolchainRoot returns a directory with the packages llvm and compiler-rt
// together, because Clang finds its builtins relative to its own path.
func toolchainRoot() string {
	a, b := pkgMeta("llvm"), pkgMeta("compiler-rt")
	if a == nil || b == nil {
		die("the toolchain (llvm, compiler-rt) is not built")
	}
	root := filepath.Join(WORK, "host", "toolchain", a.Input[:12]+"-"+b.Input[:12])
	if !exists(filepath.Join(root, ".done")) {
		removeAll(filepath.Dir(root))
		extractTarball(pkgTarball("llvm"), root, nil)
		extractTarball(pkgTarball("compiler-rt"), root, nil)
		writeFile(filepath.Join(root, ".done"), "", 0o644)
	}
	return root
}

// runtimeClosure returns the closure over the runtime dependencies of
// built packages.
func runtimeClosure(names []string) []string {
	seen := map[string]bool{}
	stack := append([]string{}, names...)
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if seen[n] {
			continue
		}
		seen[n] = true
		m := pkgMeta(n)
		if m == nil {
			die("package %q is not built", n)
		}
		stack = append(stack, m.Depends...)
	}
	var out []string
	for n := range seen {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

func writeExec(p, content string) { writeFile(p, content, 0o755) }

func symlink(target, link string) { check(os.Symlink(target, link)) }

func shellQuoteList(s string) string {
	var q []string
	for _, f := range strings.Fields(s) {
		q = append(q, "'"+f+"'")
	}
	return "[" + strings.Join(q, ", ") + "]"
}

// buildEnv makes the compiler wrappers and the environment for a build.
func buildEnv(r *Recipe, bdir, sysroot string) []string {
	bindir := filepath.Join(bdir, "bin")
	mkdirAll(bindir)
	mkdirAll(filepath.Join(bdir, "home"))
	mkdirAll(filepath.Join(bdir, "tmp"))
	target := CONFIG.Distribution.Target
	env := map[string]string{
		"PATH":              bindir + ":/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin",
		"HOME":              filepath.Join(bdir, "home"),
		"TMPDIR":            filepath.Join(bdir, "tmp"),
		"LANG":              "C.UTF-8",
		"LC_ALL":            "C.UTF-8",
		"TZ":                "UTC",
		"SOURCE_DATE_EPOCH": fmt.Sprint(SDE),
		"JOBS":              JOBS,
		"KLOS_TOP":          TOP,
		"KLOS_VERSION":      VERSION,
		"PKGNAME":           r.Name,
		"PKGVER":            r.Version,
		"RECIPE_DIR":        r.Dir,
		"FILESDIR":          r.Dir,
		"TARGET":            target,
		"BUILD_TRIPLE":      buildTriple,
		"DESTDIR":           filepath.Join(bdir, "dest"),
		"SYSROOT":           sysroot,
		"HOST_CC":           "/usr/bin/gcc",
		"HOST_CXX":          "/usr/bin/g++",
		"CC_FOR_BUILD":      "/usr/bin/gcc",
		"CXX_FOR_BUILD":     "/usr/bin/g++",
		"BUILD_CC":          "/usr/bin/gcc",
		"GOCACHE":           filepath.Join(bdir, "gocache"),
		"GOPATH":            filepath.Join(bdir, "gopath"),
		"GOFLAGS":           "-mod=vendor -trimpath -buildvcs=false",
		"GOTOOLCHAIN":       "local",
		"GOPROXY":           "off",
	}
	for _, h := range r.HostDepends {
		env["HOST_"+strings.ToUpper(strings.ReplaceAll(h, "-", "_"))] = hostRoot(h)
	}
	if r.Kind == "host" {
		env["CC"], env["CXX"] = "clang", "clang++"
		env["PREFIX"] = "/" + toolchainPrefix
		return envList(env)
	}
	if !contains(r.HostDepends, "compiler-rt") {
		// A target package without compiled code (for example headers or data).
		return envList(env)
	}

	tc := filepath.Join(toolchainRoot(), toolchainPrefix)
	tcbin := filepath.Join(tc, "bin")
	env["TOOLCHAIN"] = tc
	prefixMap := fmt.Sprintf("-ffile-prefix-map=%s/src=/usr/src/%s", bdir, r.Name)
	cflags := CONFIG.Flags.CFlags + " " + prefixMap
	ldflags := CONFIG.Flags.LDFlags
	if r.Options.CFlags != "" {
		cflags += " " + r.Options.CFlags
	}
	if r.Options.LDFlags != "" {
		ldflags += " " + r.Options.LDFlags
	}
	cxxflags := cflags + " " + CONFIG.Flags.CXXFlags
	drv := fmt.Sprintf(`--target=%s --sysroot="%s"`, target, sysroot)
	for _, w := range [][2]string{{"cc", "clang"}, {"clang", "clang"}, {"gcc", "clang"},
		{"c++", "clang++"}, {"clang++", "clang++"}, {"g++", "clang++"}, {"cpp", "clang-cpp"}} {
		writeExec(filepath.Join(bindir, w[0]),
			fmt.Sprintf("#!/bin/sh\nexec \"%s/%s\" %s \"$@\"\n", tcbin, w[1], drv))
		switch w[0] {
		case "cc", "c++", "gcc", "g++", "cpp":
			symlink(w[0], filepath.Join(bindir, target+"-"+w[0]))
		}
	}
	for _, t := range []string{"ar", "nm", "ranlib", "strip", "objcopy", "objdump",
		"readelf", "size", "strings"} {
		symlink(filepath.Join(tcbin, "llvm-"+t), filepath.Join(bindir, t))
		symlink(filepath.Join(tcbin, "llvm-"+t), filepath.Join(bindir, target+"-"+t))
	}
	symlink(filepath.Join(tcbin, "ld.lld"), filepath.Join(bindir, "ld"))
	symlink(filepath.Join(tcbin, "ld.lld"), filepath.Join(bindir, target+"-ld"))
	writeExec(filepath.Join(bindir, "pkg-config"), fmt.Sprintf(`#!/bin/sh
export PKG_CONFIG_SYSROOT_DIR="%[1]s"
export PKG_CONFIG_LIBDIR="%[1]s/usr/lib/pkgconfig:%[1]s/usr/share/pkgconfig"
unset PKG_CONFIG_PATH
exec /usr/bin/pkgconf "$@"
`, sysroot))
	symlink("pkg-config", filepath.Join(bindir, target+"-pkg-config"))

	for _, t := range r.Options.RunTools {
		writeRunTool(bindir, sysroot, t)
	}

	cmakeTC := filepath.Join(bdir, "toolchain.cmake")
	writeFile(cmakeTC, fmt.Sprintf(`set(CMAKE_SYSTEM_NAME Linux)
set(CMAKE_SYSTEM_PROCESSOR x86_64)
set(CMAKE_SYSROOT "%[1]s")
set(CMAKE_C_COMPILER "%[2]s/cc")
set(CMAKE_CXX_COMPILER "%[2]s/c++")
set(CMAKE_ASM_COMPILER "%[2]s/cc")
set(CMAKE_AR "%[3]s/llvm-ar")
set(CMAKE_RANLIB "%[3]s/llvm-ranlib")
set(CMAKE_NM "%[3]s/llvm-nm")
set(CMAKE_STRIP "%[3]s/llvm-strip")
set(CMAKE_OBJCOPY "%[3]s/llvm-objcopy")
set(CMAKE_FIND_ROOT_PATH "%[1]s")
set(CMAKE_FIND_ROOT_PATH_MODE_PROGRAM NEVER)
set(CMAKE_FIND_ROOT_PATH_MODE_LIBRARY ONLY)
set(CMAKE_FIND_ROOT_PATH_MODE_INCLUDE ONLY)
set(CMAKE_FIND_ROOT_PATH_MODE_PACKAGE ONLY)
`, sysroot, bindir, tcbin), 0o644)

	mesonCross := filepath.Join(bdir, "meson-cross.ini")
	writeFile(mesonCross, fmt.Sprintf(`[binaries]
c = '%[1]s/cc'
cpp = '%[1]s/c++'
ar = '%[2]s/llvm-ar'
nm = '%[2]s/llvm-nm'
strip = '%[2]s/llvm-strip'
objcopy = '%[2]s/llvm-objcopy'
pkg-config = '%[1]s/pkg-config'

[host_machine]
system = 'linux'
cpu_family = 'x86_64'
cpu = 'x86_64'
endian = 'little'

[properties]
sys_root = '%[3]s'

[built-in options]
c_args = %[4]s
cpp_args = %[5]s
c_link_args = %[6]s
cpp_link_args = %[6]s
`, bindir, tcbin, sysroot, shellQuoteList(cflags), shellQuoteList(cxxflags),
		shellQuoteList(ldflags)), 0o644)

	for k, v := range map[string]string{
		"CC": "cc", "CXX": "c++", "CPP": "cpp",
		"AR": "llvm-ar", "NM": "llvm-nm", "RANLIB": "llvm-ranlib",
		"STRIP": "llvm-strip", "OBJCOPY": "llvm-objcopy",
		"OBJDUMP": "llvm-objdump", "READELF": "llvm-readelf", "LD": "ld.lld",
		"PATH":                 bindir + ":" + tcbin + ":/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin",
		"CFLAGS":               cflags,
		"CXXFLAGS":             cxxflags,
		"LDFLAGS":              ldflags,
		"CMAKE_TOOLCHAIN_FILE": cmakeTC,
		"MESON_CROSS_FILE":     mesonCross,
		"PKG_CONFIG":           "pkg-config",
		// The builder has no g++. Build-time C++ helpers use the KLOS
		// toolchain, linked static so that they run in the builder.
		"HOST_CXX":      bindir + "/c++ -static",
		"CXX_FOR_BUILD": bindir + "/c++ -static",
	} {
		env[k] = v
	}
	return envList(env)
}

// writeRunTool makes a wrapper that runs a target program of the sysroot in
// the builder. A script runs with the interpreter of the builder. An ELF
// program runs with the KLOS dynamic linker and the sysroot libraries.
func writeRunTool(bindir, sysroot, tool string) {
	prog := filepath.Join(sysroot, "usr/bin", tool)
	b, err := os.ReadFile(prog)
	if err != nil {
		die("run-tools: %s is not in the sysroot", tool)
	}
	var w string
	if len(b) > 2 && string(b[:2]) == "#!" {
		line := strings.Fields(strings.SplitN(string(b[2:]), "\n", 2)[0])
		interp := filepath.Base(line[0])
		if interp == "env" && len(line) > 1 {
			interp = line[1]
		}
		w = fmt.Sprintf("#!/bin/sh\nexec %s \"%s\" \"$@\"\n", interp, prog)
	} else {
		w = fmt.Sprintf("#!/bin/sh\nexec \"%[1]s/usr/lib/libc.so\" --library-path \"%[1]s/usr/lib\" \"%[2]s\" \"$@\"\n", sysroot, prog)
	}
	writeExec(filepath.Join(bindir, tool), w)
}

func envList(m map[string]string) []string {
	var out []string
	for k, v := range m {
		out = append(out, k+"="+v)
	}
	sort.Strings(out)
	return out
}

var archiveRe = regexp.MustCompile(`\.(tar(\.\w+)?|tgz|tbz2|txz)$`)

// extractSources unpacks the sources and returns the main source directory.
func extractSources(r *Recipe, srcdir string) string {
	mkdirAll(srcdir)
	top := ""
	for i, s := range r.Source {
		p := checkSource(s)
		extract := s.Extract == nil || *s.Extract
		if extract && archiveRe.MatchString(s.filename()) {
			dest := srcdir
			if s.Dest != "" {
				dest = filepath.Join(srcdir, s.Dest)
			}
			mkdirAll(dest)
			before := dirNames(dest)
			run(Cmd{Args: []string{"tar", "-xf", p, "-C", dest, "--no-same-owner", "--warning=no-timestamp"}})
			if i == 0 && s.Dest == "" {
				var added []string
				for n := range dirNames(dest) {
					if !before[n] {
						added = append(added, n)
					}
				}
				sort.Strings(added)
				if len(added) == 1 {
					if st, err := os.Stat(filepath.Join(dest, added[0])); err == nil && st.IsDir() {
						top = filepath.Join(dest, added[0])
					}
				}
			}
		} else {
			copyFile(p, filepath.Join(srcdir, s.filename()))
		}
	}
	if r.Package.LocalSource != "" {
		top = filepath.Join(srcdir, filepath.Base(r.Package.LocalSource))
		run(Cmd{Args: []string{"cp", "-a", filepath.Join(TOP, r.Package.LocalSource), top}})
	}
	if top == "" {
		top = srcdir
	}
	return top
}

func dirNames(dir string) map[string]bool {
	m := map[string]bool{}
	ents, _ := os.ReadDir(dir)
	for _, e := range ents {
		m[e.Name()] = true
	}
	return m
}

var mergeDirs = [][2]string{{"bin", "usr/bin"}, {"sbin", "usr/bin"}, {"usr/sbin", "usr/bin"},
	{"lib", "usr/lib"}, {"lib64", "usr/lib"}, {"usr/lib64", "usr/lib"}}

// mergeUsr moves files into the merged /usr layout (docs/filesystem.md).
func mergeUsr(dest string) {
	for _, m := range mergeDirs {
		src := filepath.Join(dest, m[0])
		st, err := os.Lstat(src)
		if err != nil || !st.IsDir() {
			continue
		}
		dst := filepath.Join(dest, m[1])
		mkdirAll(dst)
		ents, _ := os.ReadDir(src)
		for _, e := range ents {
			t := filepath.Join(dst, e.Name())
			if exists(t) {
				die("usr merge: %s/%s conflicts with %s", m[0], e.Name(), m[1])
			}
			check(os.Rename(filepath.Join(src, e.Name()), t))
		}
		check(os.Remove(src))
	}
}

func elfType(p string) elf.Type {
	f, err := elf.Open(p)
	if err != nil {
		return elf.ET_NONE
	}
	defer f.Close()
	return f.Type
}

func postprocess(r *Recipe, dest string, env []string) {
	if r.Kind == "target" {
		mergeUsr(dest)
	}
	if !r.Options.KeepDocs {
		for _, d := range []string{"usr/share/info", "usr/share/doc", "usr/share/gtk-doc"} {
			removeAll(filepath.Join(dest, d))
		}
	}
	strip := ""
	stripTool := func() string {
		if strip == "" {
			strip = filepath.Join(hostRoot("llvm"), toolchainPrefix, "bin", "llvm-strip")
		}
		return strip
	}
	var files []string
	filepath.WalkDir(dest, func(p string, d fs.DirEntry, err error) error {
		check(err)
		if d.Type().IsRegular() {
			files = append(files, p)
		}
		return nil
	})
	for _, p := range files {
		if strings.HasSuffix(p, ".la") {
			check(os.Remove(p))
			continue
		}
		if r.Kind == "target" && !r.Options.NoStrip {
			switch t := elfType(p); {
			case t == elf.ET_EXEC || t == elf.ET_DYN:
				run(Cmd{Args: []string{stripTool(), "--strip-unneeded", p}})
			case strings.HasSuffix(p, ".a"):
				run(Cmd{Args: []string{stripTool(), "--strip-debug", p}})
			}
		}
		st, err := os.Lstat(p)
		check(err)
		if st.Mode().Perm()&0o022 != 0 {
			check(os.Chmod(p, st.Mode()&^0o022))
		}
	}
	if !r.Options.KeepEmptyDirs {
		removeEmptyDirs(dest)
	}
}

func removeEmptyDirs(root string) {
	var dirs []string
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		check(err)
		if d.IsDir() && p != root {
			dirs = append(dirs, p)
		}
		return nil
	})
	sort.Slice(dirs, func(i, j int) bool { return len(dirs[i]) > len(dirs[j]) })
	for _, d := range dirs {
		if ents, _ := os.ReadDir(d); len(ents) == 0 {
			check(os.Remove(d))
		}
	}
}

// scanELF returns the sonames that files in dir provide and need.
func scanELF(dir string) (provides, needed map[string]bool) {
	provides, needed = map[string]bool{}, map[string]bool{}
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		check(err)
		if !d.Type().IsRegular() {
			return nil
		}
		f, err := elf.Open(p)
		if err != nil {
			return nil
		}
		defer f.Close()
		if f.Type != elf.ET_EXEC && f.Type != elf.ET_DYN {
			return nil
		}
		if so, err := f.DynString(elf.DT_SONAME); err == nil && len(so) > 0 {
			provides[so[0]] = true
		} else if f.Type == elf.ET_DYN && filepath.Dir(p) == filepath.Join(dir, "usr/lib") {
			// A library without a soname (for example musl libc.so).
			provides[filepath.Base(p)] = true
		}
		if libs, err := f.DynString(elf.DT_NEEDED); err == nil {
			for _, l := range libs {
				needed[l] = true
			}
		}
		return nil
	})
	return
}

// makeTarball writes a reproducible, zstd-compressed tar archive of dir.
func makeTarball(dir, out string) {
	tmp := out + ".tmp"
	f, err := os.Create(tmp)
	check(err)
	tar := exec.Command("tar", "-C", dir, "--sort=name", "--format=posix",
		"--pax-option=exthdr.name=%d/PaxHeaders/%f,delete=atime,delete=ctime",
		fmt.Sprintf("--mtime=@%d", SDE), "--owner=0", "--group=0", "--numeric-owner",
		"-cf", "-", ".")
	zstd := exec.Command("zstd", "-q", "-19", "-T1", "-c")
	pipe, err := tar.StdoutPipe()
	check(err)
	tar.Stderr = os.Stderr
	zstd.Stdin = pipe
	zstd.Stdout = f
	zstd.Stderr = os.Stderr
	check(tar.Start())
	check(zstd.Start())
	if err := tar.Wait(); err != nil {
		die("tar failed for %s: %v", dir, err)
	}
	if err := zstd.Wait(); err != nil {
		die("zstd failed for %s: %v", dir, err)
	}
	check(f.Close())
	check(os.Rename(tmp, out))
}

func buildOne(r *Recipe) {
	ih := inputHash(r)
	logf("build %s %s (%s)", r.Name, r.FullVersion(), ih[:12])
	bdir := filepath.Join(WORK, "build", r.Name)
	dest := filepath.Join(bdir, "dest")
	sysroot := filepath.Join(bdir, "sysroot")
	var sysPkgs []string
	if len(r.BuildDepends) > 0 {
		sysPkgs = runtimeClosure(r.BuildDepends)
	}
	// KLOS_RESUME=1 runs the script again in the source tree of a failed
	// build (for development only: the sources and patches are not new).
	resume := os.Getenv("KLOS_RESUME") == "1" && exists(filepath.Join(bdir, "S"))
	var S string
	if resume {
		b, err := os.ReadFile(filepath.Join(bdir, "S"))
		check(err)
		S = string(b)
		logf("resume the build in %s", rel(S))
		removeAll(dest)
		removeAll(filepath.Join(bdir, "bin"))
		removeAll(filepath.Join(bdir, "home"))
		removeAll(filepath.Join(bdir, "tmp"))
		mkdirAll(dest)
	} else {
		removeAll(bdir)
		mkdirAll(dest)
		mkdirAll(sysroot)
		for _, n := range sysPkgs {
			extractTarball(pkgTarball(n), sysroot, nil)
		}
	}
	env := buildEnv(r, bdir, sysroot)
	if !resume {
		S = extractSources(r, filepath.Join(bdir, "src"))
		patches, _ := filepath.Glob(filepath.Join(r.Dir, "*.patch"))
		sort.Strings(patches)
		for _, p := range patches {
			logf("apply %s", filepath.Base(p))
			run(Cmd{Args: []string{"patch", "-p1", "-N", "-i", p}, Dir: S, Quiet: true})
		}
		writeFile(filepath.Join(bdir, "S"), S, 0o644)
	}
	env = append(env, "S="+S)
	script := r.Build.Script
	if r.Kind == "target" {
		script = ". " + filepath.Join(TOP, "tools", "recipe-lib.sh") + "\n" + script
	}
	logdir := filepath.Join(WORK, "logs")
	mkdirAll(logdir)
	logfile := filepath.Join(logdir, r.Name+".log")
	lf, err := os.Create(logfile)
	check(err)
	cmd := exec.Command("bash", "-e", "-u", "-o", "pipefail", "-x", "-c", script)
	cmd.Dir, cmd.Env, cmd.Stdout, cmd.Stderr = S, env, lf, lf
	err = cmd.Run()
	lf.Close()
	if err != nil {
		tail, _ := exec.Command("tail", "-n", "60", logfile).Output()
		os.Stderr.Write(tail)
		die("build of %s failed; read %s", r.Name, rel(logfile))
	}
	postprocess(r, dest, env)

	depends := append([]string{}, r.Depends...)
	var provides []string
	if r.Kind == "target" {
		prov, needed := scanELF(dest)
		for so := range prov {
			provides = append(provides, so)
		}
		index := map[string]string{}
		for _, n := range sysPkgs {
			for _, so := range pkgMeta(n).Provides {
				if _, ok := index[so]; !ok {
					index[so] = n
				}
			}
		}
		var need []string
		for so := range needed {
			need = append(need, so)
		}
		sort.Strings(need)
		for _, so := range need {
			if prov[so] {
				continue
			}
			if p, ok := index[so]; ok {
				depends = append(depends, p)
			} else if !r.Options.AllowUnresolved {
				die("%s: no package provides %q", r.Name, so)
			}
		}
	}
	depends = uniqSorted(depends)
	sort.Strings(provides)
	if depends == nil {
		depends = []string{}
	}
	if provides == nil {
		provides = []string{}
	}
	mkdirAll(pkgDir(r.Name))
	tb := pkgTarball(r.Name)
	makeTarball(dest, tb)
	meta := PkgMeta{Name: r.Name, Version: r.FullVersion(), Kind: r.Kind,
		License: r.License, Summary: r.Summary, Input: ih, SHA256: sha256File(tb),
		Depends: depends, Provides: provides}
	b, _ := json.MarshalIndent(meta, "", " ")
	writeFile(filepath.Join(pkgDir(r.Name), r.Name+".json"), string(b)+"\n", 0o644)
	removeAll(bdir)
	logf("done %s: %s depends=%v", r.Name, meta.SHA256[:16], depends)
}

func build(names []string, force bool) {
	for _, r := range topoClosure(names) {
		if isCurrent(r) && !(force && contains(names, r.Name)) {
			continue
		}
		buildOne(r)
	}
}
