#!/usr/bin/env bash
# Build, package, and optionally push the Karpenter Helm chart.
#
# Usage:
#   ./hack/release-chart.sh [options]
#
# Options:
#   --version VERSION   Override chart version (default: read from Chart.yaml)
#   --registry URI      OCI registry to push to (e.g. oci://ghcr.io/azure/aks-flex/charts)
#   --output DIR        Output directory for the .tgz package (default: .helm-packages)
#   --push              Push the chart to the OCI registry after packaging
#   --help              Show this help message
#
# Examples:
#   # Lint and package only (local dev)
#   ./hack/release-chart.sh
#
#   # Package with a specific version
#   ./hack/release-chart.sh --version 0.2.0
#
#   # Package and push to GHCR
#   ./hack/release-chart.sh --version 0.2.0 --registry oci://ghcr.io/azure/aks-flex/charts --push

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CHART_DIR="${SCRIPT_DIR}/../charts/karpenter"

# Defaults
VERSION=""
REGISTRY=""
OUTPUT_DIR="${SCRIPT_DIR}/../../.helm-packages"
PUSH=false

usage() {
  sed -n '2,/^$/s/^# \{0,1\}//p' "$0"
  exit 0
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version)  VERSION="$2";   shift 2 ;;
    --registry) REGISTRY="$2";  shift 2 ;;
    --output)   OUTPUT_DIR="$2"; shift 2 ;;
    --push)     PUSH=true;      shift ;;
    --help|-h)  usage ;;
    *) echo "Unknown option: $1" >&2; usage ;;
  esac
done

# ---------------------------------------------------------------------------
# Resolve chart version
# ---------------------------------------------------------------------------
if [[ -z "$VERSION" ]]; then
  VERSION=$(grep '^version:' "$CHART_DIR/Chart.yaml" | awk '{print $2}')
fi
echo "Chart version: ${VERSION}"

# ---------------------------------------------------------------------------
# Step 1: Lint
# ---------------------------------------------------------------------------
echo "==> Linting chart..."
helm lint "$CHART_DIR"

# ---------------------------------------------------------------------------
# Step 2: Package
# ---------------------------------------------------------------------------
mkdir -p "$OUTPUT_DIR"
echo "==> Packaging chart..."
helm package "$CHART_DIR" --version "$VERSION" --destination "$OUTPUT_DIR"

CHART_PACKAGE="${OUTPUT_DIR}/karpenter-${VERSION}.tgz"
if [[ ! -f "$CHART_PACKAGE" ]]; then
  echo "Error: expected package not found: $CHART_PACKAGE" >&2
  exit 1
fi
echo "Package created: ${CHART_PACKAGE}"

# ---------------------------------------------------------------------------
# Step 3: Push (optional)
# ---------------------------------------------------------------------------
if [[ "$PUSH" == true ]]; then
  if [[ -z "$REGISTRY" ]]; then
    echo "Error: --registry is required when --push is set" >&2
    exit 1
  fi
  echo "==> Pushing chart to ${REGISTRY}..."
  helm push "$CHART_PACKAGE" "$REGISTRY"
  echo "Chart pushed successfully."
fi

echo "Done."
