# Security

## Properties

| Property | How KLOS gets it |
|----------|------------------|
| Known software | All software comes from verified upstream sources and has a recipe. |
| No change to the OS | The OS image is read-only (erofs) and verified (dm-verity). |
| Encrypted data | LUKS2 encrypts all data except the ESP. |
| Small attack surface | No interpreters (except a shell), no setuid programs, no unnecessary daemons, no kernel modules on servers. |
| Signed updates | Update bundles have an Ed25519 signature. |
| Safe updates | Trial boots with watchdog and automatic rollback. |
| Hardened code | Clang hardening flags, libc++ hardening, kernel CFI and KSPP options. |
| Firewall | nftables drops all incoming traffic except SSH and ICMP. |
| SSH | Keys only; modern key exchange (ML-KEM hybrid), ciphers and MACs. OpenSSH has no OpenSSL dependency. |

## No setuid programs

The base system has no setuid or setgid programs. There is no `su` or
`sudo`. Administrators log in as root with an SSH key, or on the console.

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
- An update that does not start (trial boot and rollback).

KLOS does not yet protect against:

- A person with physical access who replaces the EFI binary on the ESP. The
  fix is UEFI Secure Boot with signed EFI binaries (planned, see
  [status.md](status.md)).
- A person with physical access who reads the passphrase (for example
  with a keyboard logger).
- A compromised upstream project that signs a bad release.
- A compromised bootstrap builder. The planned fix is a second, independent
  build that compares the results (see [status.md](status.md)).

## Report a security problem

Send a private report to the maintainers. Do not open a public issue.
