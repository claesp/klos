// SPDX-License-Identifier: BSD-2-Clause

package main

// Signed Git tags (signature-type git-tag). Many projects sign their Git
// tags, but not the release tarballs. kbuild gets the tag, the commit and
// the trees (no file contents) with the Git protocol version 2 over HTTP,
// checks the OpenPGP signature of the tag, and compares each file of the
// tarball with the Git blob of the same path in the signed tree. A file
// that the release process adds or changes (for example a generated
// file) must be in extra-files of the recipe. Read docs/sources-and-keys.md.

import (
	"bytes"
	"compress/zlib"
	"context"
	"crypto/sha1" // Git object IDs are SHA-1 values (read gitObjectID)
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/ProtonMail/go-crypto/openpgp"
)

// gitObject is an object of a pack file.
type gitObject struct {
	typ  string // commit, tree, blob or tag
	data []byte
}

// gitObjectID returns the SHA-1 object ID. The signed tag names the commit
// by its ID, and the commit names the tree by its ID.
func gitObjectID(typ string, data []byte) string {
	h := sha1.New()
	_, _ = fmt.Fprintf(h, "%s %d\x00", typ, len(data)) // hash.Hash.Write never fails
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

// gitPost sends a protocol version 2 command and returns the answer.
func gitPost(ctx context.Context, repo, body string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimSuffix(repo, "/")+"/git-upload-pack",
		strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "git/2.50-klos-kbuild")
	req.Header.Set("Git-Protocol", "version=2")
	req.Header.Set("Content-Type", "application/x-git-upload-pack-request")
	req.Header.Set("Accept", "application/x-git-upload-pack-result")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer closeQuietly(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: HTTP %s", repo, resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 1<<30))
}

// gitCapabilities returns the capabilities of the fetch command.
func gitCapabilities(ctx context.Context, repo string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSuffix(repo, "/")+"/info/refs?service=git-upload-pack", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "git/2.50-klos-kbuild")
	req.Header.Set("Git-Protocol", "version=2")
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer closeQuietly(resp.Body)
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if !bytes.Contains(b, []byte("version 2")) {
		return "", fmt.Errorf("%s: the server does not have Git protocol version 2", repo)
	}
	for _, l := range pktLines(b) {
		if v, ok := strings.CutPrefix(strings.TrimSpace(string(l)), "fetch="); ok {
			return v, nil
		}
	}
	return "", nil
}

// pktLines splits pkt-line data into the payloads (flush, delimiter and
// end packets are not in the result).
func pktLines(data []byte) [][]byte {
	var out [][]byte
	for len(data) >= 4 {
		n, err := strconv.ParseUint(string(data[:4]), 16, 16)
		if err != nil || (n >= 4 && int(n) > len(data)) {
			break
		}
		if n < 4 {
			data = data[4:]
			continue
		}
		out = append(out, data[4:n])
		data = data[n:]
	}
	return out
}

// gitTagObjectID returns the object ID of an annotated tag.
func gitTagObjectID(ctx context.Context, repo, tag string) (string, error) {
	body := pktLine("command=ls-refs\n") + "0001" + pktLine("peel\n") + pktLine("ref-prefix refs/tags/"+tag+"\n") + "0000"
	b, err := gitPost(ctx, repo, body)
	if err != nil {
		return "", err
	}
	for _, l := range pktLines(b) {
		f := strings.Fields(string(l))
		if len(f) >= 2 && f[1] == "refs/tags/"+tag {
			if len(f) < 3 || !strings.HasPrefix(f[2], "peeled:") {
				return "", fmt.Errorf("%s: tag %s is not an annotated tag (it cannot have a signature)", repo, tag)
			}
			return f[0], nil
		}
	}
	return "", fmt.Errorf("%s: no tag %s", repo, tag)
}

// gitFetch gets an object (a tag or a commit), the commit and the trees of
// the commit, without the file contents.
func gitFetch(ctx context.Context, repo, id string) (map[string]gitObject, error) {
	caps, err := gitCapabilities(ctx, repo)
	if err != nil {
		return nil, err
	}
	body := pktLine("command=fetch\n") + "0001" + pktLine("ofs-delta\n") + pktLine("no-progress\n") +
		pktLine("want "+id+"\n") + pktLine("deepen 1\n")
	if strings.Contains(" "+caps+" ", " filter ") {
		body += pktLine("filter blob:none\n")
	}
	body += pktLine("done\n") + "0000"
	b, err := gitPost(ctx, repo, body)
	if err != nil {
		return nil, err
	}
	pack, err := packFromResponse(b)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", repo, err)
	}
	return parsePack(pack)
}

