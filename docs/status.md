# Status

This document shows what works now and what is planned. "Tested" means that
`./klos test <flavor>` runs the function in QEMU and checks it.

## Works

| Function | State |
|----------|-------|
| LLVM 23 cross toolchain, musl 1.2.6, libc++ (hardened) | Works |
| Verified sources (SHA-256 for all, OpenPGP or published values for most) | Works |
| Reproducible packages and images (input hashes, fixed time stamps, sorted archives) | Checked: `./klos verify` gave the same images as the lock files for all three flavors |
| Server flavor: install, LUKS2, LVM, erofs + dm-verity, `/etc` overlay | Tested |
| Signed update bundles, trial boot with watchdog, bless, rollback | Tested |
| Merge of new system users and groups into the local `/etc/passwd`, `/etc/group` and `/etc/shadow` after an update | Tested |
| Firewall (nftables), SSH (keys only, no OpenSSL), DHCP, NTP | Tested |
| chronyd system call filter (`chronyd -F 1`, libseccomp) | Tested (with NTP packets from a local server) |
| Secure Boot: signed EFI binaries, key enrollment, refusal of unsigned binaries, dbx revocation | Tested (OVMF) |
| Secure Boot with the Microsoft certificates for option ROMs (`enroll-microsoft`) | Tested (OVMF, desktop flavor) |
| TPM2 unlock (PCR 7 and 15, salted sessions, passphrase fallback, new enrollment) | Tested (swtpm) |
| TPM PIN (PolicyAuthValue, TPM lockout protection, wrong PIN refused) | Tested (swtpm) |
| No setuid programs, no scripts except the shell and the dhcpcd hook | Tested (build check and test) |
| KLOS builds itself: the KLOS builder (only KLOS packages, with QEMU, swtpm and OVMF for the tests) builds the next KLOS builder | Checked: `./klos builder` gives a builder that is byte-identical to the seed (a fixed point) |
| The first builder was checked with two independent bootstrap distributions (Alpine with Clang and musl, Arch Linux with GCC and glibc): the same images, packages and toolchain for all three flavors | Checked (diverse double compiling); the bootstrap distributions are no longer in the repository |
| SELinux in enforcing mode (KLOS policy, labeled erofs image, no mode or policy change after boot, W^X, confined dhcpcd and chronyd) | Tested (no denials in the test) |
| Kubernetes flavor: containerd, crun, CNI plugins, kubelet, kubeadm | Tested (preflight, not a full cluster) |
| Desktop flavor: sway, foot, seatd, devd | Tested (headless sway) |
| Vulkan: radv, anv, lavapipe, venus, the Vulkan loader, vulkaninfo | Built; lavapipe tested (vulkaninfo) |
| Desktop firmware (linux-firmware subset, wireless-regdb) | Built; regulatory.db load tested (firmware loading through LoadPin) |
| Mesa with LLVM: iris, crocus, radeonsi, nouveau, virgl, llvmpipe, softpipe | Built; llvmpipe tested (sway with GLES2 on llvmpipe). The hardware drivers are not tested (no hardware in QEMU). |

## Planned

| Function | Notes |
|----------|-------|
| NVK, Rust | The NVIDIA Vulkan driver needs a Rust toolchain. |
| SELinux domains for more services | sshd, containerd, kubelet and the desktop services are unconfined (`init_t`). |
| Sound server, browser | The desktop has no sound server and no web browser yet. |
| Other architectures | Only x86-64 now. |

## Known limits

- The firmware boot menu, not a boot loader, selects the OS version by hand.
- Without UEFI variables (`efivarfs`), an update has no automatic rollback.
- `kubeadm` warns that it cannot find an init system. This is normal on KLOS.
