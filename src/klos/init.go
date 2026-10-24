// SPDX-License-Identifier: BSD-2-Clause

package main

// The initramfs /init. The steps are in docs/boot.md:
//
//  1. Mount /proc, /sys, /dev and /run.
//  2. Unlock the LUKS2 partition "klos-luks" (the passphrase prompt is on
//     all consoles) and activate the LVM volume group "klos".
//  3. Open the OS volume os-<version> with dm-verity. The root hash is in
//     the kernel command line, which is part of the signed EFI binary.
//  4. Mount the erofs root read-only, the /var volume and the /etc overlay.
//  5. Arm the watchdog if this is a trial boot of a new version.
//  6. Switch to the new root and start dinit.
//
// In live mode (installer media), the OS image is a file on the media, and
// /var and the /etc upper layer are in memory.
//
// There is no rescue shell. If a step fails, init shows the error and
// restarts the computer after 30 seconds.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

const sysroot = "/sysroot"

var kmsg *os.File

func ilog(format string, a ...any) {
	msg := fmt.Sprintf(format, a...)
	if kmsg != nil {
		fmt.Fprintf(kmsg, "<6>klos-init: %s\n", msg)
	}
	consoleWrite("klos-init: " + msg + "\n")
}

func ifail(err error) {
	ilog("ERROR: %v", err)
	ilog("the computer restarts in 30 seconds")
	time.Sleep(30 * time.Second)
	syscall.Sync()
	syscall.Reboot(syscall.LINUX_REBOOT_CMD_RESTART)
	select {}
}

func mount(src, target, fstype string, flags uintptr, data string) error {
	if err := os.MkdirAll(target, 0o755); err != nil {
		return err
	}
	if err := syscall.Mount(src, target, fstype, flags, data); err != nil {
		return fmt.Errorf("mount %s on %s (%s): %w", src, target, fstype, err)
	}
	return nil
}

func initMain() {
	os.Setenv("PATH", "/usr/bin")
	for _, m := range []struct {
		src, target, fs string
		flags           uintptr
		data            string
	}{
		{"proc", "/proc", "proc", syscall.MS_NOSUID | syscall.MS_NODEV | syscall.MS_NOEXEC, "hidepid=invisible"},
		{"sysfs", "/sys", "sysfs", syscall.MS_NOSUID | syscall.MS_NODEV | syscall.MS_NOEXEC, ""},
		{"devtmpfs", "/dev", "devtmpfs", syscall.MS_NOSUID | syscall.MS_NOEXEC, "mode=0755"},
		{"tmpfs", "/run", "tmpfs", syscall.MS_NOSUID | syscall.MS_NODEV, "mode=0755"},
	} {
		if err := mount(m.src, m.target, m.fs, m.flags, m.data); err != nil {
			ifail(err)
		}
	}
	kmsg, _ = os.OpenFile("/dev/kmsg", os.O_WRONLY, 0)
	if _, err := os.Stat("/sys/firmware/efi"); err == nil {
		mount("efivarfs", efivarsDir, "efivarfs", syscall.MS_NOSUID|syscall.MS_NODEV|syscall.MS_NOEXEC, "")
	}
	os.MkdirAll(runDir, 0o755)
	c := kernelCmdline()
	version := c["klos.version"]
	ilog("KLOS %s (%s), mode %s", version, c["klos.flavor"], c["klos.mode"])
	var err error
	if c["klos.mode"] == "live" {
		err = setupLive(c)
	} else {
		err = setupInstalled(c)
	}
	if err == nil {
		err = prepareRoot(c["klos.mode"] == "live")
	}
	if err == nil && c["klos.mode"] != "live" {
		armWatchdog(version)
	}
	if err == nil {
		err = switchRoot("/usr/bin/dinit")
	}
	ifail(err)
}

func openVerity(dev string, c map[string]string) error {
	hash, off := c["klos.verity"], c["klos.hashoffset"]
	if len(hash) != 64 || off == "" {
		return errors.New("the kernel command line has no dm-verity root hash")
	}
	ilog("open %s with dm-verity", dev)
	return run("veritysetup", "open", dev, rootName, dev, hash, "--hash-offset="+off)
}

