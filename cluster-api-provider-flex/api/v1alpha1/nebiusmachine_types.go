package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NebiusMachineSpec defines the desired state of NebiusMachine
type NebiusMachineSpec struct {
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=512
	ProviderID string `json:"providerID,omitempty"`

	// TODO: addresses

	// Platform is the nebius platform to use for this machine.
	// +required
	Platform string `json:"platform,omitempty"`
	// Preset is the nebius preset to use for this machine.
	// +required
	Preset string `json:"preset,omitempty"`
	// ImageFamily is the nebius image family to use for this machine.
	// Defaults to infer from platform and preset if not specified.
	// +optional
	ImageFamily string `json:"imageFamily,omitempty"`
	// OSDiskSizeGibibytes is the size of the OS disk in GiB.
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

	// conditions represent the current state of the NebiusMachine resource.
	// Each condition has a unique type and reflects the status of a specific aspect of the resource.
	//
	// Standard condition types include:
	// - "Available": the resource is fully functional
	// - "Progressing": the resource is being created or updated
	// - "Degraded": the resource failed to reach or maintain its desired state
	//
	// The status of each condition is one of True, False, or Unknown.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// NebiusMachine is the Schema for the nebiusmachines API
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

// NebiusMachineList contains a list of NebiusMachine
type NebiusMachineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []NebiusMachine `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NebiusMachine{}, &NebiusMachineList{})
}
