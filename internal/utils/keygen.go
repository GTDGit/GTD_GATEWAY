package utils

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// GenerateAPIKey generates a random API key with the given prefix.
// Format: prefix_randomhex
// Example: gb_live_a1b2c3d4e5f6...
func GenerateAPIKey(prefix string) (string, error) {
	b := make([]byte, 32) // 64 char hex
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s_%s", prefix, hex.EncodeToString(b)), nil
}

// GenerateLiveKey generates a live API key: gb_live_xxx
func GenerateLiveKey() (string, error) {
	return GenerateAPIKey("gb_live")
}

// GenerateSandboxKey generates a sandbox API key: gb_sandbox_xxx
func GenerateSandboxKey() (string, error) {
	return GenerateAPIKey("gb_sandbox")
}

// GenerateWebhookSecret generates a webhook secret: gb_secret_xxx
func GenerateWebhookSecret() (string, error) {
	return GenerateAPIKey("gb_secret")
}

// GenerateRandomString generates a random alphanumeric string of specified length
func GenerateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	for i := range b {
		b[i] = charset[int(b[i])%len(charset)]
	}
	return string(b)
}
