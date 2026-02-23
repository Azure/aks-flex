package kaito

import (
	"context"
	"fmt"

	"github.com/awslabs/operatorpkg/status"
	v1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	corecloudprovider "sigs.k8s.io/karpenter/pkg/cloudprovider"

	kaitov1alpha1 "github.com/Azure/aks-flex/karpenter-provider-flex/pkg/apis/kaito/v1alpha1"
	"github.com/Azure/aks-flex/karpenter-provider-flex/pkg/cloudproviders"
)

func Register(
	ctx context.Context,
	hub *cloudproviders.CloudProvidersHub,
) error {
	cp := newCloudProvider()
	hub.Register(cp, GroupKind, ProviderIDScheme)

	return nil
}

type CloudProvider struct {
}

func newCloudProvider() *CloudProvider {
	return &CloudProvider{}
}

var _ corecloudprovider.CloudProvider = (*CloudProvider)(nil)

func (c *CloudProvider) Create(ctx context.Context, nodeClaim *v1.NodeClaim) (*v1.NodeClaim, error) {
	fmt.Println("Creating node claim", nodeClaim.Spec)
	panic("unimplemented")
}

func (c *CloudProvider) Delete(ctx context.Context, nodeClaim *v1.NodeClaim) error {
	panic("unimplemented")
}

func (c *CloudProvider) Get(ctx context.Context, providerID string) (*v1.NodeClaim, error) {
	panic("unimplemented")
}

func (c *CloudProvider) GetInstanceTypes(ctx context.Context, nodePool *v1.NodePool) ([]*corecloudprovider.InstanceType, error) {
	// NOTE: in kaito no node pool is required, hence we won't imlpement this method
	panic("unimplemented")
}

func (c *CloudProvider) List(ctx context.Context) ([]*v1.NodeClaim, error) {
	return []*v1.NodeClaim{}, nil
}

func (c *CloudProvider) GetSupportedNodeClasses() []status.Object {
	return []status.Object{
		&kaitov1alpha1.KaitoNodeClass{},
	}
}

func (c *CloudProvider) IsDrifted(ctx context.Context, nodeClaim *v1.NodeClaim) (corecloudprovider.DriftReason, error) {
	return "", nil
}

func (c *CloudProvider) Name() string {
	return ProviderIDScheme
}

func (c *CloudProvider) RepairPolicies() []corecloudprovider.RepairPolicy {
	return []corecloudprovider.RepairPolicy{}
}
