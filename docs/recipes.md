# Package recipes

Each package has a directory `pkgs/<name>/` with the file `recipe.toml`.
The directory can also have patches (`*.patch`) and other files that the
build script uses (`$FILESDIR`).

## Example

```toml
[package]
name = "zlib"
version = "1.3.2"
release = 1
summary = "Deflate compression library"
license = "Zlib"
homepage = "https://zlib.net/"

[[source]]
url = "https://zlib.net/zlib-1.3.2.tar.xz"
sha256 = "d7a0654783a4da529d1bb793b7ad9c3318020af77667bcae35f95d0e42a792f3"
signature = "https://zlib.net/zlib-1.3.2.tar.xz.asc"
key = ["zlib-madler"]

[build]
script = '''
CHOST="$TARGET" ./configure --prefix=/usr --libdir=/usr/lib --shared
kmake
make DESTDIR="$DESTDIR" install
'''
```

## The [package] table

| Key | Meaning |
|-----|---------|
| `name` | The package name. It must be the same as the directory name. |
| `version` | The upstream version. |
| `release` | The KLOS release of this version (default 1). Increase it when you change the recipe but not the version. |
| `summary`, `license`, `homepage` | Information. Use SPDX names for `license`. |
| `kind` | `target` (default): the package is for KLOS. `host`: the package is a build tool that runs in the builder (for example the toolchain). |
| `depends` | Runtime dependencies that kbuild cannot find from the ELF files (for example a program that another program starts). |
| `build-depends` | Target packages in the sysroot for the build. |
| `host-depends` | Host packages for the build. The variable `HOST_<NAME>` is the root directory of each one. |
| `implicit-deps` | `false` removes the default dependencies (see below). |
| `local-source` | A directory in this repository with the source code (for example `src/klos`). |
| `options` | See below. |

Each target package has these default dependencies: the build dependencies
`filesystem`, `linux-headers`, `musl` and `llvm-runtimes`, and the host
dependency `llvm`.

## The [watch] table

The `[watch]` table tells `./klos outdated` where to find new releases.
Read [maintenance.md](maintenance.md).

## The [package.options] table

| Key | Meaning |
|-----|---------|
| `cflags`, `ldflags` | Flags that kbuild adds to the default flags. |
| `keep-docs` | Do not remove `/usr/share/doc` and `/usr/share/info`. |
| `no-strip` | Do not strip the ELF files. |
| `keep-empty-dirs` | Do not remove empty directories. |
| `allow-unresolved` | Do not stop for a library that no package supplies. |
| `run-tools` | Programs of the sysroot (in `/usr/bin`) that the build runs, for example `wayland-scanner`. kbuild makes a wrapper for each one (see below). |

## Programs that a build runs

The target is x86_64 musl, and the builder is also x86_64 musl. Thus a
target program can run in the builder with the KLOS dynamic linker:

- `run-tools` makes a wrapper in `PATH` for a program of the sysroot. An ELF
  program runs with the KLOS dynamic linker and the sysroot libraries. A
  script runs with the interpreter of the builder.
- The Meson cross file has an `exe_wrapper`, so Meson can run the target
  programs that it compiles.
- Build-time C++ helpers use the KLOS toolchain with `-static`
  (`CXX_FOR_BUILD`, `HOST_CXX`).

## The compiler wrapper

The `cc` and `c++` wrappers change the builder paths `-L/usr/lib`, `-L/lib`
and `-I/usr/include` to the sysroot paths. Some build tools (for example the
libtool relink step) add these paths. Without the change, a target library
could link to a library of the builder.

## The [[source]] tables

