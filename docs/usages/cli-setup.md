# AKS Flex CLI Setup

## Overview

The AKS Flex CLI (`aks-flex-cli`) is a command-line tool for managing AKS Flex clusters that span across Azure and remote cloud providers (e.g. Nebius). It handles the end-to-end lifecycle of a multi-cloud Kubernetes environment, including:

- **Network provisioning** -- deploy and manage the Azure-side network infrastructure
- **AKS cluster deployment** -- create and configure AKS clusters with options such as Cilium CNI and WireGuard encryption
- **Remote cloud integration** -- configure networking and agent pools on remote cloud providers
- **Configuration generation** -- bootstrap environment configs, Kubernetes cluster settings, and node bootstrap scripts

The CLI is organized into four top-level commands:

| Command   | Description                                                  |
| --------- | ------------------------------------------------------------ |
| `config`  | Generate environment files, network configs, agent pool configs, and bootstrap manifests |
| `network` | Deploy and manage Azure network resources                    |
| `aks`     | Deploy and manage the AKS cluster                            |
| `plugin`  | Apply, get, and delete remote cloud resources (networks, agent pools) |

## Getting Started

### Prerequisites

The following tools must be installed and available on your `PATH`:

- **Go 1.26+** -- required to build the CLI from source
- **Azure CLI (`az`)** -- used to resolve your Azure subscription and manage Azure resources. You must be logged in (`az login`) before using the CLI.
- **kubectl** -- required for interacting with the deployed AKS cluster
- **Helm (`helm`)** *(optional)* -- used for deploying Helm charts to the cluster
- **Cilium CLI (`cilium`)** *(optional)* -- used for managing and validating Cilium networking. See [Cilium install guide](https://docs.cilium.io/en/stable/gettingstarted/k8s-install-default/#install-the-cilium-cli).
- **Nebius CLI (`nebius`)** *(optional)* -- required only if you plan to integrate with Nebius as a remote cloud provider

### Install from source

Clone the repository and build the binary:

```bash
git clone https://github.com/Azure/aks-flex.git
cd aks-flex/cli
make build-local
```

The compiled binary is placed at `./bin/aks-flex-cli`. You can move it to a directory on your `PATH`:

```bash
cp ./bin/aks-flex-cli /usr/local/bin/
```

Verify the installation:

```bash
$ aks-flex-cli version
aks-flex-cli 2034ea2 (commit: 2034ea244f2bff421a43644a1865b07331d13235, built: 2026-02-22T03:14:16Z)
```

### Download pre-built binaries

<!-- TODO: Add download instructions once release artifacts are published (e.g. GitHub Releases URL, supported platforms, checksums). -->

## Configuration

<!-- LLM-AGENT: When the CLI configuration format changes (new env vars, renamed flags, new subcommands), update this section to reflect the current behavior. -->

Before running any provisioning commands, you need to generate an environment configuration file that supplies Azure (and optionally remote cloud) settings to the CLI.

### Generate the environment file

Run the `config env` subcommand and redirect the output to a `.env` file in the `cli/` directory:

```bash
$ aks-flex-cli config env > .env
```

The command auto-detects your current Azure subscription (via `az account show`) and generates a `.env` file similar to:

```bash
# !! This file includes sensitive information, please do not commit it to any public repository !!
# Please update the values in this file for your environment.

# -----------------------------------------------------------------------------
# Azure Config
# -----------------------------------------------------------------------------
# Azure side resource location
export LOCATION=southcentralus
# Azure side subscription ID
export AZURE_SUBSCRIPTION_ID=<your-subscription-id>
# Azure side resource group name
export RESOURCE_GROUP_NAME=rg-aks-flex-<username>
```

To include Nebius configuration, pass the `--nebius` flag:

```bash
$ aks-flex-cli config env --nebius > .env
```

This appends Nebius-specific variables (project ID, region, credentials file path) to the generated output. If the Nebius CLI is installed and configured, the project ID is resolved automatically; otherwise placeholder values are inserted that you must fill in manually.

### Review and update the configuration

Open the generated `.env` file and verify or update the following values:

| Variable                   | Description                                | Default                          |
| -------------------------- | ------------------------------------------ | -------------------------------- |
| `LOCATION`                 | Azure region for resources                 | `southcentralus`                 |
| `AZURE_SUBSCRIPTION_ID`    | Azure subscription ID                      | auto-detected from `az` CLI     |
| `RESOURCE_GROUP_NAME`      | Resource group name                        | `rg-aks-flex-<username>`         |
| `NEBIUS_PROJECT_ID`        | Nebius project ID *(if `--nebius`)*        | auto-detected or placeholder     |
| `NEBIUS_REGION`            | Nebius region *(if `--nebius`)*            | placeholder -- must be updated   |
| `NEBIUS_CREDENTIALS_FILE`  | Path to Nebius credentials JSON *(if `--nebius`)* | placeholder -- must be updated   |

### Authentication

The CLI uses your existing Azure CLI session for authentication. Make sure you are logged in before running any commands:

```bash
az login
az account set --subscription <your-subscription-id>
```

For Nebius integration, follow the [Nebius authorized keys documentation](https://docs.nebius.com/iam/service-accounts/authorized-keys) to create a credentials file and set its path in `NEBIUS_CREDENTIALS_FILE`.

Once the `.env` file is in place, the CLI loads it automatically on startup (via `godotenv`), so no additional sourcing step is required.
