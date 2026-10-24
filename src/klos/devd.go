// SPDX-License-Identifier: BSD-2-Clause

package main

// "klos devd" is the device manager of the desktop flavor. It replaces udev
// with a small set of functions:
//
//  1. At start, it loads the kernel modules for all devices (coldplug).
//     It reads the "modalias" files in /sys/devices and runs modprobe.
//  2. It reads the kernel uevents (netlink) and loads the modules for new
//     devices (hotplug).
//  3. It sends each uevent again to the netlink group 4. libudev-zero
//     (libinput, wlroots) listens on that group.
//  4. It sets the group and the mode of device nodes from a fixed table
//     (for example /dev/snd/* is root:audio 0660 and /dev/dri/* is
//     root:video 0660). seatd opens the DRM master device and the input
//     devices for the user; the render nodes need the group "video".

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

const (
	netlinkKobjectUevent = 15
	ueventGroupKernel    = 1
	ueventGroupLibudev   = 4
)

type devRule struct {
	subsystem string
	group     string
	mode      os.FileMode
}

var devRules = []devRule{
	{"sound", "audio", 0o660},
	{"drm", "video", 0o660},
	{"video4linux", "video", 0o660},
	{"rfkill", "rfkill", 0o664},
}

func devLog(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "klos devd: "+format+"\n", a...)
}

// modprobe loads the modules for a list of aliases. modprobe ignores
// aliases that have no module.
func modprobe(aliases []string) {
	for len(aliases) > 0 {
		n := len(aliases)
		if n > 64 {
			n = 64
		}
		args := append([]string{"-b", "-q", "-a"}, aliases[:n]...)
		if err := exec.Command("modprobe", args...).Run(); err != nil {
			// A failed alias is normal (no driver); log only other errors.
			if _, ok := err.(*exec.ExitError); !ok {
				devLog("modprobe: %v", err)
			}
		}
		aliases = aliases[n:]
	}
}

func coldplug() {
	seen := map[string]bool{}
	filepath.WalkDir("/sys/devices", func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.Name() == "modalias" && !d.IsDir() {
			if b, err := os.ReadFile(p); err == nil {
				if a := strings.TrimSpace(string(b)); a != "" {
					seen[a] = true
				}
			}
		}
		return nil
	})
	var aliases []string
	for a := range seen {
		aliases = append(aliases, a)
	}
	sort.Strings(aliases)
	devLog("coldplug: %d device aliases", len(aliases))
	modprobe(aliases)
	// Load the modules of devices that the first modules added.
	time.Sleep(time.Second)
	filepath.WalkDir("/dev", func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			applyRules(p)
		}
		return nil
	})
}

func lookupGroup(name string) (int, bool) {
	id, err := lookupID("/etc/group", name)
	return id, err == nil
}

// applyRules sets the group and mode of a device node from devRules.
func applyRules(dev string) {
	var st syscall.Stat_t
	if syscall.Stat(dev, &st) != nil {
		return
	}
	kind := "char"
	if st.Mode&syscall.S_IFMT == syscall.S_IFBLK {
		kind = "block"
	} else if st.Mode&syscall.S_IFMT != syscall.S_IFCHR {
		return
	}
	link, err := os.Readlink(fmt.Sprintf("/sys/dev/%s/%d:%d/subsystem", kind,
		(st.Rdev>>8)&0xfff|((st.Rdev>>32)&^0xfff), (st.Rdev&0xff)|((st.Rdev>>12)&^0xff)))
	if err != nil {
		return
	}
	sub := filepath.Base(link)
	for _, r := range devRules {
		if r.subsystem != sub {
			continue
		}
		if gid, ok := lookupGroup(r.group); ok {
			os.Chown(dev, 0, gid)
			os.Chmod(dev, r.mode)
		}
	}
}

func parseUevent(b []byte) map[string]string {
	m := map[string]string{}
	for _, f := range bytes.Split(b, []byte{0}) {
		if k, v, ok := strings.Cut(string(f), "="); ok {
			m[k] = v
		}
	}
	return m
}

func cmdDevd(args []string) error {
	if err := requireRoot(); err != nil {
		return err
	}
	fd, err := syscall.Socket(syscall.AF_NETLINK, syscall.SOCK_DGRAM|syscall.SOCK_CLOEXEC, netlinkKobjectUevent)
	if err != nil {
		return err
	}
	if err := syscall.Bind(fd, &syscall.SockaddrNetlink{Family: syscall.AF_NETLINK, Groups: ueventGroupKernel}); err != nil {
		return err
	}
	syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_RCVBUF, 4<<20)
	coldplug()
	libudev := &syscall.SockaddrNetlink{Family: syscall.AF_NETLINK, Groups: ueventGroupLibudev}
	buf := make([]byte, 64<<10)
	for {
		n, from, err := syscall.Recvfrom(fd, buf, 0)
		if err != nil {
			if err == syscall.EINTR || err == syscall.ENOBUFS {
				continue
			}
			return err
		}
		// Accept only messages from the kernel (port ID 0).
		if nl, ok := from.(*syscall.SockaddrNetlink); !ok || nl.Pid != 0 {
			continue
		}
		msg := buf[:n]
		ev := parseUevent(msg)
		if ev["ACTION"] == "add" {
			if a := ev["MODALIAS"]; a != "" {
				modprobe([]string{a})
			}
			if dn := ev["DEVNAME"]; dn != "" {
				applyRules("/dev/" + dn)
			}
		}
		syscall.Sendto(fd, msg, 0, libudev)
	}
}
