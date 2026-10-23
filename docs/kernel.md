# The kernel

## Version and source

`kernel/linux.toml` has the kernel version and the source. KLOS uses a
long-term (LTS) kernel from kernel.org. kbuild checks the SHA-256 value and
the OpenPGP signature of the tarball. Patches in `kernel/patches/*.patch`
are applied in alphabetical order.

## Configuration

kbuild does not start from a large default configuration. It starts from
`allnoconfig` (all options off) and adds the fragments of the flavor
(`kernel/config/<name>.config`):

| Fragment | Contents |
|----------|----------|
| `common` | Options for all flavors: storage stack, file systems, console, network, nftables. |
| `hardening` | Kernel self-protection (CFI, stack protection, memory initialization, lockdown, Landlock, Yama). |
| `kvm-guest` | Drivers for KVM virtual machines (virtio). |
| `server` | No loadable modules. |
| `kubernetes` | Containers, pod networking and netfilter NAT. No loadable modules. |
| `desktop` | Hardware drivers as modules, graphics, sound, input and Wi-Fi. |

After `allnoconfig`, kbuild compares the final `.config` with each line of
the fragments. If a value is different (for example because a dependency
is missing), the build stops. Thus the configuration cannot change silently.

## Compiler

Clang compiles the kernel (`make LLVM=...`) with ThinLTO and kernel
control-flow integrity (kCFI). Host helper programs of the kernel build use
the GCC of the builder; they are not part of the kernel.

## Kernel modules

The `server` and `kubernetes` flavors have no module support
(`CONFIG_MODULES=n`). The kernel image contains all drivers. Nobody can add
code to the running kernel.

The `desktop` flavor needs modules for many hardware types. The modules
are in the read-only, verified OS image.

## The EFI binary

The kernel is an EFI application (`CONFIG_EFI_STUB`). kbuild links the
initramfs and the command line into the kernel image
(`CONFIG_INITRAMFS_SOURCE`, `CONFIG_CMDLINE` with `CONFIG_CMDLINE_OVERRIDE`).
The firmware cannot change the command line. The command line contains the
dm-verity root hash of the OS image. Thus one file (`klos.efi`) identifies
the complete OS version.
