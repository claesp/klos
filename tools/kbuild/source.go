// SPDX-License-Identifier: BSD-2-Clause

package main

import (
	"bytes"
	"context"
	"fmt"
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

// gpgKeyring makes a binary keyring from the armored key files in keys/.
func gpgKeyring(keys []string, dir string) string {
	ring := filepath.Join(dir, "ring.gpg")
	var buf bytes.Buffer
	for _, k := range keys {
		asc := filepath.Join(KEYS, k+".asc")
		in, err := os.Open(asc)
		if err != nil {
			die("missing key file %s", asc)
		}
		run(Cmd{Args: []string{"gpg", "--dearmor"}, Stdin: in, Stdout: &buf})
		closeQuietly(in)
	}
	check(os.WriteFile(ring, buf.Bytes(), 0o644))
	return ring
}

func verifySignature(s Source, path string) {
	if s.Signature == "" {
		return
	}
	if len(s.Key) == 0 {
		die("%s: a signature needs a key", s.URL)
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
	var stderr bytes.Buffer
	switch s.SignatureType {
	case "", "pgp":
		ring := gpgKeyring(s.Key, tmp)
		verr = runCapture(&stderr, "gpgv", "--keyring", ring, sig, data)
	case "minisign":
		verr = runCapture(&stderr, "minisign", "-V", "-q", "-p",
			filepath.Join(KEYS, s.Key[0]+".pub"), "-x", sig, "-m", data)
	default:
		die("unknown signature type %q", s.SignatureType)
	}
	if verr != nil {
		_ = os.Remove(sig) // download it again next time; the build stops
		die("signature check failed for %s:\n%s", filepath.Base(path), stderr.String())
	}
	logf("signature good: %s", s.filename())
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
