// SPDX-License-Identifier: BSD-2-Clause

package main

// UEFI Secure Boot (read docs/secure-boot.md). The image has the signed
// variable updates /usr/share/klos/secureboot/{db,KEK,PK}.auth. When the
// firmware is in Setup Mode (no platform key), "klos secureboot enroll"
// writes db, KEK and then PK. The firmware then leaves Setup Mode, and from
// the next boot it starts only EFI binaries that the KLOS db key signed.

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

const (
	sbFiles          = "/usr/share/klos/secureboot"
	imageSecDBGUID   = "d719b2cb-3d3a-4596-a3bc-dad00e67656f"
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
		return errors.New("usage: klos secureboot status|enroll")
	}
	switch args[0] {
	case "status":
		fmt.Printf("Secure Boot: %s\n", secureBootState())
		return nil
	case "enroll":
		return secureBootEnroll(args[1:])
	}
	return errors.New("usage: klos secureboot status|enroll")
}

func secureBootEnroll(args []string) error {
	fs := flag.NewFlagSet("secureboot enroll", flag.ExitOnError)
	dir := fs.String("dir", sbFiles, "directory with db.auth, KEK.auth and PK.auth")
	fs.Parse(args)
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
		data, err := os.ReadFile(filepath.Join(*dir, v.name+".auth"))
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
