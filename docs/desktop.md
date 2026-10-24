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

`llvmpipe` and `radeonsi` use LLVM at run time (`libLLVM.so` from the
`llvm-target` package). The Intel drivers need `mesa_clc` at build time;
it is not in the image. Read [toolchain.md](toolchain.md).

## Kernel modules

The desktop kernel has modules for graphics (Intel, AMD, NVIDIA nouveau,
virtio), sound (HD Audio, USB audio), network (Ethernet, Intel and other
Wi-Fi chips) and USB devices. LoadPin lets the kernel load modules and
firmware only from the verified OS image.

## Limits of this version

- There are no Vulkan drivers yet (Mesa is built with OpenGL drivers only).
- The image has no firmware files yet. AMD graphics cards (amdgpu) and new
  Intel graphics (xe, and i915 for GuC/HuC) need firmware, so these do not
  work yet on real hardware. Many Wi-Fi chips also need firmware.
- There is no sound server and no web browser yet.
