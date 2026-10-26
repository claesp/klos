# Security

## Properties

| Property | How KLOS gets it |
|----------|------------------|
| Known software | All software comes from verified upstream sources and has a recipe. |
| No change to the OS | The OS image is read-only (erofs) and verified (dm-verity). |
| Encrypted data | LUKS2 encrypts all data except the ESP. |
| Small attack surface | No interpreters (except a shell), no setuid programs, no unnecessary daemons, no kernel modules on servers. |
| Signed updates | Update bundles have an Ed25519 signature. |
| Secure Boot | The EFI binaries have a Secure Boot signature. The signature covers the kernel, the initramfs and the command line with the root hash. |
| TPM2 unlock | The TPM unseals the disk key only for the expected Secure Boot state (PCR 7), and only once in each boot (PCR 15). An optional PIN, with the TPM lockout, adds a second factor. |
| Safe updates | Trial boots with watchdog and automatic rollback. |
| Hardened code | Clang hardening flags, libc++ hardening, kernel CFI and KSPP options. |
| Firewall | nftables drops all incoming traffic except SSH and ICMP. |
| SELinux | Enforcing mode with a small KLOS policy. No process can load a policy or change the mode. No process can use writable and executable memory (except for JIT compilers on the kubernetes and desktop flavors). dhcpcd and chronyd are confined. Read [selinux.md](selinux.md). |
| Diverse builders | A builder with only KLOS software builds the images. Two bootstrap builders with different compilers and C libraries (Alpine with Clang and musl, Arch Linux with GCC and glibc) must give the same images. Read [build-system.md](build-system.md). |
| System call filter | chronyd runs with its seccomp filter (`chronyd -F 1`). The kernel stops chronyd if it uses a system call that is not on its list. |
| SSH | Keys only; modern key exchange (ML-KEM hybrid), ciphers and MACs. OpenSSH has no OpenSSL dependency. |

## No setuid programs

The base system has no setuid or setgid programs. There is no `su` or
`sudo`. Administrators log in as root with an SSH key, or on the console.
To start a program as another user, use `setpriv` (it needs no setuid).

`kbuild image` stops if the image has a setuid or setgid file, or an
executable script that the manifest does not allow. The test also searches
the installed system for setuid and setgid files.

## The kernel

- `lockdown=integrity`: root cannot change the running kernel.
- No `/dev/mem`, no `/dev/port`, no kexec, no hibernation, no 32-bit
  system calls, no `/proc/kcore`.
- The sysctl settings in `/usr/lib/sysctl.d/50-klos.conf` restrict
  `dmesg`, kernel pointers, BPF, io_uring, ptrace and user faults.

## Threat model

KLOS protects against:

- A change to the OS files on the disk (dm-verity stops the read).
- A lost or stolen disk (LUKS2).
- A false or changed source file in the build (SHA-256 and signatures).
- A false update bundle (Ed25519 signature).
- A changed kernel, initramfs or command line on the ESP (Secure Boot, when
  the KLOS keys are enrolled).
- An update that does not start (trial boot and rollback).

KLOS does not yet protect against:

- A person with physical access who replaces the EFI binary on the ESP,
  if the KLOS Secure Boot keys are not enrolled (read
  [secure-boot.md](secure-boot.md)).
- A person with physical access who clears the Secure Boot keys in the
  firmware setup. Use a firmware setup password. With the TPM key, the
  changed Secure Boot state stops the TPM unlock (read [tpm.md](tpm.md)).
- A person with physical access who reads the passphrase (for example
  with a keyboard logger).
- A compromised upstream project that signs a bad release.
- A compromised bootstrap builder. The planned fix is a second, independent
  build that compares the results (see [status.md](status.md)).

## Report a security problem

Send a private report to the maintainers. Do not open a public issue.
