package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/domain"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestGetTermIDsDatabaseFailures(t *testing.T) {
	for _, stage := range []string{"query", "scan", "iteration"} {
		t.Run(stage, func(t *testing.T) {
			const sensitive = "synthetic-private-driver-detail"
			cause := errors.New(sensitive)
			id := uuid.New()
			tx := NewMockTx(t)
			ctx := context.WithValue(context.Background(), transactionContextKey{}, tx)
			repo := NewApplicationRepository(NewMockApplicationDB(t))
			query := tx.EXPECT().Query(ctx, "SELECT term_id FROM terms_acceptances WHERE application_id=$1", []any{id})
			if stage == "query" {
				query.Return(nil, cause).Once()
			} else {
				rows := NewMockRows(t)
				query.Return(rows, nil).Once()
				if stage == "scan" {
					next := rows.EXPECT().Next().Return(true).Once()
					scan := rows.EXPECT().Scan(mock.MatchedBy(func(dest []any) bool {
						return len(dest) == 1
					})).Return(cause).Once()
					closeRows := rows.EXPECT().Close().Once()
					mock.InOrder(next, scan, closeRows)
				} else {
					next := rows.EXPECT().Next().Return(false).Once()
					closeRows := rows.EXPECT().Close().Once()
					rowsErr := rows.EXPECT().Err().Return(cause).Once()
					mock.InOrder(next, closeRows, rowsErr)
				}
			}

			terms, err := repo.GetTermIDs(ctx, id)
			require.ErrorIs(t, err, repository.ErrDatabase)
			require.Nil(t, terms)
			require.NotContains(t, err.Error(), sensitive)
			require.NotErrorIs(t, err, cause)
		})
	}
}

func TestInsertTermsAcceptanceDatabaseFailure(t *testing.T) {
	const sensitive = "synthetic-private-driver-detail"
	cause := errors.New(sensitive)
	tx := NewMockTx(t)
	ctx := context.WithValue(context.Background(), transactionContextKey{}, tx)
	acceptance := domain.TermsAcceptance{
		AcceptanceID: uuid.New(), ApplicationID: uuid.New(),
		TermID: " V1 ", AcceptedAt: time.Now().UTC(),
	}
	tx.EXPECT().Exec(ctx, `INSERT INTO terms_acceptances
		(acceptance_id, application_id, term_id, accepted_at) VALUES ($1,$2,$3,$4)`,
		[]any{acceptance.AcceptanceID, acceptance.ApplicationID, acceptance.TermID, acceptance.AcceptedAt}).
		Return(pgconn.CommandTag{}, cause).Once()

	err := NewApplicationRepository(NewMockApplicationDB(t)).InsertTermsAcceptance(ctx, acceptance)
	require.ErrorIs(t, err, repository.ErrDatabase)
	require.NotContains(t, err.Error(), sensitive)
	require.NotErrorIs(t, err, cause)
}
