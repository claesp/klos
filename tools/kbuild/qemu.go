// SPDX-License-Identifier: BSD-2-Clause

package main

// The boot test (read docs/testing.md). It runs QEMU with KVM in the
// builder container and uses the serial console:
//
//  1. Install the flavor from an unattended installer image.
//  2. Boot the installed system, unlock the disk and log in.
//  3. Check the read-only root, the firewall and the "bless" step.
//  4. Update to a new version (a trial boot), and check it.
//  5. Roll back to the first version, and check it.

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	testPassphrase = "klos-test-passphrase"
	testPassword   = "klos-test"
	ovmfCode       = "/usr/share/OVMF/OVMF_CODE.fd"
	ovmfVars       = "/usr/share/OVMF/OVMF_VARS.fd"
)

type vm struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser
	mu    sync.Mutex
	buf   []byte
	pos   int
	done  chan error
	log   *os.File
}

func qemuArgs(vars string, drives []string, extra ...string) []string {
	args := []string{"qemu-system-x86_64", "-nodefaults", "-no-user-config",
		"-machine", "q35", "-m", "2048", "-smp", "2",
		"-display", "none", "-serial", "stdio",
		"-drive", "if=pflash,format=raw,unit=0,readonly=on,file=" + ovmfCode,
		"-drive", "if=pflash,format=raw,unit=1,file=" + vars,
		"-device", "virtio-rng-pci",
		"-netdev", "user,id=n0", "-device", "virtio-net-pci,netdev=n0"}
	if exists("/dev/kvm") {
		args = append(args, "-accel", "kvm", "-cpu", "host")
	} else {
		logf("no /dev/kvm: QEMU uses software emulation (slow)")
		args = append(args, "-accel", "tcg", "-cpu", "max")
	}
	for _, d := range drives {
		args = append(args, "-drive", d)
	}
	return append(args, extra...)
}

func startVM(args []string, logPath string) *vm {
	v := &vm{done: make(chan error, 1)}
	var err error
	v.log, err = os.Create(logPath)
	check(err)
	v.cmd = exec.Command(args[0], args[1:]...)
	v.stdin, err = v.cmd.StdinPipe()
	check(err)
	out, err := v.cmd.StdoutPipe()
	check(err)
	v.cmd.Stderr = v.log
	check(v.cmd.Start())
	go func() {
		b := make([]byte, 4096)
		for {
			n, err := out.Read(b)
			if n > 0 {
				v.mu.Lock()
				v.buf = append(v.buf, b[:n]...)
				v.mu.Unlock()
				v.log.Write(b[:n])
				if os.Getenv("KLOS_TEST_VERBOSE") != "" {
					os.Stderr.Write(b[:n])
				}
			}
			if err != nil {
				break
			}
		}
		v.done <- v.cmd.Wait()
	}()
	return v
}

func (v *vm) tail() string {
	v.mu.Lock()
	defer v.mu.Unlock()
	t := v.buf
	if len(t) > 3000 {
		t = t[len(t)-3000:]
	}
	return string(t)
}

