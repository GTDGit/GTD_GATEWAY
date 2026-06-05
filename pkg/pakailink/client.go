package pakailink

import (
	"bytes"
	"context"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	TokenPath          = "/snap/v1.0/access-token/b2b"
	CreateVAPath       = "/snap/v1.0/transfer-va/create-va"
	InquiryVAPath      = "/snap/v1.0/transfer-va/create-va-status"
	DeleteVAPath       = "/snap/v1.0/transfer-va/delete-va"
	GenerateQRPath     = "/snap/v1.0/qr/qr-mpm-generate"
	InquiryQRPath      = "/snap/v1.0/qr/qr-mpm-query"

	DefaultChannelID = "95221"

	StatusSuccess   = "00"
	StatusInitiated = "01"
	StatusPaying    = "02"
	StatusCancelled = "05"
	StatusFailed    = "06"
)

type Config struct {
	BaseURL        string
	ClientID       string
	ClientSecret   string
	PartnerID      string
	ChannelID      string
	PrivateKeyPath string
	PrivateKeyPEM  string
	HTTPClient     *http.Client
}

type Client struct {
	cfg        Config
	privateKey *rsa.PrivateKey
	httpClient *http.Client

	mu           sync.Mutex
	accessToken  string
	tokenExpires time.Time
}

func NewClient(cfg Config) (*Client, error) {
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("pakailink: base URL is required")
	}
	if cfg.ClientID == "" || cfg.ClientSecret == "" {
		return nil, fmt.Errorf("pakailink: clientID and clientSecret are required")
	}
	if cfg.PartnerID == "" {
		cfg.PartnerID = cfg.ClientID
	}
	if cfg.ChannelID == "" {
		cfg.ChannelID = DefaultChannelID
	}
	key, err := loadPrivateKey(cfg.PrivateKeyPath, cfg.PrivateKeyPEM)
	if err != nil {
		return nil, err
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{cfg: cfg, privateKey: key, httpClient: httpClient}, nil
}

// ClientSecret exposes the configured secret so webhook verification helpers
// can reuse it without reaching into the struct directly.
func (c *Client) ClientSecret() string { return c.cfg.ClientSecret }

// GetAccessToken returns a cached token or mints a new one via SNAP B2B.
func (c *Client) GetAccessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.accessToken != "" && time.Now().Before(c.tokenExpires) {
		return c.accessToken, nil
	}

	ts := formatTimestamp(time.Now())
	sig, err := signAsymmetric(c.cfg.ClientID, ts, c.privateKey)
	if err != nil {
		return "", err
	}

	body, _ := json.Marshal(map[string]string{"grantType": "client_credentials"})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.BaseURL+TokenPath, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-TIMESTAMP", ts)
	req.Header.Set("X-CLIENT-KEY", c.cfg.ClientID)
	req.Header.Set("X-SIGNATURE", sig)

	var tr tokenResponse
	if _, err := c.doRequest(req, &tr); err != nil {
		return "", err
	}

	expiresIn, _ := strconv.Atoi(tr.ExpiresIn)
	if expiresIn <= 0 {
		expiresIn = 900
	}
	c.accessToken = tr.AccessToken
	c.tokenExpires = time.Now().Add(time.Duration(expiresIn-60) * time.Second)
	return c.accessToken, nil
}

// CreateVA provisions a dynamic VA per SNAP BI /transfer-va/create-va.
func (c *Client) CreateVA(ctx context.Context, req CreateVARequest) (*CreateVAResponse, error) {
	body := map[string]any{
		"partnerReferenceNo": req.PartnerReferenceNo,
		"customerNo":         req.CustomerNo,
		"virtualAccountName": req.VirtualAccountName,
		"totalAmount": Amount{
			Value:    formatAmount(req.TotalAmount),
			Currency: "IDR",
		},
		"additionalInfo": map[string]any{
			"bankCode":    req.BankCode,
			"callbackUrl": req.CallbackURL,
		},
	}
	if req.VirtualAccountPhone != "" {
		body["virtualAccountPhone"] = req.VirtualAccountPhone
	}
	if req.VirtualAccountEmail != "" {
		body["virtualAccountEmail"] = req.VirtualAccountEmail
	}
	if req.ExpiredDate != "" {
		body["expiredDate"] = req.ExpiredDate
	}

	var resp CreateVAResponse
	raw, err := c.doSNAPRequest(ctx, http.MethodPost, CreateVAPath, body, &resp)
	if err != nil {
		return nil, err
	}
	resp.RawResponse = raw
	return &resp, nil
}

// InquiryVA queries a VA transaction's status.
func (c *Client) InquiryVA(ctx context.Context, partnerReferenceNo string) (*InquiryVAResponse, error) {
	body := map[string]any{"originalPartnerReferenceNo": partnerReferenceNo}
	var resp InquiryVAResponse
	raw, err := c.doSNAPRequest(ctx, http.MethodPost, InquiryVAPath, body, &resp)
	if err != nil {
		return nil, err
	}
	resp.RawResponse = raw
	return &resp, nil
}

