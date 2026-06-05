package dana

import (
	"crypto"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"os"
	"strings"
	"time"
)

// signAsymmetric returns Base64(SHA256withRSA(clientId + "|" + ts, privateKey)).
func signAsymmetric(clientID, timestamp string, key *rsa.PrivateKey) (string, error) {
	payload := clientID + "|" + timestamp
	hashed := sha256.Sum256([]byte(payload))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, hashed[:])
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(sig), nil
}

// signSymmetric returns Base64(HMAC-SHA512(<method>:<path>:<token>:<hex(sha256(body))>:<ts>, secret)).
func signSymmetric(method, path, accessToken string, body []byte, timestamp, clientSecret string) string {
	bodyHash := sha256.Sum256(minifyJSON(body))
	stringToSign := strings.Join([]string{
		method,
		path,
		accessToken,
		strings.ToLower(hex.EncodeToString(bodyHash[:])),
		timestamp,
	}, ":")
	mac := hmac.New(sha512.New, []byte(clientSecret))
	mac.Write([]byte(stringToSign))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func minifyJSON(body []byte) []byte {
	if len(body) == 0 {
		return body
	}
	var out strings.Builder
	inString := false
	escape := false
	for _, b := range body {
		if escape {
			out.WriteByte(b)
			escape = false
			continue
		}
		if b == '\\' {
			out.WriteByte(b)
			escape = true
			continue
		}
		if b == '"' {
			inString = !inString
			out.WriteByte(b)
			continue
		}
		if !inString && (b == ' ' || b == '\n' || b == '\r' || b == '\t') {
			continue
		}
		out.WriteByte(b)
	}
	return []byte(out.String())
}

func formatTimestamp(t time.Time) string {
	wib := time.FixedZone("WIB", 7*3600)
	return t.In(wib).Format("2006-01-02T15:04:05+07:00")
}

func loadPrivateKey(path, pemData string) (*rsa.PrivateKey, error) {
	var raw []byte
	if pemData != "" {
		raw = []byte(pemData)
	} else if path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		raw = b
	} else {
		return nil, fmt.Errorf("dana: private key path or PEM required")
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, fmt.Errorf("dana: failed to decode PEM block")
	}
	if k, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return k, nil
	}
	any, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("dana: parse private key: %w", err)
	}
	k, ok := any.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("dana: private key is not RSA")
	}
	return k, nil
}
