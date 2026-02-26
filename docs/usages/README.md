# AKS Flex Usage Guides

This folder contains documentation and samples for using AKS Flex to build multi-cloud Kubernetes clusters that span Azure and remote cloud providers.

## Getting Started

Start with the CLI setup guide, then follow the scenario that best fits your use case.

| Guide | Description |
| ----- | ----------- |
| [CLI Setup](cli-setup.md) | Install the `aks-flex-cli` tool and generate a `.env` configuration file |
| [AKS Cluster Setup](cli-prepare-aks-cluster.md) | Deploy Azure network resources, create an AKS cluster with Cilium CNI, and optionally enable WireGuard |

## Scenarios

AKS Flex supports multiple approaches for joining remote cloud nodes to an AKS cluster:

### 1. Plugin-managed node pools

Use the `aks-flex-cli plugin` commands to declaratively manage networks and agent pools on remote clouds. The plugin handles VM provisioning, node bootstrapping, and lifecycle management.

| Guide | Description |
| ----- | ----------- |
| [Nebius Cloud Integration](cli-plugin-nebius.md) | Join CPU and GPU nodes from Nebius to the AKS cluster using the plugin workflow |

### 2. Karpenter autoscaling

Use Karpenter with the `karpenter` to automatically provision and deprovision remote cloud nodes based on workload demand. Karpenter watches for unschedulable pods and creates nodes as needed.

| Guide | Description |
| ----- | ----------- |
| [Karpenter Provider Flex](karpenter.md) | Deploy karpenter and use NodePools to autoscale Nebius nodes |

### 3. Manual node bootstrapping

For advanced users who want full control over the bootstrapping process. Use `aks-flex-cli config` commands to retrieve cluster configurations and generate cloud-init scripts, then apply them to any VM or bare-metal node that supports cloud-init.

| Guide | Description |
| ----- | ----------- |
| [Node Bootstrapping](cli-node-bootstrap.md) | Manually prepare the cluster and generate cloud-init user data to join nodes via `kubeadm` |
