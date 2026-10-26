// SPDX-License-Identifier: BSD-2-Clause

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCpioDeterministic(t *testing.T) {
	SDE = 1792800000
	dir := t.TempDir()
	make1 := func(path string, order []string) []byte {
		c := newCpio()
		for _, n := range order {
			switch n {
			case "dev":
				c.dir("dev", 0o755)
			case "dev/console":
				c.chardev("dev/console", 0o600, 5, 1)
			case "init":
				c.file("init", 0o755, []byte("program"))
			case "lib":
				c.symlink("lib", "usr/lib")
			}
		}
		c.write(path)
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	a := make1(filepath.Join(dir, "a.cpio"), []string{"init", "dev", "lib", "dev/console"})
	b := make1(filepath.Join(dir, "b.cpio"), []string{"dev/console", "lib", "dev", "init"})
	if !bytes.Equal(a, b) {
		t.Error("the archive must not depend on the order of the calls")
	}
	if !bytes.HasPrefix(a, []byte("070701")) || !bytes.Contains(a, []byte("TRAILER!!!")) {
		t.Error("not a newc archive")
	}
	if len(a)%4 != 0 {
		t.Errorf("the archive size %d is not a multiple of 4", len(a))
	}
}

func TestParseSize(t *testing.T) {
	for in, want := range map[string]int64{"512M": 512 << 20, "2G": 2 << 30, "4096": 4096, "8K": 8 << 10} {
		if got := parseSize(in); got != want {
			t.Errorf("parseSize(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestExcluded(t *testing.T) {
	CONFIG.Image.Exclude = []string{"usr/include/*", "usr/lib/*.a"}
	tests := []struct {
		path  string
		extra []string
		want  bool
	}{
		{"usr/include/stdio.h", nil, true},
		{"usr/include/sys/types.h", nil, true},
		{"usr/lib/libc.a", nil, true},
		{"usr/lib/libc.so", nil, false},
		{"usr/bin/llvm-config", []string{"usr/bin/*"}, true},
	}
	for _, tt := range tests {
		if got := excluded(tt.path, tt.extra); got != tt.want {
			t.Errorf("excluded(%q, %q) = %v, want %v", tt.path, tt.extra, got, tt.want)
		}
	}
}

func TestReadConfigFragments(t *testing.T) {
	TOP = t.TempDir()
	cfg := filepath.Join(TOP, "kernel", "config")
	if err := os.MkdirAll(cfg, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"a.config": "# comment\nCONFIG_A=y\nCONFIG_B=n\nCONFIG_S=\"x\"\n",
		"b.config": "CONFIG_B=y\nCONFIG_C=m\n",
	}
	for n, s := range files {
		if err := os.WriteFile(filepath.Join(cfg, n), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	opts, text := readConfigFragments([]string{"a", "b"})
	want := [][2]string{{"CONFIG_A", "y"}, {"CONFIG_B", "y"}, {"CONFIG_S", `"x"`}, {"CONFIG_C", "m"}}
	if len(opts) != len(want) {
		t.Fatalf("opts = %v, want %v", opts, want)
	}
	for i := range want {
		if opts[i] != want[i] {
			t.Errorf("opts[%d] = %v, want %v (a later fragment replaces a value)", i, opts[i], want[i])
		}
	}
	if text != "CONFIG_A=y\nCONFIG_B=y\nCONFIG_S=\"x\"\nCONFIG_C=m\n" {
		t.Errorf("text = %q", text)
	}
}

func TestCheckChecksumList(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.tar.xz")
	if err := os.WriteFile(p, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	s256 := "3a6eb0790f39ac87c94f3856b2dd2c5d110e6811602261a9a923d3bb23adc8b7"
	s512 := "77c7ce9a5d86bb386d443bb96390faa120633158699c8844c30b13ab0bf92760b7e4416aea397db91b4ac0e5dd56b8ef7e4b066162ab1fdc088319ce6defc876"
	for _, c := range []struct {
		list string
		ok   bool
	}{
		{s256 + " *a.tar.xz\n", true},
		{s512 + "  a.tar.xz\n", true},
		{s256 + "  b.tar.xz\n", false},
		{strings.Repeat("0", 64) + "  a.tar.xz\n", false},
		{"abc  a.tar.xz\n", false},
	} {
		if err := checkChecksumList(c.list, "a.tar.xz", p); (err == nil) != c.ok {
			t.Errorf("%q: error %v", c.list, err)
		}
	}
}

func TestReproducible(t *testing.T) {
	a := "# comment a\n[image]\nroothash = \"1\"\n\n[[tool]]\nname = \"llvm\"\nsha256 = \"aa\"\n\n[[package]]\nname = \"musl\"\nsha256 = \"cc\"\n"
	b := "# comment b\n[image]\nroothash = \"1\"\n\n[[tool]]\nname = \"llvm\"\nsha256 = \"bb\"\n\n[[package]]\nname = \"musl\"\nsha256 = \"cc\"\n"
	if reproducible(a) != reproducible(b) {
		t.Fatalf("a different tool changed the result:\n%s\n---\n%s", reproducible(a), reproducible(b))
	}
	if c := strings.Replace(b, "\"cc\"", "\"dd\"", 1); reproducible(a) == reproducible(c) {
		t.Fatal("a different package did not change the result")
	}
}

func TestParseBuilderConf(t *testing.T) {
	c, err := parseBuilderConf("# comment\nbuild-triple = x86_64-pc-linux-gnu\nhost-cc = gcc\nhost-cxx = g++\nbuild-cc=gcc\n", "test")
	if err != nil {
		t.Fatal(err)
	}
	if c.BuildTriple != "x86_64-pc-linux-gnu" || c.HostCC != "gcc" || c.HostCXX != "g++" || c.BuildCC != "gcc" {
		t.Fatalf("got %+v", c)
	}
	for _, bad := range []string{"host-cc = gcc\n", "build-triple x\n", "colour = red\nbuild-triple=a\nhost-cc=b\nhost-cxx=c\nbuild-cc=d\n"} {
		if _, err := parseBuilderConf(bad, "test"); err == nil {
			t.Errorf("%q: no error", bad)
		}
	}
}
