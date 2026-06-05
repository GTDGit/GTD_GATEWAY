package bri

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
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/GTDGit/gtd_gateway/pkg/bnc"
)

const (
	snapTokenPath             = "/snap/v1.0/access-token/b2b"
	internalInquiryPath       = "/intrabank/snap/v2.0/account-inquiry-internal"
	intrabankTransferPath     = "/intrabank/snap/v2.0/transfer-intrabank"
	transferStatusPath        = "/snap/v1.0/transfer/status"
	brivaCreatePath           = "/snap/v1.0/transfer-va/create-va"
	brivaUpdatePath           = "/snap/v1.0/transfer-va/update-va"
	brivaUpdateStatusPath     = "/snap/v1.0/transfer-va/update-status"
	brivaInquiryPath          = "/snap/v1.0/transfer-va/inquiry-va"
	brivaDeletePath           = "/snap/v1.0/transfer-va/delete-va"
	brivaReportPath           = "/snap/v1.0/transfer-va/report"
	brivaInquiryStatusPath    = "/snap/v1.0/transfer-va/status"
	brizziOAuthTokenPath      = "/oauth/client_credential/accesstoken"
	brizziValidatePath        = "/v2.0/brizzi/checknum"
	brizziTopupPath           = "/v2.0/brizzi/topup"
	brizziCheckStatusPath     = "/v2.0/brizzi/checktrx"
	defaultSnapTokenLifetime  = 900
	defaultOAuthTokenLifetime = 179999
)

type Config struct {
	BaseURL        string
	ClientID       string
	ClientSecret   string
	PartnerID      string
	ChannelID      string
	SourceAccount  string
	PrivateKeyPath string
	BRIZZIUsername string
}

type Client struct {
	cfg        Config
	httpClient *http.Client
	privateKey *rsa.PrivateKey

	mu               sync.Mutex
	snapAccessToken  string
	snapTokenExpires time.Time
	oauthAccessToken string
	oauthTokenExpiry time.Time
}

type APIError = bnc.APIError
type Amount = bnc.Amount
type AccountInquiryResponse = bnc.AccountInquiryResponse
type TransferRequest = bnc.TransferRequest
type TransferResponse = bnc.TransferResponse
type TransferStatusRequest = bnc.TransferStatusRequest
type TransferStatusResponse = bnc.TransferStatusResponse

type GenericSNAPResponse struct {
	ResponseCode    string          `json:"responseCode"`
	ResponseMessage string          `json:"responseMessage"`
	RawResponse     json.RawMessage `json:"-"`
}

type oauthTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   string `json:"expires_in"`
}

type snapTokenResponse struct {
	ResponseCode    string `json:"responseCode"`
	ResponseMessage string `json:"responseMessage"`
	AccessToken     string `json:"accessToken"`
	TokenType       string `json:"tokenType"`
	ExpiresIn       string `json:"expiresIn"`
}

type BRIZZIValidateResponse struct {
	ResponseCode        string          `json:"responseCode"`
	ResponseDescription string          `json:"responseDescription"`
	RawResponse         json.RawMessage `json:"-"`
}

type BRIZZITopupData struct {
	BRIZZICardNo   string `json:"brizziCardNo"`
	PendingBalance string `json:"pendingBalance"`
	Reff           string `json:"reff"`
}

type BRIZZITopupResponse struct {
	ErrorCode           string           `json:"errorCode"`
	ResponseCode        string           `json:"responseCode"`
	ResponseDescription string           `json:"responseDescription"`
	Data                BRIZZITopupData  `json:"data"`
	RawResponse         json.RawMessage  `json:"-"`
}

type BRIZZICheckStatusData struct {
	JenisTrx string `json:"jenisTrx"`
	Reversal string `json:"reversal"`
}

type BRIZZICheckStatusResponse struct {
	ResponseCode        string                 `json:"responseCode"`
	ResponseDescription string                 `json:"responseDescription"`
	Data                BRIZZICheckStatusData  `json:"data"`
	RawResponse         json.RawMessage        `json:"-"`
}

func NewClient(cfg Config) (*Client, error) {
	cfg.PartnerID = strings.TrimSpace(cfg.PartnerID)
	cfg.ChannelID = deriveBRIChannelID(cfg.ChannelID, cfg.PartnerID)

	if strings.TrimSpace(cfg.BaseURL) == "" {
		return nil, fmt.Errorf("bri base url is required")
	}
	if strings.TrimSpace(cfg.ClientID) == "" || strings.TrimSpace(cfg.ClientSecret) == "" {
		return nil, fmt.Errorf("bri configuration incomplete")
	}

	var (
		privateKey *rsa.PrivateKey
		err        error
	)
	if strings.TrimSpace(cfg.PrivateKeyPath) != "" {
		privateKey, err = loadPrivateKey(cfg.PrivateKeyPath)
		if err != nil {
			return nil, err
		}
	}

	return &Client{
		cfg:        cfg,
		privateKey: privateKey,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}, nil
}

