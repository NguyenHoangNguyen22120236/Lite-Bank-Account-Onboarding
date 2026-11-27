package postgres

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	onboardingv1 "github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/gen/onboarding/v1"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/gen/onboarding/v1/onboardingv1connect"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/domain"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/handler"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/repository"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/service"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/platform/config"
	platformpostgres "github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/platform/postgres"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"
)

// Set TEST_DATABASE_URL only for a disposable, already migrated database.
// Tests optionally load the root .env; they do not run migrations or truncate tables.
func integrationPool(t *testing.T) (*pgxpool.Pool, context.Context) {
	t.Helper()
	if err := config.LoadDotEnv("../../../../.env"); err != nil {
		t.Fatal(err)
	}
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
		Status:             domain.ApplicationStatusPendingTermsAcceptance,
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
	r := NewApplicationRepository(pool)
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
	r := NewApplicationRepository(pool)
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
	r := NewApplicationRepository(pool)
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

func TestIntegrationStartApplication(t *testing.T) {
	pool, ctx := integrationPool(t)
	r := NewApplicationRepository(pool)
	requestID := uuid.New()
	call := func(input string) *httptest.ResponseRecorder {
		t.Helper()
		// Fresh instances prove replay depends on PostgreSQL rather than process memory.
		_, rpc := onboardingv1connect.NewApplicationServiceHandler(handler.New(
			service.NewStartApplicationService(r), service.NewAcceptTermsService(NewTransactor(pool), r), service.NewSubmitIdentityService(NewTransactor(pool), r, nil)))
		body, err := protojson.Marshal(&onboardingv1.StartApplicationRequest{RequestId: input})
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPost, onboardingv1connect.ApplicationServiceStartApplicationProcedure, bytes.NewReader(body)).WithContext(ctx)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Connect-Protocol-Version", "1")
		response := httptest.NewRecorder()
		rpc.ServeHTTP(response, req)
		return response
	}
	decode := func(response *httptest.ResponseRecorder) *onboardingv1.StartApplicationResponse {
		t.Helper()
		if response.Code != http.StatusOK {
			t.Fatalf("expected success, got HTTP %d", response.Code)
		}
		result := new(onboardingv1.StartApplicationResponse)
		if err := protojson.Unmarshal(response.Body.Bytes(), result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	before := time.Now().UTC().Truncate(time.Microsecond)
	first := decode(call(requestID.String()))
	after := time.Now().UTC()
	id, err := uuid.Parse(first.ApplicationId)
	if err != nil || id == uuid.Nil || first.Status != onboardingv1.ApplicationStatus_APPLICATION_STATUS_PENDING_TERMS_ACCEPTANCE {
		t.Fatalf("unexpected response: %v", first)
	}
	cleanupApplication(t, pool, id)
	stored, err := r.GetByRequestID(ctx, requestID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ApplicationID != id || stored.RequestID != requestID ||
		stored.Status != domain.ApplicationStatusPendingTermsAcceptance || stored.TemporalWorkflowID != id.String() ||
		stored.CreatedAt.Before(before) || stored.CreatedAt.After(after) || !stored.CreatedAt.Equal(stored.UpdatedAt) ||
		stored.CompletedAt != nil || stored.RejectedAtStep != nil {
		t.Fatalf("unexpected persisted application: %+v", stored)
	}
	replay := decode(call(requestID.String()))
	if !proto.Equal(first, replay) {
		t.Fatalf("replay changed response: %v versus %v", first, replay)
	}
	again, err := r.GetByRequestID(ctx, requestID)
	if err != nil {
		t.Fatal(err)
	}
	assertApplication(t, again, stored)
	var rows int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM applications WHERE request_id = $1", requestID).Scan(&rows); err != nil {
		t.Fatal("count query failed (details omitted)")
	}
	if rows != 1 {
		t.Fatalf("expected one row for the request ID, got %d", rows)
	}
	var beforeInvalid int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM applications").Scan(&beforeInvalid); err != nil {
		t.Fatal("count query failed (details omitted)")
	}
	invalid := call("invalid-request-id")
	var failure struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(invalid.Body.Bytes(), &failure); err != nil {
		t.Fatal(err)
	}
	if invalid.Code != http.StatusBadRequest || failure.Code != "invalid_argument" {
		t.Fatalf("invalid request: HTTP %d, code %q", invalid.Code, failure.Code)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM applications").Scan(&rows); err != nil {
		t.Fatal("count query failed (details omitted)")
	}
	if rows != beforeInvalid {
		t.Fatal("invalid input changed the number of persisted rows")
	}
}

func TestIntegrationWorkflowStartupConditionalTransition(t *testing.T) {
	pool, ctx := integrationPool(t)
	r := NewApplicationRepository(pool)
	for _, target := range []domain.ApplicationStatus{domain.ApplicationStatusPendingIdentityVerification, domain.ApplicationStatusFailedTechnical} {
		t.Run(string(target), func(t *testing.T) {
			app := fixture()
			app.Status = domain.ApplicationStatusPendingWorkflowStart
			if err := r.Create(ctx, app); err != nil {
				t.Fatal(err)
			}
			cleanupApplication(t, pool, app.ApplicationID)
			now := app.UpdatedAt.Add(time.Hour)
			got, err := resolveWorkflowStartForTest(ctx, pool, r, app.ApplicationID, target, now)
			if err != nil {
				t.Fatal(err)
			}
			want := app
			want.Status, want.UpdatedAt = target, now
			assertApplication(t, got, want)
			stored, err := r.GetByID(ctx, app.ApplicationID)
			if err != nil {
				t.Fatal(err)
			}
			assertApplication(t, stored, want)
		})
	}
}

func TestIntegrationWorkflowStartupPreservesProgressedStatus(t *testing.T) {
	pool, ctx := integrationPool(t)
	r := NewApplicationRepository(pool)
	for _, initial := range []domain.ApplicationStatus{
		domain.ApplicationStatusPendingIdentityVerification,
		domain.ApplicationStatusPendingSanctionsScreening,
		domain.ApplicationStatusPendingHumanReview,
		domain.ApplicationStatusApproved,
		domain.ApplicationStatusPendingAccountCreation,
		domain.ApplicationStatusCompleted,
		domain.ApplicationStatusRejected,
		domain.ApplicationStatusFailedTechnical,
	} {
		t.Run(string(initial), func(t *testing.T) {
			app := fixture()
			app.Status = initial
			if initial == domain.ApplicationStatusCompleted || initial == domain.ApplicationStatusRejected {
				completed := app.UpdatedAt
				app.CompletedAt = &completed
			}
			if err := r.Create(ctx, app); err != nil {
				t.Fatal(err)
			}
			cleanupApplication(t, pool, app.ApplicationID)
			// One representative target is enough to check the pending-start guard.
			got, err := resolveWorkflowStartForTest(ctx, pool, r, app.ApplicationID, domain.ApplicationStatusPendingIdentityVerification, app.UpdatedAt.Add(time.Hour))
			if err != nil {
				t.Fatal(err)
			}
			assertApplication(t, got, app)
			stored, err := r.GetByID(ctx, app.ApplicationID)
			if err != nil {
				t.Fatal(err)
			}
			assertApplication(t, stored, app)
		})
	}
}

func TestIntegrationWorkflowStartupConcurrentOutcomes(t *testing.T) {
	pool, ctx := integrationPool(t)
	r := NewApplicationRepository(pool)
	app := fixture()
	app.Status = domain.ApplicationStatusPendingWorkflowStart
	if err := r.Create(ctx, app); err != nil {
		t.Fatal(err)
	}
	cleanupApplication(t, pool, app.ApplicationID)
	type result struct {
		app domain.Application
		err error
	}
	results := make(chan result, 2)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for index, status := range []domain.ApplicationStatus{domain.ApplicationStatusPendingIdentityVerification, domain.ApplicationStatusFailedTechnical} {
		wg.Add(1)
		go func(target domain.ApplicationStatus, now time.Time) {
			defer wg.Done()
			<-start
			got, err := resolveWorkflowStartForTest(ctx, pool, r, app.ApplicationID, target, now)
			results <- result{got, err}
		}(status, app.UpdatedAt.Add(time.Duration(index+1)*time.Hour))
	}
	close(start)
	wg.Wait()
	close(results)
	stored, err := r.GetByID(ctx, app.ApplicationID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != domain.ApplicationStatusPendingIdentityVerification && stored.Status != domain.ApplicationStatusFailedTechnical {
		t.Fatal("neither startup outcome was persisted")
	}
	for result := range results {
		if result.err != nil {
			t.Fatal(result.err)
		}
		assertApplication(t, result.app, stored)
	}
}

func resolveWorkflowStartForTest(ctx context.Context, db TransactorDB, r *ApplicationRepository, id uuid.UUID, target domain.ApplicationStatus, now time.Time) (domain.Application, error) {
	var app domain.Application
	err := NewTransactor(db).WithinTx(ctx, func(txCtx context.Context) error {
		if _, err := r.GetByIDForUpdate(txCtx, id); err != nil {
			return err
		}
		var err error
		app, err = r.ResolveWorkflowStart(txCtx, id, target, now)
		return err
	})
	if err != nil {
		return domain.Application{}, err
	}
	return app, nil
}