// packFromResponse returns the pack file of a fetch answer (side band 1 of
// the packfile section).
func packFromResponse(b []byte) ([]byte, error) {
	var pack bytes.Buffer
	in := false
	for _, l := range pktLines(b) {
		if !in {
			in = string(l) == "packfile\n"
			continue
		}
		if len(l) == 0 {
			continue
		}
		switch l[0] {
		case 1:
			pack.Write(l[1:])
		case 3:
			return nil, fmt.Errorf("server error: %s", strings.TrimSpace(string(l[1:])))
		}
	}
	if pack.Len() == 0 {
		return nil, errors.New("the answer has no pack file")
	}
	return pack.Bytes(), nil
}

var packTypes = map[int]string{1: "commit", 2: "tree", 3: "blob", 4: "tag"}

// parsePack reads all objects of a pack file (version 2) and resolves
// the deltas. The result maps object IDs to objects.
func parsePack(p []byte) (map[string]gitObject, error) {
	if len(p) < 12 || string(p[:4]) != "PACK" || binary.BigEndian.Uint32(p[4:8]) != 2 {
		return nil, errors.New("not a version 2 pack file")
	}
	count := int(binary.BigEndian.Uint32(p[8:12]))
	type entry struct {
		typ      int
		data     []byte
		baseOff  int    // OFS_DELTA
		baseID   string // REF_DELTA
		resolved *gitObject
	}
	byOff := map[int]*entry{}
	var order []int
	off := 12
	for i := 0; i < count; i++ {
		start := off
		if off >= len(p) {
			return nil, errors.New("short pack file")
		}
		c := p[off]
		off++
		typ := int(c>>4) & 7
		size := int(c & 15)
		shift := 4
		for c&0x80 != 0 {
			if off >= len(p) {
				return nil, errors.New("short pack file")
			}
			c = p[off]
			off++
			size |= int(c&0x7f) << shift
			shift += 7
		}
		e := &entry{typ: typ}
		switch typ {
		case 6: // OFS_DELTA
			c = p[off]
			off++
			o := int(c & 0x7f)
			for c&0x80 != 0 {
				c = p[off]
				off++
				o = ((o + 1) << 7) | int(c&0x7f)
			}
			e.baseOff = start - o
		case 7: // REF_DELTA
			e.baseID = hex.EncodeToString(p[off : off+20])
			off += 20
		}
		r := bytes.NewReader(p[off:])
		zr, err := zlib.NewReader(r)
		if err != nil {
			return nil, fmt.Errorf("object %d: %w", i, err)
		}
		data, err := io.ReadAll(zr)
		if err != nil {
			return nil, fmt.Errorf("object %d: %w", i, err)
		}
		if len(data) != size {
			return nil, fmt.Errorf("object %d: size %d, want %d", i, len(data), size)
		}
		off = len(p) - r.Len()
		e.data = data
		byOff[start] = e
		order = append(order, start)
	}
	objs := map[string]gitObject{}
	var resolve func(e *entry, depth int) (*gitObject, error)
	resolve = func(e *entry, depth int) (*gitObject, error) {
		if e.resolved != nil {
			return e.resolved, nil
		}
		if depth > 1000 {
			return nil, errors.New("delta chain too long")
		}
		var o gitObject
		switch e.typ {
		case 1, 2, 3, 4:
			o = gitObject{packTypes[e.typ], e.data}
		case 6, 7:
			var base *gitObject
			if e.typ == 6 {
				be, ok := byOff[e.baseOff]
				if !ok {
					return nil, errors.New("delta base not in the pack file")
				}
				b, err := resolve(be, depth+1)
				if err != nil {
					return nil, err
				}
				base = b
			} else {
				b, ok := objs[e.baseID]
				if !ok {
					// The base can be later in the pack file.
					for _, so := range order {
						if r, err := resolve(byOff[so], depth+1); err == nil && gitObjectID(r.typ, r.data) == e.baseID {
							b, ok = *r, true
							break
						}
					}
				}
				if !ok {
					return nil, fmt.Errorf("delta base %s not in the pack file", e.baseID)
				}
				base = &b
			}
			d, err := applyDelta(base.data, e.data)
			if err != nil {
				return nil, err
			}
			o = gitObject{base.typ, d}
		default:
			return nil, fmt.Errorf("unknown object type %d", e.typ)
		}
		e.resolved = &o
		return &o, nil
	}
	for _, so := range order {
		o, err := resolve(byOff[so], 0)
		if err != nil {
			return nil, err
		}
		objs[gitObjectID(o.typ, o.data)] = *o
	}
	return objs, nil
}

