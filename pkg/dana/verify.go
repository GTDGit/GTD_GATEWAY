package dana

import (
	"crypto/hmac"
	"crypto/subtle"
	"encoding/base64"
	"strings"
)

// VerifyWebhookSignature recomputes the SNAP symmetric signature used by
// DANA payment notifications. Use method=POST, path="/v1/webhook/dana", and
// token="" (DANA does not require a bearer token on inbound notifications).
func VerifyWebhookSignature(method, path, token string, body []byte, timestamp, signature, clientSecret string) bool {
	if signature == "" || clientSecret == "" {
		return false
	}
	expected := signSymmetric(strings.ToUpper(method), path, token, body, timestamp, clientSecret)
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
