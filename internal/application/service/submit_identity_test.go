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

func serviceIdentity() *domain.Identity {
	email := ""
	return &domain.Identity{IDNumber: "079123456789", FullName: " Name ", DateOfBirth: "2000-01-01", IDExpiryDate: "2032-01-01", PhoneNumber: "0901234567", Email: &email}
}

func TestSubmitIdentityValidation(t *testing.T) {
	for _, tc := range []struct {
		name, id string
		identity *domain.Identity
		want     error
	}{
		{"empty ID", "", serviceIdentity(), ErrInvalidApplicationID},
		{"malformed ID", "APP-001", serviceIdentity(), ErrInvalidApplicationID},
		{"nil UUID", uuid.Nil.String(), serviceIdentity(), ErrInvalidApplicationID},
		{"missing identity", uuid.NewString(), nil, domain.ErrInvalidIdentity},
		{"invalid identity", uuid.NewString(), &domain.Identity{}, domain.ErrInvalidIdentity},
	} {
		t.Run(tc.name, func(t *testing.T) {

			got, err := NewSubmitIdentityService(noTransaction(t), nil, nil).SubmitIdentity(context.Background(), tc.id, tc.identity)
			if !errors.Is(err, tc.want) || got != (domain.Application{}) {
				t.Fatalf("got %+v, %v; want %v", got, err, tc.want)
			}
		})
	}
}

func TestSubmitIdentityPersistence(t *testing.T) {
	for _, cause := range []error{nil, repository.ErrNotFound, repository.ErrDatabase, domain.ErrConflictingIdentity,
		domain.ErrInvalidIdentityState, domain.ErrInvalidIdentity, context.Canceled, context.DeadlineExceeded} {
		t.Run(fmt.Sprint(cause), func(t *testing.T) {
			f := newOrchestrationFixture(t, domain.ApplicationStatusCompleted)
			input := serviceIdentity()
			// Expired identity remains a valid exact replay.
			input.IDExpiryDate = "2001-01-01"
			original := *input
			stored := *input
			stored.ApplicationID = f.app.ApplicationID
			f.identity = &stored
			want := f.app
			if cause != nil {
				f.failAt, f.cause = "read identity", fmt.Errorf("submit: %w", cause)
			}
			got, err := NewSubmitIdentityService(f, f, nil).SubmitIdentity(context.Background(), f.app.ApplicationID.String(), input)
			if !errors.Is(err, cause) {
				t.Fatalf("error=%v, want %v", err, cause)
			}
			reads := 0
			for _, event := range f.events {
				if event == "read identity" {
					reads++
				}
			}
			if reads != 1 {
				t.Fatalf("read calls=%d, want one", reads)
			}
			if cause == nil && got != want {
				t.Fatal("persisted result not returned")
			}
			if cause != nil && got != (domain.Application{}) {
				t.Fatal("failure returned an application")
			}
			if !reflect.DeepEqual(*input, original) {
				t.Fatal("caller input mutated")
			}
			if f.insertedIdentity != nil || f.app != want {
				t.Fatal("replay modified persistence")
			}
		})
	}
}

