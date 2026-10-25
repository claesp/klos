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
   firewall and the `/etc` overlay. Then run the `[test] commands` of the
   manifests (see below).
6. Install the update with `klos update -reboot`. Check the trial boot and
   the bless step. Check that the `/etc` changes are still there.
7. Run `klos rollback` and restart. Check that the first version runs.
8. Install an update whose EFI binary has no Secure Boot signature. Check
   that the firmware refuses it and that the first version starts again.

9. Apply a dbx update that revokes the test update. The changed PCR 7 must
   stop the TPM unlock, and the passphrase must unlock the disk. Then enroll
   the TPM key again, with a PIN. At the next boot, type a wrong PIN (the
   TPM must refuse it), then the correct PIN, and check the TPM unlock.

The installer of the test enrolls the KLOS Secure Boot keys, so steps 4 to
9 run with Secure Boot enabled. After step 5, the test enrolls a TPM key
(`klos tpm enroll`), and it checks that the TPM unlocks each later boot,
also the trial boot of the update.

The test virtual machine has a software TPM (swtpm). The test image
`localhost/klos-tester` has swtpm; `./klos bootstrap` makes it.

The console logs are in `work/test/<flavor>/`. To see the console during the
test, set `KLOS_TEST_VERBOSE=1`.

## Flavor checks

Each manifest can have test commands (`[test] commands`). The test runs them
as root on the serial console. Each command must exit with 0.

| Manifest | Checks |
|----------|--------|
| `base` | DHCP gives a default route. |
| `server` | sshd, chronyd and dhcpcd run. There is no module support. There are no setuid or setgid files. |
| `kubernetes` | containerd answers `crictl`. crun, kubelet, kubeadm and kubectl run. The kubeadm preflight checks pass (without the image pull). IP forwarding and the bridge and overlay features are present. |
| `desktop` | seatd runs and `klos devd` loaded virtio-net. The test user exists. sway starts with the headless back end, and `swaymsg` gets its version. foot runs, and fontconfig finds the DejaVu fonts. The firmware files are present, and the kernel loads and checks `regulatory.db` when cfg80211 loads. The Vulkan ICD files of radv, anv, lavapipe and venus are present, and `vulkaninfo` finds the lavapipe device. libgallium has the iris, crocus, radeonsi, nouveau, virtio_gpu and llvmpipe drivers. sway starts with the GLES2 renderer on the virtio GPU, and the renderer is llvmpipe (Mesa with LLVM). |

The test installation makes the user `klos` (UID 1000) with the password
`klos-test`.

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
