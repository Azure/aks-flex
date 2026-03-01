package nebius

import (
	"context"
	"fmt"
	"iter"

	nebiusinstance "github.com/Azure/aks-flex/plugin/pkg/services/agentpools/nebius/instance"
	"github.com/Azure/karpenter-provider-azure/pkg/providers/instancetype"
	"github.com/nebius/gosdk"
	nebiuscommonv1 "github.com/nebius/gosdk/proto/nebius/common/v1"
	nebiuscomputev1 "github.com/nebius/gosdk/proto/nebius/compute/v1"
	"google.golang.org/protobuf/proto"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	v1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	corecloudprovider "sigs.k8s.io/karpenter/pkg/cloudprovider"
	"sigs.k8s.io/karpenter/pkg/scheduling"
	"sigs.k8s.io/karpenter/pkg/utils/resources"
)

type platformPreset struct {
	platform *nebiuscomputev1.Platform
	preset   *nebiuscomputev1.Preset
	region   string
}

// example: cpu-d3-4vcpu-16gb
func (p *platformPreset) InstanceTypeName() string {
	return fmt.Sprintf("%s-%s", p.platform.GetMetadata().GetName(), p.preset.GetName())
}

func (p *platformPreset) ToInstanceType() *corecloudprovider.InstanceType {
	instanceTypeName := p.InstanceTypeName()
	preset := p.preset

	// TODO: fix this mess
	vcpusCount := fmt.Sprint(preset.GetResources().GetVcpuCount())
	memoryGiB := fmt.Sprintf("%dGi", preset.GetResources().GetMemoryGibibytes())
	gpuCount := fmt.Sprint(preset.GetResources().GetGpuCount())

	// NOTE: nebius doesn't have the concept of availability zone. However, karpenter requires zone
	// information for scheduling/consolidation purposes. Therefore, we fallback to using region as zone for now.
	zones := []string{p.region}

	return &corecloudprovider.InstanceType{
		Name: instanceTypeName,
		Requirements: scheduling.NewRequirements(
			// well-known upstream
			scheduling.NewRequirement(corev1.LabelInstanceTypeStable, corev1.NodeSelectorOpIn, instanceTypeName),
			scheduling.NewRequirement(corev1.LabelTopologyZone, corev1.NodeSelectorOpIn, zones...),
			scheduling.NewRequirement(corev1.LabelTopologyRegion, corev1.NodeSelectorOpIn, p.region),
			scheduling.NewRequirement(corev1.LabelOSStable, corev1.NodeSelectorOpIn, string(corev1.Linux)),
			// TODO: add corev1.LabelArchStable
			// wel-known for karpenter
			scheduling.NewRequirement(v1.CapacityTypeLabelKey, corev1.NodeSelectorOpIn, v1.CapacityTypeOnDemand), // FIXME: this should be determined based on the platform preset
			// TODO: add Azure/Flex well known labels
		),
		// FIXME: extract this part out
		Offerings: corecloudprovider.Offerings{
			// FIXME: determine real availability zones from Nebius platform data
			{
				Price:     1000, // FIXME: calculate real price
				Available: true,
				Requirements: scheduling.NewRequirements(
					scheduling.NewRequirement(v1.CapacityTypeLabelKey, corev1.NodeSelectorOpIn, v1.CapacityTypeOnDemand), // FIXME: this should be determined based on the platform preset
					scheduling.NewRequirement(corev1.LabelTopologyZone, corev1.NodeSelectorOpIn, zones...),
				),
			},
		},
		Capacity: corev1.ResourceList{
			corev1.ResourceCPU:                    *resources.Quantity(vcpusCount),
			corev1.ResourceMemory:                 *resources.Quantity(memoryGiB),
			corev1.ResourceEphemeralStorage:       *resource.NewScaledQuantity(100, resource.Giga), // FIXME: read from node class
			corev1.ResourcePods:                   *resources.Quantity("110"),                      // FIXME: read from node class
			corev1.ResourceName("nvidia.com/gpu"): *resources.Quantity(gpuCount),
		},
		Overhead: &corecloudprovider.InstanceTypeOverhead{
			KubeReserved: instancetype.KubeReservedResources(
				int64(preset.Resources.VcpuCount),
				float64(preset.Resources.MemoryGibibytes),
			),
			// TODO: calculate system reserved
			SystemReserved: corev1.ResourceList{
				corev1.ResourceCPU:    resource.Quantity{},
				corev1.ResourceMemory: resource.Quantity{},
			},
			// TODO: calculate eviction threshold
			EvictionThreshold: instancetype.EvictionThreshold(),
		},
	}
}

