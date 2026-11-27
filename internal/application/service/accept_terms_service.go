package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/domain"
)

// AcceptTermsRepository is the persistence subset consumed by AcceptTermsService.
type AcceptTermsRepository interface {
	GetTermIDs(context.Context, uuid.UUID) ([]string, error)
	InsertTermsAcceptance(context.Context, domain.TermsAcceptance) error
	GetByIDForUpdate(context.Context, uuid.UUID) (domain.Application, error)
	AdvanceAfterTerms(context.Context, domain.TermsAcceptance) error
}

type AcceptTermsService struct {
	transactor Transactor
	repository AcceptTermsRepository
}

func NewAcceptTermsService(transactor Transactor, repository AcceptTermsRepository) *AcceptTermsService {
	return &AcceptTermsService{transactor: transactor, repository: repository}
}

func (s *AcceptTermsService) AcceptTerms(ctx context.Context, applicationID, termID string) error {
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
	if err := s.transactor.WithinTx(ctx, func(txCtx context.Context) error {
		app, err := s.repository.GetByIDForUpdate(txCtx, id)
		if err != nil {
			return err
		}
		terms, err := s.repository.GetTermIDs(txCtx, id)
		if err != nil {
			return err
		}
		replay, err := domain.CheckTermsAcceptance(app.Status, terms, termID)
		if err != nil {
			return err
		}
		if replay {
			return nil
		}
		if err := s.repository.InsertTermsAcceptance(txCtx, acceptance); err != nil {
			return err
		}
		return s.repository.AdvanceAfterTerms(txCtx, acceptance)
	}); err != nil {
		return fmt.Errorf("accept terms: %w", err)
	}
	return nil
}
