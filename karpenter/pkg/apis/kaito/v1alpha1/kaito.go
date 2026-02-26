package v1alpha1

import (
	"github.com/awslabs/operatorpkg/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// KaitoNodeClass is the Schema for the KaitoNodeClass API
// +kubebuilder:object:root=true
// +kubebuilder:resource:path=kaitonodeclasses,scope=Cluster,categories=karpenter,shortName={knc}
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
type KaitoNodeClass struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KaitoNodeClassSpec   `json:"spec,omitempty"`
	Status KaitoNodeClassStatus `json:"status,omitempty"`
}

func (in *KaitoNodeClass) StatusConditions() status.ConditionSet {
	return status.ConditionSet{}
}

func (in *KaitoNodeClass) GetConditions() []status.Condition {
	return []status.Condition{}
}

func (in *KaitoNodeClass) SetConditions(conditions []status.Condition) {
	// no-op
}

type KaitoNodeClassSpec struct {
	// Add fields here
}

type KaitoNodeClassStatus struct {
	// Add fields here
}

// KaitoNodeClassList contains a list of KaitoNodeClass
// +kubebuilder:object:root=true
type KaitoNodeClassList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KaitoNodeClass `json:"items"`
}
