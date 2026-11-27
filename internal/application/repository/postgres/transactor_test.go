package postgres

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/domain"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestTransactorLifecycle(t *testing.T) {
	businessErr := domain.ErrConflictingIdentity
	for _, tc := range []struct {
		name                                      string
		callbackErr, commitErr, rollbackErr, want error
		panics, cancel                            bool
	}{
		{name: "commit"},
		{name: "callback error", callbackErr: businessErr, want: businessErr},
		{name: "commit failure", commitErr: errors.New("synthetic-sensitive-driver-detail"), want: repository.ErrDatabase},
		{name: "canceled commit", commitErr: context.Canceled, want: context.Canceled},
		{name: "deadline commit", commitErr: context.DeadlineExceeded, want: context.DeadlineExceeded},
		{name: "panic", panics: true},
		{name: "cancellation cleanup", callbackErr: context.Canceled, want: context.Canceled, cancel: true},
		{name: "rollback error preserves cause", callbackErr: businessErr, rollbackErr: errors.New("cleanup failed"), want: businessErr},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			db, tx := NewMockTransactorDB(t), NewMockTx(t)
			begin := db.EXPECT().Begin(mock.MatchedBy(func(got context.Context) bool { return got == ctx })).Return(tx, nil).Once()
			callbackCalls := 0
			var commit *mock.Call
			if tc.callbackErr == nil && !tc.panics {
				commit = tx.EXPECT().Commit(ctx).Run(func(context.Context) {
					require.Equal(t, 1, callbackCalls)
				}).Return(tc.commitErr).Once()
			}
			rollback := tx.EXPECT().Rollback(mock.Anything).Run(func(cleanupCtx context.Context) {
				require.Equal(t, 1, callbackCalls)
				require.NoError(t, cleanupCtx.Err())
				deadline, ok := cleanupCtx.Deadline()
				require.True(t, ok, "rollback needs bounded cleanup context")
				require.False(t, deadline.After(time.Now().Add(6*time.Second)))
			}).Return(tc.rollbackErr).Once()
			if commit != nil {
				mock.InOrder(begin, commit, rollback)
			} else {
				mock.InOrder(begin, rollback)
			}
			run := func() error {
				return NewTransactor(db).WithinTx(ctx, func(txCtx context.Context) error {
					callbackCalls++
					deadline, ok := txCtx.Deadline()
					wantDeadline, _ := ctx.Deadline()
					require.True(t, ok)
					require.True(t, deadline.Equal(wantDeadline), "transaction context lost deadline")
					bound, ok := transactionFromContext(txCtx)
					require.True(t, ok)
					require.Same(t, tx, bound)
					if tc.cancel {
						cancel()
					}
					if tc.panics {
						panic("callback panic")
					}
					return tc.callbackErr
				})
			}
			if tc.panics {
				require.PanicsWithValue(t, "callback panic", func() { _ = run() })
			} else {
				var err error
				require.NotPanics(t, func() { err = run() })
				require.ErrorIs(t, err, tc.want)
				if err != nil {
					require.NotContains(t, err.Error(), "synthetic-sensitive-driver-detail")
				}
			}
			require.Equal(t, 1, callbackCalls)
		})
	}
}

func TestTransactorBeginFailures(t *testing.T) {
	for _, cause := range []error{errors.New("synthetic-sensitive-driver-detail"), context.Canceled, context.DeadlineExceeded} {
		t.Run(cause.Error(), func(t *testing.T) {
			ctx := context.Background()
			db := NewMockTransactorDB(t)
			db.EXPECT().Begin(ctx).Return(nil, cause).Once()
			want := cause
			if !errors.Is(cause, context.Canceled) && !errors.Is(cause, context.DeadlineExceeded) {
				want = repository.ErrDatabase
			}
			err := NewTransactor(db).WithinTx(ctx, func(context.Context) error {
				require.FailNow(t, "callback after Begin failure")
				return nil
			})
			require.ErrorIs(t, err, want)
			require.NotContains(t, err.Error(), "synthetic-sensitive-driver-detail")
		})
	}
}

