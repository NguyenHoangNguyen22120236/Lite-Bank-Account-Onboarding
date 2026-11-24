package postgres

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	onboardingv1 "github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/gen/onboarding/v1"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/gen/onboarding/v1/onboardingv1connect"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/domain"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/handler"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/service"
)

func TestIntegrationStartApplication(t *testing.T) {
	pool, ctx := integrationPool(t)
	r := New(pool)
	requestID := uuid.New()
	call := func(input string) *httptest.ResponseRecorder {
		t.Helper()
		// Fresh instances prove replay depends on PostgreSQL rather than process memory.
		_, rpc := onboardingv1connect.NewApplicationServiceHandler(handler.New(service.New(r, nil)))
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
