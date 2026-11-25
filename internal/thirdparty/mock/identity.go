package mock

import (
	"context"

	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/thirdparty/domain"
)

// IdentityVerifier checks fixed synthetic government records without mutable state.
type IdentityVerifier struct{}

func NewIdentityVerifier() *IdentityVerifier {
	return &IdentityVerifier{}
}

func (*IdentityVerifier) VerifyIdentity(ctx context.Context, identity domain.Identity) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	// Keep fixtures local so callers cannot mutate shared provider data.
	records := [...]domain.Identity{
		{IDNumber: "000000000001", FullName: "Mock Citizen", DateOfBirth: "1990-01-01", IDExpiryDate: "2040-01-01"},
		{IDNumber: "000000000002", FullName: "Mock Citizen Two", DateOfBirth: "1985-06-15", IDExpiryDate: "2041-06-15"},
		{IDNumber: "000000000003", FullName: "Mock Citizen Three", DateOfBirth: "1995-03-20", IDExpiryDate: "2042-03-20"},
		{IDNumber: "000000000004", FullName: "Mock Citizen Four", DateOfBirth: "2000-09-10", IDExpiryDate: "2043-09-10"},
		{IDNumber: "000000000005", FullName: "Mock Citizen Five", DateOfBirth: "1988-12-25", IDExpiryDate: "2044-12-25"},
		{IDNumber: "000000000006", FullName: "Mock Citizen Six", DateOfBirth: "1992-02-29", IDExpiryDate: "2044-02-29"},
		{IDNumber: "079123456789", FullName: "Nguyen Van A", DateOfBirth: "2000-05-15", IDExpiryDate: "2032-05-15"},
	}
	for _, record := range records {
		if identity.IDNumber == record.IDNumber {
			return identity.FullName == record.FullName &&
				identity.DateOfBirth == record.DateOfBirth &&
				identity.IDExpiryDate == record.IDExpiryDate, nil
		}
	}
	return false, nil
}