func TestTransactorRepositoriesShareTransaction(t *testing.T) {
	ctx := context.Background()
	app := fixture()
	db, tx := NewMockTransactorDB(t), NewMockTx(t)
	pool := NewMockApplicationDB(t) // No pool operation is allowed inside the transaction.
	apps := NewApplicationRepository(pool)
	db.EXPECT().Begin(ctx).Return(tx, nil).Once()
	txContext := mock.MatchedBy(func(ctx context.Context) bool {
		bound, ok := transactionFromContext(ctx)
		return ok && bound == tx
	})
	appRow, identityRow, termsRows := NewMockRow(t), NewMockRow(t), NewMockRows(t)
	lock := tx.EXPECT().QueryRow(txContext, mock.MatchedBy(func(sql string) bool {
		return strings.Contains(sql, "FROM applications") && strings.Contains(sql, "FOR UPDATE")
	}), []any{app.ApplicationID}).Return(appRow).Once()
	appRow.EXPECT().Scan(mock.MatchedBy(func(dest []any) bool { return len(dest) == 8 })).Run(func(dest ...any) {
		*dest[0].(*uuid.UUID) = app.ApplicationID
		*dest[1].(*uuid.UUID) = app.RequestID
		*dest[2].(*domain.ApplicationStatus) = app.Status
		*dest[3].(*string) = app.TemporalWorkflowID
		*dest[4].(*time.Time) = app.CreatedAt
		*dest[5].(*time.Time) = app.UpdatedAt
		*dest[6].(**time.Time) = app.CompletedAt
		*dest[7].(**domain.RejectedAtStep) = app.RejectedAtStep
	}).Return(nil).Once()
	identity := tx.EXPECT().QueryRow(txContext, mock.MatchedBy(func(sql string) bool {
		return strings.Contains(sql, "FROM application_identities")
	}), []any{app.ApplicationID}).Return(identityRow).Once()
	identityRow.EXPECT().Scan(mock.MatchedBy(func(dest []any) bool { return len(dest) == 7 })).Return(pgx.ErrNoRows).Once()
	terms := tx.EXPECT().Query(txContext, "SELECT term_id FROM terms_acceptances WHERE application_id=$1", []any{app.ApplicationID}).Return(termsRows, nil).Once()
	next := termsRows.EXPECT().Next().Return(false).Once()
	closeRows := termsRows.EXPECT().Close().Once()
	rowsErr := termsRows.EXPECT().Err().Return(nil).Once()
	mock.InOrder(next, closeRows, rowsErr)
	i := domain.Identity{ApplicationID: app.ApplicationID, DateOfBirth: "2000-01-01", IDExpiryDate: "2099-01-01"}
	birth, err := time.Parse("2006-01-02", i.DateOfBirth)
	require.NoError(t, err)
	expiry, err := time.Parse("2006-01-02", i.IDExpiryDate)
	require.NoError(t, err)
	insertIdentity := tx.EXPECT().Exec(txContext, mock.MatchedBy(func(sql string) bool {
		return strings.HasPrefix(sql, "INSERT INTO application_identities")
	}), []any{app.ApplicationID, i.IDNumber, i.FullName, birth, expiry, i.PhoneNumber, i.Email, i.CreatedAt}).
		Return(pgconn.NewCommandTag("INSERT 0 1"), nil).Once()
	acceptance := domain.TermsAcceptance{ApplicationID: app.ApplicationID}
	insertTerms := tx.EXPECT().Exec(txContext, mock.MatchedBy(func(sql string) bool {
		return strings.HasPrefix(sql, "INSERT INTO terms_acceptances")
	}), []any{acceptance.AcceptanceID, app.ApplicationID, acceptance.TermID, acceptance.AcceptedAt}).
		Return(pgconn.NewCommandTag("INSERT 0 1"), nil).Once()
	commit := tx.EXPECT().Commit(ctx).Return(nil).Once()
	rollback := tx.EXPECT().Rollback(mock.Anything).Return(nil).Once()
	mock.InOrder(lock, identity, terms, insertIdentity, insertTerms, commit, rollback)

	err = NewTransactor(db).WithinTx(ctx, func(txCtx context.Context) error {
		got, err := apps.GetByIDForUpdate(txCtx, app.ApplicationID)
		require.NoError(t, err)
		require.Equal(t, app, got)
		storedIdentity, err := apps.GetIdentity(txCtx, app.ApplicationID)
		require.NoError(t, err)
		require.Nil(t, storedIdentity)
		storedTerms, err := apps.GetTermIDs(txCtx, app.ApplicationID)
		require.NoError(t, err)
		require.Empty(t, storedTerms)
		require.NoError(t, apps.InsertIdentity(txCtx, i))
		return apps.InsertTermsAcceptance(txCtx, acceptance)
	})
	require.NoError(t, err)
}

