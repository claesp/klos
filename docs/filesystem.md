# The file system

## Read-only OS

The OS image is an erofs file system on dm-verity. It is read-only. The
kernel verifies each block when it reads it. A changed block gives an I/O
error.

KLOS has a merged `/usr`: `/bin`, `/sbin` and `/lib` are symlinks to
`/usr/bin` and `/usr/lib`. There is no `/usr/sbin`.

## Writable paths

| Path | Storage | Contents |
|------|---------|----------|
| `/var` | the `var` volume (ext4) | All persistent data. |
| `/etc` | overlay: the image `/etc` with an upper layer in `/var/lib/klos/etc/upper` | The local configuration. |
| `/home` | symlink to `/var/home` | User directories. |
| `/root` | symlink to `/var/roothome` | The home directory of root. |
| `/srv` | symlink to `/var/srv` | Data that the system serves. |
| `/usr/local` | symlink to `/var/usrlocal` | Local programs. |
| `/tmp` | tmpfs | Temporary files. The contents go at each boot. |
| `/run` | tmpfs | Run-time data. |

## The /etc overlay

The image contains the default configuration in `/etc`. When you change a
file in `/etc`, the overlay writes the new file to
`/var/lib/klos/etc/upper`. An update replaces the image `/etc`, but your
changes stay.

To see your changes:

```sh
find /var/lib/klos/etc/upper
```

To go back to the default of a file, remove it from the upper layer and
restart the computer:

```sh
rm /var/lib/klos/etc/upper/ssh/sshd_config
```

Note: a changed file stays in the upper layer after an update. If a new
version changes the default of that file, your file stays. Compare the
files after an update.

## Directories at boot

`klos boot-setup` makes the directories in `/usr/lib/klos/dirs.d/*.conf`
and `/etc/klos/dirs.d/*.conf` at each boot. Each line has the format
`d PATH MODE USER GROUP`.
