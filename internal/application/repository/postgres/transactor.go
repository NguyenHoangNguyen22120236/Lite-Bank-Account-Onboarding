package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/repository"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/service"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TransactorDB is the database capability consumed by Transactor.
type TransactorDB interface {
	Begin(context.Context) (pgx.Tx, error)
}

type Transactor struct{ db TransactorDB }

var (
	_ TransactorDB       = (*pgxpool.Pool)(nil)
	_ service.Transactor = (*Transactor)(nil)
)

func NewTransactor(db TransactorDB) *Transactor { return &Transactor{db: db} }

type transactionContextKey struct{}

// WithinTx never retries fn. The context binding is private to this package;
// concurrent requests keep separate transactions without mutable repository state.
func (t *Transactor) WithinTx(ctx context.Context, fn func(context.Context) error) error {
	if _, ok := transactionFromContext(ctx); ok {
		return fmt.Errorf("nested transaction: %w", repository.ErrDatabase)
	}
	tx, err := t.db.Begin(ctx)
	if err != nil {
		return databaseError(ctx, "begin transaction", err)
	}
	defer func() {
		// Release resources even when the request was canceled or fn panicked.
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanupCtx)
	}()
	if err := fn(context.WithValue(ctx, transactionContextKey{}, tx)); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return databaseError(ctx, "commit transaction", err)
	}
	return nil
}

func transactionFromContext(ctx context.Context) (pgx.Tx, bool) {
	tx, ok := ctx.Value(transactionContextKey{}).(pgx.Tx)
	return tx, ok && tx != nil
}

func transactionRequired() error {
	return fmt.Errorf("transaction required: %w", repository.ErrDatabase)
}
