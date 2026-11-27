package postgres

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	onboardingv1 "github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/gen/onboarding/v1"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/gen/onboarding/v1/onboardingv1connect"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/domain"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/handler"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/service"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/encoding/protojson"
)

func identityPayload() *onboardingv1.IdentityInfo {
	return &onboardingv1.IdentityInfo{IdNumber: "079123456789", FullName: " Nguyen Van A ", DateOfBirth: "2000-02-29", IdExpiryDate: "2099-05-15", PhoneNumber: "0901234567"}
}

// Fresh layers exercise the generated Connect route without an HTTP listener.
type acceptedWorkflowStarter struct{}

func (acceptedWorkflowStarter) EnsureStarted(context.Context, string, string) (service.WorkflowStartOutcome, error) {
	return service.WorkflowStartEstablished, nil
}

func submitIdentityRPC(ctx context.Context, db integrationDB, id string, identity *onboardingv1.IdentityInfo) *httptest.ResponseRecorder {
	r := NewApplicationRepository(db)
	_, rpc := onboardingv1connect.NewApplicationServiceHandler(handler.New(
		service.NewStartApplicationService(r), service.NewAcceptTermsService(NewTransactor(db), r), service.NewSubmitIdentityService(NewTransactor(db), r, acceptedWorkflowStarter{})))
	body, err := protojson.Marshal(&onboardingv1.SubmitIdentityRequest{ApplicationId: id, Identity: identity})
	if err != nil {
		panic(err)
	}
	req := httptest.NewRequest(http.MethodPost, onboardingv1connect.ApplicationServiceSubmitIdentityProcedure, bytes.NewReader(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connect-Protocol-Version", "1")
	response := httptest.NewRecorder()
	rpc.ServeHTTP(response, req)
	return response
}

func assertIdentityRPC(t *testing.T, response *httptest.ResponseRecorder, id uuid.UUID, status domain.ApplicationStatus, code string) {
	t.Helper()
	if code != "" {
		wantHTTP := map[string]int{"invalid_argument": 400, "not_found": 404, "already_exists": 409, "failed_precondition": 400, "internal": 500, "canceled": 499, "deadline_exceeded": 504}[code]
		var failure struct {
			Code string `json:"code"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &failure); err != nil {
			t.Fatal("invalid RPC error JSON")
		}
		if response.Code != wantHTTP || failure.Code != code {
			t.Fatalf("HTTP=%d, code=%q; want %d, %q", response.Code, failure.Code, wantHTTP, code)
		}
		return
	}
	if response.Code != http.StatusOK {
		t.Fatalf("HTTP=%d, want success", response.Code)
	}
	var result onboardingv1.SubmitIdentityResponse
	if err := protojson.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.ApplicationId != id.String() || result.Status.String() != string(status) {
		t.Fatalf("unexpected result: id=%q status=%s", result.ApplicationId, result.Status)
	}
}

func identityFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool, status domain.ApplicationStatus) domain.Application {
	t.Helper()
	app := termsFixture(t, ctx, pool, status)
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := pool.Exec(cleanupCtx, "DELETE FROM application_identities WHERE application_id=$1", app.ApplicationID); err != nil {
			t.Error("identity cleanup failed (details omitted)")
		}
	})
	if status != domain.ApplicationStatusPendingTermsAcceptance {
		if _, err := pool.Exec(ctx, "INSERT INTO terms_acceptances (acceptance_id, application_id, term_id, accepted_at) VALUES ($1,$2,$3,$4)", uuid.New(), app.ApplicationID, "v1", app.UpdatedAt); err != nil {
			t.Fatal("seed terms failed")
		}
	}
	return app
}

type identitySnapshot struct {
	ID, Name, Birth, Expiry, Phone string
	Email                          *string
	CreatedAt                      time.Time
}

func readIdentity(t *testing.T, ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) *identitySnapshot {
	t.Helper()
	rows, err := pool.Query(ctx, `SELECT id_number, full_name, date_of_birth::text, id_expiry_date::text, phone_number, email, created_at FROM application_identities WHERE application_id=$1`, id)
	if err != nil {
		t.Fatal("read identity failed")
	}
	defer rows.Close()
	var result *identitySnapshot
	for rows.Next() {
		if result != nil {
			t.Fatal("multiple stored identities")
		}
		result = &identitySnapshot{}
		if err := rows.Scan(&result.ID, &result.Name, &result.Birth, &result.Expiry, &result.Phone, &result.Email, &result.CreatedAt); err != nil {
			t.Fatal("scan identity failed")
		}
	}
	if rows.Err() != nil {
		t.Fatal("read identity failed")
	}
	return result
}

func TestIntegrationSubmitIdentityPersistenceAndReplay(t *testing.T) {
	pool, ctx := integrationPool(t)
	for _, present := range []bool{false, true} {
		name := "absent email"
		if present {
			name = "present empty email"
		}
		t.Run(name, func(t *testing.T) {
			app := identityFixture(t, ctx, pool, domain.ApplicationStatusPendingIdentitySubmission)
			input := identityPayload()
			if present {
				email := ""
				input.Email = &email
			}
			terms := readTerms(t, ctx, pool, app.ApplicationID)
			before := time.Now().UTC().Truncate(time.Microsecond)
			assertIdentityRPC(t, submitIdentityRPC(ctx, pool, app.ApplicationID.String(), input), app.ApplicationID, domain.ApplicationStatusPendingIdentityVerification, "")
			stored := readIdentity(t, ctx, pool, app.ApplicationID)
			if stored == nil {
				t.Fatal("identity missing after success")
			}
			want := &identitySnapshot{ID: input.IdNumber, Name: input.FullName, Birth: input.DateOfBirth, Expiry: input.IdExpiryDate, Phone: input.PhoneNumber, Email: input.Email, CreatedAt: stored.CreatedAt}
			if !reflect.DeepEqual(stored, want) || stored.CreatedAt.Before(before) || stored.CreatedAt.After(time.Now()) {
				t.Fatal("identity fields, optional email, or timestamp incorrect")
			}
			app.Status = domain.ApplicationStatusPendingIdentityVerification
			got, err := NewApplicationRepository(pool).GetByID(ctx, app.ApplicationID)
			if err != nil {
				t.Fatal(err)
			}
			if got.UpdatedAt.Before(stored.CreatedAt) || got.UpdatedAt.After(time.Now()) {
				t.Fatal("startup transition timestamp outside submission interval")
			}
			app.UpdatedAt = got.UpdatedAt
			assertApplication(t, got, app)
			assertIdentityRPC(t, submitIdentityRPC(ctx, pool, app.ApplicationID.String(), input), app.ApplicationID, app.Status, "")
			if !reflect.DeepEqual(readIdentity(t, ctx, pool, app.ApplicationID), stored) {
				t.Fatal("replay modified identity")
			}
			got, err = NewApplicationRepository(pool).GetByID(ctx, app.ApplicationID)
			if err != nil {
				t.Fatal(err)
			}
			assertApplication(t, got, app)
			// Seed an already accepted expired identity and progressed state; replay must not revalidate expiry.
			input.IdExpiryDate = "2001-01-01"
			stored.Expiry = input.IdExpiryDate
			if _, err := pool.Exec(ctx, "UPDATE application_identities SET id_expiry_date=$2 WHERE application_id=$1", app.ApplicationID, input.IdExpiryDate); err != nil {
				t.Fatal("seed expired identity failed")
			}
			if _, err := pool.Exec(ctx, "UPDATE applications SET status=$2 WHERE application_id=$1", app.ApplicationID, string(domain.ApplicationStatusCompleted)); err != nil {
				t.Fatal("seed progressed status failed")
			}
			app.Status = domain.ApplicationStatusCompleted
			assertIdentityRPC(t, submitIdentityRPC(ctx, pool, app.ApplicationID.String(), input), app.ApplicationID, app.Status, "")
			if !reflect.DeepEqual(readIdentity(t, ctx, pool, app.ApplicationID), stored) {
				t.Fatal("progressed replay modified identity")
			}
			got, err = NewApplicationRepository(pool).GetByID(ctx, app.ApplicationID)
			if err != nil {
				t.Fatal(err)
			}
			assertApplication(t, got, app)
			if !reflect.DeepEqual(readTerms(t, ctx, pool, app.ApplicationID), terms) {
				t.Fatal("submission modified terms")
			}
		})
	}
}

func TestIntegrationSubmitIdentityConflictsAndStates(t *testing.T) {
	pool, ctx := integrationPool(t)
	for _, tc := range []struct {
		name     string
		status   domain.ApplicationStatus
		existing bool
		change   string
		code     string
	}{
		{"name conflict", domain.ApplicationStatusPendingWorkflowStart, true, "name", "already_exists"},
		{"email presence conflict", domain.ApplicationStatusPendingWorkflowStart, true, "email", "already_exists"},
		{"terms not accepted", domain.ApplicationStatusPendingTermsAcceptance, false, "", "failed_precondition"},
		{"missing identity after progression", domain.ApplicationStatusPendingWorkflowStart, false, "", "failed_precondition"},
		{"terminal missing identity", domain.ApplicationStatusRejected, false, "", "failed_precondition"},
		{"inconsistent initial identity", domain.ApplicationStatusPendingIdentitySubmission, true, "", "failed_precondition"},
		{"invalid input", domain.ApplicationStatusPendingIdentitySubmission, false, "invalid", "invalid_argument"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := identityFixture(t, ctx, pool, tc.status)
			input := identityPayload()
			if tc.existing {
				if _, err := pool.Exec(ctx, `INSERT INTO application_identities (application_id,id_number,full_name,date_of_birth,id_expiry_date,phone_number,email,created_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, app.ApplicationID, input.IdNumber, input.FullName, input.DateOfBirth, input.IdExpiryDate, input.PhoneNumber, input.Email, app.UpdatedAt); err != nil {
					t.Fatal("seed identity failed")
				}
			}
			before := readIdentity(t, ctx, pool, app.ApplicationID)
			switch tc.change {
			case "name":
				input.FullName += "x"
			case "email":
				email := ""
				input.Email = &email
			case "invalid":
				input.IdNumber = "bad"
			}
			assertIdentityRPC(t, submitIdentityRPC(ctx, pool, app.ApplicationID.String(), input), app.ApplicationID, "", tc.code)
			got, err := NewApplicationRepository(pool).GetByID(ctx, app.ApplicationID)
			if err != nil {
				t.Fatal(err)
			}
			assertApplication(t, got, app)
			if !reflect.DeepEqual(before, readIdentity(t, ctx, pool, app.ApplicationID)) {
				t.Fatal("rejected request changed identity")
			}
		})
	}
	assertIdentityRPC(t, submitIdentityRPC(ctx, pool, uuid.NewString(), identityPayload()), uuid.Nil, "", "not_found")
	assertIdentityRPC(t, submitIdentityRPC(ctx, pool, "APP-001", identityPayload()), uuid.Nil, "", "invalid_argument")
	assertIdentityRPC(t, submitIdentityRPC(ctx, pool, uuid.NewString(), nil), uuid.Nil, "", "invalid_argument")
}

