package cloudproviders

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/awslabs/operatorpkg/status"
	"k8s.io/apimachinery/pkg/runtime/schema"
	v1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
)

var ErrNoCloudProviderRegistered = errors.New("no cloud provider registered")

type cloudProviderSet struct {
	providers          []cloudprovider.CloudProvider
	byGroupKind        map[schema.GroupKind]int
	byProviderIDScheme map[string]int
}

func newCloudProviderSet() *cloudProviderSet {
	return &cloudProviderSet{
		byGroupKind:        make(map[schema.GroupKind]int),
		byProviderIDScheme: make(map[string]int),
	}
}

func (set *cloudProviderSet) GetByGroupKind(gk schema.GroupKind) (cloudprovider.CloudProvider, bool) {
	idx, ok := set.byGroupKind[gk]
	if !ok {
		return nil, false
	}
	return set.providers[idx], true
}

func (set *cloudProviderSet) GetByProviderIDScheme(scheme string) (cloudprovider.CloudProvider, bool) {
	idx, ok := set.byProviderIDScheme[scheme]
	if !ok {
		return nil, false
	}
	return set.providers[idx], true
}

func (set *cloudProviderSet) All() []cloudprovider.CloudProvider {
	return set.providers
}

func (set *cloudProviderSet) Register(c cloudprovider.CloudProvider, gk schema.GroupKind, scheme string) {
	idx := len(set.providers)
	set.providers = append(set.providers, c)
	set.byGroupKind[gk] = idx
	set.byProviderIDScheme[scheme] = idx
}

type Closeable interface {
	Close(context.Context) error
}

type CloudProvidersHub struct {
	set *cloudProviderSet
}

func NewCloudProvidersHub() *CloudProvidersHub {
	return &CloudProvidersHub{
		set: newCloudProviderSet(),
	}
}

func (c *CloudProvidersHub) Register(cloudProvider cloudprovider.CloudProvider, gk schema.GroupKind, scheme string) {
	c.set.Register(cloudProvider, gk, scheme)
}

var _ cloudprovider.CloudProvider = (*CloudProvidersHub)(nil)

func (c *CloudProvidersHub) Create(ctx context.Context, nodeClaim *v1.NodeClaim) (*v1.NodeClaim, error) {
	gk := nodeClaim.Spec.NodeClassRef.GroupKind()
	cp, ok := c.set.GetByGroupKind(gk)
	if !ok {
		return nil, fmt.Errorf("%w for %s", ErrNoCloudProviderRegistered, gk)
	}
	return cp.Create(ctx, nodeClaim)
}

func (c *CloudProvidersHub) Delete(ctx context.Context, nodeClaim *v1.NodeClaim) error {
	gk := nodeClaim.Spec.NodeClassRef.GroupKind()
	cp, ok := c.set.GetByGroupKind(gk)
	if !ok {
		return fmt.Errorf("%w for %s", ErrNoCloudProviderRegistered, gk)
	}
	return cp.Delete(ctx, nodeClaim)
}

func (c *CloudProvidersHub) Get(ctx context.Context, providerID string) (*v1.NodeClaim, error) {
	u, err := url.Parse(providerID)
	if err != nil {
		return nil, fmt.Errorf("parsing provider ID %q: %w", providerID, err)
	}
	cp, ok := c.set.GetByProviderIDScheme(u.Scheme)
	if !ok {
		return nil, fmt.Errorf("%w for provider ID scheme %q", ErrNoCloudProviderRegistered, u.Scheme)
	}
	return cp.Get(ctx, providerID)
}

func (c *CloudProvidersHub) GetInstanceTypes(ctx context.Context, nodePool *v1.NodePool) ([]*cloudprovider.InstanceType, error) {
	gk := nodePool.Spec.Template.Spec.NodeClassRef.GroupKind()
	cp, ok := c.set.GetByGroupKind(gk)
	if !ok {
		return nil, fmt.Errorf("%w for %s", ErrNoCloudProviderRegistered, gk)
	}
	return cp.GetInstanceTypes(ctx, nodePool)
}

func (c *CloudProvidersHub) GetSupportedNodeClasses() []status.Object {
	var result []status.Object
	for _, cp := range c.set.providers {
		result = append(result, cp.GetSupportedNodeClasses()...)
	}
	return result
}

func (c *CloudProvidersHub) IsDrifted(ctx context.Context, nodeClaim *v1.NodeClaim) (cloudprovider.DriftReason, error) {
	gk := nodeClaim.Spec.NodeClassRef.GroupKind()
	cp, ok := c.set.GetByGroupKind(gk)
	if !ok {
		return "", fmt.Errorf("%w for %s", ErrNoCloudProviderRegistered, gk)
	}
	return cp.IsDrifted(ctx, nodeClaim)
}

func (c *CloudProvidersHub) List(ctx context.Context) ([]*v1.NodeClaim, error) {
	var result []*v1.NodeClaim
	for _, cp := range c.set.providers {
		nodeClaims, err := cp.List(ctx)
		if err != nil {
			return nil, err
		}
		result = append(result, nodeClaims...)
	}
	return result, nil
}

func (c *CloudProvidersHub) Name() string {
	return "aks-flex"
}

func (c *CloudProvidersHub) RepairPolicies() []cloudprovider.RepairPolicy {
	var result []cloudprovider.RepairPolicy
	for _, cp := range c.set.providers {
		result = append(result, cp.RepairPolicies()...)
	}
	return result
}

func (c *CloudProvidersHub) Close(ctx context.Context) error {
	var errs []error
	for _, cp := range c.set.providers {
		if closeable, ok := cp.(Closeable); ok {
			if err := closeable.Close(ctx); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}
