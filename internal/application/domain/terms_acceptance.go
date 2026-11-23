package domain

import (
	"time"

	"github.com/google/uuid"
)

type TermsAcceptance struct {
	AcceptanceID  uuid.UUID
	ApplicationID uuid.UUID
	TermID        string
	AcceptedAt    time.Time
}

// CheckTermsAcceptance decides whether to insert or replay, using state read
// under the application's database lock. It never changes the supplied state.
func CheckTermsAcceptance(status ApplicationStatus, existing []string, term string) (replay bool, err error) {
	for _, accepted := range existing {
		if accepted != term {
			return false, ErrConflictingTerms
		}
	}
	if !status.Valid() {
		return false, ErrInvalidTermsState
	}
	if len(existing) > 0 {
		if status == ApplicationStatusPendingTermsAcceptance {
			return false, ErrInvalidTermsState
		}
		return true, nil
	}
	if status != ApplicationStatusPendingTermsAcceptance {
		return false, ErrInvalidTermsState
	}
	return false, nil
}
