# Release Guide

This document covers how to create releases for each component in the repository.

## Tag Convention

Each component uses a prefixed tag to trigger its release workflow:

| Component | Tag pattern | Example | Workflow |
|-----------|------------|---------|----------|
| CLI | `cli/v<semver>` | `cli/v1.2.3` | `.github/workflows/cli-release.yaml` |
| Karpenter image | `karpenter/v<semver>` | `karpenter/v0.5.0` | `.github/workflows/karpenter-publish.yaml` |
| Karpenter Helm chart | `karpenter-chart/v<semver>` | `karpenter-chart/v0.2.0` | `.github/workflows/karpenter-chart-release.yaml` |

All tags must follow [Semantic Versioning](https://semver.org/).

---

## CLI Release

The CLI release workflow builds cross-platform binaries with [GoReleaser](https://goreleaser.com/) and publishes them as a GitHub Release.

### Artifacts

Each release includes:

- `aks-flex-cli_<version>_linux_amd64.tar.gz`
- `aks-flex-cli_<version>_linux_arm64.tar.gz`
- `aks-flex-cli_<version>_darwin_arm64.tar.gz`
- `checksums.txt`
- Auto-generated changelog

### Steps

1. Make sure all changes are merged to `main`.

2. Decide on a version number following semver (e.g. `v1.0.0`).

3. Create and push the tag:

   ```bash
   git tag cli/v1.0.0
   git push origin cli/v1.0.0
   ```

4. The [Release CLI](.../../.github/workflows/cli-release.yaml) workflow runs automatically. Monitor it in the **Actions** tab.

5. Once complete, the GitHub Release appears under **Releases** with all binary archives attached.

### Local Snapshot Build

To build binaries locally without publishing:

```bash
cd cli
make build           # binaries only
make build-archives  # binaries + tar.gz archives
```

Snapshot builds append a `-snapshot-<commit>` suffix to the version.

---

## Karpenter Release

The Karpenter workflow builds a multi-platform Docker image and pushes it to GHCR.

### Image

```
ghcr.io/<owner>/aks-flex/karpenter:<tag>
```

### Triggers

The workflow runs on:

- Push to `main` when files under `karpenter/` or `plugin/` change (tagged as `main` and `sha-<short>`).
- A `karpenter/v*` tag push (tagged with the semver and `latest`).
- Manual `workflow_dispatch`.

### Steps

1. Make sure all changes are merged to `main`.

2. Decide on a version number (e.g. `v0.5.0`).

3. Create and push the tag:

   ```bash
   git tag karpenter/v0.5.0
   git push origin karpenter/v0.5.0
   ```

4. The [Publish Karpenter Image](../../.github/workflows/karpenter-publish.yaml) workflow runs automatically.

5. Once complete, the image is available at:

   ```
   ghcr.io/<owner>/aks-flex/karpenter:0.5.0
   ghcr.io/<owner>/aks-flex/karpenter:latest
   ```

---

## Karpenter Helm Chart Release

The Karpenter Helm chart workflow packages the chart in `karpenter/charts/karpenter/` and publishes it as an OCI artifact to GHCR. A GitHub Release is also created when triggered by a tag.

### Chart OCI reference

```
oci://ghcr.io/<owner>/aks-flex/charts/karpenter:<version>
```

### Triggers

The workflow runs on:

- A `karpenter-chart/v*` tag push (packages, pushes, and creates a GitHub Release).
- Manual `workflow_dispatch` with an optional version override.

### Steps

1. Make sure all changes are merged to `main`.

2. Update `karpenter/charts/karpenter/Chart.yaml` if the `version` field does not already reflect the release version you want to publish.

3. Decide on a chart version (e.g. `v0.2.0`).

4. Create and push the tag:

   ```bash
   git tag karpenter-chart/v0.2.0
   git push origin karpenter-chart/v0.2.0
   ```

5. The [Release Karpenter Helm Chart](../../.github/workflows/karpenter-chart-release.yaml) workflow runs automatically. Monitor it in the **Actions** tab.

6. Once complete:
   - The chart is available from the OCI registry:

     ```bash
     helm install karpenter oci://ghcr.io/<owner>/aks-flex/charts/karpenter \
       --version 0.2.0 \
       --namespace kube-system
     ```

   - A GitHub Release named **Karpenter Helm Chart v0.2.0** is created under **Releases** with the `.tgz` archive attached.

### Local Chart Build

To lint and package the chart locally without publishing:

```bash
cd karpenter
./hack/release-chart.sh                   # lint + package using Chart.yaml version
./hack/release-chart.sh --version 0.2.0   # lint + package with a specific version
```

The `.tgz` is written to `.helm-packages/karpenter-<version>.tgz` at the repository root.

---

## Troubleshooting

- **Workflow did not trigger** -- Verify the tag matches the expected pattern exactly (`cli/v*`, `karpenter/v*`, or `karpenter-chart/v*`). Tags like `CLI/v1.0.0` or `v1.0.0` without a prefix will not trigger the workflows.
- **GoReleaser fails with "tag is not a semver"** -- Ensure the portion after the prefix is valid semver (e.g. `v1.2.3`, not `v1.2`).
- **Permission denied on release** -- The workflow requires `contents: write` permission. This is configured in the workflow file but may need to be allowed in the repository settings if the default token permissions are restricted.
- **Helm chart package not found** -- The `release-chart.sh` script expects the chart to be at `karpenter/charts/karpenter/`. Ensure the `Chart.yaml` exists there and the `version` field is set before running the workflow.
