package service

import (
	"context"
	"errors"
	"time"

	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/domain"
)

type WorkflowStartOutcome int

const (
	WorkflowStartEstablished WorkflowStartOutcome = iota + 1
	WorkflowStartCompleted
	WorkflowStartClosedUnsuccessfully
)

var (
	ErrWorkflowClosed            = errors.New("workflow ended unsuccessfully; operational recovery required")
	ErrWorkflowStateInconsistent = errors.New("workflow and application state are inconsistent")
	ErrWorkflowUnavailable       = errors.New("workflow startup unavailable; retry the same submission")
	ErrWorkflowStartup           = errors.New("could not establish workflow startup")
)

// WorkflowStarter hides Temporal transport details from application operations.
type WorkflowStarter interface {
	EnsureStarted(context.Context, string, string) (WorkflowStartOutcome, error)
}

func (s *Service) startWorkflow(ctx context.Context, app domain.Application) (domain.Application, error) {
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
	current, err := s.repository.ResolveWorkflowStart(ctx, app.ApplicationID, target, time.Now().UTC())
	if err != nil {
		return domain.Application{}, err
	}
	if outcome == WorkflowStartClosedUnsuccessfully && current.Status == domain.ApplicationStatusFailedTechnical {
		return domain.Application{}, ErrWorkflowClosed
	}
	return current, nil
}
