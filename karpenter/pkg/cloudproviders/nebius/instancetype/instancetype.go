package instancetype

import (
	"fmt"

	azurev1beta1 "github.com/Azure/karpenter-provider-azure/pkg/apis/v1beta1"
	k8scorev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/sets"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	karpcloudprovider "sigs.k8s.io/karpenter/pkg/cloudprovider"
	"sigs.k8s.io/karpenter/pkg/scheduling"
)

// NewInstanceType returns the karpcloudprovider.InstanceType for the given platform preset.
func NewInstanceType(
	key NodeClassKey,
	platformPreset *PlatformPreset,
	offerings karpcloudprovider.Offerings,
) *karpcloudprovider.InstanceType {
	return &karpcloudprovider.InstanceType{
		Name:         platformPreset.InstanceTypeName(),
		Requirements: computeRequirements(key.Region, platformPreset, offerings),
		Offerings:    offerings,
		Capacity:     computeCapacity(key, platformPreset),
		Overhead:     computeOverhead(platformPreset),
	}
}

func computeRequirements(
	region string,
	p *PlatformPreset,
	offerings karpcloudprovider.Offerings,
) scheduling.Requirements {
	availableOfferings := offerings.Available()
	availableZones := sets.New[string]()
	availableCapacityTypes := sets.New[string]()
	for _, o := range availableOfferings {
		availableZones.Insert(o.Requirements.Get(k8scorev1.LabelTopologyZone).Values()...)
		availableCapacityTypes.Insert(o.Requirements.Get(karpv1.CapacityTypeLabelKey).Values()...)
	}

	vCPUCount := fmt.Sprint(p.VCPUCount().Value())
	memoryMIB := fmt.Sprint(p.MemoryGiB().ScaledValue(resource.Milli))
	gpuCount := fmt.Sprint(p.NvidiaGPUCount().Value())

	return scheduling.NewRequirements(
		// well-known to upstream
		scheduling.NewRequirement(k8scorev1.LabelInstanceTypeStable, k8scorev1.NodeSelectorOpIn, p.InstanceTypeName()),
		scheduling.NewRequirement(k8scorev1.LabelTopologyZone, k8scorev1.NodeSelectorOpIn, availableZones.UnsortedList()...),
		scheduling.NewRequirement(k8scorev1.LabelTopologyRegion, k8scorev1.NodeSelectorOpIn, region),
		scheduling.NewRequirement(k8scorev1.LabelOSStable, k8scorev1.NodeSelectorOpIn, string(k8scorev1.Linux)),
		scheduling.NewRequirement(k8scorev1.LabelArchStable, k8scorev1.NodeSelectorOpIn, p.Architecture()),
		// well-known to karpenter
		scheduling.NewRequirement(karpv1.CapacityTypeLabelKey, k8scorev1.NodeSelectorOpIn, availableCapacityTypes.UnsortedList()...),
		// well known to Azure
		scheduling.NewRequirement(azurev1beta1.LabelSKUCPU, k8scorev1.NodeSelectorOpIn, vCPUCount),
		scheduling.NewRequirement(azurev1beta1.LabelSKUMemory, k8scorev1.NodeSelectorOpIn, memoryMIB), // in MiB
		scheduling.NewRequirement(azurev1beta1.AKSLabelCPU, k8scorev1.NodeSelectorOpIn, vCPUCount),    // AKS domain.
		scheduling.NewRequirement(azurev1beta1.AKSLabelMemory, k8scorev1.NodeSelectorOpIn, memoryMIB), // AKS domain.
		scheduling.NewRequirement(azurev1beta1.LabelSKUGPUCount, k8scorev1.NodeSelectorOpIn, gpuCount),
		// TODO: delare Flex well known labels here
	)
}

func computeCapacity(
	key NodeClassKey,
	p *PlatformPreset,
) k8scorev1.ResourceList {
	ephermalStorage := *resource.NewScaledQuantity(key.OSDiskSizeGiB, resource.Giga)
	podsCount := resource.MustParse(fmt.Sprintf("%d", key.PerNodePodsCount))

	return k8scorev1.ResourceList{
		k8scorev1.ResourceCPU:                    *p.VCPUCount(),
		k8scorev1.ResourceMemory:                 *p.MemoryGiB(),
		k8scorev1.ResourceEphemeralStorage:       ephermalStorage,
		k8scorev1.ResourcePods:                   podsCount,
		k8scorev1.ResourceName("nvidia.com/gpu"): *p.NvidiaGPUCount(),
	}
}

func computeOverhead(
	p *PlatformPreset,
) *karpcloudprovider.InstanceTypeOverhead {
	return &karpcloudprovider.InstanceTypeOverhead{
		KubeReserved:      p.KubeReservedResources(),
		SystemReserved:    p.SystemReservedResource(),
		EvictionThreshold: p.EvictionThreshold(),
	}
}
