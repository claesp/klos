# TPM2 unlock

A TPM 2.0 can unlock the LUKS2 disk without the passphrase. The TPM gives
the disk key only to a boot with the expected Secure Boot state, and only
once in each boot.

## How it works

`klos tpm enroll` does these steps:

1. Make a random disk key (256 bits).
2. Seal the key in the TPM, under the storage root key (SRK), with a policy
   for two PCRs of the SHA-256 bank:
   - PCR 7: the Secure Boot state, the keys (PK, KEK, db, dbx) and the
     certificate that verified the EFI binary. All KLOS versions that the
     KLOS db key signed give the same PCR 7 value.
   - PCR 15: zero, which is its value at the start of each boot.
3. Add the key to a new LUKS2 keyslot.
4. Put the sealed object in a LUKS2 token (type `klos-tpm2`) in the disk
   header. The token also records the name of the SRK.

At boot, `/init` does these steps:

1. Read the `klos-tpm2` token from the disk header.
2. Make the SRK and compare its name with the token. A different TPM cannot
   answer.
3. Unseal the key with a policy session that the SRK salts. The session
   encrypts the key between the TPM and the processor, which stops simple
   bus sniffing.
4. Unlock the disk with the key.
5. Extend PCR 15, also after a failure. After this step, the TPM does not
   unseal the key again before the next boot. Thus root on the running
   system cannot get the key from the TPM.

If a step fails, `/init` asks for the passphrase, as before. The passphrase
keyslot stays valid at all times.

## Requirements

- A TPM 2.0 with a SHA-256 PCR bank (for a KVM virtual machine: a virtual
  TPM, for example `swtpm` with the `tpm-crb` device).
- Secure Boot is enabled with the KLOS keys (read
  [secure-boot.md](secure-boot.md)). Without Secure Boot, PCR 7 does not show
  which kernel started, so `klos tpm enroll` stops.

## Enroll

```sh
klos tpm enroll
```

The command asks for the disk passphrase. To give the passphrase in a script,
use `-key-file FILE` (`-` is standard input).

The TPM unlocks the disk from the next boot. Updates do not change PCR 7,
because the same db key signs all KLOS versions.

## TPM PIN

A PIN adds a second factor: the TPM gives the key only for the expected
Secure Boot state **and** the correct PIN. A stolen computer then does not
unlock without the PIN.

```sh
klos tpm enroll -pin
```

The command asks for the passphrase, and then two times for the new PIN.
Use 6 characters or more. In a script, use `-pin-file FILE`.

At boot, `/init` asks for the TPM PIN on all consoles:

- You have three tries.
- An empty PIN skips the TPM, and `/init` asks for the passphrase.
- After three wrong PINs, `/init` asks for the passphrase.

How it works:

- The policy of the sealed key has PolicyAuthValue after PolicyPCR. The auth
  value of the key is a SHA-256 digest of the PIN.
- The PIN goes into the salted session HMAC. It does not go over the bus as
  text.
- A key without a PIN has the attribute "no dictionary attack protection".
  A key with a PIN does not have it. Thus the TPM counts the wrong PINs, and
  after too many wrong PINs it stops all PIN tries for a time (lockout).
  Then `/init` shows "the TPM is in lockout" and asks for the passphrase.
- `klos tpm status` does not try to unseal a key with a PIN, because a wrong
  try counts toward the lockout.

To change or remove the PIN, enroll again (with or without `-pin`).

## When the TPM does not unlock

These changes make a different PCR 7 value. Then the boot asks for the
passphrase:

- A change of db, dbx, KEK or PK (for example `klos secureboot apply`).
- Secure Boot is off in the firmware setup.
- A firmware update that measures Secure Boot in a different way.

After such a change, enroll again (this replaces the old TPM keyslot):

```sh
klos tpm status
klos tpm enroll
```

## Status and removal

```sh
klos tpm status
```

The command shows the keyslot and the PCRs, whether PCR 7 is the same as at
the enrollment, and whether PCR 15 is locked. It also tries to unseal the key
(without showing it): after a normal boot, the TPM must refuse.

To remove the TPM key and its keyslot:

```sh
klos tpm remove
```

## Limits

- An attacker with physical access who can boot a KLOS version with a known
  vulnerability, signed by the same db key, can get the same PCR 7 value.
  Revoke such versions in dbx (read [secure-boot.md](secure-boot.md)). After
  a dbx update, enroll the TPM key again.
- Attacks on the TPM hardware or on the memory of the running computer are
  not in the scope.
