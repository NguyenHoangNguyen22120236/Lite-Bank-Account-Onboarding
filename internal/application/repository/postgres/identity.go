package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/domain"
)

// SubmitIdentity serializes submission decisions on the parent application row.
// Identity and status commit together; replay never updates either record.
func (r *Repository) SubmitIdentity(ctx context.Context, i domain.Identity) (domain.Application, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return domain.Application{}, databaseError(ctx, "begin identity submission", err)
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanupCtx)
	}()
	app, err := readApplication(ctx, "lock application", tx.QueryRow(ctx,
		`SELECT `+columns+` FROM applications WHERE application_id=$1 FOR UPDATE`, i.ApplicationID))
	if err != nil {
		return domain.Application{}, err
	}
	var stored domain.Identity
	var birth, expiry time.Time
	err = tx.QueryRow(ctx, `SELECT id_number, full_name, date_of_birth, id_expiry_date,
		phone_number, email, created_at FROM application_identities WHERE application_id=$1`, i.ApplicationID).
		Scan(&stored.IDNumber, &stored.FullName, &birth, &expiry, &stored.PhoneNumber, &stored.Email, &stored.CreatedAt)
	var existing *domain.Identity
	if err == nil {
		stored.ApplicationID = i.ApplicationID
		stored.DateOfBirth, stored.IDExpiryDate = birth.Format("2006-01-02"), expiry.Format("2006-01-02")
		existing = &stored
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return domain.Application{}, databaseError(ctx, "read identity", err)
	}
	replay, err := domain.CheckIdentitySubmission(app.Status, existing, i, i.CreatedAt)
	if err != nil {
		return domain.Application{}, err
	}
	if !replay {
		// Dates were validated by the service and domain decision. Pass typed dates
		// to pgx, whose binary date codec does not accept submitted strings.
		birth, err = time.Parse("2006-01-02", i.DateOfBirth)
		if err != nil {
			return domain.Application{}, domain.ErrInvalidIdentity
		}
		expiry, err = time.Parse("2006-01-02", i.IDExpiryDate)
		if err != nil {
			return domain.Application{}, domain.ErrInvalidIdentity
		}
		_, err = tx.Exec(ctx, `INSERT INTO application_identities
			(application_id,id_number,full_name,date_of_birth,id_expiry_date,phone_number,email,created_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, i.ApplicationID, i.IDNumber, i.FullName,
			birth, expiry, i.PhoneNumber, i.Email, i.CreatedAt)
		if err != nil {
			return domain.Application{}, databaseError(ctx, "insert identity", err)
		}
		app, err = readApplication(ctx, "advance application after identity", tx.QueryRow(ctx,
			`UPDATE applications SET status=$2, updated_at=$3 WHERE application_id=$1 AND status=$4 RETURNING `+columns,
			i.ApplicationID, string(domain.ApplicationStatusPendingWorkflowStart), i.CreatedAt,
			string(domain.ApplicationStatusPendingIdentitySubmission)))
		if err != nil {
			return domain.Application{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Application{}, databaseError(ctx, "commit identity submission", err)
	}
	return app, nil
}
