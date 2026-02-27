package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	infrastructurev1beta2 "github.com/Azure/aks-flex/cluster-api-provider-flex/api/v1beta2"
)

// FlexNodeConfigTemplateReconciler reconciles a FlexNodeConfigTemplate object
type FlexNodeConfigTemplateReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=infrastructure.flex-capi.aks.azure.com,resources=flexnodeconfigtemplates,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.flex-capi.aks.azure.com,resources=flexnodeconfigtemplates/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=infrastructure.flex-capi.aks.azure.com,resources=flexnodeconfigtemplates/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the FlexNodeConfigTemplate object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.23.1/pkg/reconcile
func (r *FlexNodeConfigTemplateReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = logf.FromContext(ctx)

	// TODO(user): your logic here

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *FlexNodeConfigTemplateReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrastructurev1beta2.FlexNodeConfigTemplate{}).
		Named("flexnodeconfigtemplate").
		Complete(r)
}
