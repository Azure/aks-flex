# AKS Flex

AKS Flex lets you extend an Azure Kubernetes Service (AKS) cluster with nodes from external
cloud providers or on-premises infrastructure. Worker nodes running on remote clouds or
on-prem hardware join the AKS control plane over an encrypted overlay network, giving you
a single Kubernetes cluster that spans hybrid and multi-cloud environments.

## Components

| Component | Description |
|-----------|-------------|
| [`cli/`](cli/) | `aks-flex-cli` — a command-line tool for provisioning the Azure network, creating an AKS cluster, and managing remote node pools |
| [`karpenter/`](karpenter/) | A Karpenter provider that autoscales remote-cloud nodes in response to workload demand |

## Getting Started

See the [usage guides](docs/usages/README.md) for end-to-end walkthroughs, including:

- [CLI setup](docs/usages/cli-setup.md)
- [AKS cluster setup](docs/usages/cli-prepare-aks-cluster.md)
- [Nebius Cloud integration](docs/usages/cli-plugin-nebius.md)
- [Karpenter autoscaling](docs/usages/karpenter.md)
- [Manual node bootstrapping](docs/usages/cli-node-bootstrap.md)

## Contributing

Contributions are welcome! Please read [CONTRIBUTING.md](CONTRIBUTING.md) for details on the
Contributor License Agreement, build instructions, coding conventions, and how to submit a
pull request.

## Trademarks

This project may contain trademarks or logos for projects, products, or services. Authorized use of Microsoft
trademarks or logos is subject to and must follow
[Microsoft's Trademark & Brand Guidelines](https://www.microsoft.com/legal/intellectualproperty/trademarks/usage/general).
Use of Microsoft trademarks or logos in modified versions of this project must not cause confusion or imply Microsoft sponsorship.
Any use of third-party trademarks or logos are subject to those third-party's policies.
