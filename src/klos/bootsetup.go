// SPDX-License-Identifier: BSD-2-Clause

package main

// "klos boot-setup" is the first dinit service. It does the early boot
// steps without a shell script:
//
//   - mount /dev/pts, /dev/shm, /tmp, the cgroup2 hierarchy and securityfs
//   - make the directories in /usr/lib/klos/dirs.d/*.conf
//   - apply /usr/lib/sysctl.d/*.conf and then /etc/sysctl.d/*.conf
//   - set the host name from /etc/hostname
//   - bring up the loopback interface
//   - make /etc/machine-id and the SSH host key on the first boot

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"unsafe"
)

func cmdBootSetup(args []string) error {
	if err := requireRoot(); err != nil {
		return err
	}
	var errs []string
	note := func(err error) {
		if err != nil {
			errs = append(errs, err.Error())
			fmt.Fprintf(os.Stderr, "klos boot-setup: %v\n", err)
		}
	}
	nosuidNodev := uintptr(syscall.MS_NOSUID | syscall.MS_NODEV)
	for _, m := range []struct {
		src, target, fs string
		flags           uintptr
		data            string
	}{
		{"devpts", "/dev/pts", "devpts", syscall.MS_NOSUID | syscall.MS_NOEXEC, "gid=5,mode=0620,ptmxmode=0666"},
		{"shm", "/dev/shm", "tmpfs", nosuidNodev, "mode=1777"},
		{"tmpfs", "/tmp", "tmpfs", nosuidNodev, "mode=1777,size=25%"},
		{"cgroup2", "/sys/fs/cgroup", "cgroup2", nosuidNodev | syscall.MS_NOEXEC, "nsdelegate"},
		{"securityfs", "/sys/kernel/security", "securityfs", nosuidNodev | syscall.MS_NOEXEC, ""},
	} {
		if isMounted(m.target) {
			continue
		}
		if _, err := os.Stat(filepath.Dir(m.target)); err != nil {
			continue
		}
		note(mount(m.src, m.target, m.fs, m.flags, m.data))
	}
	note(makeDirs())
	note(applySysctl())
	note(setHostname())
	note(loopbackUp())
	note(machineID())
	note(userRuntimeDirs())
	if _, err := os.Stat("/usr/bin/ssh-keygen"); err == nil {
		key := "/etc/ssh/ssh_host_ed25519_key"
		if _, err := os.Stat(key); err != nil {
			note(run("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", key))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%d steps failed", len(errs))
	}
	return nil
}

func isMounted(target string) bool {
	b, _ := os.ReadFile("/proc/self/mounts")
	for _, l := range strings.Split(string(b), "\n") {
		f := strings.Fields(l)
		if len(f) > 1 && f[1] == target {
			return true
		}
	}
	return false
}

// confFiles returns the *.conf files of the directories. A file in a later
// directory replaces a file with the same name in an earlier directory.
func confFiles(dirs ...string) []string {
	byName := map[string]string{}
	for _, d := range dirs {
		m, _ := filepath.Glob(filepath.Join(d, "*.conf"))
		for _, f := range m {
			byName[filepath.Base(f)] = f
		}
	}
	var names []string
	for n := range byName {
		names = append(names, n)
	}
	sort.Strings(names)
	var out []string
	for _, n := range names {
		out = append(out, byName[n])
	}
	return out
}

func confLines(path string) [][]string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out [][]string
	s := bufio.NewScanner(f)
	for s.Scan() {
		l := strings.TrimSpace(s.Text())
		if l == "" || l[0] == '#' || l[0] == ';' {
			continue
		}
		out = append(out, strings.Fields(l))
	}
	return out
}

func lookupID(file, name string) (int, error) {
	if n, err := strconv.Atoi(name); err == nil {
		return n, nil
	}
	b, err := os.ReadFile(file)
	if err != nil {
		return 0, err
	}
	for _, l := range strings.Split(string(b), "\n") {
		f := strings.Split(l, ":")
		if len(f) > 2 && f[0] == name {
			return strconv.Atoi(f[2])
		}
	}
	return 0, fmt.Errorf("%s: no entry %q", file, name)
}