func (c *Client) SNAPAvailable() bool {
	return c != nil &&
		c.privateKey != nil &&
		strings.TrimSpace(c.cfg.PartnerID) != "" &&
		strings.TrimSpace(c.cfg.ChannelID) != ""
}

func (c *Client) InternalAccountInquiry(ctx context.Context, accountNo string) (*bnc.AccountInquiryResponse, error) {
	body := map[string]string{
		"beneficiaryAccountNo": strings.TrimSpace(accountNo),
	}

	var resp bnc.AccountInquiryResponse
	raw, err := c.doSNAPRequest(ctx, http.MethodPost, internalInquiryPath, body, &resp)
	if err != nil {
		return nil, err
	}
	resp.RawResponse = raw
	return &resp, nil
}

func (c *Client) ExternalAccountInquiry(_ context.Context, _ string, _ string) (*bnc.AccountInquiryResponse, error) {
	return nil, fmt.Errorf("bri external account inquiry is not implemented")
}

func (c *Client) IntrabankTransfer(ctx context.Context, req bnc.TransferRequest) (*bnc.TransferResponse, error) {
	body := map[string]any{
		"partnerReferenceNo": req.PartnerReferenceNo,
		"amount": bnc.Amount{
			Value:    formatAmount(req.Amount),
			Currency: "IDR",
		},
		"beneficiaryAccountNo":   req.BeneficiaryAccountNo,
		"beneficiaryAccountName": req.BeneficiaryAccountName,
		"sourceAccountNo":        c.cfg.SourceAccount,
		"transactionDate":        formatSNAPTimestamp(req.TransactionDate),
	}
	if strings.TrimSpace(req.Remark) != "" {
		body["remark"] = strings.TrimSpace(req.Remark)
	}

	var resp bnc.TransferResponse
	raw, err := c.doSNAPRequest(ctx, http.MethodPost, intrabankTransferPath, body, &resp)
	if err != nil {
		return nil, err
	}
	resp.RawResponse = raw
	return &resp, nil
}

func (c *Client) InterbankTransfer(_ context.Context, _ bnc.TransferRequest) (*bnc.TransferResponse, error) {
	return nil, fmt.Errorf("bri interbank transfer is not implemented")
}

func (c *Client) TransferStatus(ctx context.Context, req bnc.TransferStatusRequest) (*bnc.TransferStatusResponse, error) {
	body := map[string]any{
		"originalPartnerReferenceNo": req.OriginalPartnerReferenceNo,
		"serviceCode":                req.ServiceCode,
		"transactionDate":            formatSNAPTimestamp(req.TransactionDate),
	}

	var resp bnc.TransferStatusResponse
	raw, err := c.doSNAPRequest(ctx, http.MethodPost, transferStatusPath, body, &resp)
	if err != nil {
		return nil, err
	}
	resp.RawResponse = raw
	return &resp, nil
}

func (c *Client) CreateVA(ctx context.Context, body any) (*GenericSNAPResponse, error) {
	return c.doGenericSNAPRequest(ctx, http.MethodPost, brivaCreatePath, body)
}

func (c *Client) UpdateVA(ctx context.Context, body any) (*GenericSNAPResponse, error) {
	return c.doGenericSNAPRequest(ctx, http.MethodPut, brivaUpdatePath, body)
}

func (c *Client) UpdateVAStatus(ctx context.Context, body any) (*GenericSNAPResponse, error) {
	return c.doGenericSNAPRequest(ctx, http.MethodPut, brivaUpdateStatusPath, body)
}

func (c *Client) InquiryVA(ctx context.Context, body any) (*GenericSNAPResponse, error) {
	return c.doGenericSNAPRequest(ctx, http.MethodPut, brivaInquiryPath, body)
}

func (c *Client) DeleteVA(ctx context.Context, body any) (*GenericSNAPResponse, error) {
	return c.doGenericSNAPRequest(ctx, http.MethodDelete, brivaDeletePath, body)
}

func (c *Client) GetVAReport(ctx context.Context, body any) (*GenericSNAPResponse, error) {
	return c.doGenericSNAPRequest(ctx, http.MethodPost, brivaReportPath, body)
}

func (c *Client) InquiryVAStatus(ctx context.Context, body any) (*GenericSNAPResponse, error) {
	return c.doGenericSNAPRequest(ctx, http.MethodPost, brivaInquiryStatusPath, body)
}

