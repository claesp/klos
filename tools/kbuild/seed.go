// SPDX-License-Identifier: BSD-2-Clause

package main

// The seed is the KLOS builder image (a tarball) that builds KLOS, and the
// next builder. builder/seed.conf has its SHA-256 value. "seed-export"
// writes the seed with a minisign signature and a checksum list, for a
// backup or a release. Anyone can check the signature with minisign(1) and
// keys/klos-seed.pub. Read docs/build-system.md.

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/blake2b"
)

func seedKeyPaths() (key, pub string) {
	return filepath.Join(SECRETS, "seed.key"), filepath.Join(KEYS, "klos-seed.pub")
}

// minisignKeyID is the key ID of a public key: the first 8 bytes of the
// BLAKE2b-256 hash of the key. (minisign makes a random ID; any value is
// correct, and this one needs no storage.)
func minisignKeyID(pk ed25519.PublicKey) []byte {
	h := blake2b.Sum256(pk)
	return h[:8]
}

// minisignPublicKey returns the text of a minisign public key file.
func minisignPublicKey(pk ed25519.PublicKey) string {
	id := minisignKeyID(pk)
	b := append(append([]byte("Ed"), id...), pk...)
	return fmt.Sprintf("untrusted comment: minisign public key %X\n%s\n", id,
		base64.StdEncoding.EncodeToString(b))
}

// minisignSign returns a minisign signature (algorithm ED: the signature is
// of the BLAKE2b-512 hash of the data) with a trusted comment.
func minisignSign(sk ed25519.PrivateKey, data io.Reader, trusted string) (string, error) {
	if strings.ContainsAny(trusted, "\r\n") {
		return "", errors.New("the trusted comment must be one line")
	}
	h, err := blake2b.New512(nil)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(h, data); err != nil {
		return "", err
	}
	pk, ok := sk.Public().(ed25519.PublicKey)
	if !ok {
		return "", errors.New("not an Ed25519 key")
	}
	id := minisignKeyID(pk)
	sig := ed25519.Sign(sk, h.Sum(nil))
	global := ed25519.Sign(sk, append(append([]byte{}, sig...), trusted...))
	return fmt.Sprintf("untrusted comment: signature from the KLOS seed key %X\n%s\ntrusted comment: %s\n%s\n",
		id, base64.StdEncoding.EncodeToString(append(append([]byte("ED"), id...), sig...)),
		trusted, base64.StdEncoding.EncodeToString(global)), nil
}

// cmdSeedKeygen makes the seed signing key: secrets/seed.key (private) and
// keys/klos-seed.pub (public, in the repository).
func cmdSeedKeygen(args []string) {
	force := len(args) == 1 && args[0] == "-force"
	if len(args) > 1 || (len(args) == 1 && !force) {
		die("usage: seed-keygen [-force]")
	}
	key, pub := seedKeyPaths()
	if exists(key) && !force {
		die("%s exists (use -force to replace it)", rel(key))
	}
	pk, sk, err := ed25519.GenerateKey(rand.Reader)
	check(err)
	skb, err := x509.MarshalPKCS8PrivateKey(sk)
	check(err)
	mkdirAll(SECRETS)
	writeFile(key, string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: skb})), 0o600)
	writeFile(pub, minisignPublicKey(pk), 0o644)
	logf("made %s and %s (commit the public key; keep a copy of the private key offline)", rel(key), rel(pub))
}

func readSeedKey() ed25519.PrivateKey {
	key, _ := seedKeyPaths()
	b, err := os.ReadFile(key)
	if err != nil {
		die("no seed signing key (run './klos seed-keygen'): %v", err)
	}
	blk, _ := pem.Decode(b)
	if blk == nil {
		die("%s: no PEM data", rel(key))
	}
	k, err := x509.ParsePKCS8PrivateKey(blk.Bytes)
	check(err)
	sk, ok := k.(ed25519.PrivateKey)
	if !ok {
		die("%s: not an Ed25519 key", rel(key))
	}
	return sk
}

func seedTarball(sha string) string {
	return filepath.Join(CACHE, "builder", "klos-builder-"+sha[:16]+".tar")
}

// cmdSeedExport writes the seed, its minisign signature, a SHA256SUMS file
// and seed.conf into a directory.
func cmdSeedExport(args []string) {
	if len(args) != 1 {
		die("usage: seed-export <directory>")
	}
	dir := args[0]
	sha, err := seedSHA256()
	check(err)
	src := seedTarball(sha)
	if got := sha256File(src); got != sha {
		die("%s: SHA-256 %s, but builder/seed.conf has %s", rel(src), got, sha)
	}
	sk := readSeedKey()
	_, pub := seedKeyPaths()
	pubText, err := os.ReadFile(pub)
	check(err)
	pk, ok := sk.Public().(ed25519.PublicKey)
	if !ok || string(pubText) != minisignPublicKey(pk) {
		die("%s is not the public key of %s", rel(pub), "secrets/seed.key")
	}
	mkdirAll(dir)
	name := filepath.Base(src)
	dst := filepath.Join(dir, name)
	if !exists(dst) || sha256File(dst) != sha {
		copyFile(src, dst)
	}
	f, err := os.Open(dst)
	check(err)
	trusted := fmt.Sprintf("timestamp:%d\tfile:%s\tsha256:%s\tklos-builder seed", time.Now().Unix(), name, sha)
	sig, err := minisignSign(sk, f, trusted)
	closeQuietly(f)
	check(err)
	writeFile(dst+".minisig", sig, 0o644)
	if err := verifyMinisign(pub, dst+".minisig", dst); err != nil {
		die("the new signature does not verify: %v", err)
	}
	writeFile(filepath.Join(dir, "SHA256SUMS"), sha+"  "+name+"\n", 0o644)
	conf, err := os.ReadFile(filepath.Join(TOP, "builder", "seed.conf"))
	check(err)
	writeFile(filepath.Join(dir, "seed.conf"), string(conf), 0o644)
	writeFile(filepath.Join(dir, "klos-seed.pub"), string(pubText), 0o644)
	shown := dir
	if v := os.Getenv("KLOS_EXTRA_DIR"); v != "" && dir == "/extra" {
		shown = v // the directory on the host
	}
	logf("wrote the seed to %s:", shown)
	for _, n := range []string{name, name + ".minisig", "SHA256SUMS", "seed.conf", "klos-seed.pub"} {
		logf("  %s", n)
	}
	logf("check it with: minisign -Vm %s -p klos-seed.pub (in that directory)", name)
}

// cmdSeedVerify checks the minisign signature of a seed tarball with
// keys/klos-seed.pub, and its SHA-256 value with builder/seed.conf.
func cmdSeedVerify(args []string) {
	if len(args) != 1 {
		die("usage: seed-verify <klos-builder-....tar>")
	}
	tar := args[0]
	_, pub := seedKeyPaths()
	if err := verifyMinisign(pub, tar+".minisig", tar); err != nil {
		die("%s: %v", tar, err)
	}
	logf("signature good: %s (key %s)", filepath.Base(tar), rel(pub))
	sha, err := seedSHA256()
	check(err)
	if got := sha256File(tar); got != sha {
		die("%s: the signature is good, but the SHA-256 value %s is not the one in builder/seed.conf (%s)", tar, got, sha)
	}
	logf("SHA-256 is the same as in builder/seed.conf")
}
