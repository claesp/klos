// SPDX-License-Identifier: BSD-2-Clause

package main

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"encoding/hex"
	"strings"
	"testing"
)

// packObject writes the header and the compressed data of a pack entry.
func packObject(buf *bytes.Buffer, typ int, data []byte, prefix []byte) {
	size := len(data)
	c := byte(typ<<4) | byte(size&15)
	size >>= 4
	for size > 0 {
		buf.WriteByte(c | 0x80)
		c = byte(size & 0x7f)
		size >>= 7
	}
	buf.WriteByte(c)
	buf.Write(prefix)
	zw := zlib.NewWriter(buf)
	_, _ = zw.Write(data) // a bytes.Buffer does not fail
	_ = zw.Close()
}

func TestParsePackDelta(t *testing.T) {
	base := []byte("hello, world\n")
	// Delta: base size 13, result size 19, copy 13 bytes at 0, insert "again\n".
	delta := []byte{13, 19, 0x90, 13, 6}
	delta = append(delta, "again\n"...)
	var buf bytes.Buffer
	buf.WriteString("PACK")
	_ = binary.Write(&buf, binary.BigEndian, uint32(2))
	_ = binary.Write(&buf, binary.BigEndian, uint32(2))
	off0 := buf.Len()
	packObject(&buf, 3, base, nil)
	off1 := buf.Len()
	packObject(&buf, 6, delta, []byte{byte(off1 - off0)})
	objs, err := parsePack(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	want := "hello, world\nagain\n"
	if o, ok := objs[gitObjectID("blob", []byte(want))]; !ok || string(o.data) != want {
		t.Fatalf("delta object not found: %v", objs)
	}
	if _, ok := objs[gitObjectID("blob", base)]; !ok {
		t.Fatal("base object not found")
	}
}

func TestGitObjectID(t *testing.T) {
	// git hash-object of an empty file
	if got := gitObjectID("blob", nil); got != "e69de29bb2d1d6434b8b29ae775ad8c2e48c5391" {
		t.Fatalf("empty blob ID %s", got)
	}
}

func TestTreeAndCompare(t *testing.T) {
	a := gitObjectID("blob", []byte("a\n"))
	sub := treeEntry("100644", "b.c", gitObjectID("blob", []byte("b\n")))
	subID := gitObjectID("tree", sub)
	root := append(treeEntry("100644", "a.txt", a), treeEntry("40000", "src", subID)...)
	root = append(root, treeEntry("160000", "ext", strings.Repeat("ab", 20))...)
	rootID := gitObjectID("tree", root)
	objs := map[string]gitObject{rootID: {"tree", root}, subID: {"tree", sub}}
	files, links, err := gitTreeFiles(objs, rootID)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || files["src/b.c"].ID == "" || links["ext"] == "" {
		t.Fatalf("files %v links %v", files, links)
	}
	tar := map[string]GitFile{"a.txt": files["a.txt"], "src/b.c": files["src/b.c"]}
	if _, err := compareTree(files, tar, nil, nil); err != nil {
		t.Fatalf("same files: %v", err)
	}
	tar["configure"] = GitFile{Mode: "100755", ID: "x"}
	if _, err := compareTree(files, tar, nil, nil); err == nil {
		t.Fatal("extra file: no error")
	}
	if added, err := compareTree(files, tar, []string{"configure"}, nil); err != nil || len(added) != 1 {
		t.Fatalf("extra-files: %v %v", added, err)
	}
	tar["src/b.c"] = GitFile{Mode: "100644", ID: "changed"}
	if _, err := compareTree(files, tar, []string{"configure"}, nil); err == nil {
		t.Fatal("changed file: no error")
	}
	delete(tar, "src/b.c")
	if _, err := compareTree(files, tar, []string{"configure"}, []string{"src/*"}); err != nil {
		t.Fatalf("missing-files: %v", err)
	}
}

func treeEntry(mode, name, id string) []byte {
	raw, _ := hex.DecodeString(id)
	return append([]byte(mode+" "+name+"\x00"), raw...)
}

func TestSplitTag(t *testing.T) {
	tag := "object 1111111111111111111111111111111111111111\ntype commit\ntag v1.0\ntagger A <a@example.org> 1 +0000\n\nRelease 1.0\n-----BEGIN PGP SIGNATURE-----\nabc\n-----END PGP SIGNATURE-----\n"
	h, signed, sig, err := splitTag([]byte(tag))
	if err != nil {
		t.Fatal(err)
	}
	if h["tag"] != "v1.0" || h["type"] != "commit" || !strings.HasSuffix(string(signed), "Release 1.0\n") ||
		!strings.HasPrefix(string(sig), "-----BEGIN PGP SIGNATURE-----") {
		t.Fatalf("splitTag: %v %q %q", h, signed, sig)
	}
	if _, _, _, err := splitTag([]byte("object x\ntype commit\ntag v1\n\nno signature\n")); err == nil {
		t.Fatal("unsigned tag: no error")
	}
}
