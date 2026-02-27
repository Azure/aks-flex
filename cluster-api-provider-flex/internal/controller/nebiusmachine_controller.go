package controller

import (
	"context"
	"fmt"
	"time"

	nebiuscompute "github.com/nebius/gosdk/proto/nebius/compute/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/predicates"

	infrastructurev1beta2 "github.com/Azure/aks-flex/cluster-api-provider-flex/api/v1beta2"
	"github.com/Azure/aks-flex/cluster-api-provider-flex/internal/service"
)

const (
	// requeueAfterInstancePending is the requeue interval when the instance is still being provisioned.
	requeueAfterInstancePending = 30 * time.Second
)

// NebiusMachineReconciler reconciles a NebiusMachine object.
type NebiusMachineReconciler struct {
	client.Client
	Scheme           *runtime.Scheme
	InstanceService  *service.NebiusInstanceService
	WatchFilterValue string
}

// +kubebuilder:rbac:groups=infrastructure.flex-capi.aks.azure.com,resources=nebiusmachines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.flex-capi.aks.azure.com,resources=nebiusmachines/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=infrastructure.flex-capi.aks.azure.com,resources=nebiusmachines/finalizers,verbs=update
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machines;machines/status,verbs=get;list;watch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;clusters/status,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile handles the reconciliation loop for NebiusMachine resources.
func (r *NebiusMachineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, retErr error) {
	log := logf.FromContext(ctx)

	// Fetch the NebiusMachine.
	nebiusMachine := &infrastructurev1beta2.NebiusMachine{}
	if err := r.Get(ctx, req.NamespacedName, nebiusMachine); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Fetch the owner Machine.
	machine, err := util.GetOwnerMachine(ctx, r.Client, nebiusMachine.ObjectMeta)
	if err != nil {
		return ctrl.Result{}, err
	}
	if machine == nil {
		log.Info("Waiting for Machine controller to set OwnerRef on NebiusMachine")
		return ctrl.Result{}, nil
	}

	log = log.WithValues("Machine", machine.Name)

	// Fetch the Cluster.
	cluster, err := util.GetClusterFromMetadata(ctx, r.Client, machine.ObjectMeta)
	if err != nil {
		log.Info("NebiusMachine owner Machine is missing cluster label or cluster does not exist")
		return ctrl.Result{}, err
	}

	log = log.WithValues("Cluster", cluster.Name)
	ctx = logf.IntoContext(ctx, log)

	// Return early if the object or Cluster is paused.
	if annotations.IsPaused(cluster, nebiusMachine) {
		log.Info("Reconciliation is paused for this object")
		return ctrl.Result{}, nil
	}

	// Always update status at the end of reconciliation.
	defer func() {
		if err := r.Status().Update(ctx, nebiusMachine); err != nil {
			if retErr == nil {
				retErr = err
			} else {
				log.Error(err, "Failed to update NebiusMachine status")
			}
		}
	}()

	// Handle deletion reconciliation.
	if !nebiusMachine.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, nebiusMachine)
	}

	// Handle normal reconciliation.
	return r.reconcileNormal(ctx, nebiusMachine, machine)
}

// reconcileNormal handles creating and reconciling the Nebius instance.
func (r *NebiusMachineReconciler) reconcileNormal(
	ctx context.Context,
	nebiusMachine *infrastructurev1beta2.NebiusMachine,
	machine *clusterv1.Machine,
) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Add finalizer if not present.
	if controllerutil.AddFinalizer(nebiusMachine, infrastructurev1beta2.MachineFinalizer) {
		if err := r.Update(ctx, nebiusMachine); err != nil {
			return ctrl.Result{}, err
		}
	}

	// If the instance already exists, reconcile its state.
	if nebiusMachine.Status.InstanceID != "" {
		return r.reconcileInstanceStatus(ctx, nebiusMachine)
	}

	// Wait for the bootstrap data to be ready.
	if machine.Spec.Bootstrap.DataSecretName == nil {
		log.Info("Waiting for bootstrap data to be available")
		return ctrl.Result{}, nil
	}

	// Read the bootstrap data secret.
	bootstrapData, err := r.getBootstrapData(ctx, machine)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("getting bootstrap data: %w", err)
	}

	// Derive the instance name from the Machine name.
	instanceName := machine.Name

	// Derive the boot disk name.
	diskName := fmt.Sprintf("%s-boot", instanceName)

	// Resolve image family.
	imageFamily := nebiusMachine.Spec.ImageFamily
	if imageFamily == "" {
		imageFamily = "ubuntu24.04-driverless"
	}

	// Step 1: Create the boot disk.
	disk, err := r.InstanceService.CreateDisk(
		ctx, log,
		nebiusMachine.Spec.ProjectID,
		diskName,
		imageFamily,
		nebiusMachine.Spec.OSDiskSizeGibibytes,
	)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("creating boot disk: %w", err)
	}

	nebiusMachine.Status.OSDiskID = disk.GetMetadata().GetId()

	// Step 2: Create the instance.
	instance, err := r.InstanceService.CreateInstance(
		ctx, log,
		nebiusMachine.Spec.ProjectID,
		instanceName,
		nebiusMachine.Spec.Platform,
		nebiusMachine.Spec.Preset,
		nebiusMachine.Spec.SubnetID,
		disk.GetMetadata().GetId(),
		bootstrapData,
	)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("creating instance: %w", err)
	}

	instanceID := instance.GetMetadata().GetId()
	nebiusMachine.Status.InstanceID = instanceID

	// Set ProviderID on spec.
	nebiusMachine.Spec.ProviderID = fmt.Sprintf("%s://%s", infrastructurev1beta2.ProviderIDPrefix, instanceID)

	if err := r.Update(ctx, nebiusMachine); err != nil {
		return ctrl.Result{}, fmt.Errorf("updating NebiusMachine spec with ProviderID: %w", err)
	}

	// Reconcile the instance status.
	return r.reconcileInstanceStatus(ctx, nebiusMachine)
}

