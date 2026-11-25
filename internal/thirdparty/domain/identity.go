package domain

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

var ErrInvalidIdentity = errors.New("identity information is invalid")

// Identity contains submitted values without transport or persistence concerns.
type Identity struct {
	IDNumber     string
	FullName     string
	DateOfBirth  string
	IDExpiryDate string
	PhoneNumber  string
	Email        *string
}

// ValidateIdentityStructure preserves input and leaves age/expiry eligibility to Main BE.
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
	for _, value := range []string{i.DateOfBirth, i.IDExpiryDate} {
		date, err := time.Parse("2006-01-02", value)
		if err != nil || date.Year() < 1 || date.Format("2006-01-02") != value {
			return ErrInvalidIdentity
		}
	}
	return nil
}
