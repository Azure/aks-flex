package nebius

import (
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func IsNotFound(err error) bool {
	if s, ok := status.FromError(err); ok {
		return s.Code() == codes.NotFound
	}
	return false
}

func IsQuotaError(err error) bool {
	if err == nil {
		return false
	}
	// FIXME: check how to identify the error using nebius sdk
	errString := err.Error()
	return strings.Contains(errString, "Quota failure") ||
		strings.Contains(errString, "Not enough resources") ||
		strings.Contains(errString, "insufficient capacity") // example: insufficient capacity, rpc error: code = ResourceExhausted
}