func deltaSize(d []byte, i *int) (int, error) {
	n, shift := 0, 0
	for {
		if *i >= len(d) {
			return 0, errors.New("short delta")
		}
		c := d[*i]
		*i++
		n |= int(c&0x7f) << shift
		shift += 7
		if c&0x80 == 0 {
			return n, nil
		}
	}
}

// applyDelta makes an object from a base and a Git delta.
func applyDelta(base, d []byte) ([]byte, error) {
	i := 0
	bs, err := deltaSize(d, &i)
	if err != nil {
		return nil, err
	}
	if bs != len(base) {
		return nil, errors.New("delta base size mismatch")
	}
	rs, err := deltaSize(d, &i)
	if err != nil {
		return nil, err
	}
	out := make([]byte, 0, rs)
	for i < len(d) {
		c := d[i]
		i++
		if c&0x80 != 0 {
			var off, size int
			for b := 0; b < 4; b++ {
				if c&(1<<b) != 0 {
					if i >= len(d) {
						return nil, errors.New("short delta")
					}
					off |= int(d[i]) << (8 * b)
					i++
				}
			}
			for b := 0; b < 3; b++ {
				if c&(0x10<<b) != 0 {
					if i >= len(d) {
						return nil, errors.New("short delta")
					}
					size |= int(d[i]) << (8 * b)
					i++
				}
			}
			if size == 0 {
				size = 0x10000
			}
			if off+size > len(base) {
				return nil, errors.New("delta copy out of range")
			}
			out = append(out, base[off:off+size]...)
		} else if c != 0 {
			if i+int(c) > len(d) {
				return nil, errors.New("short delta")
			}
			out = append(out, d[i:i+int(c)]...)
			i += int(c)
		} else {
			return nil, errors.New("bad delta instruction")
		}
	}
	if len(out) != rs {
		return nil, errors.New("delta result size mismatch")
	}
	return out, nil
}

// splitTag returns the headers of a tag object, the signed part and the
// armored OpenPGP signature.
func splitTag(data []byte) (headers map[string]string, signed, sig []byte, err error) {
	i := bytes.Index(data, []byte("-----BEGIN PGP SIGNATURE-----"))
	if i < 0 {
		if bytes.Contains(data, []byte("-----BEGIN SSH SIGNATURE-----")) {
			return nil, nil, nil, errors.New("the tag has an SSH signature (not supported)")
		}
		return nil, nil, nil, errors.New("the tag has no signature")
	}
	signed, sig = data[:i], data[i:]
	headers = map[string]string{}
	head, _, _ := bytes.Cut(data, []byte("\n\n"))
	for _, l := range strings.Split(string(head), "\n") {
		if k, v, ok := strings.Cut(l, " "); ok {
			headers[k] = v
		}
	}
	return headers, signed, sig, nil
}

// GitFile is a file of a Git tree.
type GitFile struct {
	Mode string
	ID   string
}

// gitTreeFiles returns the files of a tree (with the paths) and the
// submodules (gitlinks: path to commit ID).
func gitTreeFiles(objs map[string]gitObject, treeID string) (map[string]GitFile, map[string]string, error) {
	files := map[string]GitFile{}
	links := map[string]string{}
	var walk func(id, prefix string) error
	walk = func(id, prefix string) error {
		t, ok := objs[id]
		if !ok || t.typ != "tree" {
			return fmt.Errorf("tree %s is not in the pack file", id)
		}
		d := t.data
		for len(d) > 0 {
			sp := bytes.IndexByte(d, ' ')
			nul := bytes.IndexByte(d, 0)
			if sp < 0 || nul < sp || nul+21 > len(d) {
				return errors.New("bad tree object")
			}
			mode, name := string(d[:sp]), string(d[sp+1:nul])
			oid := hex.EncodeToString(d[nul+1 : nul+21])
			d = d[nul+21:]
			p := path.Join(prefix, name)
			switch mode {
			case "40000":
				if err := walk(oid, p); err != nil {
					return err
				}
			case "160000":
				links[p] = oid
			default:
				files[p] = GitFile{Mode: mode, ID: oid}
			}
		}
		return nil
	}
	return files, links, walk(treeID, "")
}

