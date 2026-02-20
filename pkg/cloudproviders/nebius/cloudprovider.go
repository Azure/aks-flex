package nebius

import (
	"context"
	"errors"
	"fmt"

	karpoptions "github.com/Azure/karpenter-provider-azure/pkg/operator/options"
	labelspkg "github.com/Azure/karpenter-provider-azure/pkg/providers/labels"
	"github.com/awslabs/operatorpkg/status"
	"github.com/nebius/gosdk"
	"github.com/samber/lo"
	"google.golang.org/grpc"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	v1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
	corecloudprovider "sigs.k8s.io/karpenter/pkg/cloudprovider"
	"sigs.k8s.io/karpenter/pkg/scheduling"

	stretchhelper "github.com/azure-management-and-platforms/aks-unbounded/stretch/plugin/pkg/helper"
	stretchservices "github.com/azure-management-and-platforms/aks-unbounded/stretch/plugin/pkg/services"
	agentpoolsapi "github.com/azure-management-and-platforms/aks-unbounded/stretch/plugin/pkg/services/agentpools/api"
	nebiusinstance "github.com/azure-management-and-platforms/aks-unbounded/stretch/plugin/pkg/services/agentpools/nebius/instance"

	"github.com/Azure/karpenter-provider-azure/pkg/utils"
	"github.com/Azure/karpenter-provider-flex/pkg/apis"
	"github.com/Azure/karpenter-provider-flex/pkg/apis/v1alpha1"
	"github.com/Azure/karpenter-provider-flex/pkg/cloudproviders"
)

type CloudProvider struct {
	sdk                     *gosdk.SDK
	stretchPluginConn       *grpc.ClientConn
	stretchAgentPoolsClient agentpoolsapi.AgentPoolsClient

	kubeClient        client.Client
	clusterRestConfig *rest.Config
}

func newCloudProvider(
	sdk *gosdk.SDK,
	stretchPluginConn *grpc.ClientConn,
	kubeClient client.Client,
	restConfig *rest.Config,
) *CloudProvider {
	return &CloudProvider{
		sdk:                     sdk,
		stretchPluginConn:       stretchPluginConn,
		stretchAgentPoolsClient: agentpoolsapi.NewAgentPoolsClient(stretchPluginConn),

		kubeClient:        kubeClient,
		clusterRestConfig: restConfig,
	}
}

func Register(
	hub *cloudproviders.CloudProvidersHub,
	sdk *gosdk.SDK,
	kubeClient client.Client,
	restConfig *rest.Config,
) error {
	nebiusinstance.SetSDKDoNotUseInProd(sdk)

	stretchPluginConn, err := stretchservices.NewConnection()
	if err != nil {
		return fmt.Errorf("creating stretch plugin connection: %w", err)
	}

	cp := newCloudProvider(sdk, stretchPluginConn, kubeClient, restConfig)
	hub.Register(cp, GroupKind, ProviderIDScheme)

	return nil
}

var _ corecloudprovider.CloudProvider = (*CloudProvider)(nil)

func (c *CloudProvider) getNodeClass( // TODO: make it reusable
	ctx context.Context,
	nodeClassRef *v1.NodeClassReference,
) (*v1alpha1.NebiusNodeClass, error) {
	if nodeClassRef == nil {
		return nil, fmt.Errorf("nodeClaim must reference a node class")
	}
	if nodeClassRef.Group != apis.Group {
		return nil, fmt.Errorf("nodeClassRef %s references a node class in group %q, expected %q", nodeClassRef.Name, nodeClassRef.Group, apis.Group)
	}

	rv := &v1alpha1.NebiusNodeClass{}
	if err := c.kubeClient.Get(ctx, client.ObjectKey{Name: nodeClassRef.Name}, rv); err != nil {
		return nil, fmt.Errorf("getting NebiusNodeClass %s: %w", nodeClassRef.Name, err)
	}

	if !rv.DeletionTimestamp.IsZero() {
		return nil, utils.NewTerminatingResourceError(schema.GroupResource{Group: apis.Group, Resource: "nebiusnodeclass"}, rv.Name)
	}

	return rv, nil
}

func (c *CloudProvider) Create(ctx context.Context, nodeClaim *v1.NodeClaim) (*v1.NodeClaim, error) {
	logger := log.FromContext(ctx).WithValues("nodeClaim", nodeClaim.Name)
	logger.Info("creating nebius VM for nodeClaim")

	nodeClass, err := c.getNodeClass(ctx, nodeClaim.Spec.NodeClassRef)
	if err != nil {
		// FIXME: proper error attribution
		return nil, err
	}

	// resolve instance type to use based on pricing/offerings
	platformPresetToLaunch, err := resolvePlatformPresetFromNodeClaim(
		ctx,
		nodeClass.Spec.ProjectID,
		c.sdk,
		nodeClaim,
	)
	if err != nil {
		return nil, err
	}
	logger.Info(
		"resolved platform preset for launching instance",
		"platformPreset", platformPresetToLaunch.InstanceTypeName(),
	)

	agentPool := nodeClaimToStretchAgentPool(
		karpoptions.FromContext(ctx),
		c.clusterRestConfig.CAData,
		nodeClass,
		nodeClaim,
		platformPresetToLaunch,
	)
	// TODO: create async - we just need to retrieve the resource id for rebuilding the claim
	agentPoolCreated, err := stretchhelper.CreateOrUpdate(
		c.stretchAgentPoolsClient.CreateOrUpdate,
		ctx, agentPool,
	)
	if err != nil {
		if isQuotaError(err) {
			// stop karpenter from creating more node claims
			return nil, cloudprovider.NewInsufficientCapacityError(err)
		}
		return nil, fmt.Errorf("creating stretch agent pool: %w", err)
	}

	// rebuild node claim object to reflect the created instance
	newNodeClaim := stretchAgentPoolToNodeClaim(agentPoolCreated, platformPresetToLaunch.ToInstanceType())
	// TODO: figure out meaning
	newNodeClaim.Labels = lo.Assign(
		newNodeClaim.Labels,
		labelspkg.GetWellKnownSingleValuedRequirementLabels(scheduling.NewNodeSelectorRequirementsWithMinValues(nodeClaim.Spec.Requirements...)),
	)

	return newNodeClaim, nil
}

