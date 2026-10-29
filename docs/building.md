# Building KLOS

## Host requirements

The host needs these programs and files:

- Podman (rootless Podman is sufficient)
- sha256sum, to check the seed
- `/dev/kvm`, for the QEMU tests (without it, the tests are very slow)
- the seed: the image of the KLOS builder,
  `cache/builder/klos-builder-<hash>.tar`. The hash is the start of the
  SHA-256 value in `builder/seed.conf`. Copy the file from a KLOS release
  or from a computer that has it.

The build uses approximately 40 GB of disk space and 16 GB of memory.

## Procedure

1. Import the KLOS builder:

   ```sh
   ./klos bootstrap
   ```

   The command checks the SHA-256 value of the seed and imports it as the
   container image `localhost/klos-builder:<hash>`.

2. Build a flavor:

   ```sh
   ./klos image server
   ```

   The first build compiles the LLVM toolchain. This takes approximately
   one hour with 8 processor cores. Later builds use the packages that are
   already built.

3. Make an installer image:

   ```sh
   ./klos installer server
   ```

4. Test the flavor in QEMU:

   ```sh
   ./klos test server
   ```

5. (Optional) Build the KLOS builder again and check that it is the same
   as the seed:

   ```sh
   ./klos builder
   ```

## Commands

| Command | Function |
|---------|----------|
| `./klos bootstrap [dir]` | Check the seed and import it as the builder image. With `dir` (an export of `seed-export`), copy the seed from `dir` first. |
| `./klos seed-keygen` | Make the key that signs exported seeds (`secrets/seed.key`, `keys/klos-seed.pub`). |
| `./klos seed-export <dir>` | Write the seed, its minisign signature and its checksum to `dir` (read [maintenance.md](maintenance.md)). |
| `./klos seed-verify <tar>` | Check the signature and the SHA-256 value of a seed tarball. |
| `./klos outdated [package...]` | Show new upstream releases and the known vulnerabilities of the pinned versions. |
| `./klos gc [-n]` | Remove sources, seeds and packages that no recipe uses. |
| `./klos nightly` | Run all checks, and write the logs to `out/nightly/` (read [maintenance.md](maintenance.md)). |
| `./klos builder [-update]` | Build the KLOS builder in the KLOS builder and compare it with the seed. `-update` makes the new builder the seed. |
| `./klos compare -a A -b B <flavor>` | Compare the builds of two builders (`out/A` and `out/B`). |
| `./klos check-sources [package...]` | Check the SHA-256 value and the signature of the sources again (all sources without arguments). |
| `./klos selinux <flavor>` | Compile the SELinux policy of a flavor (read [selinux.md](selinux.md)). |
| `./klos shell` | Open a shell in the builder (no network). |
| `./klos check` | Run gofmt, go vet and the Go unit tests of kbuild and the klos tool. |
| `./klos keygen` | Make a key pair for update bundles in `secrets/`. |
| `./klos fetch <target>...` | Download and verify the sources of packages or flavors. |
| `./klos build [-force] <package>...` | Build packages and their dependencies. |
| `./klos list` | Show all recipes and their build state. |
| `./klos image <flavor>` | Build the OS image, the kernel and the update bundle. |
| `./klos installer [-config FILE] <flavor>` | Build an installer disk image. |
| `./klos lock <flavor>` | Write `manifests/<flavor>.lock` from the last image build. |
| `./klos verify <flavor>` | Build the flavor again and compare it with the lock file. |
| `./klos test <flavor>` | Install, boot, update and roll back the flavor in QEMU. |
| `./klos revoke <klos.efi>...` | Make a signed dbx update that revokes EFI binaries (read [secure-boot.md](secure-boot.md)). |
| `./klos qemu <flavor>` | Start the test disk of a flavor with the console on the terminal. |

## The network

Only the `fetch`, `check-sources` and `outdated` steps use the network. All build steps run
in a container without a network (`podman run --network=none`). The `build`,
`image`, `installer`, `verify` and `test` commands run `fetch` first.

## Output files

| Path | Contents |
|------|----------|
| `cache/sources/` | Downloaded source files. |
| `out/<builder>/pkgs/` | Packages (`<name>.tar.zst`) and their metadata (`<name>.json`). |
| `out/<builder>/images/<flavor>/<version>/` | The OS image, the EFI binaries, the update bundle and the installer. |
| `out/klos/builder/` | The root file system of a new KLOS builder (`./klos builder`). |
| `cache/builder/` | The seed (`klos-builder-<hash>.tar`). |
| `work/<builder>/` | Temporary build directories and the build logs (`work/<builder>/logs/`). |
| `out/nightly/<time>/` | The logs and the summary of `./klos nightly`. |
| `secrets/` | The private keys: update bundles (`bundle.key`), Secure Boot (`secureboot/`), kernel modules (`module-signing.pem`) and exported seeds (`seed.key`). Do not put this in Git. |

## Environment variables

| Variable | Function |
|----------|----------|
| `KLOS_BUILDER` | The name of the output directories (`out/<name>`, `work/<name>`; default `klos`). |
| `KLOS_IMAGE` | A different builder image (for example an older builder). |
| `KLOS_JOBS` | The number of parallel compile jobs (default: all processors). |
| `KLOS_VERSION` | Use a different OS version than `config/klos.toml`. |
| `SOURCE_DATE_EPOCH` | Use a different time stamp than `config/klos.toml`. |
| `KLOS_TEST_VERBOSE` | If set, the test shows the console output of QEMU. |
