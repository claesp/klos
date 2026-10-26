// SPDX-License-Identifier: BSD-2-Clause

package main

import (
	"path/filepath"
	"strings"
)

type User struct {
	Name   string   `toml:"name"`
	UID    int      `toml:"uid"`
	GID    int      `toml:"gid"`
	Home   string   `toml:"home"`
	Shell  string   `toml:"shell"`
	Groups []string `toml:"groups"`
	Gecos  string   `toml:"gecos"`
}

type Group struct {
	Name string `toml:"name"`
	GID  int    `toml:"gid"`
}

// Manifest describes a flavor, or a part of one that other manifests include.
// Read docs/manifests.md for the format.
type Manifest struct {
	Include []string `toml:"include"`
	Flavor  struct {
		Name        string `toml:"name"`
		Description string `toml:"description"`
		SlotSize    string `toml:"slot-size"`
		VarSize     string `toml:"var-size"`
	} `toml:"flavor"`
	Kernel struct {
		Config  []string `toml:"config"`
		Cmdline []string `toml:"cmdline"`
	} `toml:"kernel"`
	Packages struct {
		Install []string `toml:"install"`
	} `toml:"packages"`
	Services struct {
		Enable []string `toml:"enable"`
	} `toml:"services"`
	Rootfs struct {
		Overlays []string `toml:"overlays"`
		Dirs     []string `toml:"dirs"`
		// Scripts are the executable scripts ("#!") that the image can
		// have. kbuild stops for all other scripts.
		Scripts []string `toml:"scripts"`
		// Exclude are paths that the image does not get, in addition to
		// [image] exclude in config/klos.toml.
		Exclude []string `toml:"exclude"`
		// PackageExclude has "package:pattern" entries: paths of one
		// package that the image does not get.
		PackageExclude []string `toml:"package-exclude"`
	} `toml:"rootfs"`
	Initramfs struct {
		Files []string `toml:"files"`
	} `toml:"initramfs"`
	// Test has shell commands that "klos test" runs on the installed
	// system after the first boot. Each command must exit with 0.
	Test struct {
		Commands []string `toml:"commands"`
		// QemuArgs are more QEMU arguments for the test (for example a GPU).
		QemuArgs []string `toml:"qemu-args"`
		// SecureBoot is the secureboot value of the test install.conf:
		// enroll (the default) or enroll-microsoft.
		SecureBoot string `toml:"secureboot"`
	} `toml:"test"`
	// SELinux has the policy files and the file contexts (in selinux/),
	// and the mode. Read docs/selinux.md.
	SELinux struct {
		Policy       []string `toml:"policy"`
		FileContexts []string `toml:"file-contexts"`
		Mode         string   `toml:"mode"`
	} `toml:"selinux"`
	Users  []User  `toml:"users"`
	Groups []Group `toml:"groups"`
}

func appendUniq(a, b []string) []string {
	for _, x := range b {
		if !contains(a, x) {
			a = append(a, x)
		}
	}
	return a
}

func pick(a, b string) string {
	if b != "" {
		return b
	}
	return a
}

// merge adds the manifest o (which includes m) to m.
func (m *Manifest) merge(o *Manifest) {
	m.Flavor.Name = pick(m.Flavor.Name, o.Flavor.Name)
	m.Flavor.Description = pick(m.Flavor.Description, o.Flavor.Description)
	m.Flavor.SlotSize = pick(m.Flavor.SlotSize, o.Flavor.SlotSize)
	m.Flavor.VarSize = pick(m.Flavor.VarSize, o.Flavor.VarSize)
	m.Kernel.Config = appendUniq(m.Kernel.Config, o.Kernel.Config)
	m.Kernel.Cmdline = appendUniq(m.Kernel.Cmdline, o.Kernel.Cmdline)
	m.Packages.Install = appendUniq(m.Packages.Install, o.Packages.Install)
	m.Services.Enable = appendUniq(m.Services.Enable, o.Services.Enable)
	m.Rootfs.Overlays = appendUniq(m.Rootfs.Overlays, o.Rootfs.Overlays)
	m.Rootfs.Dirs = appendUniq(m.Rootfs.Dirs, o.Rootfs.Dirs)
	m.Rootfs.Scripts = appendUniq(m.Rootfs.Scripts, o.Rootfs.Scripts)
	m.Rootfs.Exclude = appendUniq(m.Rootfs.Exclude, o.Rootfs.Exclude)
	m.Rootfs.PackageExclude = appendUniq(m.Rootfs.PackageExclude, o.Rootfs.PackageExclude)
	m.Initramfs.Files = appendUniq(m.Initramfs.Files, o.Initramfs.Files)
	m.Test.Commands = append(m.Test.Commands, o.Test.Commands...)
	m.Test.QemuArgs = append(m.Test.QemuArgs, o.Test.QemuArgs...)
	if o.Test.SecureBoot != "" {
		m.Test.SecureBoot = o.Test.SecureBoot
	}
	m.SELinux.Policy = appendUniq(m.SELinux.Policy, o.SELinux.Policy)
	m.SELinux.FileContexts = appendUniq(m.SELinux.FileContexts, o.SELinux.FileContexts)
	if o.SELinux.Mode != "" {
		m.SELinux.Mode = o.SELinux.Mode
	}
	m.Users = append(m.Users, o.Users...)
	m.Groups = append(m.Groups, o.Groups...)
}

func loadManifest(name string, chain []string) *Manifest {
	if contains(chain, name) {
		die("manifest include cycle: %s", strings.Join(append(chain, name), " -> "))
	}
	path := filepath.Join(TOP, "manifests", name+".toml")
	if !exists(path) {
		die("no manifest %s", rel(path))
	}
	var data Manifest
	loadTOML(path, &data)
	result := &Manifest{}
	for _, inc := range data.Include {
		result.merge(loadManifest(inc, append(chain, name)))
	}
	result.merge(&data)
	return result
}

func isFlavor(name string) bool {
	path := filepath.Join(TOP, "manifests", name+".toml")
	if !exists(path) {
		return false
	}
	var data Manifest
	loadTOML(path, &data)
	return data.Flavor.Name != ""
}

func flavorManifest(name string) *Manifest {
	if !isFlavor(name) {
		die("%q is not a flavor (manifests/%s.toml has no [flavor] name)", name, name)
	}
	return loadManifest(name, nil)
}

// KernelDef is kernel/linux.toml.
type KernelDef struct {
	Version string   `toml:"version"`
	Source  []Source `toml:"source"`
}

func kernelDef() *KernelDef {
	k := &KernelDef{}
	loadTOML(filepath.Join(TOP, "kernel", "linux.toml"), k)
	return k
}
