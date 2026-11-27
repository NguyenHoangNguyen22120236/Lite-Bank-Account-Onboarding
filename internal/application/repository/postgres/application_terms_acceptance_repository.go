package postgres

import (
	"context"

	"github.com/google/uuid"

	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/domain"
)

func (r *ApplicationRepository) GetTermIDs(ctx context.Context, applicationID uuid.UUID) ([]string, error) {
	tx, ok := transactionFromContext(ctx)
	if !ok {
		return nil, transactionRequired()
	}
	rows, err := tx.Query(ctx, `SELECT term_id FROM terms_acceptances WHERE application_id=$1`, applicationID)
	if err != nil {
		return nil, databaseError(ctx, "read accepted terms", err)
	}
	var terms []string
	for rows.Next() {
		var term string
		if err := rows.Scan(&term); err != nil {
			rows.Close()
			return nil, databaseError(ctx, "read accepted terms", err)
		}
		terms = append(terms, term)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, databaseError(ctx, "read accepted terms", err)
	}
	return terms, nil
}

func (r *ApplicationRepository) InsertTermsAcceptance(ctx context.Context, a domain.TermsAcceptance) error {
	tx, ok := transactionFromContext(ctx)
	if !ok {
		return transactionRequired()
	}
	_, err := tx.Exec(ctx, `INSERT INTO terms_acceptances
		(acceptance_id, application_id, term_id, accepted_at) VALUES ($1,$2,$3,$4)`,
		a.AcceptanceID, a.ApplicationID, a.TermID, a.AcceptedAt)
	if err != nil {
		return databaseError(ctx, "insert terms acceptance", err)
	}
	return nil
}
