package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type NebiusMachineTemplateResource struct {
	// +optional
	ObjectMeta metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec       NebiusMachineSpec `json:"spec,omitempty"`
}

// NebiusMachineTemplateSpec defines the desired state of NebiusMachineTemplate
type NebiusMachineTemplateSpec struct {
	Template NebiusMachineTemplateResource `json:"template"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// NebiusMachineTemplate is the Schema for the nebiusmachinetemplates API
type NebiusMachineTemplate struct {
	metav1.TypeMeta `json:",inline"`
	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of NebiusMachineTemplate
	// +required
	Spec NebiusMachineTemplateSpec `json:"spec"`
}

// +kubebuilder:object:root=true

// NebiusMachineTemplateList contains a list of NebiusMachineTemplate
type NebiusMachineTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []NebiusMachineTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NebiusMachineTemplate{}, &NebiusMachineTemplateList{})
}
