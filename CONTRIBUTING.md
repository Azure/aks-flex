# Contributing to aks-flex

Thank you for your interest in contributing to this project!

## Contributor License Agreement

Most contributions require you to agree to a Contributor License Agreement (CLA) declaring
that you have the right to, and actually do, grant us the rights to use your contribution.
For details, visit [https://cla.opensource.microsoft.com](https://cla.opensource.microsoft.com).

When you submit a pull request, a CLA bot will automatically determine whether you need to
provide a CLA and decorate the PR appropriately (e.g., status check, comment). Simply follow
the instructions provided by the bot. You will only need to do this once across all repos
using our CLA.

## Code of Conduct

This project has adopted the [Microsoft Open Source Code of Conduct](https://opensource.microsoft.com/codeofconduct/).
For more information see the [Code of Conduct FAQ](https://opensource.microsoft.com/codeofconduct/faq/) or
contact [opencode@microsoft.com](mailto:opencode@microsoft.com) with any additional questions or comments.

## How to Contribute

1. Fork the repository and create a branch from `main`.
2. Make your changes, following the coding conventions below.
3. Ensure all tests pass and linting is clean.
4. Open a pull request against `main`.

Please search existing issues and PRs before opening new ones to avoid duplicates.

## Project Structure

The repository contains two independently buildable components, each with their own `go.mod`
and `Makefile`:

| Directory    | Description |
|--------------|-------------|
| `cli/`       | The `aks-flex-cli` command-line tool for provisioning and managing AKS Flex infrastructure |
| `karpenter/` | A Karpenter controller that extends AKS with external cloud-provider node classes |

## Prerequisites

- [Go](https://go.dev/dl/) 1.22 or later
- [Docker](https://docs.docker.com/get-docker/) (for building the karpenter container image)
- `kubectl` (for deploying to a cluster)

All other build tools (goreleaser, golangci-lint, controller-gen, etc.) are downloaded
automatically into a local `bin/` directory by `make` targets — no manual installation needed.

## Building

### CLI (`cli/`)

```bash
# Build a binary for the current platform (outputs to cli/bin/aks-flex-cli)
cd cli && make build-local

# Build release archives for all supported platforms using goreleaser
cd cli && make build

# Run directly from source
cd cli && make run
```

### Karpenter controller (`karpenter/`)

The karpenter module vendors its dependencies and applies a set of patches on top:

```bash
# Vendor dependencies and apply patches (required before building)
cd karpenter && make vendor-patch

# Build the controller binary for the current platform
cd karpenter && make build

# Build and tag a Docker image (outputs ghcr.io/azure/aks-flex/karpenter:<branch>-<sha>)
cd karpenter && make docker-build

# Regenerate CRD / RBAC manifests from Go types
cd karpenter && make manifests

# Regenerate DeepCopy method implementations
cd karpenter && make generate
```

## Testing

### CLI

```bash
cd cli && make test
```

### Karpenter controller

```bash
# Unit tests (downloads envtest assets automatically on first run)
cd karpenter && make test
```

## Linting

Both components use [golangci-lint](https://golangci-lint.run/):

```bash
# Report lint issues
make lint

# Report and auto-fix lint issues where possible
make lint-fix
```

Run these from within the relevant component directory (`cli/` or `karpenter/`).

## Coding Conventions

- **Formatting**: All Go code must be formatted with `gofmt` / `go fmt`. The `make test` and
  `make build-local` targets run `go fmt` automatically before building.
- **Vetting**: Code is checked with `go vet` as part of every build and test invocation.
- **Linting**: PRs should have zero golangci-lint warnings. Use `make lint-fix` to resolve
  auto-fixable issues before opening a PR.
- **Commit messages**: Use the conventional prefix style (`fix:`, `feat:`, `docs:`, `ci:`,
  `chore:`) that is already used in the project history. Changelog entries are generated
  from commit messages and `docs:`/`test:` prefixed commits are excluded automatically.

## Vendored Patches (karpenter only)

The karpenter module vendors upstream dependencies and maintains a small set of patches in
`karpenter/patches/*.diff`. If your change requires modifying vendored code:

1. Make the change directly in the `vendor/` tree.
2. Produce a diff with `git diff vendor/ > karpenter/patches/<NNN>-<description>.diff`.
3. Verify the full round-trip works: `make vendor-patch` followed by `make verify-vendor`.

This ensures the patches remain reproducible and reviewable.
