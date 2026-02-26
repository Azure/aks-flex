package aws

import (
	"errors"

	"github.com/aws/smithy-go"
)

func isErrorCode(err error, errorCode string) bool {
	var apiErr smithy.APIError
	return errors.As(err, &apiErr) && apiErr.ErrorCode() == errorCode
}

func isErrorMessage(err error, errorMessage string) bool {
	var apiErr smithy.APIError
	return errors.As(err, &apiErr) && apiErr.ErrorMessage() == errorMessage
}
