# Installing KLOS

## Requirements

- A computer or virtual machine with UEFI firmware (no legacy BIOS).
- An x86-64-v2 processor.
- A disk of 8 GB or more for the server flavor.
- For a virtual machine: QEMU/KVM with the q35 machine type and OVMF.

## Make the installer

```sh
./klos installer server
```

The result is `out/images/server/<version>/klos-server-<version>-installer.img`.
Write it to a USB drive, or attach it to a virtual machine as a disk:

```sh
dd if=klos-server-2026.10.0-installer.img of=/dev/sdX bs=4M conv=fsync
```

## Interactive installation

1. Start the computer from the installer.
2. The installer shows the disks. Type the target disk (for example
   `/dev/vda`).
3. Type the disk encryption passphrase two times. Use 8 characters or more.
4. Type a root password, or push Enter for no password login.
5. Type `YES` to delete all data on the disk.
6. When the installer is complete, the computer stops. Remove the installer
   media and start the computer.

## Unattended installation

Make an installation configuration file, for example `install.conf`:

```
disk=/dev/vda
passphrase=a long passphrase
hostname=server1
root_ssh_key=ssh-ed25519 AAAA... admin@example.org
poweroff=yes
```

| Key | Meaning |
|-----|---------|
| `disk` | The target disk. All data on it is deleted. |
| `passphrase` | The disk encryption passphrase. |
| `hostname` | The host name (default `klos`). |
| `root_password_hash` | A crypt(3) SHA-512 hash of the root password. |
| `root_ssh_key` | An `authorized_keys` line for root. |
| `poweroff` | `yes` (default): stop the computer after the installation. |
| `reserved_slots` | Free space for this number of OS versions (default 2). |
| `secureboot` | `enroll`: enroll the KLOS Secure Boot keys if the firmware is in Setup Mode. `no` (default): do not change the keys. Read [secure-boot.md](secure-boot.md). |
| `user`, `user_password_hash`, `user_ssh_key` | A normal user (UID 1000), for example for the desktop. |

Then make an installer with this file:

```sh
./klos installer -config install.conf server
```

**Caution:** the file contains the disk passphrase. Keep the installer image
safe, and do not use the passphrase of a test installation on a real system.

## TPM unlock

After the first boot with Secure Boot, you can let the TPM unlock the disk:

```sh
klos tpm enroll
```

Read [tpm.md](tpm.md).

## After the installation

- Log in with SSH as root (with the key) or on the console (with the password).
- The SSH server accepts only keys. It does not accept passwords.
- Read [administration.md](administration.md).
