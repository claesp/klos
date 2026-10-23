// SPDX-License-Identifier: BSD-2-Clause

package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// Names that the installer, the initramfs and the update tool share.
const (
	vgName      = "klos"
	varLV       = "var"
	cryptName   = "klos-crypt"
	rootName    = "klos-root"
	espLabel    = "klos-esp"
	luksLabel   = "klos-luks"
	mediaLabel  = "klos-install"
	osLVPrefix  = "os-"
	espMount    = "/run/klos/esp"
	bundlePub   = "/usr/share/klos/bundle.pub"
	stateDir    = "/var/lib/klos"
	runDir      = "/run/klos"
	keepVersion = 2 // the running version and one previous version
)

// lvmConfig makes LVM work without udev (the initramfs has no udev).
const lvmConfig = "activation{udev_sync=0 udev_rules=0} devices{obtain_device_list_from_udev=0} global{use_lvmpolld=0}"

func run(name string, args ...string) error {
	return runInput(nil, name, args...)
}

func runInput(stdin []byte, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	} else {
		cmd.Stdin = os.Stdin
	}
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return nil
}

func output(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return string(out), nil
}

func lvm(args ...string) error {
	return run("lvm", append([]string{args[0], "--config", lvmConfig}, args[1:]...)...)
}

// dmName returns the device-mapper name of a logical volume.
func dmName(vg, lv string) string {
	return strings.ReplaceAll(vg, "-", "--") + "-" + strings.ReplaceAll(lv, "-", "--")
}

func lvPath(lv string) string {
	return "/dev/mapper/" + dmName(vgName, lv)
}

// kernelCmdline returns the klos.* and other key=value kernel arguments.
func kernelCmdline() map[string]string {
	b, _ := os.ReadFile("/proc/cmdline")
	m := map[string]string{}
	for _, f := range strings.Fields(string(b)) {
		k, v, _ := strings.Cut(f, "=")
		if k == "console" {
			m[k] = strings.TrimSpace(m[k] + " " + v)
			continue
		}
		m[k] = v
	}
	return m
}

// compareVersions compares dotted numeric versions such as 2026.10.1.
func compareVersions(a, b string) int {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(pa) || i < len(pb); i++ {
		var x, y int
		if i < len(pa) {
			x, _ = strconv.Atoi(pa[i])
		}
		if i < len(pb) {
			y, _ = strconv.Atoi(pb[i])
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return strings.Compare(a, b)
}

func isLive() bool {
	return kernelCmdline()["klos.mode"] == "live"
}

func requireRoot() error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("this command must run as root")
	}
	return nil
}
