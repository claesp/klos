# SELinux

KLOS uses SELinux in enforcing mode on all flavors. The policy is a KLOS
policy in CIL (the Common Intermediate Language of SELinux). It is not the
SELinux reference policy.

## Properties

- The policy is small. You can read all of it in `selinux/`.
- The installed system has no SELinux user space (no libselinux, no
  policycoreutils, no Python). The `klos` tool loads the policy and sets
  the labels.
- The OS image contains the labels of its files (erofs extended
  attributes). dm-verity protects the labels as it protects the files.
- After the boot, no process can load a policy, change the mode or change
  a boolean. The root user cannot turn SELinux off.
- No process can make memory that is writable and executable (W^X), except
  on the flavors that need a JIT compiler (see below).
- The network daemons of the base system (dhcpcd and chronyd) are
  confined: they get only the permissions that they need.

## The model

| Domain | Processes | Confined |
|--------|-----------|----------|
| `kernel_t` | The kernel threads, and `/init` before the policy load. | No |
| `init_t` | dinit and all processes that dinit starts, except the processes below. The login shells and the administrator tools are also `init_t`. | No |
| `dhcpcd_t` | dhcpcd and its hook scripts. | Yes |
| `chronyd_t` | chronyd. | Yes |

An unconfined domain (attribute `unconfined_domain`) gets all permissions
on all types, except these permissions:

| Class | Permissions that no process gets |
|-------|----------------------------------|
| `security` | `load_policy`, `setenforce`, `setbool`, `setsecparam`, `setcheckreqprot` |
| `process` | `execmem`, `execstack`, `execheap` |
| `file` | `execmod` |
| `capability2` | `mac_admin`, `mac_override` |

The attribute `execmem_domain` gives `execmem` back. The `kubernetes`
flavor (containers) and the `desktop` flavor (the llvmpipe JIT compiler of
Mesa) put `init_t` in this attribute.

A process changes its domain only when it starts a program with an entry
point type (a domain transition). For example, dinit (`init_t`) starts
`/usr/bin/dhcpcd` (`dhcpcd_exec_t`), and dhcpcd runs as `dhcpcd_t`.

## Files of the policy

| File | Contents |
|------|----------|
| `selinux/base.cil` | The users, roles, types, file system labels, domain transitions and the rules of the confined daemons. |
| `selinux/base.fc` | The file contexts (the labels of files). |
| `selinux/<flavor>.cil`, `selinux/<flavor>.fc` | The additions of a flavor. |

The `[selinux]` table of a manifest selects the files:

```toml
[selinux]
policy = ["base.cil"]
file-contexts = ["base.fc"]
mode = "enforcing"
```

`mode` is `enforcing` (the default) or `permissive`. In permissive mode,
SELinux does not stop operations. It only writes the denials to the kernel
log. Use permissive mode only to develop the policy.

## How kbuild makes the policy

1. kbuild reads the security classes, the permissions and the initial SIDs
   from the kernel source (`security/selinux/include/classmap.h` and
   `initial_sid_to_string.h`). Thus the policy always agrees with the
   kernel.
2. kbuild writes `kernel.cil` with these declarations, the MLS levels
   (`s0` with the categories `c0` to `c1023`), the policy capabilities and
   the rules of `unconfined_domain`.
3. `secilc` (from the SELinux project, a build tool only) compiles
   `kernel.cil` and the CIL files of the manifest. The policy version is
   the highest version that the kernel supports.
4. kbuild adds file context lines for the `/etc` overlay upper layer: each
   `/etc` line also applies to `/var/lib/klos/etc/upper`.
5. kbuild writes the files to `/usr/lib/klos/selinux/` in the image:
   `policy`, `file_contexts` and `config` (the mode). In enforcing mode,
   kbuild adds `enforcing=1` to the kernel command line.
6. kbuild labels each file of the image from the file contexts. The image
   build stops if a file gets no label, or if the file contexts use a type
   that the policy does not declare.

To compile only the policy (for policy development):

```sh
./klos selinux server
```

## The file contexts

Each line has the format `REGEX [TYPE] CONTEXT`:

- `REGEX` must match the full path.
- `TYPE` is one of `--` (regular file), `-d` (directory), `-l` (symbolic
  link), `-c` (character device), `-b` (block device), `-p` (pipe) or `-s`
  (socket). Without `TYPE`, the line applies to all file types.
- `CONTEXT` is a context, for example `system_u:object_r:etc_t:s0`, or
  `<<none>>` (do not set a label).

The last line that matches a file applies. Thus put the general lines
first and the specific lines after them.

## Labels at boot

1. `/init` mounts the OS image and loads the policy of the image. Then it
   labels `/run` and `/dev`. In enforcing mode, the kernel command line has
   `enforcing=1` (it is part of the signed EFI binary), so SELinux enforces
   from the policy load. `/init` checks that the mode of the kernel is the
   mode of the image. If it is not, the computer does not start.
2. `/init` labels all of `/var` if the file contexts of the image are not
   the same as at the last labeling. This occurs at the first boot after
   the installation and after an update that changes the file contexts.
   The file `/var/lib/klos/selinux-contexts` has the SHA-256 value of the
   file contexts of the last labeling.
3. `/init` labels the `/etc` overlay upper layer at each boot. A program
   that replaces a file in `/etc` (for example `/etc/shadow`) can give the
   new file the label of the directory. The next boot repairs the label.
4. `klos boot-setup` labels `/tmp` and the directories of `dirs.d`.

## Commands

```sh
klos selinux status
```

The command shows the mode, the policy version, the context of the
process and the state of the `/var` labels.

```sh
klos selinux relabel /var/lib/myservice
```

The command sets the labels of the files from the file contexts.

To see the denials:

```sh
dmesg | grep 'avc:  denied'
```

## Tests

`./klos test <flavor>` checks the SELinux mode and the domain of dinit,
and that a write to `/sys/fs/selinux/enforce` fails. In enforcing mode, a
denial in the kernel log is an error. The test writes the denials to
`work/<builder>/test/<flavor>/selinux-denials.log`. The kernel command line
has `audit_backlog_limit=8192`, so that the kernel does not lose denials at
boot. If a test command fails, the test shows the last denials.
