package xendit

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	DefaultBaseURL    = "https://api.xendit.co"
	DefaultAPIVersion = "2024-11-11"

	CreatePaymentRequestPath = "/v3/payment_requests"
)

type Config struct {
	BaseURL       string
	APIKey        string
	APIVersion    string
	WebhookToken  string
	HTTPClient    *http.Client
}

type Client struct {
	cfg        Config
	httpClient *http.Client
}

func NewClient(cfg Config) (*Client, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("xendit: api key is required")
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	if cfg.APIVersion == "" {
		cfg.APIVersion = DefaultAPIVersion
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	hc := cfg.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{cfg: cfg, httpClient: hc}, nil
}

// WebhookToken returns the configured callback verification token.
func (c *Client) WebhookToken() string { return c.cfg.WebhookToken }

func (c *Client) authHeader() string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(c.cfg.APIKey+":"))
}

func (c *Client) CreatePaymentRequest(ctx context.Context, req PaymentRequestCreate) (*PaymentRequest, error) {
	if req.Type == "" {
		req.Type = "PAY"
	}
	if req.Country == "" {
		req.Country = "ID"
	}
	if req.Currency == "" {
		req.Currency = "IDR"
	}
	var resp PaymentRequest
	raw, err := c.do(ctx, http.MethodPost, CreatePaymentRequestPath, req, &resp)
	if err != nil {
		return nil, err
	}
	resp.RawResponse = raw
	return &resp, nil
}

func (c *Client) GetPaymentRequest(ctx context.Context, paymentRequestID string) (*PaymentRequest, error) {
	path := CreatePaymentRequestPath + "/" + paymentRequestID
	var resp PaymentRequest
	raw, err := c.do(ctx, http.MethodGet, path, nil, &resp)
	if err != nil {
		return nil, err
	}
	resp.RawResponse = raw
	return &resp, nil
}

func (c *Client) CancelPaymentRequest(ctx context.Context, paymentRequestID string) (*PaymentRequest, error) {
	path := CreatePaymentRequestPath + "/" + paymentRequestID + "/cancel"
	var resp PaymentRequest
	raw, err := c.do(ctx, http.MethodPost, path, nil, &resp)
	if err != nil {
		return nil, err
	}
	resp.RawResponse = raw
	return &resp, nil
}

// CreateRefund issues a refund against a payment_request.
func (c *Client) CreateRefund(ctx context.Context, paymentRequestID string, req RefundCreate) (*Refund, error) {
	path := CreatePaymentRequestPath + "/" + paymentRequestID + "/refunds"
	var resp Refund
	raw, err := c.do(ctx, http.MethodPost, path, req, &resp)
	if err != nil {
		return nil, err
	}
	resp.RawResponse = raw
	return &resp, nil
}

// SimulatePayment triggers a mock payment in sandbox.
func (c *Client) SimulatePayment(ctx context.Context, paymentRequestID string) (json.RawMessage, error) {
	path := CreatePaymentRequestPath + "/" + paymentRequestID + "/payments/simulate"
	return c.do(ctx, http.MethodPost, path, nil, nil)
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any) (json.RawMessage, error) {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.cfg.BaseURL+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", c.authHeader())
	req.Header.Set("api-version", c.cfg.APIVersion)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= http.StatusBadRequest {
		apiErr := &APIError{HTTPStatus: resp.StatusCode, RawResponse: raw}
		_ = json.Unmarshal(raw, apiErr)
		return raw, apiErr
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return raw, fmt.Errorf("xendit: decode response: %w", err)
		}
	}
	return raw, nil
}
