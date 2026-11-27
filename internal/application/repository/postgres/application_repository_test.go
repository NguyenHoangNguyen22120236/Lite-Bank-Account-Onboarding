package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/domain"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/repository"
)

// This stub returns errors only; successful scanning is tested against PostgreSQL.
type integrationDB interface {
	ApplicationDB
	TransactorDB
}

type failingDB struct{ err error }

func (db failingDB) Query(context.Context, string, ...any) (pgx.Rows, error) { return nil, db.err }

func (db failingDB) Begin(context.Context) (pgx.Tx, error) {
	return nil, db.err
}

func (db failingDB) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, db.err
}

func (db failingDB) QueryRow(context.Context, string, ...any) pgx.Row {
	return errorRow{db.err}
}

type errorRow struct{ err error }

func (row errorRow) Scan(...any) error { return row.err }

func TestCreateErrorClassification(t *testing.T) {
	const sensitive = "synthetic-sensitive-detail"
	for _, tc := range []struct {
		name        string
		cause, want error
	}{
		{"application_duplicate", &pgconn.PgError{Code: "23505", ConstraintName: "applications_pkey", Detail: sensitive}, repository.ErrDuplicateApplicationID},
		{"request_duplicate", &pgconn.PgError{Code: "23505", ConstraintName: "applications_request_id_key", Detail: sensitive}, repository.ErrDuplicateRequestID},
		{"workflow_duplicate", &pgconn.PgError{Code: "23505", ConstraintName: "applications_temporal_workflow_id_key", Detail: sensitive}, repository.ErrDuplicateWorkflowID},
		{"unknown_unique", &pgconn.PgError{Code: "23505", ConstraintName: "unknown", Message: sensitive}, repository.ErrDatabase},
		{"other_database_error", &pgconn.PgError{Code: "23502", Message: sensitive}, repository.ErrDatabase},
		{"driver_error", errors.New(sensitive), repository.ErrDatabase},
		{"cancelled", context.Canceled, context.Canceled},
		{"deadline", context.DeadlineExceeded, context.DeadlineExceeded},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := NewApplicationRepository(failingDB{tc.cause}).Create(context.Background(), fixture())
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
			if strings.Contains(err.Error(), sensitive) {
				t.Fatal("error exposes driver details")
			}
			var pgerr *pgconn.PgError
			if errors.As(err, &pgerr) {
				t.Fatal("raw PostgreSQL error escaped")
			}
		})
	}
}

func TestLookupErrorClassification(t *testing.T) {
	for _, tc := range []struct {
		name        string
		cause, want error
	}{
		{"missing", pgx.ErrNoRows, repository.ErrNotFound},
		{"scan_failure", errors.New("scan failed"), repository.ErrDatabase},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NewApplicationRepository(failingDB{tc.cause}).GetByID(context.Background(), uuid.New())
			if !errors.Is(err, tc.want) || got != (domain.Application{}) {
				t.Fatalf("expected zero application and %v, got %+v, %v", tc.want, got, err)
			}
		})
	}
}

func TestWorkflowStartupTransitionDatabaseFailure(t *testing.T) {
	app := fixture()
	_, err := resolveWorkflowStartForTest(context.Background(), failingDB{err: errors.New("synthetic driver failure")}, NewApplicationRepository(failingDB{err: errors.New("synthetic driver failure")}), app.ApplicationID, domain.ApplicationStatusPendingIdentityVerification, time.Now().UTC())
	if !errors.Is(err, repository.ErrDatabase) {
		t.Fatalf("database error classification lost: %v", err)
	}
}
