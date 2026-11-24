package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/domain"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/repository"
)

// Repository is the persistence subset needed by application operations.
type Repository interface {
	SubmitIdentity(context.Context, domain.Identity) (domain.Application, error)
	AcceptTerms(context.Context, domain.TermsAcceptance) error
	Create(context.Context, domain.Application) error
	GetByRequestID(context.Context, uuid.UUID) (domain.Application, error)
}

type Service struct {
	repository Repository
}

func New(repository Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) SubmitIdentity(ctx context.Context, applicationID string, input *domain.Identity) (domain.Application, error) {
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
	app, err := s.repository.SubmitIdentity(ctx, i)
	if err != nil {
		return domain.Application{}, fmt.Errorf("submit identity: %w", err)
	}
	return app, nil
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

func (s *Service) AcceptTerms(ctx context.Context, applicationID, termID string) error {
	id, err := uuid.Parse(applicationID)
	if err != nil || id == uuid.Nil {
		return ErrInvalidApplicationID
	}
	if strings.TrimSpace(termID) == "" || strings.ContainsRune(termID, '\x00') {
		return ErrInvalidTermID
	}
	acceptance := domain.TermsAcceptance{
		AcceptanceID: uuid.New(), ApplicationID: id,
		TermID: termID, AcceptedAt: time.Now().UTC(),
	}
	if err := s.repository.AcceptTerms(ctx, acceptance); err != nil {
		return fmt.Errorf("accept terms: %w", err)
	}
	return nil
}
