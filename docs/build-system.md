# The build system

`kbuild` (in `tools/kbuild/`) is the KLOS build tool. It is a Go program.
The `./klos` script compiles it in the builder container and starts it.

## Package builds

kbuild builds each package in these steps:

1. Read the recipe `pkgs/<name>/recipe.toml`.
2. Calculate the input hash (see below). If the package in `out/pkgs/` has
   the same input hash, do not build it again.
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
8. Write the package `out/pkgs/<name>.tar.zst` and its metadata
   `out/pkgs/<name>.json`.

## The input hash

The input hash is a SHA-256 value of all inputs of a build:

- all files in the recipe directory, and the `local-source` directory
- the target, the time stamp and the compiler flags of `config/klos.toml`
- the recipe helper library `tools/recipe-lib.sh`
- the package list of the builder (`/etc/klos-builder.lock`)
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

`./klos verify <flavor>` builds all target packages, the kernel and the
image again, with the same build paths, into `out/verify/`. Then it compares
the result with the lock file. The host tools (llvm, go) are not built
again.

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
