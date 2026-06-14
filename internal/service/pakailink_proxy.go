package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// PakailinkProxy is the gateway's thin HTTP client to the api service's internal
// QRIS endpoints. The Pakailink provider client lives only in the api service, so
// the gateway delegates register/generate over an internal-token-guarded channel
// instead of holding provider credentials itself.
type PakailinkProxy struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewPakailinkProxy builds a proxy targeting apiInternalURL. When apiInternalURL
// or token is blank the proxy is disabled (Enabled reports false) and callers
// should surface a PROXY_UNAVAILABLE error.
func NewPakailinkProxy(apiInternalURL, token string) *PakailinkProxy {
	return &PakailinkProxy{
		baseURL: strings.TrimRight(strings.TrimSpace(apiInternalURL), "/"),
		token:   strings.TrimSpace(token),
		httpClient: &http.Client{
			Timeout: 20 * time.Second,
		},
	}
}

// Enabled reports whether the proxy has the configuration needed to reach api.
func (p *PakailinkProxy) Enabled() bool {
	return p != nil && p.baseURL != "" && p.token != ""
}

// PakailinkGenerateRequest is the gateway-side input forwarded to the api
// internal generate endpoint.
type PakailinkGenerateRequest struct {
	PartnerReferenceNo string `json:"partnerReferenceNo,omitempty"`
	MerchantID         string `json:"merchantId,omitempty"`
	StoreID            string `json:"storeId"`
	TerminalID         string `json:"terminalId,omitempty"`
	MerchantName       string `json:"merchantName,omitempty"`
}

// PakailinkGenerateResponse mirrors the api internal generate data payload.
type PakailinkGenerateResponse struct {
	PartnerReferenceNo string          `json:"partnerReferenceNo"`
	ReferenceNo        string          `json:"referenceNo"`
	QRContent          string          `json:"qrContent"`
	TerminalID         string          `json:"terminalId"`
	Parsed             json.RawMessage `json:"parsed,omitempty"`
}

// PakailinkRegisterRequest is forwarded to the api internal register endpoint.
type PakailinkRegisterRequest struct {
	PartnerReferenceNo string `json:"partnerReferenceNo,omitempty"`
	MerchantName       string `json:"merchantName,omitempty"`
	MerchantEmail      string `json:"merchantEmail,omitempty"`
	StoreName          string `json:"storeName,omitempty"`
	City               string `json:"city,omitempty"`
	OwnerFirstName     string `json:"ownerFirstName,omitempty"`
	OwnerLastName      string `json:"ownerLastName,omitempty"`
	OwnerEmail         string `json:"ownerEmail,omitempty"`
	OwnerPhone         string `json:"ownerPhone,omitempty"`
}

// PakailinkRegisterResponse mirrors the api internal register data payload.
type PakailinkRegisterResponse struct {
	PartnerReferenceNo string          `json:"partnerReferenceNo"`
	DetailData         json.RawMessage `json:"detailData,omitempty"`
	Raw                json.RawMessage `json:"raw,omitempty"`
}

// apiEnvelope is the standard api response wrapper.
type apiEnvelope struct {
	Success bool            `json:"success"`
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
	Error   *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// Generate drives Pakailink static QR generation via the api internal endpoint.
func (p *PakailinkProxy) Generate(ctx context.Context, req PakailinkGenerateRequest) (*PakailinkGenerateResponse, error) {
	var out PakailinkGenerateResponse
	if err := p.post(ctx, "/v1/internal/qris/pakailink/generate", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Register drives Pakailink static QRIS merchant registration via the api
// internal endpoint.
func (p *PakailinkProxy) Register(ctx context.Context, req PakailinkRegisterRequest) (*PakailinkRegisterResponse, error) {
	var out PakailinkRegisterResponse
	if err := p.post(ctx, "/v1/internal/qris/pakailink/register", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (p *PakailinkProxy) post(ctx context.Context, path string, body, out any) error {
	if !p.Enabled() {
		return newPaymentError(http.StatusServiceUnavailable, "PROXY_UNAVAILABLE", "api internal proxy is not configured", nil)
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return newPaymentError(http.StatusInternalServerError, "INTERNAL_ERROR", "failed to encode proxy request", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return newPaymentError(http.StatusInternalServerError, "INTERNAL_ERROR", "failed to build proxy request", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Internal-Token", p.token)

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return newPaymentError(http.StatusBadGateway, "PROXY_UNREACHABLE", "failed to reach api internal endpoint", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return newPaymentError(http.StatusBadGateway, "PROXY_READ_ERROR", "failed to read api internal response", err)
	}

	var env apiEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return newPaymentError(http.StatusBadGateway, "PROXY_DECODE_ERROR", "invalid api internal response", fmt.Errorf("%s", string(raw)))
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 || !env.Success {
		code := "PROVIDER_ERROR"
		msg := env.Message
		if env.Error != nil {
			if env.Error.Code != "" {
				code = env.Error.Code
			}
			if env.Error.Message != "" {
				msg = env.Error.Message
			}
		}
		if msg == "" {
			msg = "api internal call failed"
		}
		status := resp.StatusCode
		if status < 400 {
			status = http.StatusBadGateway
		}
		return newPaymentError(status, code, msg, nil)
	}

	if out != nil && len(env.Data) > 0 {
		if err := json.Unmarshal(env.Data, out); err != nil {
			return newPaymentError(http.StatusBadGateway, "PROXY_DECODE_ERROR", "failed to decode api internal data", err)
		}
	}
	return nil
}
