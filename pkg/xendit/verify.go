package xendit

import (
	"crypto/hmac"
	"strings"
)

// VerifyWebhookToken compares the x-callback-token header against the configured
// webhook token using constant-time comparison.
func VerifyWebhookToken(providedToken, configuredToken string) bool {
	if providedToken == "" || configuredToken == "" {
		return false
	}
	return hmac.Equal(
		[]byte(strings.TrimSpace(providedToken)),
		[]byte(strings.TrimSpace(configuredToken)),
	)
}
