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

var ErrInvalidRequestID = errors.New("request_id must be a non-nil UUID")

// Repository is the persistence subset needed to start or replay an application.
type Repository interface {
	Create(context.Context, domain.Application) error
	GetByRequestID(context.Context, uuid.UUID) (domain.Application, error)
}

type Service struct {
	repository Repository
}

func New(repository Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) StartApplication(ctx context.Context, requestID string) (domain.Application, error) {
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
		return existing, nil
	}
	return app, nil
}
