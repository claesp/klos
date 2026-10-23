# KLOS

KLOS is a Linux distribution for security, predictability and performance.
The KLOS project builds all of the software from source code. LLVM/Clang
compiles all C and C++ code. Each source file comes from the official
upstream project, and the build checks its SHA-256 value and, if upstream
supplies one, its signature.

KLOS has these flavors:

| Flavor       | Use                                              |
|--------------|--------------------------------------------------|
| `server`     | A very small server for KVM virtual machines (primary flavor) |
| `kubernetes` | A Kubernetes node                                |
| `desktop`    | A Wayland desktop                                |

## Main properties

- The operating system (OS) is a read-only erofs image with dm-verity.
- The disk has LUKS2 encryption and LVM.
- An update installs a new OS image next to the current one. The previous
  version stays available. If the new version does not start, the computer
  starts the previous version.
- Manifests in `manifests/` describe each flavor. Lock files record the
  result of each build, so that you can do the same build again.
- The user space uses musl, libc++, toybox, dinit and static Go tools. The
  installed system has no interpreted languages, except a POSIX shell for
  administration.

## Quick start

The host needs Podman, QEMU support (`/dev/kvm`), curl, gpg and sha256sum.

```sh
./klos bootstrap        # verify Alpine and make the builder image
./klos image server     # build the packages, the kernel and the OS image
./klos test server      # install in QEMU, boot, update and roll back
```

Read [docs/index.md](docs/index.md) for all documentation.

## License

KLOS is available under the BSD 2-Clause license. Read [LICENSE](LICENSE).
Each package has its own upstream license (see the recipes in `pkgs/`).
