package nodes

import (
	"context"
	"time"

	opcontroller "github.com/awslabs/operatorpkg/controller"
	"github.com/awslabs/operatorpkg/reasonable"
	corev1 "k8s.io/api/core/v1"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"

	"github.com/Azure/aks-flex/karpenter-provider-flex/pkg/cloudproviders"
)

// SetProviderIDController backfills the provider id for a node object to match with
// the corresponding node claim.
// FIXME: technically the provider id should be set by the cloud node manager
// or in the kubelet config. However, we will rely on this node controller
// for now to set the correct provider ID after bootstrapping.
// We will rely on the cloudproviders.NodeClaimLabelKey to pinpoint to the node
// claim that was used to launch the instance, and set the provider ID to the node
// accordingly.
type SetProviderIDController struct {
	kubeClient client.Client
}

var (
	_ opcontroller.Controller                  = (*SetProviderIDController)(nil)
	_ reconcile.ObjectReconciler[*corev1.Node] = (*SetProviderIDController)(nil)
)

func NewSetProviderIDController(kubeClient client.Client) *SetProviderIDController {
	return &SetProviderIDController{
		kubeClient: kubeClient,
	}
}

func (n *SetProviderIDController) Reconcile(
	ctx context.Context,
	node *corev1.Node,
) (reconcile.Result, error) {
	if node.Spec.ProviderID != "" {
		return reconcile.Result{}, nil
	}

	nodeClaimName, exists := node.GetLabels()[cloudproviders.NodeClaimLabelKey]
	if !exists {
		// not managed by karpenter at all
		return reconcile.Result{}, nil
	}

	nodeClaim := &karpv1.NodeClaim{}
	if err := n.kubeClient.Get(ctx, client.ObjectKey{Name: nodeClaimName}, nodeClaim); err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}

	if nodeClaim.Status.ProviderID == "" {
		// FIXME: ad-hoc requeue, better to have an event-based approach
		return reconcile.Result{RequeueAfter: 30 * time.Second}, nil
	}

	future := node.DeepCopy()
	future.Spec.ProviderID = nodeClaim.Status.ProviderID
	log.FromContext(ctx).Info(
		"setting provider on node",
		"providerID", future.Spec.ProviderID,
		"nodeClaim", nodeClaim.Name,
		"node", future.Name,
	)
	if err := n.kubeClient.Patch(ctx, future, client.MergeFrom(node)); err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (n *SetProviderIDController) Register(ctx context.Context, m manager.Manager) error {
	return controllerruntime.NewControllerManagedBy(m).
		Named("aks-flex.nodes.set_provider_id").
		For(&corev1.Node{}).
		WithOptions(controller.Options{
			RateLimiter: reasonable.RateLimiter(),
			// TODO: Document why this magic number used. If we want to consistently use it accoss reconcilers, refactor to a reused const.
			// Comments thread discussing this: https://github.com/Azure/karpenter-provider-azure/pull/729#discussion_r2006629809
			MaxConcurrentReconciles: 10,
		}).
		Complete(reconcile.AsReconciler(m.GetClient(), n))
}
