# The desktop flavor

The `desktop` flavor is a Wayland desktop. It uses these parts:

| Part | Package | Function |
|------|---------|----------|
| Compositor | `sway`, `wlroots` | A tiling Wayland compositor, with a bar and a lock tool. |
| Terminal | `foot` | A fast Wayland terminal. |
| Wallpaper | `swaybg` | Shows a background. |
| Graphics | `mesa`, `llvm-target` | OpenGL, OpenGL ES and EGL, with hardware drivers. |
| Input | `libinput`, `libxkbcommon`, `xkeyboard-config` | Keyboards, mice and touchpads. |
| Seats | `seatd` | Gives the session access to the screen and the input devices. |
| Devices | `klos devd`, `kmod` | Loads the kernel modules for the hardware. |
| Fonts | `font-dejavu`, `fontconfig` | The DejaVu fonts. |

There is no udev daemon. `klos devd` loads the modules for the devices, and
`libudev-zero` supplies the libudev API to libinput and wlroots.

## Use the desktop

1. Install the flavor with a normal user. In `install.conf`, set
   `user=<name>` and `user_password_hash=<hash>`.
2. Log in as that user on the first console (tty1). sway starts
   automatically.
3. Push `Super+Enter` to open a terminal. Push `Super+Shift+E` to stop sway.

The sway configuration is in `/etc/sway/config`. Copy it to
`~/.config/sway/config` to change it.

## Graphics drivers

Mesa has these OpenGL drivers:

| Driver | Hardware |
|--------|----------|
| `iris` | Intel graphics, generation 8 (Broadwell) and later |
| `crocus` | Intel graphics, generations 4 to 7 |
| `radeonsi` | AMD graphics (GCN and later) |
| `nouveau` | NVIDIA graphics (the open driver) |
| `virgl` | The virtio GPU of virtual machines (with virgl on the host) |
| `llvmpipe` | A fast software renderer (when there is no GPU) |
| `softpipe` | The reference software renderer |

Mesa also has these Vulkan drivers. The Vulkan loader (`libvulkan.so`)
finds them through the files in `/usr/share/vulkan/icd.d`:

| Driver | Hardware |
|--------|----------|
| `radv` | AMD graphics |
| `anv` | Intel graphics, generation 9 (Skylake) and later |
| `lavapipe` | Software Vulkan (with LLVM) |
| `venus` | Vulkan through the virtio GPU (needs Venus support on the host) |

`vulkaninfo --summary` shows the Vulkan devices. NVK (Vulkan for NVIDIA)
needs Rust, which KLOS does not have yet.

`llvmpipe`, `lavapipe` and `radeonsi` use LLVM at run time (`libLLVM.so` from the
`llvm-target` package). The Intel drivers need `mesa_clc` at build time;
it is not in the image. Read [toolchain.md](toolchain.md).

## Firmware

The packages `linux-firmware` and `wireless-regdb` supply the firmware in
`/usr/lib/firmware`. The full linux-firmware release is about 2 GB. The
package keeps only the firmware of the drivers in the desktop kernel:

| Firmware | Driver | Hardware |
|----------|--------|----------|
| `amdgpu/` | amdgpu | AMD graphics |
| `i915/`, `xe/` | i915, xe | Intel graphics (GuC, HuC, DMC) |
| `nvidia/` | nouveau | NVIDIA graphics (GSP firmware) |
| `intel/iwlwifi/` | iwlwifi | Intel Wi-Fi |
| `intel/ibt-*` | btusb | Intel Bluetooth |
| `ath11k/`, `qca/` | ath11k, btusb | Qualcomm Wi-Fi and Bluetooth |
| `mediatek/*MT7961*`, `mediatek/*MT7922*` | mt7921e, btusb | MediaTek Wi-Fi and Bluetooth |
| `rtl_nic/`, `rtl_bt/` | r8169, btusb | Realtek Ethernet and Bluetooth |
| `regulatory.db` | cfg80211 | The allowed Wi-Fi channels for each country |

The files are zstd-compressed. The kernel loads firmware only from the
verified OS image (LoadPin). The kernel checks the signature of
`regulatory.db` with its built-in keys. To support more devices, add the
driver to `kernel/config/desktop.config` and its firmware to the `keep`
list in `pkgs/linux-firmware/recipe.toml`.

## Kernel modules

The desktop kernel has modules for graphics (Intel, AMD, NVIDIA nouveau,
virtio), sound (HD Audio, USB audio), network (Ethernet, Intel and other
Wi-Fi chips) and USB devices. LoadPin lets the kernel load modules and
firmware only from the verified OS image.

## Limits of this version

- There is no Vulkan driver for NVIDIA (NVK needs Rust).
- The image has firmware only for the drivers in the desktop kernel (see
  Firmware). Other devices that need firmware do not work.
- There is no sound server and no web browser yet.
