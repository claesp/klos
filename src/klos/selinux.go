// SPDX-License-Identifier: BSD-2-Clause

package main

// SELinux (read docs/selinux.md). KLOS has no SELinux user space (no
// libselinux, no policycoreutils). klos does the work that a distribution
// usually does with these tools:
//
//   - /init loads the binary policy of the OS image and labels /run and
//     /dev. The kernel command line sets the mode (enforcing=1), and /init
//     checks it. No process can change the policy or the mode.
//   - /init labels all of /var when the file contexts of the OS image are
//     not the same as at the last labeling (after the installation and
//     after an update), and the /etc overlay upper layer at each boot.
//   - "klos boot-setup" labels the directories that it makes.
//   - "klos selinux" shows the state and labels files again.

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

const (
	seDir       = "/usr/lib/klos/selinux"
	selinuxfs   = "/sys/fs/selinux"
	seXattr     = "security.selinux"
	seVarMarker = "/var/lib/klos/selinux-contexts"
	etcUpper    = "/var/lib/klos/etc/upper"
)

// seContexts are the file contexts of the running system (nil: SELinux is
// not active).
var seContexts *fileContexts

func seEnabled() bool {
	_, err := os.Stat(selinuxfs + "/enforce")
	return err == nil
}

// readSEConfig reads the mode from the config file of the policy.
func readSEConfig(root string) (string, error) {
	b, err := os.ReadFile(root + seDir + "/config")
	if err != nil {
		return "", err
	}
	for _, l := range strings.Split(string(b), "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(l), "mode="); ok {
			if v == "enforcing" || v == "permissive" {
				return v, nil
			}
			return "", fmt.Errorf("selinux config: bad mode %q", v)
		}
	}
	return "", errors.New("selinux config: no mode")
}

// loadSELinux loads the policy of the OS image in root (the initramfs
// /init calls it with /sysroot). An image without a policy has no SELinux.
func loadSELinux(root string) error {
	policy, err := os.ReadFile(root + seDir + "/policy")
	if os.IsNotExist(err) {
		ilog("the OS image has no SELinux policy")
		return nil
	}
	if err != nil {
		return err
	}
	mode, err := readSEConfig(root)
	if err != nil {
		return err
	}
	fcText, err := os.ReadFile(root + seDir + "/file_contexts")
	if err != nil {
		return err
	}
	fc, err := parseFileContexts(string(fcText))
	if err != nil {
		return err
	}
	if err := mount("selinuxfs", selinuxfs, "selinuxfs", syscall.MS_NOSUID|syscall.MS_NOEXEC, ""); err != nil {
		return fmt.Errorf("mount selinuxfs (does the kernel have SELinux?): %w", err)
	}
	if err := writeAll(selinuxfs+"/load", policy); err != nil {
		return fmt.Errorf("load the SELinux policy: %w", err)
	}
	seContexts = fc
	for _, d := range []string{"/run", "/dev"} {
		if err := labelTree(d, d); err != nil {
			return err
		}
	}
	// kbuild puts enforcing=1 on the kernel command line of an image in
	// enforcing mode. Do not start an image whose mode is not correct.
	b, err := os.ReadFile(selinuxfs + "/enforce")
	if err != nil {
		return err
	}
	if got := strings.TrimSpace(string(b)) == "1"; got != (mode == "enforcing") {
		return fmt.Errorf("SELinux: the kernel mode (enforce=%s) is not the mode of the image (%s)",
			strings.TrimSpace(string(b)), mode)
	}
	ilog("SELinux: loaded the policy (%d bytes), mode %s", len(policy), mode)
	return nil
}

// writeAll writes data to a file in one write (selinuxfs needs this).
func writeAll(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close() // the write error is the error to report
		return err
	}
	return f.Close()
}

// contextsHash identifies the file contexts (and thus the labels on /var).
func contextsHash(root string) (string, error) {
	b, err := os.ReadFile(root + seDir + "/file_contexts")
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

// labelVar labels /var (mounted at root+"/var") if the file contexts are new,
// and the /etc overlay upper layer at each boot. The labels of files that
// a process makes in /etc can be wrong (for example a new /etc/shadow gets
// the label of the directory); the next boot repairs them.
func labelVar(root string) error {
	if seContexts == nil {
		return nil
	}
	hash, err := contextsHash(root)
	if err != nil {
		return err
	}
	marker := root + seVarMarker
	if b, err := os.ReadFile(marker); err == nil && strings.TrimSpace(string(b)) == hash {
		return labelTree(root+etcUpper, etcUpper)
	}
	ilog("SELinux: label /var (new file contexts)")
	if err := labelTree(root+"/var", "/var"); err != nil {
		return err
	}
	if err := os.WriteFile(marker, []byte(hash+"\n"), 0o644); err != nil {
		return err
	}
	return labelPath(marker, seVarMarker)
}

// labelTree labels the files below dir (path is the name of dir in the
// running system). It does not go into other file systems.
func labelTree(dir, path string) error {
	var st syscall.Stat_t
	if err := syscall.Lstat(dir, &st); err != nil {
		return err
	}
	dev := st.Dev
	return filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil // a file that a process removed during the walk
			}
			return err
		}
		var s syscall.Stat_t
		if err := syscall.Lstat(p, &s); err != nil {
			return nil // removed during the walk
		}
		if s.Dev != dev {
			return filepath.SkipDir
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		return labelPath(p, filepath.Join(path, rel))
	})
}

