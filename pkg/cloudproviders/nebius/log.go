package nebius

import (
	"context"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/Azure/karpenter-provider-flex/pkg/options"
)

func loggerFromContext(ctx context.Context) logr.Logger {
	return log.FromContext(ctx).
		WithName(ProviderIDScheme).
		WithValues("nebius.project_id", options.MustGetNebiusProjectID(ctx))
}
