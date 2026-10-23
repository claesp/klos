# Administration

## Services

dinit manages the services. The service files are in `/etc/dinit.d/`.

| Task | Command |
|------|---------|
| Show the services | `dinitctl list` |
| Stop a service | `dinitctl stop sshd` |
| Start a service | `dinitctl start sshd` |
| Enable a service at boot | `dinitctl enable sshd` |
| Disable a service at boot | `dinitctl disable sshd` |

The service logs are in `/var/log/<service>.log`.

## Network

`dhcpcd` configures all Ethernet interfaces with DHCP and IPv6 router
advertisements. For a static address, edit `/etc/dhcpcd.conf`, for example:

```
interface eth0
static ip_address=192.0.2.10/24
static routers=192.0.2.1
static domain_name_servers=192.0.2.1
```

## Firewall

The firewall is in `/etc/nftables.conf`. The default accepts only SSH, ICMP
and the replies to outgoing connections. After a change, load the file:

```sh
nft -f /etc/nftables.conf
```

## Kernel parameters

Put sysctl settings in `/etc/sysctl.d/<name>.conf`. The defaults are in
`/usr/lib/sysctl.d/50-klos.conf`.

## Disk encryption keys

| Task | Command |
|------|---------|
| Add a passphrase | `cryptsetup luksAddKey /dev/vda2` |
| Remove a passphrase | `cryptsetup luksRemoveKey /dev/vda2` |
| Show the key slots | `cryptsetup luksDump /dev/vda2` |

## Software

You cannot install packages on the running OS. To add software:

1. Add a recipe and put the package in the flavor manifest.
2. Build a new image.
3. Install it with `klos update`.

You can also run software in containers, or put static programs in
`/usr/local/bin` (that is `/var/usrlocal/bin`).