| Key | Meaning |
|-----|---------|
| `url` | The official download address. |
| `sha256` | The SHA-256 value of the file. It is mandatory. |
| `signature` | The address of the signature file, if upstream signs the release. |
| `signature-type` | `pgp` (default), `minisign`, `pgp-checksums`, `pgp-checksum-file`, `sigstore`, `pypi-attestation` or `git-tag`. Read [sources-and-keys.md](sources-and-keys.md) for each type. |
| `signed-content` | `uncompressed` if the signature is for the `.tar` file in a `.tar.xz` file (kernel.org does this). |
| `checksums` | The address of the checksum list (only with `pgp-checksum-file`). |
| `key` | A list of key names in `keys/` (`keys/<name>.asc`). |
| `filename` | A different file name in the cache. |
| `extract` | `false` copies the file into the source directory and does not extract it. |
| `dest` | Extract into this subdirectory of the source directory. |
| `identity`, `issuer`, `certificate` | The Sigstore signer, its OIDC issuer and the address of the certificate (`sigstore`, `pypi-attestation`). |
| `git`, `tag`, `extra-files`, `missing-files`, `subtrees` | The repository and the signed tag (`git-tag`). |
| `go-modules` | The directories of the source with a `go.mod` file. "fetch" downloads the Go modules (read below). |
| `cargo-lock` | The path of `Cargo.lock` in the source (first source only). "fetch" downloads the Rust crates (read below). |

The first source that is an archive sets the start directory `$S` of the
build script.

## Go modules and Rust crates

Some programs do not include their dependencies (`vendor/`):

- Go (`go-modules`): "fetch" runs `go mod download` with the network.
  Go checks each module with the `go.sum` file of the source and with the
  Go checksum database (sum.golang.org). If the download changes `go.sum`,
  the fetch stops: the source must list all modules. The modules go to
  `cache/gomod/`. The build gets `GOMODCACHE` and `KLOS_GOFLAGS`
  (`-mod=readonly`); set `GOFLAGS="$KLOS_GOFLAGS" GOPROXY=off` after
  `kgoenv`.
- Rust (`cargo-lock`): "fetch" downloads each crates.io package of
  `Cargo.lock` into `cache/crates/` and compares its SHA-256 value with
  `Cargo.lock`. A package from a Git repository is an error. The build
  unpacks the crates into `$S/../cargo-vendor` and writes
  `$CARGO_HOME/config.toml`, so that Cargo uses them without the network.

The SHA-256 value (and the signature) of the source covers `go.sum` and
`Cargo.lock`, so these files cover the dependencies.

A recipe with the host dependency `rust` gets the Rust compiler and Cargo
in `PATH`, `CARGO_HOME`, and `RUSTFLAGS` with the path remapping. For a
target package, Cargo builds for `x86_64-unknown-linux-musl` and links with
the KLOS C library (`-C target-feature=-crt-static`).

## The build script

The script runs in `$S` with `bash -e -u -o pipefail`. It must install the
files into `$DESTDIR`. These variables are available:

| Variable | Value |
|----------|-------|
| `S` | The source directory. |
| `DESTDIR` | The installation directory. |
| `SYSROOT` | The sysroot with the build dependencies. |
| `TARGET` | `x86_64-unknown-linux-musl` |
| `JOBS` | The number of parallel jobs. |
| `FILESDIR` | The recipe directory. |
| `CC`, `CXX`, `AR`, `LD`, ... | The KLOS toolchain (Clang, lld, LLVM tools). |
| `CFLAGS`, `CXXFLAGS`, `LDFLAGS` | The default flags (`config/klos.toml`). |
| `CMAKE_TOOLCHAIN_FILE`, `MESON_CROSS_FILE` | Cross files for CMake and Meson. |
| `HOST_<NAME>` | The root directory of each host dependency. |

These helper functions are available for target packages
(`tools/recipe-lib.sh`): `kconfigure`, `kmake`, `kcmake`, `kmeson`, `kninja`,
`kninja_install` and `kinstall`.

## Rules

- Use only the official release files of the upstream project.
- Add the signature and the key if upstream signs its releases. Read
  [sources-and-keys.md](sources-and-keys.md).
- Disable optional features that KLOS does not use. Each feature is more
  code that can have errors.
- Do not install setuid or setgid programs.
- Write a comment at the top of the recipe if the package is not obvious.
