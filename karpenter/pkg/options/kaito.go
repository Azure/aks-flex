package options

import (
	"context"
	"fmt"

	"sigs.k8s.io/karpenter/pkg/operator/options"
)

func init() {
	options.Injectables = append(options.Injectables, &KaitoOptions{})
}

type kaitoOptionsKey struct{}

type KaitoOptions struct {
	NebiusProjectID string
	NebiusRegion    string
	NebiusSubnetID  string
}

var _ options.Injectable = (*KaitoOptions)(nil)

func (k *KaitoOptions) AddFlags(fs *options.FlagSet) {
	fs.StringVar(&k.NebiusProjectID, "flex-kaito.nebius-project-id", "", "The Nebius project ID to use for provisioning instances.")
	fs.StringVar(&k.NebiusRegion, "flex-kaito.nebius-region", "", "The Nebius region to use for provisioning instances.")
	fs.StringVar(&k.NebiusSubnetID, "flex-kaito.nebius-subnet-id", "", "The Nebius subnet ID to use for provisioning instances.")
}

func (k *KaitoOptions) Parse(fs *options.FlagSet, args ...string) error {
	return nil
}

func (k *KaitoOptions) Validate() error {
	if k.NebiusProjectID == "" {
		return fmt.Errorf("flex-kaito.nebius-project-id is required")
	}
	if k.NebiusRegion == "" {
		return fmt.Errorf("flex-kaito.nebius-region is required")
	}
	if k.NebiusSubnetID == "" {
		return fmt.Errorf("flex-kaito.nebius-subnet-id is required")
	}
	return nil
}

func (k *KaitoOptions) ToContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, kaitoOptionsKey{}, k)
}

func MustGetKaitoOptionsFromContext(ctx context.Context) *KaitoOptions {
	if v, ok := ctx.Value(kaitoOptionsKey{}).(*KaitoOptions); ok {
		return v
	}
	panic("kaito options not found in context")
}
