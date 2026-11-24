package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func validIdentity() Identity {
	return Identity{IDNumber: "079123456789", FullName: " Nguyen Van A ", DateOfBirth: "2000-02-29", IDExpiryDate: "2032-05-15", PhoneNumber: "0901234567"}
}

func TestIdentityStructure(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*Identity)
	}{
		{"short ID", func(i *Identity) { i.IDNumber = "123" }},
		{"non ASCII ID", func(i *Identity) { i.IDNumber = "０79123456789" }},
		{"nondigit ID", func(i *Identity) { i.IDNumber = "07912345678x" }},
		{"blank name", func(i *Identity) { i.FullName = " \t" }},
		{"blank phone", func(i *Identity) { i.PhoneNumber = " " }},
		{"long phone", func(i *Identity) { i.PhoneNumber = strings.Repeat("1", 21) }},
		{"impossible birth", func(i *Identity) { i.DateOfBirth = "2001-02-29" }},
		{"noncanonical birth", func(i *Identity) { i.DateOfBirth = "2000-2-29" }},
		{"year zero", func(i *Identity) { i.DateOfBirth = "0000-01-01" }},
		{"impossible expiry", func(i *Identity) { i.IDExpiryDate = "2032-04-31" }},
		{"NUL name", func(i *Identity) { i.FullName += "\x00" }},
		{"NUL phone", func(i *Identity) { i.PhoneNumber += "\x00" }},
		{"NUL email", func(i *Identity) { s := "x\x00"; i.Email = &s }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			i := validIdentity()
			tc.change(&i)
			if !errors.Is(ValidateIdentityStructure(&i), ErrInvalidIdentity) {
				t.Fatal("expected invalid identity")
			}
		})
	}
	if !errors.Is(ValidateIdentityStructure(nil), ErrInvalidIdentity) {
		t.Fatal("nil identity accepted")
	}
	i := validIdentity()
	i.PhoneNumber = strings.Repeat("界", 20)
	email := ""
	i.Email = &email
	if err := ValidateIdentityStructure(&i); err != nil {
		t.Fatal(err)
	}
}

func TestIdentityCalendar(t *testing.T) {
	for _, tc := range []struct {
		name, birth, expiry, today string
		valid                      bool
	}{
		{"eighteenth birthday", "2008-11-23", "2026-11-24", "2026-11-23", true},
		{"underage", "2008-11-24", "2026-11-24", "2026-11-23", false},
		{"future birth", "2027-01-01", "2032-01-01", "2026-11-23", false},
		{"expiry today", "2000-01-01", "2026-11-23", "2026-11-23", false},
		{"expired", "2000-01-01", "2026-11-22", "2026-11-23", false},
		{"leap before birthday", "2008-02-29", "2032-01-01", "2026-02-28", false},
		{"leap birthday", "2008-02-29", "2032-01-01", "2026-03-01", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			i := validIdentity()
			i.DateOfBirth, i.IDExpiryDate = tc.birth, tc.expiry
			now, _ := time.Parse("2006-01-02", tc.today)
			if err := ValidateIdentityDates(i, now); (err == nil) != tc.valid {
				t.Fatalf("valid=%t, error=%v", tc.valid, err)
			}
		})
	}
	// The local date is November 24, but the UTC date is November 23.
	i := validIdentity()
	i.DateOfBirth = "2008-11-24"
	now := time.Date(2026, 11, 24, 1, 0, 0, 0, time.FixedZone("ICT", 7*3600))
	if !errors.Is(ValidateIdentityDates(i, now), ErrInvalidIdentity) {
		t.Fatal("calendar must use UTC")
	}
}

func TestIdentityEquality(t *testing.T) {
	original := validIdentity()
	for _, change := range []func(*Identity){
		func(i *Identity) { i.IDNumber = "079123456788" },
		func(i *Identity) { i.FullName = strings.TrimSpace(i.FullName) },
		func(i *Identity) { i.DateOfBirth = "2000-02-28" },
		func(i *Identity) { i.IDExpiryDate = "2032-05-16" },
		func(i *Identity) { i.PhoneNumber += " " },
		func(i *Identity) { s := ""; i.Email = &s },
	} {
		other := original
		change(&other)
		if EqualIdentity(original, other) {
			t.Fatal("different identity compared equal")
		}
	}
	a, b := "Example@test", "Example@test"
	original.Email = &a
	other := original
	other.Email = &b
	other.CreatedAt = time.Now()
	if !EqualIdentity(original, other) {
		t.Fatal("value equality must ignore pointer addresses and timestamps")
	}
	b = "example@test"
	if EqualIdentity(original, other) {
		t.Fatal("email equality must be case sensitive")
	}
}

func TestIdentitySubmissionDecision(t *testing.T) {
	i := validIdentity()
	now := time.Date(2026, 11, 23, 0, 0, 0, 0, time.UTC)
	for _, status := range []ApplicationStatus{ApplicationStatusPendingTermsAcceptance, ApplicationStatusPendingIdentitySubmission,
		ApplicationStatusPendingWorkflowStart, ApplicationStatusPendingIdentityVerification, ApplicationStatusPendingSanctionsScreening,
		ApplicationStatusPendingHumanReview, ApplicationStatusApproved, ApplicationStatusRejected, ApplicationStatusCompleted,
		ApplicationStatusPendingAccountCreation, ApplicationStatusFailedTechnical, ""} {
		t.Run(string(status), func(t *testing.T) {
			replay, err := CheckIdentitySubmission(status, nil, i, now)
			if status == ApplicationStatusPendingIdentitySubmission {
				if err != nil || replay {
					t.Fatalf("first submission: %t, %v", replay, err)
				}
			} else if !errors.Is(err, ErrInvalidIdentityState) {
				t.Fatalf("expected invalid state, got %v", err)
			}
			replay, err = CheckIdentitySubmission(status, &i, i, now.AddDate(20, 0, 0))
			progressed := status.Valid() && status != ApplicationStatusPendingTermsAcceptance && status != ApplicationStatusPendingIdentitySubmission
			if progressed {
				if err != nil || !replay {
					t.Fatalf("durable replay: %t, %v", replay, err)
				}
				changed := i
				changed.FullName += "x"
				if _, err := CheckIdentitySubmission(status, &i, changed, now); !errors.Is(err, ErrConflictingIdentity) {
					t.Fatalf("expected conflict, got %v", err)
				}
			} else if !errors.Is(err, ErrInvalidIdentityState) {
				t.Fatalf("inconsistent state accepted: %v", err)
			}
		})
	}
	i.IDExpiryDate = "2026-11-23"
	if _, err := CheckIdentitySubmission(ApplicationStatusPendingIdentitySubmission, nil, i, now); !errors.Is(err, ErrInvalidIdentity) {
		t.Fatal("expired first submission accepted")
	}
}
