package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/domain"
)

var (
	ErrNotFound               = errors.New("application not found")
	ErrDuplicateRequestID     = errors.New("application request ID already exists")
	ErrDuplicateApplicationID = errors.New("application ID already exists")
	ErrDuplicateWorkflowID    = errors.New("application workflow ID already exists")
	ErrDatabase               = errors.New("application database operation failed")
)

// Repository persists application-owned state. Create never overwrites a row;
// callers can resolve duplicate requests through GetByRequestID.
type Repository interface {
	Create(context.Context, domain.Application) error
	GetByID(context.Context, uuid.UUID) (domain.Application, error)
	GetByRequestID(context.Context, uuid.UUID) (domain.Application, error)
}
