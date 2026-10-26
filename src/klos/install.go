// SPDX-License-Identifier: BSD-2-Clause

package main

// "klos install" runs on the installer (live mode). It makes this layout
// on the target disk (read docs/disk-layout.md):
//
//	GPT partition 1  "klos-esp"   EFI system partition, FAT32, 512 MiB
//	GPT partition 2  "klos-luks"  LUKS2 (argon2id, AES-XTS), all other space
//	  LVM volume group "klos"
//	    os-<version>  erofs OS image with dm-verity (read-only)
//	    var           ext4, all writable data
//	    free space    for the next OS versions
//
// A configuration file (key=value lines) can give the answers. Without it,
// the installer asks on the console. Read docs/installing.md.

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type installConfig struct {
	Disk          string
	Passphrase    string
	Hostname      string
	RootPassword  string // a crypt(3) hash, or empty to lock the password
	SSHKey        string // an authorized_keys line for root
	User          string // a normal user (UID 1000), optional
	UserPassword  string // a crypt(3) hash for the user
	UserSSHKey    string // an authorized_keys line for the user
	PowerOff      bool
	ReservedSlots int
	SecureBoot    string // "enroll" or "enroll-microsoft": enroll the KLOS keys if the firmware is in Setup Mode
}

func readInstallConfig(path string) (*installConfig, error) {
	c := &installConfig{Hostname: "klos", PowerOff: true, ReservedSlots: 2}
	if path == "" {
		return c, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer closeQuietly(f)
	s := bufio.NewScanner(f)
	for s.Scan() {
		l := strings.TrimSpace(s.Text())
		if l == "" || l[0] == '#' {
			continue
		}
		k, v, ok := strings.Cut(l, "=")
		if !ok {
			return nil, fmt.Errorf("%s: bad line %q", path, l)
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		switch k {
		case "disk":
			c.Disk = v
		case "passphrase":
			c.Passphrase = v
		case "hostname":
			c.Hostname = v
		case "root_password_hash":
			c.RootPassword = v
		case "root_ssh_key":
			c.SSHKey = v
		case "user":
			c.User = v
		case "user_password_hash":
			c.UserPassword = v
		case "user_ssh_key":
			c.UserSSHKey = v
		case "secureboot":
			if v != "enroll" && v != "enroll-microsoft" && v != "no" {
				return nil, fmt.Errorf("%s: secureboot must be enroll, enroll-microsoft or no", path)
			}
			c.SecureBoot = v
		case "poweroff":
			c.PowerOff = v == "yes"
		case "reserved_slots":
			c.ReservedSlots, err = strconv.Atoi(v)
			if err != nil {
				return nil, fmt.Errorf("%s: reserved_slots: %w", path, err)
			}
		default:
			return nil, fmt.Errorf("%s: unknown key %q", path, k)
		}
	}
	return c, s.Err()
}

// progress shows an installer message on all consoles (for example the
// screen and the serial port), so that each one shows the installation.
func progress(format string, a ...any) {
	consoleWrite(fmt.Sprintf(format, a...) + "\n")
}

func prompt(q string) string {
	fmt.Print(q)
	s := bufio.NewScanner(os.Stdin)
	s.Scan()
	return strings.TrimSpace(s.Text())
}

func askMissing(c *installConfig) error {
	if c.Disk == "" {
		fmt.Println("Disks:")
		for _, d := range disks() {
			sz, _ := readSysInt("/sys/block/" + d + "/size")
			ro, _ := readSysInt("/sys/block/" + d + "/ro")
			if ro == 1 || strings.HasPrefix(d, "loop") {
				continue
			}
			fmt.Printf("  /dev/%-10s %6d GiB\n", d, sz*512>>30)
		}
		c.Disk = prompt("Install on which disk (for example /dev/vda)? ")
	}
	if c.Passphrase == "" {
		for {
			a, err := readSecret("Disk encryption passphrase: ")
			if err != nil {
				return err
			}
			b, err := readSecret("Type the passphrase again: ")
			if err != nil {
				return err
			}
			if a != b {
				fmt.Println("The passphrases are not the same.")
				continue
			}
			if len(a) < 8 {
				fmt.Println("Use 8 characters or more.")
				continue
			}
			c.Passphrase = a
			break
		}
	}
	if c.RootPassword == "" && c.SSHKey == "" {
		pw, err := readSecret("Root password (empty: no password login): ")
		if err != nil {
			return err
		}
		if pw != "" {
			h, err := hashPassword(pw)
			if err != nil {
				return err
			}
			c.RootPassword = h
		}
	}
	return nil
}

// hashPassword makes a SHA-512 crypt(3) hash with the toybox mkpasswd
// command. The password goes through a pipe, not the command line.
func hashPassword(pw string) (string, error) {
	cmd := exec.Command("mkpasswd", "-m", "sha512", "-P", "0")
	cmd.Stdin = strings.NewReader(pw + "\n")
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("mkpasswd: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// cmdInstallService is the dinit service "klos-installer". It runs only in
// live mode. With /run/klos/media/install.conf it installs without
// questions; without it, it asks on the console.
func cmdInstallService(args []string) error {
	if !isLive() {
		return nil
	}
	conf := runDir + "/media/install.conf"
	if _, err := os.Stat(conf); err == nil {
		if err := cmdInstall([]string{"-config", conf}); err != nil {
			progress("The installation did not complete: %v", err)
			return err
		}
		return nil
	}
	for {
		fmt.Println("\nKLOS installer")
		err := cmdInstall(nil)
		if err == nil {
			return nil
		}
		progress("The installation did not complete: %v", err)
		if prompt("Type R to try again, or P to power off: ") == "P" {
			return run("poweroff")
		}
	}
}

func cmdInstall(args []string) error {
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	cfgPath := fs.String("config", "", "configuration file (key=value)")
	yes := fs.Bool("yes", false, "do not ask for confirmation")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := requireRoot(); err != nil {
		return err
	}
	if !isLive() {
		return errors.New("'klos install' runs only on the installer")
	}
	c, err := readInstallConfig(*cfgPath)
	if err != nil {
		return err
	}
	if err := askMissing(c); err != nil {
		return err
	}
	disk := strings.TrimPrefix(c.Disk, "/dev/")
	if _, err := os.Stat("/sys/block/" + disk); err != nil || strings.Contains(disk, "/") {
		return fmt.Errorf("%s is not a disk", c.Disk)
	}
	if media, err := findPartition(mediaLabel); err == nil && media.Disk == disk {
		return errors.New("that disk is the installer media")
	}
	if !*yes && *cfgPath == "" {
		if prompt(fmt.Sprintf("All data on /dev/%s will be deleted. Type YES to continue: ", disk)) != "YES" {
			return errors.New("stopped")
		}
	}
	bundle := runDir + "/media/bundle"
	info, err := verifyBundle(bundle, bundlePub)
	if err != nil {
		return err
	}
	progress("install KLOS %s (%s) on /dev/%s", info.Version, info.Flavor, disk)
	if err := partitionDisk(disk); err != nil {
		return err
	}
	esp, luks, err := waitInstallParts(disk)
	if err != nil {
		return err
	}
	if err := run("mkfs.fat", "-F", "32", "-n", "KLOS-ESP", esp.Dev); err != nil {
		return err
	}
	progress("encrypt the disk (LUKS2, argon2id)")
	key := []byte(c.Passphrase)
	if err := runInput(key, "cryptsetup", "luksFormat", "--batch-mode", "--type", "luks2",
		"--pbkdf", "argon2id", "--cipher", "aes-xts-plain64", "--key-size", "512",
		"--label", "klos", "--key-file=-", luks.Dev); err != nil {
		return err
	}
	if err := runInput(key, "cryptsetup", "open", "--type", "luks2", "--key-file=-", luks.Dev, cryptName); err != nil {
		return err
	}
	// The deferred steps clean up after an error. The error of the main
	// step is the error to report, so their errors are not.
	defer func() { _ = run("cryptsetup", "close", cryptName) }()
	pv := "/dev/mapper/" + cryptName
	if err := lvm("pvcreate", "--yes", pv); err != nil {
		return err
	}
	if err := lvm("vgcreate", "--yes", vgName, pv); err != nil {
		return err
	}
	defer func() { _ = lvm("vgchange", "-an", vgName) }()
	if err := mount(esp.Dev, espMount, "vfat", syscall.MS_NOSUID|syscall.MS_NODEV|syscall.MS_NOEXEC, "umask=0077,shortname=winnt"); err != nil {
		return err
	}
	defer func() { _ = syscall.Unmount(espMount, 0) }()
	if err := installVersion(bundle, info, esp); err != nil {
		return err
	}
	// The /var volume gets the free space, but not the reserved space for
	// the next OS versions.
	slot := (info.SlotSize + (4 << 20) - 1) / (4 << 20) * (4 << 20)
	out, err := output("lvm", "vgs", "--config", lvmConfig, "--noheadings", "--units", "b",
		"--nosuffix", "-o", "vg_free", vgName)
	if err != nil {
		return err
	}
	free, err := strconv.ParseInt(strings.TrimSpace(out), 10, 64)
	if err != nil {
		return err
	}
	varSize := (free - int64(c.ReservedSlots)*slot) / (4 << 20) * (4 << 20)
	if varSize < 1<<30 {
		return fmt.Errorf("the disk is too small (/var would be %d MiB)", varSize>>20)
	}
	if err := lvm("lvcreate", "--yes", "-L", strconv.FormatInt(varSize, 10)+"b", "-n", varLV, vgName); err != nil {
		return err
	}
	if err := run("mkfs.ext4", "-q", "-L", "klos-var", lvPath(varLV)); err != nil {
		return err
	}
	if err := setupVar(c); err != nil {
		return err
	}
	if err := copyAtomic(espPath(espFile(info.Version)), espPath(`\EFI\BOOT\BOOTX64.EFI`)); err != nil {
		return err
	}
	if efiAvailable() {
		n, err := createBootEntry(entryDesc(info.Version), esp, espFile(info.Version))
		if err != nil {
			return err
		}
		if err := moveToFront(n); err != nil {
			return err
		}
	}
	if c.SecureBoot == "enroll" || c.SecureBoot == "enroll-microsoft" {
		if efiFlag("SetupMode") == 1 {
			var args []string
			if c.SecureBoot == "enroll-microsoft" {
				args = []string{"-microsoft"}
			}
			if err := secureBootEnroll(args); err != nil {
				return err
			}
			progress("enrolled the KLOS Secure Boot keys")
		} else {
			progress("Secure Boot: the firmware is not in Setup Mode, so the keys are not enrolled")
		}
	}
	syscall.Sync()
	progress("KLOS is installed. Remove the installer media and start the computer.")
	if c.PowerOff {
		// The installation is complete and synced; these steps are tidy-up
		// before the power off.
		_ = syscall.Unmount(espMount, 0)
		_ = lvm("vgchange", "-an", vgName)
		_ = run("cryptsetup", "close", cryptName)
		syscall.Sync()
		return run("poweroff")
	}
	return nil
}

func partitionDisk(disk string) error {
	progress("make the partition table on /dev/%s", disk)
	// Remove old signatures; sfdisk --wipe always does the same for the
	// partition table, so a wipefs error is not fatal.
	if err := run("wipefs", "--all", "--force", "/dev/"+disk); err != nil {
		progress("wipefs: %v", err)
	}
	script := "label: gpt\n" +
		"size=512MiB, type=C12A7328-F81F-11D2-BA4B-00A0C93EC93B, name=\"" + espLabel + "\"\n" +
		"type=CA7D7CCB-63ED-4C53-861C-1742536059CC, name=\"" + luksLabel + "\"\n"
	return runInput([]byte(script), "sfdisk", "--quiet", "--wipe", "always", "/dev/"+disk)
}

func waitInstallParts(disk string) (esp, luks *Partition, err error) {
	for i := 0; i < 40; i++ {
		parts, perr := readGPT(disk)
		if perr == nil {
			for j := range parts {
				p := &parts[j]
				if _, serr := os.Stat(p.Dev); p.Dev == "" || serr != nil {
					continue
				}
				switch p.Name {
				case espLabel:
					esp = p
				case luksLabel:
					luks = p
				}
			}
			if esp != nil && luks != nil {
				return esp, luks, nil
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	return nil, nil, errors.New("the new partitions did not appear")
}

// setupVar writes the first contents of /var: the /etc overlay upper layer
// with the host name and the root password, and the root SSH key.
func setupVar(c *installConfig) error {
	mnt := runDir + "/target-var"
	if err := mount(lvPath(varLV), mnt, "ext4", syscall.MS_NOSUID|syscall.MS_NODEV, ""); err != nil {
		return err
	}
	defer func() { _ = syscall.Unmount(mnt, 0) }() // the caller syncs the disks
	for _, d := range varDirs {
		if err := os.MkdirAll(filepath.Join(mnt, d.path), 0o755); err != nil {
			return err
		}
		if err := os.Chmod(filepath.Join(mnt, d.path), d.mode); err != nil {
			return err
		}
	}
	for _, l := range [][2]string{{"../run", "run"}, {"../run/lock", "lock"}} {
		if err := os.Symlink(l[0], filepath.Join(mnt, l[1])); err != nil && !os.IsExist(err) {
			return err
		}
	}
	upper := filepath.Join(mnt, "lib/klos/etc/upper")
	if err := os.WriteFile(filepath.Join(upper, "hostname"), []byte(c.Hostname+"\n"), 0o644); err != nil {
		return err
	}
	if c.RootPassword != "" {
		b, err := os.ReadFile("/etc/shadow")
		if err != nil {
			return err
		}
		var lines []string
		for _, l := range strings.Split(strings.TrimRight(string(b), "\n"), "\n") {
			f := strings.Split(l, ":")
			if len(f) > 1 && f[0] == "root" {
				f[1] = c.RootPassword
				l = strings.Join(f, ":")
			}
			lines = append(lines, l)
		}
		if err := os.WriteFile(filepath.Join(upper, "shadow"), []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
			return err
		}
	}
	if c.User != "" {
		if err := addUser(c, mnt, upper); err != nil {
			return err
		}
	}
	if c.SSHKey != "" {
		d := filepath.Join(mnt, "roothome/.ssh")
		if err := os.MkdirAll(d, 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(d, "authorized_keys"), []byte(c.SSHKey+"\n"), 0o600); err != nil {
			return err
		}
	}
	return nil
}

// addUser adds the normal user (UID and GID 1000) to the /etc overlay upper
// layer, and makes the home directory /var/home/<user>. The user gets the
// groups in /usr/lib/klos/user-groups (a list that the flavor supplies).
func addUser(c *installConfig, mnt, upper string) error {
	name := c.User
	for _, r := range name {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return fmt.Errorf("bad user name %q", name)
		}
	}
	const id = 1000
	edit := func(file string, mode os.FileMode, change func([]string) []string) error {
		src := filepath.Join(upper, file)
		b, err := os.ReadFile(src)
		if err != nil {
			b, err = os.ReadFile(filepath.Join("/etc", file))
			if err != nil {
				return err
			}
		}
		lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
		lines = change(lines)
		return os.WriteFile(src, []byte(strings.Join(lines, "\n")+"\n"), mode)
	}
	groups := map[string]bool{}
	if b, err := os.ReadFile("/usr/lib/klos/user-groups"); err == nil {
		for _, g := range strings.Fields(string(b)) {
			groups[g] = true
		}
	}
	if err := edit("passwd", 0o644, func(l []string) []string {
		return append(l, fmt.Sprintf("%s:x:%d:%d:%s:/home/%s:/bin/sh", name, id, id, name, name))
	}); err != nil {
		return err
	}
	if err := edit("group", 0o644, func(l []string) []string {
		for i, line := range l {
			f := strings.Split(line, ":")
			if len(f) == 4 && groups[f[0]] {
				if f[3] == "" {
					f[3] = name
				} else {
					f[3] += "," + name
				}
				l[i] = strings.Join(f, ":")
			}
		}
		return append(l, fmt.Sprintf("%s:x:%d:", name, id))
	}); err != nil {
		return err
	}
	pw := c.UserPassword
	if pw == "" {
		pw = "!"
	}
	if err := edit("shadow", 0o600, func(l []string) []string {
		return append(l, fmt.Sprintf("%s:%s:0::::::", name, pw))
	}); err != nil {
		return err
	}
	home := filepath.Join(mnt, "home", name)
	if err := os.MkdirAll(home, 0o700); err != nil {
		return err
	}
	if c.UserSSHKey != "" {
		if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(home, ".ssh/authorized_keys"), []byte(c.UserSSHKey+"\n"), 0o600); err != nil {
			return err
		}
		for _, p := range []string{".ssh", ".ssh/authorized_keys"} {
			if err := os.Chown(filepath.Join(home, p), id, id); err != nil {
				return err
			}
		}
	}
	return os.Chown(home, id, id)
}
