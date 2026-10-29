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

## The control plane images

The flavor contains the images of the control plane. KLOS builds them from
source (package `kubernetes-images`):

| Image | Contents |
|-------|----------|
| `klos.local/kube-apiserver:v1.37.1` | kube-apiserver |
| `klos.local/kube-controller-manager:v1.37.1` | kube-controller-manager |
| `klos.local/kube-scheduler:v1.37.1` | kube-scheduler |
| `klos.local/etcd:3.7.3-0` | etcd 3.7.3 |
| `klos.local/pause:3.10.2` | pause (the sandbox image of containerd) |

Each image contains only the static program. The service `k8s-images`
imports them into containerd at each start. Thus `kubeadm init` works
without a network. The names start with `klos.local/`, so that nobody
confuses them with the images of registry.k8s.io. KLOS does not build
CoreDNS and kube-proxy: get them from registry.k8s.io, or use a pod network
that replaces kube-proxy.

## Make a cluster

1. Install the flavor on each node (read [installing.md](installing.md)).
   Give each node a different host name.
2. On the first control plane node, write `/root/kubeadm.yaml`:

   ```yaml
   apiVersion: kubeadm.k8s.io/v1beta4
   kind: InitConfiguration
   nodeRegistration:
     criSocket: unix:///run/containerd/containerd.sock
   patches:
     directory: /usr/share/kubernetes/kubeadm-patches
   ---
   apiVersion: kubeadm.k8s.io/v1beta4
   kind: ClusterConfiguration
   kubernetesVersion: v1.37.1
   imageRepository: klos.local
   etcd:
     local:
       imageTag: 3.7.3-0
   networking:
     podSubnet: 10.244.0.0/16
   ---
   apiVersion: kubelet.config.k8s.io/v1beta1
   kind: KubeletConfiguration
   cgroupDriver: cgroupfs
   ```

   The patches give the static pods the SELinux domain of the control
   plane (read [selinux.md](selinux.md)). Without them, etcd and
   kube-apiserver cannot read their keys.
3. Start the cluster:

   ```sh
   kubeadm init --config /root/kubeadm.yaml
   ```

   kubeadm writes the kubelet configuration. dinit starts kubelet again
   every 10 seconds, so kubelet starts with the new configuration.
   kubeadm shows the warning "No supported init system detected". You can
   ignore it.
4. Copy the administrator configuration:

   ```sh
   mkdir -p /root/.kube
   cp /etc/kubernetes/admin.conf /root/.kube/config
   ```

5. Install a pod network (CNI) plugin, for example Calico, Cilium or
   Flannel, with the instructions of that project.
6. On each other node, run the `kubeadm join` command that `kubeadm init`
   shows. On a control plane node, also give `--patches
   /usr/share/kubernetes/kubeadm-patches`.

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
- The test (`./klos test kubernetes`) makes a one-node cluster with
  `kubeadm init` and the images of the flavor, and starts a pod. It uses
  the CNI plugin `bridge`, without CoreDNS and kube-proxy.
