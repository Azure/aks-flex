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

### Download pre-built binaries

Pre-built binaries are available for the following platforms:

| Platform      | Architecture | Download |
| ------------- | ------------ | -------- |
| macOS (Darwin) | arm64        | [aks-flex-cli_0.0.1-snapshot-910c096_darwin_arm64.tar.gz](https://aksflxcli.z20.web.core.windows.net/aks-flex-cli/aks-flex-cli_0.0.1-snapshot-910c096_darwin_arm64.tar.gz) |
| Linux          | amd64        | [aks-flex-cli_0.0.1-snapshot-910c096_linux_amd64.tar.gz](https://aksflxcli.z20.web.core.windows.net/aks-flex-cli/aks-flex-cli_0.0.1-snapshot-910c096_linux_amd64.tar.gz) |
| Linux          | arm64        | [aks-flex-cli_0.0.1-snapshot-910c096_linux_arm64.tar.gz](https://aksflxcli.z20.web.core.windows.net/aks-flex-cli/aks-flex-cli_0.0.1-snapshot-910c096_linux_arm64.tar.gz) |

Download and install the binary for your platform:

```bash
# Example for macOS (arm64)
curl -LO https://aksflxcli.z20.web.core.windows.net/aks-flex-cli/aks-flex-cli_0.0.1-snapshot-910c096_darwin_arm64.tar.gz
tar -xzf aks-flex-cli_0.0.1-snapshot-910c096_darwin_arm64.tar.gz
chmod +x aks-flex-cli_darwin_arm64
mkdir -p ~/.local/bin
mv aks-flex-cli_darwin_arm64 ~/.local/bin/aks-flex-cli
```

```bash
# Example for WSL / Linux (amd64)
curl -LO https://aksflxcli.z20.web.core.windows.net/aks-flex-cli/aks-flex-cli_0.0.1-snapshot-910c096_linux_amd64.tar.gz
tar -xzf aks-flex-cli_0.0.1-snapshot-910c096_linux_amd64.tar.gz
chmod +x aks-flex-cli_linux_amd64
mkdir -p ~/.local/bin
mv aks-flex-cli_linux_amd64 ~/.local/bin/aks-flex-cli
```

Make sure `~/.local/bin` is on your `PATH`. If it isn't, add the following to your shell profile (`~/.bashrc`, `~/.zshrc`, etc.):

```bash
export PATH="$HOME/.local/bin:$PATH"
```

#### Verify checksums

After downloading, verify the integrity of the archive using SHA-256:

```
eed0025cff7685578dc0e52febc919f592270a10a55a5e2801f9b1da928ce19d  aks-flex-cli_0.0.1-snapshot-910c096_darwin_arm64.tar.gz
2f49d97716aed6f99966da8efa70e72dd894efa3312a74f3687ad31da8108103  aks-flex-cli_0.0.1-snapshot-910c096_linux_amd64.tar.gz
53f5fccf8e7cec4a912254999d7749724b6203c4c0f2fd30c3004e468e8058d0  aks-flex-cli_0.0.1-snapshot-910c096_linux_arm64.tar.gz
```

```bash
shasum -a 256 aks-flex-cli_*.tar.gz
```

> **TODO:** These binaries will be published to [GitHub Releases](https://github.com/Azure/aks-flex/releases) in the future. Once available, download URLs and installation instructions will be updated accordingly.


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
aks-flex-cli <version> (commit: <commit>, built: <build-time>)
```


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
