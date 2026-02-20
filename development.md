# Development

## Prerequisites

- `kubectl` configured with access to the target cluster
- `helm` v3+
- Go 1.26+ (for local builds)

## Deploying the Controller

### 1. Create the namespace

```bash
kubectl create namespace karpenter --dry-run=client -o yaml | kubectl apply -f -
```

### 2. Install CRDs

```bash
kubectl apply -f ./vendor/github.com/Azure/karpenter-provider-azure/pkg/apis/crds
kubectl apply -f ./pkg/apis/crds
```

### 3. Upload Nebius credentials

The controller optionally accepts a Nebius credentials file. Upload it as a
Kubernetes Secret before installing the chart:

```bash
./hack/upload-nebius-credentials.sh /path/to/credentials.json
```

This creates a Secret named `nebius-credentials` in the `karpenter` namespace.
You can customise the namespace and secret name:

```bash
./hack/upload-nebius-credentials.sh /path/to/credentials.json <namespace> <secret-name>
```

### 4. Install with Helm

The full set of flags used during development (see `start_hbc.sh`) can be
passed via `--set` overrides. Replace the placeholder values below with your own:

```bash
helm upgrade --install karpenter charts/karpenter \
  --namespace karpenter \
  --set settings.clusterName="aks" \
  --set settings.clusterEndpoint="<cluster-api-server-url>" \
  --set logLevel=debug \
  --set controller.nebiusCredentials.enabled=true \
  --set controller.env[0].name=ARM_CLOUD,controller.env[0].value=AzurePublicCloud \
  --set controller.env[1].name=LOCATION,controller.env[1].value=eastus2 \
  --set controller.env[2].name=ARM_RESOURCE_GROUP,controller.env[2].value=<resource-group> \
  --set controller.env[3].name=AZURE_TENANT_ID,controller.env[3].value=<tenant-id> \
  --set controller.env[4].name=AZURE_SUBSCRIPTION_ID,controller.env[4].value=<subscription-id> \
  --set controller.env[5].name=AZURE_NODE_RESOURCE_GROUP,controller.env[5].value=<node-resource-group> \
  --set controller.env[6].name=SSH_PUBLIC_KEY,controller.env[6].value="<ssh-public-key>" \
  --set controller.env[7].name=VNET_SUBNET_ID,controller.env[7].value=<vnet-subnet-id> \
  --set controller.env[8].name=KUBELET_BOOTSTRAP_TOKEN,controller.env[8].value=<bootstrap-token> \
  --set-string controller.env[9].name=DISABLE_LEADER_ELECTION,controller.env[9].value=true
```

To override the image tag (e.g. when testing a local build):

```bash
helm upgrade --install karpenter charts/karpenter \
  --namespace karpenter \
  --set controller.image.tag=<tag> \
  --set controller.image.digest="" \
  ...
```

### 5. Verify

```bash
kubectl -n karpenter get pods
kubectl -n karpenter logs -l app.kubernetes.io/name=karpenter
```

## Running Locally

You can also run the controller directly on your machine (useful for
debugging). This is equivalent to the Helm deployment above but runs outside
the cluster:

```bash
export KUBECONFIG=/path/to/kubeconfig

export CLUSTER_NAME="aks"
export ARM_CLOUD="AzurePublicCloud"
export LOCATION="eastus2"
export ARM_RESOURCE_GROUP="<resource-group>"
export AZURE_TENANT_ID="<tenant-id>"
export AZURE_SUBSCRIPTION_ID="<subscription-id>"
export AZURE_TOKEN_CREDENTIALS="dev"

CLUSTER_API_SERVER=$(kubectl config view --minify -o jsonpath='{.clusters[0].cluster.server}')
NODE_RESOURCE_GROUP=$(kubectl get nodes -o jsonpath="{.items[0].metadata.labels.kubernetes\.azure\.com/cluster}")

go run ./cmd/controller \
  -cluster-endpoint "${CLUSTER_API_SERVER}" \
  -cluster-name "${CLUSTER_NAME}" \
  -disable-leader-election \
  -kubelet-bootstrap-token "<bootstrap-token>" \
  -node-resource-group "${NODE_RESOURCE_GROUP}" \
  -ssh-public-key "<ssh-public-key>" \
  -vnet-subnet-id "<vnet-subnet-id>" \
  -flex-nebius.credentials-file="/path/to/credentials.json" \
  -log-level=debug
```