func TestTransactionRequiredOperationsRejectCallerContext(t *testing.T) {
	app := NewApplicationRepository(NewMockApplicationDB(t))
	ctx, id := context.Background(), uuid.New()
	_, lockErr := app.GetByIDForUpdate(ctx, id)
	_, identityErr := app.GetIdentity(ctx, id)
	_, termsErr := app.GetTermIDs(ctx, id)
	insertIdentityErr := app.InsertIdentity(ctx, domain.Identity{ApplicationID: id})
	insertTermsErr := app.InsertTermsAcceptance(ctx, domain.TermsAcceptance{ApplicationID: id})
	advanceTermsErr := app.AdvanceAfterTerms(ctx, domain.TermsAcceptance{ApplicationID: id})
	_, advanceIdentityErr := app.AdvanceAfterIdentity(ctx, domain.Identity{ApplicationID: id})
	_, workflowErr := app.ResolveWorkflowStart(ctx, id, domain.ApplicationStatusPendingIdentityVerification, time.Now())
	for _, err := range []error{lockErr, identityErr, termsErr, insertIdentityErr, insertTermsErr, advanceTermsErr, advanceIdentityErr, workflowErr} {
		require.ErrorIs(t, err, repository.ErrDatabase)
	}
}

func TestTransactorConcurrentContextIsolation(t *testing.T) {
	type callerKey struct{}
	ctx := context.Background()
	db := NewMockTransactorDB(t)
	apps := NewApplicationRepository(NewMockApplicationDB(t))
	txs := []*MockTx{NewMockTx(t), NewMockTx(t)}
	ids := []uuid.UUID{uuid.New(), uuid.New()}
	for index, tx := range txs {
		callerCtx := mock.MatchedBy(func(ctx context.Context) bool { return ctx.Value(callerKey{}) == index })
		begin := db.EXPECT().Begin(callerCtx).Return(tx, nil).Once()
		txContext := mock.MatchedBy(func(ctx context.Context) bool {
			bound, ok := transactionFromContext(ctx)
			return ok && bound == tx && ctx.Value(callerKey{}) == index
		})
		row := NewMockRow(t)
		query := tx.EXPECT().QueryRow(txContext, mock.MatchedBy(func(sql string) bool {
			return strings.Contains(sql, "FROM application_identities")
		}), []any{ids[index]}).Return(row).Once()
		row.EXPECT().Scan(mock.MatchedBy(func(dest []any) bool { return len(dest) == 7 })).Return(pgx.ErrNoRows).Once()
		commit := tx.EXPECT().Commit(callerCtx).Return(nil).Once()
		rollback := tx.EXPECT().Rollback(mock.Anything).Return(nil).Once()
		mock.InOrder(begin, query, commit, rollback)
	}
	transactor := NewTransactor(db)
	ready := make(chan struct{}, 2)
	release := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for index := range txs {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			errs <- transactor.WithinTx(context.WithValue(ctx, callerKey{}, index), func(txCtx context.Context) error {
				ready <- struct{}{}
				<-release
				_, err := apps.GetIdentity(txCtx, ids[index])
				return err
			})
		}(index)
	}
	<-ready
	<-ready
	close(release)
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
}
