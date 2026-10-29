# Maintenance

This document tells how to keep KLOS current and how to keep the seed safe.

## New releases and vulnerabilities

`./klos outdated` looks for new upstream releases of all recipes and of
the kernel. It shows the recipes that have a newer release:

- "SAME SERIES" is the newest release with the same first two numbers
  (for example 6.18.59 for 6.18.58). Such a release usually has only
  fixes.
- "NEWEST" is the newest release.

kbuild finds the releases in the Git tags of the project (Git protocol
version 2, the same as `git ls-remote`) or in the download directory. The
`[watch]` table of a recipe can change this:

| Key | Function |
|-----|----------|
| `url` | A page or a Git repository (ending in `.git`) with the releases. |
| `git` | `true` if `url` is a Git repository without `.git` at the end. |
| `regex` | A regular expression that finds the versions (group 1 is the version). |
| `ignore` | A regular expression of versions that are not releases (for example the development series of Perl). |
| `disabled` | A reason: kbuild does not look for releases. |
| `osv` | Packages in the OSV.dev database (`"ecosystem:name"`) with the version of the recipe. |

The `[watch]` table is not part of the input hash: a change of it does
not cause a new build.

Then `./klos outdated` asks OSV.dev for the known vulnerabilities of these
package versions:

- Go: the main module and the modules in `vendor/modules.txt` of each Go
  program, and the Go standard library (`stdlib`) of the Go toolchain.
- Rust: the crates in `Cargo.lock` of each Rust program.
- Python: the packages from PyPI.

A vulnerability in a Go or Rust module applies only if the program uses
the affected code. Examine the report of the vulnerability, then update
the recipe if necessary. `./klos outdated -fail` exits with status 1 if
it finds a vulnerability. `-no-osv` does not ask OSV.dev.

OSV.dev has no good data for C libraries that come directly from
upstream. For these, read the security announcements of the projects,
and use the "SAME SERIES" column.

## The nightly check

`./klos nightly` runs all checks, one after the other:

1. `./klos check` (the Go checks).
2. `./klos check-sources` (the SHA-256 value and the signature of each
   source, again from the upstream servers).
3. `./klos outdated -fail`.
4. `./klos builder` (the builder must build itself byte-identically).
5. `./klos verify <flavor>` and `./klos test <flavor>` for each flavor.

The logs and a summary go to `out/nightly/<time>/`. The exit status is 1
if a step failed. A run takes some hours. Only one run can be active.

To run it each night, use a systemd user timer. The files are in
`tools/nightly/`:

```sh
mkdir -p ~/.config/systemd/user
cp tools/nightly/klos-nightly.service tools/nightly/klos-nightly.timer ~/.config/systemd/user/
systemctl --user daemon-reload
systemctl --user enable --now klos-nightly.timer
loginctl enable-linger "$USER"
```

Change `WorkingDirectory` in the service file to the directory of the
repository. `loginctl enable-linger` lets the timer run when you are not
logged in.

## The seed backup

The seed (`cache/builder/klos-builder-<hash>.tar`) is the only start of a
KLOS build. Git does not contain it (it is about 1.5 GB). Keep copies of it
in more than one place, for example on an offline disk and with a release.

1. Make the seed key one time: `./klos seed-keygen`. Commit
   `keys/klos-seed.pub`. Keep a copy of `secrets/seed.key` offline.
2. Export the seed: `./klos seed-export <dir>`. The directory gets the
   tarball, its minisign signature (`.minisig`), `SHA256SUMS`, `seed.conf`
   and the public key.
3. Copy the directory to the backup location.

To use a copy on a new computer:

1. Clone the repository.
2. Run `./klos bootstrap <dir>`. It checks the SHA-256 value of the seed
   with `builder/seed.conf` and imports it.
3. Optional: run `./klos seed-verify <dir>/klos-builder-<hash>.tar`.

Anyone can check the signature without KLOS:
`minisign -Vm klos-builder-<hash>.tar -p klos-seed.pub`.

The SHA-256 value in `builder/seed.conf` (in Git) is the trust anchor.
The signature lets a person check a seed before they have the
repository, for example a seed of a release.

## Cleanup

`./klos gc` removes these files:

- Sources in `cache/sources/` that no recipe uses (for example the old
  version after an update).
- Builder tarballs in `cache/builder/` that are not the seed.
- Packages, host tool directories and logs of packages that have no
  recipe.

`./klos gc -n` only shows the files.
