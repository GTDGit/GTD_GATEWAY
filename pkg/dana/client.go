package dana

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
	TokenPath       = "/v1.0/access-token/b2b.htm"
	CreateOrderPath = "/payment-gateway/v1.0/debit/payment-host-to-host.htm"
	InquiryPath     = "/payment-gateway/v1.0/debit/status.htm"
	CancelPath      = "/payment-gateway/v1.0/debit/cancel.htm"
	RefundPath      = "/payment-gateway/v1.0/debit/refund.htm"

	DefaultChannelID = "95221"
	ServiceCodeDebit = "54"
)

type Config struct {
	BaseURL        string
	MerchantID     string
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
		return nil, fmt.Errorf("dana: base URL is required")
	}
	if cfg.ClientID == "" || cfg.ClientSecret == "" {
		return nil, fmt.Errorf("dana: clientID and clientSecret are required")
	}
	if cfg.MerchantID == "" {
		return nil, fmt.Errorf("dana: merchantID is required")
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

func (c *Client) ClientSecret() string { return c.cfg.ClientSecret }
func (c *Client) MerchantID() string   { return c.cfg.MerchantID }

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
	req.Header.Set("CHANNEL-ID", c.cfg.ChannelID)

	var tr tokenResponse
	if _, err := c.doRequest(req, &tr); err != nil {
		return "", err
	}
	expiresIn, _ := strconv.Atoi(tr.ExpiresIn)
	if expiresIn <= 0 {
		expiresIn = 3600
	}
	c.accessToken = tr.AccessToken
	c.tokenExpires = time.Now().Add(time.Duration(expiresIn-60) * time.Second)
	return c.accessToken, nil
}

func (c *Client) CreateOrder(ctx context.Context, req CreateOrderRequest) (*CreateOrderResponse, error) {
	if req.PayMethod == "" {
		req.PayMethod = PayMethodBalance
	}
	if req.OrderScenario == "" {
		req.OrderScenario = "API"
	}
	if req.MCC == "" {
		req.MCC = "5732"
	}
	if req.MerchantID == "" {
		req.MerchantID = c.cfg.MerchantID
	}

	urls := []map[string]any{}
	if req.ReturnURL != "" {
		urls = append(urls, map[string]any{
			"url":        req.ReturnURL,
			"type":       "PAY_RETURN",
			"isDeeplink": "Y",
		})
	}
	if req.NotificationURL != "" {
		urls = append(urls, map[string]any{
			"url":        req.NotificationURL,
			"type":       "NOTIFICATION",
			"isDeeplink": "N",
		})
	}

	amount := Amount{Value: formatAmount(req.Amount), Currency: "IDR"}
	payOption := map[string]any{
		"payMethod":   req.PayMethod,
		"payOption":   req.PayOption,
		"transAmount": amount,
	}

	body := map[string]any{
		"partnerReferenceNo": req.PartnerReferenceNo,
		"merchantId":         req.MerchantID,
		"amount":             amount,
		"validUpTo":          req.ValidUpTo,
		"urlParams":          urls,
		"payOptionDetails":   []any{payOption},
		"additionalInfo": map[string]any{
			"order": map[string]any{
				"orderTitle": firstNonEmpty(req.OrderTitle, req.PartnerReferenceNo),
				"scenario":   req.OrderScenario,
			},
			"mcc": req.MCC,
			"envInfo": map[string]any{
				"sourcePlatform":    "IPG",
				"terminalType":      "SYSTEM",
				"orderTerminalType": "WEB",
			},
		},
	}
	if req.ExternalStoreID != "" {
		body["externalStoreId"] = req.ExternalStoreID
	}

	var resp CreateOrderResponse
	raw, err := c.doSNAPRequest(ctx, http.MethodPost, CreateOrderPath, body, &resp)
	if err != nil {
		return nil, err
	}
	resp.RawResponse = raw
	return &resp, nil
}

func (c *Client) InquiryOrder(ctx context.Context, partnerReferenceNo string) (*InquiryOrderResponse, error) {
	body := map[string]any{
		"originalPartnerReferenceNo": partnerReferenceNo,
		"merchantId":                 c.cfg.MerchantID,
		"serviceCode":                ServiceCodeDebit,
	}
	var resp InquiryOrderResponse
	raw, err := c.doSNAPRequest(ctx, http.MethodPost, InquiryPath, body, &resp)
	if err != nil {
		return nil, err
	}
	resp.RawResponse = raw
	return &resp, nil
}

func (c *Client) CancelOrder(ctx context.Context, req CancelOrderRequest) (*CancelOrderResponse, error) {
	body := map[string]any{
		"originalPartnerReferenceNo": req.PartnerReferenceNo,
		"merchantId":                 firstNonEmpty(req.MerchantID, c.cfg.MerchantID),
	}
	if req.Reason != "" {
		body["reason"] = req.Reason
	}
	var resp CancelOrderResponse
	raw, err := c.doSNAPRequest(ctx, http.MethodPost, CancelPath, body, &resp)
	if err != nil {
		return nil, err
	}
	resp.RawResponse = raw
	return &resp, nil
}

func (c *Client) Refund(ctx context.Context, req RefundRequest) (*RefundResponse, error) {
	body := map[string]any{
		"originalPartnerReferenceNo": req.OriginalPartnerReference,
		"partnerRefundNo":            req.PartnerRefundNo,
		"merchantId":                 firstNonEmpty(req.MerchantID, c.cfg.MerchantID),
		"refundAmount": Amount{
			Value:    formatAmount(req.RefundAmount),
			Currency: "IDR",
		},
		"reason": req.Reason,
	}
	if req.OriginalReferenceNo != "" {
		body["originalReferenceNo"] = req.OriginalReferenceNo
	}
	var resp RefundResponse
	raw, err := c.doSNAPRequest(ctx, http.MethodPost, RefundPath, body, &resp)
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
			return raw, fmt.Errorf("dana: decode response: %w", err)
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

func isSuccessCode(code string) bool { return strings.HasPrefix(code, "200") }

func formatAmount(a int64) string { return fmt.Sprintf("%d.00", a) }

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return ""
}

// ParseWebhookAmount decodes a SNAP amount string to int64 (drops decimals).
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
