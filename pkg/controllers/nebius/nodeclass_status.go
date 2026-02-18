package nebius

import (
	"context"
	"fmt"

	opcontroller "github.com/awslabs/operatorpkg/controller"
	"github.com/awslabs/operatorpkg/reasonable"
	"k8s.io/apimachinery/pkg/api/equality"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/karpenter/pkg/operator/injection"

	"github.com/Azure/karpenter-provider-flex/pkg/apis/v1alpha1"
)

// TODO: this can be shared across other cloud providers

const controllerNameStatus = "nebius_nodeclass.status"

type NodeClassStatusController struct {
	kubeClient client.Client
}

var (
	_ opcontroller.Controller                               = (*NodeClassStatusController)(nil)
	_ reconcile.ObjectReconciler[*v1alpha1.NebiusNodeClass] = (*NodeClassStatusController)(nil)
)

func NewNodeClassStatusController(
	kubeClient client.Client,
) *NodeClassStatusController {
	return &NodeClassStatusController{
		kubeClient: kubeClient,
	}
}

func (c *NodeClassStatusController) Register(ctx context.Context, mgr manager.Manager) error {
	return controllerruntime.NewControllerManagedBy(mgr).
		Named(controllerNameStatus).
		For(&v1alpha1.NebiusNodeClass{}).
		WithOptions(controller.Options{
			RateLimiter: reasonable.RateLimiter(),
			// TODO: Document why this magic number used. If we want to consistently use it accoss reconcilers, refactor to a reused const.
			// Comments thread discussing this: https://github.com/Azure/karpenter-provider-azure/pull/729#discussion_r2006629809
			MaxConcurrentReconciles: 10,
		}).
		Complete(reconcile.AsReconciler(mgr.GetClient(), c))
}

func (c *NodeClassStatusController) Reconcile(
	ctx context.Context,
	nodeClass *v1alpha1.NebiusNodeClass,
) (reconcile.Result, error) {
	ctx = injection.WithControllerName(ctx, controllerNameStatus)

	existing := nodeClass
	future := nodeClass.DeepCopy()

	if err := c.ensureFinalizer(ctx, future); err != nil {
		return reconcile.Result{}, err
	}

	// TODO: validation and other preparation logic

	// set ready
	future.StatusConditions().SetTrue(v1alpha1.ConditionTypeValidationSucceeded)

	if !equality.Semantic.DeepEqual(existing, future) {
		// We use client.MergeFromWithOptimisticLock because patching a list with a JSON merge patch
		// can cause races due to the fact that it fully replaces the list on a change
		// Here, we are updating the status condition list
		if err := c.kubeClient.Status().Patch(ctx, future, client.MergeFrom(existing)); err != nil {
			return reconcile.Result{}, err
		}
	}

	return reconcile.Result{}, nil
}

func (c *NodeClassStatusController) ensureFinalizer(
	ctx context.Context,
	nodeClass *v1alpha1.NebiusNodeClass,
) error {
	if controllerutil.ContainsFinalizer(nodeClass, v1alpha1.TerminationFinalizer) {
		return nil
	}

	controllerutil.AddFinalizer(nodeClass, v1alpha1.TerminationFinalizer)

	// a patch is needed here to update the annotations
	if err := c.kubeClient.Patch(ctx, nodeClass, client.MergeFrom(nodeClass)); err != nil {
		return fmt.Errorf("patch finalizer: %w", err)
	}

	return nil
}
