package nebius

import (
	"context"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func loggerFromContext(ctx context.Context) logr.Logger {
	return log.FromContext(ctx).
		WithName(ProviderIDScheme)
}
