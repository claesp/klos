// SPDX-License-Identifier: BSD-2-Clause

package main

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// fakePE makes a small PE32+ file with one section.
func fakePE() []byte {
	b := make([]byte, 0x400)
	b[0], b[1] = 'M', 'Z'
	binary.LittleEndian.PutUint32(b[0x3c:], 0x80)
	copy(b[0x80:], "PE\x00\x00")
	coff := 0x84
	binary.LittleEndian.PutUint16(b[coff:], 0x8664)
	binary.LittleEndian.PutUint16(b[coff+2:], 1)    // sections
	binary.LittleEndian.PutUint16(b[coff+16:], 240) // optional header size
	opt := coff + 20
	binary.LittleEndian.PutUint16(b[opt:], 0x20b)
	binary.LittleEndian.PutUint32(b[opt+60:], 0x200) // SizeOfHeaders
	binary.LittleEndian.PutUint32(b[opt+108:], 16)   // data directories
	sec := opt + 240
	copy(b[sec:], ".text")
	binary.LittleEndian.PutUint32(b[sec+16:], 0x200) // SizeOfRawData
	binary.LittleEndian.PutUint32(b[sec+20:], 0x200) // PointerToRawData
	for i := 0x200; i < 0x400; i++ {
		b[i] = byte(i)
	}
	return b
}

func TestSignPE(t *testing.T) {
	dir := t.TempDir()
	SECRETS = dir
	TOP = dir
	keys := ensureSecureBootKeys()
	path := filepath.Join(dir, "test.efi")
	if err := os.WriteFile(path, fakePE(), 0o644); err != nil {
		t.Fatal(err)
	}
	signPE(path, keys["db"])
	if err := verifyPE(path, keys["db"].cert); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if err := verifyPE(path, keys["KEK"].cert); err == nil {
		t.Fatal("a different certificate verified the signature")
	}
	b, _ := os.ReadFile(path)
	b[0x300] ^= 1
	os.WriteFile(path, b, 0o644)
	if err := verifyPE(path, keys["db"].cert); err == nil {
		t.Fatal("a changed file verified")
	}
}

func TestAuthVariableLayout(t *testing.T) {
	dir := t.TempDir()
	SECRETS = dir
	TOP = dir
	keys := ensureSecureBootKeys()
	out := filepath.Join(dir, "out")
	writeSecureBootFiles(keys, out)
	for _, n := range sbKeyNames {
		b, err := os.ReadFile(filepath.Join(out, n+".auth"))
		if err != nil {
			t.Fatal(err)
		}
		// EFI_TIME (16), then WIN_CERTIFICATE_UEFI_GUID.
		l := int(binary.LittleEndian.Uint32(b[16:]))
		if binary.LittleEndian.Uint16(b[22:]) != winCertEFIGUID || 16+l >= len(b) {
			t.Fatalf("%s.auth: bad header", n)
		}
		esl := b[16+l:]
		if string(esl[:16]) != string(guidBytes(guidCertX509)) {
			t.Fatalf("%s.auth: no X.509 signature list", n)
		}
	}
}
