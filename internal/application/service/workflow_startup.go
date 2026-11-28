package service

import (
	"context"

	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/domain"
)

type WorkflowStartOutcome int

const (
	WorkflowStartEstablished WorkflowStartOutcome = iota + 1
	WorkflowStartCompleted
	WorkflowStartClosedUnsuccessfully
)

// WorkflowStarter is the startup port consumed by StartApplicationService.
type WorkflowStarter interface {
	EnsureStarted(context.Context, string, string) (WorkflowStartOutcome, error)
}

// startWorkflow runs after persistence and never writes business state.
func (s *StartApplicationService) startWorkflow(ctx context.Context, app domain.Application) (domain.Application, error) {
	if app.Status.Terminal() {
		return app, nil
	}
	if !app.Status.Valid() || app.ApplicationID.String() != app.TemporalWorkflowID {
		return domain.Application{}, ErrWorkflowStateInconsistent
	}
	if s.starter == nil {
		return domain.Application{}, ErrWorkflowStartup
	}
	outcome, err := s.starter.EnsureStarted(ctx, app.ApplicationID.String(), app.TemporalWorkflowID)
	if err != nil {
		return domain.Application{}, err
	}
	switch outcome {
	case WorkflowStartEstablished, WorkflowStartCompleted, WorkflowStartClosedUnsuccessfully:
	default:
		return domain.Application{}, ErrWorkflowStartup
	}
	current, err := s.repository.GetByRequestID(ctx, app.RequestID)
	if err != nil {
		return domain.Application{}, err
	}
	if current.ApplicationID != app.ApplicationID || current.TemporalWorkflowID != app.TemporalWorkflowID {
		return domain.Application{}, ErrWorkflowStateInconsistent
	}
	switch outcome {
	case WorkflowStartEstablished:
		return current, nil
	case WorkflowStartCompleted:
		if current.Status == domain.ApplicationStatusCompleted || current.Status == domain.ApplicationStatusRejected {
			return current, nil
		}
		return domain.Application{}, ErrWorkflowStateInconsistent
	default:
		// A concurrent terminal business result is authoritative; never overwrite it.
		if current.Status.Terminal() {
			return current, nil
		}
		return domain.Application{}, ErrWorkflowClosed
	}
}
