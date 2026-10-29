// SPDX-License-Identifier: BSD-2-Clause

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func (s Source) filename() string {
	if s.Filename != "" {
		return s.Filename
	}
	u := strings.TrimRight(s.URL, "/")
	return u[strings.LastIndex(u, "/")+1:]
}

func (s Source) path() string {
	return filepath.Join(SRCCACHE, s.SHA256[:16]+"-"+s.filename())
}

// downloadTimeout limits one download (the largest sources are about 700 MB).
const downloadTimeout = 30 * time.Minute

func download(url, dest string) {
	logf("download %s", url)
	ctx, cancel := context.WithTimeout(context.Background(), downloadTimeout)
	defer cancel()
	if err := fetchURL(ctx, url, dest); err != nil {
		die("download %s: %v", url, err)
	}
}

// fetchURL writes the contents of url to dest (through dest.part).
func fetchURL(ctx context.Context, url, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "klos-kbuild/1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer closeQuietly(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %s", resp.Status)
	}
	tmp := dest + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		_ = f.Close() // the copy error is the error to report
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, dest)
}

func verifySignature(s Source, path string) {
	if s.SignatureType == "git-tag" {
		if len(s.Key) == 0 {
			die("%s: a signature needs a key", s.URL)
		}
		if err := verifyGitTag(s, path); err != nil {
			die("signature check failed for %s: %v", filepath.Base(path), err)
		}
		logf("signature good: %s", s.filename())
		return
	}
	if s.Signature == "" {
		return
	}
	sigstore := s.SignatureType == "sigstore" || s.SignatureType == "pypi-attestation"
	if len(s.Key) == 0 && !sigstore {
		die("%s: a signature needs a key", s.URL)
	}
	if sigstore && (s.Identity == "" || s.Issuer == "") {
		die("%s: a Sigstore signature needs identity and issuer", s.URL)
	}
	sig := path + ".sig"
	if !exists(sig) {
		download(s.Signature, sig)
	}
	tmp, err := os.MkdirTemp("", "kbuild-sig")
	check(err)
	defer func() { _ = os.RemoveAll(tmp) }() // temporary files only
	data := path
	if s.SignedContent == "uncompressed" {
		data = filepath.Join(tmp, "data")
		f, err := os.Create(data)
		check(err)
		run(Cmd{Args: []string{"xz", "-dc", path}, Stdout: f})
		check(f.Close())
	}
	var verr error
	switch s.SignatureType {
	case "", "pgp":
		ring, err := readKeyring(s.Key)
		check(err)
		verr = verifyDetached(ring, sig, data, s.AllowExpiredKey)
	case "minisign":
		verr = verifyMinisign(filepath.Join(KEYS, s.Key[0]+".pub"), sig, data)
	case "pgp-checksum-file":
		// The signature is a detached signature of a checksum list
		// (for example cmake-<version>-SHA-256.txt).
		if s.Checksums == "" {
			die("%s: signature-type pgp-checksum-file needs checksums", s.URL)
		}
		sums := path + ".sums"
		if !exists(sums) {
			download(s.Checksums, sums)
		}
		ring, err := readKeyring(s.Key)
		check(err)
		verr = verifyDetached(ring, sig, sums, s.AllowExpiredKey)
		if verr == nil {
			var b []byte
			if b, verr = os.ReadFile(sums); verr == nil {
				verr = checkChecksumList(string(b), filepath.Base(s.URL), data)
			}
		}
		if verr != nil {
			_ = os.Remove(sums) // download it again next time; the build stops
		}
	case "pgp-checksums":
		// The signature file is a signed OpenPGP message with a
		// checksum list.
		ring, err := readKeyring(s.Key)
		check(err)
		var text string
		if text, verr = verifyMessage(ring, sig); verr == nil {
			verr = checkChecksumList(text, filepath.Base(s.URL), data)
		}
	case "sigstore":
		if s.Certificate == "" {
			die("%s: signature-type sigstore needs certificate", s.URL)
		}
		cert := path + ".cert"
		if !exists(cert) {
			download(s.Certificate, cert)
		}
		verr = verifySigstoreBlob(s, sig, cert, data)
		if verr != nil {
			_ = os.Remove(cert) // download it again next time; the build stops
		}
	case "pypi-attestation":
		verr = verifyPyPIProvenance(s, sig, data)
	default:
		die("unknown signature type %q", s.SignatureType)
	}
	if verr != nil {
		_ = os.Remove(sig) // download it again next time; the build stops
		die("signature check failed for %s: %v", filepath.Base(path), verr)
	}
	logf("signature good: %s", s.filename())
}

// checkChecksumList finds the line for name in a sha256sum(1) or
// sha512sum(1) list, and compares the value with the file at path. The
// length of the value selects the algorithm.
func checkChecksumList(list, name, path string) error {
	if sum, ok := cpanChecksum(list, name); ok {
		return compareChecksum(name, sum, path)
	}
	for _, l := range strings.Split(list, "\n") {
		f := strings.Fields(l)
		if len(f) != 2 || strings.TrimPrefix(f[1], "*") != name {
			continue
		}
		return compareChecksum(name, f[0], path)
	}
	return fmt.Errorf("the signed checksum list has no line for %s", name)
}

// compareChecksum compares a SHA-256 or SHA-512 value (the length selects
// the algorithm) with the file at path.
func compareChecksum(name, sum, path string) error {
	var h hash.Hash
	switch len(sum) {
	case 64:
		h = sha256.New()
	case 128:
		h = sha512.New()
	default:
		return fmt.Errorf("%s: unknown checksum length %d", name, len(sum))
	}
	in, err := os.Open(path)
	if err != nil {
		return err
	}
	defer closeQuietly(in)
	if _, err := io.Copy(h, in); err != nil {
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != strings.ToLower(sum) {
		return fmt.Errorf("%s: the signed checksum is %s, the file has %s", name, sum, got)
	}
	return nil
}

// cpanChecksum finds the SHA-256 value of a file in a CPAN CHECKSUMS file
// (a Perl data structure: 'name' => { ..., 'sha256' => 'value', ... }).
func cpanChecksum(list, name string) (string, bool) {
	start := strings.Index(list, "'"+name+"' => {")
	if start < 0 {
		return "", false
	}
	block := list[start:]
	if end := strings.Index(block, "}"); end >= 0 {
		block = block[:end]
	}
	for _, l := range strings.Split(block, "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(l), "'sha256' => '"); ok {
			return strings.TrimSuffix(strings.TrimSuffix(v, ","), "'"), true
		}
	}
	return "", false
}

// runCapture runs a command and keeps its error output in stderr.
func runCapture(stderr *bytes.Buffer, args ...string) error {
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stderr = stderr
	return cmd.Run()
}

func fetchSource(s Source) string {
	mkdirAll(SRCCACHE)
	p := s.path()
	if exists(p) {
		return p
	}
	download(s.URL, p)
	if got := sha256File(p); got != s.SHA256 {
		_ = os.Rename(p, p+".bad") // keep the file for inspection; the build stops
		die("SHA-256 mismatch for %s\n  expected %s\n  got      %s", s.URL, s.SHA256, got)
	}
	verifySignature(s, p)
	return p
}

// checkSource returns the cached path of a source after a new SHA-256 check.
func checkSource(s Source) string {
	p := s.path()
	if !exists(p) {
		die("source not in cache: %s (run './klos fetch')", s.URL)
	}
	if sha256File(p) != s.SHA256 {
		die("cached source changed: %s", p)
	}
	return p
}
