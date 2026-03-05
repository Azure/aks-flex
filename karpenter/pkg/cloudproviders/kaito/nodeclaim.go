package kaito

import (
	"context"
	"fmt"
	"net/url"

	"github.com/Azure/karpenter-provider-azure/pkg/operator/options"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "sigs.k8s.io/karpenter/pkg/apis/v1"

	stretchapi "github.com/Azure/aks-flex/plugin/api"
	"github.com/Azure/aks-flex/plugin/pkg/services/agentpools/api/features/kubeadm"
	nebiusinstance "github.com/Azure/aks-flex/plugin/pkg/services/agentpools/nebius/instance"
	"github.com/Azure/aks-flex/plugin/pkg/topology"

	"github.com/Azure/aks-flex/karpenter/pkg/cloudproviders"
	flexopts "github.com/Azure/aks-flex/karpenter/pkg/options"
)

// ```
// spec:
//   expireAfter: Never
//   nodeClassRef:
//     group: kaito.sh
//     kind: KaitoNodeClass
//     name: default
//   requirements:
//   - key: karpenter.sh/nodepool
//     operator: In
//     values:
//     - kaito
//   - key: node.kubernetes.io/instance-type
//     operator: In
//     values:
//     - Standard_NC6s_v3
//   - key: kubernetes.io/os
//     operator: In
//     values:
//     - linux
//   - key: karpenter.azure.com/sku-name
//     operator: In
//     values:
//     - Standard_NC6s_v3
//   resources:
//     requests:
//       storage: 70Gi
//   taints: # TODO: add claim?
//   - effect: NoSchedule
//     key: sku
//     value: gpu
// ```

func providerIDToAgentPoolName(providerID string) (string, error) {
	parsedURL, err := url.Parse(providerID)
	if err != nil {
		return "", fmt.Errorf("parsing providerID %q: %w", providerID, err)
	}
	if parsedURL.Scheme != ProviderIDScheme {
		return "", fmt.Errorf("unexpected providerID scheme %q, expected %q", parsedURL.Scheme, ProviderIDScheme)
	}
	// aks-kaito://<agentPoolName> -> <agentPoolName>
	return parsedURL.Host, nil
}

type nodeClaimRequirements struct {
	InstanceTypeRequested      string
	DiskSizeGibibytesRequested int64
}

func resolveNodeClaimRequirements(nodeClaim *v1.NodeClaim) (nodeClaimRequirements, error) {
	var rv nodeClaimRequirements

	for _, req := range nodeClaim.Spec.Requirements {
		switch req.Key {
		case corev1.LabelInstanceTypeStable:
			if len(req.Values) > 0 {
				rv.InstanceTypeRequested = req.Values[0]
			}
		case "karpenter.azure.com/sku-name": // FIXME: who defines this label?
			if rv.InstanceTypeRequested == "" && len(req.Values) > 0 {
				// NOTE: prefer corev1.LabelInstanceTypeStable, but allow fallback to this
				// if the stable label is not provided
				rv.InstanceTypeRequested = req.Values[0]
			}
		}
	}

	if rv.InstanceTypeRequested == "" {
		var zero nodeClaimRequirements
		return zero, fmt.Errorf("no instance type requirement from kaito node claim: %q", nodeClaim.Name)
	}

	for resourceName, quantity := range nodeClaim.Spec.Resources.Requests {
		switch resourceName {
		case corev1.ResourceStorage:
			rv.DiskSizeGibibytesRequested = quantity.Value() / (1024 * 1024 * 1024)
		}
	}
	if rv.DiskSizeGibibytesRequested <= 0 {
		var zero nodeClaimRequirements
		return zero, fmt.Errorf("invalid disk size requirement from kaito node claim: %q", nodeClaim.Name)
	}

	return rv, nil
}

type resolvedNebiusAgentPoolSettings struct {
	Platform            string
	Preset              string
	ImageFamily         string
	OSDiskSizeGibibytes int64
}