// reconcileInstanceStatus fetches the current instance state from Nebius and updates the NebiusMachine status accordingly.
func (r *NebiusMachineReconciler) reconcileInstanceStatus(
	ctx context.Context,
	nebiusMachine *infrastructurev1beta2.NebiusMachine,
) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	instance, err := r.InstanceService.GetInstance(ctx, nebiusMachine.Status.InstanceID)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("getting instance status: %w", err)
	}

	if instance == nil {
		// Instance was deleted externally.
		log.Info("Instance not found, it may have been deleted externally", "instanceID", nebiusMachine.Status.InstanceID)
		failureReason := "InstanceNotFound"
		failureMessage := fmt.Sprintf("Nebius instance %s was not found", nebiusMachine.Status.InstanceID)
		nebiusMachine.Status.FailureReason = &failureReason
		nebiusMachine.Status.FailureMessage = &failureMessage
		return ctrl.Result{}, nil
	}

	// Map Nebius instance state to our InstanceState.
	nebiusMachine.Status.InstanceState = mapInstanceState(instance.GetStatus().GetState())

	// Populate addresses from network interfaces.
	nebiusMachine.Status.Addresses = extractAddresses(instance)

	switch nebiusMachine.Status.InstanceState {
	case infrastructurev1beta2.InstanceStateRunning:
		log.Info("Instance is running", "instanceID", nebiusMachine.Status.InstanceID)
		nebiusMachine.Status.Initialization.Provisioned = ptr.To(true)
		nebiusMachine.Status.FailureReason = nil
		nebiusMachine.Status.FailureMessage = nil
		return ctrl.Result{}, nil

	case infrastructurev1beta2.InstanceStateFailed:
		log.Info("Instance is in error state", "instanceID", nebiusMachine.Status.InstanceID)
		failureReason := "InstanceError"
		failureMessage := fmt.Sprintf("Nebius instance %s is in ERROR state", nebiusMachine.Status.InstanceID)
		nebiusMachine.Status.FailureReason = &failureReason
		nebiusMachine.Status.FailureMessage = &failureMessage
		return ctrl.Result{}, nil

	default:
		// Instance is still being provisioned (CREATING, STARTING, UPDATING, etc.).
		log.Info("Instance is not yet running, requeueing", "instanceID", nebiusMachine.Status.InstanceID, "state", nebiusMachine.Status.InstanceState)
		return ctrl.Result{RequeueAfter: requeueAfterInstancePending}, nil
	}
}

// reconcileDelete handles the deletion of Nebius resources.
func (r *NebiusMachineReconciler) reconcileDelete(
	ctx context.Context,
	nebiusMachine *infrastructurev1beta2.NebiusMachine,
) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.Info("Reconciling NebiusMachine delete")

	// Delete the instance first, then the disk (disk cannot be deleted while attached).
	if err := r.InstanceService.DeleteInstance(ctx, log, nebiusMachine.Status.InstanceID); err != nil {
		return ctrl.Result{}, fmt.Errorf("deleting instance: %w", err)
	}

	if err := r.InstanceService.DeleteDisk(ctx, log, nebiusMachine.Status.OSDiskID); err != nil {
		return ctrl.Result{}, fmt.Errorf("deleting boot disk: %w", err)
	}

	// Remove the finalizer.
	controllerutil.RemoveFinalizer(nebiusMachine, infrastructurev1beta2.MachineFinalizer)
	if err := r.Update(ctx, nebiusMachine); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("Successfully deleted NebiusMachine resources")
	return ctrl.Result{}, nil
}

