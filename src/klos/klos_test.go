// SPDX-License-Identifier: BSD-2-Clause

package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"2026.10.0", "2026.10.0", 0},
		{"2026.10.0.1", "2026.10.0", 1},
		{"2026.9.9", "2026.10.0", -1},
		{"2026.10.10", "2026.10.9", 1},
	}
	for _, tt := range tests {
		if got := compareVersions(tt.a, tt.b); got != tt.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestDmName(t *testing.T) {
	if got := dmName("klos", "os-2026.10.0"); got != "klos-os--2026.10.0" {
		t.Errorf("dmName = %q", got)
	}
}

func TestLoadOptionRoundTrip(t *testing.T) {
	p := &Partition{Number: 1, FirstLBA: 2048, LastLBA: 1050623}
	copy(p.PartGUID[:], bytes.Repeat([]byte{0xab}, 16))
	b := makeLoadOption(entryDesc("2026.10.0"), p, espFile("2026.10.0"))
	e, err := parseLoadOption(5, b)
	if err != nil {
		t.Fatal(err)
	}
	if e.Description != "KLOS 2026.10.0" || e.File != `\EFI\Linux\klos-2026.10.0.efi` {
		t.Errorf("got %+v", e)
	}
}

func TestUCS2(t *testing.T) {
	s, n := fromUCS2(ucs2("KLOS"))
	if s != "KLOS" || n != 10 {
		t.Errorf("fromUCS2(ucs2) = %q, %d", s, n)
	}
}

func TestRunEnv(t *testing.T) {
	f := filepath.Join(t.TempDir(), "kubeadm-flags.env")
	if err := os.WriteFile(f, []byte("KUBELET_KUBEADM_ARGS=\"--a=1 --b=2\"\n# comment\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	vars := map[string]string{}
	if err := readEnvFile(f, vars); err != nil {
		t.Fatal(err)
	}
	if err := readEnvFile(filepath.Join(t.TempDir(), "missing"), vars); err != nil {
		t.Errorf("a missing file must be empty: %v", err)
	}
	got := expandArgs([]string{"kubelet", "@KUBELET_KUBEADM_ARGS@", "@EXTRA@", "--x"}, vars)
	want := []string{"kubelet", "--a=1", "--b=2", "--x"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("expandArgs = %q, want %q", got, want)
	}
}

func TestPCRPolicy(t *testing.T) {
	pcr7 := make([]byte, sha256.Size)
	a, err := pcrPolicy(pcr7, false)
	if err != nil {
		t.Fatal(err)
	}
	b, err := pcrPolicy(pcr7, false)
	if err != nil {
		t.Fatal(err)
	}
	c, err := pcrPolicy(pcr7, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != sha256.Size || !bytes.Equal(a, b) {
		t.Error("the policy must be a deterministic SHA-256 digest")
	}
	if bytes.Equal(a, c) {
		t.Error("a PIN must change the policy")
	}
	if bytes.Equal(pinAuth("123456"), pinAuth("123457")) || len(pinAuth("x")) != sha256.Size {
		t.Error("pinAuth must give a different 32-byte value for each PIN")
	}
}

func TestParseUevent(t *testing.T) {
	ev := parseUevent([]byte("add@/devices/x\x00ACTION=add\x00MODALIAS=pci:v1\x00"))
	if ev["ACTION"] != "add" || ev["MODALIAS"] != "pci:v1" {
		t.Errorf("parseUevent = %v", ev)
	}
}

func TestReadInstallConfig(t *testing.T) {
	dir := t.TempDir()
	write := func(s string) string {
		p := filepath.Join(dir, "install.conf")
		if err := os.WriteFile(p, []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	c, err := readInstallConfig(write("disk=/dev/vda\nhostname=h1\nsecureboot=enroll\npoweroff=no\n"))
	if err != nil {
		t.Fatal(err)
	}
	if c.Disk != "/dev/vda" || c.Hostname != "h1" || c.SecureBoot != "enroll" || c.PowerOff {
		t.Errorf("got %+v", c)
	}
	if _, err := readInstallConfig(write("color=blue\n")); err == nil {
		t.Error("an unknown key must be an error")
	}
	if _, err := readInstallConfig(write("secureboot=maybe\n")); err == nil {
		t.Error("a bad secureboot value must be an error")
	}
}

// makeBundle writes a signed test bundle and returns its directory and the
// public key file.
func makeBundle(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	pubFile := filepath.Join(dir, "bundle.pub")
	if err := os.WriteFile(pubFile, pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}), 0o644); err != nil {
		t.Fatal(err)
	}
	info := BundleInfo{Distribution: "KLOS", Flavor: "server", Version: "2026.10.0",
		SlotSize: 1 << 20, RootHash: hex.EncodeToString(make([]byte, 32)), Files: map[string]BundleFile{}}
	for _, n := range bundleFiles {
		data := []byte("contents of " + n)
		if err := os.WriteFile(filepath.Join(dir, n), data, 0o644); err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		info.Files[n] = BundleFile{Size: int64(len(data)), SHA256: hex.EncodeToString(sum[:])}
	}
	js, err := json.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bundle.json"), js, 0o644); err != nil {
		t.Fatal(err)
	}
	sig := base64.StdEncoding.EncodeToString(ed25519.Sign(priv, js))
	if err := os.WriteFile(filepath.Join(dir, "bundle.sig"), []byte(sig), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir, pubFile
}

func TestVerifyBundle(t *testing.T) {
	dir, pub := makeBundle(t)
	if _, err := verifyBundle(dir, pub); err != nil {
		t.Fatalf("a good bundle: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "root.img"), []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := verifyBundle(dir, pub); err == nil {
		t.Error("a changed root.img must be an error")
	}
	dir, pub = makeBundle(t)
	js, err := os.ReadFile(filepath.Join(dir, "bundle.json"))
	if err != nil {
		t.Fatal(err)
	}
	js = bytes.Replace(js, []byte("2026.10.0"), []byte("2026.10.9"), 1)
	if err := os.WriteFile(filepath.Join(dir, "bundle.json"), js, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := verifyBundle(dir, pub); err == nil {
		t.Error("a changed bundle.json must be an error")
	}
}

func TestConfFiles(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	for _, f := range []string{filepath.Join(a, "10-x.conf"), filepath.Join(a, "20-y.conf"), filepath.Join(b, "10-x.conf")} {
		if err := os.WriteFile(f, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got := confFiles(a, b)
	want := []string{filepath.Join(b, "10-x.conf"), filepath.Join(a, "20-y.conf")}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("confFiles = %q, want %q", got, want)
	}
}

func accountLines(s string) *accountFile {
	return &accountFile{lines: strings.Split(strings.TrimSpace(s), "\n")}
}

func TestMergeAccountFiles(t *testing.T) {
	img := map[string]*accountFile{
		"passwd": accountLines("root:x:0:0:root:/root:/bin/sh\nsshd:x:22:22::/var/empty:/bin/false\nnew:x:90:90::/var/empty:/bin/false\nclash:x:1000:1000::/:/bin/false"),
		"group":  accountLines("root:x:0:\nsshd:x:22:\nvideo:x:27:new,old\nnew:x:90:new\nclashg:x:1000:"),
		"shadow": accountLines("root:!:1::::::\nsshd:!:1::::::\nnew:!:1::::::\nclash:!:1::::::"),
	}
	local := map[string]*accountFile{
		"passwd": accountLines("root:x:0:0:root:/root:/bin/sh\nsshd:x:22:22::/var/empty:/bin/false\nklos:x:1000:1000:klos:/home/klos:/bin/sh"),
		"group":  accountLines("root:x:0:\nsshd:x:22:\nvideo:x:27:klos\nklos:x:1000:"),
		"shadow": accountLines("root:HASH:0::::::\nsshd:!:1::::::\nklos:HASH:0::::::"),
	}
	r := mergeAccountFiles(img, local)
	if strings.Join(r.users, ",") != "new" || strings.Join(r.groups, ",") != "new" {
		t.Fatalf("added users %v, groups %v", r.users, r.groups)
	}
	if len(r.warnings) != 2 {
		t.Fatalf("warnings: %v", r.warnings)
	}
	wantGroup := "root:x:0:\nsshd:x:22:\nvideo:x:27:klos,new\nklos:x:1000:\nnew:x:90:new\n"
	if got := local["group"].String(); got != wantGroup {
		t.Fatalf("group:\n%s\nwant:\n%s", got, wantGroup)
	}
	if got := local["shadow"].String(); !strings.HasSuffix(got, "klos:HASH:0::::::\nnew:!:1::::::\n") || !strings.HasPrefix(got, "root:HASH:") {
		t.Fatalf("shadow:\n%s", got)
	}
	if !r.changed["passwd"] || !r.changed["group"] || !r.changed["shadow"] {
		t.Fatalf("changed: %v", r.changed)
	}
	// A second merge changes nothing.
	if r := mergeAccountFiles(img, local); len(r.changed) != 0 {
		t.Fatalf("second merge changed %v", r.changed)
	}
	// Without a local file, there is no merge.
	if r := mergeAccountFiles(img, map[string]*accountFile{}); len(r.changed) != 0 {
		t.Fatalf("merge without local files changed %v", r.changed)
	}
}

func TestCountSignatures(t *testing.T) {
	// Two lists: one X.509 entry of another owner, and two SHA-256
	// entries, one of them with the Microsoft owner.
	list := func(entries ...[]byte) []byte {
		sigSize := len(entries[0])
		b := make([]byte, 28)
		binary.LittleEndian.PutUint32(b[16:], uint32(28+sigSize*len(entries)))
		binary.LittleEndian.PutUint32(b[24:], uint32(sigSize))
		for _, e := range entries {
			b = append(b, e...)
		}
		return b
	}
	other := guidBytes("11111111-2222-3333-4444-555555555555")
	ms := guidBytes(microsoftOwner)
	b := append(list(append(append([]byte{}, other...), make([]byte, 100)...)),
		list(append(append([]byte{}, ms...), make([]byte, 32)...), append(append([]byte{}, other...), make([]byte, 32)...))...)
	n, m := countSignatures(b, microsoftOwner)
	if n != 3 || m != 1 {
		t.Fatalf("got %d entries, %d Microsoft; want 3, 1", n, m)
	}
	if g := guidBytes("8be4df61-93ca-11d2-aa0d-00e098032b8c"); g[0] != 0x61 || g[4] != 0xca || g[8] != 0xaa {
		t.Fatalf("guidBytes: % x", g)
	}
}
