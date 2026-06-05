package midtrans

import (
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"strings"
)

// VerifyWebhookSignature recomputes the signature_key per Midtrans docs:
// SHA512(order_id + status_code + gross_amount + server_key).
func VerifyWebhookSignature(orderID, statusCode, grossAmount, serverKey, providedSignature string) bool {
	if providedSignature == "" || serverKey == "" {
		return false
	}
	payload := orderID + statusCode + grossAmount + serverKey
	sum := sha512.Sum512([]byte(payload))
	expected := hex.EncodeToString(sum[:])
	return hmac.Equal([]byte(strings.ToLower(strings.TrimSpace(providedSignature))), []byte(expected))
}
