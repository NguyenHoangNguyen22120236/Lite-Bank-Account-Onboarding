package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/domain"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/repository"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/service"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// Forward normal operations to PostgreSQL to verify real transaction rollback.
func statusFailureDB(t *testing.T, pool *pgxpool.Pool) TransactorDB {
	t.Helper()
	db := NewMockTransactorDB(t)
	db.EXPECT().Begin(mock.Anything).RunAndReturn(func(ctx context.Context) (pgx.Tx, error) {
		realTx, err := pool.Begin(ctx)
		if err != nil {
			return nil, err
		}
		t.Cleanup(func() {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = realTx.Rollback(cleanupCtx)
		})
		tx := NewMockTx(t)
		tx.EXPECT().Exec(mock.Anything, mock.Anything, mock.Anything).RunAndReturn(func(ctx context.Context, query string, args ...any) (pgconn.CommandTag, error) {
			if strings.HasPrefix(query, "UPDATE applications") {
				return pgconn.CommandTag{}, errors.New("synthetic update failure")
			}
			return realTx.Exec(ctx, query, args...)
		})
		tx.EXPECT().Query(mock.Anything, mock.Anything, mock.Anything).RunAndReturn(realTx.Query).Maybe()
		tx.EXPECT().QueryRow(mock.Anything, mock.Anything, mock.Anything).RunAndReturn(func(ctx context.Context, query string, args ...any) pgx.Row {
			if strings.HasPrefix(query, "UPDATE applications") {
				row := NewMockRow(t)
				row.EXPECT().Scan(mock.Anything).Return(errors.New("synthetic update failure")).Once()
				return row
			}
			return realTx.QueryRow(ctx, query, args...)
		})
		tx.EXPECT().Rollback(mock.Anything).RunAndReturn(realTx.Rollback).Once()
		return tx, nil
	}).Once()
	return db
}

func TestIntegrationServiceTransactionsRollbackAssociatedRecords(t *testing.T) {
	pool, ctx := integrationPool(t)
	apps := NewApplicationRepository(pool)
	t.Run("terms insert rolled back after status failure", func(t *testing.T) {
		app := termsFixture(t, ctx, pool, domain.ApplicationStatusPendingTermsAcceptance)
		transactor := NewTransactor(statusFailureDB(t, pool))
		err := service.NewAcceptTermsService(transactor, apps).AcceptTerms(ctx, app.ApplicationID.String(), "v1")
		require.ErrorIs(t, err, repository.ErrDatabase)
		require.Empty(t, readTerms(t, ctx, pool, app.ApplicationID), "terms survived failed status update")
		got, err := NewApplicationRepository(pool).GetByID(ctx, app.ApplicationID)
		require.NoError(t, err)
		assertApplication(t, got, app)
	})
	t.Run("identity insert rolled back after status failure", func(t *testing.T) {
		app := identityFixture(t, ctx, pool, domain.ApplicationStatusPendingIdentitySubmission)
		transactor := NewTransactor(statusFailureDB(t, pool))
		input := identityPayload()
		identity := &domain.Identity{IDNumber: input.IdNumber, FullName: input.FullName, DateOfBirth: input.DateOfBirth,
			IDExpiryDate: input.IdExpiryDate, PhoneNumber: input.PhoneNumber, Email: input.Email}
		got, err := service.NewSubmitIdentityService(transactor, apps).SubmitIdentity(ctx, app.ApplicationID.String(), identity)
		require.ErrorIs(t, err, repository.ErrDatabase)
		require.Equal(t, (domain.Application{}), got)
		require.Nil(t, readIdentity(t, ctx, pool, app.ApplicationID), "identity survived failed status update")
		stored, err := NewApplicationRepository(pool).GetByID(ctx, app.ApplicationID)
		require.NoError(t, err)
		assertApplication(t, stored, app)
	})
}
