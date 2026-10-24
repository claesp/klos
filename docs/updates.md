# Updates and rollback

## Principle

An update does not change the files of the running OS. It installs a new
OS image next to the current image. Then the next boot is a trial boot of
the new version. If the new version does not start correctly, the computer
starts the previous version again.

The `/etc` overlay upper layer and `/var` stay the same. Thus your
configuration and data go with you to the new version (read
[filesystem.md](filesystem.md)).

## The update bundle

An update bundle is a tar file `klos-<flavor>-<version>.tar` with:

| File | Contents |
|------|----------|
| `bundle.json` | The flavor, the version, the slot size, the dm-verity root hash, and the size and SHA-256 value of each file. |
| `bundle.sig` | The Ed25519 signature of `bundle.json` (base64). |
| `root.img` | The erofs OS image with the dm-verity hash tree. |
| `klos.efi` | The kernel with the initramfs and the command line, with a Secure Boot signature. |

The OS image contains the public key `/usr/share/klos/bundle.pub`.
`klos update` accepts only bundles with a valid signature from this key.

## Procedure

1. Install the update:

   ```sh
   klos update https://example.org/klos-server-2026.11.0.tar
   ```

   The source can also be a local tar file, a directory or a block device
   that contains the tar data.

   `klos update` does these steps:
   1. Verify the signature, the sizes and the SHA-256 values.
   2. Make sure that the flavor is the same and the version is newer.
   3. Remove old versions (keep the running version and the default version).
   4. Make the volume `os-<version>` and write the image to it.
   5. Verify the volume with `veritysetup verify`.
   6. Copy `klos.efi` to `\EFI\Linux\klos-<version>.efi` on the ESP.
   7. Make the UEFI boot entry `KLOS <version>` and set `BootNext` to it.

2. Restart the computer (or use `klos update -reboot`).

## The trial boot

The firmware uses `BootNext` one time only. Thus:

1. The firmware starts the new version one time.
2. `/init` sees that the running boot entry (`BootCurrent`) is not the first
   entry in `BootOrder`. This is a trial boot. `/init` starts the watchdog
   timer (300 seconds).
3. When `sshd` runs, the service `klos-bless` runs `klos bless`:
   - It stops the watchdog.
   - It moves the new entry to the start of `BootOrder`.
   - It copies the EFI file to the fallback path `\EFI\BOOT\BOOTX64.EFI`.
   - It removes versions that are older than the previous version.
   - It writes the version to `/var/lib/klos/good`.

If the new version stops before `klos bless`:

- A kernel panic restarts the computer after 10 seconds.
- A hang stops the watchdog pings, and the watchdog restarts the computer.
- A failed `sshd` (or another failure before `klos-bless`) prevents the
  bless step. The watchdog restarts the computer.

After the restart, the firmware uses `BootOrder`, which starts with the
previous version.

## Manual rollback

```sh
klos rollback
reboot
```

`klos rollback` makes the other installed version the first in `BootOrder`.
You can also select the version in the boot menu of the firmware.

## Status

```sh
klos status
```

This command shows the running version, the installed versions, the UEFI
boot entries and whether this is a trial boot.

## Make a bundle

`./klos image <flavor>` makes the bundle in `out/images/<flavor>/<version>/`.
It uses the key `secrets/bundle.key`. If this key does not exist, kbuild
makes a development key. For releases, make the key with `./klos keygen`
on a protected computer, and keep the key offline.

## Without UEFI variables

If the firmware has no UEFI variables (or `efivarfs` is not available),
`klos update` copies the new EFI file to the fallback path. Then there is
no automatic rollback.
