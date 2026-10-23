# Testing

## The boot test

```sh
./klos test server
```

The test runs QEMU with KVM in the builder container. It uses the OVMF UEFI
firmware and the serial console. It does these steps:

1. Build the image of the current version and of a test update (the
   version with `.1` added).
2. Make an unattended installer with a test configuration.
3. Install KLOS on an empty 8 GB disk. The installer stops the virtual
   machine.
4. Start the installed system. Type the passphrase and log in on the serial
   console.
5. Check the version, the bless step, the read-only root, dm-verity, the
   firewall and the `/etc` overlay.
6. Install the update with `klos update -reboot`. Check the trial boot and
   the bless step. Check that the `/etc` changes are still there.
7. Run `klos rollback` and restart. Check that the first version runs.

The console logs are in `work/test/<flavor>/`. To see the console during the
test, set `KLOS_TEST_VERBOSE=1`.

## Manual test

After `./klos test <flavor>`, you can start the test disk again:

```sh
./klos qemu server
```

The passphrase is `klos-test-passphrase`. The root password is `klos-test`.
Type `Ctrl-A` and then `X` to stop QEMU.

## Reproducibility test

```sh
./klos lock server
./klos verify server
```

`verify` builds all target packages and the image again in a separate
directory, and compares the result with the lock file.
