package objectstore

import (
	"errors"

	"github.com/aws/smithy-go"
)

func readError(err error) error {
	var apiError smithy.APIError
	var httpError interface{ HTTPStatusCode() int }
	if errors.As(err, &apiError) && apiError.ErrorCode() == "NoSuchKey" &&
		errors.As(err, &httpError) && httpError.HTTPStatusCode() == 404 {
		return errors.Join(ErrNotFound, safeError("read", err))
	}
	return safeError("read", err)
}
