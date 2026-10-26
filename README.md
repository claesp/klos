# KLOS

KLOS is a Linux distribution for security, predictability and performance.
The KLOS project builds all of the software from source code. LLVM/Clang
compiles all C and C++ code. Each source file comes from the official
upstream project, and the build checks its SHA-256 value and, if upstream
supplies one, its signature.

KLOS has these flavors:

| Flavor       | Use | OS image (compressed) |
|--------------|-----|----------|
| `server`     | A very small server for KVM virtual machines (primary flavor) | approximately 20 MiB |
| `kubernetes` | A Kubernetes node (containerd, crun, kubelet, kubeadm) | approximately 200 MiB |
| `desktop`    | A Wayland desktop (sway, foot, Mesa OpenGL and Vulkan drivers, firmware) | approximately 450 MiB |

## Main properties

- The operating system (OS) is a read-only erofs image with dm-verity.
- The disk has LUKS2 encryption and LVM.
- The EFI binaries have UEFI Secure Boot signatures. The signature covers the
  kernel, the initramfs and the dm-verity root hash of the OS image.
- A TPM 2.0 can unlock the disk (sealed to the Secure Boot state, once in
  each boot). The passphrase stays as a second key.
- SELinux is in enforcing mode, with a small KLOS policy. No process can
  turn it off, and no process can have writable and executable memory
  (except where a flavor needs a JIT compiler).
- An update installs a new OS image next to the current one. The previous
  version stays available. If the new version does not start, the computer
  starts the previous version.
- Manifests in `manifests/` describe each flavor. Lock files record the
  result of each build, so that you can do the same build again.
- A bootstrap distribution (Alpine) supplies only the first compiler. KLOS
  then builds its own build tools, and a builder with only KLOS software
  builds the OS images. A second bootstrap distribution (Arch Linux, with
  GCC and glibc) must give the same images.
- The user space uses musl, libc++, toybox, dinit and static Go tools. The
  installed system has no interpreted languages, except a POSIX shell for
  administration.

## Quick start

The host needs Podman, QEMU support (`/dev/kvm`), curl, gpg and sha256sum.

```sh
./klos bootstrap        # verify Alpine and make the bootstrap builder image
./klos stage2           # build the KLOS builder with the bootstrap builder
./klos image server     # build the packages, the kernel and the OS image
./klos test server      # install in QEMU, boot, update and roll back
```

Read [docs/index.md](docs/index.md) for all documentation, and
[docs/status.md](docs/status.md) for what works now and what is planned.

## License

KLOS is available under the BSD 2-Clause license. Read [LICENSE](LICENSE).
Each package has its own upstream license (see the recipes in `pkgs/`).
