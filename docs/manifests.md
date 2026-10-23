# Manifests and lock files

A manifest (`manifests/<name>.toml`) describes a flavor or a part of a
flavor. A manifest with a `[flavor]` table is a flavor. Other manifests
(for example `base.toml`) are parts that flavors include.

The manifest lists all the software that the OS image contains, in
addition to the kernel. The kernel version is in `kernel/linux.toml`.

## Example

```toml
include = ["base"]

[flavor]
name = "server"
description = "Minimal server for KVM virtual machines"
slot-size = "512M"

[kernel]
config = ["kvm-guest", "server"]
cmdline = ["console=tty0", "console=ttyS0,115200"]

[packages]
install = ["htop"]

[services]
enable = ["sshd"]
```

## Keys

| Key | Meaning |
|-----|---------|
| `include` | Manifests to include first. |
| `[flavor] name` | The flavor name. |
| `[flavor] description` | A short description. |
| `[flavor] slot-size` | The size of one OS volume (for example `512M` or `4G`). The OS image must be smaller. |
| `[kernel] config` | The kernel configuration fragments (`kernel/config/<name>.config`). |
| `[kernel] cmdline` | Kernel arguments. kbuild adds the `klos.*` arguments. |
| `[packages] install` | The packages in the image. kbuild adds their runtime dependencies. |
| `[services] enable` | The dinit services that start at boot. |
| `[rootfs] overlays` | Directories in `rootfs/` that kbuild copies into the image. |
| `[rootfs] dirs` | Empty directories to make in the image. |
| `[initramfs] files` | Programs to copy into the initramfs, with their libraries. |
| `[[users]]` | User accounts: `name`, `uid`, `gid`, `home`, `shell`, `gecos`, `groups`. |
| `[[groups]]` | Groups: `name`, `gid`. |

## Include rules

kbuild reads the included manifests first. Then:

- Lists get the new values at the end. A value is not added two times.
- A value in the including manifest replaces a single value.
- `[[users]]` and `[[groups]]` get the new entries at the end.

## Lock files

`./klos lock <flavor>` writes `manifests/<flavor>.lock` from the last image
build. The lock file records:

- the OS version, the dm-verity root hash and the SHA-256 values of the OS
  image and the EFI binaries
- the kernel release and the SHA-256 value of its configuration
- the version and SHA-256 value of each host tool and package, and the
  input hash of each package

Put the lock file in Git with the change that caused it. Then
`./klos verify <flavor>` can build the flavor again and compare the result
with the lock file. If the result is the same, the build is reproducible.
