// SPDX-License-Identifier: BSD-2-Clause

package main

import (
	"archive/tar"
	"bytes"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const testClassMap = `
#define COMMON_A \
	"read", "write"
#define COMMON_B COMMON_A, "exec"
#ifdef __KERNEL__
#error not this
#endif
/* a comment with { "braces" } */
const struct security_class_mapping secclass_map[] = {
	{ "security", { "load_policy", "setenforce", NULL } },
	{ "file",
	  { COMMON_B, "entrypoint", NULL } },
	{ NULL }
};
`

func TestParseClassMap(t *testing.T) {
	got, err := parseClassMap(testClassMap)
	if err != nil {
		t.Fatal(err)
	}
	want := []seClass{
		{"security", []string{"load_policy", "setenforce"}},
		{"file", []string{"read", "write", "exec", "entrypoint"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestParseInitialSIDs(t *testing.T) {
	text := `static const char *const initial_sid_to_string[] = {
	NULL, /* zero placeholder, not used */
	"kernel", /* kernel / SECINITSID_KERNEL */
	NULL, /* fs */
	"devnull", /* devnull / SECINITSID_DEVNULL */
};`
	if got := parseInitialSIDs(text); !reflect.DeepEqual(got, []string{"kernel", "fs", "devnull"}) {
		t.Fatalf("got %v", got)
	}
}

func TestPolicyVersion(t *testing.T) {
	text := "#define POLICYDB_VERSION_NEVERAUDIT 35\n#define POLICYDB_VERSION_MAX POLICYDB_VERSION_NEVERAUDIT\n"
	if v, err := policyVersion(text); err != nil || v != 35 {
		t.Fatalf("got %d, %v", v, err)
	}
}

func TestKernelCIL(t *testing.T) {
	classes := []seClass{{"security", []string{"load_policy", "compute_av"}}, {"dir", []string{"read"}}}
	cil := kernelCIL(classes, []string{"kernel", "fs"}, "(sidcontext kernel (u r t l))")
	for _, want := range []string{
		"(allow unconfined_domain any_type (security (compute_av)))",
		"(allow unconfined_domain any_type (dir (all)))",
		"(sidcontext fs (system_u object_r unlabeled_t low_low))",
		"(sidorder (kernel fs))",
	} {
		if !strings.Contains(cil, want) {
			t.Errorf("no %q", want)
		}
	}
	if strings.Contains(cil, "load_policy)))") || strings.Contains(cil, "(sidcontext kernel (system_u") {
		t.Error("a restricted permission or a second kernel sidcontext")
	}
}

func TestFileContexts(t *testing.T) {
	fc, err := parseFileContexts(`
/.*		u:object_r:root_t:s0
/etc(/.*)?	u:object_r:etc_t:s0
/etc/shadow --	u:object_r:shadow_t:s0
/dev/pts(/.*)?	<<none>>
`)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		path string
		mode fs.FileMode
		want string
	}{
		{"/", fs.ModeDir, "u:object_r:root_t:s0"},
		{"/etc", fs.ModeDir, "u:object_r:etc_t:s0"},
		{"/etc/shadow", 0, "u:object_r:shadow_t:s0"},
		{"/etc/shadow", fs.ModeDir, "u:object_r:etc_t:s0"},
		{"/etcx", 0, "u:object_r:root_t:s0"},
		{"/dev/pts/0", fs.ModeCharDevice | fs.ModeDevice, ""},
	} {
		if got := fc.lookup(c.path, c.mode); got != c.want {
			t.Errorf("%s: got %q, want %q", c.path, got, c.want)
		}
	}
	for _, bad := range []string{"/x -q u:r:t:s0", "/x", "( u:r:t:s0", "/x notacontext"} {
		if _, err := parseFileContexts(bad); err == nil {
			t.Errorf("%q: no error", bad)
		}
	}
	upper := etcUpperContexts("/etc(/.*)?\tu:object_r:etc_t:s0\n/var(/.*)?\tu:object_r:var_t:s0\n")
	if !strings.Contains(upper, "/var/lib/klos/etc/upper(/.*)?\tu:object_r:etc_t:s0") || strings.Contains(upper, "var_t") {
		t.Errorf("upper contexts: %q", upper)
	}
}

func TestWriteLabeledTar(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "etc"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "etc/a"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(filepath.Join(root, "etc/a"), filepath.Join(root, "etc/b")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("a", filepath.Join(root, "etc/c")); err != nil {
		t.Fatal(err)
	}
	fc, err := parseFileContexts("/.* u:object_r:root_t:s0\n/etc(/.*)? u:object_r:etc_t:s0\n")
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := writeLabeledTar(root, fc, &buf); err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(&buf)
	got := map[string]string{}
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		got[h.Name] = string(h.Typeflag) + " " + h.PAXRecords["SCHILY.xattr.security.selinux"]
		if h.Uid != 0 || h.ModTime.Unix() != SDE {
			t.Errorf("%s: uid %d, time %v", h.Name, h.Uid, h.ModTime)
		}
	}
	want := map[string]string{
		"./":    "5 u:object_r:root_t:s0",
		"etc/":  "5 u:object_r:etc_t:s0",
		"etc/a": "0 u:object_r:etc_t:s0",
		"etc/b": "1 ",
		"etc/c": "2 u:object_r:etc_t:s0",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	// A file without a label is an error.
	fc2, _ := parseFileContexts("/etc(/.*)? u:object_r:etc_t:s0\n")
	if err := writeLabeledTar(root, fc2, io.Discard); err == nil {
		t.Fatal("no error for an unlabeled file")
	}
}
