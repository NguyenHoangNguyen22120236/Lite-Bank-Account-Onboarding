package service_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/thirdparty/domain"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/thirdparty/service"
	"github.com/stretchr/testify/require"
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
		require.FailNow(t, "invalid input reached provider")
		return false, nil
	}))
	for _, input := range []*domain.Identity{nil, &malformed} {
		_, err := s.VerifyIdentity(context.Background(), input)
		require.ErrorIs(t, err, domain.ErrInvalidIdentity)
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
		require.True(t, gotCtx == ctx, "caller context changed")
		require.Equal(t, i, got)
		return true, nil
	}))
	got, err := s.VerifyIdentity(ctx, &i)
	require.NoError(t, err)
	require.True(t, got)
	require.Equal(t, 1, calls)
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
			require.False(t, got)
			require.ErrorIs(t, err, tt.err)
			require.Equal(t, 1, calls)
		})
	}
}

func TestVerifyIdentityCanceledContextSkipsProvider(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	i := validIdentity()
	s := service.New(verifierFunc(func(context.Context, domain.Identity) (bool, error) {
		require.FailNow(t, "canceled request reached provider")
		return false, nil
	}))
	_, err := s.VerifyIdentity(ctx, &i)
	require.ErrorIs(t, err, context.Canceled)
}
