package service

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/domain"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/repository"
	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func serviceIdentity() *domain.Identity {
	email := ""
	return &domain.Identity{IDNumber: "079123456789", FullName: " Name ", DateOfBirth: "2000-01-01", IDExpiryDate: "2032-01-01", PhoneNumber: "0901234567", Email: &email}
}

func TestSubmitIdentityValidation(t *testing.T) {
	for _, tc := range []struct {
		name, id string
		identity *domain.Identity
		want     error
	}{
		{"empty ID", "", serviceIdentity(), ErrInvalidApplicationID},
		{"malformed ID", "APP-001", serviceIdentity(), ErrInvalidApplicationID},
		{"nil UUID", uuid.Nil.String(), serviceIdentity(), ErrInvalidApplicationID},
		{"missing identity", uuid.NewString(), nil, domain.ErrInvalidIdentity},
		{"invalid identity", uuid.NewString(), &domain.Identity{}, domain.ErrInvalidIdentity},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tx, repo := NewMockTransactor(t), NewMockSubmitIdentityRepository(t)
			got, err := NewSubmitIdentityService(tx, repo).SubmitIdentity(context.Background(), tc.id, tc.identity)
			require.ErrorIs(t, err, tc.want)
			require.Empty(t, got)
		})
	}
}

func TestSubmitIdentityPersistence(t *testing.T) {
	for _, cause := range []error{nil, repository.ErrNotFound, repository.ErrDatabase, domain.ErrConflictingIdentity,
		domain.ErrInvalidIdentityState, domain.ErrInvalidIdentity, context.Canceled, context.DeadlineExceeded} {
		t.Run(fmt.Sprint(cause), func(t *testing.T) {
			ctx := context.Background()
			txCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			id := uuid.New()
			app := domain.Application{ApplicationID: id, Status: domain.ApplicationStatusCompleted}
			input := serviceIdentity()
			input.IDExpiryDate = "2001-01-01" // Expired identity remains a valid exact replay.
			original := *input
			stored := *input
			stored.ApplicationID = id
			tx, repo := NewMockTransactor(t), NewMockSubmitIdentityRepository(t)
			tx.EXPECT().WithinTx(ctx, mock.Anything).RunAndReturn(func(_ context.Context, fn func(context.Context) error) error {
				err := fn(txCtx)
				require.ErrorIs(t, err, cause)
				return err
			}).Once()
			lock := repo.EXPECT().GetByIDForUpdate(txCtx, id).Return(app, nil).Once()
			var readErr error
			if cause != nil {
				readErr = fmt.Errorf("submit: %w", cause)
			}
			read := repo.EXPECT().GetIdentity(txCtx, id).Return(&stored, readErr).Once()
			mock.InOrder(lock, read)
			got, err := NewSubmitIdentityService(tx, repo).SubmitIdentity(ctx, id.String(), input)
			require.ErrorIs(t, err, cause)
			if cause == nil {
				require.Equal(t, app, got)
			} else {
				require.Empty(t, got)
			}
			require.Equal(t, original, *input)
		})
	}
}

func TestSubmitIdentityOrchestration(t *testing.T) {
	ctx := context.Background()
	identityCtx, cancelIdentity := context.WithCancel(ctx)
	defer cancelIdentity()
	id := uuid.New()
	initial := domain.Application{ApplicationID: id, TemporalWorkflowID: id.String(), Status: domain.ApplicationStatusPendingIdentitySubmission}
	pending := initial
	pending.Status = domain.ApplicationStatusPendingIdentityVerification
	input := serviceIdentity()
	original := *input
	tx, repo := NewMockTransactor(t), NewMockSubmitIdentityRepository(t)
	firstTx := tx.EXPECT().WithinTx(ctx, mock.Anything).RunAndReturn(func(_ context.Context, fn func(context.Context) error) error {
		err := fn(identityCtx)
		require.NoError(t, err)
		return err
	}).Once()
	lock := repo.EXPECT().GetByIDForUpdate(identityCtx, id).Return(initial, nil).Once()
	read := repo.EXPECT().GetIdentity(identityCtx, id).Return(nil, nil).Once()
	var inserted domain.Identity
	insert := repo.EXPECT().InsertIdentity(identityCtx, mock.Anything).
		Run(func(_ context.Context, i domain.Identity) { inserted = i }).Return(nil).Once()
	advance := repo.EXPECT().AdvanceAfterIdentity(identityCtx, mock.MatchedBy(func(i domain.Identity) bool {
		return reflect.DeepEqual(i, inserted)
	})).Return(pending, nil).Once()
	mock.InOrder(firstTx, lock, read, insert, advance)

	before := time.Now().UTC()
	got, err := NewSubmitIdentityService(tx, repo).SubmitIdentity(ctx, id.String(), input)
	require.NoError(t, err)
	require.Equal(t, pending, got)
	require.Equal(t, id, inserted.ApplicationID)
	require.False(t, inserted.CreatedAt.Before(before))
	require.False(t, inserted.CreatedAt.After(time.Now()))
	require.Equal(t, time.UTC, inserted.CreatedAt.Location())
	copyIdentity := inserted
	copyIdentity.ApplicationID, copyIdentity.CreatedAt = original.ApplicationID, original.CreatedAt
	require.Equal(t, original, copyIdentity)
	require.Equal(t, original, *input)
	require.NotSame(t, input.Email, inserted.Email)
}

