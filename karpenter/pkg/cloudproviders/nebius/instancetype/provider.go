package instancetype

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/nebius/gosdk"
	nebiuscomputev1 "github.com/nebius/gosdk/proto/nebius/compute/v1"
	nebiuscomputeservice "github.com/nebius/gosdk/services/nebius/compute/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/lru"
	"sigs.k8s.io/controller-runtime/pkg/log"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	karpcloudprovider "sigs.k8s.io/karpenter/pkg/cloudprovider"
	"sigs.k8s.io/karpenter/pkg/scheduling"
)

const (
	// cacheSize is the maximum number of entries in the LRU cache.
	cacheSize = 1000

	// defaultOSDiskSizeGB is the fallback OS disk size when the node class does not specify one.
	defaultOSDiskSizeGB int32 = 128

	// DefaultPerNodePodsCount is the default maximum number of pods per node.
	// Used when the NebiusNodeClass does not specify MaxPodsPerNode.
	DefaultPerNodePodsCount int32 = 110

	// defaultRefreshInterval is how often the background goroutine re-fetches
	// cached entries from the Nebius APIs.
	defaultRefreshInterval = 5 * time.Minute
)

// NodeClassKey holds the fields from a NebiusNodeClassSpec that affect instance type
// listing, pricing, and capacity. It is used as the LRU cache key and passed through
// the provider instead of the full spec, keeping this package decoupled from the API types.
//
// TODO: confirm with Nebius whether pricing varies by project ID and/or region.
// If it does, they should remain part of the cache key; if not, we can drop them
// to improve cache hit rates across node classes that share the same disk size.
type NodeClassKey struct {
	ProjectID        string
	Region           string
	OSDiskSizeGiB    int64
	PerNodePodsCount int32
}

// cachedEntry is stored in the LRU cache. It bundles the assembled Karpenter
// InstanceTypes together with the raw PlatformPresets they were built from,
// so that callers needing presets (e.g. GetPlatformPreset) can avoid a
// redundant API call when the cache is warm.
type cachedEntry struct {
	instanceTypes []*karpcloudprovider.InstanceType
	presets       []*PlatformPreset
}

// Provider lists Nebius platform/preset pairs, prices them, and assembles
// Karpenter InstanceTypes. Results are cached in an LRU and refreshed in the background.
type Provider struct {
	platformService nebiuscomputeservice.PlatformService
	pricingProvider *PricingProvider

	cache *lru.Cache

	// activeKeys tracks cache keys that have been fetched at least once,
	// so the background refresh loop knows what to re-fetch.
	// (k8s.io/utils/lru does not expose a Keys() method.)
	activeKeys   map[NodeClassKey]struct{}
	activeKeysMu sync.Mutex

	refreshInterval time.Duration
	stopCh          chan struct{}
}

func NewProvider(sdk *gosdk.SDK) *Provider {
	return newProvider(
		sdk.Services().Compute().V1().Platform(),
		NewPricingProvider(sdk.Services().Billing().V1Alpha1().Calculator()),
	)
}

// newProvider creates an Provider and starts its background refresh loop.
func newProvider(
	platformService nebiuscomputeservice.PlatformService,
	pricingProvider *PricingProvider,
) *Provider {
	op := &Provider{
		platformService: platformService,
		pricingProvider: pricingProvider,
		cache:           lru.New(cacheSize),
		activeKeys:      make(map[NodeClassKey]struct{}),
		refreshInterval: defaultRefreshInterval,
		stopCh:          make(chan struct{}),
	}
	go op.refreshLoop()
	return op
}

// Stop terminates the background refresh goroutine.
func (op *Provider) Stop() {
	close(op.stopCh)
}

func (op *Provider) getOrResolveCachedEntry(
	ctx context.Context,
	key NodeClassKey,
) (cachedEntry, error) {
	// Fast path: cache hit.
	if cached, ok := op.cache.Get(key); ok {
		return cached.(cachedEntry), nil
	}

	// Slow path: resolve, cache, and return.
	entry, err := op.resolve(ctx, key)
	if err != nil {
		return cachedEntry{}, err
	}
	op.cache.Add(key, entry)
	op.trackKey(key)
	return entry, nil
}

// GetInstanceTypes returns the Karpenter InstanceTypes for the given node class key.
// On cache hit, the cached slice is returned immediately.
// On cache miss, platforms are listed, priced, and assembled before being cached and returned.
func (op *Provider) GetInstanceTypes(
	ctx context.Context,
	key NodeClassKey,
) ([]*karpcloudprovider.InstanceType, error) {
	entry, err := op.getOrResolveCachedEntry(ctx, key)
	if err != nil {
		return nil, err
	}
	return entry.instanceTypes, nil
}

