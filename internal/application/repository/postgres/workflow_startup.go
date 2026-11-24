package postgres

import (
	"context"
	"time"

	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/domain"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/repository"
	"github.com/google/uuid"
)

// ResolveWorkflowStart preserves any state already advanced by another writer.
// It follows the parent-row locking protocol used by identity and terms writes.
func (r *Repository) ResolveWorkflowStart(ctx context.Context, id uuid.UUID, target domain.ApplicationStatus, now time.Time) (domain.Application, error) {
	if target != domain.ApplicationStatusPendingIdentityVerification && target != domain.ApplicationStatusFailedTechnical {
		return domain.Application{}, repository.ErrDatabase
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return domain.Application{}, databaseError(ctx, "begin workflow startup transition", err)
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanupCtx)
	}()
	app, err := readApplication(ctx, "lock application for workflow startup", tx.QueryRow(ctx, `SELECT `+columns+` FROM applications WHERE application_id=$1 FOR UPDATE`, id))
	if err != nil {
		return domain.Application{}, err
	}
	if app.Status == domain.ApplicationStatusPendingWorkflowStart {
		app, err = readApplication(ctx, "advance workflow startup", tx.QueryRow(ctx, `UPDATE applications SET status=$2, updated_at=$3 WHERE application_id=$1 AND status=$4 RETURNING `+columns, id, string(target), now, string(domain.ApplicationStatusPendingWorkflowStart)))
		if err != nil {
			return domain.Application{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Application{}, databaseError(ctx, "commit workflow startup transition", err)
	}
	return app, nil
}
