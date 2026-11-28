package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/domain"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/repository"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/service"
)

// ApplicationDB is the pool subset needed for application persistence.
type ApplicationDB interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

// ApplicationRepository persists the application aggregate, including identity and terms,
// within service-owned transactions.
type ApplicationRepository struct {
	db ApplicationDB
}

var (
	_ ApplicationDB                      = (*pgxpool.Pool)(nil)
	_ service.StartApplicationRepository = (*ApplicationRepository)(nil)
	_ service.AcceptTermsRepository      = (*ApplicationRepository)(nil)
	_ service.SubmitIdentityRepository   = (*ApplicationRepository)(nil)
)

// NewApplicationRepository uses a pool opened by platform/postgres.Open. The caller owns its lifetime.
func NewApplicationRepository(db ApplicationDB) *ApplicationRepository {
	return &ApplicationRepository{db: db}
}

const columns = `application_id, request_id, status, temporal_workflow_id,
	created_at, updated_at, completed_at, rejected_at_step`

// Create relies on PostgreSQL uniqueness for atomic duplicate detection.
// It performs no preflight lookup, upsert, or retry.
func (r *ApplicationRepository) Create(ctx context.Context, app domain.Application) error {
	_, err := r.database(ctx).Exec(ctx, `INSERT INTO applications (`+columns+`)
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

func (r *ApplicationRepository) GetByID(ctx context.Context, id uuid.UUID) (domain.Application, error) {
	return readApplication(ctx, "get application by ID", r.database(ctx).QueryRow(ctx,
		`SELECT `+columns+` FROM applications WHERE application_id = $1`, id))
}

func (r *ApplicationRepository) GetByRequestID(ctx context.Context, requestID uuid.UUID) (domain.Application, error) {
	return readApplication(ctx, "get application by request ID", r.database(ctx).QueryRow(ctx,
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

// GetByIDForUpdate holds the application row lock until the caller's transaction ends.
func (r *ApplicationRepository) GetByIDForUpdate(ctx context.Context, id uuid.UUID) (domain.Application, error) {
	tx, ok := transactionFromContext(ctx)
	if !ok {
		return domain.Application{}, transactionRequired()
	}
	return readApplication(ctx, "lock application", tx.QueryRow(ctx,
		`SELECT `+columns+` FROM applications WHERE application_id=$1 FOR UPDATE`, id))
}

func (r *ApplicationRepository) AdvanceAfterTerms(ctx context.Context, a domain.TermsAcceptance) error {
	tx, ok := transactionFromContext(ctx)
	if !ok {
		return transactionRequired()
	}
	result, err := tx.Exec(ctx, `UPDATE applications SET status=$2, updated_at=$3
   WHERE application_id=$1 AND status=$4`, a.ApplicationID,
		string(domain.ApplicationStatusPendingIdentitySubmission), a.AcceptedAt,
		string(domain.ApplicationStatusPendingTermsAcceptance))
	if err != nil {
		return databaseError(ctx, "advance application after terms", err)
	}
	if result.RowsAffected() != 1 {
		return repository.ErrDatabase
	}
	return nil
}

func (r *ApplicationRepository) AdvanceAfterIdentity(ctx context.Context, i domain.Identity) (domain.Application, error) {
	tx, ok := transactionFromContext(ctx)
	if !ok {
		return domain.Application{}, transactionRequired()
	}
	return readApplication(ctx, "advance application after identity", tx.QueryRow(ctx,
		`UPDATE applications SET status=$2, updated_at=$3 WHERE application_id=$1 AND status=$4 RETURNING `+columns,
		i.ApplicationID, string(domain.ApplicationStatusPendingIdentityVerification), i.CreatedAt,
		string(domain.ApplicationStatusPendingIdentitySubmission)))
}

// ResolveWorkflowStart performs only the conditional persistence update.
// Retained for legacy compatibility; PR 09 startup does not call this method.
// The caller owns the transaction, row lock, and decision to advance.
func (r *ApplicationRepository) ResolveWorkflowStart(ctx context.Context, id uuid.UUID, target domain.ApplicationStatus, now time.Time) (domain.Application, error) {
	tx, ok := transactionFromContext(ctx)
	if !ok {
		return domain.Application{}, transactionRequired()
	}
	if target != domain.ApplicationStatusPendingIdentityVerification && target != domain.ApplicationStatusFailedTechnical {
		return domain.Application{}, repository.ErrDatabase
	}
	app, err := readApplication(ctx, "advance workflow startup", tx.QueryRow(ctx,
		`UPDATE applications SET status=$2, updated_at=$3 WHERE application_id=$1 AND status=$4 RETURNING `+columns,
		id, string(target), now, string(domain.ApplicationStatusPendingWorkflowStart)))
	if errors.Is(err, repository.ErrNotFound) {
		// A conditional no-op returns the current persisted application.
		return r.GetByID(ctx, id)
	}
	return app, err
}

func (r *ApplicationRepository) database(ctx context.Context) ApplicationDB {
	if tx, ok := transactionFromContext(ctx); ok {
		return tx
	}
	return r.db
}