func TestSubmitIdentityDomainDecisions(t *testing.T) {
	for _, tc := range []struct {
		name              string
		status            domain.ApplicationStatus
		existing, changed bool
		expiry            string
		want              error
	}{
		{"exact replay", domain.ApplicationStatusCompleted, true, false, "2001-01-01", nil},
		{"conflicting identity", domain.ApplicationStatusCompleted, true, true, "2032-01-01", domain.ErrConflictingIdentity},
		{"identity before submission", domain.ApplicationStatusPendingIdentitySubmission, true, false, "2032-01-01", domain.ErrInvalidIdentityState},
		{"terms not accepted", domain.ApplicationStatusPendingTermsAcceptance, false, false, "2032-01-01", domain.ErrInvalidIdentityState},
		{"missing progressed identity", domain.ApplicationStatusCompleted, false, false, "2032-01-01", domain.ErrInvalidIdentityState},
		{"expired first submission", domain.ApplicationStatusPendingIdentitySubmission, false, false, "2001-01-01", domain.ErrInvalidIdentity},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			txCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			id := uuid.New()
			app := domain.Application{ApplicationID: id, Status: tc.status}
			input := serviceIdentity()
			input.IDExpiryDate = tc.expiry
			var stored *domain.Identity
			if tc.existing {
				copy := *input
				copy.ApplicationID = id
				stored = &copy
			}
			if tc.changed {
				input.FullName += "changed"
			}
			tx, repo := NewMockTransactor(t), NewMockSubmitIdentityRepository(t)
			tx.EXPECT().WithinTx(ctx, mock.Anything).RunAndReturn(func(_ context.Context, fn func(context.Context) error) error {
				err := fn(txCtx)
				require.ErrorIs(t, err, tc.want)
				return err
			}).Once()
			lock := repo.EXPECT().GetByIDForUpdate(txCtx, id).Return(app, nil).Once()
			read := repo.EXPECT().GetIdentity(txCtx, id).Return(stored, nil).Once()
			mock.InOrder(lock, read)
			// Replays and rejections must not write or start a workflow.
			got, err := NewSubmitIdentityService(tx, repo).SubmitIdentity(ctx, id.String(), input)
			require.ErrorIs(t, err, tc.want)
			if err == nil {
				require.Equal(t, app, got)
			} else {
				require.Empty(t, got)
			}
		})
	}
}

func TestSubmitIdentityTransactionFailures(t *testing.T) {
	for stageIndex, stage := range []string{"begin", "lock", "read identity", "insert identity", "advance identity", "commit"} {
		t.Run(stage, func(t *testing.T) {
			ctx := context.Background()
			txCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			id := uuid.New()
			initial := domain.Application{ApplicationID: id, TemporalWorkflowID: id.String(), Status: domain.ApplicationStatusPendingIdentitySubmission}
			pending := initial
			pending.Status = domain.ApplicationStatusPendingIdentityVerification
			tx, repo := NewMockTransactor(t), NewMockSubmitIdentityRepository(t)
			if stage == "begin" {
				tx.EXPECT().WithinTx(ctx, mock.Anything).Return(repository.ErrDatabase).Once()
			} else {
				tx.EXPECT().WithinTx(ctx, mock.Anything).RunAndReturn(func(_ context.Context, fn func(context.Context) error) error {
					err := fn(txCtx)
					if stage == "commit" {
						require.NoError(t, err)
						return repository.ErrDatabase
					}
					require.ErrorIs(t, err, repository.ErrDatabase)
					return err
				}).Once()
				failureAt := func(name string) error {
					if stage == name {
						return repository.ErrDatabase
					}
					return nil
				}
				calls := []*mock.Call{repo.EXPECT().GetByIDForUpdate(txCtx, id).Return(initial, failureAt("lock")).Once()}
				if stageIndex > 1 {
					calls = append(calls, repo.EXPECT().GetIdentity(txCtx, id).Return(nil, failureAt("read identity")).Once())
				}
				if stageIndex > 2 {
					calls = append(calls, repo.EXPECT().InsertIdentity(txCtx, mock.Anything).Return(failureAt("insert identity")).Once())
				}
				if stageIndex > 3 {
					calls = append(calls, repo.EXPECT().AdvanceAfterIdentity(txCtx, mock.Anything).Return(pending, failureAt("advance identity")).Once())
				}
				mock.InOrder(calls...)
			}
			got, err := NewSubmitIdentityService(tx, repo).SubmitIdentity(ctx, id.String(), serviceIdentity())
			require.ErrorIs(t, err, repository.ErrDatabase)
			require.Empty(t, got)
		})
	}
}
