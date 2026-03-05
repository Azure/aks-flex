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

The command supports two bootstrap targets:

| Target   | Purpose                                                                                              |
| -------- | ---------------------------------------------------------------------------------------------------- |
| `flex`   | **(Recommended)** Uses the `aks-flex-node` agent to bootstrap the node with a component-based lifecycle manager |
| `ubuntu` | Traditional bootstrap using APT packages and direct `kubeadm join`                                    |

### Flags

| Flag        | Type     | Default      | Description                                                                 |
| ----------- | -------- | ------------ | --------------------------------------------------------------------------- |
| `--gpu`     | `bool`   | `false`      | Indicates whether the node has a GPU. Affects the generated userdata (flex only). |
| `--variant` | `string` | `cloud-init` | Output variant. `cloud-init` produces cloud-init YAML user data; `script` produces an equivalent standalone bash script. |

### Generate user data

Using the recommended `flex` target:

```bash
$ aks-flex-cli config node-bootstrap flex > user-data.yaml
```

For GPU nodes:

```bash
$ aks-flex-cli config node-bootstrap flex --gpu > user-data.yaml
```

Using the `ubuntu` target (traditional `kubeadm join`):

```bash
$ aks-flex-cli config node-bootstrap ubuntu > user-data.yaml
```

#### Standalone bash script variant

By default the command outputs cloud-init YAML, which requires a cloud-init-enabled environment (most cloud VMs). For nodes that do not have cloud-init — such as bare-metal servers or pre-provisioned VMs — use `--variant script` to generate an equivalent standalone bash script that can be executed directly:

```bash
$ aks-flex-cli config node-bootstrap flex --variant script > bootstrap.sh
```

```bash
$ aks-flex-cli config node-bootstrap ubuntu --variant script > bootstrap.sh
```

The generated script performs the same steps as the cloud-init variant (writing files, installing packages, running commands) but as a plain `#!/bin/bash` script. You can copy it to the target node and run it:

```bash
$ chmod +x bootstrap.sh
$ sudo ./bootstrap.sh
```

### Sample output

