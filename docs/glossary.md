# Glossary

| Term | Meaning |
|------|---------|
| bless | Mark an OS version as good after a trial boot (`klos bless`). |
| bootstrap builder | The Alpine container that builds KLOS. It is not part of KLOS. |
| bundle | A signed tar file with a new OS version (read [updates.md](updates.md)). |
| dm-verity | A Linux device-mapper target that verifies each block with a hash tree. |
| EFI stub | A Linux kernel that the UEFI firmware can start directly. |
| erofs | Enhanced Read-Only File System: a compressed, read-only Linux file system. |
| ESP | EFI system partition: the FAT partition with the EFI binaries. |
| flavor | A variant of KLOS for one use (server, kubernetes, desktop). |
| host package | A package that runs in the builder (for example the toolchain). |
| input hash | A hash of all inputs of a package build (read [build-system.md](build-system.md)). |
| kbuild | The KLOS build tool (`tools/kbuild`). |
| live mode | The installer mode: the OS runs from the installer media. |
| lock file | A file that records the results of a flavor build (`manifests/<flavor>.lock`). |
| LUKS2 | Linux Unified Key Setup, version 2: the disk encryption format. |
| manifest | A file that describes the contents of a flavor (`manifests/<flavor>.toml`). |
| OS volume | The LVM volume `os-<version>` with the OS image of one version. |
| overlay | A Linux file system that puts a writable layer on a read-only layer. |
| recipe | A file that tells kbuild how to build a package (`pkgs/<name>/recipe.toml`). |
| root hash | The top hash of the dm-verity hash tree. It identifies the OS image. |
| sysroot | A directory with the headers and libraries that a build can use. |
| target package | A package that can go into a KLOS image. |
| trial boot | The first boot of a new OS version, before the bless step. |
