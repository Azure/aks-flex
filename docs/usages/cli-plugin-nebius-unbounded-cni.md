# Nebius Cloud Integration (Unbounded CNI)

## Overview

This guide walks through joining nodes from Nebius cloud to an existing AKS Flex cluster using **Unbounded CNI** for cross-cloud networking. By the end you will have:

- A Nebius VPC network connected to the Azure-side network
- A CPU node running in Nebius, joined to the AKS cluster

> **Note:** GPU nodes can be added following the same steps described in the [Nebius Cloud Integration](cli-plugin-nebius.md) guide.

The workflow uses two CLI command groups:

- `aks-flex-cli config` -- generates configuration templates (including Unbounded CNI Site manifests)
- `aks-flex-cli plugin` -- applies, queries, and deletes Nebius resources via the plugin backend

## Setup

### Prerequisites

- An AKS cluster deployed with the `--unbounded-cni` flag -- see [Enable with Unbounded CNI](cli-prepare-aks-cluster.md#enable-with-unbounded-cni)
- The `.env` file must include Nebius configuration (generate with `aks-flex-cli config env --nebius`):

```bash
# Nebius Config
export NEBIUS_PROJECT_ID=<your-nebius-project-id>
export NEBIUS_REGION=<your-nebius-region>
export NEBIUS_CREDENTIALS_FILE=<path-to-nebius-credentials-json>
```

See the [Nebius authorized keys documentation](https://docs.nebius.com/iam/service-accounts/authorized-keys) for creating the credentials file.

### Desired Cluster Setup

We will join a Nebius CPU node to the AKS cluster:

| Node       | Platform   | Preset       | Image Family                | Purpose                     |
| ---------- | ---------- | ------------ | --------------------------- | --------------------------- |
| CPU node   | `cpu-d3`   | `4vcpu-16gb` | `ubuntu24.04-driverless`    | General-purpose workloads   |

## Unbounded CNI Networking

AKS Flex uses the Unbounded CNI operator to establish cross-cloud connectivity between the Azure VNet and the Nebius VPC. The operator manages Site, GatewayPool, and SiteGatewayPoolAssignment resources to define the network topology and route traffic between clouds.

The following diagram illustrates the connectivity:

```
                  Azure                                             Nebius
  ┌──────────────────────────────┐             ┌──────────────────────────────┐
  │  VNet: 172.16.0.0/16         │             │  VPC: 172.20.0.0/16          │
  │                              │             │                              │
  │  ┌────────────┐              │  Unbounded  │              ┌────────────┐  │
  │  │ AKS Node   │              │  CNI        │              │ Nebius VM  │  │
  │  │            │              │◄───────────►│              │            │  │
  │  └────────────┘              │  Gateway    │              └────────────┘  │
  │                              │  Tunnel     │                              │
  │  ┌────────────┐              │             │              ┌────────────┐  │
  │  │ Gateway    │──────────────┼─────────────┼──────────────│ Gateway    │  │
  │  │ Node Pool  │              │             │              │ Peer       │  │
  │  └────────────┘              │             │              └────────────┘  │
  │                              │             │                              │
  │       Unbounded CNI overlay spans across both clouds                     │
  └──────────────────────────────┘             └──────────────────────────────┘
```

When the AKS cluster is deployed with `--unbounded-cni`, the following resources are automatically provisioned:

- A `gateway` node pool with public IPs and allowed UDP ports 51820-51999
- The Unbounded CNI operator (CRDs, controller, node agents)
- An Azure Site, GatewayPool, and SiteGatewayPoolAssignment

## Create Nebius Network Resources

Before creating nodes, you need to provision a VPC network in Nebius that will be connected to the Azure-side network.

### Generate the network config

Use `config networks` to generate a default Nebius network JSON template:

```bash
$ aks-flex-cli config networks nebius > nebius-network.json
```

This produces a JSON file like:

```json
{
  "metadata": {
    "type": "networks.nebius.network.Network",
    "id": "<replace-with-unique-network-name>"
  },
  "spec": {
    "projectId": "<your-nebius-project-id>",
    "region": "<your-nebius-region>",
    "vnet": {
      "cidrBlock": "172.20.0.0/16"
    }
  }
}
```

Review the generated file and update the placeholder values:

| Field              | Description                              | Default            |
| ------------------ | ---------------------------------------- | ------------------ |
| `metadata.id`      | Name of the network resource, should be unique within the Nebius project | `nebius-default` (replace with your own) |
| `spec.projectId`   | Nebius project ID                        | from `.env`        |
| `spec.region`      | Nebius region                            | from `.env`        |
| `spec.vnet.cidrBlock` | CIDR block for the Nebius VPC         | `172.20.0.0/16`    |

### Apply the network config

Pipe the JSON into the `plugin apply` command:

```bash
$ cat nebius-network.json | aks-flex-cli plugin apply networks
```

Expected output:

```
2026/02/21 20:07:00 Applied "nebius-default" (type: networks.nebius.network.Network)
```

### Verify the network

```bash
$ aks-flex-cli plugin get networks nebius-default
```

### Configure the Nebius Site

To connect Nebius nodes, you need to create a Site resource representing the Nebius side. Use the config command to generate a template:

```bash
$ aks-flex-cli config unbounded-cni site \
    --name site-remote \
    --node-cidr 172.20.0.0/16 \
    --pod-cidr 10.200.0.0/16 \
    > site-remote.yaml
```

> **Note:** The `--node-cidr` value should match the Nebius VPC CIDR block configured in the network resource above.

### Apply the Nebius Site

```bash
$ kubectl apply -f site-remote.yaml
```

Expected output:

```
site.unbounded.aks.azure.com/site-remote created
sitegatewaypoolassignment.unbounded.aks.azure.com/main-gateway-site-remote created
```

## Create Nebius CPU Node

### Generate the agent pool config

Use `config agentpools` to generate a default Nebius agent pool JSON template:

```bash
$ aks-flex-cli config agentpools nebius > nebius-cpu.json
```

Edit the file to configure a CPU node. Update the placeholder fields:

| Field               | Value for CPU node              | Description                                          |
| ------------------- | ------------------------------- | ---------------------------------------------------- |
| `metadata.id`       | `nebius-cpu`                    | Unique name for this agent pool                      |
| `spec.subnetId`     | *(from Nebius network output)*  | Subnet ID from the network created in the previous step |
| `spec.platform`     | `cpu-d3`                        | Nebius compute platform                              |
| `spec.preset`       | `4vcpu-16gb`                    | VM size preset                                       |
| `spec.imageFamily`  | `ubuntu24.04-driverless`        | OS image family                                      |

> **Note:** When using Unbounded CNI, the `spec.wireguard` section in the agent pool config is not needed. The Unbounded CNI operator handles cross-cloud routing automatically.

The `kubeadm` section is auto-populated from the running AKS cluster when the `.env` is configured correctly. If the cluster is not reachable, placeholder values are generated that must be replaced manually.

### Apply the agent pool config

```bash
$ cat nebius-cpu.json | aks-flex-cli plugin apply agentpools
```

Expected output:

```
2026/02/21 20:10:24 Applied "nebius-cpu" (type: agentpools.nebius.instance.AgentPool)
```

### Verify the node joined the cluster

After the node provisions and bootstraps (this may take a few minutes), verify it appears in the AKS cluster:

```bash
$ aks-flex-cli plugin get agentpools nebius-cpu
```

```bash
$ export KUBECONFIG=./aks.kubeconfig
$ kubectl get nodes -o wide
NAME                                 STATUS   ROLES    AGE     VERSION   INTERNAL-IP   EXTERNAL-IP   OS-IMAGE             KERNEL-VERSION       CONTAINER-RUNTIME
aks-gateway-26665104-vmss000000      Ready    <none>   3h27m   v1.34.2   172.16.2.4    <MASKED>      Ubuntu 22.04.5 LTS   5.15.0-1102-azure    containerd://1.7.30-1
aks-system-14211521-vmss000000       Ready    <none>   3h31m   v1.34.2   172.16.1.4    <none>        Ubuntu 22.04.5 LTS   5.15.0-1102-azure    containerd://1.7.30-1
aks-system-14211521-vmss000001       Ready    <none>   3h31m   v1.34.2   172.16.1.5    <none>        Ubuntu 22.04.5 LTS   5.15.0-1102-azure    containerd://1.7.30-1
aks-system-14211521-vmss000002       Ready    <none>   3h31m   v1.34.2   172.16.1.6    <none>        Ubuntu 22.04.5 LTS   5.15.0-1102-azure    containerd://1.7.30-1
computeinstance-e00ky0djhsjh1jh7d1   Ready    <none>   58m     v1.33.3   172.20.0.32   <none>        Ubuntu 24.04.4 LTS   6.11.0-1016-nvidia   containerd://2.0.4
```

## Validating cross-cloud connectivity

With the Unbounded CNI operator in place, pods running on the Nebius nodes should be able to communicate with pods on the AKS nodes, and vice versa. We can validate this by checking the logs from pods running on the Nebius nodes:

```bash
$ kubectl get pod -o wide
NAME                          READY   STATUS    RESTARTS   AGE   IP            NODE                                 NOMINATED NODE   READINESS GATES
sample-app-7487b844b7-4skp4   1/1     Running   0          54m   10.200.7.10   computeinstance-e00yrmd6zyz1ezrh6m   <none>           <none>
```

```bash
$ kubectl logs -f sample-app-7487b844b7-4skp4
/docker-entrypoint.sh: /docker-entrypoint.d/ is not empty, will attempt to perform configuration
/docker-entrypoint.sh: Looking for shell scripts in /docker-entrypoint.d/
/docker-entrypoint.sh: Launching /docker-entrypoint.d/10-listen-on-ipv6-by-default.sh
```

## Clean up resources

To remove the Nebius node and network, delete them in reverse order: agent pool first, then the network.

### Delete agent pool

```bash
$ aks-flex-cli plugin delete agentpools nebius-cpu
```

Verify the node is showing as NotReady in Kubernetes:

```bash
$ kubectl get nodes
```

### Delete the network

```bash
$ aks-flex-cli plugin delete networks nebius-default
```

### List remaining resources

Confirm all Nebius resources are cleaned up:

```bash
$ aks-flex-cli plugin get networks
[]
$ aks-flex-cli plugin get agentpools
[]
```

Both commands should return empty lists.