func TestIntegrationSubmitIdentityConcurrent(t *testing.T) {
	pool, ctx := integrationPool(t)
	for _, same := range []bool{true, false} {
		name := "conflicting"
		if same {
			name = "identical"
		}
		t.Run(name, func(t *testing.T) {
			app := identityFixture(t, ctx, pool, domain.ApplicationStatusPendingIdentitySubmission)
			inputs := []*onboardingv1.IdentityInfo{identityPayload(), identityPayload()}
			if !same {
				inputs[1].FullName = "Other name"
			}
			responses := make([]*httptest.ResponseRecorder, 2)
			start := make(chan struct{})
			var wg sync.WaitGroup
			for i := range inputs {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					<-start
					responses[i] = submitIdentityRPC(ctx, pool, app.ApplicationID.String(), inputs[i])
				}(i)
			}
			close(start)
			wg.Wait()
			successes, winner := 0, -1
			for i, response := range responses {
				if response.Code == http.StatusOK {
					successes++
					winner = i
					assertIdentityRPC(t, response, app.ApplicationID, domain.ApplicationStatusPendingIdentityVerification, "")
				} else {
					assertIdentityRPC(t, response, app.ApplicationID, "", "already_exists")
				}
			}
			want := 1
			if same {
				want = 2
			}
			if successes != want {
				t.Fatalf("successes=%d, want %d", successes, want)
			}
			stored := readIdentity(t, ctx, pool, app.ApplicationID)
			if stored == nil || stored.Name != inputs[winner].FullName {
				t.Fatal("winning identity not preserved")
			}
			app.Status = domain.ApplicationStatusPendingIdentityVerification
			got, err := NewApplicationRepository(pool).GetByID(ctx, app.ApplicationID)
			if err != nil {
				t.Fatal(err)
			}
			if got.UpdatedAt.Before(stored.CreatedAt) || got.UpdatedAt.After(time.Now()) {
				t.Fatal("startup transition timestamp outside submission interval")
			}
			app.UpdatedAt = got.UpdatedAt
			assertApplication(t, got, app)
		})
	}
}