// GetInstanceTypeByPlatformPreset returns a single Karpenter InstanceType for a specific
// platform/preset pair. It first checks the LRU cache for a matching instance type;
// on cache miss it falls back to pricing the preset directly.
func (op *Provider) GetInstanceTypeByPlatformPreset(
	ctx context.Context,
	key NodeClassKey,
	preset *PlatformPreset,
) *karpcloudprovider.InstanceType {
	name := preset.InstanceTypeName()

	// Fast path: find the instance type in the cached list.
	if cached, ok := op.cache.Get(key); ok {
		for _, it := range cached.(cachedEntry).instanceTypes {
			if it.Name == name {
				return it
			}
		}
	}

	// Slow path: price the single preset and assemble.
	prices := op.pricingProvider.GetPrices(ctx, key.ProjectID, []*PlatformPreset{preset}, key.OSDiskSizeGiB)

	priceKey := PriceKey{
		PlatformName:  preset.Platform().GetMetadata().GetName(),
		PresetName:    preset.Preset().GetName(),
		OSDiskSizeGiB: key.OSDiskSizeGiB,
	}
	presetPrices, ok := prices[priceKey]
	if !ok {
		presetPrices = Prices{OnDemand: defaultPrice, Preemptible: defaultPrice}
	}

	offerings := CreateOfferings(ctx, preset, key.Region, presetPrices)
	return NewInstanceType(key, preset, offerings)
}

// resolve lists platforms from the SDK, prices each preset, and assembles InstanceTypes.
func (op *Provider) resolve(
	ctx context.Context,
	key NodeClassKey,
) (cachedEntry, error) {
	logger := log.FromContext(ctx)

	// 1. List all platform/preset pairs.
	presets, err := op.listPlatformPresets(ctx, key.ProjectID)
	if err != nil {
		return cachedEntry{}, fmt.Errorf("listing platform presets for project %q: %w", key.ProjectID, err)
	}
	if len(presets) == 0 {
		logger.V(5).Info("no platform presets found", "projectID", key.ProjectID)
		return cachedEntry{}, nil
	}

	// 2. Fetch prices for all presets (includes OS disk cost).
	prices := op.pricingProvider.GetPrices(ctx, key.ProjectID, presets, key.OSDiskSizeGiB)

	// 3. Assemble Karpenter InstanceTypes.
	instanceTypes := make([]*karpcloudprovider.InstanceType, 0, len(presets))
	for _, p := range presets {
		priceKey := PriceKey{
			PlatformName:  p.Platform().GetMetadata().GetName(),
			PresetName:    p.Preset().GetName(),
			OSDiskSizeGiB: key.OSDiskSizeGiB,
		}
		presetPrices, ok := prices[priceKey]
		if !ok {
			// Should not happen — GetPrices always returns an entry per preset.
			presetPrices = Prices{OnDemand: defaultPrice, Preemptible: defaultPrice}
		}

		offerings := CreateOfferings(ctx, p, key.Region, presetPrices)
		it := NewInstanceType(key, p, offerings)
		instanceTypes = append(instanceTypes, it)
	}

	logger.V(5).Info("resolved instance types",
		"projectID", key.ProjectID,
		"region", key.Region,
		"count", len(instanceTypes),
	)

	return cachedEntry{
		instanceTypes: instanceTypes,
		presets:       presets,
	}, nil
}

// listPlatformPresets calls the Nebius Platform.Filter API and collects all
// platform/preset pairs into a slice.
func (op *Provider) listPlatformPresets(
	ctx context.Context,
	projectID string,
) ([]*PlatformPreset, error) {
	req := &nebiuscomputev1.ListPlatformsRequest{
		ParentId: projectID,
	}

	var presets []*PlatformPreset
	for platform, err := range op.platformService.Filter(ctx, req) {
		if err != nil {
			return nil, err
		}
		for _, preset := range platform.GetSpec().GetPresets() {
			presets = append(presets, NewPlatformPreset(platform, preset))
		}
	}
	return presets, nil
}

// GetPlatformPreset resolves a PlatformPreset by platform and preset name
// (e.g. "cpu-d3", "4vcpu-16gb"). It first checks the LRU cache for a match;
// on cache miss it lists platforms from the API.
func (op *Provider) GetPlatformPreset(
	ctx context.Context,
	key NodeClassKey,
	platformName, presetName string,
) (*PlatformPreset, error) {
	instanceTypeName := platformName + "-" + presetName

	// Try the cached presets first; only call the API on miss.
	presets, err := op.getCachedOrListPresets(ctx, key)
	if err != nil {
		return nil, err
	}

	for _, p := range presets {
		if p.InstanceTypeName() == instanceTypeName {
			return p, nil
		}
	}

	return nil, fmt.Errorf("platform preset %q not found in project %q", instanceTypeName, key.ProjectID)
}

