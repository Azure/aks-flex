package instancetype

import (
	"context"
	"strconv"
	"sync"

	nebiusbillingv1alpha1 "github.com/nebius/gosdk/proto/nebius/billing/v1alpha1"
	nebiuscommonv1 "github.com/nebius/gosdk/proto/nebius/common/v1"
	nebiuscomputev1 "github.com/nebius/gosdk/proto/nebius/compute/v1"
	nebiusbillingservice "github.com/nebius/gosdk/services/nebius/billing/v1alpha1"
	"golang.org/x/sync/errgroup"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"
	v1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	karpcloudprovider "sigs.k8s.io/karpenter/pkg/cloudprovider"
	"sigs.k8s.io/karpenter/pkg/scheduling"
)

const (
	// defaultPrice is a fallback hourly price when the billing calculator is unavailable.
	// Set intentionally high so that unknown-priced instance types sort last in OrderByPrice.
	defaultPrice = 1000.0
)

// Prices holds the hourly prices for a single platform/preset.
// When the billing API call fails, Err is set and OnDemand/Preemptible
// fall back to defaultPrice. Callers can inspect Err to distinguish
// between a real price and a fallback.
type Prices struct {
	OnDemand    float64
	Preemptible float64 // only meaningful when the platform allows preemptibles
	Err         error   // non-nil when the price could not be resolved from Nebius
}

// CreateOfferings builds the set of Karpenter offerings for a single PlatformPreset.
//
// Every instance type gets at least one on-demand offering.
// If the platform allows preemptible instances, a second offering with capacity type "spot"
// is added (Karpenter uses "spot" as the generic term for preemptible/interruptible).
//
// An offering is marked unavailable when:
//   - the pricing lookup failed (prices.Err != nil), or
//   - the instance type + zone + capacity type combination is present in the
//     unavailableOfferings cache (recent quota/capacity failure).
//
// The zone is set to the region because Nebius does not expose availability zones.
func CreateOfferings(
	ctx context.Context,
	p *PlatformPreset,
	region string,
	prices Prices,
	unavailableOfferings *UnavailableOfferings,
) karpcloudprovider.Offerings {
	pricingAvailable := true
	if prices.Err != nil {
		pricingAvailable = false
		log.FromContext(ctx).Error(prices.Err, "failed to resolve price for instance type, marking as unavailable",
			"instanceType", p.InstanceTypeName(),
		)
	}

	instanceTypeName := p.InstanceTypeName()

	onDemandAvailable := pricingAvailable && !unavailableOfferings.IsUnavailable(instanceTypeName, region, v1.CapacityTypeOnDemand)
	offerings := karpcloudprovider.Offerings{
		newOffering(v1.CapacityTypeOnDemand, region, prices.OnDemand, onDemandAvailable),
	}

	if p.AllowedForPreemptibles() {
		spotAvailable := pricingAvailable && !unavailableOfferings.IsUnavailable(instanceTypeName, region, v1.CapacityTypeSpot)
		offerings = append(offerings, newOffering(v1.CapacityTypeSpot, region, prices.Preemptible, spotAvailable))
	}

	return offerings
}

// newOffering constructs a single Karpenter offering with the mandatory requirements.
func newOffering(capacityType, zone string, price float64, available bool) *karpcloudprovider.Offering {
	return &karpcloudprovider.Offering{
		Price:     price,
		Available: available,
		Requirements: scheduling.NewRequirements(
			scheduling.NewRequirement(v1.CapacityTypeLabelKey, corev1.NodeSelectorOpIn, capacityType),
			scheduling.NewRequirement(corev1.LabelTopologyZone, corev1.NodeSelectorOpIn, zone),
		),
	}
}

// PricingProvider fetches hourly on-demand prices from the Nebius Billing Calculator API.
type PricingProvider struct {
	billingCalculator nebiusbillingservice.CalculatorService
}

// NewPricingProvider creates a PricingProvider backed by the given Nebius SDK.
func NewPricingProvider(billingCalculator nebiusbillingservice.CalculatorService) *PricingProvider {
	return &PricingProvider{billingCalculator: billingCalculator}
}

// PriceKey uniquely identifies a platform/preset/disk-size combination for price lookup.
type PriceKey struct {
	PlatformName  string
	PresetName    string
	OSDiskSizeGiB int64
}

// maxConcurrentPriceRequests limits the number of concurrent EstimateBatch API calls.
const maxConcurrentPriceRequests = 10

