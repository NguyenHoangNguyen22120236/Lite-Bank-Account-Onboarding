package domain_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/thirdparty/domain"
)

func TestValidateIdentityStructure(t *testing.T) {
	for _, tt := range []struct {
		name   string
		change func(*domain.Identity)
		valid  bool
	}{
		{"valid, absent email", func(*domain.Identity) {}, true},
		{"empty email", func(i *domain.Identity) { i.Email = new(string) }, true},
		{"unformatted email", func(i *domain.Identity) { s := "not-an-email"; i.Email = &s }, true},
		{"20 Unicode phone characters", func(i *domain.Identity) { i.PhoneNumber = strings.Repeat("\u754c", 20) }, true},
		{"no age or expiry eligibility check", func(i *domain.Identity) { i.DateOfBirth = "2090-01-01"; i.IDExpiryDate = "2000-01-01" }, true},
		{"short ID", func(i *domain.Identity) { i.IDNumber = "123" }, false},
		{"nondigit ID", func(i *domain.Identity) { i.IDNumber = "00000000000x" }, false},
		{"non-ASCII digits", func(i *domain.Identity) { i.IDNumber = strings.Repeat("\u0661", 6) }, false},
		{"blank name", func(i *domain.Identity) { i.FullName = " \t" }, false},
		{"blank phone", func(i *domain.Identity) { i.PhoneNumber = " " }, false},
		{"long phone", func(i *domain.Identity) { i.PhoneNumber = strings.Repeat("\u754c", 21) }, false},
		{"NUL name", func(i *domain.Identity) { i.FullName = "Mock\x00Citizen" }, false},
		{"invalid UTF-8 phone", func(i *domain.Identity) { i.PhoneNumber = string([]byte{0xff}) }, false},
		{"NUL email", func(i *domain.Identity) { s := "a\x00b"; i.Email = &s }, false},
		{"invalid UTF-8 email", func(i *domain.Identity) { s := string([]byte{0xff}); i.Email = &s }, false},
		{"non-strict date", func(i *domain.Identity) { i.DateOfBirth = "1990-1-01" }, false},
		{"impossible birth date", func(i *domain.Identity) { i.DateOfBirth = "1990-02-30" }, false},
		{"zero year", func(i *domain.Identity) { i.DateOfBirth = "0000-01-01" }, false},
		{"invalid expiry", func(i *domain.Identity) { i.IDExpiryDate = "2040-13-01" }, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			i := domain.Identity{IDNumber: "000000000001", FullName: "Mock Citizen", DateOfBirth: "1990-01-01", IDExpiryDate: "2040-01-01", PhoneNumber: "0900000000"}
			tt.change(&i)
			err := domain.ValidateIdentityStructure(&i)
			if tt.valid && err != nil {
				t.Fatalf("valid input: %v", err)
			}
			if !tt.valid && !errors.Is(err, domain.ErrInvalidIdentity) {
				t.Fatalf("error = %v, want invalid identity", err)
			}
		})
	}
	if err := domain.ValidateIdentityStructure(nil); !errors.Is(err, domain.ErrInvalidIdentity) {
		t.Fatalf("missing identity error = %v", err)
	}
}
