package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/domain"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/repository"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestStartApplicationStartsAfterPersistence(t *testing.T) {
	ctx := context.Background()
	repo, starter := NewMockStartApplicationRepository(t), NewMockWorkflowStarter(t)
	var persisted domain.Application
	create := repo.EXPECT().Create(ctx, mock.Anything).Run(func(_ context.Context, app domain.Application) {
		persisted = app
	}).Return(nil).Once()
	start := starter.EXPECT().EnsureStarted(ctx, mock.Anything, mock.Anything).
		Run(func(_ context.Context, applicationID, workflowID string) {
			require.Equal(t, persisted.ApplicationID.String(), applicationID)
			require.Equal(t, applicationID, workflowID)
			require.Equal(t, domain.ApplicationStatusPendingTermsAcceptance, persisted.Status)
		}).Return(WorkflowStartEstablished, nil).Once()
	read := repo.EXPECT().GetByRequestID(ctx, mock.Anything).RunAndReturn(func(_ context.Context, requestID uuid.UUID) (domain.Application, error) {
		require.Equal(t, persisted.RequestID, requestID)
		return persisted, nil
	}).Once()
	mock.InOrder(create, start, read)
	got, err := NewStartApplicationService(repo, starter).StartApplication(ctx, uuid.NewString())
	require.NoError(t, err)
	require.Equal(t, persisted, got, "startup must not modify business status or timestamps")
}

func TestStartApplicationStartupReplay(t *testing.T) {
	for _, status := range []domain.ApplicationStatus{
		domain.ApplicationStatusPendingTermsAcceptance, domain.ApplicationStatusPendingIdentitySubmission,
		domain.ApplicationStatusPendingIdentityVerification, domain.ApplicationStatusPendingSanctionsScreening,
		domain.ApplicationStatusPendingHumanReview, domain.ApplicationStatusApproved,
		domain.ApplicationStatusPendingAccountCreation, domain.ApplicationStatusPendingWorkflowStart,
		domain.ApplicationStatusCompleted, domain.ApplicationStatusRejected, domain.ApplicationStatusFailedTechnical,
	} {
		t.Run(string(status), func(t *testing.T) {
			ctx := context.Background()
			app, err := domain.NewApplication(uuid.New(), uuid.New(), time.Now())
			require.NoError(t, err)
			app.Status = status
			repo, starter := NewMockStartApplicationRepository(t), NewMockWorkflowStarter(t)
			create := repo.EXPECT().Create(ctx, mock.Anything).Return(repository.ErrDuplicateRequestID).Once()
			lookup := repo.EXPECT().GetByRequestID(ctx, app.RequestID).Return(app, nil).Once()
			mock.InOrder(create, lookup)
			if status != domain.ApplicationStatusCompleted && status != domain.ApplicationStatusRejected && status != domain.ApplicationStatusFailedTechnical {
				start := starter.EXPECT().EnsureStarted(ctx, app.ApplicationID.String(), app.TemporalWorkflowID).Return(WorkflowStartEstablished, nil).Once()
				refresh := repo.EXPECT().GetByRequestID(ctx, app.RequestID).Return(app, nil).Once()
				mock.InOrder(lookup, start, refresh)
			}
			got, err := NewStartApplicationService(repo, starter).StartApplication(ctx, app.RequestID.String())
			require.NoError(t, err)
			require.Equal(t, app, got)
		})
	}
}

func TestStartApplicationStartupFailureAndRetry(t *testing.T) {
	for _, cause := range []error{ErrWorkflowUnavailable, ErrWorkflowStartup, context.Canceled, context.DeadlineExceeded} {
		t.Run(cause.Error(), func(t *testing.T) {
			ctx := context.Background()
			repo, starter := NewMockStartApplicationRepository(t), NewMockWorkflowStarter(t)
			var app domain.Application
			create := repo.EXPECT().Create(ctx, mock.Anything).Run(func(_ context.Context, value domain.Application) { app = value }).Return(nil).Once()
			start := starter.EXPECT().EnsureStarted(ctx, mock.Anything, mock.Anything).Return(WorkflowStartOutcome(0), cause).Once()
			mock.InOrder(create, start)
			s := NewStartApplicationService(repo, starter)
			requestID := uuid.New()
			got, err := s.StartApplication(ctx, requestID.String())
			require.ErrorIs(t, err, cause)
			require.Empty(t, got)
			starter.AssertNumberOfCalls(t, "EnsureStarted", 1)
			retry := repo.EXPECT().Create(ctx, mock.Anything).Return(repository.ErrDuplicateRequestID).Once()
			lookup := repo.EXPECT().GetByRequestID(ctx, requestID).Return(app, nil).Once()
			restart := starter.EXPECT().EnsureStarted(ctx, app.ApplicationID.String(), app.TemporalWorkflowID).Return(WorkflowStartEstablished, nil).Once()
			refresh := repo.EXPECT().GetByRequestID(ctx, requestID).Return(app, nil).Once()
			mock.InOrder(retry, lookup, restart, refresh)
			got, err = s.StartApplication(ctx, requestID.String())
			require.NoError(t, err)
			require.Equal(t, app, got)
		})
	}
}

