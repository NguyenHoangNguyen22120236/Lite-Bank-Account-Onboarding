package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/domain"
)

func (r *ApplicationRepository) GetIdentity(ctx context.Context, applicationID uuid.UUID) (*domain.Identity, error) {
	tx, ok := transactionFromContext(ctx)
	if !ok {
		return nil, transactionRequired()
	}
	var stored domain.Identity
	var birth, expiry time.Time
	err := tx.QueryRow(ctx, `SELECT id_number, full_name, date_of_birth, id_expiry_date,
		phone_number, email, created_at FROM application_identities WHERE application_id=$1`, applicationID).
		Scan(&stored.IDNumber, &stored.FullName, &birth, &expiry, &stored.PhoneNumber, &stored.Email, &stored.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, databaseError(ctx, "read identity", err)
	}
	stored.ApplicationID = applicationID
	stored.DateOfBirth, stored.IDExpiryDate = birth.Format("2006-01-02"), expiry.Format("2006-01-02")
	return &stored, nil
}

func (r *ApplicationRepository) InsertIdentity(ctx context.Context, i domain.Identity) error {
	tx, ok := transactionFromContext(ctx)
	if !ok {
		return transactionRequired()
	}
	// Dates were validated by the service and domain decision. Pass typed dates
	// to pgx, whose binary date codec does not accept submitted strings.
	birth, err := time.Parse("2006-01-02", i.DateOfBirth)
	if err != nil {
		return domain.ErrInvalidIdentity
	}
	expiry, err := time.Parse("2006-01-02", i.IDExpiryDate)
	if err != nil {
		return domain.ErrInvalidIdentity
	}
	_, err = tx.Exec(ctx, `INSERT INTO application_identities
		(application_id,id_number,full_name,date_of_birth,id_expiry_date,phone_number,email,created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, i.ApplicationID, i.IDNumber, i.FullName,
		birth, expiry, i.PhoneNumber, i.Email, i.CreatedAt)
	if err != nil {
		return databaseError(ctx, "insert identity", err)
	}
	return nil
}
