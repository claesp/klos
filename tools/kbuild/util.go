// SPDX-License-Identifier: BSD-2-Clause

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

var (
	TOP, CACHE, SRCCACHE, OUT, PKGOUT, WORK, KEYS, SECRETS string
	CONFIG                                                 Config
	SDE                                                    int64
	VERSION                                                string
	JOBS                                                   string
)

const (
	toolchainPrefix = "opt/klos/toolchain"
	buildTriple     = "x86_64-alpine-linux-musl"
	kbuildVersion   = "kbuild-v1"
)

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
	OUT = filepath.Join(TOP, "out")
	PKGOUT = filepath.Join(OUT, "pkgs")
	WORK = filepath.Join(TOP, "work")
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
	defer f.Close()
	h := sha256.New()
	_, err = io.Copy(h, f)
	check(err)
	return hex.EncodeToString(h.Sum(nil))
}

func copyFile(src, dst string) {
	in, err := os.Open(src)
	check(err)
	defer in.Close()
	st, err := in.Stat()
	check(err)
	mkdirAll(filepath.Dir(dst))
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, st.Mode().Perm())
	check(err)
	_, err = io.Copy(out, in)
	check(err)
	check(out.Close())
}

// Cmd describes a command for run().
type Cmd struct {
	Args   []string
	Dir    string
	Env    []string
	Stdin  io.Reader
	Stdout io.Writer
	Quiet  bool // drop stdout
	NoFail bool // return the error instead of stopping
}

func run(c Cmd) error {
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
	err := cmd.Run()
	if err != nil && !c.NoFail {
		die("command failed: %s: %v", strings.Join(c.Args, " "), err)
	}
	return err
}

// output runs a command and returns its standard output.
func output(args ...string) string {
	out, err := exec.Command(args[0], args[1:]...).Output()
	if err != nil {
		die("command failed: %s: %v", strings.Join(args, " "), err)
	}
	return string(out)
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
