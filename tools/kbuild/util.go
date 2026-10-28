// SPDX-License-Identifier: BSD-2-Clause

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/BurntSushi/toml"
)

var (
	TOP, CACHE, SRCCACHE, OUT, PKGOUT, WORK, KEYS, SECRETS string
	CONFIG                                                 Config
	SDE                                                    int64
	VERSION                                                string
	JOBS                                                   string
	// BUILDER is the name of the output directories of the builder
	// (out/<name>, work/<name>; default klos). Read docs/build-system.md.
	BUILDER string
	// BCONF is /etc/klos-builder.conf of the builder.
	BCONF BuilderConf
)

const (
	toolchainPrefix = "opt/klos/toolchain"
	kbuildVersion   = "kbuild-v2"
	builderConfPath = "/etc/klos-builder.conf"
)

// BuilderConf tells kbuild about the compilers of the builder. Each line of
// /etc/klos-builder.conf is "key = value".
type BuilderConf struct {
	BuildTriple string // the --build triple of configure scripts
	HostCC      string // the C compiler of host packages
	HostCXX     string // the C++ compiler of host packages
	BuildCC     string // CC_FOR_BUILD of target packages
}

func readBuilderConf(path string) (BuilderConf, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return BuilderConf{}, err
	}
	return parseBuilderConf(string(b), path)
}

// parseBuilderConf reads the text of a builder configuration (name is the
// name of the file in messages).
func parseBuilderConf(text, path string) (BuilderConf, error) {
	c := BuilderConf{}
	for n, l := range strings.Split(text, "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		k, v, ok := strings.Cut(l, "=")
		if !ok {
			return c, fmt.Errorf("%s:%d: want key = value", path, n+1)
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		switch k {
		case "build-triple":
			c.BuildTriple = v
		case "host-cc":
			c.HostCC = v
		case "host-cxx":
			c.HostCXX = v
		case "build-cc":
			c.BuildCC = v
		default:
			return c, fmt.Errorf("%s:%d: unknown key %q", path, n+1, k)
		}
	}
	if c.BuildTriple == "" || c.HostCC == "" || c.HostCXX == "" || c.BuildCC == "" {
		return c, fmt.Errorf("%s: build-triple, host-cc, host-cxx and build-cc are necessary", path)
	}
	return c, nil
}

type Config struct {
	Distribution struct {
		Name            string `toml:"name"`
		ID              string `toml:"id"`
		Version         string `toml:"version"`
		SourceDateEpoch int64  `toml:"source-date-epoch"`
		Target          string `toml:"target"`
		Arch            string `toml:"arch"`
	} `toml:"distribution"`
	Flags struct {
		CFlags   string `toml:"cflags" json:"cflags"`
		CXXFlags string `toml:"cxxflags" json:"cxxflags"`
		LDFlags  string `toml:"ldflags" json:"ldflags"`
	} `toml:"flags"`
	Image struct {
		Exclude    []string `toml:"exclude"`
		SecureBoot bool     `toml:"secure-boot"`
	} `toml:"image"`
}

func initPaths() {
	TOP = os.Getenv("KLOS_TOP")
	if TOP == "" {
		TOP, _ = os.Getwd()
	}
	CACHE = filepath.Join(TOP, "cache")
	SRCCACHE = filepath.Join(CACHE, "sources")
	BUILDER = os.Getenv("KLOS_BUILDER")
	if BUILDER == "" {
		BUILDER = "klos"
	}
	OUT = filepath.Join(TOP, "out", BUILDER)
	PKGOUT = filepath.Join(OUT, "pkgs")
	// "./klos" mounts the work directory of the builder at /build
	// (KLOS_WORK), so that all builders have the same build paths.
	WORK = filepath.Join(TOP, "work", BUILDER)
	if v := os.Getenv("KLOS_WORK"); v != "" {
		WORK = v
	}
	KEYS = filepath.Join(TOP, "keys")
	SECRETS = filepath.Join(TOP, "secrets")
	loadTOML(filepath.Join(TOP, "config", "klos.toml"), &CONFIG)
	SDE = CONFIG.Distribution.SourceDateEpoch
	if v := os.Getenv("SOURCE_DATE_EPOCH"); v != "" {
		SDE, _ = strconv.ParseInt(v, 10, 64)
	}
	VERSION = CONFIG.Distribution.Version
	if v := os.Getenv("KLOS_VERSION"); v != "" {
		VERSION = v
	}
	if exists(builderConfPath) {
		c, err := readBuilderConf(builderConfPath)
		check(err)
		// Use absolute paths: in a target build, "gcc" and "clang" on the
		// PATH are the wrappers of the KLOS cross compiler.
		for _, p := range []*string{&c.HostCC, &c.HostCXX, &c.BuildCC} {
			if !filepath.IsAbs(*p) {
				if abs, err := exec.LookPath(*p); err == nil {
					*p = abs
				}
			}
		}
		BCONF = c
	}
	JOBS = os.Getenv("KLOS_JOBS")
	if JOBS == "" {
		JOBS = "4"
	}
}

func logf(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "kbuild: "+format+"\n", a...)
}