// GetPrices returns the hourly on-demand and preemptible prices for each PlatformPreset,
// including the cost of the OS disk. The returned map is keyed by PriceKey (which
// includes osDiskSizeGiB). Presets whose price cannot be determined are assigned
// defaultPrice with the error captured in Prices.Err.
// Preemptible prices are only fetched for platforms that allow preemptible instances.
//
// parentID is the Nebius project/container ID required by the billing calculator API.
//
// Pricing is fetched via the Nebius Billing Calculator EstimateBatch API. Each call
// bundles the compute instance spec together with the OS disk spec so the response
// contains the combined hourly cost.
func (pp *PricingProvider) GetPrices(
	ctx context.Context,
	parentID string,
	presets []*PlatformPreset,
	osDiskSizeGiB int64,
) map[PriceKey]Prices {
	if len(presets) == 0 {
		return map[PriceKey]Prices{}
	}

	var mu sync.Mutex
	prices := make(map[PriceKey]Prices, len(presets))

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(maxConcurrentPriceRequests)

	diskSpec := buildComputeDiskResourceSpec(parentID, osDiskSizeGiB)

	for _, p := range presets {
		g.Go(func() error {
			key := PriceKey{
				PlatformName:  p.platform.GetMetadata().GetName(),
				PresetName:    p.preset.GetName(),
				OSDiskSizeGiB: osDiskSizeGiB,
			}

			entry := Prices{
				OnDemand:    defaultPrice,
				Preemptible: defaultPrice,
			}

			// Fetch on-demand price: instance + OS disk combined.
			onDemandInstanceSpec := buildComputeInstanceResourceSpec(parentID, p, nil)
			onDemandResp, err := pp.billingCalculator.EstimateBatch(ctx, &nebiusbillingv1alpha1.EstimateBatchRequest{
				ResourceSpecs: []*nebiusbillingv1alpha1.ResourceSpec{onDemandInstanceSpec, diskSpec},
			})
			if err != nil {
				entry.Err = err
				mu.Lock()
				prices[key] = entry
				mu.Unlock()
				return nil
			}
			entry.OnDemand = extractHourlyPrice(onDemandResp.GetHourlyCost())

			// Fetch preemptible price if the platform supports it.
			if p.AllowedForPreemptibles() {
				preemptibleInstanceSpec := buildComputeInstanceResourceSpec(parentID, p, &nebiuscomputev1.PreemptibleSpec{
					OnPreemption: nebiuscomputev1.PreemptibleSpec_STOP,
				})
				preemptibleResp, err := pp.billingCalculator.EstimateBatch(ctx, &nebiusbillingv1alpha1.EstimateBatchRequest{
					ResourceSpecs: []*nebiusbillingv1alpha1.ResourceSpec{preemptibleInstanceSpec, diskSpec},
				})
				if err != nil {
					entry.Err = err
					mu.Lock()
					prices[key] = entry
					mu.Unlock()
					return nil
				}
				entry.Preemptible = extractHourlyPrice(preemptibleResp.GetHourlyCost())
			}

			mu.Lock()
			prices[key] = entry
			mu.Unlock()

			return nil
		})
	}

	_ = g.Wait() // individual errors are captured in Prices.Err
	return prices
}

// extractHourlyPrice parses the hourly cost from a ResourceGroupCost.
func extractHourlyPrice(cost *nebiusbillingv1alpha1.ResourceGroupCost) float64 {
	costStr := cost.GetGeneral().GetTotal().GetCostRounded()
	if costStr == "" {
		return defaultPrice
	}

	price, err := strconv.ParseFloat(costStr, 64)
	if err != nil {
		return defaultPrice
	}

	return price
}

// buildComputeInstanceResourceSpec creates the billing calculator ResourceSpec
// for estimating the cost of a compute instance with the given platform/preset.
// If preemptible is non-nil, the instance is priced as a preemptible VM.
func buildComputeInstanceResourceSpec(
	parentID string,
	p *PlatformPreset,
	preemptible *nebiuscomputev1.PreemptibleSpec,
) *nebiusbillingv1alpha1.ResourceSpec {
	return &nebiusbillingv1alpha1.ResourceSpec{
		ResourceSpec: &nebiusbillingv1alpha1.ResourceSpec_ComputeInstanceSpec{
			ComputeInstanceSpec: &nebiuscomputev1.CreateInstanceRequest{
				Metadata: &nebiuscommonv1.ResourceMetadata{
					ParentId: parentID,
				},
				Spec: &nebiuscomputev1.InstanceSpec{
					Resources: &nebiuscomputev1.ResourcesSpec{
						Platform: p.platform.GetMetadata().GetName(),
						Size: &nebiuscomputev1.ResourcesSpec_Preset{
							Preset: p.preset.GetName(),
						},
					},
					Preemptible: preemptible,
				},
			},
		},
	}
}

// buildComputeDiskResourceSpec creates the billing calculator ResourceSpec
// for estimating the cost of an OS disk with the given size.
func buildComputeDiskResourceSpec(parentID string, sizeGiB int64) *nebiusbillingv1alpha1.ResourceSpec {
	return &nebiusbillingv1alpha1.ResourceSpec{
		ResourceSpec: &nebiusbillingv1alpha1.ResourceSpec_ComputeDiskSpec{
			ComputeDiskSpec: &nebiuscomputev1.CreateDiskRequest{
				Metadata: &nebiuscommonv1.ResourceMetadata{
					ParentId: parentID,
				},
				Spec: &nebiuscomputev1.DiskSpec{
					Type: nebiuscomputev1.DiskSpec_NETWORK_SSD,
					Size: &nebiuscomputev1.DiskSpec_SizeGibibytes{
						SizeGibibytes: sizeGiB,
					},
				},
			},
		},
	}
}
