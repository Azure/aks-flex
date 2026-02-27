package controller

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	infrav1 "github.com/Azure/aks-flex/cluster-api-provider-flex/api/v1beta2"
)

// AKSClusterReconciler reconciles a AKSCluster object
type AKSClusterReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=infrastructure.flex-capi.aks.azure.com,resources=aksclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.flex-capi.aks.azure.com,resources=aksclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=infrastructure.flex-capi.aks.azure.com,resources=aksclusters/finalizers,verbs=update

func (r *AKSClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	aksCluster := &infrav1.AKSCluster{}
	err := r.Get(ctx, req.NamespacedName, aksCluster)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("AKSCluster resource not found or already deleted")
			return ctrl.Result{}, nil
		}

		log.Error(err, "Unable to fetch AKSCluster resource")
		return ctrl.Result{}, err
	}

	// TODO: fetch and resolve cluster data/kubeconfig from resource id
	// publish kubeconfig as secret

	aksCluster.Status.Initialization.Provisioned = ptr.To(true)
	if err := r.Status().Update(ctx, aksCluster); err != nil {
		log.Error(err, "Unable to update AKSCluster status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AKSClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrav1.AKSCluster{}).
		Named("akscluster").
		Complete(r)
}
