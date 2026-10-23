# Start sway when a normal user logs in on tty1.
if [ "$(id -u)" -ge 1000 ] && [ -z "${WAYLAND_DISPLAY:-}" ] && [ "$(tty)" = /dev/tty1 ]; then
	export XDG_RUNTIME_DIR="/run/user/$(id -u)"
	export XDG_SESSION_TYPE=wayland
	export LIBSEAT_BACKEND=seatd
	exec sway
fi
