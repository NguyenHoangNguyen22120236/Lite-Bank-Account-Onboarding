package service

import "errors"

var (
	ErrInvalidRequestID     = errors.New("request_id must be a non-nil UUID")
	ErrInvalidApplicationID = errors.New("application_id must be a non-nil UUID")
	ErrInvalidTermID        = errors.New("term_id must be nonblank and contain no NUL characters")
)
