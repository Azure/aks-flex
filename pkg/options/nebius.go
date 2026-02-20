package options

import (
	"context"
	"fmt"

	"github.com/nebius/gosdk"
	"github.com/nebius/gosdk/auth"
	"github.com/samber/lo"
	"sigs.k8s.io/karpenter/pkg/operator/options"
)

func init() {
	options.Injectables = append(options.Injectables, &nebiusOptions{})
}

type nebiusOptionsKey struct{}

type nebiusOptions struct {
	CredentialsFile string
}

var _ options.Injectable = (*nebiusOptions)(nil)

func (n *nebiusOptions) AddFlags(fs *options.FlagSet) {
	fs.StringVar(&n.CredentialsFile, "flex-nebius.credentials-file", "", "The path to the Nebius credentials file.")
}

func (n *nebiusOptions) Parse(fs *options.FlagSet, args ...string) error {
	// NOTE: just assume other options have been parsed

	if n.CredentialsFile == "" {
		return fmt.Errorf("flex-nebius.credentials-file is required")
	}

	return nil
}

func (n *nebiusOptions) ToContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, nebiusOptionsKey{}, n)
}

func nebiusOptionsFromContext(ctx context.Context) *nebiusOptions {
	if v, ok := ctx.Value(nebiusOptionsKey{}).(*nebiusOptions); ok {
		return v
	}
	return nil
}

func MustNewNebiusSDK(ctx context.Context) *gosdk.SDK {
	opts := nebiusOptionsFromContext(ctx)
	lo.Assert(opts != nil, "nebius options not found in context")

	return lo.Must(gosdk.New(ctx,
		gosdk.WithCredentials(
			gosdk.ServiceAccountReader(
				auth.NewServiceAccountCredentialsFileParser(nil, opts.CredentialsFile),
			),
		),
	))
}
