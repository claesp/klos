// SPDX-License-Identifier: BSD-2-Clause

package main

// A reproducible cpio "newc" archive writer for the initramfs. All entries
// belong to root and have the same time stamp. The order is sorted.

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"sort"
)

type cpioEntry struct {
	name   string
	mode   uint32 // includes the file type bits
	data   []byte // file contents or symlink target
	rmajor uint32
	rminor uint32
}

const (
	cpioDir  = 0o040000
	cpioReg  = 0o100000
	cpioLink = 0o120000
	cpioChr  = 0o020000
)

type cpioWriter struct {
	entries map[string]cpioEntry
}

func newCpio() *cpioWriter { return &cpioWriter{entries: map[string]cpioEntry{}} }

func (c *cpioWriter) dir(name string, perm uint32) {
	c.entries[name] = cpioEntry{name: name, mode: cpioDir | perm}
}

func (c *cpioWriter) file(name string, perm uint32, data []byte) {
	c.entries[name] = cpioEntry{name: name, mode: cpioReg | perm, data: data}
}

func (c *cpioWriter) symlink(name, target string) {
	c.entries[name] = cpioEntry{name: name, mode: cpioLink | 0o777, data: []byte(target)}
}

func (c *cpioWriter) chardev(name string, perm, major, minor uint32) {
	c.entries[name] = cpioEntry{name: name, mode: cpioChr | perm, rmajor: major, rminor: minor}
}

func (c *cpioWriter) write(path string) {
	f, err := os.Create(path)
	check(err)
	w := bufio.NewWriter(f)
	var names []string
	for n := range c.entries {
		names = append(names, n)
	}
	sort.Strings(names)
	ino := uint32(1)
	pad := func(n int) {
		if r := n % 4; r != 0 {
			w.Write(make([]byte, 4-r))
		}
	}
	hdr := func(e cpioEntry, ino uint32) {
		nlink := uint32(1)
		if e.mode&0o170000 == cpioDir {
			nlink = 2
		}
		fmt.Fprintf(w, "070701%08X%08X%08X%08X%08X%08X%08X%08X%08X%08X%08X%08X%08X",
			ino, e.mode, 0, 0, nlink, uint32(SDE), len(e.data), 0, 0,
			e.rmajor, e.rminor, len(e.name)+1, 0)
		io.WriteString(w, e.name+"\x00")
		pad(110 + len(e.name) + 1)
		w.Write(e.data)
		pad(len(e.data))
	}
	for _, n := range names {
		hdr(c.entries[n], ino)
		ino++
	}
	hdr(cpioEntry{name: "TRAILER!!!"}, 0)
	check(w.Flush())
	check(f.Close())
}
