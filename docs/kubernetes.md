# The Kubernetes flavor

The `kubernetes` flavor is a node for a Kubernetes cluster. It has the
`server` base and these parts:

| Part | Package | Function |
|------|---------|----------|
| Container runtime | `containerd` | The CRI service for kubelet. |
| OCI runtime | `crun` | Starts the containers (written in C). |
| Network plugins | `cni-plugins` | `bridge`, `host-local`, `loopback`, `portmap` and others, in `/usr/lib/cni`. |
| CRI tool | `cri-tools` | `crictl`. |
| Kubernetes | `kubernetes` | `kubelet`, `kubeadm` and `kubectl`. |

The kernel has no modules. It contains the features that containers and
pod networks need: namespaces, cgroups, overlayfs, seccomp, bridge, VXLAN,
Geneve, WireGuard, IPVS and nftables NAT. kube-proxy can use the nftables
mode, so the node does not need the `iptables` program.

## Make a cluster

1. Install the flavor on each node (read [installing.md](installing.md)).
   Give each node a different host name.
2. On the first control plane node, start the cluster:

   ```sh
   kubeadm init --pod-network-cidr=10.244.0.0/16 \
       --cri-socket=unix:///run/containerd/containerd.sock
   ```

   kubeadm writes the kubelet configuration. dinit starts kubelet again
   every 10 seconds, so kubelet starts with the new configuration.
   kubeadm shows the warning "No supported init system detected". You can
   ignore it.
3. Copy the administrator configuration:

   ```sh
   mkdir -p /root/.kube
   cp /etc/kubernetes/admin.conf /root/.kube/config
   ```

4. Install a pod network (CNI) plugin, for example Calico, Cilium or
   Flannel, with the instructions of that project.
5. On each other node, run the `kubeadm join` command that `kubeadm init`
   shows.

## kube-proxy in nftables mode

Set the mode in the kubeadm configuration:

```yaml
apiVersion: kubeproxy.config.k8s.io/v1alpha1
kind: KubeProxyConfiguration
mode: nftables
```

## Configuration files

| File | Contents |
|------|----------|
| `/etc/containerd/config.toml` | containerd with crun and the CNI paths. |
| `/etc/crictl.yaml` | The containerd socket for `crictl`. |
| `/etc/nftables.conf` | The node firewall (API server, etcd, kubelet, NodePorts, overlays). |
| `/usr/lib/sysctl.d/60-kubernetes.conf` | IP forwarding and bridge netfilter. |
| `/etc/default/kubelet` | Optional: `KUBELET_EXTRA_ARGS="..."` for kubelet. |

## Limits

- The cgroup driver is `cgroupfs` (KLOS has no systemd).
- The test (`./klos test kubernetes`) checks containerd, crun, kubelet and
  the kubeadm preflight checks. It does not make a full cluster, because the
  test virtual machine has no Internet access for the container images.
