package nebius

import (
	"context"
	"fmt"
	"time"

	"github.com/Azure/karpenter-provider-azure/pkg/utils"
	opcontroller "github.com/awslabs/operatorpkg/controller"
	"github.com/awslabs/operatorpkg/reasonable"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/events"
	"sigs.k8s.io/karpenter/pkg/operator/injection"

	"github.com/Azure/aks-flex/karpenter-provider-flex/pkg/apis"
	"github.com/Azure/aks-flex/karpenter-provider-flex/pkg/apis/v1alpha1"
)

// TODO: this can be shared across other controllers

const controllerNameTermination = "nebius_nodeclass.termination"

type NodeClassTerminationController struct {
	kubeClient client.Client
	recorder   events.Recorder
}

var (
	_ opcontroller.Controller                               = (*NodeClassTerminationController)(nil)
	_ reconcile.ObjectReconciler[*v1alpha1.NebiusNodeClass] = (*NodeClassTerminationController)(nil)
)

func NewNodeClassTerminationController(
	kubeClient client.Client,
	recorder events.Recorder,
) *NodeClassTerminationController {
	return &NodeClassTerminationController{
		kubeClient: kubeClient,
		recorder:   recorder,
	}
}

func (c *NodeClassTerminationController) Register(ctx context.Context, mgr manager.Manager) error {
	return controllerruntime.NewControllerManagedBy(mgr).
		Named(controllerNameTermination).
		For(&v1alpha1.NebiusNodeClass{}).
		Watches(
			&karpv1.NodeClaim{},
			handler.EnqueueRequestsFromMapFunc(func(_ context.Context, o client.Object) []reconcile.Request {
				nc := o.(*karpv1.NodeClaim)
				if nc.Spec.NodeClassRef == nil {
					return nil
				}
				if nc.Spec.NodeClassRef.Group != apis.Group {
					return nil
				}
				if nc.Spec.NodeClassRef.Kind != "NebiusNodeClass" {
					// TODO: support other kinds of NodeClass in the future if needed
					return nil
				}
				return []reconcile.Request{{NamespacedName: types.NamespacedName{Name: nc.Spec.NodeClassRef.Name}}}
			}),
			// Watch for NodeClaim deletion events
			builder.WithPredicates(predicate.Funcs{
				CreateFunc: func(e event.CreateEvent) bool { return false },
				UpdateFunc: func(e event.UpdateEvent) bool { return false },
				DeleteFunc: func(e event.DeleteEvent) bool { return true },
			}),
		).
		WithOptions(controller.Options{
			RateLimiter: reasonable.RateLimiter(),
			// TODO: Document why this magic number used. If we want to consistently use it accoss reconcilers, refactor to a reused const.
			// Comments thread discussing this: https://github.com/Azure/karpenter-provider-azure/pull/729#discussion_r2006629809
			MaxConcurrentReconciles: 10,
		}).
		Complete(reconcile.AsReconciler(mgr.GetClient(), c))
}

func (c *NodeClassTerminationController) Reconcile(
	ctx context.Context,
	nodeClass *v1alpha1.NebiusNodeClass,
) (reconcile.Result, error) {
	ctx = injection.WithControllerName(ctx, controllerNameTermination)

	if nodeClass.GetDeletionTimestamp().IsZero() {
		// not deleting
		return reconcile.Result{}, nil
	}

	return c.finalize(ctx, nodeClass)
}

func (c *NodeClassTerminationController) finalize(
	ctx context.Context,
	nodeClass *v1alpha1.NebiusNodeClass,
) (reconcile.Result, error) {
	// NOTE: finalizer is added by status controller
	if !controllerutil.ContainsFinalizer(nodeClass, v1alpha1.TerminationFinalizer) {
		return reconcile.Result{}, nil
	}

	stored := nodeClass.DeepCopy()

	nodeClaimList := &karpv1.NodeClaimList{}
	if err := c.kubeClient.List(ctx, nodeClaimList, client.MatchingFields{"spec.nodeClassRef.name": nodeClass.Name}); err != nil {
		return reconcile.Result{}, fmt.Errorf("listing nodeclaims that are using nodeclass, %w", err)
	}
	if len(nodeClaimList.Items) > 0 {
		c.recorder.Publish(WaitingOnNodeClaimTerminationEvent(nodeClass, lo.Map(nodeClaimList.Items, func(nc karpv1.NodeClaim, _ int) string { return nc.Name })))
		return reconcile.Result{RequeueAfter: time.Minute * 10}, nil // periodically fire the event
	}

	// any other processing before removing NodeClass goes here

	controllerutil.RemoveFinalizer(nodeClass, v1alpha1.TerminationFinalizer)
	if !equality.Semantic.DeepEqual(stored, nodeClass) {
		// We use client.MergeFromWithOptimisticLock because patching a list with a JSON merge patch
		// can cause races due to the fact that it fully replaces the list on a change
		// Here, we are updating the finalizer list
		// https://github.com/kubernetes/kubernetes/issues/111643#issuecomment-2016489732
		if err := c.kubeClient.Patch(ctx, nodeClass, client.MergeFromWithOptions(stored, client.MergeFromWithOptimisticLock{})); err != nil {
			if errors.IsConflict(err) {
				return reconcile.Result{Requeue: true}, nil
			}
			return reconcile.Result{}, client.IgnoreNotFound(fmt.Errorf("removing termination finalizer, %w", err))
		}
	}

	return reconcile.Result{}, nil
}

type RuntimeObjectWithUID interface {
	runtime.Object
	GetUID() types.UID
}

func WaitingOnNodeClaimTerminationEvent(nodeClass RuntimeObjectWithUID, names []string) events.Event {
	return events.Event{
		InvolvedObject: nodeClass,
		Type:           corev1.EventTypeNormal,
		Reason:         "WaitingOnNodeClaimTermination",
		Message:        fmt.Sprintf("Waiting on NodeClaim termination for %s", utils.PrettySlice(names, 5)),
		DedupeValues:   []string{string(nodeClass.GetUID())},
	}
}
