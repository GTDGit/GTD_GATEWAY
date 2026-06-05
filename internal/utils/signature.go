package utils

import (
    "crypto/hmac"
    "crypto/sha256"
    "encoding/hex"
)

// GenerateSignature creates HMAC-SHA256 signature
func GenerateSignature(payload []byte, secret string) string {
    mac := hmac.New(sha256.New, []byte(secret))
    mac.Write(payload)
    return hex.EncodeToString(mac.Sum(nil))
}

// VerifySignature validates HMAC-SHA256 signature
func VerifySignature(payload []byte, signature, secret string) bool {
    expected := GenerateSignature(payload, secret)
    return hmac.Equal([]byte(signature), []byte(expected))
}
