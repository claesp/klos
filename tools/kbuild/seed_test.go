// SPDX-License-Identifier: BSD-2-Clause

package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMinisignSignVerify(t *testing.T) {
	dir := t.TempDir()
	pk, sk, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pub := filepath.Join(dir, "key.pub")
	data := filepath.Join(dir, "seed.tar")
	if err := os.WriteFile(pub, []byte(minisignPublicKey(pk)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(data, []byte("the seed"), 0o644); err != nil {
		t.Fatal(err)
	}
	sig, err := minisignSign(sk, strings.NewReader("the seed"), "file:seed.tar")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(data+".minisig", []byte(sig), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := verifyMinisign(pub, data+".minisig", data); err != nil {
		t.Fatalf("good signature: %v", err)
	}
	if err := os.WriteFile(data, []byte("the seeD"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := verifyMinisign(pub, data+".minisig", data); err == nil {
		t.Fatal("changed data: no error")
	}
	if _, err := minisignSign(sk, strings.NewReader("x"), "two\nlines"); err == nil {
		t.Fatal("trusted comment with a new line: no error")
	}
}

func TestHumanSize(t *testing.T) {
	for n, want := range map[int64]string{0: "0 B", 1023: "1023 B", 1536: "1.5 KiB", 3 << 30: "3.0 GiB"} {
		if got := humanSize(n); got != want {
			t.Errorf("humanSize(%d) = %q, want %q", n, got, want)
		}
	}
}
