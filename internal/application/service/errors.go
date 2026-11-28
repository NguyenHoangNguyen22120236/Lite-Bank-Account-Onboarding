package service

import "errors"

var (
	ErrInvalidRequestID     = errors.New("request_id must be a non-nil UUID")
	ErrInvalidApplicationID = errors.New("application_id must be a non-nil UUID")
	ErrInvalidTermID        = errors.New("term_id must be nonblank and contain no NUL characters")
)

var (
	ErrWorkflowClosed            = errors.New("workflow ended unsuccessfully; operational recovery required")
	ErrWorkflowStateInconsistent = errors.New("workflow and application state are inconsistent")
	ErrWorkflowUnavailable       = errors.New("workflow startup unavailable; retry StartApplication with the same request_id")
	ErrWorkflowStartup           = errors.New("could not establish workflow startup")
)
