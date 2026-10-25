// SPDX-License-Identifier: BSD-2-Clause

package main

import (
	"errors"
	"os"
	"strings"
	"syscall"
	"unsafe"
)

func ioctlTermios(fd uintptr, req uintptr, t *syscall.Termios) error {
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, fd, req, uintptr(unsafe.Pointer(t))); e != 0 {
		return e
	}
	return nil
}

// consoles returns the terminal devices of the console= kernel arguments.
func consoles() []string {
	var out []string
	for _, c := range strings.Fields(kernelCmdline()["console"]) {
		name, _, _ := strings.Cut(c, ",")
		if name == "tty0" {
			name = "tty1"
		}
		out = append(out, "/dev/"+name)
	}
	if len(out) == 0 {
		out = []string{"/dev/console"}
	}
	return out
}

type ttyLine struct {
	text string
	err  error
}

// readSecret shows a prompt on all consoles and returns the first line that
// a person types on one of them. The terminals do not echo the input.
func readSecret(prompt string) (string, error) {
	type tty struct {
		f   *os.File
		old syscall.Termios
	}
	var ttys []tty
	lines := make(chan ttyLine, 8)
	for _, dev := range consoles() {
		f, err := os.OpenFile(dev, os.O_RDWR|syscall.O_NOCTTY, 0)
		if err != nil {
			continue
		}
		var t syscall.Termios
		if ioctlTermios(f.Fd(), syscall.TCGETS, &t) != nil {
			closeQuietly(f) // not a terminal
			continue
		}
		raw := t
		raw.Lflag &^= syscall.ECHO
		raw.Lflag |= syscall.ICANON
		if ioctlTermios(f.Fd(), syscall.TCSETS, &raw) != nil {
			// Without "no echo", the secret would show; do not use it.
			closeQuietly(f)
			continue
		}
		ttys = append(ttys, tty{f, t})
		_, _ = f.WriteString(prompt) // a console that fails here gets no input
		go func(f *os.File) {
			var b []byte
			one := make([]byte, 1)
			for {
				n, err := f.Read(one)
				if err != nil {
					lines <- ttyLine{err: err}
					return
				}
				if n == 1 {
					if one[0] == '\n' || one[0] == '\r' {
						lines <- ttyLine{text: string(b)}
						return
					}
					b = append(b, one[0])
				}
			}
		}(f)
	}
	if len(ttys) == 0 {
		return "", errors.New("no console for the passphrase prompt")
	}
	var res ttyLine
	for range ttys {
		res = <-lines
		if res.err == nil {
			break
		}
	}
	// Restore the terminals. These steps are best effort: the secret is
	// read already, and a failure only changes the terminal display.
	for _, t := range ttys {
		_ = ioctlTermios(t.f.Fd(), syscall.TCSETS, &t.old)
		_, _ = t.f.WriteString("\n")
		// Closing the file stops the other readers.
		closeQuietly(t.f)
	}
	return res.text, res.err
}

// consoleWrite writes a message to all consoles of the kernel command line,
// for example the screen and the serial port.
func consoleWrite(msg string) {
	wrote := false
	for _, dev := range consoles() {
		f, err := os.OpenFile(dev, os.O_WRONLY|syscall.O_NOCTTY, 0)
		if err != nil {
			continue
		}
		// Messages are best effort: one console with an error must not
		// stop the messages on the other consoles.
		_, _ = f.WriteString(msg)
		closeQuietly(f)
		wrote = true
	}
	if !wrote {
		_, _ = os.Stdout.WriteString(msg)
	}
}
