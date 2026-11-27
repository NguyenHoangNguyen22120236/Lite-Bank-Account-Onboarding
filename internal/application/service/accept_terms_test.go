package service

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/domain"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/repository"
	"github.com/google/uuid"
)

func TestAcceptTermsValidation(t *testing.T) {
	valid := uuid.NewString()
	for _, tc := range []struct {
		name, id, term string
		want           error
	}{
		{"empty ID", "", "v1", ErrInvalidApplicationID},
		{"malformed ID", "invalid", "v1", ErrInvalidApplicationID},
		{"nil ID", uuid.Nil.String(), "v1", ErrInvalidApplicationID},
		{"empty term", valid, "", ErrInvalidTermID},
		{"blank term", valid, " \t\n", ErrInvalidTermID},
		{"NUL term", valid, "v1\x00", ErrInvalidTermID},
	} {
		t.Run(tc.name, func(t *testing.T) {

			if err := NewAcceptTermsService(noTransaction(t), nil).AcceptTerms(context.Background(), tc.id, tc.term); !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
		})
	}
}

func TestAcceptTermsPersistence(t *testing.T) {
	for _, cause := range []error{nil, repository.ErrNotFound, domain.ErrConflictingTerms,
		domain.ErrInvalidTermsState, repository.ErrDatabase, context.Canceled, context.DeadlineExceeded} {
		t.Run(fmt.Sprint(cause), func(t *testing.T) {
			f := newOrchestrationFixture(t, domain.ApplicationStatusPendingTermsAcceptance)
			if cause != nil {
				f.failAt, f.cause = "insert terms", fmt.Errorf("accept: %w", cause)
			}
			before := time.Now().UTC()
			err := NewAcceptTermsService(f, f).AcceptTerms(context.Background(), f.app.ApplicationID.String(), " V1 ")
			if !errors.Is(err, cause) {
				t.Fatalf("got %v, want %v", err, cause)
			}
			inserts := 0
			for _, event := range f.events {
				if event == "insert terms" {
					inserts++
				}
			}
			if inserts != 1 {
				t.Fatalf("insert calls=%d, want one", inserts)
			}
			if cause == nil {
				a := f.insertedTerms
				if a == nil || a.ApplicationID != f.app.ApplicationID || a.AcceptanceID == uuid.Nil || a.TermID != " V1 " ||
					a.AcceptedAt.Before(before) || a.AcceptedAt.After(time.Now()) || a.AcceptedAt.Location() != time.UTC {
					t.Fatalf("unexpected persistence input: %+v", a)
				}
				if !f.committed || f.app.Status != domain.ApplicationStatusPendingIdentitySubmission {
					t.Fatal("acceptance not committed")
				}
			}
		})
	}
}

func TestAcceptTermsOrchestration(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status domain.ApplicationStatus
		terms  []string
		want   error
	}{
		{"first acceptance", domain.ApplicationStatusPendingTermsAcceptance, nil, nil},
		{"same term replay", domain.ApplicationStatusCompleted, []string{" V1 "}, nil},
		{"different term", domain.ApplicationStatusPendingIdentitySubmission, []string{"other"}, domain.ErrConflictingTerms},
		{"inconsistent initial acceptance", domain.ApplicationStatusPendingTermsAcceptance, []string{" V1 "}, domain.ErrInvalidTermsState},
		{"missing acceptance after progression", domain.ApplicationStatusPendingIdentitySubmission, nil, domain.ErrInvalidTermsState},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newOrchestrationFixture(t, tc.status)
			f.terms = tc.terms
			before := f.app
			err := NewAcceptTermsService(f, f).AcceptTerms(context.Background(), f.app.ApplicationID.String(), " V1 ")
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
			if tc.name == "first acceptance" {
				want := []string{"begin", "lock", "read terms", "insert terms", "advance terms", "commit"}
				if !reflect.DeepEqual(f.events, want) {
					t.Fatalf("events=%v, want %v", f.events, want)
				}
			} else if f.insertedTerms != nil || f.app != before {
				t.Fatal("replay or rejection changed application")
			}
		})
	}
}

func TestAcceptTermsTransactionFailures(t *testing.T) {
	for _, stage := range []string{"begin", "lock", "read terms", "insert terms", "advance terms", "commit"} {
		t.Run(stage, func(t *testing.T) {
			f := newOrchestrationFixture(t, domain.ApplicationStatusPendingTermsAcceptance)
			before := f.app
			f.failAt = stage
			err := NewAcceptTermsService(f, f).AcceptTerms(context.Background(), f.app.ApplicationID.String(), "v1")
			if !errors.Is(err, repository.ErrDatabase) || f.committed || f.app != before || len(f.terms) != 0 {
				t.Fatalf("failed transaction changed state: err=%v events=%v", err, f.events)
			}
			if stage != "begin" && f.events[len(f.events)-1] != "rollback" {
				t.Fatal("missing rollback")
			}
		})
	}
}
