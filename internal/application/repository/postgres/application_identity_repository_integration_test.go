package postgres

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
)

func identityPayload() *onboardingv1.IdentityInfo {
	return &onboardingv1.IdentityInfo{IdNumber: "079123456789", FullName: " Nguyen Van A ", DateOfBirth: "2000-02-29", IdExpiryDate: "2099-05-15", PhoneNumber: "0901234567"}
}

// Fresh layers exercise the generated Connect route without an HTTP listener.
func submitIdentityRPC(t *testing.T, ctx context.Context, db ApplicationDB, txDB TransactorDB, id string, identity *onboardingv1.IdentityInfo) *httptest.ResponseRecorder {
	t.Helper()
	r := NewApplicationRepository(db)
	_, rpc := onboardingv1connect.NewApplicationServiceHandler(handler.New(
		service.NewStartApplicationService(r, nil), service.NewAcceptTermsService(NewTransactor(txDB), r), service.NewSubmitIdentityService(NewTransactor(txDB), r)))
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
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &failure))
		require.Equal(t, wantHTTP, response.Code)
		require.Equal(t, code, failure.Code)
		return
	}
	require.Equal(t, http.StatusOK, response.Code)
	var result onboardingv1.SubmitIdentityResponse
	require.NoError(t, protojson.Unmarshal(response.Body.Bytes(), &result))
	require.Equal(t, id.String(), result.ApplicationId)
	require.Equal(t, string(status), result.Status.String())
}

func identityFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool, status domain.ApplicationStatus) domain.Application {
	t.Helper()
	app := termsFixture(t, ctx, pool, status)
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, cleanupErr := pool.Exec(cleanupCtx, "DELETE FROM application_identities WHERE application_id=$1", app.ApplicationID)
		assert.True(t, cleanupErr == nil, "identity cleanup failed (details omitted)")
	})
	if status != domain.ApplicationStatusPendingTermsAcceptance {
		_, termsErr := pool.Exec(ctx, "INSERT INTO terms_acceptances (acceptance_id, application_id, term_id, accepted_at) VALUES ($1,$2,$3,$4)", uuid.New(), app.ApplicationID, "v1", app.UpdatedAt)
		require.True(t, termsErr == nil, "seed terms failed")
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
	require.True(t, err == nil, "read identity failed")
	defer rows.Close()
	var result *identitySnapshot
	for rows.Next() {
		require.Nil(t, result, "multiple stored identities")
		result = &identitySnapshot{}
		require.True(t, rows.Scan(&result.ID, &result.Name, &result.Birth, &result.Expiry, &result.Phone, &result.Email, &result.CreatedAt) == nil, "scan identity failed")
	}
	require.True(t, rows.Err() == nil, "read identity failed")
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
			assertIdentityRPC(t, submitIdentityRPC(t, ctx, pool, pool, app.ApplicationID.String(), input), app.ApplicationID, domain.ApplicationStatusPendingIdentityVerification, "")
			stored := readIdentity(t, ctx, pool, app.ApplicationID)
			require.NotNil(t, stored, "identity missing after success")
			want := &identitySnapshot{ID: input.IdNumber, Name: input.FullName, Birth: input.DateOfBirth, Expiry: input.IdExpiryDate, Phone: input.PhoneNumber, Email: input.Email, CreatedAt: stored.CreatedAt}
			require.Equal(t, want, stored, "identity fields, optional email, or timestamp incorrect")
			require.False(t, stored.CreatedAt.Before(before), "identity fields, optional email, or timestamp incorrect")
			require.False(t, stored.CreatedAt.After(time.Now()), "identity fields, optional email, or timestamp incorrect")
			app.Status = domain.ApplicationStatusPendingIdentityVerification
			got, err := NewApplicationRepository(pool).GetByID(ctx, app.ApplicationID)
			require.NoError(t, err)
			require.False(t, got.UpdatedAt.Before(stored.CreatedAt), "startup transition timestamp outside submission interval")
			require.False(t, got.UpdatedAt.After(time.Now()), "startup transition timestamp outside submission interval")
			app.UpdatedAt = got.UpdatedAt
			assertApplication(t, got, app)
			assertIdentityRPC(t, submitIdentityRPC(t, ctx, pool, pool, app.ApplicationID.String(), input), app.ApplicationID, app.Status, "")
			require.Equal(t, stored, readIdentity(t, ctx, pool, app.ApplicationID), "replay modified identity")
			got, err = NewApplicationRepository(pool).GetByID(ctx, app.ApplicationID)
			require.NoError(t, err)
			assertApplication(t, got, app)
			// Seed an already accepted expired identity and progressed state; replay must not revalidate expiry.
			input.IdExpiryDate = "2001-01-01"
			stored.Expiry = input.IdExpiryDate
			_, expiryErr := pool.Exec(ctx, "UPDATE application_identities SET id_expiry_date=$2 WHERE application_id=$1", app.ApplicationID, input.IdExpiryDate)
			require.True(t, expiryErr == nil, "seed expired identity failed")
			_, progressErr := pool.Exec(ctx, "UPDATE applications SET status=$2 WHERE application_id=$1", app.ApplicationID, string(domain.ApplicationStatusCompleted))
			require.True(t, progressErr == nil, "seed progressed status failed")
			app.Status = domain.ApplicationStatusCompleted
			assertIdentityRPC(t, submitIdentityRPC(t, ctx, pool, pool, app.ApplicationID.String(), input), app.ApplicationID, app.Status, "")
			require.Equal(t, stored, readIdentity(t, ctx, pool, app.ApplicationID), "progressed replay modified identity")
			got, err = NewApplicationRepository(pool).GetByID(ctx, app.ApplicationID)
			require.NoError(t, err)
			assertApplication(t, got, app)
			require.Equal(t, terms, readTerms(t, ctx, pool, app.ApplicationID), "submission modified terms")
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
				_, identityErr := pool.Exec(ctx, `INSERT INTO application_identities (application_id,id_number,full_name,date_of_birth,id_expiry_date,phone_number,email,created_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, app.ApplicationID, input.IdNumber, input.FullName, input.DateOfBirth, input.IdExpiryDate, input.PhoneNumber, input.Email, app.UpdatedAt)
				require.True(t, identityErr == nil, "seed identity failed")
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
			assertIdentityRPC(t, submitIdentityRPC(t, ctx, pool, pool, app.ApplicationID.String(), input), app.ApplicationID, "", tc.code)
			got, err := NewApplicationRepository(pool).GetByID(ctx, app.ApplicationID)
			require.NoError(t, err)
			assertApplication(t, got, app)
			require.Equal(t, before, readIdentity(t, ctx, pool, app.ApplicationID), "rejected request changed identity")
		})
	}
	assertIdentityRPC(t, submitIdentityRPC(t, ctx, pool, pool, uuid.NewString(), identityPayload()), uuid.Nil, "", "not_found")
	assertIdentityRPC(t, submitIdentityRPC(t, ctx, pool, pool, "APP-001", identityPayload()), uuid.Nil, "", "invalid_argument")
	assertIdentityRPC(t, submitIdentityRPC(t, ctx, pool, pool, uuid.NewString(), nil), uuid.Nil, "", "invalid_argument")
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
					responses[i] = submitIdentityRPC(t, ctx, pool, pool, app.ApplicationID.String(), inputs[i])
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
			require.Equal(t, want, successes)
			stored := readIdentity(t, ctx, pool, app.ApplicationID)
			require.NotNil(t, stored, "winning identity not preserved")
			require.Equal(t, inputs[winner].FullName, stored.Name, "winning identity not preserved")
			app.Status = domain.ApplicationStatusPendingIdentityVerification
			got, err := NewApplicationRepository(pool).GetByID(ctx, app.ApplicationID)
			require.NoError(t, err)
			require.False(t, got.UpdatedAt.Before(stored.CreatedAt), "startup transition timestamp outside submission interval")
			require.False(t, got.UpdatedAt.After(time.Now()), "startup transition timestamp outside submission interval")
			app.UpdatedAt = got.UpdatedAt
			assertApplication(t, got, app)
		})
	}
}

func TestIntegrationSubmitIdentityDatabaseErrors(t *testing.T) {
	pool, ctx := integrationPool(t)
	app := identityFixture(t, ctx, pool, domain.ApplicationStatusPendingIdentitySubmission)
	// Inject a begin failure through generated mocks; PostgreSQL verifies no data changed.
	const sensitive = "synthetic-sensitive-driver-detail"
	txDB := NewMockTransactorDB(t)
	txDB.EXPECT().Begin(mock.MatchedBy(func(actual context.Context) bool {
		deadline, ok := actual.Deadline()
		wantDeadline, wantOK := ctx.Deadline()
		return ok == wantOK && deadline.Equal(wantDeadline) && actual.Done() == ctx.Done()
	})).Return(nil, errors.New(sensitive)).Once()
	response := submitIdentityRPC(t, ctx, NewMockApplicationDB(t), txDB, app.ApplicationID.String(), identityPayload())
	assertIdentityRPC(t, response, app.ApplicationID, "", "internal")
	require.NotContains(t, response.Body.String(), sensitive, "RPC exposed database detail")
	for _, deadline := range []bool{false, true} {
		requestCtx, cancel := context.WithCancel(ctx)
		code := "canceled"
		if deadline {
			cancel()
			requestCtx, cancel = context.WithDeadline(ctx, time.Now().Add(-time.Second))
			code = "deadline_exceeded"
		}
		cancel()
		assertIdentityRPC(t, submitIdentityRPC(t, requestCtx, pool, pool, app.ApplicationID.String(), identityPayload()), app.ApplicationID, "", code)
	}
	got, err := NewApplicationRepository(pool).GetByID(ctx, app.ApplicationID)
	require.NoError(t, err)
	assertApplication(t, got, app)
	require.Nil(t, readIdentity(t, ctx, pool, app.ApplicationID), "failed request stored identity")
}
