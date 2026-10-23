// SPDX-License-Identifier: BSD-2-Clause

package main

// An update bundle is a directory or an uncompressed tar archive with:
//
//	bundle.json   the description (BundleInfo)
//	bundle.sig    the Ed25519 signature of bundle.json (base64)
//	root.img      the erofs OS image with the dm-verity hash tree
//	klos.efi      the kernel EFI stub with the initramfs and command line
//
// The public key is /usr/share/klos/bundle.pub (PEM, PKIX).

import (
	"archive/tar"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type BundleFile struct {
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type BundleInfo struct {
	Distribution string                `json:"distribution"`
	Flavor       string                `json:"flavor"`
	Version      string                `json:"version"`
	Created      int64                 `json:"created"`
	SlotSize     int64                 `json:"slot_size"`
	RootHash     string                `json:"roothash"`
	HashOffset   int64                 `json:"hashoffset"`
	Files        map[string]BundleFile `json:"files"`
}

var bundleFiles = []string{"root.img", "klos.efi"}

func loadPublicKey(path string) (ed25519.PublicKey, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	blk, _ := pem.Decode(b)
	if blk == nil || blk.Type != "PUBLIC KEY" {
		return nil, fmt.Errorf("%s: no PEM public key", path)
	}
	k, err := x509.ParsePKIXPublicKey(blk.Bytes)
	if err != nil {
		return nil, err
	}
	pk, ok := k.(ed25519.PublicKey)
	if !ok {
		return nil, fmt.Errorf("%s: not an Ed25519 key", path)
	}
	return pk, nil
}

// fetchBundle makes a local directory with the bundle files from a
// directory, a tar file or an https:// URL of a tar file.
func fetchBundle(src, workdir string) (string, error) {
	if strings.HasPrefix(src, "http://") {
		return "", errors.New("use https:// for bundles")
	}
	if strings.HasPrefix(src, "https://") {
		if err := os.MkdirAll(workdir, 0o700); err != nil {
			return "", err
		}
		tmp := filepath.Join(workdir, "download.tar")
		fmt.Printf("download %s\n", src)
		client := &http.Client{Timeout: 2 * time.Hour}
		resp, err := client.Get(src)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return "", fmt.Errorf("%s: %s", src, resp.Status)
		}
		f, err := os.Create(tmp)
		if err != nil {
			return "", err
		}
		if _, err := io.Copy(f, resp.Body); err != nil {
			f.Close()
			return "", err
		}
		f.Close()
		src = tmp
	}
	st, err := os.Stat(src)
	if err != nil {
		return "", err
	}
	if st.IsDir() {
		return src, nil
	}
	dir := filepath.Join(workdir, "bundle")
	os.RemoveAll(dir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	f, err := os.Open(src)
	if err != nil {
		return "", err
	}
	defer f.Close()
	tr := tar.NewReader(f)
	allowed := map[string]bool{"bundle.json": true, "bundle.sig": true}
	for _, n := range bundleFiles {
		allowed[n] = true
	}
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		name := filepath.Base(filepath.Clean(h.Name))
		if h.Typeflag != tar.TypeReg || !allowed[name] {
			continue
		}
		out, err := os.OpenFile(filepath.Join(dir, name), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			return "", err
		}
		if _, err := io.Copy(out, tr); err != nil {
			out.Close()
			return "", err
		}
		out.Close()
	}
	return dir, nil
}

// verifyBundle checks the signature, then the size and SHA-256 of each file.
func verifyBundle(dir, pubkey string) (*BundleInfo, error) {
	pk, err := loadPublicKey(pubkey)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(dir, "bundle.json"))
	if err != nil {
		return nil, err
	}
	sigText, err := os.ReadFile(filepath.Join(dir, "bundle.sig"))
	if err != nil {
		return nil, err
	}
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(sigText)))
	if err != nil {
		return nil, fmt.Errorf("bundle.sig: %w", err)
	}
	if !ed25519.Verify(pk, data, sig) {
		return nil, errors.New("the bundle signature is not valid")
	}
	info := &BundleInfo{}
	if err := json.Unmarshal(data, info); err != nil {
		return nil, err
	}
	for _, n := range bundleFiles {
		want, ok := info.Files[n]
		if !ok {
			return nil, fmt.Errorf("bundle.json has no entry for %s", n)
		}
		f, err := os.Open(filepath.Join(dir, n))
		if err != nil {
			return nil, err
		}
		h := sha256.New()
		size, err := io.Copy(h, f)
		f.Close()
		if err != nil {
			return nil, err
		}
		if size != want.Size || hex.EncodeToString(h.Sum(nil)) != want.SHA256 {
			return nil, fmt.Errorf("%s does not agree with bundle.json", n)
		}
	}
	if info.Version == "" || len(info.RootHash) != 64 || info.SlotSize <= 0 {
		return nil, errors.New("bundle.json is not complete")
	}
	if strings.ContainsAny(info.Version, "/\\ \t") {
		return nil, errors.New("bad version in bundle.json")
	}
	return info, nil
}
