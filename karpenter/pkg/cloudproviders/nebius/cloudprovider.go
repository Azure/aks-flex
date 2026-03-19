package nebius

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	karpoptions "github.com/Azure/karpenter-provider-azure/pkg/operator/options"
	labelspkg "github.com/Azure/karpenter-provider-azure/pkg/providers/labels"
	"github.com/Azure/karpenter-provider-azure/pkg/utils"
	"github.com/awslabs/operatorpkg/status"
	"github.com/nebius/gosdk"
	"github.com/samber/lo"
	"google.golang.org/grpc"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	v1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
	corecloudprovider "sigs.k8s.io/karpenter/pkg/cloudprovider"
	"sigs.k8s.io/karpenter/pkg/scheduling"

	stretchhelper "github.com/Azure/aks-flex/plugin/pkg/helper"
	stretchservices "github.com/Azure/aks-flex/plugin/pkg/services"
	agentpoolsapi "github.com/Azure/aks-flex/plugin/pkg/services/agentpools/api"
	"github.com/Azure/aks-flex/plugin/pkg/services/agentpools/api/features/wireguard"
	nebiusinstance "github.com/Azure/aks-flex/plugin/pkg/services/agentpools/nebius/instance"

	"github.com/Azure/aks-flex/karpenter/pkg/apis"
	"github.com/Azure/aks-flex/karpenter/pkg/apis/v1alpha1"
	"github.com/Azure/aks-flex/karpenter/pkg/cloudproviders"
	"github.com/Azure/aks-flex/karpenter/pkg/cloudproviders/nebius/instancetype"
	wgallocator "github.com/Azure/aks-flex/karpenter/pkg/utils/wireguard"
)

type CloudProvider struct {
	sdk                     *gosdk.SDK
	stretchPluginConn       *grpc.ClientConn
	stretchAgentPoolsClient agentpoolsapi.AgentPoolsClient

	kubeClient client.Client

	clusterVersionNoVPrefix string
	clusterCA               []byte

	wgAllocator          *wgallocator.IPAllocator
	instanceTypeProvider *instancetype.Provider
}

func newCloudProvider(
	sdk *gosdk.SDK,
	stretchPluginConn *grpc.ClientConn,
	kubeClient client.Client,
	clusterVersion string,
	clusterCA []byte,
	wgAlloc *wgallocator.IPAllocator,
) *CloudProvider {
	return &CloudProvider{
		sdk:                     sdk,
		stretchPluginConn:       stretchPluginConn,
		stretchAgentPoolsClient: agentpoolsapi.NewAgentPoolsClient(stretchPluginConn),

		kubeClient:              kubeClient,
		clusterVersionNoVPrefix: strings.TrimPrefix(clusterVersion, "v"),
		clusterCA:               clusterCA,
		wgAllocator:             wgAlloc,
		instanceTypeProvider:    instancetype.NewProvider(sdk),
	}
}

