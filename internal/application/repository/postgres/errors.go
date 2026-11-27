package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/repository"
)

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
