// SPDX-License-Identifier: BSD-2-Clause

package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/armor"
	"golang.org/x/crypto/blake2b"
)

func writeTestKey(t *testing.T, dir, name string, e *openpgp.Entity) {
	t.Helper()
	var buf bytes.Buffer
	w, err := armor.Encode(&buf, openpgp.PublicKeyType, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Serialize(w); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+".asc"), buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyDetached(t *testing.T) {
	dir := t.TempDir()
	KEYS = dir
	good, err := openpgp.NewEntity("good", "", "good@example.org", nil)
	if err != nil {
		t.Fatal(err)
	}
	other, err := openpgp.NewEntity("other", "", "other@example.org", nil)
	if err != nil {
		t.Fatal(err)
	}
	writeTestKey(t, dir, "good", good)
	writeTestKey(t, dir, "other", other)
	data := filepath.Join(dir, "data")
	if err := os.WriteFile(data, []byte("release"), 0o644); err != nil {
		t.Fatal(err)
	}
	var sig bytes.Buffer
	if err := openpgp.ArmoredDetachSign(&sig, good, bytes.NewReader([]byte("release")), nil); err != nil {
		t.Fatal(err)
	}
	sigPath := filepath.Join(dir, "data.sig")
	if err := os.WriteFile(sigPath, sig.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	ring, err := readKeyring([]string{"good"})
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyDetached(ring, sigPath, data, false); err != nil {
		t.Fatalf("good signature: %v", err)
	}
	otherRing, err := readKeyring([]string{"other"})
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyDetached(otherRing, sigPath, data, true); err == nil {
		t.Fatal("a signature of an unknown key verified")
	}
	if err := os.WriteFile(data, []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := verifyDetached(ring, sigPath, data, true); err == nil {
		t.Fatal("changed data verified")
	}
}

func TestVerifyMinisign(t *testing.T) {
	dir := t.TempDir()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	keyID := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	pk := append(append([]byte("Ed"), keyID...), pub...)
	pubPath := filepath.Join(dir, "key.pub")
	if err := os.WriteFile(pubPath, []byte("untrusted comment: test\n"+base64.StdEncoding.EncodeToString(pk)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	data := []byte("release")
	dataPath := filepath.Join(dir, "data")
	if err := os.WriteFile(dataPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	h := blake2b.Sum512(data)
	s := ed25519.Sign(priv, h[:])
	trusted := "timestamp:1 file:data"
	global := ed25519.Sign(priv, append(append([]byte{}, s...), trusted...))
	sig := append(append([]byte("ED"), keyID...), s...)
	sigText := "untrusted comment: sig\n" + base64.StdEncoding.EncodeToString(sig) + "\ntrusted comment: " + trusted + "\n" + base64.StdEncoding.EncodeToString(global) + "\n"
	sigPath := filepath.Join(dir, "data.minisig")
	if err := os.WriteFile(sigPath, []byte(sigText), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := verifyMinisign(pubPath, sigPath, dataPath); err != nil {
		t.Fatalf("good signature: %v", err)
	}
	bad := bytes.Replace([]byte(sigText), []byte("file:data"), []byte("file:other"), 1)
	if err := os.WriteFile(sigPath, bad, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := verifyMinisign(pubPath, sigPath, dataPath); err == nil {
		t.Fatal("a changed trusted comment verified")
	}
	if err := os.WriteFile(sigPath, []byte(sigText), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dataPath, []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := verifyMinisign(pubPath, sigPath, dataPath); err == nil {
		t.Fatal("changed data verified")
	}
}

func TestCPANChecksum(t *testing.T) {
	list := `$cksum = {
  'perl-5.42.3.tar.gz' => {
    'sha256' => 'aaaa',
    'size' => 1
  },
  'perl-5.42.3.tar.xz' => {
    'md5' => 'fb96',
    'sha256' => 'c9387e14',
    'size' => 14495300
  },
};`
	if got, ok := cpanChecksum(list, "perl-5.42.3.tar.xz"); !ok || got != "c9387e14" {
		t.Fatalf("cpanChecksum = %q, %v", got, ok)
	}
	if _, ok := cpanChecksum(list, "perl-5.42.4.tar.xz"); ok {
		t.Fatal("found a file that is not in the list")
	}
}
