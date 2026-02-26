package wireguard

import (
	"context"
	"encoding/binary"
	"fmt"
	"math/rand/v2"
	"net"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
)

// IPAllocator manages WireGuard peer IP allocation for NodeClaims.
//
// Because node creation takes time, we need to track "pre-allocated" IPs that have
// been handed out but haven't yet appeared on actual Nodes. Without this, concurrent
// NodeClaim creations could receive the same IP.
type IPAllocator struct {
	cache     cache.Cache
	groupKind schema.GroupKind

	// mu serializes all IP allocations. This is a heavy lock (one allocation at a
	// time across all NodeClasses) but is necessary for correctness since we need
	// to read-then-write the pre-allocated state atomically.
	mu sync.Mutex

	// preAllocated tracks IPs that have been allocated but not yet confirmed on a
	// Node. Keyed by NodeClass name -> (IP -> NodeClaim name).
	preAllocated map[string]map[string]string

	// cancel stops the background cleanup goroutine.
	cancel context.CancelFunc
}

// NewIPAllocator creates a new IPAllocator and starts its background cleanup goroutine.
// The caller must call Close() to stop the cleanup goroutine when the allocator is no
// longer needed.
func NewIPAllocator(kubeCache cache.Cache, groupKind schema.GroupKind, cleanupInterval time.Duration) *IPAllocator {
	ctx, cancel := context.WithCancel(context.Background())
	a := &IPAllocator{
		cache:        kubeCache,
		groupKind:    groupKind,
		preAllocated: make(map[string]map[string]string),
		cancel:       cancel,
	}
	a.startCleanup(ctx, cleanupInterval)
	return a
}

// Close stops the background cleanup goroutine.
func (a *IPAllocator) Close() {
	a.cancel()
}

// AllocateIP allocates a WireGuard peer IP for a NodeClaim under the given NodeClass.
// The cidr parameter is the WireGuard peer CIDR to allocate from.
// Returns the allocated IP string, or an error (InsufficientCapacityError if CIDR is exhausted).
func (a *IPAllocator) AllocateIP(
	ctx context.Context,
	cidr string,
	nodeClassName string,
	nodeClaimName string,
) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// TODO: replace this per-call List with a k8s informer-based approach. We can
	// use cache.GetInformer() to register an event handler on Nodes and maintain
	// the confirmed IP set reactively, avoiding the List cost on every allocation.

	// Step 1: Read current Nodes for this NodeClass from the informer-backed cache.
	confirmedIPs := make(map[string]struct{})
	nodeClassLabelKey := karpv1.NodeClassLabelKey(a.groupKind)
	nodeList := &corev1.NodeList{}
	if err := a.cache.List(ctx, nodeList, client.MatchingLabels{nodeClassLabelKey: nodeClassName}); err != nil {
		return "", fmt.Errorf("listing nodes for nodeclass %q: %w", nodeClassName, err)
	}
	for _, node := range nodeList.Items {
		for _, addr := range node.Status.Addresses {
			if addr.Type == corev1.NodeInternalIP {
				confirmedIPs[addr.Address] = struct{}{}
			}
		}
	}

	// Step 2: Reconcile the pre-allocated list — remove entries whose IPs have
	// appeared on actual Nodes (they are now confirmed).
	preAlloc := a.preAllocated[nodeClassName]
	if preAlloc == nil {
		preAlloc = make(map[string]string)
		a.preAllocated[nodeClassName] = preAlloc
	}
	for ip := range preAlloc {
		if _, confirmed := confirmedIPs[ip]; confirmed {
			delete(preAlloc, ip)
		}
	}

	// Step 3: Combine confirmed IPs and remaining pre-allocated IPs as the full
	// set of taken addresses, then allocate the next available IP.
	var allAllocatedIPs []string
	for ip := range confirmedIPs {
		allAllocatedIPs = append(allAllocatedIPs, ip)
	}
	for ip := range preAlloc {
		allAllocatedIPs = append(allAllocatedIPs, ip)
	}

	peerIP, err := AllocateRandomIP(cidr, allAllocatedIPs)
	if err != nil {
		return "", cloudprovider.NewInsufficientCapacityError(fmt.Errorf("allocating wireguard peer IP: %w", err))
	}

	// Step 4: Record the new IP as pre-allocated, associated with the NodeClaim.
	preAlloc[peerIP] = nodeClaimName

	return peerIP, nil
}

