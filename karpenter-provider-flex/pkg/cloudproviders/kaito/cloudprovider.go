package kaito

import (
	"context"
	"errors"
	"fmt"

	karpoptions "github.com/Azure/karpenter-provider-azure/pkg/operator/options"
	labelspkg "github.com/Azure/karpenter-provider-azure/pkg/providers/labels"
	"github.com/awslabs/operatorpkg/status"
	"github.com/samber/lo"
	"google.golang.org/grpc"
	"sigs.k8s.io/controller-runtime/pkg/log"
	v1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
	corecloudprovider "sigs.k8s.io/karpenter/pkg/cloudprovider"
	"sigs.k8s.io/karpenter/pkg/scheduling"

	stretchhelper "github.com/Azure/aks-flex/flex-plugin/pkg/helper"
	stretchservices "github.com/Azure/aks-flex/flex-plugin/pkg/services"
	agentpoolsapi "github.com/Azure/aks-flex/flex-plugin/pkg/services/agentpools/api"
	nebiusinstance "github.com/Azure/aks-flex/flex-plugin/pkg/services/agentpools/nebius/instance"

	kaitov1alpha1 "github.com/Azure/aks-flex/karpenter-provider-flex/pkg/apis/kaito/v1alpha1"
	"github.com/Azure/aks-flex/karpenter-provider-flex/pkg/cloudproviders"
	nebiuscloudprovider "github.com/Azure/aks-flex/karpenter-provider-flex/pkg/cloudproviders/nebius"
	flexopts "github.com/Azure/aks-flex/karpenter-provider-flex/pkg/options"
	wgallocator "github.com/Azure/aks-flex/karpenter-provider-flex/pkg/utils/wireguard"
)

func Register(
	ctx context.Context,
	hub *cloudproviders.CloudProvidersHub,
	clusterCA []byte,
	wgAlloc *wgallocator.IPAllocator,
) error {
	stretchPluginConn, err := stretchservices.NewConnection()
	if err != nil {
		return fmt.Errorf("creating stretch plugin connection: %w", err)
	}

	cp := newCloudProvider(stretchPluginConn, clusterCA, wgAlloc)
	hub.Register(cp, GroupKind, ProviderIDScheme)

	return nil
}

type CloudProvider struct {
	stretchPluginConn       *grpc.ClientConn
	stretchAgentPoolsClient agentpoolsapi.AgentPoolsClient

	clusterCA   []byte
	wgAllocator *wgallocator.IPAllocator
}

func newCloudProvider(
	stretchPluginConn *grpc.ClientConn,
	clusterCA []byte,
	wgAlloc *wgallocator.IPAllocator,
) *CloudProvider {
	return &CloudProvider{
		stretchPluginConn:       stretchPluginConn,
		stretchAgentPoolsClient: agentpoolsapi.NewAgentPoolsClient(stretchPluginConn),

		clusterCA:   clusterCA,
		wgAllocator: wgAlloc,
	}
}

var _ corecloudprovider.CloudProvider = (*CloudProvider)(nil)

func (c *CloudProvider) Create(ctx context.Context, nodeClaim *v1.NodeClaim) (*v1.NodeClaim, error) {
	logger := log.FromContext(ctx).WithValues("nodeClaim", nodeClaim.Name)
	logger.Info("creating node for kaito node claim")

	nodeClaimReqs, err := resolveNodeClaimRequirements(nodeClaim)
	if err != nil {
		return nil, err
	}

	kaitoOpts := flexopts.MustGetKaitoOptionsFromContext(ctx)

	nebiusAgentPoolSettings, err := resolveNebiusAgentPoolSettings(
		ctx, kaitoOpts, c.wgAllocator,
		nodeClaim, nodeClaimReqs,
	)
	if err != nil {
		return nil, err
	}

	agentPool := nodeClaimToNebiusAgentPool(
		karpoptions.FromContext(ctx), kaitoOpts, c.clusterCA,
		nodeClaim, nodeClaimReqs, nebiusAgentPoolSettings,
	)
	// TODO: create async - we just need to retrieve the resource id for rebuilding the claim
	agentPoolCreated, err := stretchhelper.CreateOrUpdate(
		c.stretchAgentPoolsClient.CreateOrUpdate,
		ctx, agentPool,
	)
	if err != nil {
		if nebiuscloudprovider.IsQuotaError(err) {
			// NOTE: nebius doesn't block creation for quota issues, instead they
			// create the resource but mark it as failed. This could lead to contention
			// of other resources (disk, nic, etc). So here we delete the created resource
			// in best effort when seeing quota error.
			// FIXME: don't leak go routine here
			go func() {
				if err := c.Delete(ctx, nodeClaim); err != nil {
					logger.Error(err, "deleting nodeClaim after quota error")
				}
			}()

			// stop karpenter from creating more node claims
			return nil, cloudprovider.NewInsufficientCapacityError(err)
		}
		return nil, fmt.Errorf("creating stretch agent pool: %w", err)
	}

	logger.Info("launched nebius agent pool for kaito node claim")

	// rebuild node claim object to reflect the created instance
	newNodeClaim := strechAgentPoolToNodeClaim(agentPoolCreated)
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
		if nebiuscloudprovider.IsNotFound(err) {
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
		if nebiuscloudprovider.IsNotFound(err) {
			// return NodeClaimNotFoundError to signal deletion later
			return nil, cloudprovider.NewNodeClaimNotFoundError(err)
		}
		return nil, err
	}

	nodeClaim := strechAgentPoolToNodeClaim(agentPool)
	return nodeClaim, nil
}

func (c *CloudProvider) GetInstanceTypes(ctx context.Context, nodePool *v1.NodePool) ([]*corecloudprovider.InstanceType, error) {
	// NOTE: in kaito no node pool is required, hence we won't imlpement this method
	panic("unimplemented")
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
		nodeClaims[i] = strechAgentPoolToNodeClaim(agentPool)
	}

	return nodeClaims, nil
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

func (c *CloudProvider) Close(context.Context) error {
	var closeErr error

	if c.stretchPluginConn != nil {
		closeErr = errors.Join(closeErr, c.stretchPluginConn.Close())
	}

	return closeErr
}
