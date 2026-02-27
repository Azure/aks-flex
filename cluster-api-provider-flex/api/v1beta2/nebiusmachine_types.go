package v1beta2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// MachineFinalizer allows cleanup of resources associated with a NebiusMachine before removal from the API.
	MachineFinalizer = "nebiusmachine.infrastructure.flex-capi.aks.azure.com"

	// ProviderIDPrefix is the prefix for the NebiusMachine provider ID.
	ProviderIDPrefix = "nebius"
)

// InstanceState describes the state of a Nebius instance.
type InstanceState string

const (
	// InstanceStateUnspecified indicates the instance state is unknown.
	InstanceStateUnspecified InstanceState = ""
	// InstanceStatePending indicates the instance is being created.
	InstanceStatePending InstanceState = "Pending"
	// InstanceStateRunning indicates the instance is running and ready.
	InstanceStateRunning InstanceState = "Running"
	// InstanceStateStopping indicates the instance is being stopped.
	InstanceStateStopping InstanceState = "Stopping"
	// InstanceStateStopped indicates the instance has been stopped.
	InstanceStateStopped InstanceState = "Stopped"
	// InstanceStateDeleting indicates the instance is being deleted.
	InstanceStateDeleting InstanceState = "Deleting"
	// InstanceStateFailed indicates the instance has failed.
	InstanceStateFailed InstanceState = "Failed"
)

// NebiusMachineSpec defines the desired state of NebiusMachine.
type NebiusMachineSpec struct {
	// providerID is the unique identifier as specified by the cloud provider.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=512
	ProviderID string `json:"providerID,omitempty"`

	// projectID is the Nebius project ID where the instance should be created.
	// +required
	ProjectID string `json:"projectID"`

	// region is the Nebius region where the instance should be created.
	// +required
	Region string `json:"region"`

	// subnetID is the Nebius subnet ID to attach the instance to.
	// +required
	SubnetID string `json:"subnetID"`

	// platform is the Nebius platform to use for this machine (e.g. "cpu-d3", "gpu-h200-sxm").
	// +required
	Platform string `json:"platform"`

	// preset is the Nebius preset to use for this machine (e.g. "4vcpu-16gb", "1gpu-128vcpu-1600gb").
	// +required
	Preset string `json:"preset"`

	// imageFamily is the Nebius image family to use for this machine (e.g. "ubuntu24.04-driverless").
	// Defaults to infer from platform and preset if not specified.
	// +optional
	ImageFamily string `json:"imageFamily,omitempty"`

	// osDiskSizeGibibytes is the size of the OS disk in GiB.
	// +optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default:=100
	OSDiskSizeGibibytes int32 `json:"osDiskSizeGibibytes,omitempty"`
}

type NebiusMachineInitializationStatus struct {
	// provisioned is true when the infrastructure provider reports that the Machine's infrastructure is fully provisioned.
	// NOTE: this field is part of the Cluster API contract, and it is used to orchestrate initial Machine provisioning.
	// +optional
	Provisioned *bool `json:"provisioned,omitempty"`
}

// NebiusMachineStatus defines the observed state of NebiusMachine.
type NebiusMachineStatus struct {
	// initialization provides observations of the NebiusMachine initialization process.
	// NOTE: Fields in this struct are part of the Cluster API contract, and they are used to orchestrate initial Machine provisioning.
	// +optional
	Initialization NebiusMachineInitializationStatus `json:"initialization,omitzero"`

	// instanceID is the Nebius instance ID of the machine.
	// +optional
	InstanceID string `json:"instanceID,omitempty"`

	// osDiskID is the Nebius disk ID of the OS disk attached to the instance.
	// +optional
	OSDiskID string `json:"osDiskID,omitempty"`

	// instanceState is the state of the Nebius instance.
	// +optional
	InstanceState InstanceState `json:"instanceState,omitempty"`

	// addresses contains the machine's associated addresses.
	// +optional
	Addresses []MachineAddress `json:"addresses,omitempty"`

	// failureReason will be set in the event that there is a terminal problem
	// reconciling the Machine and will contain a succinct value suitable for machine interpretation.
	// +optional
	FailureReason *string `json:"failureReason,omitempty"`

	// failureMessage will be set in the event that there is a terminal problem
	// reconciling the Machine and will contain a more verbose string suitable for logging and human consumption.
	// +optional
	FailureMessage *string `json:"failureMessage,omitempty"`

	// conditions represent the current state of the NebiusMachine resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// MachineAddress contains information for the machine's address.
type MachineAddress struct {
	// type is the type of the machine address.
	// +required
	Type MachineAddressType `json:"type"`
	// address is the machine address.
	// +required
	Address string `json:"address"`
}

// MachineAddressType describes a valid MachineAddress type.
type MachineAddressType string

const (
	// MachineInternalIP is a machine internal IP address.
	MachineInternalIP MachineAddressType = "InternalIP"
	// MachineExternalIP is a machine external IP address.
	MachineExternalIP MachineAddressType = "ExternalIP"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="ProviderID",type="string",JSONPath=".spec.providerID"
// +kubebuilder:printcolumn:name="State",type="string",JSONPath=".status.instanceState"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.initialization.provisioned"

// NebiusMachine is the Schema for the nebiusmachines API.
type NebiusMachine struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of NebiusMachine
	// +required
	Spec NebiusMachineSpec `json:"spec"`

	// status defines the observed state of NebiusMachine
	// +optional
	Status NebiusMachineStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// NebiusMachineList contains a list of NebiusMachine.
type NebiusMachineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []NebiusMachine `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NebiusMachine{}, &NebiusMachineList{})
}