func setupInstalled(c map[string]string) error {
	ilog("wait for the partition %q", luksLabel)
	part, err := waitPartition(luksLabel, 60*time.Second)
	if err != nil {
		return err
	}
	unlocked := false
	askPIN := func() (string, error) {
		return readSecret("TPM PIN for " + part.Dev + " (empty: use the passphrase): ")
	}
	warn := func(err error) { ilog("%v", err) }
	switch err := tpmUnlock(part.Dev, askPIN, warn); {
	case err == nil:
		unlocked = true
		ilog("unlocked %s with the TPM", part.Dev)
	case errors.Is(err, errNoToken), errors.Is(err, errSkipped):
	default:
		ilog("the TPM did not unlock the disk: %v", err)
	}
	// Always lock PCR 15 before the next step, also after a failure: then
	// the disk key cannot be unsealed again before the next boot.
	lockPCR()
	for try := 1; try <= 3 && !unlocked; try++ {
		pass, err := readSecret("Passphrase for " + part.Dev + ": ")
		if err != nil {
			return err
		}
		err = runInput([]byte(pass), "cryptsetup", "open", "--type", "luks2",
			"--key-file=-", part.Dev, cryptName)
		if err == nil {
			unlocked = true
		} else {
			ilog("the passphrase is not correct (try %d of 3)", try)
		}
	}
	if !unlocked {
		return errors.New("the disk is still locked")
	}
	if err := lvm("vgchange", "--sysinit", "-ay", vgName); err != nil {
		return err
	}
	lv := osLVPrefix + c["klos.version"]
	if _, err := os.Stat(lvPath(lv)); err != nil {
		return fmt.Errorf("no OS volume %s/%s: %w", vgName, lv, err)
	}
	if err := openVerity(lvPath(lv), c); err != nil {
		return err
	}
	if err := mount("/dev/mapper/"+rootName, sysroot, "erofs", syscall.MS_RDONLY|syscall.MS_NODEV, ""); err != nil {
		return err
	}
	varDev := lvPath(varLV)
	if err := runFsck(varDev); err != nil {
		return err
	}
	return mount(varDev, sysroot+"/var", "ext4", syscall.MS_NOSUID|syscall.MS_NODEV, "")
}

// runFsck checks the /var file system. e2fsck exit codes: 0 and 1 are good,
// 2 needs a restart, 4 or more are errors that need a person.
func runFsck(dev string) error {
	err := run("e2fsck", "-p", dev)
	if err == nil {
		return nil
	}
	var ee interface{ ExitCode() int }
	if errors.As(err, &ee) {
		switch ee.ExitCode() {
		case 1:
			return nil
		case 2:
			ilog("e2fsck repaired /var; restart")
			syscall.Sync()
			syscall.Reboot(syscall.LINUX_REBOOT_CMD_RESTART)
		}
	}
	return err
}

func setupLive(c map[string]string) error {
	ilog("wait for the installer media %q", mediaLabel)
	part, err := waitPartition(mediaLabel, 60*time.Second)
	if err != nil {
		return err
	}
	media := runDir + "/media"
	if err := mount(part.Dev, media, "erofs", syscall.MS_RDONLY|syscall.MS_NODEV|syscall.MS_NOSUID, ""); err != nil {
		return err
	}
	out, err := output("losetup", "--find", "--show", "--read-only", media+"/bundle/root.img")
	if err != nil {
		return err
	}
	if err := openVerity(strings.TrimSpace(out), c); err != nil {
		return err
	}
	if err := mount("/dev/mapper/"+rootName, sysroot, "erofs", syscall.MS_RDONLY|syscall.MS_NODEV, ""); err != nil {
		return err
	}
	return mount("tmpfs", sysroot+"/var", "tmpfs", syscall.MS_NOSUID|syscall.MS_NODEV, "mode=0755,size=50%")
}

// varDirs are the directories that must exist on /var. The root image has
// symlinks to some of them (for example /home -> var/home).
var varDirs = []struct {
	path string
	mode os.FileMode
}{
	{"home", 0o755}, {"roothome", 0o700}, {"srv", 0o755}, {"usrlocal", 0o755},
	{"usrlocal/bin", 0o755}, {"lib", 0o755}, {"lib/klos", 0o700},
	{"lib/klos/etc", 0o700}, {"lib/klos/etc/upper", 0o755}, {"lib/klos/etc/work", 0o700},
	{"log", 0o755}, {"cache", 0o755}, {"tmp", 0o1777}, {"empty", 0o555},
}

