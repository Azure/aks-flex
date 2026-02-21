package nebius

import (
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/Azure/aks-flex/karpenter-provider-flex/pkg/apis"
)

const (
	ProviderIDScheme = "aks-nebius"
)

var GroupKind = schema.GroupKind{
	Group: apis.Group,
	Kind:  "NebiusNodeClass",
}