func (p *platformPreset) IsCheaperThan(other *platformPreset) bool {
	// Lazy implementation to assume price is based on vCPU count & memory
	// FIXME: calculate based on real price
	pVCPUCount := p.preset.GetResources().GetVcpuCount()
	otherVCPUCount := other.preset.GetResources().GetVcpuCount()

	if pVCPUCount < otherVCPUCount {
		return true
	}
	if pVCPUCount == otherVCPUCount {
		pMemory := p.preset.GetResources().GetMemoryGibibytes()
		otherMemory := other.preset.GetResources().GetMemoryGibibytes()
		return pMemory < otherMemory
	}
	return false
}

func (p *platformPreset) DeepClone() *platformPreset {
	return &platformPreset{
		platform: proto.Clone(p.platform).(*nebiuscomputev1.Platform),
		preset:   proto.Clone(p.preset).(*nebiuscomputev1.Preset),
		region:   p.region,
	}
}

func filterPlatformPresets(
	ctx context.Context,
	projectID string,
	region string,
	sdk *gosdk.SDK,
) iter.Seq2[platformPreset, error] {
	req := &nebiuscomputev1.ListPlatformsRequest{
		ParentId: projectID,
	}
	platformSeq := sdk.Services().Compute().V1().Platform().Filter(ctx, req)

	return func(yield func(platformPreset, error) bool) {
		for platform, err := range platformSeq {
			if err != nil {
				if !yield(platformPreset{}, err) {
					return
				}
			}

			for _, preset := range platform.GetSpec().GetPresets() {
				pp := platformPreset{
					platform: platform,
					preset:   preset,
					region:   region,
				}
				if !yield(pp, nil) {
					return
				}
			}
		}
	}
}

func resolvePlatformPresetFromNodeClaim(
	ctx context.Context,
	projectID string,
	region string,
	sdk *gosdk.SDK,
	nodeClaim *v1.NodeClaim,
) (*platformPreset, error) {
	// Lazy implementation to just match instance type names
	// FIXME: should we do a fit check on all requirements again?
	instanceTypeNameSet := map[string]struct{}{}
	for _, req := range nodeClaim.Spec.Requirements {
		if req.Key != corev1.LabelInstanceTypeStable {
			continue
		}
		for _, value := range req.Values {
			instanceTypeNameSet[value] = struct{}{}
		}
		break
	}

	var rv *platformPreset
	for item, err := range filterPlatformPresets(ctx, projectID, region, sdk) {
		if err != nil {
			return nil, fmt.Errorf("list platform presets: %w", err)
		}

		_, requested := instanceTypeNameSet[item.InstanceTypeName()]
		if !requested {
			continue
		}

		if rv == nil || item.IsCheaperThan(rv) {
			rv = item.DeepClone()
		}
	}

	if rv != nil {
		return rv, nil
	}

	return nil, fmt.Errorf("no matching platform preset found")
}

func resolvePlatformPresetFromInstance(
	ctx context.Context,
	projectID string,
	region string,
	sdk *gosdk.SDK,
	agentPool *nebiusinstance.AgentPool,
) (*platformPreset, error) {
	platformName := agentPool.GetSpec().GetPlatform()
	presetName := agentPool.GetSpec().GetPreset()

	getReq := &nebiuscommonv1.GetByNameRequest{
		ParentId: projectID,
		Name:     platformName,
	}
	platform, err := sdk.Services().Compute().V1().Platform().GetByName(ctx, getReq)
	if err != nil {
		return nil, fmt.Errorf("get platform %q: %w", platformName, err)
	}

	var preset *nebiuscomputev1.Preset
	for _, p := range platform.GetSpec().GetPresets() {
		if p.GetName() == presetName {
			preset = p
			break
		}
	}
	if preset == nil {
		return nil, fmt.Errorf("preset %q not found in platform %q", presetName, platformName)
	}

	return &platformPreset{
		platform: platform,
		preset:   preset,
		region:   region,
	}, nil
}
