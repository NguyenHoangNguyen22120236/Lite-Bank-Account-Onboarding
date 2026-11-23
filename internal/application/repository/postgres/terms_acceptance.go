package postgres

import (
	"context"
	"time"

	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/domain"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/repository"
)

// AcceptTerms serializes decisions on the parent row, including requests for
// different terms which the per-term unique constraint cannot serialize alone.
func (r *Repository) AcceptTerms(ctx context.Context, a domain.TermsAcceptance) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return databaseError(ctx, "begin terms acceptance", err)
	}
	defer func() {
		// The request may have been canceled; still release the transaction.
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanupCtx)
	}()

	app, err := readApplication(ctx, "lock application", tx.QueryRow(ctx,
		`SELECT `+columns+` FROM applications WHERE application_id=$1 FOR UPDATE`, a.ApplicationID))
	if err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT term_id FROM terms_acceptances WHERE application_id=$1`, a.ApplicationID)
	if err != nil {
		return databaseError(ctx, "read accepted terms", err)
	}
	var terms []string
	for rows.Next() {
		var term string
		if err := rows.Scan(&term); err != nil {
			rows.Close()
			return databaseError(ctx, "read accepted terms", err)
		}
		terms = append(terms, term)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return databaseError(ctx, "read accepted terms", err)
	}
	replay, err := domain.CheckTermsAcceptance(app.Status, terms, a.TermID)
	if err != nil {
		return err
	}
	if !replay {
		_, err = tx.Exec(ctx, `INSERT INTO terms_acceptances
			(acceptance_id, application_id, term_id, accepted_at) VALUES ($1,$2,$3,$4)`,
			a.AcceptanceID, a.ApplicationID, a.TermID, a.AcceptedAt)
		if err != nil {
			return databaseError(ctx, "insert terms acceptance", err)
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
	}
	if err := tx.Commit(ctx); err != nil {
		return databaseError(ctx, "commit terms acceptance", err)
	}
	return nil
}