func Register(
	ctx context.Context,
	hub *cloudproviders.CloudProvidersHub,
	sdk *gosdk.SDK,
	kubeClient client.Client,
	clusterVersion string,
	clusterCA []byte,
	wgAlloc *wgallocator.IPAllocator,
) error {
	stretchPluginConn, err := stretchservices.NewConnection()
	if err != nil {
		return fmt.Errorf("creating stretch plugin connection: %w", err)
	}

	cp := newCloudProvider(sdk, stretchPluginConn, kubeClient, clusterVersion, clusterCA, wgAlloc)
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

	key := instancetype.NodeClassKey{
		ProjectID:        nodeClass.Spec.ProjectID,
		Region:           nodeClass.Spec.Region,
		OSDiskSizeGiB:    int64(lo.FromPtrOr(nodeClass.Spec.OSDiskSizeGB, 100)),
		PerNodePodsCount: lo.FromPtrOr(nodeClass.Spec.MaxPodsPerNode, instancetype.DefaultPerNodePodsCount),
	}

	// resolve instance type to use based on pricing/offerings
	launchSettings, err := c.instanceTypeProvider.ResolvePlatformPresetFromNodeClaim(ctx, key, nodeClaim)
	if err != nil {
		return nil, err
	}
	logger.Info(
		"resolved platform preset for launching instance",
		"platformPreset", launchSettings.InstanceTypeName(),
		"capacityType", launchSettings.CapacityType,
		"zone", launchSettings.Zone,
	)

	// Allocate a WireGuard peer IP if enabled for this NodeClass.
	var wgConfig *wireguard.Config
	if nodeClass.Spec.WireguardPeerCIDR != nil {
		peerIP, err := c.wgAllocator.AllocateIP(ctx, *nodeClass.Spec.WireguardPeerCIDR, nodeClass.Name, nodeClaim.Name)
		if err != nil {
			return nil, err
		}
		logger.Info("allocated WireGuard peer IP", "peerIP", peerIP)
		wgConfig = wireguard.Config_builder{
			PeerIp: lo.ToPtr(peerIP),
		}.Build()
	}

	agentPool := nodeClaimToStretchAgentPool(
		karpoptions.FromContext(ctx),
		c.clusterVersionNoVPrefix,
		c.clusterCA,
		nodeClass,
		nodeClaim,
		launchSettings,
		wgConfig,
	)
	// TODO: create async - we just need to retrieve the resource id for rebuilding the claim
	agentPoolCreated, err := stretchhelper.CreateOrUpdate(
		c.stretchAgentPoolsClient.CreateOrUpdate,
		ctx, agentPool,
	)
	if err != nil {
		if IsQuotaError(err) {
			// NOTE: nebius doesn't block creation for quota issues, instead they
			// create the resource but mark it as failed. This could lead to contention
			// of other resources (disk, nic, etc). So here we delete the created resource
			// in best effort when seeing quota error.
			// FIXME: use a better clean up helper to perform the clean up in background

			// Record the failed instance type in the unavailable offerings cache
			// so that ResolvePlatformPresetFromNodeClaim will skip this offering
			// on subsequent attempts. The entry expires after a TTL, at which point
			// the offering becomes eligible for selection again.
			// NOTE: Nebius does not expose availability zones at this moment,
			// so we use the region as the location dimension for the cache key.
			c.instanceTypeProvider.UnavailableOfferings.MarkUnavailable(
				ctx,
				"QuotaFailure",
				launchSettings.InstanceTypeName(),
				nodeClass.Spec.Region,
				launchSettings.CapacityType,
			)

			go func() {
				cleanUpCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
				defer cancel()
				if err := c.Delete(cleanUpCtx, nodeClaim); err != nil {
					logger.Error(err, "deleting nodeClaim after quota error")
				}
			}()

			// stop karpenter from creating more node claims
			return nil, cloudprovider.NewInsufficientCapacityError(err)
		}
		return nil, fmt.Errorf("creating stretch agent pool: %w", err)
	}

	logger.Info("launched nebius agent pool")

	// rebuild node claim object to reflect the created instance
	launchedInstanceType := c.instanceTypeProvider.GetInstanceTypeByPlatformPreset(ctx, key, launchSettings.PlatformPreset)
	newNodeClaim := stretchAgentPoolToNodeClaim(agentPoolCreated, launchedInstanceType)
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
		if IsNotFound(err) {
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
		if IsNotFound(err) {
			// return NodeClaimNotFoundError to signal deletion later
			return nil, cloudprovider.NewNodeClaimNotFoundError(err)
		}
		return nil, err
	}

	key := instancetype.NodeClassKey{
		ProjectID:        agentPool.GetSpec().GetProjectId(),
		Region:           agentPool.GetSpec().GetRegion(),
		OSDiskSizeGiB:    int64(agentPool.GetSpec().GetOsDiskSizeGibibytes()),
		PerNodePodsCount: instancetype.DefaultPerNodePodsCount, // agent pool doesn't store max pods; use default
	}

	platformName := agentPool.GetSpec().GetPlatform()
	presetName := agentPool.GetSpec().GetPreset()

	preset, err := c.instanceTypeProvider.GetPlatformPreset(ctx, key, platformName, presetName)
	if err != nil {
		return nil, fmt.Errorf("resolving platform preset %q/%q: %w", platformName, presetName, err)
	}

	it := c.instanceTypeProvider.GetInstanceTypeByPlatformPreset(ctx, key, preset)
	nodeClaim := stretchAgentPoolToNodeClaim(agentPool, it)
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

	nodeClaims := make([]*v1.NodeClaim, 0, len(agentPools))
	for _, agentPool := range agentPools {
		key := instancetype.NodeClassKey{
			ProjectID:        agentPool.GetSpec().GetProjectId(),
			Region:           agentPool.GetSpec().GetRegion(),
			OSDiskSizeGiB:    int64(agentPool.GetSpec().GetOsDiskSizeGibibytes()),
			PerNodePodsCount: instancetype.DefaultPerNodePodsCount, // agent pool doesn't store max pods; use default
		}

		platformName := agentPool.GetSpec().GetPlatform()
		presetName := agentPool.GetSpec().GetPreset()

		preset, err := c.instanceTypeProvider.GetPlatformPreset(ctx, key, platformName, presetName)
		if err != nil {
			return nil, fmt.Errorf("resolving platform preset %q/%q: %w", platformName, presetName, err)
		}

		it := c.instanceTypeProvider.GetInstanceTypeByPlatformPreset(ctx, key, preset)
		nodeClaims = append(nodeClaims, stretchAgentPoolToNodeClaim(agentPool, it))
	}

	return nodeClaims, nil
}

func (c *CloudProvider) GetInstanceTypes(ctx context.Context, nodePool *v1.NodePool) ([]*corecloudprovider.InstanceType, error) {
	logger := loggerFromContext(ctx).WithValues("nodePool", nodePool.Name)

	nodeClass, err := c.getNodeClass(ctx, nodePool.Spec.Template.Spec.NodeClassRef)
	if err != nil {
		return nil, fmt.Errorf("getting node class for node pool: %w", err)
	}

	key := instancetype.NodeClassKey{
		ProjectID:        nodeClass.Spec.ProjectID,
		Region:           nodeClass.Spec.Region,
		OSDiskSizeGiB:    int64(*nodeClass.Spec.OSDiskSizeGB),
		PerNodePodsCount: lo.FromPtrOr(nodeClass.Spec.MaxPodsPerNode, instancetype.DefaultPerNodePodsCount),
	}

	instanceTypes, err := c.instanceTypeProvider.GetInstanceTypes(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("getting instance types for project %q region %q: %w",
			key.ProjectID, key.Region, err)
	}

	logger.V(5).Info("listed instance types", "count", len(instanceTypes))

	return instanceTypes, nil
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

	if c.instanceTypeProvider != nil {
		c.instanceTypeProvider.Stop()
	}

	return closeErr
}
