package service_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/thirdparty/domain"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/thirdparty/service"
)

type verifierFunc func(context.Context, domain.Identity) (bool, error)

func (f verifierFunc) VerifyIdentity(ctx context.Context, i domain.Identity) (bool, error) {
	return f(ctx, i)
}

func validIdentity() domain.Identity {
	return domain.Identity{IDNumber: "000000000001", FullName: "Mock Citizen", DateOfBirth: "1990-01-01", IDExpiryDate: "2040-01-01", PhoneNumber: "0900000000"}
}

func TestVerifyIdentityRejectsInvalidInputBeforeProvider(t *testing.T) {
	malformed := validIdentity()
	malformed.IDNumber = "bad"
	s := service.New(verifierFunc(func(context.Context, domain.Identity) (bool, error) {
		t.Fatal("invalid input reached provider")
		return false, nil
	}))
	for _, input := range []*domain.Identity{nil, &malformed} {
		if _, err := s.VerifyIdentity(context.Background(), input); !errors.Is(err, domain.ErrInvalidIdentity) {
			t.Fatalf("error = %v, want invalid identity", err)
		}
	}
}

func TestVerifyIdentityForwardsContextAndInput(t *testing.T) {
	i := validIdentity()
	i.FullName = " Mock Citizen "
	i.Email = new(string)
	type key struct{}
	ctx := context.WithValue(context.Background(), key{}, "caller")
	calls := 0
	s := service.New(verifierFunc(func(gotCtx context.Context, got domain.Identity) (bool, error) {
		calls++
		if gotCtx != ctx || !reflect.DeepEqual(got, i) {
			t.Fatal("context or identity changed")
		}
		return true, nil
	}))
	if got, err := s.VerifyIdentity(ctx, &i); !got || err != nil || calls != 1 {
		t.Fatalf("got %v, %v, calls=%d", got, err, calls)
	}
}

func TestVerifyIdentityProviderOutcomes(t *testing.T) {
	for _, tt := range []struct {
		name string
		err  error
	}{
		{"business rejection", nil},
		{"canceled", fmt.Errorf("wrapped: %w", context.Canceled)},
		{"deadline", fmt.Errorf("wrapped: %w", context.DeadlineExceeded)},
		{"unavailable", fmt.Errorf("wrapped: %w", service.ErrProviderUnavailable)},
		{"unexpected", errors.New("provider failure")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			i := validIdentity()
			calls := 0
			s := service.New(verifierFunc(func(context.Context, domain.Identity) (bool, error) {
				calls++
				return false, tt.err
			}))
			got, err := s.VerifyIdentity(context.Background(), &i)
			if got || !errors.Is(err, tt.err) || calls != 1 {
				t.Fatalf("got %v, %v, calls=%d", got, err, calls)
			}
		})
	}
}

func TestVerifyIdentityCanceledContextSkipsProvider(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	i := validIdentity()
	s := service.New(verifierFunc(func(context.Context, domain.Identity) (bool, error) {
		t.Fatal("canceled request reached provider")
		return false, nil
	}))
	if _, err := s.VerifyIdentity(ctx, &i); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want canceled", err)
	}
}
