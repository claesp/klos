// SPDX-License-Identifier: BSD-2-Clause

package main

import (
	"bytes"
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

func download(url, dest string) {
	logf("download %s", url)
	client := &http.Client{Timeout: 30 * time.Minute}
	req, err := http.NewRequest("GET", url, nil)
	check(err)
	req.Header.Set("User-Agent", "klos-kbuild/1")
	resp, err := client.Do(req)
	check(err)
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		die("download %s: HTTP %s", url, resp.Status)
	}
	tmp := dest + ".part"
	f, err := os.Create(tmp)
	check(err)
	_, err = io.Copy(f, resp.Body)
	check(err)
	check(f.Close())
	check(os.Rename(tmp, dest))
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
		in.Close()
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
	defer os.RemoveAll(tmp)
	data := path
	if s.SignedContent == "uncompressed" {
		data = filepath.Join(tmp, "data")
		f, err := os.Create(data)
		check(err)
		run(Cmd{Args: []string{"xz", "-dc", path}, Stdout: f})
		f.Close()
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
		os.Remove(sig)
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
		os.Rename(p, p+".bad")
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
