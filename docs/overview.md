# Overview

## Goals

KLOS is a general-purpose Linux distribution. It has three goals:

1. **Security.** The OS cannot change while it runs. The disk is encrypted.
   The software contains no unnecessary parts.
2. **Predictability.** The same inputs always give the same OS image.
   An update replaces the full OS image. Each computer with the same
   version has the same OS files.
3. **Performance.** The OS is small. The compiler uses modern
   optimizations, and the kernel has only the necessary drivers.

## Main design decisions

| Area | Decision | Reason |
|------|----------|--------|
| Source code | The KLOS project builds all software from upstream source code. | Each file in the OS has a known origin. |
| Verification | Each source has a pinned SHA-256 value. If upstream signs the release, kbuild also checks the signature. | A changed or false source file stops the build. |
| Compiler | LLVM/Clang and lld compile all C and C++ code. | One modern toolchain with good hardening (CFI, stack protection, automatic variable initialization). |
| C library | musl | musl is small, simple and easy to audit. Clang supports it fully. |
| C++ library | libc++, libc++abi and libunwind | There are no GCC run-time libraries. |
| Tools | Go programs for the KLOS tools | Static binaries with memory safety. No interpreter is necessary. |
| Interpreters | None in the installed system, except a POSIX shell | Less attack surface and fewer dependencies. |
| Init | dinit | A small service manager with dependencies. Its services are declarative files, not shell scripts. |
| Boot | The kernel EFI stub. There is no separate boot loader. | Fewer components. The kernel, the initramfs and the command line are one file. |
| OS storage | erofs with dm-verity on an LVM volume | Read-only, compressed and verified for each block. |
| Disk | LUKS2 (argon2id, AES-XTS) with LVM | Full disk encryption. LVM supplies a volume for each OS version. |
| Updates | A new OS volume for each version. UEFI `BootNext` starts a trial boot. | The previous version stays available, and a failed update rolls back automatically. |
| Secure Boot | KLOS keys (PK, KEK, db) sign each EFI binary. | The firmware checks the kernel, the initramfs and the root hash before it starts them. |
| Builds | Podman containers, QEMU for tests | The host needs only a few programs. |

## Components

```
klos                 host script: starts the builder container
builder/             the bootstrap builder (verified Alpine minirootfs)
tools/kbuild/        the build tool (Go)
src/klos/            the KLOS system tool (Go): init, update, install
pkgs/                one recipe for each package
manifests/           the flavor manifests and the lock files
kernel/              the kernel version and the configuration fragments
rootfs/              files that the image builder adds to the OS images
keys/                the upstream signing keys
docs/                this documentation
```

## The bootstrap builder

The build runs in a container. The container has an Alpine Linux root file
system, and the `klos bootstrap` command verifies it with its SHA-256 value
and the OpenPGP signature of Alpine. Alpine supplies only build tools (for
example cmake, make, perl and python) and the first compiler. No file from
Alpine goes into a KLOS image. Read [toolchain.md](toolchain.md).
