package api

import "errors"

// IsStatus reports whether err is (or wraps) an APIError with the given HTTP
// status code.
func IsStatus(err error, status int) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == status
}