// expect waits for a regular expression in the new console output.
func (v *vm) expect(pattern string, timeout time.Duration) []string {
	re := regexp.MustCompile(pattern)
	end := time.Now().Add(timeout)
	for {
		v.mu.Lock()
		loc := re.FindSubmatchIndex(v.buf[v.pos:])
		var m []string
		if loc != nil {
			for i := 0; i < len(loc); i += 2 {
				if loc[i] >= 0 {
					m = append(m, string(v.buf[v.pos+loc[i]:v.pos+loc[i+1]]))
				} else {
					m = append(m, "")
				}
			}
			v.pos += loc[1]
		}
		v.mu.Unlock()
		if m != nil {
			return m
		}
		select {
		case err := <-v.done:
			v.done <- err
			die("test: QEMU stopped (%v) before %q:\n%s", err, pattern, v.tail())
		default:
		}
		if time.Now().After(end) {
			v.cmd.Process.Kill()
			die("test: no %q after %s:\n%s", pattern, timeout, v.tail())
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func (v *vm) send(s string) {
	// Send slowly: the serial console has a small buffer.
	for _, line := range strings.SplitAfter(s, "\n") {
		io.WriteString(v.stdin, line)
		time.Sleep(50 * time.Millisecond)
	}
}

func (v *vm) waitExit(timeout time.Duration) {
	select {
	case err := <-v.done:
		if err != nil {
			die("test: QEMU: %v", err)
		}
	case <-time.After(timeout):
		v.cmd.Process.Kill()
		die("test: QEMU did not stop after %s:\n%s", timeout, v.tail())
	}
}

var shellSeq int

// sh runs a shell command on the serial console and returns its output.
func (v *vm) sh(command string, timeout time.Duration) string {
	shellSeq++
	begin := fmt.Sprintf("@@B%d@@", shellSeq)
	v.send(fmt.Sprintf("echo @@B$((%d))@@; %s; echo \"@@RC=$?@@\"\n", shellSeq, command))
	v.expect(regexp.QuoteMeta(begin), timeout)
	m := v.expect(`(?s)^(.*?)@@RC=([0-9]+)@@`, timeout)
	if m[2] != "0" {
		die("test: %q failed (exit %s):\n%s", command, m[2], m[1])
	}
	return m[1]
}

func (v *vm) unlock() {
	v.expect(`Passphrase for /dev/\S+: `, 3*time.Minute)
	v.send(testPassphrase + "\n")
}

func (v *vm) unlockAndLogin(version string) {
	v.unlock()
	v.login(version)
}

func (v *vm) login(version string) {
	v.expect(`login: `, 3*time.Minute)
	v.send("root\n")
	v.expect(`Password: `, 30*time.Second)
	v.send(testPassword + "\n")
	v.expect(`# `, 30*time.Second)
	out := v.sh("cat /proc/cmdline", 30*time.Second)
	if !strings.Contains(out, "klos.version="+version+" ") {
		die("test: version %s is not running:\n%s", version, out)
	}
	logf("test: version %s runs and the login is good", version)
}

func (v *vm) waitBlessed(version string) {
	out := v.sh("for i in $(seq 60); do [ -s /var/lib/klos/good ] && break; sleep 2; done; cat /var/lib/klos/good", 3*time.Minute)
	if strings.TrimSpace(out) != version {
		die("test: version %s is not blessed: %q", version, out)
	}
	logf("test: version %s is blessed", version)
}

func cmdTest(args []string) {
	if len(args) != 1 {
		die("usage: test <flavor>")
	}
	flavor := args[0]
	tw := filepath.Join(WORK, "test", flavor)
	removeAll(tw)
	mkdirAll(tw)

	// Two versions: the installed version and the update.
	v1 := VERSION
	v2 := VERSION + ".1"
	buildImage(flavor)
	VERSION = v2
	buildImage(flavor)
	update := filepath.Join(outDir(flavor), fmt.Sprintf("klos-%s-%s.tar", flavor, v2))
	VERSION = v1

	hash := strings.TrimSpace(output("openssl", "passwd", "-6", "-salt", "klostest", testPassword))
	conf := filepath.Join(tw, "install.conf")
	writeFile(conf, strings.Join([]string{
		"# Unattended test installation. Do not use these values on a real system.",
		"disk=/dev/vdb",
		"passphrase=" + testPassphrase,
		"hostname=klos-test",
		"root_password_hash=" + hash,
		"poweroff=yes",
	}, "\n")+"\n", 0o600)
	installer := buildInstaller(flavor, conf)

	vars := filepath.Join(tw, "OVMF_VARS.fd")
	copyFile(ovmfVars, vars)
	disk := filepath.Join(tw, "disk.raw")
	f, err := os.Create(disk)
	check(err)
	check(f.Truncate(8 << 30))
	f.Close()

	logf("test 1: install %s %s", flavor, v1)
	vm1 := startVM(qemuArgs(vars, []string{
		"file=" + installer + ",format=raw,if=virtio,readonly=on",
		"file=" + disk + ",format=raw,if=virtio",
	}, "-no-reboot"), filepath.Join(tw, "1-install.log"))
	vm1.expect(`KLOS is installed`, 20*time.Minute)
	vm1.waitExit(2 * time.Minute)
	logf("test 1: the installation is complete")

	logf("test 2: boot the installed system")
	vm2 := startVM(qemuArgs(vars, []string{
		"file=" + disk + ",format=raw,if=virtio",
		"file=" + update + ",format=raw,if=virtio,readonly=on",
	}), filepath.Join(tw, "2-boot.log"))
	vm2.unlockAndLogin(v1)
	vm2.waitBlessed(v1)
	out := vm2.sh("touch /usr/klos-test 2>/dev/null && echo WRITABLE || echo READONLY", 30*time.Second)
	if !strings.Contains(out, "READONLY") {
		die("test: the OS volume is writable")
	}
	vm2.sh("nft list chain inet filter input >/dev/null", 30*time.Second)
	vm2.sh("dmsetup table klos-root | grep -q verity", 30*time.Second)
	vm2.sh("echo test > /etc/klos-test-file && test -f /var/lib/klos/etc/upper/klos-test-file", 30*time.Second)
	logf("test 2: read-only root, dm-verity, firewall and /etc overlay are good")

	logf("test 3: update to %s", v2)
	vm2.send("klos update -reboot /dev/vdb\n")
	vm2.expect(`the next boot is a trial boot of `+regexp.QuoteMeta(v2), 5*time.Minute)
	// klos bless runs when sshd starts, so it can be complete before the
	// login. The console message of /init shows the trial boot.
	vm2.unlock()
	vm2.expect(`klos-init: trial boot of `+regexp.QuoteMeta(v2), 5*time.Minute)
	vm2.login(v2)
	vm2.waitBlessed(v2)
	vm2.sh("test ! -f /run/klos/trial && test -f /etc/klos-test-file", 30*time.Second)
	logf("test 3: the update, the trial boot and the bless are good; /etc changes are kept")

	logf("test 4: roll back to %s", v1)
	vm2.sh("klos rollback", time.Minute)
	vm2.send("reboot\n")
	vm2.unlockAndLogin(v1)
	vm2.waitBlessed(v1)
	vm2.sh("klos status", time.Minute)
	logf("test 4: the rollback is good")
	vm2.send("poweroff\n")
	vm2.waitExit(2 * time.Minute)
	logf("test: all tests passed for %s", flavor)
}

// cmdQemu starts the installed test disk with the console on the terminal.
func cmdQemu(args []string) {
	if len(args) != 1 {
		die("usage: qemu <flavor>")
	}
	tw := filepath.Join(WORK, "test", args[0])
	disk := filepath.Join(tw, "disk.raw")
	if !exists(disk) {
		die("no test disk; run './klos test %s' first", args[0])
	}
	a := qemuArgs(filepath.Join(tw, "OVMF_VARS.fd"), []string{"file=" + disk + ",format=raw,if=virtio"})
	for i, x := range a {
		if x == "stdio" {
			a[i] = "mon:stdio"
		}
	}
	cmd := exec.Command(a[0], a[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	check(cmd.Run())
}
