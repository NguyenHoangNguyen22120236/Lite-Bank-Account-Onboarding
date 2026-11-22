package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNewApplication(t *testing.T) {
	id, requestID := uuid.New(), uuid.New()
	now := time.Date(2026, 11, 21, 10, 20, 30, 123456000, time.FixedZone("ICT", 7*60*60))
	got, err := NewApplication(id, requestID, now)
	if err != nil {
		t.Fatal(err)
	}
	if got.ApplicationID != id || got.RequestID != requestID || got.TemporalWorkflowID != id.String() {
		t.Fatalf("unexpected identifiers: %+v", got)
	}
	if got.Status != ApplicationStatusPendingTermsAcceptance || got.CompletedAt != nil || got.RejectedAtStep != nil {
		t.Fatalf("unexpected initial state: %+v", got)
	}
	if !got.CreatedAt.Equal(now) || !got.UpdatedAt.Equal(now) || got.CreatedAt.Location() != time.UTC || got.UpdatedAt.Location() != time.UTC {
		t.Fatalf("timestamps must preserve the supplied instant in UTC: %+v", got)
	}
}

func TestNewApplicationRejectsInvalidInputs(t *testing.T) {
	for _, tc := range []struct {
		name          string
		id, requestID uuid.UUID
		now           time.Time
	}{
		{"nil_application_id", uuid.Nil, uuid.New(), time.Now()},
		{"nil_request_id", uuid.New(), uuid.Nil, time.Now()},
		{"zero_time", uuid.New(), uuid.New(), time.Time{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NewApplication(tc.id, tc.requestID, tc.now)
			if err == nil || got != (Application{}) {
				t.Fatalf("expected error and zero application, got %+v, %v", got, err)
			}
		})
	}
}

func TestApplicationStatusValid(t *testing.T) {
	valid := []ApplicationStatus{
		ApplicationStatusPendingTermsAcceptance, ApplicationStatusPendingIdentitySubmission,
		ApplicationStatusPendingIdentityVerification, ApplicationStatusPendingSanctionsScreening,
		ApplicationStatusPendingHumanReview, ApplicationStatusApproved, ApplicationStatusRejected,
		ApplicationStatusCompleted, ApplicationStatusPendingWorkflowStart,
		ApplicationStatusPendingAccountCreation, ApplicationStatusFailedTechnical,
	}
	for _, status := range valid {
		if !status.Valid() {
			t.Errorf("expected valid status: %q", status)
		}
	}
	for _, status := range []ApplicationStatus{"", "APPLICATION_STATUS_UNSPECIFIED", "UNKNOWN"} {
		if status.Valid() {
			t.Errorf("unexpected valid status: %q", status)
		}
	}
}

func TestRejectedAtStepValid(t *testing.T) {
	for _, step := range []RejectedAtStep{RejectedAtStepIdentityVerification, RejectedAtStepHumanReview} {
		if !step.Valid() {
			t.Errorf("expected valid rejection step: %q", step)
		}
	}
	for _, step := range []RejectedAtStep{"", "REJECTED_AT_STEP_UNSPECIFIED", "UNKNOWN"} {
		if step.Valid() {
			t.Errorf("unexpected valid rejection step: %q", step)
		}
	}
}
