package kiosbank

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// Config holds Kiosbank API configuration
type Config struct {
	BaseURL            string
	MerchantID         string
	MerchantName       string
	CounterID          string
	AccountID          string
	Mitra              string
	Username           string
	Password           string
	InsecureSkipVerify bool
}

// TransportError represents an uncertain provider transport result after the
// authenticated request was sent to Kiosbank.
type TransportError struct {
	Err error
}

func (e *TransportError) Error() string {
	if e == nil || e.Err == nil {
		return "kiosbank transport error"
	}
	return e.Err.Error()
}

func (e *TransportError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// IsTransportUncertainError returns true when the request result is ambiguous
// and the transaction should be treated as pending instead of definitively
// failed.
func IsTransportUncertainError(err error) bool {
	var transportErr *TransportError
	return errors.As(err, &transportErr)
}

// Client is the Kiosbank API client with HTTP Digest authentication
type Client struct {
	httpClient *http.Client
	config     Config
	sessionID  string
	sessionMu  sync.RWMutex
	sessionExp time.Time
	debug      bool
}

// NewClient creates a new Kiosbank client
func NewClient(config Config) *Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if config.InsecureSkipVerify {
		if transport.TLSClientConfig == nil {
			transport.TLSClientConfig = &tls.Config{}
		}
		transport.TLSClientConfig.InsecureSkipVerify = true
		log.Warn().
			Str("base_url", config.BaseURL).
			Msg("Kiosbank TLS verification disabled for this client")
	}

	return &Client{
		httpClient: &http.Client{
			Timeout:   60 * time.Second,
			Transport: transport,
		},
		config: config,
		debug:  os.Getenv("ENV") == "development",
	}
}

// digestAuth handles HTTP Digest Authentication
func (c *Client) digestAuth(ctx context.Context, method, uri string, body []byte) (*http.Response, error) {
	url := c.config.BaseURL + uri

	// First request to get nonce
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	// If not 401, return response
	if resp.StatusCode != http.StatusUnauthorized {
		return resp, nil
	}
	resp.Body.Close()

	// Parse WWW-Authenticate header
	authHeader := resp.Header.Get("WWW-Authenticate")
	if authHeader == "" {
		return nil, fmt.Errorf("no WWW-Authenticate header in 401 response")
	}

	authParams := parseDigestAuth(authHeader)
	nonce := authParams["nonce"]
	realm := authParams["realm"]
	qop := authParams["qop"]
	opaque := authParams["opaque"]

	// Generate client nonce
	cnonce := generateCNonce()
	nc := "00000001"

	// Calculate response hash
	ha1 := md5Hash(fmt.Sprintf("%s:%s:%s", c.config.Username, realm, c.config.Password))
	ha2 := md5Hash(fmt.Sprintf("%s:%s", method, uri))

	var response string
	if qop != "" {
		response = md5Hash(fmt.Sprintf("%s:%s:%s:%s:%s:%s", ha1, nonce, nc, cnonce, qop, ha2))
	} else {
		response = md5Hash(fmt.Sprintf("%s:%s:%s", ha1, nonce, ha2))
	}

	// Build authorization header
	authValue := fmt.Sprintf(`Digest username="%s", realm="%s", nonce="%s", uri="%s", response="%s"`,
		c.config.Username, realm, nonce, uri, response)

	if qop != "" {
		authValue += fmt.Sprintf(`, qop=%s, nc=%s, cnonce="%s"`, qop, nc, cnonce)
	}
	if opaque != "" {
		authValue += fmt.Sprintf(`, opaque="%s"`, opaque)
	}

	// Second request with auth
	req2, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Authorization", authValue)

	return c.httpClient.Do(req2)
}

// parseDigestAuth parses the WWW-Authenticate header
func parseDigestAuth(header string) map[string]string {
	result := make(map[string]string)
	header = strings.TrimPrefix(header, "Digest ")

	parts := strings.Split(header, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		idx := strings.Index(part, "=")
		if idx == -1 {
			continue
		}
		key := strings.TrimSpace(part[:idx])
		value := strings.Trim(strings.TrimSpace(part[idx+1:]), `"`)
		result[key] = value
	}
	return result
}

// generateCNonce generates a random client nonce
func generateCNonce() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// md5Hash computes MD5 hash and returns hex string
func md5Hash(data string) string {
	sum := md5.Sum([]byte(data))
	return hex.EncodeToString(sum[:])
}

// maxResponseSize is the maximum allowed response body size (10MB)
const maxResponseSize = 10 * 1024 * 1024

