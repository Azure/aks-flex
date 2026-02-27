package v1beta2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AKSClusterSpec defines the desired state of AKSCluster
type AKSClusterSpec struct {
	// resourceID is the ARM resource id of the AKS cluster.
	// +optional
	ResourceID string `json:"resourceID,omitempty"`
}

type AKSClusterInitializationStatus struct {
	// provisioned is true when the infrastructure provider reports that the Cluster's infrastructure is fully provisioned.
	// NOTE: this field is part of the Cluster API contract, and it is used to orchestrate initial Cluster provisioning.
	// +optional
	Provisioned *bool `json:"provisioned,omitempty"`
}

// AKSClusterStatus defines the observed state of AKSCluster.
type AKSClusterStatus struct {
	// initialization provides observations of the AKSCluster initialization process.
	// NOTE: Fields in this struct are part of the Cluster API contract and are used to orchestrate initial Cluster provisioning.
	// +optional
	Initialization AKSClusterInitializationStatus `json:"initialization,omitempty,omitzero"`

	// The status of each condition is one of True, False, or Unknown.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// AKSCluster is the Schema for the aksclusters API
type AKSCluster struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of AKSCluster
	// +required
	Spec AKSClusterSpec `json:"spec"`

	// status defines the observed state of AKSCluster
	// +optional
	Status AKSClusterStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// AKSClusterList contains a list of AKSCluster
type AKSClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []AKSCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AKSCluster{}, &AKSClusterList{})
}
