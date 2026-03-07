package instancetype

import (
	"fmt"

	azinstancetype "github.com/Azure/karpenter-provider-azure/pkg/providers/instancetype" // TODO: port logic to here
	nebiuscomputev1 "github.com/nebius/gosdk/proto/nebius/compute/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/karpenter/pkg/utils/resources"
)

const ArchitectureAmd64 = "amd64" // NOTE: nebius currently only supports amd64 architecture

// refs:
//
// - https://docs.nebius.com/compute/virtual-machines/types
// - https://karpenter.sh/docs/concepts/nodepools/#capacity-type
// - https://karpenter.sh/docs/concepts/nodepools/#availability-zones

// PlatformPreset holds the Nebius <platform, preset> pair, which represents the compute resource.
type PlatformPreset struct {
	platform *nebiuscomputev1.Platform
	preset   *nebiuscomputev1.Preset
}

// NewPlatformPreset constructs a PlatformPreset from the raw Nebius proto types.
func NewPlatformPreset(platform *nebiuscomputev1.Platform, preset *nebiuscomputev1.Preset) *PlatformPreset {
	return &PlatformPreset{
		platform: platform,
		preset:   preset,
	}
}

// Platform returns the underlying Nebius Platform proto.
func (p *PlatformPreset) Platform() *nebiuscomputev1.Platform {
	return p.platform
}

// Preset returns the underlying Nebius Preset proto.
func (p *PlatformPreset) Preset() *nebiuscomputev1.Preset {
	return p.preset
}

// AllowedForPreemptibles returns true if the platform supports preemptible (spot) instances.
func (p *PlatformPreset) AllowedForPreemptibles() bool {
	return p.platform.GetStatus().GetAllowedForPreemptibles()
}

// InstanceTypeName returns the fully qualified instance type name.
//
// example: cpu-d3-4vcpu-16gb
//
// NOTE: we don't use "cpu-d3" like nebius does because we need to distinguish
// different presets under the same platform for karpenter scheduling/consolidation purposes.
func (p *PlatformPreset) InstanceTypeName() string {
	return p.platform.GetMetadata().GetName() + "-" + p.preset.GetName()
}

func (p *PlatformPreset) VCPUCount() *resource.Quantity {
	return resources.Quantity(fmt.Sprint(p.preset.GetResources().GetVcpuCount()))
}

func (p *PlatformPreset) MemoryGiB() *resource.Quantity {
	valueGiB := p.preset.GetResources().GetMemoryGibibytes()
	return resource.NewScaledQuantity(int64(valueGiB), resource.Giga)
}

func (p *PlatformPreset) NvidiaGPUCount() *resource.Quantity {
	return resources.Quantity(fmt.Sprint(p.preset.GetResources().GetGpuCount()))
}

func (p *PlatformPreset) Architecture() string {
	// NOTE: nebius currently only supports amd64 architecture
	return ArchitectureAmd64
}

func (p *PlatformPreset) KubeReservedResources() corev1.ResourceList {
	return azinstancetype.KubeReservedResources(
		int64(p.preset.GetResources().GetVcpuCount()),
		float64(p.preset.GetResources().GetMemoryGibibytes()),
	)
}

func (p *PlatformPreset) SystemReservedResource() corev1.ResourceList {
	return azinstancetype.SystemReservedResources()
}

func (p *PlatformPreset) EvictionThreshold() corev1.ResourceList {
	return azinstancetype.EvictionThreshold()
}

// PlatformPresetLaunchSettings returns the launch settings for this platform preset
// with the given capacity type and zone.
type PlatformPresetLaunchSettings struct {
	*PlatformPreset
	CapacityType string // one of "on-demand" / "spot" / "reserved"
	Zone         string
}
