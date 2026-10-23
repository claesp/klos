// SPDX-License-Identifier: BSD-2-Clause

package main

// OS versions. Each version is a logical volume "os-<version>" (the erofs
// image and the dm-verity hash tree) and a file \EFI\Linux\klos-<version>.efi
// on the EFI system partition, with the UEFI boot entry "KLOS <version>".
// Read docs/updates.md.

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
)

func runningVersion() string { return kernelCmdline()["klos.version"] }

func runningFlavor() string { return kernelCmdline()["klos.flavor"] }

// osVersions returns the versions that have an OS volume, newest first.
func osVersions() ([]string, error) {
	out, err := output("lvm", "lvs", "--config", lvmConfig, "--noheadings",
		"-o", "lv_name", vgName)
	if err != nil {
		return nil, err
	}
	var vs []string
	for _, f := range strings.Fields(out) {
		if strings.HasPrefix(f, osLVPrefix) {
			vs = append(vs, strings.TrimPrefix(f, osLVPrefix))
		}
	}
	sort.Slice(vs, func(i, j int) bool { return compareVersions(vs[i], vs[j]) > 0 })
	return vs, nil
}

// mountESP mounts the EFI system partition on /run/klos/esp.
func mountESP() (*Partition, func(), error) {
	p, err := findPartition(espLabel)
	if err != nil {
		return nil, nil, err
	}
	if isMounted(espMount) {
		return p, func() {}, nil
	}
	if err := mount(p.Dev, espMount, "vfat",
		syscall.MS_NOSUID|syscall.MS_NODEV|syscall.MS_NOEXEC, "umask=0077,shortname=winnt"); err != nil {
		return nil, nil, err
	}
	return p, func() { syscall.Sync(); syscall.Unmount(espMount, 0) }, nil
}

