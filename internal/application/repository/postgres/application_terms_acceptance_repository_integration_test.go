package postgres

import (
	"bytes"
	"context"
	"encoding/json"
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
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
)

// Each call constructs fresh layers; no HTTP listener or process-local replay cache.
func acceptTermsRPC(ctx context.Context, pool *pgxpool.Pool, id, term string) *httptest.ResponseRecorder {
	r := NewApplicationRepository(pool)
	_, rpc := onboardingv1connect.NewApplicationServiceHandler(handler.New(
		service.NewStartApplicationService(r), service.NewAcceptTermsService(NewTransactor(pool), r), service.NewSubmitIdentityService(NewTransactor(pool), r, nil)))
	body, err := protojson.Marshal(&onboardingv1.AcceptTermsRequest{ApplicationId: id, TermId: term})
	if err != nil {
		panic(err)
	}
	req := httptest.NewRequest(http.MethodPost, onboardingv1connect.ApplicationServiceAcceptTermsProcedure, bytes.NewReader(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connect-Protocol-Version", "1")
	response := httptest.NewRecorder()
	rpc.ServeHTTP(response, req)
	return response
}

func assertTermsRPC(t *testing.T, response *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	require.Equal(t, status, response.Code)
	if code != "" {
		var failure struct {
			Code string `json:"code"`
		}
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &failure))
		require.Equal(t, code, failure.Code)
		return
	}
	var result onboardingv1.AcceptTermsResponse
	require.NoError(t, protojson.Unmarshal(response.Body.Bytes(), &result))
	require.True(t, result.Accepted, "expected accepted=true")
}

func termsFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool, status domain.ApplicationStatus) domain.Application {
	t.Helper()
	app := fixture()
	app.Status = status
	cleanupApplication(t, pool, app.ApplicationID)
	// LIFO cleanup removes child rows before the existing parent cleanup runs.
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, cleanupErr := pool.Exec(cleanupCtx, "DELETE FROM terms_acceptances WHERE application_id=$1", app.ApplicationID)
		assert.True(t, cleanupErr == nil, "could not clean up test terms (details omitted)")
	})
	require.NoError(t, NewApplicationRepository(pool).Create(ctx, app))
	return app
}

type storedTerms struct {
	id   uuid.UUID
	term string
	at   time.Time
}

func readTerms(t *testing.T, ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) []storedTerms {
	t.Helper()
	rows, err := pool.Query(ctx, "SELECT acceptance_id, term_id, accepted_at FROM terms_acceptances WHERE application_id=$1", id)
	require.True(t, err == nil, "read terms failed (details omitted)")
	defer rows.Close()
	var result []storedTerms
	for rows.Next() {
		var a storedTerms
		require.True(t, rows.Scan(&a.id, &a.term, &a.at) == nil, "scan terms failed")
		result = append(result, a)
	}
	require.True(t, rows.Err() == nil, "read terms failed (details omitted)")
	return result
}

func TestIntegrationAcceptTermsAndReplay(t *testing.T) {
	pool, ctx := integrationPool(t)
	app := termsFixture(t, ctx, pool, domain.ApplicationStatusPendingTermsAcceptance)
	before := time.Now().UTC().Truncate(time.Microsecond)
	assertTermsRPC(t, acceptTermsRPC(ctx, pool, app.ApplicationID.String(), " V1 "), http.StatusOK, "")
	terms := readTerms(t, ctx, pool, app.ApplicationID)
	require.Len(t, terms, 1)
	a := terms[0]
	require.NotEqual(t, uuid.Nil, a.id)
	require.Equal(t, " V1 ", a.term)
	require.False(t, a.at.Before(before))
	require.False(t, a.at.After(time.Now()))
	app.Status, app.UpdatedAt = domain.ApplicationStatusPendingIdentitySubmission, a.at
	stored, err := NewApplicationRepository(pool).GetByID(ctx, app.ApplicationID)
	require.NoError(t, err)
	assertApplication(t, stored, app)
	for _, status := range []domain.ApplicationStatus{domain.ApplicationStatusPendingIdentitySubmission, domain.ApplicationStatusPendingIdentityVerification} {
		_, err = pool.Exec(ctx, "UPDATE applications SET status=$2 WHERE application_id=$1", app.ApplicationID, string(status))
		require.True(t, err == nil, "set replay fixture state failed")
		app.Status = status
		assertTermsRPC(t, acceptTermsRPC(ctx, pool, app.ApplicationID.String(), " V1 "), http.StatusOK, "")
		stored, err = NewApplicationRepository(pool).GetByID(ctx, app.ApplicationID)
		require.NoError(t, err)
		assertApplication(t, stored, app)
		again := readTerms(t, ctx, pool, app.ApplicationID)
		require.Len(t, again, 1, "replay changed acceptance")
		require.Equal(t, a.id, again[0].id, "replay changed acceptance")
		require.Equal(t, a.term, again[0].term, "replay changed acceptance")
		require.True(t, again[0].at.Equal(a.at), "replay changed acceptance")
	}
}