// makeDirs reads lines "d PATH MODE USER GROUP" from dirs.d/*.conf.
func makeDirs() error {
	for _, f := range confFiles("/usr/lib/klos/dirs.d", "/etc/klos/dirs.d") {
		for _, l := range confLines(f) {
			if len(l) != 5 || l[0] != "d" {
				return fmt.Errorf("%s: bad line %q", f, strings.Join(l, " "))
			}
			mode, err := strconv.ParseUint(l[2], 8, 32)
			if err != nil {
				return fmt.Errorf("%s: bad mode %q", f, l[2])
			}
			uid, err := lookupID("/etc/passwd", l[3])
			if err != nil {
				return err
			}
			gid, err := lookupID("/etc/group", l[4])
			if err != nil {
				return err
			}
			if err := os.MkdirAll(l[1], 0o755); err != nil {
				return err
			}
			os.Chmod(l[1], os.FileMode(mode)|modeBits(mode))
			if err := os.Lchown(l[1], uid, gid); err != nil {
				return err
			}
		}
	}
	return nil
}

func modeBits(m uint64) os.FileMode {
	var out os.FileMode
	if m&0o1000 != 0 {
		out |= os.ModeSticky
	}
	return out
}

func applySysctl() error {
	for _, f := range confFiles("/usr/lib/sysctl.d", "/etc/sysctl.d") {
		b, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		for _, l := range strings.Split(string(b), "\n") {
			l = strings.TrimSpace(l)
			if l == "" || l[0] == '#' || l[0] == ';' {
				continue
			}
			k, v, ok := strings.Cut(l, "=")
			if !ok {
				return fmt.Errorf("%s: bad line %q", f, l)
			}
			k, v = strings.TrimSpace(k), strings.TrimSpace(v)
			ignore := strings.HasPrefix(k, "-")
			k = strings.TrimPrefix(k, "-")
			p := "/proc/sys/" + strings.ReplaceAll(k, ".", "/")
			if err := os.WriteFile(p, []byte(v), 0); err != nil && !ignore {
				fmt.Fprintf(os.Stderr, "klos boot-setup: sysctl %s: %v\n", k, err)
			}
		}
	}
	return nil
}

func setHostname() error {
	b, err := os.ReadFile("/etc/hostname")
	name := strings.TrimSpace(string(b))
	if err != nil || name == "" {
		name = "klos"
	}
	return syscall.Sethostname([]byte(name))
}

// loopbackUp sets the IFF_UP flag of "lo" with the SIOCSIFFLAGS ioctl.
func loopbackUp() error {
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_DGRAM|syscall.SOCK_CLOEXEC, 0)
	if err != nil {
		return err
	}
	defer syscall.Close(fd)
	var ifr [40]byte
	copy(ifr[:], "lo")
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), syscall.SIOCGIFFLAGS, uintptr(unsafe.Pointer(&ifr[0]))); e != 0 {
		return fmt.Errorf("lo: %v", e)
	}
	flags := *(*uint16)(unsafe.Pointer(&ifr[16]))
	*(*uint16)(unsafe.Pointer(&ifr[16])) = flags | syscall.IFF_UP
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), syscall.SIOCSIFFLAGS, uintptr(unsafe.Pointer(&ifr[0]))); e != 0 {
		return fmt.Errorf("lo: %v", e)
	}
	return nil
}

// userRuntimeDirs makes /run/user/<uid> (XDG_RUNTIME_DIR) for each normal
// user (UID 1000 to 59999). KLOS has no login daemon that does this.
func userRuntimeDirs() error {
	b, err := os.ReadFile("/etc/passwd")
	if err != nil {
		return err
	}
	for _, l := range strings.Split(string(b), "\n") {
		f := strings.Split(l, ":")
		if len(f) < 4 {
			continue
		}
		uid, err1 := strconv.Atoi(f[2])
		gid, err2 := strconv.Atoi(f[3])
		if err1 != nil || err2 != nil || uid < 1000 || uid >= 60000 {
			continue
		}
		// /run/user must be 0755, so that each user can get to its directory.
		if err := os.MkdirAll("/run/user", 0o755); err != nil {
			return err
		}
		d := fmt.Sprintf("/run/user/%d", uid)
		if err := os.Mkdir(d, 0o700); err != nil && !os.IsExist(err) {
			return err
		}
		os.Chmod(d, 0o700)
		if err := os.Chown(d, uid, gid); err != nil {
			return err
		}
	}
	return nil
}

func machineID() error {
	b, err := os.ReadFile("/etc/machine-id")
	if err == nil && len(strings.TrimSpace(string(b))) == 32 {
		return nil
	}
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return err
	}
	return os.WriteFile("/etc/machine-id", []byte(hex.EncodeToString(id)+"\n"), 0o444)
}
