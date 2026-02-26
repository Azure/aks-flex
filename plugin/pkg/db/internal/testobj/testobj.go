package testobj

import "github.com/Azure/aks-flex/plugin/api"

var _ api.Object = (*FakeObject)(nil)

func (x *FakeObject) Redact() {}
