# The desktop flavor

The `desktop` flavor is a Wayland desktop. It uses these parts:

| Part | Package | Function |
|------|---------|----------|
| Compositor | `sway`, `wlroots` | A tiling Wayland compositor, with a bar and a lock tool. |
| Terminal | `foot` | A fast Wayland terminal. |
| Wallpaper | `swaybg` | Shows a background. |
| Graphics | `mesa` | OpenGL ES and EGL. |
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

## Kernel modules

The desktop kernel has modules for graphics (Intel, AMD, NVIDIA nouveau,
virtio), sound (HD Audio, USB audio), network (Ethernet, Intel and other
Wi-Fi chips) and USB devices. LoadPin lets the kernel load modules and
firmware only from the verified OS image.

## Limits of this version

- Mesa has only the virtio GPU driver (virgl) and the software renderer
  (softpipe). The hardware drivers (Intel iris, AMD radeonsi) and llvmpipe
  need LLVM for the target. Read [status.md](status.md).
- The image has no firmware files yet, so some Wi-Fi and graphics hardware
  does not work.
- There is no sound server and no web browser yet.
