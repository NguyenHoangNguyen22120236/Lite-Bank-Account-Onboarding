package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/domain"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/repository"
	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestAcceptTermsValidation(t *testing.T) {
	valid := uuid.NewString()
	for _, tc := range []struct {
		name, id, term string
		want           error
	}{
		{"empty ID", "", "v1", ErrInvalidApplicationID},
		{"malformed ID", "invalid", "v1", ErrInvalidApplicationID},
		{"nil ID", uuid.Nil.String(), "v1", ErrInvalidApplicationID},
		{"empty term", valid, "", ErrInvalidTermID},
		{"blank term", valid, " \t\n", ErrInvalidTermID},
		{"NUL term", valid, "v1\x00", ErrInvalidTermID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tx, repo := NewMockTransactor(t), NewMockAcceptTermsRepository(t)
			err := NewAcceptTermsService(tx, repo).AcceptTerms(context.Background(), tc.id, tc.term)
			require.ErrorIs(t, err, tc.want)
		})
	}
}

func TestAcceptTermsPersistence(t *testing.T) {
	for _, cause := range []error{nil, repository.ErrNotFound, domain.ErrConflictingTerms,
		domain.ErrInvalidTermsState, repository.ErrDatabase, context.Canceled, context.DeadlineExceeded} {
		t.Run(fmt.Sprint(cause), func(t *testing.T) {
			ctx := context.Background()
			txCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			id := uuid.New()
			app := domain.Application{ApplicationID: id, Status: domain.ApplicationStatusPendingTermsAcceptance}
			tx, repo := NewMockTransactor(t), NewMockAcceptTermsRepository(t)
			tx.EXPECT().WithinTx(ctx, mock.Anything).RunAndReturn(func(_ context.Context, fn func(context.Context) error) error {
				err := fn(txCtx)
				require.ErrorIs(t, err, cause)
				return err
			}).Once()
			lock := repo.EXPECT().GetByIDForUpdate(txCtx, id).Return(app, nil).Once()
			read := repo.EXPECT().GetTermIDs(txCtx, id).Return(nil, nil).Once()
			before := time.Now().UTC()
			var inserted domain.TermsAcceptance
			var insertErr error
			if cause != nil {
				insertErr = fmt.Errorf("accept: %w", cause)
			}
			insert := repo.EXPECT().InsertTermsAcceptance(txCtx, mock.Anything).
				Run(func(_ context.Context, a domain.TermsAcceptance) { inserted = a }).
				Return(insertErr).Once()
			mock.InOrder(lock, read, insert)
			if cause == nil {
				advance := repo.EXPECT().AdvanceAfterTerms(txCtx, mock.MatchedBy(func(a domain.TermsAcceptance) bool {
					return a == inserted
				})).Return(nil).Once()
				mock.InOrder(insert, advance)
			}
			err := NewAcceptTermsService(tx, repo).AcceptTerms(ctx, id.String(), " V1 ")
			require.ErrorIs(t, err, cause)
			if cause == nil {
				require.Equal(t, id, inserted.ApplicationID)
				require.NotEqual(t, uuid.Nil, inserted.AcceptanceID)
				require.Equal(t, " V1 ", inserted.TermID)
				require.False(t, inserted.AcceptedAt.Before(before))
				require.False(t, inserted.AcceptedAt.After(time.Now()))
				require.Equal(t, time.UTC, inserted.AcceptedAt.Location())
			}
		})
	}
}

func TestAcceptTermsOrchestration(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status domain.ApplicationStatus
		terms  []string
		want   error
	}{
		{"first acceptance", domain.ApplicationStatusPendingTermsAcceptance, nil, nil},
		{"same term replay", domain.ApplicationStatusCompleted, []string{" V1 "}, nil},
		{"different term", domain.ApplicationStatusPendingIdentitySubmission, []string{"other"}, domain.ErrConflictingTerms},
		{"inconsistent initial acceptance", domain.ApplicationStatusPendingTermsAcceptance, []string{" V1 "}, domain.ErrInvalidTermsState},
		{"missing acceptance after progression", domain.ApplicationStatusPendingIdentitySubmission, nil, domain.ErrInvalidTermsState},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			txCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			id := uuid.New()
			tx, repo := NewMockTransactor(t), NewMockAcceptTermsRepository(t)
			tx.EXPECT().WithinTx(ctx, mock.Anything).RunAndReturn(func(_ context.Context, fn func(context.Context) error) error {
				err := fn(txCtx)
				require.ErrorIs(t, err, tc.want)
				return err
			}).Once()
			lock := repo.EXPECT().GetByIDForUpdate(txCtx, id).
				Return(domain.Application{ApplicationID: id, Status: tc.status}, nil).Once()
			read := repo.EXPECT().GetTermIDs(txCtx, id).Return(tc.terms, nil).Once()
			mock.InOrder(lock, read)
			if tc.name == "first acceptance" {
				var inserted domain.TermsAcceptance
				insert := repo.EXPECT().InsertTermsAcceptance(txCtx, mock.Anything).
					Run(func(_ context.Context, a domain.TermsAcceptance) { inserted = a }).Return(nil).Once()
				advance := repo.EXPECT().AdvanceAfterTerms(txCtx, mock.MatchedBy(func(a domain.TermsAcceptance) bool {
					return a == inserted
				})).Return(nil).Once()
				mock.InOrder(read, insert, advance)
			}
			// No write expectations for replay or rejection: any write fails the test.
			err := NewAcceptTermsService(tx, repo).AcceptTerms(ctx, id.String(), " V1 ")
			require.ErrorIs(t, err, tc.want)
		})
	}
}

func TestAcceptTermsTransactionFailures(t *testing.T) {
	for stageIndex, stage := range []string{"begin", "lock", "read terms", "insert terms", "advance terms", "commit"} {
		t.Run(stage, func(t *testing.T) {
			ctx := context.Background()
			txCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			id := uuid.New()
			tx, repo := NewMockTransactor(t), NewMockAcceptTermsRepository(t)
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
				calls := []*mock.Call{
					repo.EXPECT().GetByIDForUpdate(txCtx, id).
						Return(domain.Application{ApplicationID: id, Status: domain.ApplicationStatusPendingTermsAcceptance}, failureAt("lock")).Once(),
				}
				if stageIndex > 1 {
					calls = append(calls, repo.EXPECT().GetTermIDs(txCtx, id).Return(nil, failureAt("read terms")).Once())
				}
				if stageIndex > 2 {
					calls = append(calls, repo.EXPECT().InsertTermsAcceptance(txCtx, mock.Anything).Return(failureAt("insert terms")).Once())
				}
				if stageIndex > 3 {
					calls = append(calls, repo.EXPECT().AdvanceAfterTerms(txCtx, mock.Anything).Return(failureAt("advance terms")).Once())
				}
				mock.InOrder(calls...)
			}
			err := NewAcceptTermsService(tx, repo).AcceptTerms(ctx, id.String(), "v1")
			require.ErrorIs(t, err, repository.ErrDatabase)
		})
	}
}
