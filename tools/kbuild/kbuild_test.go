// SPDX-License-Identifier: BSD-2-Clause

package main

import (
	"bytes"
	"os"
	"path/filepath"
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
