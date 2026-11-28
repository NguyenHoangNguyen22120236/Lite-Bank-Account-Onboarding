package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// Application holds application-owned persistence state independently of transport.
type Application struct {
	ApplicationID      uuid.UUID
	RequestID          uuid.UUID
	Status             ApplicationStatus
	TemporalWorkflowID string
	CreatedAt          time.Time
	UpdatedAt          time.Time
	CompletedAt        *time.Time
	RejectedAtStep     *RejectedAtStep
}

// ApplicationStatus uses the persisted names from the application contract.
// Its zero value is not a valid persisted status.
type ApplicationStatus string

const (
	ApplicationStatusPendingTermsAcceptance      ApplicationStatus = "APPLICATION_STATUS_PENDING_TERMS_ACCEPTANCE"
	ApplicationStatusPendingIdentitySubmission   ApplicationStatus = "APPLICATION_STATUS_PENDING_IDENTITY_SUBMISSION"
	ApplicationStatusPendingIdentityVerification ApplicationStatus = "APPLICATION_STATUS_PENDING_IDENTITY_VERIFICATION"
	ApplicationStatusPendingSanctionsScreening   ApplicationStatus = "APPLICATION_STATUS_PENDING_SANCTIONS_SCREENING"
	ApplicationStatusPendingHumanReview          ApplicationStatus = "APPLICATION_STATUS_PENDING_HUMAN_REVIEW"
	ApplicationStatusApproved                    ApplicationStatus = "APPLICATION_STATUS_APPROVED"
	ApplicationStatusRejected                    ApplicationStatus = "APPLICATION_STATUS_REJECTED"
	ApplicationStatusCompleted                   ApplicationStatus = "APPLICATION_STATUS_COMPLETED"
	ApplicationStatusPendingWorkflowStart        ApplicationStatus = "APPLICATION_STATUS_PENDING_WORKFLOW_START"
	ApplicationStatusPendingAccountCreation      ApplicationStatus = "APPLICATION_STATUS_PENDING_ACCOUNT_CREATION"
	ApplicationStatusFailedTechnical             ApplicationStatus = "APPLICATION_STATUS_FAILED_TECHNICAL"
)

func (s ApplicationStatus) Valid() bool {
	switch s {
	case ApplicationStatusPendingTermsAcceptance, ApplicationStatusPendingIdentitySubmission,
		ApplicationStatusPendingIdentityVerification, ApplicationStatusPendingSanctionsScreening,
		ApplicationStatusPendingHumanReview, ApplicationStatusApproved, ApplicationStatusRejected,
		ApplicationStatusCompleted, ApplicationStatusPendingWorkflowStart,
		ApplicationStatusPendingAccountCreation, ApplicationStatusFailedTechnical:
		return true
	default:
		return false
	}
}

// Terminal applications cannot resume onboarding through ordinary request retries.
func (s ApplicationStatus) Terminal() bool {
	return s == ApplicationStatusCompleted || s == ApplicationStatusRejected || s == ApplicationStatusFailedTechnical
}

// RejectedAtStep is optional on Application; absence is represented by nil.
type RejectedAtStep string

const (
	RejectedAtStepIdentityVerification RejectedAtStep = "REJECTED_AT_STEP_IDENTITY_VERIFICATION"
	RejectedAtStepHumanReview          RejectedAtStep = "REJECTED_AT_STEP_HUMAN_REVIEW"
)

func (s RejectedAtStep) Valid() bool {
	return s == RejectedAtStepIdentityVerification || s == RejectedAtStepHumanReview
}

// NewApplication initializes tracking with caller-supplied IDs and time.
// It performs no I/O and does not start a Temporal workflow.
func NewApplication(applicationID, requestID uuid.UUID, now time.Time) (Application, error) {
	if applicationID == uuid.Nil {
		return Application{}, errors.New("application: application ID is required")
	}
	if requestID == uuid.Nil {
		return Application{}, errors.New("application: request ID is required")
	}
	if now.IsZero() {
		return Application{}, errors.New("application: creation time is required")
	}
	return Application{
		ApplicationID:      applicationID,
		RequestID:          requestID,
		Status:             ApplicationStatusPendingTermsAcceptance,
		TemporalWorkflowID: applicationID.String(),
		CreatedAt:          now.UTC(),
		UpdatedAt:          now.UTC(),
	}, nil
}
