package pakailink

import (
	"crypto/hmac"
	"crypto/subtle"
	"encoding/base64"
	"strings"
)

// VerifyWebhookSignature recomputes the SNAP symmetric signature over a
// webhook body and returns true when it matches the provided header value.
// The token parameter is typically "" for incoming provider webhooks (SNAP
// spec: webhook callers do not have an access token issued by us).
func VerifyWebhookSignature(method, path, token string, body []byte, timestamp, signature, clientSecret string) bool {
	if signature == "" || clientSecret == "" {
		return false
	}
	expected := signSymmetric(strings.ToUpper(method), path, token, body, timestamp, clientSecret)
	// Provider signs Base64; use constant-time compare on the decoded bytes so
	// whitespace/padding does not cause spurious mismatches.
	a, err := base64.StdEncoding.DecodeString(strings.TrimSpace(signature))
	if err != nil {
		return false
	}
	b, err := base64.StdEncoding.DecodeString(expected)
	if err != nil {
		return false
	}
	if subtle.ConstantTimeCompare(a, b) == 1 {
		return true
	}
	return hmac.Equal(a, b)
}
