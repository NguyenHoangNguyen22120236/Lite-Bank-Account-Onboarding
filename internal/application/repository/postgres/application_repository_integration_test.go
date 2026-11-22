package postgres

import (
	"context"
	"errors"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/domain"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/repository"
	platformpostgres "github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/platform/postgres"
)

// Set TEST_DATABASE_URL only for a disposable, already migrated database.
// Tests do not load .env, run migrations, or truncate tables.
func integrationPool(t *testing.T) (*pgxpool.Pool, context.Context) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	pool, err := platformpostgres.Open(ctx, dsn)
	if err != nil {
		t.Fatal("cannot open integration database (connection details omitted)")
	}
	t.Cleanup(pool.Close)
	return pool, ctx
}

func cleanupApplication(t *testing.T, pool *pgxpool.Pool, id uuid.UUID) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := pool.Exec(ctx, "DELETE FROM applications WHERE application_id = $1", id); err != nil {
			t.Error("could not clean up test application (details omitted)")
		}
	})
}

func fixture() domain.Application {
	id := uuid.New()
	now := time.Date(2026, 11, 21, 3, 4, 5, 123456000, time.UTC)
	return domain.Application{
		ApplicationID: id, RequestID: uuid.New(),
		Status: domain.ApplicationStatusPendingTermsAcceptance,
		TemporalWorkflowID: id.String(), CreatedAt: now, UpdatedAt: now.Add(time.Minute),
	}
}

func assertApplication(t *testing.T, got, want domain.Application) {
	t.Helper()
	// PostgreSQL preserves instants at microsecond precision, not Go locations.
	got.CreatedAt = got.CreatedAt.UTC().Truncate(time.Microsecond)
	got.UpdatedAt = got.UpdatedAt.UTC().Truncate(time.Microsecond)
	want.CreatedAt = want.CreatedAt.UTC().Truncate(time.Microsecond)
	want.UpdatedAt = want.UpdatedAt.UTC().Truncate(time.Microsecond)
	if got.CompletedAt != nil {
		completed := got.CompletedAt.UTC().Truncate(time.Microsecond)
		got.CompletedAt = &completed
	}
	if want.CompletedAt != nil {
		completed := want.CompletedAt.UTC().Truncate(time.Microsecond)
		want.CompletedAt = &completed
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("application mismatch: got %+v, want %+v", got, want)
	}
}

func TestIntegrationRoundTrip(t *testing.T) {
	pool, ctx := integrationPool(t)
	r := New(pool)
	for _, tc := range []struct {
		name   string
		status domain.ApplicationStatus
		step   domain.RejectedAtStep
	}{
		{"pending_null_fields", domain.ApplicationStatusPendingTermsAcceptance, ""},
		{"completed", domain.ApplicationStatusCompleted, ""},
		{"identity_rejection", domain.ApplicationStatusRejected, domain.RejectedAtStepIdentityVerification},
		{"human_rejection", domain.ApplicationStatusRejected, domain.RejectedAtStepHumanReview},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := fixture()
			app.Status = tc.status
			if tc.status == domain.ApplicationStatusCompleted || tc.status == domain.ApplicationStatusRejected {
				completed := app.UpdatedAt.Add(time.Minute)
				app.CompletedAt = &completed
			}
			if tc.step != "" {
				step := tc.step
				app.RejectedAtStep = &step
			}
			cleanupApplication(t, pool, app.ApplicationID)
			if err := r.Create(ctx, app); err != nil {
				t.Fatal(err)
			}
			got, err := r.GetByID(ctx, app.ApplicationID)
			if err != nil {
				t.Fatal(err)
			}
			assertApplication(t, got, app)
			got, err = r.GetByRequestID(ctx, app.RequestID)
			if err != nil {
				t.Fatal(err)
			}
			assertApplication(t, got, app)
		})
	}
}

func TestIntegrationDuplicateRequestID(t *testing.T) {
	pool, ctx := integrationPool(t)
	r := New(pool)
	original, candidate := fixture(), fixture()
	candidate.RequestID = original.RequestID
	candidate.Status = domain.ApplicationStatusPendingIdentitySubmission
	cleanupApplication(t, pool, original.ApplicationID)
	cleanupApplication(t, pool, candidate.ApplicationID)

	if err := r.Create(ctx, original); err != nil {
		t.Fatal(err)
	}
	if err := r.Create(ctx, candidate); !errors.Is(err, repository.ErrDuplicateRequestID) {
		t.Fatalf("expected duplicate request error, got %v", err)
	}
	got, err := r.GetByRequestID(ctx, original.RequestID)
	if err != nil {
		t.Fatal(err)
	}
	assertApplication(t, got, original)
	if _, err := r.GetByID(ctx, candidate.ApplicationID); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("duplicate candidate must not be stored, got %v", err)
	}
}

func TestIntegrationConcurrentDuplicateRequests(t *testing.T) {
	pool, ctx := integrationPool(t)
	r := New(pool)
	const count = 8
	requestID := uuid.New()
	apps := make([]domain.Application, count)
	errs := make([]error, count)
	var wg sync.WaitGroup
	start := make(chan struct{})

	for i := range apps {
		apps[i] = fixture()
		apps[i].RequestID = requestID
		cleanupApplication(t, pool, apps[i].ApplicationID)
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			errs[i] = r.Create(ctx, apps[i])
		}(i)
	}
	close(start)
	wg.Wait()

	winner, successes := -1, 0
	for i, err := range errs {
		if err == nil {
			winner = i
			successes++
		} else if !errors.Is(err, repository.ErrDuplicateRequestID) {
			t.Fatalf("insert %d: %v", i, err)
		}
	}
	if successes != 1 {
		t.Fatalf("got %d successful inserts, want one", successes)
	}
	var rows int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM applications WHERE request_id = $1", requestID).Scan(&rows); err != nil {
		t.Fatal("count query failed (details omitted)")
	}
	if rows != 1 {
		t.Fatalf("got %d rows, want one", rows)
	}
	got, err := r.GetByRequestID(ctx, requestID)
	if err != nil {
		t.Fatal(err)
	}
	assertApplication(t, got, apps[winner])
}
