package midtrans

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
	ChargePath = "/v2/charge"

	DefaultSandboxURL = "https://api.sandbox.midtrans.com"
	DefaultProdURL    = "https://api.midtrans.com"
)

type Config struct {
	BaseURL    string
	ServerKey  string
	ClientKey  string
	MerchantID string
	HTTPClient *http.Client
}

type Client struct {
	cfg        Config
	httpClient *http.Client
}

func NewClient(cfg Config) (*Client, error) {
	if cfg.ServerKey == "" {
		return nil, fmt.Errorf("midtrans: server key is required")
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultSandboxURL
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	hc := cfg.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{cfg: cfg, httpClient: hc}, nil
}

func (c *Client) ServerKey() string { return c.cfg.ServerKey }

func (c *Client) authHeader() string {
	raw := c.cfg.ServerKey + ":"
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(raw))
}

// Charge accepts any ChargeRequest (GoPay or ShopeePay) and returns the normalized response.
func (c *Client) Charge(ctx context.Context, req ChargeRequest) (*ChargeResponse, error) {
	var resp ChargeResponse
	raw, err := c.do(ctx, http.MethodPost, ChargePath, req, &resp)
	if err != nil {
		return nil, err
	}
	resp.RawResponse = raw
	return &resp, nil
}

// ChargeGoPay builds a GoPay charge request with sensible defaults.
func (c *Client) ChargeGoPay(ctx context.Context, orderID string, grossAmount int64, callbackURL string, customer *CustomerDetails) (*ChargeResponse, error) {
	req := ChargeRequest{
		PaymentType: PaymentTypeGoPay,
		TransactionDetails: TransactionDetails{
			OrderID:     orderID,
			GrossAmount: grossAmount,
		},
		GoPay: &GoPayOptions{
			EnableCallback: true,
			CallbackURL:    callbackURL,
		},
		CustomerDetails: customer,
	}
	return c.Charge(ctx, req)
}

// ChargeShopeePay builds a ShopeePay charge request with sensible defaults.
func (c *Client) ChargeShopeePay(ctx context.Context, orderID string, grossAmount int64, callbackURL string, customer *CustomerDetails) (*ChargeResponse, error) {
	req := ChargeRequest{
		PaymentType: PaymentTypeShopeePay,
		TransactionDetails: TransactionDetails{
			OrderID:     orderID,
			GrossAmount: grossAmount,
		},
		ShopeePay:       &ShopeePayOptions{CallbackURL: callbackURL},
		CustomerDetails: customer,
	}
	return c.Charge(ctx, req)
}

// Status performs GET /v2/{order_id}/status.
func (c *Client) Status(ctx context.Context, orderID string) (*StatusResponse, error) {
	path := "/v2/" + orderID + "/status"
	var resp StatusResponse
	raw, err := c.do(ctx, http.MethodGet, path, nil, &resp)
	if err != nil {
		return nil, err
	}
	resp.RawResponse = raw
	return &resp, nil
}

// Cancel performs POST /v2/{order_id}/cancel.
func (c *Client) Cancel(ctx context.Context, orderID string) (*CancelResponse, error) {
	path := "/v2/" + orderID + "/cancel"
	var resp CancelResponse
	raw, err := c.do(ctx, http.MethodPost, path, nil, &resp)
	if err != nil {
		return nil, err
	}
	resp.RawResponse = raw
	return &resp, nil
}

// Refund performs POST /v2/{order_id}/refund.
func (c *Client) Refund(ctx context.Context, orderID string, req RefundRequest) (*RefundResponse, error) {
	path := "/v2/" + orderID + "/refund"
	var resp RefundResponse
	raw, err := c.do(ctx, http.MethodPost, path, req, &resp)
	if err != nil {
		return nil, err
	}
	resp.RawResponse = raw
	return &resp, nil
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

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return raw, fmt.Errorf("midtrans: decode response: %w", err)
		}
	}

	code, msg := extractStatus(raw)
	// Midtrans uses 200/201/407 successfully; anything 400+ except 407 is an error.
	if resp.StatusCode >= http.StatusBadRequest && resp.StatusCode != 407 {
		return raw, &APIError{
			HTTPStatus:    resp.StatusCode,
			StatusCode:    code,
			StatusMessage: msg,
			RawResponse:   raw,
		}
	}
	return raw, nil
}

func extractStatus(raw json.RawMessage) (string, string) {
	var s struct {
		StatusCode    string `json:"status_code"`
		StatusMessage string `json:"status_message"`
	}
	_ = json.Unmarshal(raw, &s)
	return s.StatusCode, s.StatusMessage
}