func TestStartApplicationStartupReconciliation(t *testing.T) {
	for _, tc := range []struct {
		name          string
		outcome       WorkflowStartOutcome
		status        domain.ApplicationStatus
		readErr, want error
	}{
		{"concurrent terms", WorkflowStartEstablished, domain.ApplicationStatusPendingIdentitySubmission, nil, nil},
		{"concurrent completion", WorkflowStartEstablished, domain.ApplicationStatusCompleted, nil, nil},
		{"completed consistent", WorkflowStartCompleted, domain.ApplicationStatusCompleted, nil, nil},
		{"completed rejection", WorkflowStartCompleted, domain.ApplicationStatusRejected, nil, nil},
		{"completed inconsistent", WorkflowStartCompleted, domain.ApplicationStatusPendingTermsAcceptance, nil, ErrWorkflowStateInconsistent},
		{"closed unsuccessful", WorkflowStartClosedUnsuccessfully, domain.ApplicationStatusPendingTermsAcceptance, nil, ErrWorkflowClosed},
		{"refresh failure", WorkflowStartEstablished, domain.ApplicationStatusPendingTermsAcceptance, repository.ErrDatabase, repository.ErrDatabase},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			app, err := domain.NewApplication(uuid.New(), uuid.New(), time.Now())
			require.NoError(t, err)
			current := app
			current.Status = tc.status
			repo, starter := NewMockStartApplicationRepository(t), NewMockWorkflowStarter(t)
			create := repo.EXPECT().Create(ctx, mock.Anything).Return(repository.ErrDuplicateRequestID).Once()
			lookup := repo.EXPECT().GetByRequestID(ctx, app.RequestID).Return(app, nil).Once()
			start := starter.EXPECT().EnsureStarted(ctx, app.ApplicationID.String(), app.TemporalWorkflowID).Return(tc.outcome, nil).Once()
			refresh := repo.EXPECT().GetByRequestID(ctx, app.RequestID).Return(current, tc.readErr).Once()
			mock.InOrder(create, lookup, start, refresh)
			got, err := NewStartApplicationService(repo, starter).StartApplication(ctx, app.RequestID.String())
			require.ErrorIs(t, err, tc.want)
			if tc.want == nil {
				require.Equal(t, current, got)
			} else {
				require.Empty(t, got)
			}
		})
	}
}

func TestStartApplicationNoStartupOnPersistenceFailure(t *testing.T) {
	ctx := context.Background()
	repo, starter := NewMockStartApplicationRepository(t), NewMockWorkflowStarter(t)
	repo.EXPECT().Create(ctx, mock.Anything).Return(repository.ErrDatabase).Once()
	_, err := NewStartApplicationService(repo, starter).StartApplication(ctx, uuid.NewString())
	require.ErrorIs(t, err, repository.ErrDatabase)
}

func TestStartApplicationRejectsInconsistentWorkflowID(t *testing.T) {
	for _, workflowID := range []string{"", uuid.NewString()} {
		t.Run(workflowID, func(t *testing.T) {
			ctx := context.Background()
			app, err := domain.NewApplication(uuid.New(), uuid.New(), time.Now())
			require.NoError(t, err)
			app.TemporalWorkflowID = workflowID
			repo, starter := NewMockStartApplicationRepository(t), NewMockWorkflowStarter(t)
			create := repo.EXPECT().Create(ctx, mock.Anything).Return(repository.ErrDuplicateRequestID).Once()
			read := repo.EXPECT().GetByRequestID(ctx, app.RequestID).Return(app, nil).Once()
			mock.InOrder(create, read)
			got, err := NewStartApplicationService(repo, starter).StartApplication(ctx, app.RequestID.String())
			require.ErrorIs(t, err, ErrWorkflowStateInconsistent)
			require.Empty(t, got)
		})
	}
}

func TestStartApplicationRejectsUnknownStartupOutcome(t *testing.T) {
	ctx := context.Background()
	repo, starter := NewMockStartApplicationRepository(t), NewMockWorkflowStarter(t)
	create := repo.EXPECT().Create(ctx, mock.Anything).Return(nil).Once()
	start := starter.EXPECT().EnsureStarted(ctx, mock.Anything, mock.Anything).Return(WorkflowStartOutcome(0), nil).Once()
	mock.InOrder(create, start)
	got, err := NewStartApplicationService(repo, starter).StartApplication(ctx, uuid.NewString())
	require.ErrorIs(t, err, ErrWorkflowStartup)
	require.Empty(t, got)
}