func TestSubmitIdentityOrchestration(t *testing.T) {
	f := newOrchestrationFixture(t, domain.ApplicationStatusPendingIdentitySubmission)
	input := serviceIdentity()
	original := *input
	before := time.Now().UTC()
	starter := startupFunc(func(ctx context.Context, appID, workflowID string) (WorkflowStartOutcome, error) {
		if !f.committed || ctx.Value(transactionTestKey{}) != nil {
			t.Fatal("Temporal startup must follow commit and use caller context")
		}
		if appID != f.app.ApplicationID.String() || workflowID != f.app.TemporalWorkflowID {
			t.Fatal("wrong workflow identifiers")
		}
		return WorkflowStartEstablished, nil
	})
	got, err := NewSubmitIdentityService(f, f, starter).SubmitIdentity(context.Background(), f.app.ApplicationID.String(), input)
	if err != nil || got.Status != domain.ApplicationStatusPendingIdentityVerification {
		t.Fatalf("got %+v, %v", got, err)
	}
	i := f.insertedIdentity
	if i == nil || i.ApplicationID != f.app.ApplicationID || i.CreatedAt.Before(before) || i.CreatedAt.After(time.Now()) || i.CreatedAt.Location() != time.UTC {
		t.Fatal("incorrect persistence metadata")
	}
	copyIdentity := *i
	copyIdentity.ApplicationID, copyIdentity.CreatedAt = original.ApplicationID, original.CreatedAt
	if !reflect.DeepEqual(copyIdentity, original) || !reflect.DeepEqual(*input, original) {
		t.Fatal("submitted values changed")
	}
	if i.Email == input.Email {
		t.Fatal("persisted email aliases caller pointer")
	}
	want := []string{"begin", "lock", "read identity", "insert identity", "advance identity", "commit", "begin", "lock", "advance workflow", "commit"}
	if !reflect.DeepEqual(f.events, want) {
		t.Fatalf("events=%v, want %v", f.events, want)
	}
}

func TestSubmitIdentityDomainDecisions(t *testing.T) {
	for _, tc := range []struct {
		name              string
		status            domain.ApplicationStatus
		existing, changed bool
		expiry            string
		want              error
	}{
		{"exact replay", domain.ApplicationStatusCompleted, true, false, "2001-01-01", nil},
		{"conflicting identity", domain.ApplicationStatusCompleted, true, true, "2032-01-01", domain.ErrConflictingIdentity},
		{"identity before submission", domain.ApplicationStatusPendingIdentitySubmission, true, false, "2032-01-01", domain.ErrInvalidIdentityState},
		{"terms not accepted", domain.ApplicationStatusPendingTermsAcceptance, false, false, "2032-01-01", domain.ErrInvalidIdentityState},
		{"missing progressed identity", domain.ApplicationStatusCompleted, false, false, "2032-01-01", domain.ErrInvalidIdentityState},
		{"expired first submission", domain.ApplicationStatusPendingIdentitySubmission, false, false, "2001-01-01", domain.ErrInvalidIdentity},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newOrchestrationFixture(t, tc.status)
			input := serviceIdentity()
			input.IDExpiryDate = tc.expiry
			if tc.existing {
				stored := *input
				stored.ApplicationID = f.app.ApplicationID
				f.identity = &stored
			}
			if tc.changed {
				input.FullName += "changed"
			}
			before := f.app
			got, err := NewSubmitIdentityService(f, f, nil).SubmitIdentity(context.Background(), f.app.ApplicationID.String(), input)
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
			if f.insertedIdentity != nil || f.app != before {
				t.Fatal("replay or rejection changed persistence")
			}
			if err == nil && got != before {
				t.Fatal("lost persisted replay result")
			}
		})
	}
}

func TestSubmitIdentityTransactionFailures(t *testing.T) {
	for _, stage := range []string{"begin", "lock", "read identity", "insert identity", "advance identity", "commit"} {
		t.Run(stage, func(t *testing.T) {
			f := newOrchestrationFixture(t, domain.ApplicationStatusPendingIdentitySubmission)
			before := f.app
			f.failAt = stage
			starter := startupFunc(func(context.Context, string, string) (WorkflowStartOutcome, error) {
				t.Fatal("Temporal called after failed identity transaction")
				return 0, nil
			})
			got, err := NewSubmitIdentityService(f, f, starter).SubmitIdentity(context.Background(), f.app.ApplicationID.String(), serviceIdentity())
			if !errors.Is(err, repository.ErrDatabase) || got != (domain.Application{}) || f.committed || f.app != before || f.identity != nil {
				t.Fatalf("failed transaction changed state: err=%v events=%v", err, f.events)
			}
			if stage != "begin" && f.events[len(f.events)-1] != "rollback" {
				t.Fatal("missing rollback")
			}
		})
	}
}
