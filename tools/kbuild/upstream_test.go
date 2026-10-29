// SPDX-License-Identifier: BSD-2-Clause

package main

import (
	"reflect"
	"testing"
)

func TestCompareVersions(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want int
	}{
		{"1.2.3", "1.2.3", 0},
		{"1.2.10", "1.2.9", 1},
		{"1.2", "1.2.1", -1},
		{"10.6p1", "10.6p2", -1},
		{"2_37", "2.38", -1},
		{"20260930", "20260101", 1},
	} {
		if got := compareVersions(c.a, c.b); got != c.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestSameSeries(t *testing.T) {
	for _, c := range []struct {
		cur, v string
		want   bool
	}{
		{"6.18.58", "6.18.59", true},
		{"6.18.58", "6.19.1", false},
		{"1.19", "1.20", true},
		{"1.19", "2.0", false},
	} {
		if got := sameSeries(c.cur, c.v); got != c.want {
			t.Errorf("sameSeries(%q, %q) = %v, want %v", c.cur, c.v, got, c.want)
		}
	}
}

func TestPatternFor(t *testing.T) {
	re, ok := patternFor("dejavu-fonts-ttf-2.37.tar.bz2", "2.37")
	if !ok || !re.MatchString("dejavu-fonts-ttf-2.38.tar.bz2") || re.MatchString("dejavu-fonts-ttf-2.38.zip") {
		t.Fatalf("file pattern %v", re)
	}
	re, ok = patternFor("version_2_37", "2.37")
	if !ok {
		t.Fatal("no pattern for an underscore tag")
	}
	got := matchVersions([]string{"version_2_37", "version_2_38", "other_2_39"}, re, nil)
	if want := []string{"2.37", "2.38"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("matchVersions = %v, want %v", got, want)
	}
}

func TestParseLsRefs(t *testing.T) {
	data := pktLine("1111111111111111111111111111111111111111 refs/tags/v1.0\n") +
		pktLine("2222222222222222222222222222222222222222 refs/tags/v1.0^{}\n") +
		pktLine("3333333333333333333333333333333333333333 refs/heads/main\n") + "0000"
	got, err := parseLsRefs([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"v1.0"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("parseLsRefs = %v, want %v", got, want)
	}
}

func TestGitRepoFor(t *testing.T) {
	for u, want := range map[string]string{
		"https://github.com/lz4/lz4/releases/download/v1.10.0/lz4-1.10.0.tar.gz":           "https://github.com/lz4/lz4.git",
		"https://gitlab.freedesktop.org/slirp/libslirp/-/archive/v4.9.6/x.tar.gz":          "https://gitlab.freedesktop.org/slirp/libslirp.git",
		"https://git.sr.ht/~kennylevinsen/seatd/archive/0.9.3.tar.gz":                      "https://git.sr.ht/~kennylevinsen/seatd",
		"https://git.kernel.org/pub/scm/linux/kernel/git/xiang/erofs-utils.git/snapshot/x": "https://git.kernel.org/pub/scm/linux/kernel/git/xiang/erofs-utils.git",
	} {
		if got, ok := gitRepoFor(u); !ok || got != want {
			t.Errorf("gitRepoFor(%q) = %q, want %q", u, got, want)
		}
	}
	if _, ok := gitRepoFor("https://zlib.net/zlib-1.3.2.tar.xz"); ok {
		t.Error("zlib.net is not a forge")
	}
}
