package service

import (
	"context"
	"time"

	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/domain"
)

type WorkflowStartOutcome int

const (
	WorkflowStartEstablished WorkflowStartOutcome = iota + 1
	WorkflowStartCompleted
	WorkflowStartClosedUnsuccessfully
)

// WorkflowStarter hides Temporal transport details from application operations.
type WorkflowStarter interface {
	EnsureStarted(context.Context, string, string) (WorkflowStartOutcome, error)
}

func (s *SubmitIdentityService) startWorkflow(ctx context.Context, app domain.Application) (domain.Application, error) {
	if app.Status != domain.ApplicationStatusPendingWorkflowStart {
		return app, nil
	}
	if app.TemporalWorkflowID != app.ApplicationID.String() {
		return domain.Application{}, ErrWorkflowStateInconsistent
	}
	if s.starter == nil {
		return domain.Application{}, ErrWorkflowStartup
	}
	outcome, err := s.starter.EnsureStarted(ctx, app.ApplicationID.String(), app.TemporalWorkflowID)
	if err != nil {
		return domain.Application{}, err
	}
	if outcome == WorkflowStartCompleted {
		current, err := s.repository.GetByID(ctx, app.ApplicationID)
		if err != nil {
			return domain.Application{}, err
		}
		if current.Status != domain.ApplicationStatusCompleted && current.Status != domain.ApplicationStatusRejected {
			return domain.Application{}, ErrWorkflowStateInconsistent
		}
		return current, nil
	}
	var target domain.ApplicationStatus
	switch outcome {
	case WorkflowStartEstablished:
		target = domain.ApplicationStatusPendingIdentityVerification
	case WorkflowStartClosedUnsuccessfully:
		target = domain.ApplicationStatusFailedTechnical
	default:
		return domain.Application{}, ErrWorkflowStartup
	}
	var current domain.Application
	err = s.transactor.WithinTx(ctx, func(txCtx context.Context) error {
		var err error
		current, err = s.repository.GetByIDForUpdate(txCtx, app.ApplicationID)
		if err != nil {
			return err
		}
		if current.Status != domain.ApplicationStatusPendingWorkflowStart {
			return nil
		}
		current, err = s.repository.ResolveWorkflowStart(txCtx, app.ApplicationID, target, time.Now().UTC())
		return err
	})
	if err != nil {
		return domain.Application{}, err
	}
	if outcome == WorkflowStartClosedUnsuccessfully && current.Status == domain.ApplicationStatusFailedTechnical {
		return domain.Application{}, ErrWorkflowClosed
	}
	return current, nil
}