func (c *Client) BRIZZIValidateCard(ctx context.Context, cardNo string) (*BRIZZIValidateResponse, error) {
	body := map[string]string{
		"username":     c.cfg.BRIZZIUsername,
		"brizziCardNo": strings.TrimSpace(cardNo),
	}

	var resp BRIZZIValidateResponse
	raw, err := c.doBRIZZIRequest(ctx, http.MethodPost, brizziValidatePath, body, &resp)
	if err != nil {
		return nil, err
	}
	resp.RawResponse = raw
	return &resp, nil
}

func (c *Client) BRIZZITopup(ctx context.Context, cardNo string, amount int) (*BRIZZITopupResponse, error) {
	body := map[string]string{
		"username":     c.cfg.BRIZZIUsername,
		"brizziCardNo": strings.TrimSpace(cardNo),
		"amount":       formatAmount(int64(amount)),
	}

	var resp BRIZZITopupResponse
	raw, err := c.doBRIZZIRequest(ctx, http.MethodPost, brizziTopupPath, body, &resp)
	if err != nil {
		return nil, err
	}
	resp.RawResponse = raw
	return &resp, nil
}

func (c *Client) BRIZZICheckTopupStatus(ctx context.Context, cardNo string, amount int, ref string) (*BRIZZICheckStatusResponse, error) {
	body := map[string]string{
		"username":     c.cfg.BRIZZIUsername,
		"brizziCardNo": strings.TrimSpace(cardNo),
		"amount":       strconv.Itoa(amount),
		"reff":         strings.TrimSpace(ref),
	}

	var resp BRIZZICheckStatusResponse
	raw, err := c.doBRIZZIRequest(ctx, http.MethodPost, brizziCheckStatusPath, body, &resp)
	if err != nil {
		return nil, err
	}
	resp.RawResponse = raw
	return &resp, nil
}

func (c *Client) doGenericSNAPRequest(ctx context.Context, method, path string, body any) (*GenericSNAPResponse, error) {
	var resp GenericSNAPResponse
	raw, err := c.doSNAPRequest(ctx, method, path, body, &resp)
	if err != nil {
		return nil, err
	}
	resp.RawResponse = raw
	return &resp, nil
}

func (c *Client) doSNAPRequest(ctx context.Context, method, path string, body any, out any) (json.RawMessage, error) {
	if !c.SNAPAvailable() {
		return nil, fmt.Errorf("bri snap configuration incomplete")
	}

	token, err := c.getSNAPAccessToken(ctx)
	if err != nil {
		return nil, err
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	timestamp := formatSNAPTimestamp(time.Now())
	signature := c.signSNAP(method, path, token, bodyBytes, timestamp)
	externalID := generateSNAPExternalID(path)

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

	return c.doJSONRequest(req, out)
}

func (c *Client) doBRIZZIRequest(ctx context.Context, method, path string, body any, out any) (json.RawMessage, error) {
	token, err := c.getOAuthToken(ctx)
	if err != nil {
		return nil, err
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	timestamp := formatUTCTimestamp(time.Now())
	signature := c.signBRIZZI(path, method, "Bearer "+token, timestamp, bodyBytes)
	externalID := generateBRIZZIExternalID()

	req, err := http.NewRequestWithContext(ctx, method, c.cfg.BaseURL+path, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("BRI-Timestamp", timestamp)
	req.Header.Set("BRI-Signature", signature)
	req.Header.Set("BRI-External-Id", externalID)

	return c.doJSONRequest(req, out)
}

func (c *Client) getSNAPAccessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.snapAccessToken != "" && time.Now().Before(c.snapTokenExpires) {
		return c.snapAccessToken, nil
	}

	if c.privateKey == nil {
		return "", fmt.Errorf("bri private key is required for snap requests")
	}

	timestamp := formatSNAPTimestamp(time.Now())
	signature, err := c.signAsymmetric(c.cfg.ClientID + "|" + timestamp)
	if err != nil {
		return "", err
	}

	bodyBytes, err := json.Marshal(map[string]string{"grantType": "client_credentials"})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.BaseURL+snapTokenPath, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-TIMESTAMP", timestamp)
	req.Header.Set("X-CLIENT-KEY", c.cfg.ClientID)
	req.Header.Set("X-SIGNATURE", signature)

	var resp snapTokenResponse
	_, err = c.doJSONRequest(req, &resp)
	if err != nil {
		return "", err
	}

	expiresIn, err := strconv.Atoi(resp.ExpiresIn)
	if err != nil || expiresIn <= 0 {
		expiresIn = defaultSnapTokenLifetime
	}

	c.snapAccessToken = resp.AccessToken
	c.snapTokenExpires = time.Now().Add(time.Duration(expiresIn-60) * time.Second)
	return c.snapAccessToken, nil
}

func (c *Client) getOAuthToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.oauthAccessToken != "" && time.Now().Before(c.oauthTokenExpiry) {
		return c.oauthAccessToken, nil
	}

	form := url.Values{}
	form.Set("client_id", c.cfg.ClientID)
	form.Set("client_secret", c.cfg.ClientSecret)

	endpoint := c.cfg.BaseURL + brizziOAuthTokenPath + "?grant_type=client_credentials"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	var resp oauthTokenResponse
	_, err = c.doJSONRequest(req, &resp)
	if err != nil {
		return "", err
	}

	expiresIn, err := strconv.Atoi(resp.ExpiresIn)
	if err != nil || expiresIn <= 0 {
		expiresIn = defaultOAuthTokenLifetime
	}

	c.oauthAccessToken = resp.AccessToken
	c.oauthTokenExpiry = time.Now().Add(time.Duration(expiresIn-300) * time.Second)
	return c.oauthAccessToken, nil
}

func (c *Client) doJSONRequest(req *http.Request, out any) (json.RawMessage, error) {
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
		return raw, fmt.Errorf("failed to decode bri response: %w", err)
	}

	if resp.StatusCode >= http.StatusBadRequest {
		code, message := extractResponseStatus(out)
		return raw, &bnc.APIError{
			HTTPStatus:      resp.StatusCode,
			ResponseCode:    code,
			ResponseMessage: message,
			RawResponse:     raw,
		}
	}

	return raw, nil
}

