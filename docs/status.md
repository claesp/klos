# Status

This document shows what works now and what is planned. "Tested" means that
`./klos test <flavor>` runs the function in QEMU and checks it.

## Works

| Function | State |
|----------|-------|
| Verified bootstrap builder (Alpine minirootfs, SHA-256 and OpenPGP) | Works |
| LLVM 23 cross toolchain, musl 1.2.6, libc++ (hardened) | Works |
| Verified sources (SHA-256 for all, OpenPGP or published values for most) | Works |
| Reproducible packages (input hashes, fixed time stamps, sorted archives) | Works; `verify` command available |
| Server flavor: install, LUKS2, LVM, erofs + dm-verity, `/etc` overlay | Tested |
| Signed update bundles, trial boot with watchdog, bless, rollback | Tested |
| Firewall (nftables), SSH (keys only, no OpenSSL), DHCP, NTP | Tested |
| No setuid programs, no scripts except the shell and the dhcpcd hook | Tested (build check and test) |
| Kubernetes flavor: containerd, crun, CNI plugins, kubelet, kubeadm | Tested (preflight, not a full cluster) |
| Desktop flavor: sway, foot, mesa (softpipe, virgl), seatd, devd | Tested (headless sway) |

## Planned

| Function | Notes |
|----------|-------|
| UEFI Secure Boot | Sign `klos.efi` with a KLOS key. Then the firmware checks the kernel, the initramfs, the command line and thus the root hash. |
| TPM2 unlock | Unlock LUKS2 with a TPM2 policy (with Secure Boot measurements), and keep the passphrase as a second key. |
| LLVM for the target | Necessary for Mesa llvmpipe and the AMD radeonsi driver. |
| Mesa hardware drivers | Intel iris and AMD radeonsi need LLVM or mesa-clc for the target. |
| Firmware for the desktop | The linux-firmware files (kernel.org, signed) for Wi-Fi and graphics. |
| A self-hosted stage | Build the build tools (cmake, python, perl) with KLOS too, so that Alpine is only the first compiler. |
| A second, independent builder | Compare the results of two builders to detect a bad bootstrap builder. |
| Merge of `/etc/passwd` and `/etc/group` after updates | Now, a changed file in the `/etc` upper layer hides new system users of a new version. |
| Signature formats | Check the signed `.sha512` files of pixman and cairo. |
| Sound server, browser | The desktop has no sound server and no web browser yet. |
| Other architectures | Only x86-64 now. |

## Known limits

- The firmware boot menu, not a boot loader, selects the OS version by hand.
- Without UEFI variables (`efivarfs`), an update has no automatic rollback.
- `kubeadm` warns that it cannot find an init system. This is normal on KLOS.
