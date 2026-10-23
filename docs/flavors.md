# Flavors

All flavors share `manifests/base.toml`: the disk layout, the boot
sequence, the update system, dinit, SSH, the firewall and the basic tools.

## server

The primary flavor. It is a very small system for KVM virtual machines.

- The kernel has only the drivers for a KVM guest (virtio, NVMe, the serial
  port and the UEFI frame buffer). It has no module support.
- Services: dhcpcd, chronyd, sshd, getty on the serial port and on tty1.
- Manifest: `manifests/server.toml`.

## kubernetes

A Kubernetes node for KVM virtual machines.

- The same base as `server`, and the kernel options for containers and pod
  networking (bridge, VXLAN, WireGuard, nftables NAT). No modules.
- containerd with crun (an OCI runtime in C), the CNI plugins, crictl,
  kubelet, kubeadm and kubectl.
- kube-proxy uses the nftables mode. The node does not need iptables.
- Manifest: `manifests/kubernetes.toml`. Read [kubernetes.md](kubernetes.md).

## desktop

A Wayland desktop.

- The kernel has modules for hardware (graphics, sound, input, USB,
  Wi-Fi). The modules are in the verified OS image.
- Manifest: `manifests/desktop.toml`. Read [desktop.md](desktop.md).

## Add a flavor

1. Make `manifests/<name>.toml` with `include = ["base"]` and a `[flavor]`
   table.
2. Add kernel fragments in `kernel/config/` if necessary.
3. Add an overlay directory in `rootfs/` if necessary.
4. Build it with `./klos image <name>` and test it with `./klos test <name>`.
