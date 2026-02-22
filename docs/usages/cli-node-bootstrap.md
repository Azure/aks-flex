# AKS Flex Node Bootstrapping

## Overview

This guide shows how to prepare an existing AKS cluster for AKS Flex and manually join nodes from other clouds or local VMs. The steps are:

1. Create (or reuse) an AKS cluster with the proper network setup
2. Apply Kubernetes bootstrap settings from `config k8s-bootstrap` to the cluster
3. Generate cloud-init user data from `config node-bootstrap` for each node to join

> **Note:** This guide is for **advanced and manual usage**. The steps here can be applied to any existing AKS cluster and any VM or bare-metal node that supports [cloud-init](https://cloud-init.io/). For the automated plugin-based workflow, see [Nebius Cloud Integration](cli-plugin-nebius.md) instead.
>
> This guide is also a useful reference if you are **building plugin support for a new cloud provider**. It walks through the same bootstrapping primitives (cluster-side settings and node-side user data) that a plugin automates behind the scenes.

## Setup

### CLI

Install the `aks-flex-cli` binary by following the instructions in [CLI Setup](cli-setup.md).

### Configuration

The following minimal `.env` configuration is required so the CLI can discover the AKS cluster:

```bash
export LOCATION=southcentralus
export AZURE_SUBSCRIPTION_ID=<your-subscription-id>
export RESOURCE_GROUP_NAME=<your-resource-group>
export CLUSTER_NAME=<your-cluster-name>
```

| Variable                 | Description                                  | Default              |
| ------------------------ | -------------------------------------------- | -------------------- |
| `LOCATION`               | Azure region where the cluster is deployed   | *(required)*         |
| `AZURE_SUBSCRIPTION_ID`  | Azure subscription containing the cluster    | auto-detected        |
| `RESOURCE_GROUP_NAME`    | Resource group containing the cluster        | `rg-aks-flex-<user>` |
| `CLUSTER_NAME`           | Name of the AKS managed cluster              | `aks`                |

The CLI uses `az` credentials (`azidentity.NewDefaultAzureCredential`) to connect to the cluster and retrieve its kubeconfig, CA certificate, Kubernetes version, and service CIDR. Make sure you are logged in with `az login`.

## Apply Cluster Bootstrap Settings

Before any external node can join the cluster, the cluster needs RBAC rules, ConfigMaps, and a bootstrap token Secret that the kubeadm join process depends on.

### Generate the bootstrap manifest

```bash
$ aks-flex-cli config k8s-bootstrap > cluster-settings.yaml
```

When the CLI can reach the AKS cluster, it auto-populates all values from the live cluster. If the cluster is not reachable, placeholder values are generated and must be replaced before applying.

The generated YAML contains the following resources:

| Resource                | Namespace      | Name                            | Purpose                                                        |
| ----------------------- | -------------- | ------------------------------- | -------------------------------------------------------------- |
| Role                    | `kube-system`  | `kubeadm:nodes-kubeadm-config`  | Allows bootstrap tokens to read the `kubeadm-config` ConfigMap |
| RoleBinding             | `kube-system`  | `kubeadm:nodes-kubeadm-config`  | Binds the role to the bootstrapper group                       |
| Role                    | `kube-system`  | `kubeadm:kubelet-config`        | Allows bootstrap tokens to read the `kubelet-config` ConfigMap |
| RoleBinding             | `kube-system`  | `kubeadm:kubelet-config`        | Binds the role to the bootstrapper group                       |
| ClusterRole             | *(cluster)*    | `kubeadm:get-nodes`             | Allows bootstrap tokens to get node objects                    |
| ClusterRoleBinding      | *(cluster)*    | `kubeadm:get-nodes`             | Binds the cluster role to the bootstrapper group               |
| ConfigMap               | `kube-public`  | `cluster-info`                  | Contains the cluster CA and API server URL for discovery        |
| ConfigMap               | `kube-system`  | `kubeadm-config`                | Contains `ClusterConfiguration` (Kubernetes version, service subnet) |
| ConfigMap               | `kube-system`  | `kubelet-config`                | Contains baseline `KubeletConfiguration`                       |
| Secret                  | `kube-system`  | `bootstrap-token-<token-id>`    | Bootstrap token for kubeadm TLS bootstrapping                  |

### Apply to the cluster

```bash
$ export KUBECONFIG=./aks.kubeconfig
$ kubectl apply -f cluster-settings.yaml
```

Expected output:

```
role.rbac.authorization.k8s.io/kubeadm:nodes-kubeadm-config created
rolebinding.rbac.authorization.k8s.io/kubeadm:nodes-kubeadm-config created
role.rbac.authorization.k8s.io/kubeadm:kubelet-config created
rolebinding.rbac.authorization.k8s.io/kubeadm:kubelet-config created
clusterrole.rbac.authorization.k8s.io/kubeadm:get-nodes created
clusterrolebinding.rbac.authorization.k8s.io/kubeadm:get-nodes created
configmap/cluster-info created
configmap/kubeadm-config created
configmap/kubelet-config created
secret/bootstrap-token-<token-id> created
```

## Retrieve Node Bootstrap User Data

The `config node-bootstrap` command generates a cloud-init user data script that, when applied to a VM at boot, installs the required packages and joins the node to the AKS cluster via `kubeadm join`.

The command supports multiple cloud targets:

| Target    | Purpose                             |
| --------- | ----------------------------------- |
| `generic` | General-purpose Linux VMs           |
| `aws`     | AWS EC2 instances                   |
| `nebius`  | Nebius Cloud VMs                    |
| `azure`   | Azure VMs                           |

### Generate user data

For most clouds (generic Ubuntu VMs):

```bash
$ aks-flex-cli config node-bootstrap generic > user-data.yaml
```

For Azure VMs (uses the Flex Node agent):

```bash
$ aks-flex-cli config node-bootstrap azure > user-data.yaml
```

### Sample output

The generated output is a [cloud-init](https://cloud-init.io/) YAML document. Here is a representative example for the `generic` target:

```yaml
#cloud-config
apt:
    sources:
        kubernetes:
            source: deb https://pkgs.k8s.io/core:/stable:/v1.33/deb/ /
            keyid: DE15B14486CD377B9E876E1A234654DA9A296436
package_update: true
package_upgrade: true
packages:
    - containerd
    - kubeadm
    - kubelet
write_files:
    - path: /root/.kube/config
      permissions: "0600"
      content: |
        apiVersion: v1
        kind: Config
        clusters:
        - cluster:
            certificate-authority-data: <base64-encoded-ca-cert>
            server: https://<api-server-fqdn>:443
          name: cluster
        contexts:
        - context:
            cluster: cluster
            user: user
          name: context
        current-context: context
        users:
        - name: user
          user:
            token: <bootstrap-token>
    - path: /root/joinconfig
      content: |
        apiVersion: kubeadm.k8s.io/v1beta4
        kind: JoinConfiguration
        discovery:
          file:
            kubeConfigPath: /root/.kube/config
        nodeRegistration:
          kubeletExtraArgs:
          - name: node-labels
            value: aks.azure.com/stretch-managed=true
runcmd:
    - - set
      - -e
    - |-
      mkdir -p /etc/containerd
      containerd config default | sed -e '/SystemdCgroup/ s/false/true/' >/etc/containerd/config.toml
      systemctl restart containerd.service
    - - kubeadm
      - join
      - --config
      - /root/joinconfig
    - - rm
      - -rf
      - /root/joinconfig
      - /root/.kube/config
```

In short, the user data prepares the container runtime (containerd) and kubelet, then uses `kubeadm join` to register the node as a worker in the AKS cluster.

### Sample: joining an Azure VM with the user data

Generate the Azure-flavored user data:

```bash
$ aks-flex-cli config node-bootstrap azure > azure-user-data.yaml
```

Create an Azure VM with the user data as custom data:

```bash
$ az vm create \
    --resource-group <resource-group> \
    --name flex-node-azure \
    --image Ubuntu2404 \
    --size Standard_D2s_v3 \
    --vnet-name vnet \
    --subnet nodes \
    --custom-data azure-user-data.yaml \
    --admin-username azureuser \
    --generate-ssh-keys
```

After the VM boots and cloud-init completes, the node should appear in the cluster:

```bash
$ kubectl get nodes
NAME                                 STATUS   ROLES    AGE   VERSION
aks-system-12345678-vmss000000       Ready    <none>   1h    v1.31.x
aks-system-12345678-vmss000001       Ready    <none>   1h    v1.31.x
flex-node-azure                      Ready    <none>   3m    v1.33.x
```

### Sample: joining a QEMU VM with the user data

Generate the generic user data:

```bash
$ aks-flex-cli config node-bootstrap generic > qemu-user-data.yaml
```

Launch a QEMU VM with cloud-init support. For example, using a Ubuntu cloud image with `cloud-localds`:

> **Tip:** Download the Ubuntu 24.04 cloud image from
> <https://cloud-images.ubuntu.com/releases/24.04/release/ubuntu-24.04-server-cloudimg-amd64.img>.

```bash
# Create a cloud-init seed image
$ cloud-localds seed.img qemu-user-data.yaml

# Launch the VM (adjust paths and resources as needed)
$ qemu-system-x86_64 \
    -m 4096 \
    -smp 2 \
    -drive file=ubuntu-24.04-server-cloudimg-amd64.img,format=qcow2 \
    -drive file=seed.img,format=raw \
    -netdev user,id=net0,hostfwd=tcp::2222-:22 \
    -device virtio-net-pci,netdev=net0 \
    -nographic
```

> **Note:** The VM must have network connectivity to the AKS cluster API server. If the cluster is not publicly accessible, ensure appropriate VPN or tunnel connectivity is in place.

After cloud-init completes, verify the node joined:

```bash
$ kubectl get nodes
NAME                                 STATUS   ROLES    AGE   VERSION
aks-system-12345678-vmss000000       Ready    <none>   1h    v1.31.x
aks-system-12345678-vmss000001       Ready    <none>   1h    v1.31.x
qemu-node                           Ready    <none>   5m    v1.33.x
```

## Under the hood

The node bootstrapping process is built on top of [kubeadm](https://kubernetes.io/docs/reference/setup-tools/kubeadm/), the standard Kubernetes node bootstrapping tool.

### Cluster-side preparation (`k8s-bootstrap`)

The `config k8s-bootstrap` command generates the resources that kubeadm expects to find in the cluster during the join process. These include RBAC rules, ConfigMaps, and a bootstrap token Secret (see the [resource table above](#apply-cluster-bootstrap-settings) for the full list).

The flex CLI prepares a bootstrap token for the node join process. This token is used by `kubeadm join` to authenticate with the API server during the TLS bootstrap phase.

### Node-side bootstrapping (`node-bootstrap`)

When a new VM boots with the generated cloud-init user data, the following steps happen:

```
  New VM (cloud-init)
  │
  ├─ 1. Install packages (containerd, kubeadm, kubelet)
  │
  ├─ 2. Write bootstrap kubeconfig
  │     └─ Contains: CA cert, API server URL, bootstrap token
  │
  ├─ 3. Write kubeadm JoinConfiguration
  │     └─ Contains: discovery path, node labels, node IP (if WireGuard)
  │
  ├─ 4. Configure containerd (systemd cgroup)
  │
  ├─ 5. kubeadm join
  │     ├─ Discovers cluster via bootstrap kubeconfig
  │     ├─ Performs TLS bootstrap (CSR → signed kubelet cert)
  │     ├─ Configures kubelet with cluster settings
  │     └─ Starts kubelet → node registers with API server
  │
  └─ 6. Clean up bootstrap credentials
```

Key settings in the kubeadm `JoinConfiguration`:

| Setting                          | Purpose                                                   |
| -------------------------------- | --------------------------------------------------------- |
| `discovery.file.kubeConfigPath`  | Points to the bootstrap kubeconfig for cluster discovery   |
| `nodeRegistration.kubeletExtraArgs.node-labels` | Applies labels such as `aks.azure.com/stretch-managed=true` |
| `nodeRegistration.kubeletExtraArgs.node-ip`     | Sets the node's InternalIP (used with WireGuard tunnels)   |

> **Note:** The node-side bootstrapping will transition to use the
> `AKSFlexNode` agent in the future, which provides a
> component-based lifecycle manager. The `azure` target already uses this
> agent. Documentation for the AKSFlexNode agent architecture will be
> added once it is stabilized and enabled for all targets.
