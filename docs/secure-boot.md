# Secure Boot

With UEFI Secure Boot, the firmware starts only EFI binaries with a valid
signature. A KLOS EFI binary (`klos.efi`) contains the kernel, the initramfs
and the kernel command line, and the command line contains the dm-verity
root hash of the OS image. Thus the signature covers the complete chain:

1. The firmware checks the signature of `klos.efi` with the db certificate.
2. The kernel and the initramfs are part of the signed file.
3. The initramfs opens the OS volume with the root hash from the signed
   command line. dm-verity checks each block of the OS image.

A person who changes the ESP, the kernel, the initramfs or the OS volume
cannot start the changed system.

## Keys

kbuild makes three key pairs one time, in `secrets/secureboot/`:

| Key | Function |
|-----|----------|
| `PK` | Platform key. It signs the changes to KEK. |
| `KEK` | Key exchange key. It signs the changes to db. |
| `db` | Signature database key. It signs the EFI binaries. |

`secrets/secureboot/owner.guid` is the owner GUID of the signature lists.

kbuild signs `klos.efi` and `klos-live.efi` with the db key (Authenticode,
PKCS #7, SHA-256, RSA 2048). Then it checks each new signature. The
signatures are deterministic, so the images stay reproducible with the
same keys.

**Caution:** a person with the db key can sign EFI binaries that the
computers with the KLOS keys start. Keep the private keys (`*.key`) offline.
Do not put `secrets/` in Git.

To use your own keys, put `PK.key`, `PK.crt`, `KEK.key`, `KEK.crt`,
`db.key`, `db.crt` (PEM, RSA) and `owner.guid` into `secrets/secureboot/`
before the first build.

To build without Secure Boot signatures, set `secure-boot = false` in the
`[image]` table of `config/klos.toml`.

## Enrollment files

Each OS image contains the files for the enrollment of the keys, in
`/usr/share/klos/secureboot/`:

| File | Contents |
|------|----------|
| `db.auth` | The db certificate, as a variable update that KEK signed. |
| `KEK.auth` | The KEK certificate, as a variable update that PK signed. |
| `PK.auth` | The PK certificate, as a variable update that PK signed. |
| `*.crt` | The certificates (PEM). |

## Enroll the keys

The firmware must be in Setup Mode (it has no platform key). On most
computers, the firmware setup has a function to clear the Secure Boot keys.

**Caution:** the KLOS keys replace the keys of the computer manufacturer
and of Microsoft. After the enrollment, the firmware starts only KLOS EFI
binaries. Some graphics cards and network cards have firmware (option ROMs)
that only Microsoft signed. On such a computer, the screen or the network
boot can stop working. Keep a copy of the old keys, or make sure that you
can clear the keys in the firmware setup again.

There are two methods:

- In the installer: put `secureboot=enroll` in `install.conf`. The installer
  enrolls the keys at the end of the installation, if the firmware is in
  Setup Mode.
- On an installed system:

  ```sh
  klos secureboot enroll
  reboot
  ```

`klos secureboot enroll` writes db, then KEK, then PK. When PK is set, the
firmware leaves Setup Mode. Secure Boot is active from the next boot.

## Check the state

```sh
klos secureboot status
```

The command shows one of these states:

| State | Meaning |
|-------|---------|
| `enabled` | The firmware checks the signatures. |
| `disabled (Setup Mode: no platform key)` | You can enroll the keys. |
| `disabled (keys are enrolled; ...)` | Secure Boot is off in the firmware setup. |
| `not supported by the firmware` | The firmware has no Secure Boot. |

`klos status` also shows the state.

## Revoke an EFI binary

If a KLOS version has a vulnerability, revoke its EFI binary in dbx. Then the
firmware does not start it, also if an attacker puts it on the ESP.

1. On the build computer, make a signed dbx update (KEK signs it):

   ```sh
   ./klos revoke out/images/server/2026.10.0/klos.efi
   ```

   The result is `out/secureboot/dbx-update.auth`. It contains the
   Authenticode hash of each EFI binary.

2. Copy the file to each computer and apply it:

   ```sh
   klos secureboot apply -append dbx dbx-update.auth
   ```

**Caution:** do not revoke the running version or the previous version on a
computer. A change of dbx changes PCR 7: after the next boot, enroll the TPM
key again (read [tpm.md](tpm.md)).

## Updates

The EFI binary in each update bundle has a signature from the db key. The
trial boot of an update also works with Secure Boot. If the EFI binary of an
update has no valid signature, the firmware refuses it, and the computer
starts the previous version (see [updates.md](updates.md)).

## Test

`./klos test <flavor>` enrolls the keys with the installer. Then it checks
that Secure Boot is enabled. At the end, it installs a test update whose EFI
binary has no Secure Boot signature (the bundle signature is valid). The
test checks that the firmware refuses the update ("Access Denied") and that
the computer starts the previous version. Then it applies a dbx update that
revokes a test version (read [tpm.md](tpm.md) for the TPM part of the test).
