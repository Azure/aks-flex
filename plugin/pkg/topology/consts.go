package topology

// cloud provider specified labels

const (
	// NOTE: needs to set to false so azure cloud provider skips deleting nodes created by stretch.
	NodeLabelKeyCloudProviderManaged = "kubernetes.azure.com/managed"
	// Sets to MC resource group name.
	NodeLabelKeyCloudProviderCluster = "kubernetes.azure.com/cluster"
	// Sets to true to indicate nodes are managed by stretch.
	NodeLabelKeyStretchManaged = "aks.azure.com/stretch-managed"
)

// universal labels

const (
	NodeLabelKeyCloud  = "aks.azure.com/cloud"
	NodeLabelKeyRegion = "aks.azure.com/region"
	NodeLabelKeyZone   = "aks.azure.com/zone"

	NodeLabelKeyInstanceType = "aks.azure.com/instance-type"
)
