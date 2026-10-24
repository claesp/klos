# KLOS documentation

All documentation uses Simplified Technical English (see [style.md](style.md)).

## Concepts

- [overview.md](overview.md): The goals and the main design decisions.
- [flavors.md](flavors.md): The server, Kubernetes and desktop flavors.
- [kubernetes.md](kubernetes.md): The Kubernetes node flavor.
- [desktop.md](desktop.md): The Wayland desktop flavor.
- [security.md](security.md): The security properties and the threat model.
- [secure-boot.md](secure-boot.md): UEFI Secure Boot keys, signatures and enrollment.
- [tpm.md](tpm.md): TPM2 unlock of the disk.
- [glossary.md](glossary.md): The terms that this documentation uses.

## Build

- [building.md](building.md): How to build KLOS.
- [build-system.md](build-system.md): How kbuild builds packages and images.
- [recipes.md](recipes.md): How to write a package recipe.
- [manifests.md](manifests.md): The flavor manifests and the lock files.
- [sources-and-keys.md](sources-and-keys.md): How KLOS verifies the source code.
- [toolchain.md](toolchain.md): The LLVM toolchain and the C library.
- [kernel.md](kernel.md): The kernel configuration.

## Run

- [installing.md](installing.md): How to install KLOS.
- [disk-layout.md](disk-layout.md): Partitions, encryption and volumes.
- [boot.md](boot.md): The boot sequence.
- [filesystem.md](filesystem.md): The read-only OS and the writable paths.
- [updates.md](updates.md): Updates, trial boots and rollback.
- [administration.md](administration.md): Daily administration tasks.

## Development

- [testing.md](testing.md): The QEMU tests.
- [status.md](status.md): What works now and what is planned.
- [style.md](style.md): The rules for documentation and code.
