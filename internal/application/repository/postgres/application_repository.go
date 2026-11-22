package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/domain"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/repository"
)

// DB is the pool subset needed for single-statement application persistence.
type DB interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type Repository struct {
	db DB
}

var (
	_ DB                    = (*pgxpool.Pool)(nil)
	_ repository.Repository = (*Repository)(nil)
)

// New uses a pool opened by platform/postgres.Open. The caller owns its lifetime.
func New(db DB) *Repository {
	return &Repository{db: db}
}

const columns = `application_id, request_id, status, temporal_workflow_id,
	created_at, updated_at, completed_at, rejected_at_step`

// Create relies on PostgreSQL uniqueness for atomic duplicate detection.
// It performs no preflight lookup, upsert, or retry.
func (r *Repository) Create(ctx context.Context, app domain.Application) error {
	_, err := r.db.Exec(ctx, `INSERT INTO applications (`+columns+`)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		app.ApplicationID, app.RequestID, string(app.Status), app.TemporalWorkflowID,
		app.CreatedAt, app.UpdatedAt, app.CompletedAt, app.RejectedAtStep)
	if err == nil {
		return nil
	}
	var pgerr *pgconn.PgError
	if errors.As(err, &pgerr) && pgerr.Code == "23505" {
		switch pgerr.ConstraintName {
		case "applications_pkey":
			return fmt.Errorf("create application: %w", repository.ErrDuplicateApplicationID)
		case "applications_request_id_key":
			return fmt.Errorf("create application: %w", repository.ErrDuplicateRequestID)
		case "applications_temporal_workflow_id_key":
			return fmt.Errorf("create application: %w", repository.ErrDuplicateWorkflowID)
		}
	}
	return databaseError(ctx, "create application", err)
}

func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (domain.Application, error) {
	return readApplication(ctx, "get application by ID", r.db.QueryRow(ctx,
		`SELECT `+columns+` FROM applications WHERE application_id = $1`, id))
}

func (r *Repository) GetByRequestID(ctx context.Context, requestID uuid.UUID) (domain.Application, error) {
	return readApplication(ctx, "get application by request ID", r.db.QueryRow(ctx,
		`SELECT `+columns+` FROM applications WHERE request_id = $1`, requestID))
}

func readApplication(ctx context.Context, operation string, row pgx.Row) (domain.Application, error) {
	var app domain.Application
	err := row.Scan(&app.ApplicationID, &app.RequestID, &app.Status, &app.TemporalWorkflowID,
		&app.CreatedAt, &app.UpdatedAt, &app.CompletedAt, &app.RejectedAtStep)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Application{}, fmt.Errorf("%s: %w", operation, repository.ErrNotFound)
	}
	if err != nil {
		return domain.Application{}, databaseError(ctx, operation, err)
	}
	if !app.Status.Valid() || (app.RejectedAtStep != nil && !app.RejectedAtStep.Valid()) {
		return domain.Application{}, fmt.Errorf("%s: %w", operation, repository.ErrDatabase)
	}
	return app, nil
}

// Discard driver details, which can contain credentials or customer data.
// Only safe sentinel errors are exposed to callers.
func databaseError(ctx context.Context, operation string, err error) error {
	switch {
	case errors.Is(err, context.Canceled):
		return fmt.Errorf("%s: %w", operation, context.Canceled)
	case errors.Is(err, context.DeadlineExceeded):
		return fmt.Errorf("%s: %w", operation, context.DeadlineExceeded)
	case ctx.Err() != nil:
		return fmt.Errorf("%s: %w", operation, ctx.Err())
	default:
		return fmt.Errorf("%s: %w", operation, repository.ErrDatabase)
	}
}
