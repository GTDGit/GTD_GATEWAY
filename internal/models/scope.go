package models

import "slices"

// API key scope identifiers. Each route group is gated by exactly one scope.
const (
	ScopePPOB         = "ppob"
	ScopePayment      = "payment"
	ScopeDisbursement = "disbursement"
)

// AllScopes lists all known scope identifiers in canonical order.
// Used as the default for newly created clients (preserves pre-scope behavior)
// and for admin-input validation.
var AllScopes = []string{ScopePPOB, ScopePayment, ScopeDisbursement}

// IsValidScope reports whether s is one of the known scope identifiers.
func IsValidScope(s string) bool {
	return slices.Contains(AllScopes, s)
}