// getBootstrapData reads the bootstrap data secret referenced by the Machine.
func (r *NebiusMachineReconciler) getBootstrapData(ctx context.Context, machine *clusterv1.Machine) (string, error) {
	if machine.Spec.Bootstrap.DataSecretName == nil {
		return "", fmt.Errorf("machine %s/%s has no bootstrap data secret", machine.Namespace, machine.Name)
	}

	secret := &corev1.Secret{}
	key := client.ObjectKey{
		Namespace: machine.Namespace,
		Name:      *machine.Spec.Bootstrap.DataSecretName,
	}
	if err := r.Get(ctx, key, secret); err != nil {
		return "", fmt.Errorf("getting bootstrap data secret %s/%s: %w", key.Namespace, key.Name, err)
	}

	value, ok := secret.Data["value"]
	if !ok {
		return "", fmt.Errorf("bootstrap data secret %s/%s is missing 'value' key", key.Namespace, key.Name)
	}

	return string(value), nil
}

// mapInstanceState maps a Nebius SDK InstanceStatus_InstanceState to our InstanceState type.
func mapInstanceState(state nebiuscompute.InstanceStatus_InstanceState) infrastructurev1beta2.InstanceState {
	switch state {
	case nebiuscompute.InstanceStatus_CREATING,
		nebiuscompute.InstanceStatus_UPDATING,
		nebiuscompute.InstanceStatus_STARTING:
		return infrastructurev1beta2.InstanceStatePending
	case nebiuscompute.InstanceStatus_RUNNING:
		return infrastructurev1beta2.InstanceStateRunning
	case nebiuscompute.InstanceStatus_STOPPING:
		return infrastructurev1beta2.InstanceStateStopping
	case nebiuscompute.InstanceStatus_STOPPED:
		return infrastructurev1beta2.InstanceStateStopped
	case nebiuscompute.InstanceStatus_DELETING:
		return infrastructurev1beta2.InstanceStateDeleting
	case nebiuscompute.InstanceStatus_ERROR:
		return infrastructurev1beta2.InstanceStateFailed
	default:
		return infrastructurev1beta2.InstanceStateUnspecified
	}
}

// extractAddresses extracts IP addresses from a Nebius instance's network interfaces.
func extractAddresses(instance *nebiuscompute.Instance) []infrastructurev1beta2.MachineAddress {
	var addresses []infrastructurev1beta2.MachineAddress

	if instance.GetStatus() == nil {
		return addresses
	}

	for _, nic := range instance.GetStatus().GetNetworkInterfaces() {
		if nic.GetIpAddress() != nil && nic.GetIpAddress().GetAddress() != "" {
			addresses = append(addresses, infrastructurev1beta2.MachineAddress{
				Type:    infrastructurev1beta2.MachineInternalIP,
				Address: nic.GetIpAddress().GetAddress(),
			})
		}
		if nic.GetPublicIpAddress() != nil && nic.GetPublicIpAddress().GetAddress() != "" {
			addresses = append(addresses, infrastructurev1beta2.MachineAddress{
				Type:    infrastructurev1beta2.MachineExternalIP,
				Address: nic.GetPublicIpAddress().GetAddress(),
			})
		}
	}

	return addresses
}

// SetupWithManager sets up the controller with the Manager.
func (r *NebiusMachineReconciler) SetupWithManager(mgr ctrl.Manager) error {
	clusterToNebiusMachines, err := util.ClusterToTypedObjectsMapper(
		r.Client,
		&infrastructurev1beta2.NebiusMachineList{},
		mgr.GetScheme(),
	)
	if err != nil {
		return err
	}

	nebiusMachineGVK := schema.GroupVersionKind{
		Group:   infrastructurev1beta2.GroupVersion.Group,
		Version: infrastructurev1beta2.GroupVersion.Version,
		Kind:    "NebiusMachine",
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&infrastructurev1beta2.NebiusMachine{}).
		Named("nebiusmachine").
		WithEventFilter(predicates.ResourceNotPausedAndHasFilterLabel(mgr.GetScheme(), mgr.GetLogger(), r.WatchFilterValue)).
		Watches(
			&clusterv1.Machine{},
			handler.EnqueueRequestsFromMapFunc(util.MachineToInfrastructureMapFunc(nebiusMachineGVK)),
		).
		Watches(
			&clusterv1.Cluster{},
			handler.EnqueueRequestsFromMapFunc(clusterToNebiusMachines),
			builder.WithPredicates(predicates.ClusterPausedTransitionsOrInfrastructureProvisioned(mgr.GetScheme(), mgr.GetLogger())),
		).
		Complete(r)
}
