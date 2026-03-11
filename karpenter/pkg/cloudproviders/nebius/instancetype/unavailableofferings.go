package instancetype

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	// UnavailableOfferingsTTL is the duration for which an offering that
	// failed due to a quota/capacity error is considered unavailable.
	// After this period, the entry expires and the offering becomes
	// eligible for selection again.
	UnavailableOfferingsTTL = 30 * time.Minute

	// unavailableOfferingsCleanupInterval is how often expired entries are
	// purged from the cache.
	unavailableOfferingsCleanupInterval = 5 * time.Minute
)

// UnavailableOfferings tracks offerings (instance type + region + capacity type
// combinations) that recently failed due to quota or insufficient-capacity
// errors. Entries automatically expire after UnavailableOfferingsTTL.
//
// Region is used instead of zone because Nebius does not expose availability
// zones — the region is the finest-grained location dimension available.
//
// This cache is consulted by ResolvePlatformPresetFromNodeClaim to skip
// offerings that are known to be temporarily unavailable, preventing
// repeated launch-and-fail cycles for the same instance type.
type UnavailableOfferings struct {
	mu      sync.RWMutex
	entries map[string]time.Time // key -> expiry time

	// SeqNum is atomically incremented on every mutation or eviction,
	// so watchers can detect when the cache has changed.
	SeqNum uint64

	stopCh chan struct{}
}

// NewUnavailableOfferings creates a new UnavailableOfferings cache and starts
// a background goroutine to clean up expired entries.
func NewUnavailableOfferings() *UnavailableOfferings {
	u := &UnavailableOfferings{
		entries: make(map[string]time.Time),
		stopCh:  make(chan struct{}),
	}
	go u.cleanupLoop()
	return u
}

// MarkUnavailable records that the given instance type, region, and capacity
// type combination is currently unavailable due to a quota or capacity error.
// The entry will expire after UnavailableOfferingsTTL.
func (u *UnavailableOfferings) MarkUnavailable(ctx context.Context, reason, instanceType, region, capacityType string) {
	u.MarkUnavailableWithTTL(ctx, reason, instanceType, region, capacityType, UnavailableOfferingsTTL)
}

// MarkUnavailableWithTTL records an unavailable offering with a custom TTL.
func (u *UnavailableOfferings) MarkUnavailableWithTTL(ctx context.Context, reason, instanceType, region, capacityType string, ttl time.Duration) {
	key := unavailableOfferingKey(instanceType, region, capacityType)
	expiry := time.Now().Add(ttl)

	log.FromContext(ctx).V(1).Info("marking offering as unavailable",
		"reason", reason,
		"instanceType", instanceType,
		"region", region,
		"capacityType", capacityType,
		"ttl", ttl,
	)

	u.mu.Lock()
	u.entries[key] = expiry
	u.mu.Unlock()

	atomic.AddUint64(&u.SeqNum, 1)
}

// IsUnavailable returns true if the given instance type, region, and capacity
// type combination is currently marked as unavailable.
func (u *UnavailableOfferings) IsUnavailable(instanceType, region, capacityType string) bool {
	key := unavailableOfferingKey(instanceType, region, capacityType)

	u.mu.RLock()
	expiry, found := u.entries[key]
	u.mu.RUnlock()

	if !found {
		return false
	}

	// Treat expired entries as available; cleanup goroutine will remove them.
	return time.Now().Before(expiry)
}

// Stop terminates the background cleanup goroutine.
func (u *UnavailableOfferings) Stop() {
	close(u.stopCh)
}

// cleanupLoop periodically removes expired entries.
func (u *UnavailableOfferings) cleanupLoop() {
	ticker := time.NewTicker(unavailableOfferingsCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-u.stopCh:
			return
		case <-ticker.C:
			u.cleanup()
		}
	}
}

func (u *UnavailableOfferings) cleanup() {
	now := time.Now()
	evicted := false

	u.mu.Lock()
	for key, expiry := range u.entries {
		if now.After(expiry) {
			delete(u.entries, key)
			evicted = true
		}
	}
	u.mu.Unlock()

	if evicted {
		atomic.AddUint64(&u.SeqNum, 1)
	}
}

// unavailableOfferingKey constructs the cache key for a specific offering.
func unavailableOfferingKey(instanceType, region, capacityType string) string {
	return fmt.Sprintf("%s:%s:%s", capacityType, instanceType, region)
}
