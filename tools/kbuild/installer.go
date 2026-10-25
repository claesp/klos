// SPDX-License-Identifier: BSD-2-Clause

package main

// The installer disk image (for a USB drive or a virtual machine):
//
//	GPT partition 1  "klos-installer-esp"  FAT32, \EFI\BOOT\BOOTX64.EFI is
//	                                       klos-live.efi (live mode)
//	GPT partition 2  "klos-install"        erofs with bundle/ (the update
//	                                       bundle) and an optional install.conf
//
// The live system uses the OS image on the media as its root file system,
// so the installer is the same software that it installs.

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const mib = 1 << 20

func alignMiB(n int64) int64 { return (n + mib - 1) / mib * mib }

func writeAt(dst string, src string, off int64) {
	in, err := os.Open(src)
	check(err)
	defer closeQuietly(in)
	out, err := os.OpenFile(dst, os.O_WRONLY, 0)
	check(err)
	_, err = out.Seek(off, io.SeekStart)
	check(err)
	_, err = io.Copy(out, in)
	check(err)
	check(out.Close())
}

func linkOrCopy(src, dst string) {
	if os.Link(src, dst) != nil {
		copyFile(src, dst)
	}
}

// buildInstaller makes the installer image and returns its path. If conf
// is not empty, the image has that install.conf (unattended install).
func buildInstaller(flavor string, opt imageOptions, conf string) string {
	version := opt.Version
	od := outDir(flavor, version)
	if !exists(filepath.Join(od, "build.json")) {
		buildImage(flavor, opt)
	}
	iw := filepath.Join(WORK, "installer", flavor)
	removeAll(iw)
	media := filepath.Join(iw, "media")
	mkdirAll(filepath.Join(media, "bundle"))
	for _, f := range []string{"bundle.json", "bundle.sig", "root.img", "klos.efi"} {
		linkOrCopy(filepath.Join(od, f), filepath.Join(media, "bundle", f))
	}
	name := fmt.Sprintf("klos-%s-%s-installer.img", flavor, version)
	if conf != "" {
		copyFile(conf, filepath.Join(media, "install.conf"))
		name = fmt.Sprintf("klos-%s-%s-installer-unattended.img", flavor, version)
	}
	payload := filepath.Join(iw, "payload.erofs")
	run(Cmd{Args: []string{"mkfs.erofs", "-T", fmt.Sprint(SDE), "--ignore-mtime", "--all-root",
		"-U", detUUID("media", flavor, version), payload, media}, Quiet: true})

	efi := filepath.Join(od, "klos-live.efi")
	st, err := os.Stat(efi)
	check(err)
	espSize := alignMiB(st.Size()*2 + 8*mib)
	if espSize < 64*mib {
		espSize = 64 * mib
	}
	esp := filepath.Join(iw, "esp.img")
	env := append(os.Environ(), fmt.Sprintf("SOURCE_DATE_EPOCH=%d", SDE), "MTOOLS_SKIP_CHECK=1")
	run(Cmd{Args: []string{"mkfs.fat", "--invariant", "-i", "4B4C4F53", "-n", "KLOS-INST", "-F", "32",
		"-C", esp, fmt.Sprint(espSize / 1024)}, Quiet: true})
	run(Cmd{Args: []string{"mmd", "-i", esp, "::/EFI", "::/EFI/BOOT"}, Env: env})
	run(Cmd{Args: []string{"mcopy", "-i", esp, efi, "::/EFI/BOOT/BOOTX64.EFI"}, Env: env})

	pst, err := os.Stat(payload)
	check(err)
	paySize := alignMiB(pst.Size())
	total := mib + espSize + paySize + mib
	img := filepath.Join(od, name)
	removeIfExists(img)
	f, err := os.Create(img)
	check(err)
	check(f.Truncate(total))
	check(f.Close())
	espStart := int64(mib / 512)
	payStart := espStart + espSize/512
	script := strings.Join([]string{
		"label: gpt",
		"label-id: " + detUUID("disk", flavor, version),
		"first-lba: 2048",
		fmt.Sprintf("start=%d, size=%d, type=C12A7328-F81F-11D2-BA4B-00A0C93EC93B, name=\"klos-installer-esp\", uuid=%s",
			espStart, espSize/512, detUUID("esp", flavor, version)),
		fmt.Sprintf("start=%d, size=%d, type=0FC63DAF-8483-4772-8E79-3D69D8477DE4, name=\"klos-install\", uuid=%s",
			payStart, paySize/512, detUUID("payload", flavor, version)),
	}, "\n") + "\n"
	run(Cmd{Args: []string{"sfdisk", "--quiet", "--no-reread", "--no-tell-kernel", img},
		Stdin: strings.NewReader(script), Quiet: true})
	writeAt(img, esp, espStart*512)
	writeAt(img, payload, payStart*512)
	logf("installer image: %s (%d MiB)", rel(img), total>>20)
	return img
}

func cmdInstaller(args []string) {
	fs := flag.NewFlagSet("installer", flag.ContinueOnError)
	conf := fs.String("config", "", "install.conf for an unattended installation")
	if err := fs.Parse(args); err != nil {
		die("installer: %v", err)
	}
	if fs.NArg() != 1 {
		die("usage: installer [-config install.conf] <flavor>")
	}
	if *conf != "" && !filepath.IsAbs(*conf) {
		*conf = filepath.Join(TOP, *conf)
	}
	buildInstaller(fs.Arg(0), defaultImageOptions(), *conf)
}
