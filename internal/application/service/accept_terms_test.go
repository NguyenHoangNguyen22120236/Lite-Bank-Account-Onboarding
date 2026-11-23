package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/domain"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/repository"
	"github.com/google/uuid"
)

type termsRepositoryStub struct {
	repositoryStub
	accept func(context.Context, domain.TermsAcceptance) error
}

func (r termsRepositoryStub) AcceptTerms(ctx context.Context, acceptance domain.TermsAcceptance) error {
	return r.accept(ctx, acceptance)
}

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
			r := termsRepositoryStub{accept: func(context.Context, domain.TermsAcceptance) error {
				t.Fatal("invalid input reached repository")
				return nil
			}}
			if err := New(r).AcceptTerms(context.Background(), tc.id, tc.term); !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
		})
	}
}

func TestAcceptTermsPersistence(t *testing.T) {
	for _, cause := range []error{nil, repository.ErrNotFound, domain.ErrConflictingTerms,
		domain.ErrInvalidTermsState, repository.ErrDatabase, context.Canceled, context.DeadlineExceeded} {
		t.Run(fmt.Sprint(cause), func(t *testing.T) {
			id, ctx := uuid.New(), context.Background()
			before := time.Now().UTC()
			calls := 0
			r := termsRepositoryStub{accept: func(gotCtx context.Context, a domain.TermsAcceptance) error {
				calls++
				if gotCtx != ctx || a.ApplicationID != id || a.AcceptanceID == uuid.Nil || a.TermID != " V1 " ||
					a.AcceptedAt.Before(before) || a.AcceptedAt.After(time.Now()) || a.AcceptedAt.Location() != time.UTC {
					t.Fatalf("unexpected persistence input: %+v", a)
				}
				if cause != nil {
					return fmt.Errorf("accept: %w", cause)
				}
				return nil
			}}
			err := New(r).AcceptTerms(ctx, id.String(), " V1 ")
			if calls != 1 || !errors.Is(err, cause) {
				t.Fatalf("calls=%d, error=%v; want %v", calls, err, cause)
			}
		})
	}
}
