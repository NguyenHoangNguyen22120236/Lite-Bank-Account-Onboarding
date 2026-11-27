package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/domain"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
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
			require.ErrorIs(t, err, tc.want)
			require.NotContains(t, err.Error(), sensitive, "error exposes driver details")
			var pgerr *pgconn.PgError
			require.False(t, errors.As(err, &pgerr), "raw PostgreSQL error escaped")
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
			require.ErrorIs(t, err, tc.want)
			require.Equal(t, (domain.Application{}), got)
		})
	}
}

func TestWorkflowStartupTransitionDatabaseFailure(t *testing.T) {
	app := fixture()
	_, err := resolveWorkflowStartForTest(context.Background(), failingDB{err: errors.New("synthetic driver failure")}, NewApplicationRepository(failingDB{err: errors.New("synthetic driver failure")}), app.ApplicationID, domain.ApplicationStatusPendingIdentityVerification, time.Now().UTC())
	require.ErrorIs(t, err, repository.ErrDatabase)
}
