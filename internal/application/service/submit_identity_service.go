package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/domain"
)

// SubmitIdentityRepository is the persistence subset consumed by SubmitIdentityService.
type SubmitIdentityRepository interface {
	GetByIDForUpdate(context.Context, uuid.UUID) (domain.Application, error)
	AdvanceAfterIdentity(context.Context, domain.Identity) (domain.Application, error)
	GetByID(context.Context, uuid.UUID) (domain.Application, error)
	ResolveWorkflowStart(context.Context, uuid.UUID, domain.ApplicationStatus, time.Time) (domain.Application, error)
	GetIdentity(context.Context, uuid.UUID) (*domain.Identity, error)
	InsertIdentity(context.Context, domain.Identity) error
}

type SubmitIdentityService struct {
	transactor Transactor
	repository SubmitIdentityRepository
	starter    WorkflowStarter
}

func NewSubmitIdentityService(transactor Transactor, repository SubmitIdentityRepository, starter WorkflowStarter) *SubmitIdentityService {
	return &SubmitIdentityService{transactor: transactor, repository: repository, starter: starter}
}

func (s *SubmitIdentityService) SubmitIdentity(ctx context.Context, applicationID string, input *domain.Identity) (domain.Application, error) {
	id, err := uuid.Parse(applicationID)
	if err != nil || id == uuid.Nil {
		return domain.Application{}, ErrInvalidApplicationID
	}
	if err := domain.ValidateIdentityStructure(input); err != nil {
		return domain.Application{}, err
	}
	i := *input
	i.ApplicationID, i.CreatedAt = id, time.Now().UTC()
	if input.Email != nil {
		email := *input.Email
		i.Email = &email
	}
	var app domain.Application
	err = s.transactor.WithinTx(ctx, func(txCtx context.Context) error {
		var err error
		app, err = s.repository.GetByIDForUpdate(txCtx, id)
		if err != nil {
			return err
		}
		existing, err := s.repository.GetIdentity(txCtx, id)
		if err != nil {
			return err
		}
		replay, err := domain.CheckIdentitySubmission(app.Status, existing, i, i.CreatedAt)
		if err != nil {
			return err
		}
		if replay {
			return nil
		}
		if err := s.repository.InsertIdentity(txCtx, i); err != nil {
			return err
		}
		app, err = s.repository.AdvanceAfterIdentity(txCtx, i)
		return err
	})
	if err != nil {
		return domain.Application{}, fmt.Errorf("submit identity: %w", err)
	}
	return s.startWorkflow(ctx, app)
}