// labelPath sets the label of the file p, whose name in the running system
// is path, from the file contexts.
func labelPath(p, path string) error {
	if seContexts == nil {
		return nil
	}
	info, err := os.Lstat(p)
	if err != nil {
		return err
	}
	ctx := seContexts.lookup(path, info.Mode())
	if ctx == "" {
		return nil
	}
	buf := make([]byte, 256)
	if n, err := unix.Lgetxattr(p, seXattr, buf); err == nil && strings.TrimRight(string(buf[:n]), "\x00") == ctx {
		return nil
	}
	if err := unix.Lsetxattr(p, seXattr, []byte(ctx), 0); err != nil {
		return fmt.Errorf("label %s as %s: %w", path, ctx, err)
	}
	return nil
}

// loadRunningContexts reads the file contexts of the running system, if
// SELinux is active.
func loadRunningContexts() error {
	if seContexts != nil || !seEnabled() {
		return nil
	}
	b, err := os.ReadFile(seDir + "/file_contexts")
	if err != nil {
		return err
	}
	fc, err := parseFileContexts(string(b))
	if err != nil {
		return err
	}
	seContexts = fc
	return nil
}

func cmdSELinux(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: klos selinux status|show PATH...|relabel PATH...")
	}
	switch args[0] {
	case "status":
		return seStatus()
	case "show":
		return seShow(args[1:])
	case "relabel":
		fs := flag.NewFlagSet("selinux relabel", flag.ContinueOnError)
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if err := requireRoot(); err != nil {
			return err
		}
		if err := loadRunningContexts(); err != nil {
			return err
		}
		if seContexts == nil {
			return errors.New("SELinux is not active")
		}
		for _, p := range fs.Args() {
			abs, err := filepath.Abs(p)
			if err != nil {
				return err
			}
			if err := labelTree(abs, abs); err != nil {
				return err
			}
		}
		return nil
	}
	return errors.New("usage: klos selinux status|show PATH...|relabel PATH...")
}

// seShow prints the SELinux label of each file (as "ls -Z").
func seShow(paths []string) error {
	buf := make([]byte, 256)
	failed := false
	for _, p := range paths {
		n, err := unix.Lgetxattr(p, seXattr, buf)
		if err != nil {
			fmt.Fprintf(os.Stderr, "klos: %s: %v\n", p, err)
			failed = true
			continue
		}
		fmt.Printf("%s %s\n", strings.TrimRight(string(buf[:n]), "\x00"), p)
	}
	if failed {
		return errors.New("some files have no label")
	}
	return nil
}

func seStatus() error {
	if !seEnabled() {
		fmt.Println("SELinux: not active")
		return nil
	}
	read := func(p string) string {
		b, err := os.ReadFile(p)
		if err != nil {
			return "unknown"
		}
		return strings.TrimRight(strings.TrimSpace(string(b)), "\x00")
	}
	mode := "permissive"
	if read(selinuxfs+"/enforce") == "1" {
		mode = "enforcing"
	}
	fmt.Printf("SELinux: %s\n", mode)
	fmt.Printf("policy version: %s\n", read(selinuxfs+"/policyvers"))
	fmt.Printf("process context: %s\n", read("/proc/self/attr/current"))
	if b, err := os.ReadFile(seVarMarker); err == nil {
		if h, err := contextsHash(""); err == nil && strings.TrimSpace(string(b)) == h {
			fmt.Println("/var labels: current")
		} else {
			fmt.Println("/var labels: old (the next boot labels /var again)")
		}
	}
	return nil
}

// enterDomain starts the program again in an SELinux domain. All commands
// of the klos program are one file with one label (klos_exec_t), so a
// domain transition at exec cannot select the domain of a daemon. The
// daemon sets the domain for its next exec (/proc/thread-self/attr/exec)
// and starts itself again. The policy allows the change only from init_t.
func enterDomain(domain string) error {
	if !seEnabled() {
		return nil
	}
	cur, err := os.ReadFile("/proc/self/attr/current")
	if err != nil {
		return fmt.Errorf("SELinux context: %w", err)
	}
	f := strings.SplitN(strings.TrimRight(string(cur), "\x00\n"), ":", 4)
	if len(f) != 4 {
		return fmt.Errorf("SELinux context %q: want user:role:type:level", cur)
	}
	if f[2] == domain {
		return nil
	}
	f[2] = domain
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	// attr/exec applies to one thread: the exec must come from that thread.
	runtime.LockOSThread()
	if err := os.WriteFile("/proc/thread-self/attr/exec", []byte(strings.Join(f, ":")), 0); err != nil {
		runtime.UnlockOSThread()
		return fmt.Errorf("set the SELinux domain %s: %w", domain, err)
	}
	return syscall.Exec(exe, os.Args, os.Environ())
}
