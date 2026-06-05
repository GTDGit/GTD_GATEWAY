package kiosbank

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

var (
	refMu   sync.Mutex
	lastRef int64
)

// GenerateReferenceID returns a monotonic 12-digit numeric reference.
func GenerateReferenceID() string {
	refMu.Lock()
	defer refMu.Unlock()

	now := time.Now().UnixMicro() % 1_000_000_000_000
	if now <= lastRef {
		now = lastRef + 1
	}
	if now >= 1_000_000_000_000 {
		now = now % 1_000_000_000_000
		if now == 0 {
			now = 1
		}
	}

	lastRef = now
	return fmt.Sprintf("%012d", now)
}

// NormalizeReferenceID keeps numeric characters only and pads/truncates to 12 digits.
func NormalizeReferenceID(refID string) string {
	var digits strings.Builder
	for _, r := range refID {
		if r >= '0' && r <= '9' {
			digits.WriteRune(r)
		}
	}
	s := digits.String()
	if len(s) > 12 {
		return s[:12]
	}
	return strings.Repeat("0", 12-len(s)) + s
}

// IsNumericReferenceID returns true if the reference is already exactly 12 numeric digits.
func IsNumericReferenceID(refID string) bool {
	if len(refID) != 12 {
		return false
	}
	for _, r := range refID {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
