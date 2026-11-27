package mock_test

import (
	"context"
	"testing"

	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/thirdparty/domain"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/thirdparty/mock"
	"github.com/stretchr/testify/require"
)

func TestIdentityVerifier(t *testing.T) {
	p := mock.NewIdentityVerifier()
	for _, tt := range []struct {
		name   string
		change func(*domain.Identity)
		want   bool
	}{
		{"exact match", func(*domain.Identity) {}, true},
		{"unknown ID", func(i *domain.Identity) { i.IDNumber = "000000000999" }, false},
		{"name mismatch, no trimming", func(i *domain.Identity) { i.FullName = " Mock Citizen " }, false},
		{"birth mismatch", func(i *domain.Identity) { i.DateOfBirth = "1991-01-01" }, false},
		{"expiry mismatch", func(i *domain.Identity) { i.IDExpiryDate = "2041-01-01" }, false},
		{"contacts ignored", func(i *domain.Identity) { i.PhoneNumber = "different"; s := "other@example.invalid"; i.Email = &s }, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			i := domain.Identity{IDNumber: "000000000001", FullName: "Mock Citizen", DateOfBirth: "1990-01-01", IDExpiryDate: "2040-01-01", PhoneNumber: "0900000000"}
			tt.change(&i)
			got, err := p.VerifyIdentity(context.Background(), i)
			require.Equal(t, tt.want, got)
			require.NoError(t, err)
		})
	}
}

func TestIdentityVerifierAdditionalRecords(t *testing.T) {
	p := mock.NewIdentityVerifier()
	for _, identity := range []domain.Identity{
		{IDNumber: "000000000002", FullName: "Mock Citizen Two", DateOfBirth: "1985-06-15", IDExpiryDate: "2041-06-15"},
		{IDNumber: "000000000003", FullName: "Mock Citizen Three", DateOfBirth: "1995-03-20", IDExpiryDate: "2042-03-20"},
		{IDNumber: "000000000004", FullName: "Mock Citizen Four", DateOfBirth: "2000-09-10", IDExpiryDate: "2043-09-10"},
		{IDNumber: "000000000005", FullName: "Mock Citizen Five", DateOfBirth: "1988-12-25", IDExpiryDate: "2044-12-25"},
		{IDNumber: "000000000006", FullName: "Mock Citizen Six", DateOfBirth: "1992-02-29", IDExpiryDate: "2044-02-29"},
		{IDNumber: "079123456789", FullName: "Nguyen Van A", DateOfBirth: "2000-05-15", IDExpiryDate: "2032-05-15", PhoneNumber: "0901234567"},
	} {
		t.Run(identity.IDNumber, func(t *testing.T) {
			verified, err := p.VerifyIdentity(context.Background(), identity)
			require.True(t, verified)
			require.NoError(t, err)
			identity.FullName = "Mismatched Name"
			verified, err = p.VerifyIdentity(context.Background(), identity)
			require.False(t, verified)
			require.NoError(t, err)
		})
	}
}
