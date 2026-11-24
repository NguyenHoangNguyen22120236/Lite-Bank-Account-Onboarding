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

type identityRepositoryStub struct {
	repositoryStub
	submit func(context.Context, domain.Identity) (domain.Application, error)
}

func (r identityRepositoryStub) SubmitIdentity(ctx context.Context, i domain.Identity) (domain.Application, error) {
	return r.submit(ctx, i)
}

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
			r := identityRepositoryStub{submit: func(context.Context, domain.Identity) (domain.Application, error) {
				t.Fatal("invalid input reached persistence")
				return domain.Application{}, nil
			}}
			got, err := New(r, nil).SubmitIdentity(context.Background(), tc.id, tc.identity)
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
			ctx := context.Background()
			id := uuid.New()
			input := serviceIdentity()
			// An expired payload can be a durable replay; the locked domain decision checks dates.
			input.IDExpiryDate = "2001-01-01"
			original := *input
			want := domain.Application{ApplicationID: id, Status: domain.ApplicationStatusCompleted}
			calls := 0
			before := time.Now().UTC()
			r := identityRepositoryStub{submit: func(gotCtx context.Context, got domain.Identity) (domain.Application, error) {
				calls++
				if gotCtx != ctx || got.ApplicationID != id || got.CreatedAt.Before(before) || got.CreatedAt.After(time.Now()) || got.CreatedAt.Location() != time.UTC {
					t.Fatal("incorrect persistence metadata")
				}
				got.ApplicationID, got.CreatedAt = original.ApplicationID, original.CreatedAt
				if !reflect.DeepEqual(got, original) {
					t.Fatal("submitted fields or email presence changed")
				}
				if cause != nil {
					return domain.Application{}, fmt.Errorf("submit: %w", cause)
				}
				return want, nil
			}}
			got, err := New(r, nil).SubmitIdentity(ctx, id.String(), input)
			if calls != 1 || !errors.Is(err, cause) {
				t.Fatalf("calls=%d, error=%v", calls, err)
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
		})
	}
}
