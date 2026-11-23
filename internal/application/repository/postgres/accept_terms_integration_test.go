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

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/encoding/protojson"

	onboardingv1 "github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/gen/onboarding/v1"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/gen/onboarding/v1/onboardingv1connect"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/domain"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/handler"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/service"
)

// Each call constructs fresh layers; no HTTP listener or process-local replay cache.
func acceptTermsRPC(ctx context.Context, pool *pgxpool.Pool, id, term string) *httptest.ResponseRecorder {
	_, rpc := onboardingv1connect.NewApplicationServiceHandler(handler.New(service.New(New(pool))))
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
	if response.Code != status {
		t.Fatalf("HTTP status=%d, want %d", response.Code, status)
	}
	if code != "" {
		var failure struct {
			Code string `json:"code"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &failure); err != nil {
			t.Fatal(err)
		}
		if failure.Code != code {
			t.Fatalf("RPC code=%q, want %q", failure.Code, code)
		}
		return
	}
	var result onboardingv1.AcceptTermsResponse
	if err := protojson.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !result.Accepted {
		t.Fatal("expected accepted=true")
	}
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
		if _, err := pool.Exec(cleanupCtx, "DELETE FROM terms_acceptances WHERE application_id=$1", app.ApplicationID); err != nil {
			t.Error("could not clean up test terms (details omitted)")
		}
	})
	if err := New(pool).Create(ctx, app); err != nil {
		t.Fatal(err)
	}
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
	if err != nil {
		t.Fatal("read terms failed (details omitted)")
	}
	defer rows.Close()
	var result []storedTerms
	for rows.Next() {
		var a storedTerms
		if err := rows.Scan(&a.id, &a.term, &a.at); err != nil {
			t.Fatal("scan terms failed")
		}
		result = append(result, a)
	}
	if rows.Err() != nil {
		t.Fatal("read terms failed (details omitted)")
	}
	return result
}

func TestIntegrationAcceptTermsAndReplay(t *testing.T) {
	pool, ctx := integrationPool(t)
	app := termsFixture(t, ctx, pool, domain.ApplicationStatusPendingTermsAcceptance)
	before := time.Now().UTC().Truncate(time.Microsecond)
	assertTermsRPC(t, acceptTermsRPC(ctx, pool, app.ApplicationID.String(), " V1 "), http.StatusOK, "")
	terms := readTerms(t, ctx, pool, app.ApplicationID)
	if len(terms) != 1 {
		t.Fatalf("got %d acceptances, want one", len(terms))
	}
	a := terms[0]
	if a.id == uuid.Nil || a.term != " V1 " || a.at.Before(before) || a.at.After(time.Now()) {
		t.Fatalf("unexpected acceptance: %+v", a)
	}
	app.Status, app.UpdatedAt = domain.ApplicationStatusPendingIdentitySubmission, a.at
	stored, err := New(pool).GetByID(ctx, app.ApplicationID)
	if err != nil {
		t.Fatal(err)
	}
	assertApplication(t, stored, app)
	for _, status := range []domain.ApplicationStatus{domain.ApplicationStatusPendingIdentitySubmission, domain.ApplicationStatusPendingIdentityVerification} {
		if _, err := pool.Exec(ctx, "UPDATE applications SET status=$2 WHERE application_id=$1", app.ApplicationID, string(status)); err != nil {
			t.Fatal("set replay fixture state failed")
		}
		app.Status = status
		assertTermsRPC(t, acceptTermsRPC(ctx, pool, app.ApplicationID.String(), " V1 "), http.StatusOK, "")
		stored, err = New(pool).GetByID(ctx, app.ApplicationID)
		if err != nil {
			t.Fatal(err)
		}
		assertApplication(t, stored, app)
		again := readTerms(t, ctx, pool, app.ApplicationID)
		if len(again) != 1 || again[0].id != a.id || again[0].term != a.term || !again[0].at.Equal(a.at) {
			t.Fatal("replay changed acceptance")
		}
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
				if _, err := pool.Exec(ctx, "INSERT INTO terms_acceptances (acceptance_id, application_id, term_id, accepted_at) VALUES ($1,$2,$3,$4)", uuid.New(), app.ApplicationID, tc.existing, app.UpdatedAt); err != nil {
					t.Fatal("seed acceptance failed")
				}
			}
			original := readTerms(t, ctx, pool, app.ApplicationID)
			assertTermsRPC(t, acceptTermsRPC(ctx, pool, app.ApplicationID.String(), tc.term), tc.httpStatus, tc.code)
			stored, err := New(pool).GetByID(ctx, app.ApplicationID)
			if err != nil {
				t.Fatal(err)
			}
			assertApplication(t, stored, app)
			after := readTerms(t, ctx, pool, app.ApplicationID)
			if len(after) != len(original) {
				t.Fatal("failed request changed acceptance count")
			}
			if len(after) == 1 && (after[0].id != original[0].id || after[0].term != original[0].term || !after[0].at.Equal(original[0].at)) {
				t.Fatal("failed request changed acceptance")
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
			if successes != want {
				t.Fatalf("got %d successes, want %d", successes, want)
			}
			accepted := readTerms(t, ctx, pool, app.ApplicationID)
			if len(accepted) != 1 || accepted[0].term != terms[winner] {
				t.Fatal("concurrent requests did not preserve one winning acceptance")
			}
			app.Status, app.UpdatedAt = domain.ApplicationStatusPendingIdentitySubmission, accepted[0].at
			stored, err := New(pool).GetByID(ctx, app.ApplicationID)
			if err != nil {
				t.Fatal(err)
			}
			assertApplication(t, stored, app)
		})
	}
}
