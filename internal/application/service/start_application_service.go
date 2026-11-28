package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/domain"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/repository"
)

// StartApplicationRepository is the persistence subset consumed by StartApplicationService.
type StartApplicationRepository interface {
	Create(context.Context, domain.Application) error
	GetByRequestID(context.Context, uuid.UUID) (domain.Application, error)
}

type StartApplicationService struct {
	repository StartApplicationRepository
	starter    WorkflowStarter
}

func NewStartApplicationService(repository StartApplicationRepository, starter WorkflowStarter) *StartApplicationService {
	return &StartApplicationService{repository: repository, starter: starter}
}

func (s *StartApplicationService) StartApplication(ctx context.Context, requestID string) (domain.Application, error) {
	id, err := uuid.Parse(requestID)
	if err != nil || id == uuid.Nil {
		return domain.Application{}, ErrInvalidRequestID
	}
	app, err := domain.NewApplication(uuid.New(), id, time.Now())
	if err != nil {
		return domain.Application{}, fmt.Errorf("initialize application: %w", err)
	}
	if err := s.repository.Create(ctx, app); err != nil {
		if !errors.Is(err, repository.ErrDuplicateRequestID) {
			return domain.Application{}, fmt.Errorf("start application: %w", err)
		}
		// PostgreSQL uniqueness serializes concurrent attempts for the same request.
		existing, err := s.repository.GetByRequestID(ctx, id)
		if err != nil {
			return domain.Application{}, fmt.Errorf("replay application: %w", err)
		}
		app = existing
	}
	return s.startWorkflow(ctx, app)
}