func TestStartApplicationValidation(t *testing.T) {
	for _, input := range []string{
		"", "   ", "invalid", uuid.Nil.String(),
	} {
		t.Run(fmt.Sprintf("request_%q", input), func(t *testing.T) {
			repo := NewMockStartApplicationRepository(t)

			service := NewStartApplicationService(repo, nil)

			got, err := service.StartApplication(
				context.Background(),
				input,
			)

			require.ErrorIs(t, err, ErrInvalidRequestID)
			require.Equal(t, domain.Application{}, got)
		})
	}
}

func TestStartApplicationCreation(t *testing.T) {
	requestID := uuid.New()
	ctx := context.Background()

	repo := NewMockStartApplicationRepository(t)

	var stored domain.Application

	repo.EXPECT().
		Create(ctx, mock.Anything).
		Run(func(_ context.Context, app domain.Application) {
			stored = app
		}).
		Return(nil).
		Once()

	starter := NewMockWorkflowStarter(t)
	starter.EXPECT().EnsureStarted(ctx, mock.Anything, mock.Anything).Return(WorkflowStartEstablished, nil).Once()
	repo.EXPECT().GetByRequestID(ctx, requestID).RunAndReturn(func(context.Context, uuid.UUID) (domain.Application, error) { return stored, nil }).Once()
	service := NewStartApplicationService(repo, starter)

	before := time.Now().UTC()
	got, err := service.StartApplication(ctx, requestID.String())
	after := time.Now().UTC()

	require.NoError(t, err)
	require.Equal(t, stored, got)

	require.NotEqual(t, uuid.Nil, got.ApplicationID)
	require.Equal(t, requestID, got.RequestID)

	require.Equal(t,
		domain.ApplicationStatusPendingTermsAcceptance,
		got.Status,
	)

	require.Equal(t,
		got.ApplicationID.String(),
		got.TemporalWorkflowID,
	)

	require.Nil(t, got.CompletedAt)
	require.Nil(t, got.RejectedAtStep)

	require.False(t, got.CreatedAt.Before(before))
	require.False(t, got.CreatedAt.After(after))
	require.Equal(t, time.UTC, got.CreatedAt.Location())
	require.Equal(t, got.CreatedAt, got.UpdatedAt)
}

func TestStartApplicationReplay(t *testing.T) {
	for _, duplicate := range []error{
		repository.ErrDuplicateRequestID,
		fmt.Errorf("create: %w", repository.ErrDuplicateRequestID),
	} {
		t.Run(duplicate.Error(), func(t *testing.T) {
			original, err := domain.NewApplication(
				uuid.New(),
				uuid.New(),
				time.Now(),
			)
			require.NoError(t, err)

			original.Status =
				domain.ApplicationStatusPendingIdentitySubmission

			repo := NewMockStartApplicationRepository(t)

			repo.EXPECT().
				Create(mock.Anything, mock.Anything).
				Return(duplicate).
				Once()

			repo.EXPECT().
				GetByRequestID(mock.Anything, original.RequestID).
				Return(original, nil).
				Twice()

			starter := NewMockWorkflowStarter(t)
			starter.EXPECT().EnsureStarted(mock.Anything, original.ApplicationID.String(), original.TemporalWorkflowID).Return(WorkflowStartEstablished, nil).Once()
			service := NewStartApplicationService(repo, starter)

			got, err := service.StartApplication(
				context.Background(),
				original.RequestID.String(),
			)

			require.NoError(t, err)
			require.Equal(t, original, got)
		})
	}
}

func TestStartApplicationErrorPropagation(t *testing.T) {
	for _, cause := range []error{
		repository.ErrDatabase,
		repository.ErrDuplicateApplicationID,
		repository.ErrDuplicateWorkflowID,
		repository.ErrNotFound,
		context.Canceled,
		context.DeadlineExceeded,
		errors.New("unexpected failure"),
	} {
		for _, lookup := range []bool{false, true} {
			t.Run(fmt.Sprintf("%v/lookup=%t", cause, lookup),
				func(t *testing.T) {
					repo := NewMockStartApplicationRepository(t)
					requestID := uuid.New()

					if lookup {
						repo.EXPECT().
							Create(mock.Anything, mock.Anything).
							Return(repository.ErrDuplicateRequestID).
							Once()

						repo.EXPECT().
							GetByRequestID(mock.Anything, requestID).
							Return(
								domain.Application{},
								fmt.Errorf("lookup: %w", cause),
							).
							Once()
					} else {
						repo.EXPECT().
							Create(mock.Anything, mock.Anything).
							Return(fmt.Errorf("create: %w", cause)).
							Once()
					}

					service := NewStartApplicationService(repo, nil)

					got, err := service.StartApplication(
						context.Background(),
						requestID.String(),
					)

					require.ErrorIs(t, err, cause)
					require.Equal(t, domain.Application{}, got)
				})
		}
	}
}
