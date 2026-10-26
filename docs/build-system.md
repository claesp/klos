# The build system

`kbuild` (in `tools/kbuild/`) is the KLOS build tool. It is a Go program.
The `./klos` script compiles it in the builder container and starts it.

## Builders and stages

A builder is a container image that kbuild runs in. Its name is
`<bootstrap>-<stage>`:

| Builder | Contents |
|---------|----------|
| `alpine-1` | The Alpine bootstrap builder. Alpine supplies the build tools and the first compiler (Clang). |
| `arch-1` | The Arch Linux bootstrap builder. Arch supplies the build tools and the first compiler (GCC, with glibc). |
| `alpine-2` | The KLOS builder that `alpine-1` made. It contains only KLOS packages. This is the default builder. |
| `arch-2` | The KLOS builder that `arch-1` made. |

The stages:

1. **Stage 1.** A bootstrap builder compiles the KLOS cross toolchain (the
   package `llvm`) with its own compiler. Then it builds the packages of
   `manifests/builder.toml` with the KLOS toolchain: a native Clang, the
   shell and file tools, Make, CMake, Meson, Perl, Python, the image tools
   and the Go toolchain.
2. `./klos stage2 <bootstrap>` puts these packages together in a root file
   system (`out/<bootstrap>-1/builder/klos-builder.tar`) and imports it as
   the container image of `<bootstrap>-2`.
3. **Stage 2.** The KLOS builder builds the KLOS toolchain again with the
   native Clang of stage 1. Then it builds all packages and images. No
   software of the bootstrap distribution is in this stage.

The lock files (`manifests/<flavor>.lock`) come from `alpine-2`.

Two bootstrap builders that have no common software (Alpine with musl and
Clang, Arch Linux with glibc and GCC) must give the same stage 2 results.
If they do not, one of the bootstrap builders (or its compiler) changed
the KLOS toolchain. This is "diverse double compiling":

```sh
./klos bootstrap arch
./klos stage2 arch
KLOS_BUILDER=arch-2 ./klos image server
./klos compare server
```

`./klos compare <flavor>` compares the image hashes, the kernel and the
package hashes of `alpine-2` and `arch-2`. The toolchain packages
(`[[tool]]` in the lock file) can be different; the command shows these
differences, but they are not an error.

The file `/etc/klos-builder.conf` of each builder tells kbuild the
compilers of the builder:

| Key | Meaning |
|-----|---------|
| `build-triple` | The `--build` triple of configure scripts. It is never the target triple, so configure scripts do cross builds in all stages: they do not run test programs. |
| `host-cc`, `host-cxx` | The compilers of host packages (for example the KLOS toolchain). |
| `build-cc` | The compiler of programs that a target build runs at build time (`CC_FOR_BUILD`). |

The tests run QEMU in the test image (`alpine-1` with QEMU, OVMF and
swtpm). The builder makes the test images first (`kbuild test-images`),
then the test image runs the virtual machines (`kbuild test -prebuilt`).
Only the downloads (`fetch`) always use `alpine-1`, because they need
`gpgv` and `minisign`.

## Package builds

kbuild builds each package in these steps:

1. Read the recipe `pkgs/<name>/recipe.toml`.
2. Calculate the input hash (see below). If the package in
   `out/<builder>/pkgs/` has the same input hash, do not build it again.
3. Make an empty sysroot. Extract the build dependencies into it, with
   their runtime dependencies.
4. Extract the sources. Apply the patches `pkgs/<name>/*.patch` in
   alphabetical order.
5. Run the build script with `bash -e -u -o pipefail`. The script installs
   the files into `$DESTDIR`. The network is not available.
6. Process the files:
   - Move `/bin`, `/sbin`, `/lib` and `/usr/sbin` into the merged `/usr`.
   - Remove documentation, libtool archives and empty directories.
   - Strip the ELF binaries.
7. Find the runtime dependencies. kbuild reads the `DT_NEEDED` entries of
   each ELF file and finds the package in the sysroot that supplies each
   library. A library that no package supplies stops the build.
8. Write the package `out/<builder>/pkgs/<name>.tar.zst` and its metadata
   `out/<builder>/pkgs/<name>.json`.

## The input hash

The input hash is a SHA-256 value of all inputs of a build:

- all files in the recipe directory, and the `local-source` directory
- the target, the time stamp and the compiler flags of `config/klos.toml`
- the recipe helper library `tools/recipe-lib.sh`
- the package list and the configuration of the builder
  (`/etc/klos-builder.lock` and `/etc/klos-builder.conf`)
- the input hashes of all build and host dependencies

Thus a change in a dependency causes a new build of all the packages that
use it. Host packages (the toolchain) do not use the compiler flags or the
helper library. A change to these does not cause a new toolchain build.

## Isolation

- Each build has its own sysroot. The compiler uses only the headers and
  libraries of the declared dependencies.
- The compiler wrappers (`cc`, `c++`, `pkg-config`) always use the KLOS
  target and the sysroot. Build tools from the builder can run, but the
  output does not link to builder libraries.
- The builds run without a network.

## Reproducibility

kbuild removes the usual causes of differences between builds:

- All time stamps are `SOURCE_DATE_EPOCH` (from `config/klos.toml`).
- The tar archives have a sorted order, numeric owner 0 and fixed times.
- The compiler replaces the build directory with `/usr/src/<name>` in debug
  data and in `__FILE__` (`-ffile-prefix-map`).
- The erofs image has a fixed UUID and time stamp, and all files have the
  owner root. The dm-verity salt and UUID come from the flavor and version.
- The initramfs is a cpio archive that kbuild writes itself, with a sorted
  order and fixed values.
- The kernel build has a fixed time stamp, user name and host name.
- All builders have the same build paths: `./klos` mounts the work
  directory of the builder at `/build`. All builder containers have the
  host name `klos`.
- A build cannot see the KLOS Git repository (`GIT_CEILING_DIRECTORIES`),
  and the LLVM packages do not put a Git revision in the version string
  (`LLVM_APPEND_VC_REV=OFF`). Otherwise the compiler would put the KLOS
  commit into each object file.
- Meson runs target programs only through the KLOS dynamic linker
  (`needs_exe_wrapper`), and kbuild gives the compilers of the builder as
  absolute paths. Then a build does not depend on the C library of the
  builder.

`./klos verify <flavor>` builds all target packages, the kernel and the
image again, with the same build paths, into `out/<builder>/verify/`.
Then it compares the result with the lock file. The host tools (llvm, go)
are not built again. The `[[tool]]` entries can be different (for example
on `arch-2`); only the image, the kernel and the packages must be the same.

## Images

`./klos image <flavor>` does these steps:

1. Build all packages of the manifest and their dependencies.
2. Find all runtime dependencies. Make sure that no two packages have the
   same file.
3. Extract the packages into an empty root directory. Development files
   (headers, static libraries, pkg-config files) and documentation are not
   extracted (see `[image] exclude` in `config/klos.toml`).
4. Copy the overlay directories (`rootfs/<name>/`).
5. Write `/etc/passwd`, `/etc/group` and `/etc/shadow` from the manifest,
   the service links in `/etc/dinit.d/boot.d/` and `/usr/lib/os-release`.
6. Compile the kernel. If the flavor has kernel modules, install them.
7. Make the erofs image and add the dm-verity hash tree.
8. Make the initramfs.
9. Link two EFI binaries: `klos.efi` (installed system) and
   `klos-live.efi` (installer).
10. Write and sign `bundle.json`, and make the update bundle
    `klos-<flavor>-<version>.tar`.

Read [updates.md](updates.md) for the bundle format.
