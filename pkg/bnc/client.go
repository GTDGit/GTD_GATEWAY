package bnc

import (
	"bytes"
	"context"
	"crypto"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	tokenPath             = "/snap/v1.0/access-token/b2b"
	externalInquiryPath   = "/snap/v1.0/account-inquiry-external"
	internalInquiryPath   = "/snap/v1.0/account-inquiry-internal"
	interbankTransferPath = "/snap/v1.0/transfer-interbank"
	intrabankTransferPath = "/snap/v1.0/transfer-intrabank"
	transferStatusPath    = "/snap/v1.0/transfer/status"
)

type Config struct {
	BaseURL        string
	ClientID       string
	ClientSecret   string
	PartnerID      string
	ChannelID      string
	SourceAccount  string
	PrivateKeyPath string
}

type Client struct {
	cfg        Config
	httpClient *http.Client
	privateKey *rsa.PrivateKey

	mu           sync.Mutex
	accessToken  string
	tokenExpires time.Time
}

type APIError struct {
	HTTPStatus      int
	ResponseCode    string
	ResponseMessage string
	RawResponse     json.RawMessage
}

func (e *APIError) Error() string {
	if e == nil {
		return ""
	}
	if e.ResponseCode == "" {
		return e.ResponseMessage
	}
	return fmt.Sprintf("%s: %s", e.ResponseCode, e.ResponseMessage)
}

type tokenResponse struct {
	ResponseCode    string `json:"responseCode"`
	ResponseMessage string `json:"responseMessage"`
	AccessToken     string `json:"accessToken"`
	TokenType       string `json:"tokenType"`
	ExpiresIn       string `json:"expiresIn"`
}

type Amount struct {
	Value    string `json:"value"`
	Currency string `json:"currency"`
}

type AccountInquiryResponse struct {
	ResponseCode             string          `json:"responseCode"`
	ResponseMessage          string          `json:"responseMessage"`
	ReferenceNo              string          `json:"referenceNo"`
	BeneficiaryAccountName   string          `json:"beneficiaryAccountName"`
	BeneficiaryAccountNo     string          `json:"beneficiaryAccountNo"`
	BeneficiaryBankCode      string          `json:"beneficiaryBankCode,omitempty"`
	BeneficiaryBankName      string          `json:"beneficiaryBankName,omitempty"`
	BeneficiaryAccountStatus string          `json:"beneficiaryAccountStatus,omitempty"`
	Currency                 string          `json:"currency,omitempty"`
	RawResponse              json.RawMessage `json:"-"`
}

type TransferRequest struct {
	PartnerReferenceNo     string
	Amount                 int64
	BeneficiaryAccountNo   string
	BeneficiaryBankCode    string
	BeneficiaryAccountName string
	Remark                 string
	PurposeCode            string
	TransactionDate        time.Time
}

type TransferResponse struct {
	ResponseCode         string          `json:"responseCode"`
	ResponseMessage      string          `json:"responseMessage"`
	ReferenceNo          string          `json:"referenceNo"`
	PartnerReferenceNo   string          `json:"partnerReferenceNo"`
	Amount               Amount          `json:"amount"`
	BeneficiaryAccountNo string          `json:"beneficiaryAccountNo"`
	BeneficiaryBankCode  string          `json:"beneficiaryBankCode,omitempty"`
	SourceAccountNo      string          `json:"sourceAccountNo"`
	TransactionDate      string          `json:"transactionDate"`
	RawResponse          json.RawMessage `json:"-"`
}

type TransferStatusRequest struct {
	OriginalPartnerReferenceNo string
	ServiceCode                string
	TransactionDate            time.Time
}

type TransferStatusResponse struct {
	ResponseCode               string          `json:"responseCode"`
	ResponseMessage            string          `json:"responseMessage"`
	OriginalReferenceNo        string          `json:"originalReferenceNo"`
	OriginalPartnerReferenceNo string          `json:"originalPartnerReferenceNo"`
	ServiceCode                string          `json:"serviceCode"`
	TransactionDate            string          `json:"transactionDate"`
	Amount                     Amount          `json:"amount"`
	BeneficiaryAccountNo       string          `json:"beneficiaryAccountNo"`
	BeneficiaryBankCode        string          `json:"beneficiaryBankCode,omitempty"`
	SourceAccountNo            string          `json:"sourceAccountNo,omitempty"`
	LatestTransactionStatus    string          `json:"latestTransactionStatus"`
	TransactionStatusDesc      string          `json:"transactionStatusDesc"`
	RawResponse                json.RawMessage `json:"-"`
}

