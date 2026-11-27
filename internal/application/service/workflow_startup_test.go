package service

import (
	"context"
	"testing"
	"time"

	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/domain"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/repository"
	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// Only test data is shared; each test declares its own mock expectations.
func workflowApplication() domain.Application {
	id := uuid.New()
	return domain.Application{
		ApplicationID: id, TemporalWorkflowID: id.String(),
		Status:    domain.ApplicationStatusPendingWorkflowStart,
		UpdatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
}

func TestWorkflowStartupOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name           string
		outcome        WorkflowStartOutcome
		cause, wantErr error
		wantStatus     domain.ApplicationStatus
	}{
		{"accepted or running", WorkflowStartEstablished, nil, nil, domain.ApplicationStatusPendingIdentityVerification},
		{"closed unsuccessfully", WorkflowStartClosedUnsuccessfully, nil, ErrWorkflowClosed, domain.ApplicationStatusFailedTechnical},
		{"completed with pending application", WorkflowStartCompleted, nil, ErrWorkflowStateInconsistent, domain.ApplicationStatusPendingWorkflowStart},
		{"uncertain timeout", 0, context.DeadlineExceeded, context.DeadlineExceeded, domain.ApplicationStatusPendingWorkflowStart},
		{"unavailable", 0, ErrWorkflowUnavailable, ErrWorkflowUnavailable, domain.ApplicationStatusPendingWorkflowStart},
		{"canceled", 0, context.Canceled, context.Canceled, domain.ApplicationStatusPendingWorkflowStart},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			txCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			app := workflowApplication()
			tx, repo, starter := NewMockTransactor(t), NewMockSubmitIdentityRepository(t), NewMockWorkflowStarter(t)
			start := starter.EXPECT().EnsureStarted(ctx, app.ApplicationID.String(), app.TemporalWorkflowID).Return(tc.outcome, tc.cause).Once()
			persisted := app
			if tc.cause == nil && tc.outcome == WorkflowStartCompleted {
				read := repo.EXPECT().GetByID(ctx, app.ApplicationID).Return(app, nil).Once()
				mock.InOrder(start, read)
			} else if tc.cause == nil {
				transaction := tx.EXPECT().WithinTx(ctx, mock.Anything).RunAndReturn(func(_ context.Context, fn func(context.Context) error) error {
					return fn(txCtx)
				}).Once()
				lock := repo.EXPECT().GetByIDForUpdate(txCtx, app.ApplicationID).Return(app, nil).Once()
				persisted.Status = tc.wantStatus
				persisted.UpdatedAt = app.UpdatedAt.Add(time.Hour)
				before := time.Now().UTC()
				resolve := repo.EXPECT().ResolveWorkflowStart(txCtx, app.ApplicationID, tc.wantStatus,
					mock.MatchedBy(func(now time.Time) bool {
						return now.Location() == time.UTC && !now.Before(before) && !now.After(time.Now())
					})).
					Return(persisted, nil).Once()
				mock.InOrder(start, transaction, lock, resolve)
			}
			got, err := NewSubmitIdentityService(tx, repo, starter).startWorkflow(ctx, app)
			require.ErrorIs(t, err, tc.wantErr)
			if err == nil {
				require.Equal(t, persisted, got)
			} else {
				require.Empty(t, got)
			}
		})
	}
}

func TestWorkflowStartupPersistenceFailure(t *testing.T) {
	ctx := context.Background()
	txCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	app := workflowApplication()
	tx, repo, starter := NewMockTransactor(t), NewMockSubmitIdentityRepository(t), NewMockWorkflowStarter(t)
	tx.EXPECT().WithinTx(ctx, mock.Anything).RunAndReturn(func(_ context.Context, fn func(context.Context) error) error {
		return fn(txCtx)
	}).Once()
	repo.EXPECT().GetByIDForUpdate(txCtx, app.ApplicationID).Return(domain.Application{}, repository.ErrDatabase).Once()
	got, err := NewSubmitIdentityService(tx, repo, starter).SubmitIdentity(ctx, app.ApplicationID.String(), serviceIdentity())
	require.ErrorIs(t, err, repository.ErrDatabase)
	require.Empty(t, got)
}

