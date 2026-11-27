package postgres

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/domain"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/repository"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/service"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type beginFunc func(context.Context) (pgx.Tx, error)

func (f beginFunc) Begin(ctx context.Context) (pgx.Tx, error) { return f(ctx) }

type lifecycleTx struct {
	pgx.Tx
	app                domain.Application
	events             []string
	commitErr          error
	rollbackErr        error
	rollbackContext    context.Context
	rollbackContextErr error
	queries, execs     int
}

func (tx *lifecycleTx) Commit(context.Context) error {
	tx.events = append(tx.events, "commit")
	return tx.commitErr
}
func (tx *lifecycleTx) Rollback(ctx context.Context) error {
	tx.events = append(tx.events, "rollback")
	tx.rollbackContext = ctx
	tx.rollbackContextErr = ctx.Err()
	return tx.rollbackErr
}
func (tx *lifecycleTx) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	tx.execs++
	return pgconn.NewCommandTag("UPDATE 1"), nil
}
func (tx *lifecycleTx) QueryRow(_ context.Context, query string, args ...any) pgx.Row {
	tx.queries++
	if strings.Contains(query, "application_identities") {
		return errorRow{pgx.ErrNoRows}
	}
	return applicationTestRow{tx.app}
}
func (tx *lifecycleTx) Query(context.Context, string, ...any) (pgx.Rows, error) {
	tx.queries++
	return emptyTermRows{}, nil
}

type emptyTermRows struct{ pgx.Rows }

func (emptyTermRows) Next() bool { return false }
func (emptyTermRows) Close()     {}
func (emptyTermRows) Err() error { return nil }

type applicationTestRow struct{ app domain.Application }

func (r applicationTestRow) Scan(dest ...any) error {
	values := []any{r.app.ApplicationID, r.app.RequestID, r.app.Status, r.app.TemporalWorkflowID,
		r.app.CreatedAt, r.app.UpdatedAt, r.app.CompletedAt, r.app.RejectedAtStep}
	for i, value := range values {
		reflect.ValueOf(dest[i]).Elem().Set(reflect.ValueOf(value))
	}
	return nil
}

var _ service.Transactor = (*Transactor)(nil)

func TestTransactorLifecycle(t *testing.T) {
	businessErr := domain.ErrConflictingIdentity
	for _, tc := range []struct {
		name                                      string
		callbackErr, commitErr, rollbackErr, want error
		panics, cancel                            bool
		events                                    []string
	}{
		{name: "commit", events: []string{"callback", "commit", "rollback"}},
		{name: "callback error", callbackErr: businessErr, want: businessErr, events: []string{"callback", "rollback"}},
		{name: "commit failure", commitErr: errors.New("synthetic-sensitive-driver-detail"), want: repository.ErrDatabase, events: []string{"callback", "commit", "rollback"}},
		{name: "canceled commit", commitErr: context.Canceled, want: context.Canceled, events: []string{"callback", "commit", "rollback"}},
		{name: "deadline commit", commitErr: context.DeadlineExceeded, want: context.DeadlineExceeded, events: []string{"callback", "commit", "rollback"}},
		{name: "panic", panics: true, events: []string{"callback", "rollback"}},
		{name: "cancellation cleanup", callbackErr: context.Canceled, want: context.Canceled, cancel: true, events: []string{"callback", "rollback"}},
		{name: "rollback error preserves cause", callbackErr: businessErr, rollbackErr: errors.New("cleanup failed"), want: businessErr, events: []string{"callback", "rollback"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			tx := &lifecycleTx{commitErr: tc.commitErr, rollbackErr: tc.rollbackErr}
			begins := 0
			transactor := NewTransactor(beginFunc(func(got context.Context) (pgx.Tx, error) {
				begins++
				if got != ctx {
					t.Fatal("Begin did not receive caller context")
				}
				return tx, nil
			}))
			var err error
			var recovered any
			func() {
				defer func() { recovered = recover() }()
				err = transactor.WithinTx(ctx, func(txCtx context.Context) error {
					tx.events = append(tx.events, "callback")
					deadline, ok := txCtx.Deadline()
					wantDeadline, _ := ctx.Deadline()
					if !ok || !deadline.Equal(wantDeadline) {
						t.Fatal("transaction context lost deadline")
					}
					if tc.cancel {
						cancel()
					}
					if tc.panics {
						panic("callback panic")
					}
					return tc.callbackErr
				})
			}()
			if tc.panics {
				if recovered != "callback panic" {
					t.Fatalf("panic lost: %v", recovered)
				}
			} else if recovered != nil || !errors.Is(err, tc.want) {
				t.Fatalf("got error=%v panic=%v, want %v", err, recovered, tc.want)
			}
			if err != nil && strings.Contains(err.Error(), "synthetic-sensitive-driver-detail") {
				t.Fatal("driver details leaked")
			}
			if begins != 1 || !reflect.DeepEqual(tx.events, tc.events) {
				t.Fatalf("begins=%d events=%v", begins, tx.events)
			}
			cleanupDeadline, ok := tx.rollbackContext.Deadline()
			if !ok || cleanupDeadline.After(time.Now().Add(6*time.Second)) {
				t.Fatal("rollback needs bounded cleanup context")
			}
			if tx.rollbackContextErr != nil {
				t.Fatalf("rollback used canceled context: %v", tx.rollbackContextErr)
			}
		})
	}
}

