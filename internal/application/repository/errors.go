package repository

import "errors"

var (
	ErrNotFound               = errors.New("application not found")
	ErrDuplicateRequestID     = errors.New("application request ID already exists")
	ErrDuplicateApplicationID = errors.New("application ID already exists")
	ErrDuplicateWorkflowID    = errors.New("application workflow ID already exists")
	ErrDatabase               = errors.New("application database operation failed")
)