func TestWorkflowStartupReplaySkipsTemporal(t *testing.T) {
	for _, status := range []domain.ApplicationStatus{
		domain.ApplicationStatusPendingIdentityVerification, domain.ApplicationStatusPendingSanctionsScreening,
		domain.ApplicationStatusPendingHumanReview, domain.ApplicationStatusApproved,
		domain.ApplicationStatusPendingAccountCreation, domain.ApplicationStatusCompleted,
		domain.ApplicationStatusRejected, domain.ApplicationStatusFailedTechnical,
	} {
		t.Run(string(status), func(t *testing.T) {
			ctx := context.Background()
			txCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			app := workflowApplication()
			app.Status = status
			identity := serviceIdentity()
			identity.ApplicationID = app.ApplicationID
			tx, repo, starter := NewMockTransactor(t), NewMockSubmitIdentityRepository(t), NewMockWorkflowStarter(t)
			tx.EXPECT().WithinTx(ctx, mock.Anything).RunAndReturn(func(_ context.Context, fn func(context.Context) error) error {
				return fn(txCtx)
			}).Once()
			lock := repo.EXPECT().GetByIDForUpdate(txCtx, app.ApplicationID).Return(app, nil).Once()
			read := repo.EXPECT().GetIdentity(txCtx, app.ApplicationID).Return(identity, nil).Once()
			mock.InOrder(lock, read)
			got, err := NewSubmitIdentityService(tx, repo, starter).SubmitIdentity(ctx, app.ApplicationID.String(), serviceIdentity())
			require.NoError(t, err)
			require.Equal(t, app, got)
		})
	}
}

func TestWorkflowStartupRetryAfterUncertainOrDatabaseFailure(t *testing.T) {
	for _, databaseFailure := range []bool{false, true} {
		t.Run(map[bool]string{false: "lost acknowledgement", true: "post-start database failure"}[databaseFailure], func(t *testing.T) {
			ctx := context.Background()
			txCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			app := workflowApplication()
			advanced := app
			advanced.Status = domain.ApplicationStatusPendingIdentityVerification
			tx, repo, starter := NewMockTransactor(t), NewMockSubmitIdentityRepository(t), NewMockWorkflowStarter(t)
			identity := serviceIdentity()
			identity.ApplicationID = app.ApplicationID
			firstIdentityTx := tx.EXPECT().WithinTx(ctx, mock.Anything).RunAndReturn(func(_ context.Context, fn func(context.Context) error) error {
				return fn(txCtx)
			}).Once()
			firstIdentityLock := repo.EXPECT().GetByIDForUpdate(txCtx, app.ApplicationID).Return(app, nil).Once()
			firstIdentityRead := repo.EXPECT().GetIdentity(txCtx, app.ApplicationID).Return(identity, nil).Once()
			var firstOutcome WorkflowStartOutcome
			firstErr := error(context.DeadlineExceeded)
			if databaseFailure {
				firstOutcome, firstErr = WorkflowStartEstablished, nil
			}
			firstStart := starter.EXPECT().EnsureStarted(ctx, app.ApplicationID.String(), app.TemporalWorkflowID).Return(firstOutcome, firstErr).Once()
			mock.InOrder(firstIdentityTx, firstIdentityLock, firstIdentityRead, firstStart)
			if databaseFailure {
				transaction := tx.EXPECT().WithinTx(ctx, mock.Anything).RunAndReturn(func(_ context.Context, fn func(context.Context) error) error {
					return fn(txCtx)
				}).Once()
				lock := repo.EXPECT().GetByIDForUpdate(txCtx, app.ApplicationID).Return(app, nil).Once()
				resolve := repo.EXPECT().ResolveWorkflowStart(txCtx, app.ApplicationID, advanced.Status, mock.Anything).
					Return(domain.Application{}, repository.ErrDatabase).Once()
				mock.InOrder(firstStart, transaction, lock, resolve)
			}
			s := NewSubmitIdentityService(tx, repo, starter)
			got, err := s.SubmitIdentity(ctx, app.ApplicationID.String(), serviceIdentity())
			if databaseFailure {
				require.ErrorIs(t, err, repository.ErrDatabase)
			} else {
				require.ErrorIs(t, err, context.DeadlineExceeded)
			}
			require.Empty(t, got)
			starter.AssertNumberOfCalls(t, "EnsureStarted", 1)

			// Register the second attempt only after verifying the first made no retry.
			secondIdentityTx := tx.EXPECT().WithinTx(ctx, mock.Anything).RunAndReturn(func(_ context.Context, fn func(context.Context) error) error {
				return fn(txCtx)
			}).Once()
			secondIdentityLock := repo.EXPECT().GetByIDForUpdate(txCtx, app.ApplicationID).Return(app, nil).Once()
			secondIdentityRead := repo.EXPECT().GetIdentity(txCtx, app.ApplicationID).Return(identity, nil).Once()
			secondStart := starter.EXPECT().EnsureStarted(ctx, app.ApplicationID.String(), app.TemporalWorkflowID).Return(WorkflowStartEstablished, nil).Once()
			transaction := tx.EXPECT().WithinTx(ctx, mock.Anything).RunAndReturn(func(_ context.Context, fn func(context.Context) error) error {
				return fn(txCtx)
			}).Once()
			lock := repo.EXPECT().GetByIDForUpdate(txCtx, app.ApplicationID).Return(app, nil).Once()
			resolve := repo.EXPECT().ResolveWorkflowStart(txCtx, app.ApplicationID, advanced.Status, mock.Anything).Return(advanced, nil).Once()
			mock.InOrder(secondIdentityTx, secondIdentityLock, secondIdentityRead, secondStart, transaction, lock, resolve)
			got, err = s.SubmitIdentity(ctx, app.ApplicationID.String(), serviceIdentity())
			require.NoError(t, err)
			require.Equal(t, advanced, got)
			starter.AssertNumberOfCalls(t, "EnsureStarted", 2)
		})
	}
}