// doRequest performs a request with digest auth
func (c *Client) doRequest(ctx context.Context, uri string, body any, result any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	if c.debug {
		// Sanitize sensitive data before logging
		sanitized := sanitizeForLog(payload)
		log.Debug().
			Str("endpoint", c.config.BaseURL+uri).
			RawJSON("request", sanitized).
			Msg("[KIOSBANK] Outgoing request")
	}

	resp, err := c.digestAuth(ctx, http.MethodPost, uri, payload)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Limit response body size to prevent OOM
	limitedReader := io.LimitReader(resp.Body, maxResponseSize)
	respBody, err := io.ReadAll(limitedReader)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	// Check HTTP status code
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected HTTP status %d: %s", resp.StatusCode, string(respBody))
	}

	if c.debug {
		// Sanitize response before logging
		sanitized := sanitizeForLog(respBody)
		log.Debug().
			Str("endpoint", uri).
			Int("status_code", resp.StatusCode).
			RawJSON("response", sanitized).
			Msg("[KIOSBANK] Incoming response")
	}

	if err := json.Unmarshal(respBody, result); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	return nil
}

// sanitizeForLog removes or masks sensitive fields from JSON for logging
func sanitizeForLog(data []byte) []byte {
	var obj map[string]any
	if err := json.Unmarshal(data, &obj); err != nil {
		return []byte(`{"_error": "failed to parse for sanitization"}`)
	}

	// List of sensitive field names to mask
	sensitiveFields := []string{"password", "pin", "pwd", "secret", "token", "sessionid", "session_id"}

	sanitizeMap(obj, sensitiveFields)

	sanitized, err := json.Marshal(obj)
	if err != nil {
		return []byte(`{"_error": "failed to marshal sanitized data"}`)
	}
	return sanitized
}

// sanitizeMap recursively masks sensitive fields in a map
func sanitizeMap(obj map[string]any, sensitiveFields []string) {
	for key, value := range obj {
		keyLower := strings.ToLower(key)
		for _, sensitive := range sensitiveFields {
			if strings.Contains(keyLower, sensitive) {
				obj[key] = "***MASKED***"
				break
			}
		}
		// Recursively handle nested maps
		if nested, ok := value.(map[string]any); ok {
			sanitizeMap(nested, sensitiveFields)
		}
	}
}

func sessionExpiry(now time.Time) time.Time {
	wib := time.FixedZone("WIB", 7*3600)
	current := now.In(wib)
	return time.Date(current.Year(), current.Month(), current.Day(), 23, 59, 59, 0, wib)
}

func (c *Client) clearSession() {
	c.sessionID = ""
	c.sessionExp = time.Time{}
}

// ensureSession ensures we have a valid session
func (c *Client) ensureSession(ctx context.Context) (string, error) {
	c.sessionMu.RLock()
	if c.sessionID != "" && time.Now().Before(c.sessionExp) {
		sessionID := c.sessionID
		c.sessionMu.RUnlock()
		return sessionID, nil
	}
	c.sessionMu.RUnlock()

	// Need new session
	c.sessionMu.Lock()
	defer c.sessionMu.Unlock()

	// Double check
	if c.sessionID != "" && time.Now().Before(c.sessionExp) {
		return c.sessionID, nil
	}

	// Sign on
	resp, err := c.SignOn(ctx)
	if err != nil {
		return "", err
	}

	if resp.SessionID == "" {
		if resp.Description != "" {
			return "", fmt.Errorf("sign on failed: %s", resp.Description)
		}
		if resp.RC != "" && resp.RC != RCSuccess {
			return "", fmt.Errorf("sign on failed: rc %s", resp.RC)
		}
		return "", fmt.Errorf("sign on failed: missing SessionID")
	}

	if resp.RC != "" && resp.RC != RCSuccess {
		if resp.Description != "" {
			return "", fmt.Errorf("sign on failed: %s", resp.Description)
		}
		return "", fmt.Errorf("sign on failed: rc %s", resp.RC)
	}

	c.sessionID = resp.SessionID
	c.sessionExp = sessionExpiry(time.Now())

	return c.sessionID, nil
}

// formatAmount formats amount to 12 digit string with leading zeros
func formatAmount(amount int) string {
	return fmt.Sprintf("%012d", amount)
}

// formatReferenceID formats reference ID to 12 digits with zero padding
func formatReferenceID(refID string) string {
	return NormalizeReferenceID(refID)
}
