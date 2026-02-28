package nebius

import (
	"context"
	"sync/atomic"

	"github.com/nebius/gosdk"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var sdkPointer atomic.Pointer[gosdk.SDK]

// FIXME: figure out the pattern for passing SDK clients around
func SetSDKDoNotUseInProd(sdk *gosdk.SDK) {
	sdkPointer.Store(sdk)
}

func MustGetSDK(ctx context.Context) *gosdk.SDK {
	rv := sdkPointer.Load()
	if rv == nil {
		panic("SDK not set")
	}
	return rv
}

// IsAlreadyExists checks if the error indicates the resource already exists.
func IsAlreadyExists(err error) bool {
	if s, ok := status.FromError(err); ok {
		return s.Code() == codes.AlreadyExists
	}
	return false
}

// IsNotFound checks if the error indicates the resource was not found.
func IsNotFound(err error) bool {
	if s, ok := status.FromError(err); ok {
		return s.Code() == codes.NotFound
	}
	return false
}
