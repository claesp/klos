# The boot sequence

1. The UEFI firmware starts the boot entry `KLOS <version>`. This is the
   file `\EFI\Linux\klos-<version>.efi` on the EFI system partition (ESP).
   If there is no boot entry, the firmware starts the fallback file
   `\EFI\BOOT\BOOTX64.EFI`, which is a copy of the last good version.
2. The kernel starts `/init` in the initramfs. This is the `klos` program.
3. `/init` mounts `/proc`, `/sys`, `/dev`, `/run` and the UEFI variables.
4. `/init` finds the GPT partition with the name `klos-luks`. It shows a
   passphrase prompt on all consoles (for example the screen and the serial
   port). It unlocks the partition with `cryptsetup`. You have three tries.
5. `/init` activates the LVM volume group `klos`.
6. `/init` opens the volume `os-<version>` with dm-verity. The root hash
   and the hash offset come from the kernel command line.
7. `/init` mounts the erofs OS image on `/` (read-only).
8. `/init` checks the `/var` file system (`e2fsck -p`) and mounts it.
9. `/init` mounts an overlay on `/etc` (read Filesystem).
10. If this is a trial boot, `/init` starts the watchdog (read Updates).
11. `/init` moves the mounts to the new root, removes the initramfs files
    and starts dinit.
12. dinit starts the services of `/etc/dinit.d/boot.d/`:
    - `early-setup` (`klos boot-setup`): mounts, sysctl, host name,
      loopback interface, machine ID, SSH host key
    - `nftables`: the firewall, before the network
    - `dhcpcd`, `chronyd`, `sshd`, `getty-ttyS0`, `getty-tty1`
    - `klos-bless`: marks the version as good when `sshd` runs

If a step in `/init` fails, `/init` shows the error and restarts the computer
after 30 seconds. There is no rescue shell in the initramfs, because a shell
after the unlock step would give access to the decrypted disk.

## Live mode (installer)

The installer uses `klos-live.efi`, with `klos.mode=live` on the command
line. Then `/init` does not unlock a disk. It finds the partition
`klos-install` on the installer media and opens the file
`bundle/root.img` (the same OS image that the installer installs) with a
loop device and dm-verity. `/var` and the `/etc` upper layer are in memory.
The service `klos-installer` starts the installer on the console.

## Kernel command line arguments

| Argument | Meaning |
|----------|---------|
| `klos.version` | The OS version. The OS volume is `os-<version>`. |
| `klos.flavor` | The flavor. |
| `klos.verity` | The dm-verity root hash of the OS image. |
| `klos.hashoffset` | The offset of the hash tree in the OS volume (bytes). |
| `klos.mode` | `installed` or `live`. |
