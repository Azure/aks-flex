package nebius

import (
	"context"
	"fmt"

	"github.com/awslabs/operatorpkg/status"
	"github.com/nebius/gosdk"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	v1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	corecloudprovider "sigs.k8s.io/karpenter/pkg/cloudprovider"

	"github.com/Azure/karpenter-provider-flex/pkg/apis/v1alpha1"
	"github.com/Azure/karpenter-provider-flex/pkg/cloudproviders"
	"github.com/Azure/karpenter-provider-flex/pkg/options"
)

type CloudProvider struct {
	sdk        *gosdk.SDK
	kubeClient client.Client
	restConfig *rest.Config
}

func newCloudProvider(
	sdk *gosdk.SDK,
	kubeClient client.Client,
	restConfig *rest.Config,
) *CloudProvider {
	return &CloudProvider{
		sdk:        sdk,
		kubeClient: kubeClient,
		restConfig: restConfig,
	}
}

func Register(
	hub *cloudproviders.CloudProvidersHub,
	sdk *gosdk.SDK,
	kubeClient client.Client,
	restConfig *rest.Config,
) {
	cp := newCloudProvider(sdk, kubeClient, restConfig)
	hub.Register(cp, GroupKind, ProviderIDScheme)
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

func (c *CloudProvider) List(ctx context.Context) ([]*v1.NodeClaim, error) {
	panic("unimplemented")
}

func (c *CloudProvider) GetInstanceTypes(ctx context.Context, nodePool *v1.NodePool) ([]*corecloudprovider.InstanceType, error) {
	projectID := options.MustGetNebiusProjectID(ctx)

	logger := loggerFromContext(ctx)

	// TODO: implement caching
	var rv []*corecloudprovider.InstanceType
	for platformPreset, err := range filterPlatformPresets(ctx, projectID, c.sdk) {
		if err != nil {
			return nil, fmt.Errorf("filter supported platforms from %q: %w", projectID, err)
		}
		platform := platformPreset.platform
		preset := platformPreset.preset
		logger.V(8).Info(
			"found nebius platform preset",
			"platform.id", platform.GetMetadata().GetId(),
			"platform.name", platform.GetMetadata().GetName(),
			"platform.human_readable_name", platform.GetSpec().GetHumanReadableName(),
			"preset.name", preset.GetName(),
			"preset.vcpu_count", preset.GetResources().GetVcpuCount(),
			"preset.memory_gb", preset.GetResources().GetMemoryGibibytes(),
			"preset.gpu_count", preset.GetResources().GetGpuCount(),
		)

		rv = append(rv, platformPreset.ToInstanceType())
	}

	return rv, nil
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

func (c *CloudProvider) Close(context.Context) error {
	if c.sdk != nil {
		if err := c.sdk.Close(); err != nil {
			return err
		}
	}
	return nil
}
