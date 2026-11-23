package domain

import "errors"

var (
	ErrConflictingTerms  = errors.New("a different term has already been accepted")
	ErrInvalidTermsState = errors.New("application is not in a valid state for terms acceptance")
)