// startCleanup launches a background goroutine that periodically removes
// pre-allocated IPs whose corresponding NodeClaims no longer exist. This handles
// cases where node creation fails and the NodeClaim is cleaned up, but the IP
// was never confirmed on a Node.
func (a *IPAllocator) startCleanup(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				a.cleanup(ctx)
			}
		}
	}()
}

func (a *IPAllocator) cleanup(ctx context.Context) {
	logger := log.FromContext(ctx).WithName("wireguard-ip-cleanup")

	a.mu.Lock()
	defer a.mu.Unlock()

	for nodeClassName, preAlloc := range a.preAllocated {
		for ip, nodeClaimName := range preAlloc {
			nc := &karpv1.NodeClaim{}
			err := a.cache.Get(ctx, client.ObjectKey{Name: nodeClaimName}, nc)
			if errors.IsNotFound(err) {
				logger.V(5).Info(
					"removing pre-allocated IP for deleted nodeclaim",
					"nodeClass", nodeClassName,
					"ip", ip,
					"nodeClaim", nodeClaimName,
				)
				delete(preAlloc, ip)
			} else if err != nil {
				logger.V(5).Error(err, "checking nodeclaim existence",
					"nodeClass", nodeClassName,
					"nodeClaim", nodeClaimName,
				)
			}
		}
		if len(preAlloc) == 0 {
			delete(a.preAllocated, nodeClassName)
		}
	}
}

// AllocateRandomIP allocates a random free IP from the given CIDR, excluding the allocatedIPs.
// The network address and broadcast address are excluded from allocation (except for /31 and /32).
func AllocateRandomIP(cidr string, allocatedIPs []string) (string, error) {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", fmt.Errorf("parsing CIDR %q: %w", cidr, err)
	}

	// Build a set of already-allocated IPs for O(1) lookup.
	allocated := make(map[string]struct{}, len(allocatedIPs))
	for _, ip := range allocatedIPs {
		parsed := net.ParseIP(ip)
		if parsed == nil {
			return "", fmt.Errorf("parsing allocated IP %q: invalid IP address", ip)
		}
		// Normalize to 4-byte representation for consistent map keys.
		if v4 := parsed.To4(); v4 != nil {
			parsed = v4
		}
		allocated[parsed.String()] = struct{}{}
	}

	// Convert network address to a uint32 for iteration.
	networkIP := ipNet.IP.To4()
	if networkIP == nil {
		return "", fmt.Errorf("only IPv4 CIDRs are supported, got %q", cidr)
	}
	networkAddr := binary.BigEndian.Uint32(networkIP)

	ones, bits := ipNet.Mask.Size()
	if bits != 32 {
		return "", fmt.Errorf("only IPv4 CIDRs are supported, got %q", cidr)
	}
	hostBits := uint(bits - ones)
	totalHosts := uint32(1) << hostBits

	// Determine the usable host offset range.
	// For /32, there is exactly one IP (the address itself) — allow it.
	// For /31, there are two usable IPs per RFC 3021 — allow both.
	// Otherwise skip network (first) and broadcast (last) addresses.
	start := uint32(1)
	end := totalHosts - 2
	if hostBits <= 1 {
		start = 0
		end = totalHosts - 1
	}

	// Collect all free offsets.
	rangeSize := end - start + 1
	free := make([]uint32, 0, rangeSize)
	for offset := start; offset <= end; offset++ {
		candidate := make(net.IP, 4)
		binary.BigEndian.PutUint32(candidate, networkAddr+offset)
		if _, taken := allocated[candidate.String()]; !taken {
			free = append(free, offset)
		}
	}

	if len(free) == 0 {
		return "", fmt.Errorf("no free IP addresses available in CIDR %q", cidr)
	}

	// Pick a random free IP.
	chosen := free[rand.IntN(len(free))]
	ip := make(net.IP, 4)
	binary.BigEndian.PutUint32(ip, networkAddr+chosen)
	return ip.String(), nil
}