func extractResponseStatus(v any) (string, string) {
	type responseStatus struct {
		ResponseCode        string `json:"responseCode"`
		ResponseDescription string `json:"responseDescription"`
		ResponseMessage     string `json:"responseMessage"`
	}

	raw, err := json.Marshal(v)
	if err != nil {
		return "", ""
	}

	var resp responseStatus
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", ""
	}

	message := strings.TrimSpace(resp.ResponseMessage)
	if message == "" {
		message = strings.TrimSpace(resp.ResponseDescription)
	}

	return strings.TrimSpace(resp.ResponseCode), message
}

func (c *Client) signAsymmetric(payload string) (string, error) {
	hashed := sha256.Sum256([]byte(payload))
	signature, err := rsa.SignPKCS1v15(rand.Reader, c.privateKey, crypto.SHA256, hashed[:])
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(signature), nil
}

func (c *Client) signSNAP(method, path, accessToken string, body []byte, timestamp string) string {
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

func (c *Client) signBRIZZI(path, method, token, timestamp string, body []byte) string {
	payload := strings.Join([]string{
		"path=" + path,
		"verb=" + strings.ToUpper(strings.TrimSpace(method)),
		"token=" + token,
		"timestamp=" + timestamp,
		"body=" + string(body),
	}, "&")

	mac := hmac.New(sha256.New, []byte(c.cfg.ClientSecret))
	mac.Write([]byte(payload))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func loadPrivateKey(path string) (*rsa.PrivateKey, error) {
	pemBytes, err := os.ReadFile(strings.TrimSpace(path))
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

func formatSNAPTimestamp(t time.Time) string {
	wib := time.FixedZone("WIB", 7*3600)
	return t.In(wib).Format("2006-01-02T15:04:05+07:00")
}

func formatUTCTimestamp(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000Z")
}

func formatAmount(amount int64) string {
	return fmt.Sprintf("%d.00", amount)
}

func generateSNAPExternalID(path string) string {
	if strings.HasPrefix(strings.TrimSpace(path), "/snap/v1.0/transfer-va") {
		v := time.Now().UnixNano() % 1000000000
		if v < 0 {
			v = -v
		}
		return fmt.Sprintf("%09d", v)
	}

	now := time.Now()
	return fmt.Sprintf("%018d%018d", now.Unix(), now.UnixNano()%1000000000000000000)
}

func generateBRIZZIExternalID() string {
	v := time.Now().UnixNano() % 1000000000
	if v < 0 {
		v = -v
	}
	return fmt.Sprintf("%09d", v)
}

func deriveBRIChannelID(channelID, partnerID string) string {
	channelID = strings.TrimSpace(channelID)
	if isFixedNumeric(channelID, 5) {
		return channelID
	}

	partnerID = strings.TrimSpace(partnerID)
	if isFixedNumeric(partnerID, 5) {
		return partnerID
	}

	return channelID
}

func deriveBRIVAPartnerServiceID(companyCode, partnerID string) string {
	value := strings.TrimSpace(companyCode)
	if value == "" {
		value = strings.TrimSpace(partnerID)
	}
	if value == "" {
		return ""
	}
	if len(value) > 8 {
		value = value[len(value)-8:]
	}
	return fmt.Sprintf("%8s", value)
}

func isFixedNumeric(value string, length int) bool {
	if len(value) != length {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
