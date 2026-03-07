package nebius

import (
	"fmt"
	"net/url"

	"github.com/Azure/karpenter-provider-azure/pkg/operator/options"
	labelspkg "github.com/Azure/karpenter-provider-azure/pkg/providers/labels"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
	"sigs.k8s.io/karpenter/pkg/utils/resources"

	stretchapi "github.com/Azure/aks-flex/plugin/api"
	"github.com/Azure/aks-flex/plugin/pkg/services/agentpools/api/features/capacity"
	"github.com/Azure/aks-flex/plugin/pkg/services/agentpools/api/features/gpu"
	"github.com/Azure/aks-flex/plugin/pkg/services/agentpools/api/features/kubeadm"
	"github.com/Azure/aks-flex/plugin/pkg/services/agentpools/api/features/wireguard"
	nebiusinstance "github.com/Azure/aks-flex/plugin/pkg/services/agentpools/nebius/instance"
	"github.com/Azure/aks-flex/plugin/pkg/topology"

	"github.com/Azure/aks-flex/karpenter/pkg/apis/v1alpha1"
	"github.com/Azure/aks-flex/karpenter/pkg/cloudproviders"
	"github.com/Azure/aks-flex/karpenter/pkg/cloudproviders/nebius/instancetype"
)

func filterNoneZeroResource(
	_ corev1.ResourceName,
	v resource.Quantity,
) bool {
	return !resources.IsZero(v)
}

func providerIDToAgentPoolName(providerID string) (string, error) {
	parsedURL, err := url.Parse(providerID)
	if err != nil {
		return "", fmt.Errorf("parsing providerID %q: %w", providerID, err)
	}
	if parsedURL.Scheme != ProviderIDScheme {
		return "", fmt.Errorf("unexpected providerID scheme %q, expected %q", parsedURL.Scheme, ProviderIDScheme)
	}
	// aks-neibus://<agentPoolName> -> <agentPoolName>
	return parsedURL.Host, nil
}

func agentPoolNameToProviderID(agentPoolName string) string {
	// <agentPoolName> -> aks-neibus://<agentPoolName>
	return fmt.Sprintf("%s://%s", ProviderIDScheme, agentPoolName)
}

func stretchAgentPoolToNodeClaim(
	agentPool *nebiusinstance.AgentPool,
	instanceType *cloudprovider.InstanceType,
) *v1.NodeClaim {
	rv := &v1.NodeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:              agentPool.GetMetadata().GetId(),
			Labels:            map[string]string{},
			Annotations:       map[string]string{},
			CreationTimestamp: metav1.NewTime(agentPool.GetStatus().GetCreatedAt().AsTime()),
		},
		Spec: v1.NodeClaimSpec{},
		Status: v1.NodeClaimStatus{
			ProviderID: agentPoolNameToProviderID(agentPool.GetMetadata().GetId()),
		},
	}

	// TODO: populate deleting state to deletion timestamp

	rv.Labels = labelspkg.GetAllSingleValuedRequirementLabels(instanceType.Requirements)
	rv.Status.Capacity = lo.PickBy(instanceType.Capacity, filterNoneZeroResource)
	rv.Status.Allocatable = lo.PickBy(instanceType.Allocatable(), filterNoneZeroResource)

	// restoring zone info if possible
	if zone := agentPool.GetSpec().GetZone(); zone != "" {
		rv.Labels[corev1.LabelTopologyZone] = zone
	}
	// restoring capacity type info if possible
	switch agentPool.GetSpec().GetCapacity().GetCapacityType() {
	case capacity.CapacityType_CAPACITY_TYPE_ON_DEMAND:
		rv.Labels[v1.CapacityTypeLabelKey] = v1.CapacityTypeOnDemand
	case capacity.CapacityType_CAPACITY_TYPE_SPOT:
		rv.Labels[v1.CapacityTypeLabelKey] = v1.CapacityTypeSpot
	case capacity.CapacityType_CAPACITY_TYPE_RESERVED:
		rv.Labels[v1.CapacityTypeLabelKey] = v1.CapacityTypeReserved
	default:
		rv.Labels[v1.CapacityTypeLabelKey] = v1.CapacityTypeOnDemand
	}

	return rv
}

