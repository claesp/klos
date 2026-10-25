// SPDX-License-Identifier: BSD-2-Clause

package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
)

// Partition is one GPT partition entry.
type Partition struct {
	Disk      string // for example "vda"
	Dev       string // for example "/dev/vda1"
	Number    int
	Name      string
	TypeGUID  [16]byte
	PartGUID  [16]byte // in the on-disk (EFI_GUID) byte order
	FirstLBA  uint64
	LastLBA   uint64
	BlockSize uint64
}

func readSysInt(path string) (uint64, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.ParseUint(strings.TrimSpace(string(b)), 10, 64)
}

// readGPT reads the partition table of a whole disk such as "vda".
func readGPT(disk string) ([]Partition, error) {
	bs, err := readSysInt("/sys/block/" + disk + "/queue/logical_block_size")
	if err != nil {
		return nil, err
	}
	f, err := os.Open("/dev/" + disk)
	if err != nil {
		return nil, err
	}
	defer closeQuietly(f)
	hdr := make([]byte, 92)
	if _, err := f.ReadAt(hdr, int64(bs)); err != nil {
		return nil, err
	}
	if string(hdr[:8]) != "EFI PART" {
		return nil, errors.New("no GPT")
	}
	entLBA := binary.LittleEndian.Uint64(hdr[72:])
	count := binary.LittleEndian.Uint32(hdr[80:])
	size := binary.LittleEndian.Uint32(hdr[84:])
	if count > 1024 || size < 128 || size > 4096 {
		return nil, errors.New("bad GPT header")
	}
	buf := make([]byte, int(count)*int(size))
	if _, err := f.ReadAt(buf, int64(entLBA*bs)); err != nil {
		return nil, err
	}
	var parts []Partition
	for i := 0; i < int(count); i++ {
		e := buf[i*int(size) : (i+1)*int(size)]
		p := Partition{Disk: disk, Number: i + 1, BlockSize: bs}
		copy(p.TypeGUID[:], e[0:16])
		if p.TypeGUID == ([16]byte{}) {
			continue
		}
		copy(p.PartGUID[:], e[16:32])
		p.FirstLBA = binary.LittleEndian.Uint64(e[32:])
		p.LastLBA = binary.LittleEndian.Uint64(e[40:])
		u := make([]uint16, 36)
		for j := range u {
			u[j] = binary.LittleEndian.Uint16(e[56+2*j:])
		}
		n := 0
		for n < len(u) && u[n] != 0 {
			n++
		}
		p.Name = string(utf16.Decode(u[:n]))
		p.Dev = partDev(disk, p.Number)
		parts = append(parts, p)
	}
	return parts, nil
}

// partDev finds the device node of partition number n of a disk with sysfs.
func partDev(disk string, n int) string {
	ents, _ := filepath.Glob("/sys/block/" + disk + "/" + disk + "*/partition")
	for _, e := range ents {
		if v, err := readSysInt(e); err == nil && int(v) == n {
			return "/dev/" + filepath.Base(filepath.Dir(e))
		}
	}
	return ""
}

// disks returns the whole disks that can have a partition table.
func disks() []string {
	ents, _ := os.ReadDir("/sys/block")
	var out []string
	for _, e := range ents {
		n := e.Name()
		if strings.HasPrefix(n, "ram") || strings.HasPrefix(n, "dm-") ||
			strings.HasPrefix(n, "zram") {
			continue
		}
		if sz, err := readSysInt("/sys/block/" + n + "/size"); err != nil || sz == 0 {
			continue
		}
		out = append(out, n)
	}
	return out
}

// findPartition returns the partition with a GPT name. If more than one
// disk has the name, it is an error: KLOS does not guess.
func findPartition(name string) (*Partition, error) {
	var found []Partition
	for _, d := range disks() {
		parts, err := readGPT(d)
		if err != nil {
			continue
		}
		for _, p := range parts {
			if p.Name == name && p.Dev != "" {
				found = append(found, p)
			}
		}
	}
	switch len(found) {
	case 0:
		return nil, fmt.Errorf("no partition with the name %q", name)
	case 1:
		return &found[0], nil
	}
	return nil, fmt.Errorf("more than one partition with the name %q", name)
}

// waitPartition waits until a partition with the name is available.
func waitPartition(name string, timeout time.Duration) (*Partition, error) {
	end := time.Now().Add(timeout)
	for {
		p, err := findPartition(name)
		if err == nil {
			if _, serr := os.Stat(p.Dev); serr == nil {
				return p, nil
			}
		}
		if time.Now().After(end) {
			if err == nil {
				err = fmt.Errorf("device %s does not exist", p.Dev)
			}
			return nil, err
		}
		time.Sleep(250 * time.Millisecond)
	}
}