// tarballFiles extracts a tarball into dir and returns the Git blob IDs of
// its files (without the top directory) and the top directory.
func tarballFiles(tarball, dir string) (map[string]GitFile, string, error) {
	run(Cmd{Args: []string{"tar", "-xf", tarball, "-C", dir, "--no-same-owner", "--warning=no-timestamp"}})
	root := dir
	if ents, err := os.ReadDir(dir); err == nil && len(ents) == 1 && ents[0].IsDir() {
		root = filepath.Join(dir, ents[0].Name())
	}
	files := map[string]GitFile{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		relp, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		var data []byte
		mode := "100644"
		switch {
		case info.Mode()&fs.ModeSymlink != 0:
			t, err := os.Readlink(p)
			if err != nil {
				return err
			}
			data, mode = []byte(t), "120000"
		case info.Mode().IsRegular():
			if data, err = os.ReadFile(p); err != nil {
				return err
			}
			if info.Mode()&0o111 != 0 {
				mode = "100755"
			}
		default:
			return nil
		}
		files[filepath.ToSlash(relp)] = GitFile{Mode: mode, ID: gitObjectID("blob", data)}
		return nil
	})
	return files, root, err
}

// addSubtree adds the files of another Git repository to tree. st is
// "path=repository". The commit is the submodule (gitlink) at path in the
// signed tree, or the revision of the Meson wrap file path.wrap: the
// comparison checks that the wrap file is the one of the signed tree.
func addSubtree(ctx context.Context, st string, tree map[string]GitFile, links map[string]string,
	tar map[string]GitFile, root string) error {
	dir, repo, ok := strings.Cut(st, "=")
	if !ok {
		return errors.New(`want "path=repository"`)
	}
	commit, ok := links[dir]
	if !ok {
		wrap := path.Join(path.Dir(dir), path.Base(dir)+".wrap")
		if _, inTree := tree[wrap]; !inTree {
			return fmt.Errorf("no submodule %s and no %s in the signed tree", dir, wrap)
		}
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(wrap)))
		if err != nil {
			return err
		}
		for _, l := range strings.Split(string(b), "\n") {
			if v, ok := strings.CutPrefix(strings.TrimSpace(l), "revision="); ok {
				commit = strings.TrimSpace(v)
			}
		}
		if len(commit) != 40 {
			return fmt.Errorf("%s has no commit ID (revision=)", wrap)
		}
	}
	objs, err := gitFetch(ctx, repo, commit)
	if err != nil {
		return err
	}
	c, ok := objs[commit]
	if !ok || c.typ != "commit" {
		return fmt.Errorf("commit %s is not in the pack file", commit)
	}
	first, _, _ := strings.Cut(string(c.data), "\n")
	treeID, ok := strings.CutPrefix(first, "tree ")
	if !ok {
		return errors.New("bad commit object")
	}
	sub, _, err := gitTreeFiles(objs, treeID)
	if err != nil {
		return err
	}
	for p, f := range sub {
		tree[path.Join(dir, p)] = f
	}
	delete(links, dir)
	logf("subtree %s: commit %.12s of %s (%d files)", dir, commit, repo, len(sub))
	return nil
}

func matchAny(globs []string, p string) bool {
	for _, g := range globs {
		if ok, _ := path.Match(g, p); ok {
			return true
		}
		// "dir/*" also matches the files in subdirectories of dir.
		if strings.HasSuffix(g, "/*") && strings.HasPrefix(p, strings.TrimSuffix(g, "*")) {
			return true
		}
	}
	return false
}