func NewClient(cfg Config) (*Client, error) {
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("bnc base url is required")
	}
	if cfg.ClientID == "" || cfg.ClientSecret == "" || cfg.PartnerID == "" || cfg.ChannelID == "" || cfg.SourceAccount == "" {
		return nil, fmt.Errorf("bnc configuration incomplete")
	}
	key, err := loadPrivateKey(cfg.PrivateKeyPath)
	if err != nil {
		return nil, err
	}

	return &Client{
		cfg:        cfg,
		privateKey: key,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}, nil
}

func (c *Client) ExternalAccountInquiry(ctx context.Context, bankCode, accountNo string) (*AccountInquiryResponse, error) {
	req := map[string]string{
		"beneficiaryBankCode":  bankCode,
		"beneficiaryAccountNo": accountNo,
	}

	var resp AccountInquiryResponse
	raw, err := c.doSNAPRequest(ctx, http.MethodPost, externalInquiryPath, req, &resp)
	if err != nil {
		return nil, err
	}
	resp.RawResponse = raw
	return &resp, nil
}

func (c *Client) InternalAccountInquiry(ctx context.Context, accountNo string) (*AccountInquiryResponse, error) {
	req := map[string]string{
		"beneficiaryAccountNo": accountNo,
	}

	var resp AccountInquiryResponse
	raw, err := c.doSNAPRequest(ctx, http.MethodPost, internalInquiryPath, req, &resp)
	if err != nil {
		return nil, err
	}
	resp.RawResponse = raw
	return &resp, nil
}

func (c *Client) InterbankTransfer(ctx context.Context, req TransferRequest) (*TransferResponse, error) {
	body := map[string]any{
		"partnerReferenceNo": req.PartnerReferenceNo,
		"amount": Amount{
			Value:    formatAmount(req.Amount),
			Currency: "IDR",
		},
		"beneficiaryAccountNo":   req.BeneficiaryAccountNo,
		"beneficiaryBankCode":    req.BeneficiaryBankCode,
		"beneficiaryAccountName": req.BeneficiaryAccountName,
		"sourceAccountNo":        c.cfg.SourceAccount,
		"transactionDate":        formatTimestamp(req.TransactionDate),
		"purposeCode":            req.PurposeCode,
	}
	if req.Remark != "" {
		body["remark"] = req.Remark
	}

	var resp TransferResponse
	raw, err := c.doSNAPRequest(ctx, http.MethodPost, interbankTransferPath, body, &resp)
	if err != nil {
		return nil, err
	}
	resp.RawResponse = raw
	return &resp, nil
}

func (c *Client) IntrabankTransfer(ctx context.Context, req TransferRequest) (*TransferResponse, error) {
	body := map[string]any{
		"partnerReferenceNo": req.PartnerReferenceNo,
		"amount": Amount{
			Value:    formatAmount(req.Amount),
			Currency: "IDR",
		},
		"beneficiaryAccountNo": req.BeneficiaryAccountNo,
		"sourceAccountNo":      c.cfg.SourceAccount,
		"transactionDate":      formatTimestamp(req.TransactionDate),
	}
	if req.Remark != "" {
		body["remark"] = req.Remark
	}

	var resp TransferResponse
	raw, err := c.doSNAPRequest(ctx, http.MethodPost, intrabankTransferPath, body, &resp)
	if err != nil {
		return nil, err
	}
	resp.RawResponse = raw
	return &resp, nil
}

func (c *Client) TransferStatus(ctx context.Context, req TransferStatusRequest) (*TransferStatusResponse, error) {
	body := map[string]any{
		"originalPartnerReferenceNo": req.OriginalPartnerReferenceNo,
		"serviceCode":                req.ServiceCode,
		"transactionDate":            formatTimestamp(req.TransactionDate),
	}

	var resp TransferStatusResponse
	raw, err := c.doSNAPRequest(ctx, http.MethodPost, transferStatusPath, body, &resp)
	if err != nil {
		return nil, err
	}
	resp.RawResponse = raw
	return &resp, nil
}

