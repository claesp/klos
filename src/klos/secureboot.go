// SPDX-License-Identifier: BSD-2-Clause

package main

// UEFI Secure Boot (read docs/secure-boot.md). The image has the signed
// variable updates /usr/share/klos/secureboot/{db,KEK,PK}.auth. When the
// firmware is in Setup Mode (no platform key), "klos secureboot enroll"
// writes db, KEK and then PK. The firmware then leaves Setup Mode, and from
// the next boot it starts only EFI binaries that the KLOS db key signed.
//
// With -microsoft, KEK and db come from the microsoft/ directory. These
// updates also contain the Microsoft KEK and db certificates, so the
// firmware also starts option ROMs that Microsoft signed.

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	sbFiles          = "/usr/share/klos/secureboot"
	imageSecDBGUID   = "d719b2cb-3d3a-4596-a3bc-dad00e67656f"
	microsoftOwner   = "77fa9abd-0359-4d32-bd60-28f4e78f784b"
	sbAuthAttributes = 0x27 // NV, BS, RT, time-based authenticated write
)

// efiFlag reads a one-byte variable of the global GUID (-1: not present).
func efiFlag(name string) int {
	b, err := efiRead(name)
	if err != nil || len(b) < 1 {
		return -1
	}
	return int(b[0])
}

func secureBootState() string {
	if _, err := os.Stat(efivarsDir); err != nil {
		return "not available (no UEFI variables)"
	}
	sb, setup := efiFlag("SecureBoot"), efiFlag("SetupMode")
	switch {
	case sb < 0 && setup < 0:
		return "not supported by the firmware"
	case sb == 1:
		return "enabled"
	case setup == 1:
		return "disabled (Setup Mode: no platform key)"
	default:
		return "disabled (keys are enrolled; Secure Boot is off in the firmware setup)"
	}
}

func cmdSecureBoot(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: klos secureboot status|enroll|apply")
	}
	switch args[0] {
	case "status":
		fmt.Printf("Secure Boot: %s\n", secureBootState())
		if b, err := os.ReadFile(filepath.Join(efivarsDir, "db-"+imageSecDBGUID)); err == nil && len(b) > 4 {
			n, ms := countSignatures(b[4:], microsoftOwner)
			fmt.Printf("db entries: %d (Microsoft: %d)\n", n, ms)
		}
		return nil
	case "enroll":
		return secureBootEnroll(args[1:])
	case "apply":
		return secureBootApply(args[1:])
	}
	return errors.New("usage: klos secureboot status|enroll|apply")
}

func secureBootEnroll(args []string) error {
	fs := flag.NewFlagSet("secureboot enroll", flag.ContinueOnError)
	dir := fs.String("dir", sbFiles, "directory with db.auth, KEK.auth and PK.auth")
	microsoft := fs.Bool("microsoft", false, "also enroll the Microsoft KEK and db certificates (for option ROMs)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := requireRoot(); err != nil {
		return err
	}
	if efiFlag("SetupMode") != 1 {
		return errors.New("the firmware is not in Setup Mode; clear the Secure Boot keys in the firmware setup first")
	}
	// PK comes last: the firmware leaves Setup Mode when PK is set.
	for _, v := range []struct{ name, guid string }{
		{"db", imageSecDBGUID}, {"KEK", efiGlobalGUID}, {"PK", efiGlobalGUID},
	} {
		d := *dir
		if *microsoft && v.name != "PK" {
			d = filepath.Join(d, "microsoft")
		}
		data, err := os.ReadFile(filepath.Join(d, v.name+".auth"))
		if err != nil {
			return err
		}
		if err := efiWriteVar(v.name, v.guid, sbAuthAttributes, data); err != nil {
			return fmt.Errorf("enroll %s: %w", v.name, err)
		}
		fmt.Printf("enrolled %s\n", v.name)
	}
	if efiFlag("SetupMode") != 0 {
		return errors.New("the firmware is still in Setup Mode after the enrollment")
	}
	fmt.Println("Secure Boot keys are enrolled. Secure Boot is active from the next boot.")
	return nil
}

// secureBootApply writes a signed variable update (.auth file) to db, dbx,
// KEK or PK. For example, a dbx update from "./klos revoke" revokes old
// EFI binaries. The firmware checks the signature of the update.
func secureBootApply(args []string) error {
	fs := flag.NewFlagSet("secureboot apply", flag.ContinueOnError)
	appendWrite := fs.Bool("append", false, "add to the variable (for db and dbx updates)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 2 {
		return errors.New("usage: klos secureboot apply [-append] db|dbx|KEK|PK FILE")
	}
	if err := requireRoot(); err != nil {
		return err
	}
	name, file := fs.Arg(0), fs.Arg(1)
	guid := efiGlobalGUID
	switch name {
	case "db", "dbx":
		guid = imageSecDBGUID
	case "KEK", "PK":
	default:
		return fmt.Errorf("unknown variable %q", name)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	attrs := uint32(sbAuthAttributes)
	if *appendWrite {
		attrs |= 0x40 // EFI_VARIABLE_APPEND_WRITE
	}
	if err := efiWriteVar(name, guid, attrs, data); err != nil {
		return fmt.Errorf("the firmware refused the update of %s: %w", name, err)
	}
	fmt.Printf("updated %s; the change is active from the next boot\n", name)
	return nil
}

// countSignatures returns the number of entries in a list of
// EFI_SIGNATURE_LIST structures, and the number with the owner GUID.
func countSignatures(b []byte, owner string) (int, int) {
	want := guidBytes(owner)
	n, match := 0, 0
	for len(b) >= 28 {
		listSize := binary.LittleEndian.Uint32(b[16:])
		hdrSize := binary.LittleEndian.Uint32(b[20:])
		sigSize := binary.LittleEndian.Uint32(b[24:])
		if listSize < 28+hdrSize || int(listSize) > len(b) || sigSize < 16 {
			break
		}
		for p := 28 + hdrSize; p+sigSize <= listSize; p += sigSize {
			n++
			if bytes.Equal(b[p:p+16], want) {
				match++
			}
		}
		b = b[listSize:]
	}
	return n, match
}

// guidBytes encodes a GUID in the EFI byte order: the first three fields
// are little-endian, the last two are in the text order.
func guidBytes(s string) []byte {
	h, err := hex.DecodeString(strings.ReplaceAll(s, "-", ""))
	if err != nil || len(h) != 16 {
		panic("bad GUID constant " + s)
	}
	return []byte{h[3], h[2], h[1], h[0], h[5], h[4], h[7], h[6],
		h[8], h[9], h[10], h[11], h[12], h[13], h[14], h[15]}
}
