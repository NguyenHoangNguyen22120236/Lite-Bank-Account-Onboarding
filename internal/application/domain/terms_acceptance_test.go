package domain

import (
	"errors"
	"testing"
)

// CheckTermsAcceptance returns replay=true only when no write is needed.
func TestCheckTermsAcceptance(t *testing.T) {
	for _, tc := range []struct {
		name     string
		status   ApplicationStatus
		existing []string
		replay   bool
		want     error
	}{
		{"first acceptance", ApplicationStatusPendingTermsAcceptance, nil, false, nil},
		{"replay", ApplicationStatusPendingIdentitySubmission, []string{"terms-v1"}, true, nil},
		{"later replay", ApplicationStatusCompleted, []string{"terms-v1"}, true, nil},
		{"different term", ApplicationStatusPendingIdentitySubmission, []string{"terms-v2"}, false, ErrConflictingTerms},
		{"case sensitive", ApplicationStatusPendingIdentitySubmission, []string{"TERMS-V1"}, false, ErrConflictingTerms},
		{"exact whitespace", ApplicationStatusPendingIdentitySubmission, []string{" terms-v1 "}, false, ErrConflictingTerms},
		{"multiple conflicting records", ApplicationStatusCompleted, []string{"terms-v1", "terms-v2"}, false, ErrConflictingTerms},
		{"missing acceptance", ApplicationStatusPendingIdentitySubmission, nil, false, ErrInvalidTermsState},
		{"terminal without acceptance", ApplicationStatusRejected, nil, false, ErrInvalidTermsState},
		{"inconsistent initial state", ApplicationStatusPendingTermsAcceptance, []string{"terms-v1"}, false, ErrInvalidTermsState},
	} {
		t.Run(tc.name, func(t *testing.T) {
			replay, err := CheckTermsAcceptance(tc.status, tc.existing, "terms-v1")
			if replay != tc.replay || !errors.Is(err, tc.want) {
				t.Fatalf("got replay=%t, error=%v; want %t, %v", replay, err, tc.replay, tc.want)
			}
		})
	}
}