// compareTree compares the files of a tarball with a Git tree. extra are
// the globs of the files that the tarball can add or change; missing are
// the globs of the files of the tree that the tarball does not have.
func compareTree(tree, tar map[string]GitFile, extra, missing []string) (added []string, err error) {
	var problems []string
	for p, g := range tree {
		t, ok := tar[p]
		switch {
		case !ok:
			if !matchAny(missing, p) && !matchAny(extra, p) {
				problems = append(problems, "not in the tarball: "+p)
			}
		case t.ID != g.ID || (g.Mode == "120000") != (t.Mode == "120000"):
			if !matchAny(extra, p) {
				problems = append(problems, "different from Git: "+p)
			}
		}
	}
	for p := range tar {
		if _, ok := tree[p]; ok {
			continue
		}
		if matchAny(extra, p) {
			added = append(added, p)
			continue
		}
		problems = append(problems, "not in Git: "+p)
	}
	sort.Strings(problems)
	sort.Strings(added)
	if len(problems) > 0 {
		if len(problems) > 20 {
			problems = append(problems[:20], fmt.Sprintf("... and %d more", len(problems)-20))
		}
		return added, errors.New("the tarball is not the same as the signed tree:\n  " + strings.Join(problems, "\n  "))
	}
	return added, nil
}

// verifyGitTag checks a source tarball with a signed Git tag.
func verifyGitTag(s Source, dataPath string) error {
	if s.Git == "" || s.Tag == "" {
		return errors.New("signature-type git-tag needs git and tag")
	}
	ring, err := readKeyring(s.Key)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*upstreamTimeout)
	defer cancel()
	tagID, err := gitTagObjectID(ctx, s.Git, s.Tag)
	if err != nil {
		return err
	}
	objs, err := gitFetch(ctx, s.Git, tagID)
	if err != nil {
		return err
	}
	tag, ok := objs[tagID]
	if !ok || tag.typ != "tag" {
		return fmt.Errorf("the tag object %s is not in the pack file", tagID)
	}
	h, signed, sig, err := splitTag(tag.data)
	if err != nil {
		return err
	}
	if h["tag"] != s.Tag || h["type"] != "commit" {
		return fmt.Errorf("the tag object is for %q (%s), not for %q", h["tag"], h["type"], s.Tag)
	}
	signer, err := checkDetachedBytes(ring, sig, signed, s.AllowExpiredKey)
	if err != nil {
		return fmt.Errorf("tag %s: %w", s.Tag, err)
	}
	commit, ok := objs[h["object"]]
	if !ok || commit.typ != "commit" {
		return fmt.Errorf("the commit %s is not in the pack file", h["object"])
	}
	first, _, _ := strings.Cut(string(commit.data), "\n")
	treeID, ok := strings.CutPrefix(first, "tree ")
	if !ok {
		return errors.New("bad commit object")
	}
	tree, links, err := gitTreeFiles(objs, treeID)
	if err != nil {
		return err
	}
	tmp, err := os.MkdirTemp("", "kbuild-gittag")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmp) }() // temporary files only
	tar, root, err := tarballFiles(dataPath, tmp)
	if err != nil {
		return err
	}
	for _, st := range s.Subtrees {
		if err := addSubtree(ctx, st, tree, links, tar, root); err != nil {
			return fmt.Errorf("subtree %s: %w", st, err)
		}
	}
	added, err := compareTree(tree, tar, s.ExtraFiles, s.MissingFiles)
	if err != nil {
		return err
	}
	logf("Git tag %s (commit %.12s) signed by %s; the tarball has the %d files of the signed tree and %d files of extra-files",
		s.Tag, h["object"], signerName(signer), len(tree), len(added))
	return nil
}

// checkDetachedBytes checks an armored detached signature of data (first
// at the current time, then at the time of the signature).
func checkDetachedBytes(ring openpgp.EntityList, armored, data []byte, allowExpired bool) (*openpgp.Entity, error) {
	dir, err := os.MkdirTemp("", "kbuild-sig")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(dir) }() // temporary files only
	sigPath, dataPath := filepath.Join(dir, "sig"), filepath.Join(dir, "data")
	if err := os.WriteFile(sigPath, armored, 0o600); err != nil {
		return nil, err
	}
	if err := os.WriteFile(dataPath, data, 0o600); err != nil {
		return nil, err
	}
	return verifyDetachedSigner(ring, sigPath, dataPath, allowExpired)
}