func die(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "kbuild: error: "+format+"\n", a...)
	os.Exit(1)
}

func check(err error) {
	if err != nil {
		die("%v", err)
	}
}

func loadTOML(path string, v any) toml.MetaData {
	md, err := toml.DecodeFile(path, v)
	if err != nil {
		die("%s: %v", path, err)
	}
	if und := md.Undecoded(); len(und) > 0 {
		die("%s: unknown keys: %v", path, und)
	}
	return md
}

func exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

func mkdirAll(p string) {
	check(os.MkdirAll(p, 0o755))
}

func removeAll(p string) {
	check(os.RemoveAll(p))
}

func writeFile(p, content string, mode os.FileMode) {
	mkdirAll(filepath.Dir(p))
	check(os.WriteFile(p, []byte(content), mode))
	check(os.Chmod(p, mode))
}

func sha256File(p string) string {
	f, err := os.Open(p)
	check(err)
	defer closeQuietly(f)
	h := sha256.New()
	_, err = io.Copy(h, f)
	check(err)
	return hex.EncodeToString(h.Sum(nil))
}

func copyFile(src, dst string) {
	in, err := os.Open(src)
	check(err)
	defer closeQuietly(in)
	st, err := in.Stat()
	check(err)
	mkdirAll(filepath.Dir(dst))
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, st.Mode().Perm())
	check(err)
	_, err = io.Copy(out, in)
	check(err)
	check(out.Close())
}

// Cmd describes a command for run and tryRun.
type Cmd struct {
	Args   []string
	Dir    string
	Env    []string
	Stdin  io.Reader
	Stdout io.Writer
	Quiet  bool // drop stdout
}

// tryRun runs a command and returns its error.
func tryRun(c Cmd) error {
	cmd := exec.Command(c.Args[0], c.Args[1:]...)
	cmd.Dir = c.Dir
	if c.Env != nil {
		cmd.Env = c.Env
	}
	cmd.Stdin = c.Stdin
	cmd.Stdout = os.Stderr
	if c.Stdout != nil {
		cmd.Stdout = c.Stdout
	} else if c.Quiet {
		cmd.Stdout = nil
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", strings.Join(c.Args, " "), err)
	}
	return nil
}

// run runs a command and stops kbuild if the command fails.
func run(c Cmd) {
	if err := tryRun(c); err != nil {
		die("command failed: %v", err)
	}
}

// closeQuietly closes a file that kbuild only read. A close error of a
// read-only file has no effect on the data.
func closeQuietly(c io.Closer) {
	_ = c.Close()
}

// output runs a command and returns its standard output.
func output(args ...string) string {
	out, err := exec.Command(args[0], args[1:]...).Output()
	if err != nil {
		die("command failed: %s: %v", strings.Join(args, " "), err)
	}
	return string(out)
}

// removeIfExists removes a file. A file that does not exist is not an error.
func removeIfExists(p string) {
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		die("remove %s: %v", p, err)
	}
}

func uniqSorted(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sortStrings(out)
	return out
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// moveFile moves a file. The work directory (/build) and the output
// directory are different mounts, so a rename can fail with EXDEV; then
// moveFile copies the file.
func moveFile(src, dst string) {
	err := os.Rename(src, dst)
	if err == nil {
		return
	}
	if !errors.Is(err, syscall.EXDEV) {
		check(err)
	}
	copyFile(src, dst)
	check(os.Remove(src))
}