The generated output is a [cloud-init](https://cloud-init.io/) YAML document.

#### `flex` target (recommended)

The `flex` target generates a cloud-init config that downloads the `aks-flex-node` agent and applies a component-based configuration:

```yaml
#cloud-config
package_update: true
packages:
    - curl
write_files:
    - path: /tmp/flex-config.json
      permissions: "0644"
      content: |
        [
          {
            "metadata": {
              "type": "aks.flex.components.linux.ConfigureBaseOS",
              "name": "configure-base-os"
            },
            "spec": {}
          },
          {
            "metadata": {
              "type": "aks.flex.components.cri.DownloadCRIBinaries",
              "name": "download-cri-binaries"
            },
            "spec": {
              "containerd_version": "2.0.4",
              "runc_version": "1.2.5"
            }
          },
          {
            "metadata": {
              "type": "aks.flex.components.kubebins.DownloadKubeBinaries",
              "name": "download-kube-binaries"
            },
            "spec": {
              "kubernetes_version": "1.33.3"
            }
          },
          {
            "metadata": {
              "type": "aks.flex.components.cri.StartContainerdService",
              "name": "start-containerd-service"
            },
            "spec": {}
          },
          {
            "metadata": {
              "type": "aks.flex.components.kubeadm.KubadmNodeJoin",
              "name": "kubeadm-node-join"
            },
            "spec": {
              "control_plane": {
                "server": "https://<api-server-fqdn>:443",
                "certificate_authority_data": "<base64-encoded-ca-cert>"
              },
              "kubelet": {
                "bootstrap_auth_info": {
                  "token": "<bootstrap-token>"
                },
                "node_labels": {
                  "aks.azure.com/stretch-managed": "true"
                }
              }
            }
          }
        ]
runcmd:
    - - set
      - -e
    - |-
      mkdir -p /tmp/flex
      curl -L -o /tmp/flex/aks-flex-node https://bahestoragetest.z13.web.core.windows.net/flex/aks-flex-node-linux-amd64
      chmod +x /tmp/flex/aks-flex-node
      /tmp/flex/aks-flex-node apply -f /tmp/flex-config.json
```

The `aks-flex-node` agent takes care of installing and configuring containerd, kubelet, kubeadm, and joining the cluster.

When `--gpu` is passed, the `StartContainerdService` component includes GPU configuration to enable the NVIDIA container runtime:

```json
{
  "metadata": {
    "type": "aks.flex.components.cri.StartContainerdService",
    "name": "start-containerd-service"
  },
  "spec": {
    "gpu_config": {
      "nvidia_runtime": {}
    }
  }
}
```

#### `ubuntu` target

The `ubuntu` target generates a traditional cloud-init config that installs packages via APT and runs `kubeadm join` directly:

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

In short, the `ubuntu` target prepares the container runtime (containerd) and kubelet, then uses `kubeadm join` to register the node as a worker in the AKS cluster.

#### `--variant script` output

When `--variant script` is used, the same bootstrapping logic is rendered as a standalone bash script instead of cloud-init YAML. For example, `flex --variant script` produces:

```bash
#!/bin/bash
# Auto-generated script equivalent to cloud-init user data.
# This script can be executed directly on a node without cloud-init.
set -euo pipefail

# --- Packages ---
apt-get update -y
apt-get install -y curl

# --- Write files ---
mkdir -p '/tmp'
cat <<'EOF' > '/tmp/flex-config.json'
[{"metadata":{"type":"aks.flex.components.linux.ConfigureBaseOS", ...}, ...}]
EOF
chmod 0644 '/tmp/flex-config.json'

# --- Run commands ---
set -e
mkdir -p /tmp/flex
curl -L -o /tmp/flex/aks-flex-node-linux-amd64.tar.gz https://github.com/Azure/AKSFlexNode/releases/...
tar -xzf /tmp/flex/aks-flex-node-linux-amd64.tar.gz -C /tmp/flex
mv /tmp/flex/aks-flex-node-linux-amd64 /tmp/flex/aks-flex-node
chmod +x /tmp/flex/aks-flex-node
/tmp/flex/aks-flex-node apply -f /tmp/flex-config.json

echo 'Node bootstrap script completed.'
```

The script is self-contained and performs the same operations — writing config files, installing packages, and running bootstrap commands — without requiring the cloud-init daemon.

### Sample: joining an Azure VM with the user data

Generate the user data (using the recommended `flex` target):

```bash
$ aks-flex-cli config node-bootstrap flex > azure-user-data.yaml
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
NAME                                STATUS   ROLES    AGE     VERSION
aks-system-32742974-vmss000000      Ready    <none>   4h32m   v1.33.6
aks-system-32742974-vmss000001      Ready    <none>   4h32m   v1.33.6
flex-node-azure                     Ready    <none>   102s    v1.33.8
```

### Sample: joining a QEMU VM with the user data

Generate the user data:

```bash
$ aks-flex-cli config node-bootstrap flex > qemu-user-data.yaml
```

Launch a QEMU VM with cloud-init support. For example, using a Ubuntu cloud image:

> **Tip:** Download the Ubuntu 24.04 cloud image from
> <https://cloud-images.ubuntu.com/releases/24.04/release/ubuntu-24.04-server-cloudimg-amd64.img>.

```bash
# Create a meta-data file with the instance ID
$ echo "instance-id: qemu-node" > meta-data

# Create a cloud-init seed image
# Linux:
$ cloud-localds seed.img qemu-user-data.yaml meta-data
# macOS (install cdrtools via `brew install cdrtools`):
$ cp qemu-user-data.yaml user-data
$ mkisofs -output seed.img -volid cidata -joliet -rock user-data meta-data

# Create a qcow2 overlay so the base cloud image stays untouched and reusable
$ qemu-img create -f qcow2 -b ubuntu-24.04-server-cloudimg-amd64.img -F qcow2 disk.qcow2 20G

# Launch the VM (adjust paths and resources as needed)
$ qemu-system-x86_64 \
    -m 4096 \
    -smp 2 \
    -drive file=disk.qcow2,format=qcow2 \
    -drive file=seed.img,format=raw \
    -netdev user,id=net0,hostfwd=tcp::2222-:22 \
    -device virtio-net-pci,netdev=net0 \
    -nographic
```

> **Note:** The VM must have network connectivity to the AKS cluster API server. If the cluster is not publicly accessible, ensure appropriate VPN or tunnel connectivity is in place.

After cloud-init completes, verify the node joined:

```bash
$ kubectl get nodes
NAME                                STATUS     ROLES    AGE     VERSION
aks-system-32742974-vmss000000      Ready      <none>   5h19m   v1.33.6
aks-system-32742974-vmss000001      Ready      <none>   5h19m   v1.33.6
flex-node-azure                     Ready      <none>   48m     v1.33.8
ubuntu                              NotReady   <none>   3m15s   v1.33.8
```

> **Note:** The QEMU node might not become Ready due to CNI plugin issues.

## Under the hood

The node bootstrapping process is built on top of [kubeadm](https://kubernetes.io/docs/reference/setup-tools/kubeadm/), the standard Kubernetes node bootstrapping tool.

### Cluster-side preparation (`k8s-bootstrap`)

The `config k8s-bootstrap` command generates the resources that kubeadm expects to find in the cluster during the join process. These include RBAC rules, ConfigMaps, and a bootstrap token Secret (see the [resource table above](#apply-cluster-bootstrap-settings) for the full list).

The flex CLI prepares a bootstrap token for the node join process. This token is used by `kubeadm join` to authenticate with the API server during the TLS bootstrap phase.

### Node-side bootstrapping (`node-bootstrap`)

When a new VM boots with the generated cloud-init user data, the following steps happen:

#### `flex` target (recommended)

```
  New VM (cloud-init)
  │
  ├─ 1. Install curl
  │
  ├─ 2. Write component config JSON to /tmp/flex-config.json
  │     └─ Contains: base OS config, CRI binaries, kube binaries,
  │        containerd service config (with optional GPU support),
  │        kubeadm join config (CA cert, API server URL, bootstrap token)
  │
  ├─ 3. Download aks-flex-node agent
  │
  └─ 4. aks-flex-node apply -f /tmp/flex-config.json
        ├─ ConfigureBaseOS
        ├─ DownloadCRIBinaries (containerd, runc)
        ├─ DownloadKubeBinaries (kubeadm, kubelet)
        ├─ StartContainerdService (with NVIDIA runtime if --gpu)
        └─ KubeadmNodeJoin → node registers with API server
```

#### `ubuntu` target

```
  New VM (cloud-init)
  │
  ├─ 1. Install packages (containerd, kubeadm, kubelet) via APT
  │
  ├─ 2. Write bootstrap kubeconfig
  │     └─ Contains: CA cert, API server URL, bootstrap token
  │
  ├─ 3. Write kubeadm JoinConfiguration
  │     └─ Contains: discovery path, node labels
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
| `nodeRegistration.kubeletExtraArgs.node-ip`     | Sets the node's InternalIP                                |
