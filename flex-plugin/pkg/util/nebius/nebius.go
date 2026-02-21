package nebius

import (
	"context"
	"errors"

	"github.com/nebius/gosdk"
	"github.com/nebius/gosdk/auth"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// NewSDK creates a new Nebius SDK client with service account credentials.
func NewSDK(ctx context.Context, credentialsFile string) (*gosdk.SDK, error) {
	if credentialsFile == "" {
		return nil, errors.New("NEBIUS_CREDENTIALS_FILE environment variable not set")
	}

	return gosdk.New(ctx,
		gosdk.WithCredentials(
			gosdk.ServiceAccountReader(
				auth.NewServiceAccountCredentialsFileParser(nil, credentialsFile),
			),
		),
	)
}

// isAlreadyExists checks if the error indicates the resource already exists.
func isAlreadyExists(err error) bool {
	if s, ok := status.FromError(err); ok {
		return s.Code() == codes.AlreadyExists
	}
	return false
}

// isNotFound checks if the error indicates the resource was not found.
func isNotFound(err error) bool {
	if s, ok := status.FromError(err); ok {
		return s.Code() == codes.NotFound
	}
	return false
}
