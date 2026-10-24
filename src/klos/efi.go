// SPDX-License-Identifier: BSD-2-Clause

package main

// UEFI boot variables (UEFI specification, section 3.1). KLOS does not use a
// boot loader: each OS version is a kernel EFI stub with a load option.
// BootNext selects a trial boot and BootOrder holds the good versions.

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

const (
	efivarsDir    = "/sys/firmware/efi/efivars"
	efiGlobalGUID = "8be4df61-93ca-11d2-aa0d-00e098032b8c"
	attrNV        = 0x1
	attrBS        = 0x2
	attrRT        = 0x4
	attrDefault   = attrNV | attrBS | attrRT
	loadActive    = 0x1

	fsIocGetFlags = 0x80086601
	fsIocSetFlags = 0x40086602
	fsImmutableFl = 0x10
)

func efiAvailable() bool {
	_, err := os.Stat(filepath.Join(efivarsDir, "BootOrder-"+efiGlobalGUID))
	return err == nil
}

func efiPath(name string) string {
	return filepath.Join(efivarsDir, name+"-"+efiGlobalGUID)
}

func efiRead(name string) ([]byte, error) {
	b, err := os.ReadFile(efiPath(name))
	if err != nil {
		return nil, err
	}
	if len(b) < 4 {
		return nil, fmt.Errorf("short EFI variable %s", name)
	}
	return b[4:], nil
}

// clearImmutable removes the immutable flag that efivarfs sets on files.
func clearImmutable(path string) error {
	f, err := os.OpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	var flags int32
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), fsIocGetFlags,
		uintptr(unsafe.Pointer(&flags))); e != 0 {
		return e
	}
	flags &^= fsImmutableFl
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), fsIocSetFlags,
		uintptr(unsafe.Pointer(&flags))); e != 0 {
		return e
	}
	return nil
}

func efiWrite(name string, data []byte) error {
	return efiWriteVar(name, efiGlobalGUID, attrDefault, data)
}

