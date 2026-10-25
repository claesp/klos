# Status

This document shows what works now and what is planned. "Tested" means that
`./klos test <flavor>` runs the function in QEMU and checks it.

## Works

| Function | State |
|----------|-------|
| Verified bootstrap builder (Alpine minirootfs, SHA-256 and OpenPGP) | Works |
| LLVM 23 cross toolchain, musl 1.2.6, libc++ (hardened) | Works |
| Verified sources (SHA-256 for all, OpenPGP or published values for most) | Works |
| Reproducible packages and images (input hashes, fixed time stamps, sorted archives) | Checked: `./klos verify` gave the same images as the lock files for all three flavors |
| Server flavor: install, LUKS2, LVM, erofs + dm-verity, `/etc` overlay | Tested |
| Signed update bundles, trial boot with watchdog, bless, rollback | Tested |
| Firewall (nftables), SSH (keys only, no OpenSSL), DHCP, NTP | Tested |
| Secure Boot: signed EFI binaries, key enrollment, refusal of unsigned binaries, dbx revocation | Tested (OVMF) |
| TPM2 unlock (PCR 7 and 15, salted sessions, passphrase fallback, new enrollment) | Tested (swtpm) |
| TPM PIN (PolicyAuthValue, TPM lockout protection, wrong PIN refused) | Tested (swtpm) |
| No setuid programs, no scripts except the shell and the dhcpcd hook | Tested (build check and test) |
| Kubernetes flavor: containerd, crun, CNI plugins, kubelet, kubeadm | Tested (preflight, not a full cluster) |
| Desktop flavor: sway, foot, seatd, devd | Tested (headless sway) |
| Vulkan: radv, anv, lavapipe, venus, the Vulkan loader, vulkaninfo | Built; lavapipe tested (vulkaninfo) |
| Desktop firmware (linux-firmware subset, wireless-regdb) | Built; regulatory.db load tested (firmware loading through LoadPin) |
| Mesa with LLVM: iris, crocus, radeonsi, nouveau, virgl, llvmpipe, softpipe | Built; llvmpipe tested (sway with GLES2 on llvmpipe). The hardware drivers are not tested (no hardware in QEMU). |

## Planned

| Function | Notes |
|----------|-------|
| Secure Boot with Microsoft keys | An option to keep the Microsoft certificates in db, for option ROMs. |
| NVK, Rust | The NVIDIA Vulkan driver needs a Rust toolchain. |
| A self-hosted stage | Build the build tools (cmake, python, perl) with KLOS too, so that Alpine is only the first compiler. |
| A second, independent builder | Compare the results of two builders to detect a bad bootstrap builder. |
| Merge of `/etc/passwd` and `/etc/group` after updates | Now, a changed file in the `/etc` upper layer hides new system users of a new version. |
| Signature formats | Check the signed `.sha512` files of pixman and cairo. |
| chrony system call filter | The chrony seccomp filter (`chronyd -F 1`) stops chronyd on musl, because it does not allow all system calls of musl. |
| Sound server, browser | The desktop has no sound server and no web browser yet. |
| Other architectures | Only x86-64 now. |

## Known limits

- The firmware boot menu, not a boot loader, selects the OS version by hand.
- Without UEFI variables (`efivarfs`), an update has no automatic rollback.
- `kubeadm` warns that it cannot find an init system. This is normal on KLOS.
