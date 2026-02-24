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

	stretchapi "github.com/Azure/aks-flex/flex-plugin/api"
	"github.com/Azure/aks-flex/flex-plugin/pkg/services/agentpools/api/features/kubeadm"
	"github.com/Azure/aks-flex/flex-plugin/pkg/services/agentpools/api/features/wireguard"
	nebiusinstance "github.com/Azure/aks-flex/flex-plugin/pkg/services/agentpools/nebius/instance"
	"github.com/Azure/aks-flex/flex-plugin/pkg/topology"

	"github.com/Azure/aks-flex/karpenter-provider-flex/pkg/apis/v1alpha1"
	"github.com/Azure/aks-flex/karpenter-provider-flex/pkg/cloudproviders"
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

	// TODO: zone from instance
	// TODO: labels from instance

	return rv
}

func nodeClaimToStretchAgentPool(
	karpOpts *options.Options,
	clusterCA []byte,
	nodeClass *v1alpha1.NebiusNodeClass,
	nodeClaim *v1.NodeClaim,
	platformPreset *platformPreset,
	wgConfig *wireguard.Config,
) *nebiusinstance.AgentPool {
	mdBuilder := stretchapi.Metadata_builder{
		Id: lo.ToPtr(nodeClaim.Name),
	}

	platform := platformPreset.platform

	imageFamily := lo.FromPtrOr(nodeClass.Spec.OSDiskImageFamily, "ubuntu24.04-driverless")
	if platform.GetSpec().GetGpuMemoryGigabytes() > 0 {
		// the platform has GPU, so use a GPU image by default
		imageFamily = "ubuntu24.04-cuda12"
	}
	osDiskSize := lo.FromPtrOr(
		nodeClass.Spec.OSDiskSizeGB,
		// default to 100GB, which is the default for Nebius agent pools
		100,
	)

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
	// NOTE: the following 3 labels are required by karpenter consolidation
	kubeadmConfig.AddNodeLabels(map[string]string{
		// FIXME: this should set in stretch api side
		// NOTE: this needs to match the value returned by GetInstanceTypes
		corev1.LabelInstanceTypeStable: platformPreset.InstanceTypeName(),
		// FIXME: does nebius provide zone information?
		corev1.LabelTopologyZone: "1",
		// FIXME: this should be determined based on the platform preset
		v1.CapacityTypeLabelKey: "on-demand",
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
		Preset:              lo.ToPtr(platformPreset.preset.GetName()),
		ImageFamily:         lo.ToPtr(imageFamily),
		OsDiskSizeGibibytes: lo.ToPtr(int64(osDiskSize)),
		Kubeadm:             kubeadmConfig,
		Wireguard:           wgConfig,
	}

	return nebiusinstance.AgentPool_builder{
		Metadata: mdBuilder.Build(),
		Spec:     specBuilder.Build(),
	}.Build()
}
