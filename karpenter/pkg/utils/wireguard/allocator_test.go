package wireguard

import (
	"context"
	"net"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"

	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ---------------------------------------------------------------------------
// AllocateRandomIP tests (pure function, no dependencies)
// ---------------------------------------------------------------------------

func TestAllocateRandomIP_Basic(t *testing.T) {
	ip, err := AllocateRandomIP("10.0.0.0/30", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// /30 has 4 addresses, usable are .1 and .2 (skip network .0 and broadcast .3)
	if ip != "10.0.0.1" && ip != "10.0.0.2" {
		t.Fatalf("expected 10.0.0.1 or 10.0.0.2, got %s", ip)
	}
}

func TestAllocateRandomIP_ExcludesAllocated(t *testing.T) {
	ip, err := AllocateRandomIP("10.0.0.0/30", []string{"10.0.0.1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ip != "10.0.0.2" {
		t.Fatalf("expected 10.0.0.2 (only free host), got %s", ip)
	}
}

func TestAllocateRandomIP_Exhausted(t *testing.T) {
	_, err := AllocateRandomIP("10.0.0.0/30", []string{"10.0.0.1", "10.0.0.2"})
	if err == nil {
		t.Fatal("expected error for exhausted CIDR, got nil")
	}
}

func TestAllocateRandomIP_Slash32(t *testing.T) {
	ip, err := AllocateRandomIP("192.168.1.5/32", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ip != "192.168.1.5" {
		t.Fatalf("expected 192.168.1.5 for /32, got %s", ip)
	}
}

func TestAllocateRandomIP_Slash32_Exhausted(t *testing.T) {
	_, err := AllocateRandomIP("192.168.1.5/32", []string{"192.168.1.5"})
	if err == nil {
		t.Fatal("expected error for exhausted /32, got nil")
	}
}

func TestAllocateRandomIP_Slash31(t *testing.T) {
	// /31 has 2 usable IPs per RFC 3021
	ip, err := AllocateRandomIP("10.0.0.4/31", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ip != "10.0.0.4" && ip != "10.0.0.5" {
		t.Fatalf("expected 10.0.0.4 or 10.0.0.5 for /31, got %s", ip)
	}
}

func TestAllocateRandomIP_Slash31_OneAllocated(t *testing.T) {
	ip, err := AllocateRandomIP("10.0.0.4/31", []string{"10.0.0.4"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ip != "10.0.0.5" {
		t.Fatalf("expected 10.0.0.5, got %s", ip)
	}
}

func TestAllocateRandomIP_Slash31_Exhausted(t *testing.T) {
	_, err := AllocateRandomIP("10.0.0.4/31", []string{"10.0.0.4", "10.0.0.5"})
	if err == nil {
		t.Fatal("expected error for exhausted /31, got nil")
	}
}

func TestAllocateRandomIP_LargerCIDR(t *testing.T) {
	// /24 has 254 usable hosts (.1-.254)
	ip, err := AllocateRandomIP("172.16.0.0/24", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		t.Fatalf("returned IP %q is not valid", ip)
	}
	_, cidrNet, _ := net.ParseCIDR("172.16.0.0/24")
	if !cidrNet.Contains(parsed) {
		t.Fatalf("IP %s not in CIDR 172.16.0.0/24", ip)
	}
	// Must not be network or broadcast
	if ip == "172.16.0.0" || ip == "172.16.0.255" {
		t.Fatalf("IP %s should not be network or broadcast address", ip)
	}
}

func TestAllocateRandomIP_InvalidCIDR(t *testing.T) {
	_, err := AllocateRandomIP("not-a-cidr", nil)
	if err == nil {
		t.Fatal("expected error for invalid CIDR, got nil")
	}
}

func TestAllocateRandomIP_InvalidAllocatedIP(t *testing.T) {
	_, err := AllocateRandomIP("10.0.0.0/24", []string{"not-an-ip"})
	if err == nil {
		t.Fatal("expected error for invalid allocated IP, got nil")
	}
}

func TestAllocateRandomIP_AllocatesAllHostsInSlash29(t *testing.T) {
	// /29 = 8 addresses, 6 usable (.1 through .6)
	allocated := make(map[string]struct{})
	var allocList []string

	for i := 0; i < 6; i++ {
		ip, err := AllocateRandomIP("10.0.0.0/29", allocList)
		if err != nil {
			t.Fatalf("allocation %d: unexpected error: %v", i, err)
		}
		if _, dup := allocated[ip]; dup {
			t.Fatalf("allocation %d: duplicate IP %s", i, ip)
		}
		allocated[ip] = struct{}{}
		allocList = append(allocList, ip)
	}

	if len(allocated) != 6 {
		t.Fatalf("expected 6 unique IPs, got %d", len(allocated))
	}

	// Next allocation should fail
	_, err := AllocateRandomIP("10.0.0.0/29", allocList)
	if err == nil {
		t.Fatal("expected error after exhausting /29, got nil")
	}
}

// ---------------------------------------------------------------------------
// Minimal mock cache for IPAllocator tests
// ---------------------------------------------------------------------------

// mockCache implements cache.Cache with minimal functionality for testing.
// Only List and Get are implemented; all other methods panic.
type mockCache struct {
	cache.Cache // embed to satisfy interface; unused methods will panic via nil dereference
	nodes       []corev1.Node
	nodeClaims  map[string]*karpv1.NodeClaim // name -> NodeClaim
}

func (m *mockCache) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	nodeList, ok := list.(*corev1.NodeList)
	if !ok {
		return nil
	}
	nodeList.Items = append(nodeList.Items[:0], m.nodes...)
	return nil
}

func (m *mockCache) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	nc, ok := obj.(*karpv1.NodeClaim)
	if !ok {
		return errors.NewNotFound(schema.GroupResource{}, key.Name)
	}
	stored, exists := m.nodeClaims[key.Name]
	if !exists {
		return errors.NewNotFound(schema.GroupResource{Group: "karpenter.sh", Resource: "nodeclaims"}, key.Name)
	}
	*nc = *stored
	return nil
}

func (m *mockCache) GetInformer(ctx context.Context, obj client.Object, opts ...cache.InformerGetOption) (cache.Informer, error) {
	panic("not implemented")
}

func (m *mockCache) GetInformerForKind(ctx context.Context, gvk schema.GroupVersionKind, opts ...cache.InformerGetOption) (cache.Informer, error) {
	panic("not implemented")
}

func (m *mockCache) RemoveInformer(ctx context.Context, obj client.Object) error {
	panic("not implemented")
}

func (m *mockCache) Start(ctx context.Context) error {
	panic("not implemented")
}

func (m *mockCache) WaitForCacheSync(ctx context.Context) bool {
	panic("not implemented")
}

func (m *mockCache) IndexField(ctx context.Context, obj client.Object, field string, extractValue client.IndexerFunc) error {
	panic("not implemented")
}

func makeNode(name string, internalIP string) corev1.Node {
	return corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Status: corev1.NodeStatus{
			Addresses: []corev1.NodeAddress{
				{Type: corev1.NodeInternalIP, Address: internalIP},
			},
		},
	}
}

// newTestAllocator creates an IPAllocator with a mockCache that does NOT start
// the background cleanup goroutine (to avoid flakiness in tests).
func newTestAllocator(nodes []corev1.Node, nodeClaims map[string]*karpv1.NodeClaim) *IPAllocator {
	mc := &mockCache{
		nodes:      nodes,
		nodeClaims: nodeClaims,
	}
	_, cancel := context.WithCancel(context.Background())
	// Cancel immediately — we don't want the cleanup goroutine running in unit tests.
	cancel()
	return &IPAllocator{
		cache:        mc,
		groupKind:    schema.GroupKind{Group: "test.example.com", Kind: "TestNodeClass"},
		preAllocated: make(map[string]map[string]string),
		cancel:       cancel,
	}
}

// ---------------------------------------------------------------------------
// IPAllocator.AllocateIP tests
// ---------------------------------------------------------------------------

func TestIPAllocator_AllocateIP_Basic(t *testing.T) {
	a := newTestAllocator(nil, nil)

	ip, err := a.AllocateIP(context.Background(), "10.0.0.0/30", "my-class", "claim-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ip != "10.0.0.1" && ip != "10.0.0.2" {
		t.Fatalf("expected 10.0.0.1 or 10.0.0.2, got %s", ip)
	}
}

func TestIPAllocator_AllocateIP_AvoidsDuplicates(t *testing.T) {
	a := newTestAllocator(nil, nil)
	cidr := "10.0.0.0/30" // 2 usable IPs

	ip1, err := a.AllocateIP(context.Background(), cidr, "my-class", "claim-1")
	if err != nil {
		t.Fatalf("first allocation: unexpected error: %v", err)
	}

	ip2, err := a.AllocateIP(context.Background(), cidr, "my-class", "claim-2")
	if err != nil {
		t.Fatalf("second allocation: unexpected error: %v", err)
	}

	if ip1 == ip2 {
		t.Fatalf("two allocations returned the same IP: %s", ip1)
	}
}

func TestIPAllocator_AllocateIP_ExhaustedReturnsSufficientError(t *testing.T) {
	a := newTestAllocator(nil, nil)
	cidr := "10.0.0.0/30" // 2 usable IPs

	_, err := a.AllocateIP(context.Background(), cidr, "my-class", "claim-1")
	if err != nil {
		t.Fatalf("allocation 1: unexpected error: %v", err)
	}
	_, err = a.AllocateIP(context.Background(), cidr, "my-class", "claim-2")
	if err != nil {
		t.Fatalf("allocation 2: unexpected error: %v", err)
	}

	// Third allocation should fail
	_, err = a.AllocateIP(context.Background(), cidr, "my-class", "claim-3")
	if err == nil {
		t.Fatal("expected error for exhausted CIDR, got nil")
	}
}

func TestIPAllocator_AllocateIP_SkipsConfirmedNodeIPs(t *testing.T) {
	// Node already has 10.0.0.1 as InternalIP
	nodes := []corev1.Node{makeNode("node-1", "10.0.0.1")}
	a := newTestAllocator(nodes, nil)

	ip, err := a.AllocateIP(context.Background(), "10.0.0.0/30", "my-class", "claim-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Only 10.0.0.2 should be available
	if ip != "10.0.0.2" {
		t.Fatalf("expected 10.0.0.2 (10.0.0.1 taken by node), got %s", ip)
	}
}

func TestIPAllocator_AllocateIP_ReconcilesPreAllocatedOnConfirmation(t *testing.T) {
	a := newTestAllocator(nil, nil)
	cidr := "10.0.0.0/30" // 2 usable IPs

	// Allocate both IPs
	ip1, _ := a.AllocateIP(context.Background(), cidr, "my-class", "claim-1")
	_, _ = a.AllocateIP(context.Background(), cidr, "my-class", "claim-2")

	// Now simulate that ip1's node has appeared in the cache
	mc := a.cache.(*mockCache)
	mc.nodes = []corev1.Node{makeNode("node-1", ip1)}

	// Next allocation should succeed because ip1 is confirmed (removed from pre-allocated)
	// but also seen as confirmed, so still only 1 free IP... wait, both are taken.
	// Let me think: ip1 is confirmed on a node, ip2 is pre-allocated.
	// After reconciliation: ip1 removed from preAlloc (confirmed). ip2 stays in preAlloc.
	// Confirmed set = {ip1}, preAlloc set = {ip2}. Both taken. Still exhausted.
	// This is correct. Let me test a different scenario.

	// Better test: /29 with 6 usable hosts. Pre-allocate 1, confirm it, allocate another.
	a2 := newTestAllocator(nil, nil)
	cidr2 := "10.0.0.0/29" // 6 usable IPs

	firstIP, _ := a2.AllocateIP(context.Background(), cidr2, "my-class", "claim-1")

	// Simulate the node showing up with the first IP
	mc2 := a2.cache.(*mockCache)
	mc2.nodes = []corev1.Node{makeNode("node-1", firstIP)}

	// Allocate again — should succeed and the pre-allocated entry for firstIP should be cleaned up
	secondIP, err := a2.AllocateIP(context.Background(), cidr2, "my-class", "claim-2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if secondIP == firstIP {
		t.Fatalf("second IP should differ from confirmed first IP, both are %s", firstIP)
	}

	// Verify internal state: preAllocated should only have the second IP
	preAlloc := a2.preAllocated["my-class"]
	if len(preAlloc) != 1 {
		t.Fatalf("expected 1 pre-allocated entry, got %d", len(preAlloc))
	}
	if _, ok := preAlloc[secondIP]; !ok {
		t.Fatalf("expected pre-allocated entry for %s, not found", secondIP)
	}
}

func TestIPAllocator_AllocateIP_MultipleNodeClasses(t *testing.T) {
	a := newTestAllocator(nil, nil)
	cidr := "10.0.0.0/30" // 2 usable IPs per class

	// Allocate from class A
	ipA, err := a.AllocateIP(context.Background(), cidr, "class-a", "claim-a1")
	if err != nil {
		t.Fatalf("class-a allocation: unexpected error: %v", err)
	}

	// Allocate from class B — same CIDR but different class, so pre-allocated sets are independent
	ipB, err := a.AllocateIP(context.Background(), cidr, "class-b", "claim-b1")
	if err != nil {
		t.Fatalf("class-b allocation: unexpected error: %v", err)
	}

	// Both should get valid IPs (they may even be the same IP since classes are independent)
	_ = ipA
	_ = ipB

	if len(a.preAllocated["class-a"]) != 1 {
		t.Fatalf("expected 1 pre-allocated for class-a, got %d", len(a.preAllocated["class-a"]))
	}
	if len(a.preAllocated["class-b"]) != 1 {
		t.Fatalf("expected 1 pre-allocated for class-b, got %d", len(a.preAllocated["class-b"]))
	}
}

// ---------------------------------------------------------------------------
// IPAllocator.cleanup tests
// ---------------------------------------------------------------------------

func TestIPAllocator_Cleanup_RemovesStalePreAllocated(t *testing.T) {
	// claim-1 exists, claim-2 does not
	nodeClaims := map[string]*karpv1.NodeClaim{
		"claim-1": {ObjectMeta: metav1.ObjectMeta{Name: "claim-1"}},
	}
	a := newTestAllocator(nil, nodeClaims)

	// Seed pre-allocated state
	a.preAllocated["my-class"] = map[string]string{
		"10.0.0.1": "claim-1", // exists
		"10.0.0.2": "claim-2", // deleted
	}

	a.cleanup(context.Background())

	preAlloc := a.preAllocated["my-class"]
	if _, ok := preAlloc["10.0.0.2"]; ok {
		t.Fatal("expected stale IP 10.0.0.2 to be removed after cleanup")
	}
	if _, ok := preAlloc["10.0.0.1"]; !ok {
		t.Fatal("expected IP 10.0.0.1 for existing claim to be retained")
	}
}

func TestIPAllocator_Cleanup_RemovesEmptyNodeClassEntry(t *testing.T) {
	// No node claims exist at all
	a := newTestAllocator(nil, nil)

	a.preAllocated["my-class"] = map[string]string{
		"10.0.0.1": "claim-gone",
	}

	a.cleanup(context.Background())

	if _, ok := a.preAllocated["my-class"]; ok {
		t.Fatal("expected empty nodeclass entry to be deleted from preAllocated map")
	}
}

// ---------------------------------------------------------------------------
// IPAllocator.Close test
// ---------------------------------------------------------------------------

func TestIPAllocator_Close(t *testing.T) {
	mc := &mockCache{
		nodeClaims: make(map[string]*karpv1.NodeClaim),
	}

	// Use a very short cleanup interval so the goroutine would fire quickly if not stopped.
	a := NewIPAllocator(mc, schema.GroupKind{Group: "test", Kind: "Test"}, 10*time.Millisecond)
	a.Close()

	// After Close, the allocator should still be usable for allocation (Close only stops cleanup).
	ip, err := a.AllocateIP(context.Background(), "10.0.0.0/30", "my-class", "claim-1")
	if err != nil {
		t.Fatalf("unexpected error after Close: %v", err)
	}
	if ip == "" {
		t.Fatal("expected a valid IP after Close")
	}
}

// ---------------------------------------------------------------------------
// Helpers: verify mockCache satisfies cache.Cache at compile time
// ---------------------------------------------------------------------------

var _ cache.Cache = (*mockCache)(nil)

// Verify the runtime module is only needed to satisfy the interface embed.
var _ runtime.Object = (*corev1.NodeList)(nil)
