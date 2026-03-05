package v1alpha1

import (
	"github.com/awslabs/operatorpkg/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ConditionTypeValidationSucceeded = "ValidationSucceeded"
)

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=nebiusnodeclasses,scope=Cluster,categories={karpenter,nap},shortName={nbnc,nbncs}
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=".metadata.creationTimestamp"
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
type NebiusNodeClass struct {
	metav1.TypeMeta `json:",inline"`
	// metadata is standard object metadata.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +optional
	Spec NebiusNodeClassSpec `json:"spec,omitempty"`

	// status contains the resolved state of the NebiusNodeClass.
	// +optional
	Status NebiusNodeClassStatus `json:"status,omitempty"`
}

var _ status.Object = (*NebiusNodeClass)(nil)

func (s *NebiusNodeClass) GetConditions() []status.Condition {
	return s.Status.Conditions
}

func (s *NebiusNodeClass) SetConditions(conditions []status.Condition) {
	s.Status.Conditions = conditions
}

func (s *NebiusNodeClass) StatusConditions() status.ConditionSet {
	conds := []string{
		ConditionTypeValidationSucceeded,
	}

	return status.NewReadyConditions(conds...).For(s)
}

type NebiusNodeClassSpec struct {
	// ProjectID is the nebius project id to launch nodes in.
	// +required
	ProjectID string `json:"projectID,omitempty"`
	// Region sets the nebius region to launch nodes in. Must be a region where the specified subnet exists.
	// +required
	Region string `json:"region,omitempty"`
	// SubnetID is the nebius subnet id to launch nodes in.
	// Node will be auto-assigned an IP from this subnet.
	// +required
	SubnetID string `json:"subnetID,omitempty"`
	// OSDiskSizeGB is the size of the OS disk in GB.
	// +default=128
	// +optional
	OSDiskSizeGB *int32 `json:"osDiskSizeGB,omitempty"`
	// +default="ubuntu24.04-driverless"
	// +optional
	OSDiskImageFamily *string `json:"osDiskImageFamily,omitempty"`

	// +default=false
	// +optional
	AllocateNodePublicIP *bool `json:"allocateNodePublicIP,omitempty"`

	// TODO: other fields (kublet etc)
}

type NebiusNodeClassStatus struct {
	// conditions contains signals for health and readiness
	// +optional
	//nolint:kubeapilinter // conditions: using status.Condition from operatorpkg instead of metav1.Condition for compatibility
	Conditions []status.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
type NebiusNodeClassList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NebiusNodeClass `json:"items"`
}