func nodeClaimToStretchAgentPool(
	karpOpts *options.Options,
	clusterVersion string,
	clusterCA []byte,
	nodeClass *v1alpha1.NebiusNodeClass,
	nodeClaim *v1.NodeClaim,
	launchSettings *instancetype.PlatformPresetLaunchSettings,
	wgConfig *wireguard.Config,
) *nebiusinstance.AgentPool {
	mdBuilder := stretchapi.Metadata_builder{
		Id: lo.ToPtr(nodeClaim.Name),
	}

	platform := launchSettings.Platform()

	var gpuConfig *gpu.Config
	imageFamily := lo.FromPtrOr(nodeClass.Spec.OSDiskImageFamily, "ubuntu24.04-driverless")
	if platform.GetSpec().GetGpuMemoryGigabytes() > 0 {
		// the platform has GPU, so use a GPU image by default
		// FIXME: we should add a default GPU image family field in the node class spec
		imageFamily = "ubuntu24.04-cuda12"
		gpuConfig = gpu.Config_builder{}.Build()
	}
	osDiskSize := lo.FromPtrOr(
		nodeClass.Spec.OSDiskSizeGB,
		// default to 100GB, which is the default for Nebius agent pools
		100,
	)

	capacityConfigBuilder := capacity.Config_builder{
		Capacity: lo.ToPtr(uint32(1)),
	}
	switch launchSettings.CapacityType {
	case v1.CapacityTypeSpot:
		capacityConfigBuilder.CapacityType = lo.ToPtr(capacity.CapacityType_CAPACITY_TYPE_SPOT)
	case v1.CapacityTypeOnDemand:
		capacityConfigBuilder.CapacityType = lo.ToPtr(capacity.CapacityType_CAPACITY_TYPE_ON_DEMAND)
	case v1.CapacityTypeReserved:
		capacityConfigBuilder.CapacityType = lo.ToPtr(capacity.CapacityType_CAPACITY_TYPE_RESERVED)
	default:
		capacityConfigBuilder.CapacityType = lo.ToPtr(capacity.CapacityType_CAPACITY_TYPE_ON_DEMAND)
	}

	kubeadmConfig := kubeadm.Config_builder{
		Server:                   lo.ToPtr(karpOpts.ClusterEndpoint),
		CertificateAuthorityData: clusterCA,
		Token:                    lo.ToPtr(karpOpts.KubeletClientTLSBootstrapToken),
		NodeLabels: map[string]string{
			// NOTE: this is needed for assigning right provider id after creation
			cloudproviders.NodeClaimLabelKey:          nodeClaim.Name,
			topology.NodeLabelKeyCloudProviderManaged: "false",
			topology.NodeLabelKeyCloudProviderCluster: karpOpts.NodeResourceGroup,
			topology.NodeLabelKeyStretchManaged:       "true",
		},
	}.Build()
	// NOTE: the following labels are required by karpenter consolidation
	kubeadmConfig.AddNodeLabels(map[string]string{
		// NOTE: this needs to match the value returned by GetInstanceTypes
		corev1.LabelInstanceTypeStable: launchSettings.InstanceTypeName(),
		corev1.LabelTopologyZone:       launchSettings.Zone,
		corev1.LabelTopologyRegion:     nodeClass.Spec.Region,
		v1.CapacityTypeLabelKey:        launchSettings.CapacityType,
	})
	// NOTE: forcing user mode for karpenter created nodes
	// FIXME: confirm in which place we should set this label
	kubeadmConfig.AddNodeLabels(map[string]string{
		"kubernetes.azure.com/mode": "user",
	})
	// NOTE: needed for karpenter disruption
	kubeadmConfig.AddK8SRegisterTaints(v1.UnregisteredNoExecuteTaint)

	specBuilder := nebiusinstance.AgentPoolSpec_builder{
		ProjectId:           lo.ToPtr(nodeClass.Spec.ProjectID),
		Region:              lo.ToPtr(nodeClass.Spec.Region),
		SubnetId:            lo.ToPtr(nodeClass.Spec.SubnetID),
		Platform:            lo.ToPtr(platform.GetMetadata().GetName()),
		Preset:              lo.ToPtr(launchSettings.Preset().GetName()),
		ImageFamily:         lo.ToPtr(imageFamily),
		OsDiskSizeGibibytes: lo.ToPtr(int64(osDiskSize)),
		Kubeadm:             kubeadmConfig,
		Wireguard:           wgConfig,
		Gpu:                 gpuConfig,
		Capacity:            capacityConfigBuilder.Build(),
		KubernetesVersion:   lo.ToPtr(clusterVersion),
	}

	return nebiusinstance.AgentPool_builder{
		Metadata: mdBuilder.Build(),
		Spec:     specBuilder.Build(),
	}.Build()
}