func TestIntegrationAcceptTermsErrors(t *testing.T) {
	pool, ctx := integrationPool(t)
	for _, tc := range []struct {
		name                 string
		status               domain.ApplicationStatus
		existing, term, code string
		httpStatus           int
	}{
		{"invalid state", domain.ApplicationStatusPendingIdentitySubmission, "", "v1", "failed_precondition", http.StatusBadRequest},
		{"conflicting term", domain.ApplicationStatusPendingIdentitySubmission, "v1", "v2", "already_exists", http.StatusConflict},
		{"invalid input", domain.ApplicationStatusPendingTermsAcceptance, "", " \t", "invalid_argument", http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := termsFixture(t, ctx, pool, tc.status)
			if tc.existing != "" {
				_, acceptanceErr := pool.Exec(ctx, "INSERT INTO terms_acceptances (acceptance_id, application_id, term_id, accepted_at) VALUES ($1,$2,$3,$4)", uuid.New(), app.ApplicationID, tc.existing, app.UpdatedAt)
				require.True(t, acceptanceErr == nil, "seed acceptance failed")
			}
			original := readTerms(t, ctx, pool, app.ApplicationID)
			assertTermsRPC(t, acceptTermsRPC(ctx, pool, app.ApplicationID.String(), tc.term), tc.httpStatus, tc.code)
			stored, err := NewApplicationRepository(pool).GetByID(ctx, app.ApplicationID)
			require.NoError(t, err)
			assertApplication(t, stored, app)
			after := readTerms(t, ctx, pool, app.ApplicationID)
			require.Len(t, after, len(original), "failed request changed acceptance count")
			if len(after) == 1 {
				require.Equal(t, original[0].id, after[0].id, "failed request changed acceptance")
				require.Equal(t, original[0].term, after[0].term, "failed request changed acceptance")
				require.True(t, after[0].at.Equal(original[0].at), "failed request changed acceptance")
			}
		})
	}
	assertTermsRPC(t, acceptTermsRPC(ctx, pool, uuid.NewString(), "v1"), http.StatusNotFound, "not_found")
}

func TestIntegrationAcceptTermsConcurrent(t *testing.T) {
	for _, same := range []bool{true, false} {
		name := "different terms"
		if same {
			name = "same term"
		}
		t.Run(name, func(t *testing.T) {
			pool, ctx := integrationPool(t)
			app := termsFixture(t, ctx, pool, domain.ApplicationStatusPendingTermsAcceptance)
			terms := []string{"v1", "v1"}
			if !same {
				terms[1] = "v2"
			}
			responses := make([]*httptest.ResponseRecorder, len(terms))
			start := make(chan struct{})
			var wg sync.WaitGroup
			for i := range terms {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					<-start
					responses[i] = acceptTermsRPC(ctx, pool, app.ApplicationID.String(), terms[i])
				}(i)
			}
			close(start)
			wg.Wait()
			winner, successes := -1, 0
			for i, response := range responses {
				if response.Code == http.StatusOK {
					assertTermsRPC(t, response, http.StatusOK, "")
					successes++
					winner = i
				} else {
					assertTermsRPC(t, response, http.StatusConflict, "already_exists")
				}
			}
			want := 1
			if same {
				want = 2
			}
			require.Equal(t, want, successes)
			accepted := readTerms(t, ctx, pool, app.ApplicationID)
			require.Len(t, accepted, 1, "concurrent requests did not preserve one winning acceptance")
			require.Equal(t, terms[winner], accepted[0].term, "concurrent requests did not preserve one winning acceptance")
			app.Status, app.UpdatedAt = domain.ApplicationStatusPendingIdentitySubmission, accepted[0].at
			stored, err := NewApplicationRepository(pool).GetByID(ctx, app.ApplicationID)
			require.NoError(t, err)
			assertApplication(t, stored, app)
		})
	}
}