func TestWorkflowStartupConcurrentProgressWins(t *testing.T) {
	for _, outcome := range []WorkflowStartOutcome{WorkflowStartEstablished, WorkflowStartClosedUnsuccessfully} {
		t.Run(map[WorkflowStartOutcome]string{WorkflowStartEstablished: "established", WorkflowStartClosedUnsuccessfully: "closed"}[outcome], func(t *testing.T) {
			ctx := context.Background()
			txCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			app := workflowApplication()
			progressed := app
			progressed.Status = domain.ApplicationStatusCompleted
			tx, repo, starter := NewMockTransactor(t), NewMockSubmitIdentityRepository(t), NewMockWorkflowStarter(t)
			start := starter.EXPECT().EnsureStarted(ctx, app.ApplicationID.String(), app.TemporalWorkflowID).Return(outcome, nil).Once()
			transaction := tx.EXPECT().WithinTx(ctx, mock.Anything).RunAndReturn(func(_ context.Context, fn func(context.Context) error) error {
				return fn(txCtx)
			}).Once()
			lock := repo.EXPECT().GetByIDForUpdate(txCtx, app.ApplicationID).Return(progressed, nil).Once()
			mock.InOrder(start, transaction, lock)
			got, err := NewSubmitIdentityService(tx, repo, starter).startWorkflow(ctx, app)
			require.NoError(t, err)
			require.Equal(t, progressed, got)
		})
	}
}

func TestWorkflowStartupCompletedReconcilesPersistedState(t *testing.T) {
	for _, status := range []domain.ApplicationStatus{domain.ApplicationStatusCompleted, domain.ApplicationStatusRejected, domain.ApplicationStatusPendingIdentityVerification} {
		t.Run(string(status), func(t *testing.T) {
			ctx := context.Background()
			app := workflowApplication()
			current := app
			current.Status = status
			tx, repo, starter := NewMockTransactor(t), NewMockSubmitIdentityRepository(t), NewMockWorkflowStarter(t)
			start := starter.EXPECT().EnsureStarted(ctx, app.ApplicationID.String(), app.TemporalWorkflowID).Return(WorkflowStartCompleted, nil).Once()
			read := repo.EXPECT().GetByID(ctx, app.ApplicationID).Return(current, nil).Once()
			mock.InOrder(start, read)
			got, err := NewSubmitIdentityService(tx, repo, starter).startWorkflow(ctx, app)
			if status == domain.ApplicationStatusPendingIdentityVerification {
				require.ErrorIs(t, err, ErrWorkflowStateInconsistent)
				require.Empty(t, got)
			} else {
				require.NoError(t, err)
				require.Equal(t, current, got)
			}
		})
	}
}

func TestWorkflowStartupRejectsInconsistentStoredID(t *testing.T) {
	for _, workflowID := range []string{"", uuid.NewString()} {
		t.Run(workflowID, func(t *testing.T) {
			app := workflowApplication()
			app.TemporalWorkflowID = workflowID
			tx, repo, starter := NewMockTransactor(t), NewMockSubmitIdentityRepository(t), NewMockWorkflowStarter(t)
			got, err := NewSubmitIdentityService(tx, repo, starter).startWorkflow(context.Background(), app)
			require.ErrorIs(t, err, ErrWorkflowStateInconsistent)
			require.Empty(t, got)
		})
	}
}

func TestWorkflowStartupReconciliationReadFailure(t *testing.T) {
	ctx := context.Background()
	app := workflowApplication()
	tx, repo, starter := NewMockTransactor(t), NewMockSubmitIdentityRepository(t), NewMockWorkflowStarter(t)
	start := starter.EXPECT().EnsureStarted(ctx, app.ApplicationID.String(), app.TemporalWorkflowID).Return(WorkflowStartCompleted, nil).Once()
	read := repo.EXPECT().GetByID(ctx, app.ApplicationID).Return(domain.Application{}, repository.ErrDatabase).Once()
	mock.InOrder(start, read)
	got, err := NewSubmitIdentityService(tx, repo, starter).startWorkflow(ctx, app)
	require.ErrorIs(t, err, repository.ErrDatabase)
	require.Empty(t, got)
}
