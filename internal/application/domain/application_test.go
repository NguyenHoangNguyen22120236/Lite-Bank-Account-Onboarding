package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewApplication(t *testing.T) {
	id, requestID := uuid.New(), uuid.New()
	now := time.Date(2026, 11, 21, 10, 20, 30, 123456000, time.FixedZone("ICT", 7*60*60))
	got, err := NewApplication(id, requestID, now)
	require.NoError(t, err)
	require.Equal(t, id, got.ApplicationID)
	require.Equal(t, requestID, got.RequestID)
	require.Equal(t, id.String(), got.TemporalWorkflowID)
	require.Equal(t, ApplicationStatusPendingTermsAcceptance, got.Status)
	require.Nil(t, got.CompletedAt)
	require.Nil(t, got.RejectedAtStep)
	require.True(t, got.CreatedAt.Equal(now))
	require.True(t, got.UpdatedAt.Equal(now))
	require.Equal(t, time.UTC, got.CreatedAt.Location())
	require.Equal(t, time.UTC, got.UpdatedAt.Location())
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
			require.Error(t, err)
			require.Equal(t, (Application{}), got)
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
		assert.True(t, status.Valid())
	}
	for _, status := range []ApplicationStatus{"", "APPLICATION_STATUS_UNSPECIFIED", "UNKNOWN"} {
		assert.False(t, status.Valid())
	}
}

func TestRejectedAtStepValid(t *testing.T) {
	for _, step := range []RejectedAtStep{RejectedAtStepIdentityVerification, RejectedAtStepHumanReview} {
		assert.True(t, step.Valid())
	}
	for _, step := range []RejectedAtStep{"", "REJECTED_AT_STEP_UNSPECIFIED", "UNKNOWN"} {
		assert.False(t, step.Valid())
	}
}
