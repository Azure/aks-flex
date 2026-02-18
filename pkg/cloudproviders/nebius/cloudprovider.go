package nebius

import (
	"context"

	"github.com/awslabs/operatorpkg/status"
	v1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	corecloudprovider "sigs.k8s.io/karpenter/pkg/cloudprovider"

	"github.com/Azure/karpenter-provider-flex/pkg/apis/v1alpha1"
	"github.com/Azure/karpenter-provider-flex/pkg/cloudproviders"
)

type CloudProvider struct {
}

func newCloudProvider() *CloudProvider {
	return &CloudProvider{}
}

func Register(hub *cloudproviders.CloudProvidersHub) {
	hub.Register(newCloudProvider(), GroupKind, ProviderIDScheme)
}

var _ corecloudprovider.CloudProvider = (*CloudProvider)(nil)

func (c *CloudProvider) Create(context.Context, *v1.NodeClaim) (*v1.NodeClaim, error) {
	panic("unimplemented")
}

func (c *CloudProvider) Delete(context.Context, *v1.NodeClaim) error {
	panic("unimplemented")
}

func (c *CloudProvider) Get(context.Context, string) (*v1.NodeClaim, error) {
	panic("unimplemented")
}

func (c *CloudProvider) GetInstanceTypes(context.Context, *v1.NodePool) ([]*corecloudprovider.InstanceType, error) {
	panic("unimplemented")
}

func (c *CloudProvider) List(context.Context) ([]*v1.NodeClaim, error) {
	panic("unimplemented")
}

func (c *CloudProvider) GetSupportedNodeClasses() []status.Object {
	return []status.Object{
		&v1alpha1.NebiusNodeClass{},
	}
}

func (c *CloudProvider) IsDrifted(context.Context, *v1.NodeClaim) (corecloudprovider.DriftReason, error) {
	// TODO: implement drift detection logic
	return "", nil
}

func (c *CloudProvider) Name() string {
	return ProviderIDScheme
}

func (c *CloudProvider) RepairPolicies() []corecloudprovider.RepairPolicy {
	// TODO: define repair policies
	return []corecloudprovider.RepairPolicy{}
}