// DeleteVA invalidates an unpaid VA.
func (c *Client) DeleteVA(ctx context.Context, req DeleteVARequest) (*DeleteVAResponse, error) {
	body := map[string]any{
		"partnerReferenceNo": req.PartnerReferenceNo,
		"customerNo":         req.CustomerNo,
		"virtualAccountNo":   req.VirtualAccountNo,
	}
	if req.TrxID != "" {
		body["trxId"] = req.TrxID
	}
	var resp DeleteVAResponse
	raw, err := c.doSNAPRequest(ctx, http.MethodPost, DeleteVAPath, body, &resp)
	if err != nil {
		return nil, err
	}
	resp.RawResponse = raw
	return &resp, nil
}

// GenerateQRMPM creates a QRIS MPM code. Pakailink docs at
// https://pakaidonk.id/dokumentasi-api/generate-qr-mpm/.
func (c *Client) GenerateQRMPM(ctx context.Context, req GenerateQRRequest) (*GenerateQRResponse, error) {
	body := map[string]any{
		"partnerReferenceNo": req.PartnerReferenceNo,
		"amount": Amount{
			Value:    formatAmount(req.Amount),
			Currency: "IDR",
		},
	}
	if req.TerminalID != "" {
		body["terminalId"] = req.TerminalID
	}
	info := map[string]any{}
	if req.CallbackURL != "" {
		info["callbackUrl"] = req.CallbackURL
	}
	if req.ExpiredDate != "" {
		info["expiredDate"] = req.ExpiredDate
	}
	if req.MerchantName != "" {
		info["merchantName"] = req.MerchantName
	}
	if req.Description != "" {
		info["description"] = req.Description
	}
	if len(info) > 0 {
		body["additionalInfo"] = info
	}

	var resp GenerateQRResponse
	raw, err := c.doSNAPRequest(ctx, http.MethodPost, GenerateQRPath, body, &resp)
	if err != nil {
		return nil, err
	}
	resp.RawResponse = raw
	return &resp, nil
}

// InquiryQR queries the status of a QRIS MPM transaction.
func (c *Client) InquiryQR(ctx context.Context, partnerReferenceNo string) (*InquiryQRResponse, error) {
	body := map[string]any{"originalPartnerReferenceNo": partnerReferenceNo}
	var resp InquiryQRResponse
	raw, err := c.doSNAPRequest(ctx, http.MethodPost, InquiryQRPath, body, &resp)
	if err != nil {
		return nil, err
	}
	resp.RawResponse = raw
	return &resp, nil
}

func (c *Client) doSNAPRequest(ctx context.Context, method, path string, body any, out any) (json.RawMessage, error) {
	token, err := c.GetAccessToken(ctx)
	if err != nil {
		return nil, err
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	ts := formatTimestamp(time.Now())
	sig := signSymmetric(method, path, token, bodyBytes, ts, c.cfg.ClientSecret)
	externalID := strconv.FormatInt(time.Now().UnixNano(), 10)

	req, err := http.NewRequestWithContext(ctx, method, c.cfg.BaseURL+path, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-TIMESTAMP", ts)
	req.Header.Set("X-SIGNATURE", sig)
	req.Header.Set("X-PARTNER-ID", c.cfg.PartnerID)
	req.Header.Set("X-EXTERNAL-ID", externalID)
	req.Header.Set("CHANNEL-ID", c.cfg.ChannelID)

	return c.doRequest(req, out)
}

func (c *Client) doRequest(req *http.Request, out any) (json.RawMessage, error) {
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
			return raw, fmt.Errorf("pakailink: decode response: %w", err)
		}
	}

	code, msg := extractResponseStatus(raw)
	if resp.StatusCode >= http.StatusBadRequest || !isSuccessCode(code) {
		return raw, &APIError{
			HTTPStatus:      resp.StatusCode,
			ResponseCode:    code,
			ResponseMessage: msg,
			RawResponse:     raw,
		}
	}
	return raw, nil
}

func extractResponseStatus(raw json.RawMessage) (string, string) {
	var s struct {
		ResponseCode    string `json:"responseCode"`
		ResponseMessage string `json:"responseMessage"`
	}
	_ = json.Unmarshal(raw, &s)
	return s.ResponseCode, s.ResponseMessage
}

func isSuccessCode(code string) bool {
	return strings.HasPrefix(code, "200")
}

func formatAmount(amount int64) string {
	return fmt.Sprintf("%d.00", amount)
}

// ParseWebhookAmount extracts an int64 from an Amount value string like "10000.00".
func ParseWebhookAmount(a Amount) (int64, error) {
	v := strings.TrimSpace(a.Value)
	if v == "" {
		return 0, nil
	}
	if dot := strings.Index(v, "."); dot >= 0 {
		v = v[:dot]
	}
	return strconv.ParseInt(v, 10, 64)
}
