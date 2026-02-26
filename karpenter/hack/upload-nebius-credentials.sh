#!/usr/bin/env bash
# Upload a local Nebius credentials file as a Kubernetes Secret.
#
# Usage:
#   ./hack/upload-nebius-credentials.sh <path-to-credentials-file> [namespace] [secret-name]
#
# Arguments:
#   path-to-credentials-file  (required) Local path to the credentials JSON file.
#   namespace                 (optional) Target namespace. Defaults to "karpenter".
#   secret-name               (optional) Secret name. Defaults to "nebius-credentials".
#
# The secret key inside the Secret is "credentials.json", matching the default
# value in the Helm chart (controller.nebiusCredentials.secretKey).

set -euo pipefail

CREDENTIALS_FILE="${1:?Usage: $0 <path-to-credentials-file> [namespace] [secret-name]}"
NAMESPACE="${2:-karpenter}"
SECRET_NAME="${3:-nebius-credentials}"
SECRET_KEY="credentials.json"

if [ ! -f "$CREDENTIALS_FILE" ]; then
  echo "Error: file not found: $CREDENTIALS_FILE" >&2
  exit 1
fi

echo "Creating secret '$SECRET_NAME' in namespace '$NAMESPACE' from '$CREDENTIALS_FILE'..."

kubectl create secret generic "$SECRET_NAME" \
  --namespace "$NAMESPACE" \
  --from-file="${SECRET_KEY}=${CREDENTIALS_FILE}" \
  --dry-run=client -o yaml | kubectl apply -f -

echo "Done."
