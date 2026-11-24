package domain

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

var (
	ErrInvalidIdentity      = errors.New("identity information is invalid")
	ErrConflictingIdentity  = errors.New("a different identity has already been submitted")
	ErrInvalidIdentityState = errors.New("application is not in a valid state for identity submission")
)

// Identity preserves submitted values and optional email presence independently of transport.
type Identity struct {
	ApplicationID uuid.UUID
	IDNumber      string
	FullName      string
	DateOfBirth   string
	IDExpiryDate  string
	PhoneNumber   string
	Email         *string
	CreatedAt     time.Time
}

func identityDate(value string) (time.Time, error) {
	d, err := time.Parse("2006-01-02", value)
	if err != nil || d.Year() < 1 || d.Format("2006-01-02") != value {
		return time.Time{}, ErrInvalidIdentity
	}
	return d, nil
}

// ValidateIdentityStructure is safe before looking up a possible durable replay.
func ValidateIdentityStructure(i *Identity) error {
	if i == nil || len(i.IDNumber) != 12 || strings.TrimSpace(i.FullName) == "" ||
		strings.TrimSpace(i.PhoneNumber) == "" || utf8.RuneCountInString(i.PhoneNumber) > 20 {
		return ErrInvalidIdentity
	}
	for _, digit := range i.IDNumber {
		if digit < '0' || digit > '9' {
			return ErrInvalidIdentity
		}
	}
	fields := []string{i.IDNumber, i.FullName, i.DateOfBirth, i.IDExpiryDate, i.PhoneNumber}
	if i.Email != nil {
		fields = append(fields, *i.Email)
	}
	for _, field := range fields {
		if !utf8.ValidString(field) || strings.ContainsRune(field, '\x00') {
			return ErrInvalidIdentity
		}
	}
	if _, err := identityDate(i.DateOfBirth); err != nil {
		return err
	}
	_, err := identityDate(i.IDExpiryDate)
	return err
}

// ValidateIdentityDates uses UTC calendar dates; February 29 birthdays advance on
// March 1 in non-leap years, as time.Date normalizes the anniversary.
func ValidateIdentityDates(i Identity, now time.Time) error {
	birth, err := identityDate(i.DateOfBirth)
	if err != nil {
		return err
	}
	expiry, err := identityDate(i.IDExpiryDate)
	if err != nil {
		return err
	}
	now = now.UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	adult := time.Date(birth.Year()+18, birth.Month(), birth.Day(), 0, 0, 0, 0, time.UTC)
	if now.IsZero() || adult.After(today) || !expiry.After(today) {
		return ErrInvalidIdentity
	}
	return nil
}

// EqualIdentity compares submitted fields only, including optional email presence.
func EqualIdentity(a, b Identity) bool {
	return a.IDNumber == b.IDNumber && a.FullName == b.FullName && a.DateOfBirth == b.DateOfBirth &&
		a.IDExpiryDate == b.IDExpiryDate && a.PhoneNumber == b.PhoneNumber &&
		((a.Email == nil && b.Email == nil) || (a.Email != nil && b.Email != nil && *a.Email == *b.Email))
}

// CheckIdentitySubmission must use application and identity state read under the
// same parent-row lock. A replay never re-evaluates time-dependent eligibility.
func CheckIdentitySubmission(status ApplicationStatus, existing *Identity, incoming Identity, now time.Time) (bool, error) {
	if !status.Valid() || (existing != nil && (status == ApplicationStatusPendingTermsAcceptance || status == ApplicationStatusPendingIdentitySubmission)) {
		return false, ErrInvalidIdentityState
	}
	if existing != nil {
		if !EqualIdentity(*existing, incoming) {
			return false, ErrConflictingIdentity
		}
		return true, nil
	}
	if status != ApplicationStatusPendingIdentitySubmission {
		return false, ErrInvalidIdentityState
	}
	return false, ValidateIdentityDates(incoming, now)
}
