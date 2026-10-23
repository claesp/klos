# Disk layout

The installer makes this layout:

```
GPT disk
├── partition 1 "klos-esp"   EFI system partition, FAT32, 512 MiB
│     \EFI\Linux\klos-<version>.efi    one file for each OS version
│     \EFI\BOOT\BOOTX64.EFI            a copy of the last good version
└── partition 2 "klos-luks"  LUKS2, all other space
      └── LVM volume group "klos"
            ├── os-<version>   erofs OS image + dm-verity hash tree
            ├── os-<version>   the previous version (after an update)
            ├── var            ext4, all writable data
            └── free space     for the next OS versions
```

## Encryption

- LUKS2 with argon2id for the passphrase.
- AES-XTS with a 512-bit key (AES-256).
- The passphrase is the only key. You can add keys with
  `cryptsetup luksAddKey /dev/<disk>2`.

The ESP is not encrypted. It contains only the EFI binaries. Each EFI binary
contains the dm-verity root hash of its OS image. Thus a changed OS volume
does not mount. Note: without Secure Boot, a person with physical access can
replace the EFI binary itself. Read [security.md](security.md).

## Sizes

- Each OS volume has the size `slot-size` from the flavor manifest.
- The installer reserves space for two more OS volumes
  (`reserved_slots` in the install configuration).
- The `var` volume gets all other space.

To make `/var` larger after you add a disk, use the usual LVM commands
(`pvcreate`, `vgextend`, `lvextend`, `resize2fs`).

## Partition names

KLOS finds partitions by their GPT names (`klos-esp`, `klos-luks`,
`klos-install`). It does not use device names such as `/dev/sda`. If two
disks have a partition with the same name, KLOS stops with an error.