func (c *CloudProvider) Delete(ctx context.Context, nodeClaim *v1.NodeClaim) error {
	logger := log.FromContext(ctx).WithValues("nodeClaim", nodeClaim.Name)
	providerID := nodeClaim.Status.ProviderID
	if providerID == "" {
		logger.V(5).Info("nodeClaim has no providerID, skipping deletion")
		return nil
	}
	logger = logger.WithValues("providerID", providerID)

	agentPoolName, err := providerIDToAgentPoolName(providerID)
	if err != nil {
		logger.Error(err, "parsing providerID to get agent pool name")
		return nil // don't return error since we want to retry deletion until successful, and this will likely be a permanent error
	}

	// NOTE: this step is necessary to meet the requirement of the Delete behavior,
	// see sigs.k8s.io/karpenter/pkg/cloudprovider#CloudProvider.Delete for details.
	if _, err := stretchhelper.Get[*nebiusinstance.AgentPool](
		c.stretchAgentPoolsClient.Get,
		ctx, agentPoolName,
	); err != nil {
		if isNotFound(err) {
			// no longer exists - return NodeClaimNotFoundError to signal deletion later
			return cloudprovider.NewNodeClaimNotFoundError(err)
		}
		// get failed, but we proceed to delete in best effort
		logger.V(5).Error(err, "getting agent pool for nodeClaim, proceed to delete")
	}

	logger.Info("deleting agent pool for nodeClaim")

	err = stretchhelper.Delete(
		c.stretchAgentPoolsClient.Delete,
		ctx, agentPoolName,
	)
	if err != nil {
		logger.Error(err, "deleting agent pool")
		return fmt.Errorf("deleting agent pool: %w", err)
	}

	logger.Info("deleted agent pool for nodeClaim")

	return nil
}

func (c *CloudProvider) Get(ctx context.Context, providerID string) (*v1.NodeClaim, error) {
	agentPoolName, err := providerIDToAgentPoolName(providerID)
	if err != nil {
		return nil, err
	}

	agentPool, err := stretchhelper.Get[*nebiusinstance.AgentPool](
		c.stretchAgentPoolsClient.Get,
		ctx, agentPoolName,
	)
	if err != nil {
		if isNotFound(err) {
			// return NodeClaimNotFoundError to signal deletion later
			return nil, cloudprovider.NewNodeClaimNotFoundError(err)
		}
		return nil, err
	}

	projectID := agentPool.GetSpec().GetProjectId()
	platformPreset, err := resolvePlatformPresetFromInstance(
		ctx,
		projectID,
		c.sdk,
		agentPool,
	)
	if err != nil {
		return nil, err
	}

	nodeClaim := stretchAgentPoolToNodeClaim(agentPool, platformPreset.ToInstanceType())
	return nodeClaim, nil
}

func (c *CloudProvider) List(ctx context.Context) ([]*v1.NodeClaim, error) {
	agentPools, err := stretchhelper.List[*nebiusinstance.AgentPool](
		c.stretchAgentPoolsClient.List,
		ctx, "",
	)
	if err != nil {
		return nil, err
	}

	nodeClaims := make([]*v1.NodeClaim, len(agentPools))
	for i, agentPool := range agentPools {
		// FIXME: don't do this n+1 lookup
		// cache platform preset results
		projectID := agentPool.GetSpec().GetProjectId()
		platformPreset, err := resolvePlatformPresetFromInstance(
			ctx,
			projectID,
			c.sdk,
			agentPool,
		)
		if err != nil {
			return nil, err
		}

		nodeClaims[i] = stretchAgentPoolToNodeClaim(agentPool, platformPreset.ToInstanceType())
	}

	return nodeClaims, nil
}

func (c *CloudProvider) GetInstanceTypes(ctx context.Context, nodePool *v1.NodePool) ([]*corecloudprovider.InstanceType, error) {
	logger := loggerFromContext(ctx)

	nodeClass, err := c.getNodeClass(ctx, nodePool.Spec.Template.Spec.NodeClassRef)
	if err != nil {
		return nil, fmt.Errorf("getting node class for node pool: %w", err)
	}
	projectID := nodeClass.Spec.ProjectID

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
	var closeErr error

	if c.sdk != nil {
		closeErr = errors.Join(closeErr, c.sdk.Close())
	}

	if c.stretchPluginConn != nil {
		closeErr = errors.Join(closeErr, c.stretchPluginConn.Close())
	}

	return closeErr
}