func prepareRoot(live bool) error {
	v := sysroot + "/var"
	for _, d := range varDirs {
		p := filepath.Join(v, d.path)
		if _, err := os.Lstat(p); err != nil {
			if err := os.Mkdir(p, d.mode); err != nil {
				return err
			}
			os.Chmod(p, d.mode)
		}
	}
	for _, l := range [][2]string{{"run", "../run"}, {"lock", "../run/lock"}} {
		p := filepath.Join(v, l[0])
		if _, err := os.Lstat(p); err != nil {
			if err := os.Symlink(l[1], p); err != nil {
				return err
			}
		}
	}
	// /etc is the read-only /etc of the image, with the local changes in
	// an overlay upper layer on /var. Read docs/filesystem.md.
	opts := "lowerdir=" + sysroot + "/etc,upperdir=" + v + "/lib/klos/etc/upper,workdir=" + v + "/lib/klos/etc/work"
	if err := mount("overlay", sysroot+"/etc", "overlay", syscall.MS_NOSUID|syscall.MS_NODEV, opts); err != nil {
		return err
	}
	if live {
		os.WriteFile(runDir+"/live", nil, 0o644)
	}
	return nil
}

// Watchdog ioctls (linux/watchdog.h).
const wdiocSetTimeout = 0xc0045706

// armWatchdog starts a watchdog timer for a trial boot. A trial boot is
// a boot from BootNext: the running entry is not the first in BootOrder.
// "klos bless" stops the timer. If the new version hangs, the watchdog
// restarts the computer and the firmware starts the previous version.
func armWatchdog(version string) {
	if !efiAvailable() {
		return
	}
	cur, err := bootCurrent()
	if err != nil {
		return
	}
	order, err := bootOrder()
	if err != nil || (len(order) > 0 && order[0] == cur) {
		return
	}
	f, err := os.OpenFile("/dev/watchdog", os.O_WRONLY, 0)
	if err != nil {
		ilog("trial boot, but there is no watchdog: %v", err)
		return
	}
	timeout := int32(300)
	syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), wdiocSetTimeout, uintptr(unsafe.Pointer(&timeout)))
	// Closing without the magic character "V" keeps the timer running.
	f.Close()
	os.WriteFile(runDir+"/trial", []byte(strconv.Itoa(int(cur))), 0o644)
	ilog("trial boot of %s: the watchdog restarts the computer in %d s without 'klos bless'", version, timeout)
}

// switchRoot moves the API file systems to the new root, removes the
// initramfs files, and starts the init program of the new root.
func switchRoot(init string) error {
	for _, m := range []string{"/dev", "/proc", "/sys", "/run"} {
		if err := mount(m, sysroot+m, "", syscall.MS_MOVE, ""); err != nil {
			return err
		}
	}
	if err := os.Chdir(sysroot); err != nil {
		return err
	}
	var st syscall.Stat_t
	if err := syscall.Stat("/", &st); err != nil {
		return err
	}
	removeInitramfs("/", st.Dev)
	if err := syscall.Mount(".", "/", "", syscall.MS_MOVE, ""); err != nil {
		return fmt.Errorf("move the new root: %w", err)
	}
	if err := syscall.Chroot("."); err != nil {
		return err
	}
	if err := os.Chdir("/"); err != nil {
		return err
	}
	if kmsg != nil {
		kmsg.Close()
	}
	return syscall.Exec(init, []string{init}, []string{"PATH=/usr/bin", "TERM=linux"})
}

// removeInitramfs deletes the files of the initramfs to free the memory.
// It does not go into other file systems (such as /sysroot).
func removeInitramfs(dir string, dev uint64) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range ents {
		p := filepath.Join(dir, e.Name())
		var st syscall.Stat_t
		if syscall.Lstat(p, &st) != nil || st.Dev != dev {
			continue
		}
		if e.IsDir() {
			removeInitramfs(p, dev)
		}
		os.Remove(p)
	}
}
