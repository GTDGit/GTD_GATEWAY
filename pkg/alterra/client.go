package alterra

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

const (
	// ProductionBaseURL is the production API URL
	ProductionBaseURL = "https://horven-api.sumpahpalapa.com"
	// StagingBaseURL is the staging API URL
	StagingBaseURL = "https://horven-api-staging.sumpahpalapa.com"
)

// Config holds Alterra API configuration
type Config struct {
	BaseURL        string
	ClientID       string
	PrivateKeyPath string
	PrivateKeyPEM  string // Alternative: PEM string directly
}

// Client is the Alterra API client with RSA-SHA256 authentication
type Client struct {
	httpClient *http.Client
	config     Config
	privateKey *rsa.PrivateKey
	debug      bool
}

// NewClient creates a new Alterra client
func NewClient(config Config) (*Client, error) {
	client := &Client{
		httpClient: &http.Client{Timeout: 60 * time.Second},
		config:     config,
		debug:      os.Getenv("ENV") == "development",
	}

	// Load private key
	var keyPEM []byte
	var err error

	if config.PrivateKeyPEM != "" {
		keyPEM = []byte(config.PrivateKeyPEM)
	} else if config.PrivateKeyPath != "" {
		keyPEM, err = os.ReadFile(config.PrivateKeyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read private key: %w", err)
		}
	} else {
		return nil, fmt.Errorf("private key not provided")
	}

	// Parse private key
	block, _ := pem.Decode(keyPEM)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	// Try PKCS1 first, then PKCS8
	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		// Try PKCS8
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("failed to parse private key: %w", err)
		}
		var ok bool
		privateKey, ok = key.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("private key is not RSA")
		}
	}

	client.privateKey = privateKey
	return client, nil
}

// sign creates RSA-SHA256 signature of the data
func (c *Client) sign(data []byte) (string, error) {
	hash := sha256.Sum256(data)
	signature, err := rsa.SignPKCS1v15(rand.Reader, c.privateKey, crypto.SHA256, hash[:])
	if err != nil {
		return "", fmt.Errorf("failed to sign: %w", err)
	}
	return base64.StdEncoding.EncodeToString(signature), nil
}

func buildRequestURL(baseURL, path string) string {
	base := strings.TrimRight(baseURL, "/")
	if strings.HasSuffix(base, "/api") && strings.HasPrefix(path, "/api/") {
		base = strings.TrimSuffix(base, "/api")
	}
	return base + path
}

func cloneRawJSON(body []byte) json.RawMessage {
	if len(body) == 0 {
		return nil
	}
	raw := make([]byte, len(body))
	copy(raw, body)
	return json.RawMessage(raw)
}

func attachRawResponse(result any, status int, body []byte) {
	switch v := result.(type) {
	case *TransactionResponse:
		v.RawResponse = cloneRawJSON(body)
		v.HTTPStatus = status
	case *TransactionDetailResponse:
		v.RawResponse = cloneRawJSON(body)
		v.HTTPStatus = status
	}
}

// doRequest performs a request with RSA-SHA256 authentication
func (c *Client) doRequest(ctx context.Context, method, path string, body any, result any) error {
	url := buildRequestURL(c.config.BaseURL, path)

	var bodyBytes []byte
	var err error

	if body != nil {
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed to marshal request: %w", err)
		}
	} else {
		bodyBytes = []byte{}
	}

	// Create signature
	signature, err := c.sign(bodyBytes)
	if err != nil {
		return err
	}

	// Create timestamp
	timestamp := time.Now().Format(time.RFC3339)

	if c.debug {
		log.Debug().
			Str("method", method).
			Str("endpoint", url).
			RawJSON("request", bodyBytes).
			Msg("[ALTERRA] Outgoing request")
	}

	var req *http.Request
	if method == http.MethodGet {
		req, err = http.NewRequestWithContext(ctx, method, url, nil)
	} else {
		req, err = http.NewRequestWithContext(ctx, method, url, bytes.NewReader(bodyBytes))
	}
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Client-ID", c.config.ClientID)
	req.Header.Set("X-Client-Signature", signature)
	req.Header.Set("X-Client-Timestamp", timestamp)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if c.debug {
		log.Debug().
			Str("endpoint", path).
			Int("status_code", resp.StatusCode).
			RawJSON("response", respBody).
			Msg("[ALTERRA] Incoming response")
	}

	// Check for HTTP errors
	if resp.StatusCode >= 400 {
		// For 4xx errors, try to parse as transaction response first
		// (Alterra dummy biller uses HTTP status codes for business errors)
		if resp.StatusCode < 500 {
			if err := json.Unmarshal(respBody, result); err == nil {
				attachRawResponse(result, resp.StatusCode, respBody)
				return nil // Parsed successfully as business response
			}
		}
		var errResp ErrorResponse
		if err := json.Unmarshal(respBody, &errResp); err == nil && errResp.Error.Message != "" {
			switch v := result.(type) {
			case *TransactionResponse:
				v.Error = &errResp.Error
				attachRawResponse(v, resp.StatusCode, respBody)
				return nil
			}
			return fmt.Errorf("api error: %s (code: %s)", errResp.Error.Message, errResp.Error.Code)
		}
		return fmt.Errorf("http error: %d", resp.StatusCode)
	}

	if err := json.Unmarshal(respBody, result); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}
	attachRawResponse(result, resp.StatusCode, respBody)

	return nil
}
