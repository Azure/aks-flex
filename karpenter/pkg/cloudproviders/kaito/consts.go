package kaito

import (
	"k8s.io/apimachinery/pkg/runtime/schema"

	kaitoapis "github.com/Azure/aks-flex/karpenter/pkg/apis/kaito"
)

const (
	ProviderIDScheme = "aks-kaito"

	kaitoNodeClassName = "default"
)

var GroupKind = schema.GroupKind{
	Group: kaitoapis.Group,
	Kind:  "KaitoNodeClass",
}