func TestTransactorBeginFailures(t *testing.T) {
	for _, cause := range []error{errors.New("synthetic-sensitive-driver-detail"), context.Canceled, context.DeadlineExceeded} {
		t.Run(cause.Error(), func(t *testing.T) {
			want := cause
			if !errors.Is(cause, context.Canceled) && !errors.Is(cause, context.DeadlineExceeded) {
				want = repository.ErrDatabase
			}
			err := NewTransactor(beginFunc(func(context.Context) (pgx.Tx, error) { return nil, cause })).WithinTx(context.Background(), func(context.Context) error {
				t.Fatal("callback after Begin failure")
				return nil
			})
			if !errors.Is(err, want) || strings.Contains(err.Error(), "synthetic-sensitive-driver-detail") {
				t.Fatalf("got %v, want %v", err, want)
			}
		})
	}
}

func TestTransactorRepositoriesShareTransaction(t *testing.T) {
	tx := &lifecycleTx{app: fixture()}
	forbiddenPool := failingDB{errors.New("repository escaped transaction")}
	apps := NewApplicationRepository(forbiddenPool)
	err := NewTransactor(beginFunc(func(context.Context) (pgx.Tx, error) { return tx, nil })).WithinTx(context.Background(), func(ctx context.Context) error {
		got, err := apps.GetByIDForUpdate(ctx, tx.app.ApplicationID)
		if err != nil || got.ApplicationID != tx.app.ApplicationID {
			t.Fatalf("lock: %+v %v", got, err)
		}
		if _, err := apps.GetIdentity(ctx, tx.app.ApplicationID); err != nil {
			return err
		}
		if _, err := apps.GetTermIDs(ctx, tx.app.ApplicationID); err != nil {
			return err
		}
		i := domain.Identity{ApplicationID: tx.app.ApplicationID, DateOfBirth: "2000-01-01", IDExpiryDate: "2099-01-01"}
		if err := apps.InsertIdentity(ctx, i); err != nil {
			return err
		}
		if err := apps.InsertTermsAcceptance(ctx, domain.TermsAcceptance{ApplicationID: tx.app.ApplicationID}); err != nil {
			return err
		}
		return nil
	})
	if err != nil || tx.queries != 3 || tx.execs != 2 {
		t.Fatalf("err=%v queries=%d execs=%d", err, tx.queries, tx.execs)
	}
}

type forbiddenDatabase struct{}

func (forbiddenDatabase) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	panic("transaction-required operation used the pool")
}

func (forbiddenDatabase) QueryRow(context.Context, string, ...any) pgx.Row {
	panic("transaction-required operation used the pool")
}

func (forbiddenDatabase) Query(context.Context, string, ...any) (pgx.Rows, error) {
	panic("transaction-required operation used the pool")
}

func TestTransactionRequiredOperationsRejectCallerContext(t *testing.T) {
	db := forbiddenDatabase{}
	app := NewApplicationRepository(db)
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
		if !errors.Is(err, repository.ErrDatabase) {
			t.Fatalf("missing transaction accepted: %v", err)
		}
	}
}

func TestTransactorConcurrentContextIsolation(t *testing.T) {
	type callerKey struct{}
	txs := []*lifecycleTx{{app: fixture()}, {app: fixture()}}
	transactor := NewTransactor(beginFunc(func(ctx context.Context) (pgx.Tx, error) {
		return txs[ctx.Value(callerKey{}).(int)], nil
	}))
	app := NewApplicationRepository(failingDB{errors.New("escaped transaction")})
	ready := make(chan struct{}, 2)
	release := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for index := range txs {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			errs <- transactor.WithinTx(context.WithValue(context.Background(), callerKey{}, index), func(ctx context.Context) error {
				ready <- struct{}{}
				<-release
				got, err := app.GetByIDForUpdate(ctx, txs[index].app.ApplicationID)
				if err == nil && got.ApplicationID != txs[index].app.ApplicationID {
					return errors.New("crossed transaction contexts")
				}
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
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, tx := range txs {
		if tx.queries != 1 || !reflect.DeepEqual(tx.events, []string{"commit", "rollback"}) {
			t.Fatalf("transaction reused: %+v", tx)
		}
	}
}