// efiWriteVar writes a variable with a vendor GUID and attributes.
func efiWriteVar(name, guid string, attrs uint32, data []byte) error {
	p := filepath.Join(efivarsDir, name+"-"+guid)
	if _, err := os.Stat(p); err == nil {
		if err := clearImmutable(p); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	buf := make([]byte, 4+len(data))
	binary.LittleEndian.PutUint32(buf, attrs)
	copy(buf[4:], data)
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	// efivarfs needs the attributes and the data in one write.
	if _, err := f.Write(buf); err != nil {
		f.Close()
		return fmt.Errorf("%s: %w", name, err)
	}
	return f.Close()
}

func efiDelete(name string) error {
	p := efiPath(name)
	if err := clearImmutable(p); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	return os.Remove(p)
}

func u16List(b []byte) []uint16 {
	out := make([]uint16, len(b)/2)
	for i := range out {
		out[i] = binary.LittleEndian.Uint16(b[2*i:])
	}
	return out
}

func u16Bytes(l []uint16) []byte {
	b := make([]byte, 2*len(l))
	for i, v := range l {
		binary.LittleEndian.PutUint16(b[2*i:], v)
	}
	return b
}

func bootOrder() ([]uint16, error) {
	b, err := efiRead("BootOrder")
	if err != nil {
		return nil, err
	}
	return u16List(b), nil
}

func setBootOrder(order []uint16) error { return efiWrite("BootOrder", u16Bytes(order)) }

func setBootNext(n uint16) error { return efiWrite("BootNext", u16Bytes([]uint16{n})) }

func bootCurrent() (uint16, error) {
	b, err := efiRead("BootCurrent")
	if err != nil || len(b) < 2 {
		return 0, fmt.Errorf("BootCurrent: %v", err)
	}
	return binary.LittleEndian.Uint16(b), nil
}

// BootEntry is a parsed Boot#### variable.
type BootEntry struct {
	Num         uint16
	Description string
	File        string // the file path in the device path, if there is one
}

func bootName(n uint16) string { return fmt.Sprintf("Boot%04X", n) }

func ucs2(s string) []byte {
	u := utf16.Encode([]rune(s))
	b := make([]byte, 2*len(u)+2)
	for i, v := range u {
		binary.LittleEndian.PutUint16(b[2*i:], v)
	}
	return b
}

func fromUCS2(b []byte) (string, int) {
	var u []uint16
	i := 0
	for ; i+1 < len(b); i += 2 {
		v := binary.LittleEndian.Uint16(b[i:])
		if v == 0 {
			return string(utf16.Decode(u)), i + 2
		}
		u = append(u, v)
	}
	return string(utf16.Decode(u)), i
}

func parseLoadOption(n uint16, b []byte) (*BootEntry, error) {
	if len(b) < 6 {
		return nil, errors.New("short load option")
	}
	plen := int(binary.LittleEndian.Uint16(b[4:]))
	desc, used := fromUCS2(b[6:])
	e := &BootEntry{Num: n, Description: desc}
	dp := b[6+used:]
	if len(dp) < plen {
		return e, nil
	}
	dp = dp[:plen]
	for len(dp) >= 4 {
		typ, sub := dp[0], dp[1]
		l := int(binary.LittleEndian.Uint16(dp[2:]))
		if l < 4 || l > len(dp) {
			break
		}
		if typ == 0x04 && sub == 0x04 {
			e.File, _ = fromUCS2(dp[4:l])
		}
		if typ == 0x7f {
			break
		}
		dp = dp[l:]
	}
	return e, nil
}

// bootEntries returns all Boot#### entries.
func bootEntries() ([]*BootEntry, error) {
	ents, err := os.ReadDir(efivarsDir)
	if err != nil {
		return nil, err
	}
	var out []*BootEntry
	for _, d := range ents {
		name := d.Name()
		if len(name) != 8+1+len(efiGlobalGUID) || !strings.HasPrefix(name, "Boot") ||
			!strings.HasSuffix(name, efiGlobalGUID) {
			continue
		}
		var n uint16
		if _, err := fmt.Sscanf(name[4:8], "%04X", &n); err != nil {
			continue
		}
		b, err := efiRead(name[:8])
		if err != nil {
			continue
		}
		if e, err := parseLoadOption(n, b); err == nil {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Num < out[j].Num })
	return out, nil
}

// makeLoadOption makes an EFI_LOAD_OPTION for a file on a GPT partition.
func makeLoadOption(desc string, p *Partition, file string) []byte {
	var dp []byte
	// Hard drive media device path (type 4, subtype 1).
	hd := make([]byte, 42)
	hd[0], hd[1] = 0x04, 0x01
	binary.LittleEndian.PutUint16(hd[2:], 42)
	binary.LittleEndian.PutUint32(hd[4:], uint32(p.Number))
	binary.LittleEndian.PutUint64(hd[8:], p.FirstLBA)
	binary.LittleEndian.PutUint64(hd[16:], p.LastLBA-p.FirstLBA+1)
	copy(hd[24:40], p.PartGUID[:])
	hd[40] = 0x02 // GPT
	hd[41] = 0x02 // GUID signature
	dp = append(dp, hd...)
	// File path media device path (type 4, subtype 4).
	fp := ucs2(file)
	node := make([]byte, 4)
	node[0], node[1] = 0x04, 0x04
	binary.LittleEndian.PutUint16(node[2:], uint16(4+len(fp)))
	dp = append(dp, node...)
	dp = append(dp, fp...)
	// End of the device path.
	dp = append(dp, 0x7f, 0xff, 0x04, 0x00)

	b := make([]byte, 6)
	binary.LittleEndian.PutUint32(b, loadActive)
	binary.LittleEndian.PutUint16(b[4:], uint16(len(dp)))
	b = append(b, ucs2(desc)...)
	return append(b, dp...)
}

// createBootEntry adds a Boot#### variable and returns its number.
func createBootEntry(desc string, p *Partition, file string) (uint16, error) {
	used := map[uint16]bool{}
	ents, err := bootEntries()
	if err != nil {
		return 0, err
	}
	for _, e := range ents {
		used[e.Num] = true
	}
	for n := uint16(0); n < 0xffff; n++ {
		if !used[n] {
			return n, efiWrite(bootName(n), makeLoadOption(desc, p, file))
		}
	}
	return 0, errors.New("no free boot entry number")
}

// Each OS version has the boot entry description "KLOS <version>".
func entryDesc(version string) string { return "KLOS " + version }

func espFile(version string) string { return `\EFI\Linux\klos-` + version + ".efi" }

// findBootEntry returns the entry of an OS version, or nil.
func findBootEntry(version string) *BootEntry {
	ents, err := bootEntries()
	if err != nil {
		return nil
	}
	for _, e := range ents {
		if e.Description == entryDesc(version) {
			return e
		}
	}
	return nil
}

// moveToFront puts n first in BootOrder.
func moveToFront(n uint16) error {
	order, err := bootOrder()
	if err != nil {
		return err
	}
	out := []uint16{n}
	for _, v := range order {
		if v != n {
			out = append(out, v)
		}
	}
	return setBootOrder(out)
}

// removeBootEntry deletes an entry and removes it from BootOrder.
func removeBootEntry(n uint16) error {
	order, err := bootOrder()
	if err == nil {
		var out []uint16
		for _, v := range order {
			if v != n {
				out = append(out, v)
			}
		}
		if err := setBootOrder(out); err != nil {
			return err
		}
	}
	return efiDelete(bootName(n))
}