var azureGPUInstanceTypeToNebiusPlatformPreset = map[string]struct {
	Platform    string
	Preset      string
	ImageFamily string
}{
	// ref: https://learn.microsoft.com/en-us/azure/virtual-machines/sizes/gpu-accelerated/ncv3-series?tabs=sizeaccelerators
	"Standard_NC6s_v3": {
		Platform:    "gpu-h100-sxm",
		Preset:      "1gpu-16vcpu-200gb",
		ImageFamily: "ubuntu24.04-cuda12",
	},
}

func resolveNebiusAgentPoolSettings(
	ctx context.Context,
	kaitoOpts *flexopts.KaitoOptions,
	nodeClaim *v1.NodeClaim,
	nodeClaimReqs nodeClaimRequirements,
) (resolvedNebiusAgentPoolSettings, error) {
	var rv resolvedNebiusAgentPoolSettings

	pp, exists := azureGPUInstanceTypeToNebiusPlatformPreset[nodeClaimReqs.InstanceTypeRequested]
	if !exists {
		var zero resolvedNebiusAgentPoolSettings
		return zero, fmt.Errorf("no predefined mapping for SKU %q in nebius", nodeClaimReqs.InstanceTypeRequested)
	}
	rv.Platform = pp.Platform
	rv.Preset = pp.Preset
	rv.ImageFamily = pp.ImageFamily

	const minimalOSDiskSizeGibibytes = 100
	rv.OSDiskSizeGibibytes = max(
		nodeClaimReqs.DiskSizeGibibytesRequested+30, // add 30 GiB buffer for OS deps
		minimalOSDiskSizeGibibytes,
	)

	return rv, nil
}

func nodeClaimToNebiusAgentPool( // TODO: support more remote cloud types
	karpOpts *options.Options,
	kaitoOpts *flexopts.KaitoOptions,
	clusterCA []byte,
	nodeClaim *v1.NodeClaim,
	nodeClaimReqs nodeClaimRequirements,
	resolvedSettings resolvedNebiusAgentPoolSettings,
) *nebiusinstance.AgentPool {
	mdBuilder := stretchapi.Metadata_builder{
		Id: lo.ToPtr(nodeClaim.Name),
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
	// NOTE: the following 3 labels are required by karpenter consolidation
	kubeadmConfig.AddNodeLabels(map[string]string{
		// FIXME: we do a hack here to "map" the requested instance type
		corev1.LabelInstanceTypeStable: nodeClaimReqs.InstanceTypeRequested,
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
		ProjectId:           lo.ToPtr(kaitoOpts.NebiusProjectID),
		Region:              lo.ToPtr(kaitoOpts.NebiusRegion),
		SubnetId:            lo.ToPtr(kaitoOpts.NebiusSubnetID),
		Platform:            lo.ToPtr(resolvedSettings.Platform),
		Preset:              lo.ToPtr(resolvedSettings.Preset),
		ImageFamily:         lo.ToPtr(resolvedSettings.ImageFamily),
		OsDiskSizeGibibytes: lo.ToPtr(resolvedSettings.OSDiskSizeGibibytes),
		Kubeadm:             kubeadmConfig,
	}

	return nebiusinstance.AgentPool_builder{
		Metadata: mdBuilder.Build(),
		Spec:     specBuilder.Build(),
	}.Build()
}

func agentPoolNameToProviderID(agentPoolName string) string {
	// <agentPoolName> -> aks-kaito://<agentPoolName>
	return fmt.Sprintf("%s://%s", ProviderIDScheme, agentPoolName)
}

func strechAgentPoolToNodeClaim(
	agentPool *nebiusinstance.AgentPool,
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

	// FIXME
	// rv.Labels = labelspkg.GetAllSingleValuedRequirementLabels(instanceType.Requirements)
	// rv.Status.Capacity = lo.PickBy(instanceType.Capacity, filterNoneZeroResource)
	// rv.Status.Allocatable = lo.PickBy(instanceType.Allocatable(), filterNoneZeroResource)

	// TODO: zone from instance
	// TODO: labels from instance

	return rv
}
