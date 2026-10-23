// SPDX-License-Identifier: BSD-2-Clause

// Command klos is the KLOS system tool. It is one static binary:
//
//	/init (in the initramfs)  unlock the disk and start the OS
//	klos boot-setup           early boot set-up (dinit runs it)
//	klos status               show the OS versions and boot entries
//	klos update <bundle>      install a new OS version
//	klos bless                mark the running version as good
//	klos rollback             start the previous version on the next boot
//	klos install              install KLOS on a disk (installer only)
//	klos devd                 load modules for devices (desktop flavor)
//	klos run-env              start a program with arguments from env files
//
// Read docs/updates.md and docs/boot.md.
package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func usage() {
	fmt.Fprint(os.Stderr, `usage: klos <command> [arguments]

Commands:
  status                 Show the OS versions and the UEFI boot entries.
  update [-reboot] <bundle>
                         Install a new OS version from a bundle (a .tar file,
                         a directory or an https:// URL). The next boot
                         is a trial boot of the new version.
  bless                  Mark the running version as good (dinit does this).
  rollback               Make the previous version the default again.
  install [-config FILE] Install KLOS on a disk (only on the installer).
  boot-setup             Early boot set-up (dinit runs this).
`)
	os.Exit(2)
}

func main() {
	if filepath.Base(os.Args[0]) == "init" || os.Getpid() == 1 {
		initMain()
		return
	}
	if len(os.Args) < 2 {
		usage()
	}
	args := os.Args[2:]
	var err error
	switch os.Args[1] {
	case "status":
		err = cmdStatus(args)
	case "update":
		err = cmdUpdate(args)
	case "bless":
		err = cmdBless(args)
	case "rollback":
		err = cmdRollback(args)
	case "install":
		err = cmdInstall(args)
	case "boot-setup":
		err = cmdBootSetup(args)
	case "install-service":
		err = cmdInstallService(args)
	case "run-env":
		err = cmdRunEnv(args)
	case "devd":
		err = cmdDevd(args)
	case "-h", "--help", "help":
		usage()
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "klos: %v\n", err)
		os.Exit(1)
	}
}
