package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"strings"

	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/domain"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/repository"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/service"
)

type statusFailurePool struct{ *pgxpool.Pool }

func (p statusFailurePool) Begin(ctx context.Context) (pgx.Tx, error) {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	return statusFailureTx{Tx: tx}, nil
}

type statusFailureTx struct{ pgx.Tx }

func (tx statusFailureTx) Exec(ctx context.Context, query string, args ...any) (pgconn.CommandTag, error) {
	if strings.HasPrefix(query, "UPDATE applications") {
		return pgconn.CommandTag{}, errors.New("synthetic update failure")
	}
	return tx.Tx.Exec(ctx, query, args...)
}
func (tx statusFailureTx) QueryRow(ctx context.Context, query string, args ...any) pgx.Row {
	if strings.HasPrefix(query, "UPDATE applications") {
		return errorRow{errors.New("synthetic update failure")}
	}
	return tx.Tx.QueryRow(ctx, query, args...)
}

type unexpectedWorkflowStarter struct{ t *testing.T }

func (s unexpectedWorkflowStarter) EnsureStarted(context.Context, string, string) (service.WorkflowStartOutcome, error) {
	s.t.Fatal("workflow started after failed transaction")
	return 0, nil
}

func TestIntegrationServiceTransactionsRollbackAssociatedRecords(t *testing.T) {
	pool, ctx := integrationPool(t)
	db := statusFailurePool{pool}
	apps := NewApplicationRepository(db)
	transactor := NewTransactor(db)
	t.Run("terms insert rolled back after status failure", func(t *testing.T) {
		app := termsFixture(t, ctx, pool, domain.ApplicationStatusPendingTermsAcceptance)
		err := service.NewAcceptTermsService(transactor, apps).AcceptTerms(ctx, app.ApplicationID.String(), "v1")
		if !errors.Is(err, repository.ErrDatabase) {
			t.Fatalf("got %v, want database error", err)
		}
		if terms := readTerms(t, ctx, pool, app.ApplicationID); len(terms) != 0 {
			t.Fatal("terms survived failed status update")
		}
		got, err := NewApplicationRepository(pool).GetByID(ctx, app.ApplicationID)
		if err != nil {
			t.Fatal(err)
		}
		assertApplication(t, got, app)
	})
	t.Run("identity insert rolled back after status failure", func(t *testing.T) {
		app := identityFixture(t, ctx, pool, domain.ApplicationStatusPendingIdentitySubmission)
		input := identityPayload()
		identity := &domain.Identity{IDNumber: input.IdNumber, FullName: input.FullName, DateOfBirth: input.DateOfBirth,
			IDExpiryDate: input.IdExpiryDate, PhoneNumber: input.PhoneNumber, Email: input.Email}
		got, err := service.NewSubmitIdentityService(transactor, apps, unexpectedWorkflowStarter{t}).SubmitIdentity(ctx, app.ApplicationID.String(), identity)
		if !errors.Is(err, repository.ErrDatabase) || got != (domain.Application{}) {
			t.Fatalf("got %+v, %v", got, err)
		}
		if readIdentity(t, ctx, pool, app.ApplicationID) != nil {
			t.Fatal("identity survived failed status update")
		}
		stored, err := NewApplicationRepository(pool).GetByID(ctx, app.ApplicationID)
		if err != nil {
			t.Fatal(err)
		}
		assertApplication(t, stored, app)
	})
}
