package service

// provider_helpers.go contains small utility functions that support provider
// routing, logging, and payment deduplication.  These were previously inlined
// inside individual provider client files; they are collected here so that the
// rest of the service package continues to compile even though no live provider
// clients are wired up in the Gateway (admin-only mode).

import (
	"crypto/rand"
	"errors"
	"math/big"
	"strings"

	"github.com/lib/pq"
)

// ---------------------------------------------------------------------------
// Map helpers
// ---------------------------------------------------------------------------

// cloneAnyMap returns a shallow copy of a map[string]any.  A nil/empty source
// returns an empty (non-nil) map so callers can safely write into it.
func cloneAnyMap(src map[string]any) map[string]any {
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// mergeAnyMap copies all entries from src into dst in-place.
func mergeAnyMap(dst, src map[string]any) {
	for k, v := range src {
		dst[k] = v
	}
}

// ---------------------------------------------------------------------------
// Kiosbank request normaliser
// ---------------------------------------------------------------------------

// normalizeKiosbankRequestData canonicalises key names in the Kiosbank extra
// map (e.g. noHandphone → noHanphone).
func normalizeKiosbankRequestData(data map[string]any) map[string]any {
	if data == nil {
		return map[string]any{}
	}
	normalised := cloneAnyMap(data)
	// Kiosbank uses "noHanphone" (one 'd') in its wire format.
	if v, ok := normalised["noHandphone"]; ok {
		if _, exists := normalised["noHanphone"]; !exists {
			normalised["noHanphone"] = v
		}
		delete(normalised, "noHandphone")
	}
	return normalised
}

// ---------------------------------------------------------------------------
// Alterra payment-data helper
// ---------------------------------------------------------------------------

// buildAlterraPaymentData constructs the data payload for an Alterra payment
// (postpaid) call from the inquiry-carried extra map.
func buildAlterraPaymentData(extra map[string]any) map[string]any {
	paymentData := make(map[string]any)
	if len(extra) == 0 {
		return paymentData
	}
	// Forward reference_no and payment_period if present; strip internal keys.
	for _, key := range []string{"reference_no", "refNumber", "payment_period", "payment_amount"} {
		if v, ok := extra[key]; ok {
			paymentData[key] = v
		}
	}
	return paymentData
}

// ---------------------------------------------------------------------------
// stringFromKeys / intValueOK
// ---------------------------------------------------------------------------

// stringFromKeys returns the first non-empty string value found for any of the
// provided keys in the map.
func stringFromKeys(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if v, ok := m[key]; ok {
			switch typed := v.(type) {
			case string:
				if s := strings.TrimSpace(typed); s != "" {
					return s
				}
			}
		}
	}
	return ""
}

// intValueOK coerces a map value to int, supporting int, int64, float64, and
// string representations.
func intValueOK(v any) (int, bool) {
	switch value := v.(type) {
	case int:
		return value, true
	case int64:
		return int(value), true
	case float64:
		return int(value), true
	case string:
		var n int
		if _, err := parseIntString(value, &n); err == nil {
			return n, true
		}
	}
	return 0, false
}

func parseIntString(s string, out *int) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("empty string")
	}
	n := 0
	neg := false
	start := 0
	if s[0] == '-' {
		neg = true
		start = 1
	}
	for i := start; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0, errors.New("not a number")
		}
		n = n*10 + int(c-'0')
	}
	if neg {
		n = -n
	}
	if out != nil {
		*out = n
	}
	return n, nil
}

// ---------------------------------------------------------------------------
// Payment deduplication helpers (used by payment_service.go)
// ---------------------------------------------------------------------------

// isUniqueViolation reports whether err is a PostgreSQL unique-constraint
// violation (SQLSTATE 23505).
func isUniqueViolation(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == "23505"
}

// isReferenceUniqueViolation reports whether the unique-constraint violation
// is specifically on a reference_id column.
func isReferenceUniqueViolation(err error) bool {
	var pqErr *pq.Error
	if !errors.As(err, &pqErr) {
		return false
	}
	if pqErr.Code != "23505" {
		return false
	}
	return strings.Contains(strings.ToLower(pqErr.Constraint), "reference_id") ||
		strings.Contains(strings.ToLower(pqErr.Detail), "reference_id")
}

// randomDigits returns a cryptographically random non-negative integer with at
// most 'length' decimal digits (useful for generating payment / transfer IDs).
func randomDigits(length int) int64 {
	if length <= 0 {
		return 0
	}
	max := int64(1)
	for i := 0; i < length; i++ {
		max *= 10
	}
	n, err := rand.Int(rand.Reader, big.NewInt(max))
	if err != nil {
		return 0
	}
	return n.Int64()
}