func (c *Client) doSNAPRequest(ctx context.Context, method, path string, body any, out any) (json.RawMessage, error) {
	token, err := c.getAccessToken(ctx)
	if err != nil {
		return nil, err
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	timestamp := formatTimestamp(time.Now())
	signature := c.signSymmetric(method, path, token, bodyBytes, timestamp)
	externalID := generateExternalID()

	req, err := http.NewRequestWithContext(ctx, method, c.cfg.BaseURL+path, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-TIMESTAMP", timestamp)
	req.Header.Set("X-SIGNATURE", signature)
	req.Header.Set("X-PARTNER-ID", c.cfg.PartnerID)
	req.Header.Set("X-EXTERNAL-ID", externalID)
	req.Header.Set("CHANNEL-ID", c.cfg.ChannelID)

	return c.doRequest(req, out)
}

func (c *Client) getAccessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.accessToken != "" && time.Now().Before(c.tokenExpires) {
		return c.accessToken, nil
	}

	timestamp := formatTimestamp(time.Now())
	signature, err := c.signAsymmetric(c.cfg.ClientID + "|" + timestamp)
	if err != nil {
		return "", err
	}

	bodyBytes, err := json.Marshal(map[string]string{"grantType": "client_credentials"})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.BaseURL+tokenPath, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-TIMESTAMP", timestamp)
	req.Header.Set("X-CLIENT-KEY", c.cfg.ClientID)
	req.Header.Set("X-SIGNATURE", signature)

	var resp tokenResponse
	_, err = c.doRequest(req, &resp)
	if err != nil {
		return "", err
	}

	expiresIn, err := strconv.Atoi(resp.ExpiresIn)
	if err != nil || expiresIn <= 0 {
		expiresIn = 900
	}

	c.accessToken = resp.AccessToken
	c.tokenExpires = time.Now().Add(time.Duration(expiresIn-60) * time.Second)
	return c.accessToken, nil
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

	if err := json.Unmarshal(raw, out); err != nil {
		return raw, fmt.Errorf("failed to decode bnc response: %w", err)
	}

	respCode, respMsg := extractResponseStatus(out)
	if resp.StatusCode >= http.StatusBadRequest || !isSuccessCode(respCode) {
		return raw, &APIError{
			HTTPStatus:      resp.StatusCode,
			ResponseCode:    respCode,
			ResponseMessage: respMsg,
			RawResponse:     raw,
		}
	}

	return raw, nil
}

func extractResponseStatus(v any) (string, string) {
	type responseStatus struct {
		ResponseCode    string `json:"responseCode"`
		ResponseMessage string `json:"responseMessage"`
	}

	raw, err := json.Marshal(v)
	if err != nil {
		return "", ""
	}

	var resp responseStatus
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", ""
	}
	return resp.ResponseCode, resp.ResponseMessage
}

func isSuccessCode(code string) bool {
	return strings.HasPrefix(code, "200")
}

func (c *Client) signAsymmetric(payload string) (string, error) {
	hashed := sha256.Sum256([]byte(payload))
	signature, err := rsa.SignPKCS1v15(rand.Reader, c.privateKey, crypto.SHA256, hashed[:])
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(signature), nil
}

func (c *Client) signSymmetric(method, path, accessToken string, body []byte, timestamp string) string {
	bodyHash := sha256.Sum256(body)
	stringToSign := strings.Join([]string{
		method,
		path,
		accessToken,
		strings.ToLower(hex.EncodeToString(bodyHash[:])),
		timestamp,
	}, ":")

	mac := hmac.New(sha512.New, []byte(c.cfg.ClientSecret))
	mac.Write([]byte(stringToSign))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func loadPrivateKey(path string) (*rsa.PrivateKey, error) {
	if path == "" {
		return nil, fmt.Errorf("bnc private key path is required")
	}

	pemBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block from %s", path)
	}

	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}

	keyAny, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	key, ok := keyAny.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("private key in %s is not RSA", path)
	}
	return key, nil
}

func formatTimestamp(t time.Time) string {
	wib := time.FixedZone("WIB", 7*3600)
	return t.In(wib).Format("2006-01-02T15:04:05+07:00")
}

func formatAmount(amount int64) string {
	return fmt.Sprintf("%d.00", amount)
}

func generateExternalID() string {
	return strconv.FormatInt(time.Now().UnixNano(), 10)
}