// ResolvePlatformPresetFromNodeClaim selects the cheapest PlatformPreset that
// matches the NodeClaim's requirements. It filters by instance type name, zone,
// and capacity type, then picks the preset with the lowest compatible offering
// price from real billing data.
//
// The returned PlatformPresetLaunchSettings includes the resolved capacity type
// and zone extracted from the winning offering's requirements.
//
// On cache miss for the given key, instance types are resolved (listed, priced,
// cached) before selection.
func (op *Provider) ResolvePlatformPresetFromNodeClaim(
	ctx context.Context,
	key NodeClassKey,
	nodeClaim *karpv1.NodeClaim,
) (*PlatformPresetLaunchSettings, error) {
	// Build scheduling requirements from the NodeClaim so we can check
	// compatibility against each offering (instance type name, zone, capacity type, etc.).
	requirements := scheduling.NewNodeSelectorRequirementsWithMinValues(nodeClaim.Spec.Requirements...)

	// Collect the set of instance type names requested by the NodeClaim.
	requestedNames := map[string]struct{}{}
	if instanceTypeReq := requirements.Get(corev1.LabelInstanceTypeStable); instanceTypeReq != nil {
		for _, name := range instanceTypeReq.Values() {
			requestedNames[name] = struct{}{}
		}
	}
	if len(requestedNames) == 0 {
		return nil, fmt.Errorf("nodeClaim has no instance type requirements")
	}

	entry, err := op.getOrResolveCachedEntry(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("resolving instance types: %w", err)
	}

	// Build a name→preset index for the candidates.
	presetByName := make(map[string]*PlatformPreset, len(entry.presets))
	for _, p := range entry.presets {
		presetByName[p.InstanceTypeName()] = p
	}

	// Find the cheapest matching instance type whose offering is compatible
	// with the NodeClaim's requirements (instance type, zone, capacity type).
	var (
		bestPreset   *PlatformPreset
		bestOffering *karpcloudprovider.Offering
		bestPrice    = math.MaxFloat64
	)
	for _, it := range entry.instanceTypes {
		if _, requested := requestedNames[it.Name]; !requested {
			continue
		}

		for _, of := range it.Offerings {
			if !of.Available {
				continue
			}
			if !requirements.IsCompatible(of.Requirements, scheduling.AllowUndefinedWellKnownLabels) {
				continue
			}
			if of.Price < bestPrice {
				bestPrice = of.Price
				bestPreset = presetByName[it.Name]
				bestOffering = of
			}
		}
	}

	if bestPreset == nil || bestOffering == nil {
		// TODO: review the returning error type -- should we return the karpenter capacity error?
		return nil, fmt.Errorf("no matching available platform preset found for nodeClaim requirements")
	}

	// Extract capacity type and zone from the winning offering's requirements.
	capacityType := karpv1.CapacityTypeOnDemand
	if capReq := bestOffering.Requirements.Get(karpv1.CapacityTypeLabelKey); capReq != nil {
		if values := capReq.Values(); len(values) > 0 {
			capacityType = values[0]
		}
	}
	zone := key.Region // fallback
	if zoneReq := bestOffering.Requirements.Get(corev1.LabelTopologyZone); zoneReq != nil {
		if values := zoneReq.Values(); len(values) > 0 {
			zone = values[0]
		}
	}

	return &PlatformPresetLaunchSettings{
		PlatformPreset: bestPreset,
		CapacityType:   capacityType,
		Zone:           zone,
	}, nil
}

// On cache hit it returns the presets stored alongside the assembled InstanceTypes.
// On cache miss it falls back to listing from the Nebius Platform API.
func (op *Provider) getCachedOrListPresets(
	ctx context.Context,
	key NodeClassKey,
) ([]*PlatformPreset, error) {
	if cached, ok := op.cache.Get(key); ok {
		return cached.(cachedEntry).presets, nil
	}
	return op.listPlatformPresets(ctx, key.ProjectID)
}

// trackKey records a cache key so the background refresh loop can re-fetch it.
func (op *Provider) trackKey(key NodeClassKey) {
	op.activeKeysMu.Lock()
	defer op.activeKeysMu.Unlock()
	op.activeKeys[key] = struct{}{}
}

// refreshLoop periodically re-fetches all active cache entries.
func (op *Provider) refreshLoop() {
	ticker := time.NewTicker(op.refreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-op.stopCh:
			return
		case <-ticker.C:
			op.refreshAll()
		}
	}
}

// refreshAll re-fetches every active cache key and updates the cache.
// Errors are logged but do not remove existing cache entries (stale data is
// preferred over no data).
func (op *Provider) refreshAll() {
	op.activeKeysMu.Lock()
	keys := make([]NodeClassKey, 0, len(op.activeKeys))
	for k := range op.activeKeys {
		keys = append(keys, k)
	}
	op.activeKeysMu.Unlock()

	if len(keys) == 0 {
		return
	}

	// Use a background context with a timeout — the refresh is not tied to any request
	// but should not hang indefinitely.
	ctx, cancel := context.WithTimeout(context.Background(), op.refreshInterval/2)
	defer cancel()
	logger := log.FromContext(ctx).WithName("offerings-refresh")

	for _, key := range keys {
		entry, err := op.resolve(ctx, key)
		if err != nil {
			logger.Error(err, "background refresh failed",
				"projectID", key.ProjectID,
				"region", key.Region,
			)
			continue // keep stale cache entry
		}

		op.cache.Add(key, entry)
	}
}