func espPath(win string) string {
	return filepath.Join(espMount, strings.ReplaceAll(strings.TrimPrefix(win, `\`), `\`, "/"))
}

// copyAtomic copies a file to dst through a temporary file and a rename.
func copyAtomic(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	tmp := dst + ".new"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		return err
	}
	out.Close()
	return os.Rename(tmp, dst)
}

func sameFile(a, b string) bool {
	x, err1 := os.ReadFile(a)
	y, err2 := os.ReadFile(b)
	return err1 == nil && err2 == nil && bytes.Equal(x, y)
}

// writeImage copies an OS image to a block device and syncs it.
func writeImage(img, dev string) error {
	in, err := os.Open(img)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dev, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	if _, err := io.CopyBuffer(out, in, make([]byte, 4<<20)); err != nil {
		out.Close()
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// installVersion writes the OS volume and the EFI file of a bundle.
// The caller must mount the ESP first.
func installVersion(dir string, info *BundleInfo, esp *Partition) error {
	lv := osLVPrefix + info.Version
	size := (info.SlotSize + (4 << 20) - 1) / (4 << 20) * (4 << 20)
	fmt.Printf("make the OS volume %s/%s (%d MiB)\n", vgName, lv, size>>20)
	if err := lvm("lvcreate", "--yes", "-L", strconv.FormatInt(size, 10)+"b", "-n", lv, vgName); err != nil {
		return err
	}
	fmt.Println("write the OS image")
	if err := writeImage(filepath.Join(dir, "root.img"), lvPath(lv)); err != nil {
		return err
	}
	fmt.Println("verify the OS image on the disk")
	if err := run("veritysetup", "verify", lvPath(lv), lvPath(lv), info.RootHash,
		"--hash-offset="+strconv.FormatInt(info.HashOffset, 10)); err != nil {
		return err
	}
	if err := copyAtomic(filepath.Join(dir, "klos.efi"), espPath(espFile(info.Version))); err != nil {
		return err
	}
	syscall.Sync()
	return nil
}

func cmdUpdate(args []string) error {
	fs := flag.NewFlagSet("update", flag.ExitOnError)
	reboot := fs.Bool("reboot", false, "restart after the update")
	downgrade := fs.Bool("allow-downgrade", false, "accept an older version")
	fs.Parse(args)
	if fs.NArg() != 1 {
		return errors.New("usage: klos update [-reboot] <bundle>")
	}
	if err := requireRoot(); err != nil {
		return err
	}
	if isLive() {
		return errors.New("the installer cannot update; use 'klos install'")
	}
	work := filepath.Join("/var/tmp", "klos-update")
	defer os.RemoveAll(work)
	dir, err := fetchBundle(fs.Arg(0), work)
	if err != nil {
		return err
	}
	info, err := verifyBundle(dir, bundlePub)
	if err != nil {
		return err
	}
	fmt.Printf("bundle: %s %s %s (signature good)\n", info.Distribution, info.Flavor, info.Version)
	if info.Flavor != runningFlavor() {
		return fmt.Errorf("the bundle is for the flavor %q, this system is %q", info.Flavor, runningFlavor())
	}
	cur := runningVersion()
	if c := compareVersions(info.Version, cur); c == 0 {
		return fmt.Errorf("version %s is running now", cur)
	} else if c < 0 && !*downgrade {
		return fmt.Errorf("version %s is older than %s (use -allow-downgrade)", info.Version, cur)
	}
	vs, err := osVersions()
	if err != nil {
		return err
	}
	for _, v := range vs {
		if v == info.Version {
			return fmt.Errorf("version %s is installed; use 'klos rollback' or remove it", v)
		}
	}
	// Remove the versions that are not the running version or the default.
	if err := collectVersions(cur, 1); err != nil {
		return err
	}
	esp, umount, err := mountESP()
	if err != nil {
		return err
	}
	defer umount()
	if err := installVersion(dir, info, esp); err != nil {
		lvm("lvremove", "--yes", vgName+"/"+osLVPrefix+info.Version)
		return err
	}
	if efiAvailable() {
		n, err := createBootEntry(entryDesc(info.Version), esp, espFile(info.Version))
		if err != nil {
			return err
		}
		order, _ := bootOrder()
		if err := setBootOrder(append(order, n)); err != nil {
			return err
		}
		if err := setBootNext(n); err != nil {
			return err
		}
		fmt.Printf("the next boot is a trial boot of %s (Boot%04X)\n", info.Version, n)
	} else {
		fmt.Println("no UEFI variables: the fallback boot file now starts the new version")
		if err := copyAtomic(espPath(espFile(info.Version)), espPath(`\EFI\BOOT\BOOTX64.EFI`)); err != nil {
			return err
		}
	}
	if *reboot {
		syscall.Sync()
		return run("reboot")
	}
	fmt.Println("restart the computer to start the new version")
	return nil
}

// collectVersions removes old OS versions. It keeps the running version,
// the default version (first in BootOrder) and the newest other versions,
// up to "keep" versions in total.
func collectVersions(running string, keep int) error {
	vs, err := osVersions()
	if err != nil {
		return err
	}
	keepSet := map[string]bool{running: true}
	if efiAvailable() {
		if order, err := bootOrder(); err == nil && len(order) > 0 {
			ents, _ := bootEntries()
			for _, e := range ents {
				if e.Num == order[0] && strings.HasPrefix(e.Description, "KLOS ") {
					keepSet[strings.TrimPrefix(e.Description, "KLOS ")] = true
				}
			}
		}
	}
	for _, v := range vs {
		if len(keepSet) >= keep {
			break
		}
		keepSet[v] = true
	}
	var remove []string
	for _, v := range vs {
		if !keepSet[v] {
			remove = append(remove, v)
		}
	}
	if len(remove) == 0 {
		return nil
	}
	_, umount, err := mountESP()
	if err != nil {
		return err
	}
	defer umount()
	for _, v := range remove {
		fmt.Printf("remove the old version %s\n", v)
		if e := findBootEntry(v); e != nil {
			if err := removeBootEntry(e.Num); err != nil {
				return err
			}
		}
		os.Remove(espPath(espFile(v)))
		if err := lvm("lvremove", "--yes", vgName+"/"+osLVPrefix+v); err != nil {
			return err
		}
	}
	return nil
}

// disarmWatchdog stops the trial boot watchdog (the magic close).
func disarmWatchdog() error {
	if _, err := os.Stat(runDir + "/trial"); err != nil {
		return nil
	}
	f, err := os.OpenFile("/dev/watchdog", os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	f.Write([]byte("V"))
	if err := f.Close(); err != nil {
		return err
	}
	return os.Remove(runDir + "/trial")
}

func cmdBless(args []string) error {
	if err := requireRoot(); err != nil {
		return err
	}
	if isLive() {
		return nil
	}
	v := runningVersion()
	if err := disarmWatchdog(); err != nil {
		return fmt.Errorf("watchdog: %w", err)
	}
	_, umount, err := mountESP()
	if err != nil {
		return err
	}
	defer umount()
	if efiAvailable() {
		e := findBootEntry(v)
		if e == nil {
			// For example, the first boot after the installer had no UEFI variables.
			esp, perr := findPartition(espLabel)
			if perr != nil {
				return perr
			}
			n, err := createBootEntry(entryDesc(v), esp, espFile(v))
			if err != nil {
				return err
			}
			e = &BootEntry{Num: n}
		}
		if order, err := bootOrder(); err != nil || len(order) == 0 || order[0] != e.Num {
			fmt.Printf("make %s the default version\n", v)
			if err := moveToFront(e.Num); err != nil {
				return err
			}
		}
	}
	// The fallback file starts the newest good version if the UEFI
	// variables are lost (for example after a firmware reset).
	src, fallback := espPath(espFile(v)), espPath(`\EFI\BOOT\BOOTX64.EFI`)
	if !sameFile(src, fallback) {
		if err := copyAtomic(src, fallback); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(stateDir, "good"), []byte(v+"\n"), 0o600); err != nil {
		return err
	}
	return collectVersions(v, keepVersion)
}

func cmdRollback(args []string) error {
	if err := requireRoot(); err != nil {
		return err
	}
	if !efiAvailable() {
		return errors.New("rollback needs UEFI variables")
	}
	cur := runningVersion()
	vs, err := osVersions()
	if err != nil {
		return err
	}
	for _, v := range vs {
		if v == cur {
			continue
		}
		e := findBootEntry(v)
		if e == nil {
			continue
		}
		if err := moveToFront(e.Num); err != nil {
			return err
		}
		fmt.Printf("version %s is the default; restart the computer to start it\n", v)
		return nil
	}
	return errors.New("there is no other version")
}

func cmdStatus(args []string) error {
	c := kernelCmdline()
	fmt.Printf("running: KLOS %s, flavor %s, mode %s\n", c["klos.version"], c["klos.flavor"],
		map[bool]string{true: "live", false: "installed"}[isLive()])
	if _, err := os.Stat(runDir + "/trial"); err == nil {
		fmt.Println("this is a trial boot (not blessed)")
	}
	if !isLive() {
		if vs, err := osVersions(); err == nil {
			fmt.Printf("installed versions: %s\n", strings.Join(vs, " "))
		}
	}
	if !efiAvailable() {
		fmt.Println("no UEFI variables")
		return nil
	}
	order, _ := bootOrder()
	var ids []string
	for _, n := range order {
		ids = append(ids, fmt.Sprintf("%04X", n))
	}
	fmt.Printf("BootOrder: %s\n", strings.Join(ids, ","))
	if cur, err := bootCurrent(); err == nil {
		fmt.Printf("BootCurrent: %04X\n", cur)
	}
	ents, _ := bootEntries()
	for _, e := range ents {
		fmt.Printf("  Boot%04X %-24s %s\n", e.Num, e.Description, e.File)
	}
	return nil
}