func TestIntegrationSubmitIdentityDatabaseErrors(t *testing.T) {
	pool, ctx := integrationPool(t)
	app := identityFixture(t, ctx, pool, domain.ApplicationStatusPendingIdentitySubmission)
	// Reuse the existing failing DB adapter for a begin failure; no transaction fault injection.
	const sensitive = "synthetic-sensitive-driver-detail"
	response := submitIdentityRPC(ctx, failingDB{errors.New(sensitive)}, app.ApplicationID.String(), identityPayload())
	assertIdentityRPC(t, response, app.ApplicationID, "", "internal")
	if strings.Contains(response.Body.String(), sensitive) {
		t.Fatal("RPC exposed database detail")
	}
	for _, deadline := range []bool{false, true} {
		requestCtx, cancel := context.WithCancel(ctx)
		code := "canceled"
		if deadline {
			cancel()
			requestCtx, cancel = context.WithDeadline(ctx, time.Now().Add(-time.Second))
			code = "deadline_exceeded"
		}
		cancel()
		assertIdentityRPC(t, submitIdentityRPC(requestCtx, pool, app.ApplicationID.String(), identityPayload()), app.ApplicationID, "", code)
	}
	got, err := NewApplicationRepository(pool).GetByID(ctx, app.ApplicationID)
	if err != nil {
		t.Fatal(err)
	}
	assertApplication(t, got, app)
	if readIdentity(t, ctx, pool, app.ApplicationID) != nil {
		t.Fatal("failed request stored identity")
	}
}
