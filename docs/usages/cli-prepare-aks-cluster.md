# AKS Cluster Setup

## Overview

This guide walks through preparing a base AKS cluster and the supporting Azure resources needed for AKS Flex. By the end of this guide you will have:

- A resource group with the required Azure network infrastructure (VNet, subnets, NSG)
- An AKS managed cluster with BYO (Bring Your Own) Cilium CNI enabled
- A downloaded kubeconfig for connecting to the cluster

Optionally, you can also deploy a WireGuard gateway for site-to-site encrypted tunneling -- see [Enable with WireGuard](#enable-with-wireguard) for details.

## Setup

### CLI

Install the `aks-flex-cli` binary and generate a `.env` configuration file by following the instructions in [CLI Setup](cli-setup.md).

### Configuration

Ensure your `.env` file in the `cli/` directory contains the required Azure settings. A minimal configuration looks like:

```bash
# Azure Config
export LOCATION=southcentralus
export AZURE_SUBSCRIPTION_ID=<your-subscription-id>
export RESOURCE_GROUP_NAME=rg-aks-flex-<username>
```

The following environment variables are relevant to cluster creation:

| Variable                 | Description                            | Default              |
| ------------------------ | -------------------------------------- | -------------------- |
| `LOCATION`               | Azure region for all resources         | `southcentralus`     |
| `AZURE_SUBSCRIPTION_ID`  | Azure subscription ID                  | auto-detected        |
| `RESOURCE_GROUP_NAME`    | Resource group name                    | `rg-aks-flex-<user>` |
| `CLUSTER_NAME`           | Name of the AKS cluster               | `aks`                |
| `AKS_NODE_VM_SIZE`       | VM size for the system node pool       | `Standard_D2s_v3`    |

### Desired Cluster Setup

The CLI creates an AKS cluster with `networkPlugin: none`, which disables the built-in Azure CNI so that Cilium can be installed as the cluster's CNI instead. The cluster is deployed with:

- A **system** node pool (2 nodes) in the `aks` subnet (`172.16.1.0/24`)
- **Cilium CNI** installed via the Cilium CLI after the cluster is provisioned
- A system-assigned managed identity

If you need site-to-site connectivity without a VPN gateway (or for development/testing purposes), you can additionally enable WireGuard. Refer to the [Enable with WireGuard](#enable-with-wireguard) section below for the modified setup.

## Create Network Resources

First, deploy the Azure network infrastructure. This creates a resource group (if it does not exist) and provisions:

| Resource                | Name   | Details                          |
| ----------------------- | ------ | -------------------------------- |
| Network Security Group  | `nsg`  | Baseline inbound security rules  |
| Virtual Network         | `vnet` | Address space `172.16.0.0/16`    |
| Subnet -- GatewaySubnet | `GatewaySubnet` | `172.16.0.0/24` (reserved for VPN gateway) |
| Subnet -- AKS           | `aks`  | `172.16.1.0/24` (system node pool) |
| Subnet -- Nodes         | `nodes`| `172.16.2.0/24` (additional node pools) |

Run the network deploy command:

```bash
$ aks-flex-cli network deploy
```

Expected output:

```
2026/02/21 10:00:00 creating resource group rg-aks-flex-<username> in southcentralus
2026/02/21 10:00:02 starting deployment network in rg-aks-flex-<username>
2026/02/21 10:00:30 deployment network succeeded
```

To also deploy a VPN gateway (for production site-to-site VPN), add the `--gateway` flag:

```bash
$ aks-flex-cli network deploy --gateway
```

> **Note:** VPN gateway provisioning can take 20-40 minutes.

<!-- TODO: Add Azure portal screenshots of the created network resources. -->

## Create AKS Cluster

With the network in place, deploy the AKS cluster with Cilium enabled:

```bash
$ aks-flex-cli aks deploy --cilium
```

This command performs the following steps:

1. Deploys the AKS cluster via an ARM template into the existing VNet
2. Downloads the cluster kubeconfig (saved as `<cluster-name>.kubeconfig`, e.g. `aks.kubeconfig`)
3. Applies baseline Kubernetes resources
4. Installs Cilium CNI using the `cilium install` CLI

Expected output:

```
2026/02/21 10:05:00 starting deployment aks in rg-aks-flex-<username>
2026/02/21 10:12:00 deployment aks succeeded
2026/02/21 10:12:01 kubeconfig saved to aks.kubeconfig
ℹ️  Using Cilium version 1.17.x
🔮 Auto-detected cluster name: aks
...
✅ Cilium was successfully installed!
```

You can customize the kubeconfig output path with `--kubeconfig-to-save`:

```bash
$ aks-flex-cli aks deploy --cilium --kubeconfig-to-save ./my-cluster.kubeconfig
```

<!-- TODO: Add Azure portal screenshots of the created AKS cluster resources. -->

### Enable with WireGuard

WireGuard provides an encrypted site-to-site tunnel between the AKS cluster and remote cloud nodes. This is useful when:

- A VPN gateway is not available or not practical for the environment
- You need a lightweight tunnel for development and testing
- You want encrypted node-to-node communication across clouds

To deploy the cluster with both Cilium and WireGuard:

```bash
$ aks-flex-cli aks deploy --cilium --wireguard
```

In addition to the standard AKS resources, the `--wireguard` flag provisions:

| Resource                 | Name                    | Details                                          |
| ------------------------ | ----------------------- | ------------------------------------------------ |
| NSG rule                 | `AllowWireGuard`        | Allows inbound UDP/51820                         |
| Public IP prefix         | `nebius-wg-pip-prefix`  | Static public IP prefix for the gateway node     |
| Agent pool               | `wireguard`             | 1-node pool in the `nodes` subnet with public IP |
| Route table              | `nebius-routes`         | Routes remote cloud traffic through the gateway  |

After the ARM deployment, the CLI automatically:

1. Generates (or reuses) a WireGuard key pair stored as a Kubernetes secret
2. Waits for the WireGuard gateway node to register and receive its public IP
3. Updates the `nebius-routes` route table to forward remote cloud traffic (`100.96.0.0/12`) through the gateway node
4. Associates the route table with the `aks` and `nodes` subnets
5. Deploys the WireGuard DaemonSet to the cluster

Expected output:

```
2026/02/21 10:05:00 starting deployment aks in rg-aks-flex-<username>
2026/02/21 10:15:00 deployment aks succeeded
2026/02/21 10:15:01 kubeconfig saved to aks.kubeconfig
...
✅ Cilium was successfully installed!
2026/02/21 10:16:00 Getting WireGuard keys...
2026/02/21 10:16:00   Generating new WireGuard keys
2026/02/21 10:16:00   Public Key: <base64-encoded-public-key>
2026/02/21 10:16:00 Waiting for WireGuard gateway node to register...
2026/02/21 10:17:30 WireGuard gateway node ready
2026/02/21 10:17:30   Public IP: <gateway-public-ip>
2026/02/21 10:17:30   Private IP: <gateway-private-ip>
2026/02/21 10:17:30 Updating route table...
2026/02/21 10:17:35   Route table updated with gateway IP: <gateway-private-ip>
2026/02/21 10:17:35 Associating route table with subnets...
2026/02/21 10:17:40   Associating route table with subnet aks...
2026/02/21 10:17:45   Route table associated with subnet aks
2026/02/21 10:17:45   Associating route table with subnet nodes...
2026/02/21 10:17:50   Route table associated with subnet nodes
2026/02/21 10:17:50 Deploying WireGuard DaemonSet...
2026/02/21 10:17:52   WireGuard DaemonSet deployed successfully
```

## Connecting to the cluster

After the AKS cluster is created, the CLI saves a kubeconfig file to the working directory (default: `aks.kubeconfig`). Use it to connect to the cluster:

```bash
$ export KUBECONFIG=./aks.kubeconfig
$ kubectl get nodes
NAME                             STATUS   ROLES    AGE   VERSION
aks-system-12345678-vmss000000   Ready    <none>   5m    v1.31.x
aks-system-12345678-vmss000001   Ready    <none>   5m    v1.31.x
```

If you deployed with `--wireguard`, you will also see the WireGuard gateway node:

```bash
$ kubectl get nodes
NAME                                 STATUS   ROLES    AGE   VERSION
aks-system-12345678-vmss000000       Ready    <none>   10m   v1.31.x
aks-system-12345678-vmss000001       Ready    <none>   10m   v1.31.x
aks-wireguard-12345678-vmss000000    Ready    <none>   8m    v1.31.x
```
