package options

import (
	"context"

	"github.com/samber/lo"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/karpenter/pkg/operator/options"

	"github.com/Azure/aks-flex/plugin/pkg/services"
	nebiusutil "github.com/Azure/aks-flex/plugin/pkg/util/nebius"
)

func init() {
	options.Injectables = append(options.Injectables, &stretchOptions{})
}

type stretchOptionsKey struct{}

type stretchOptions struct {
	DatabaseNamespace string
}

var _ options.Injectable = (*stretchOptions)(nil)

func (s *stretchOptions) AddFlags(fs *options.FlagSet) {
	fs.StringVar(&s.DatabaseNamespace, "stretch.database-namespace", "karpenter", "The namespace where stretch cloud provider will store its database.")
}

func (s *stretchOptions) Parse(fs *options.FlagSet, args ...string) error {
	// NOTE: just assume other options have been parsed

	return nil
}

func (s *stretchOptions) ToContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, stretchOptionsKey{}, s)
}

func stretchOptionsFromContext(ctx context.Context) *stretchOptions {
	if v, ok := ctx.Value(stretchOptionsKey{}).(*stretchOptions); ok {
		return v
	}
	return nil
}

func MustInitalizeStretchPlugin(ctx context.Context, restConfig *rest.Config) {
	stretchOpts := stretchOptionsFromContext(ctx)
	lo.Assert(stretchOpts != nil, "stretch options not found in context")
	nebiusOpts := nebiusOptionsFromContext(ctx)
	lo.Assert(nebiusOpts != nil, "nebius options not found in context")

	nebiusutil.SetSDKDoNotUseInProd(MustNewNebiusSDK(ctx))

	kubeClient := lo.Must(kubernetes.NewForConfig(restConfig))

	_, err := services.NewConnectionWithSecretDB(
		kubeClient,
		stretchOpts.DatabaseNamespace,
	)
	lo.Must0(err, "initializing stretch plugin database connection")
}
