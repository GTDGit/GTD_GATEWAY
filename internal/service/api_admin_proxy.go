package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"time"
)

// APIAdminProxy forwards admin requests to the api service's /v1/admin/qris/*
// endpoints. The api service owns the Nobu generate client, the client webhook
// pipeline, and the rendered Excel batch files, so the gateway delegates those
// operations rather than duplicating provider credentials.
//
// Auth: the api and gateway share JWT_SECRET, so the caller's admin Bearer token
// is passed straight through — no separate internal token needed.
type APIAdminProxy struct {
	baseURL    string
	httpClient *http.Client
}

func NewAPIAdminProxy(apiBaseURL string) *APIAdminProxy {
	return &APIAdminProxy{
		baseURL:    strings.TrimRight(strings.TrimSpace(apiBaseURL), "/"),
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// Enabled reports whether the proxy can reach api.
func (p *APIAdminProxy) Enabled() bool {
	return p != nil && p.baseURL != ""
}

// ProxyResponse is the raw upstream response captured for passthrough.
type ProxyResponse struct {
	StatusCode         int
	Body               []byte
	ContentType        string
	ContentDisposition string
}

// Forward sends method+path to api with the caller's Authorization header and an
// optional JSON body, returning the raw upstream response for passthrough.
func (p *APIAdminProxy) Forward(ctx context.Context, method, path, authHeader string, body []byte) (*ProxyResponse, error) {
	if !p.Enabled() {
		return nil, newPaymentError(http.StatusServiceUnavailable, "PROXY_UNAVAILABLE", "api admin proxy is not configured", nil)
	}

	var reader io.Reader
	if len(body) > 0 {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, p.baseURL+path, reader)
	if err != nil {
		return nil, newPaymentError(http.StatusInternalServerError, "INTERNAL_ERROR", "failed to build proxy request", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, newPaymentError(http.StatusBadGateway, "PROXY_UNREACHABLE", "failed to reach api admin endpoint", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, newPaymentError(http.StatusBadGateway, "PROXY_READ_ERROR", "failed to read api admin response", err)
	}
	return &ProxyResponse{
		StatusCode:         resp.StatusCode,
		Body:               raw,
		ContentType:        resp.Header.Get("Content-Type"),
		ContentDisposition: resp.Header.Get("Content-Disposition"),
	}, nil
}
