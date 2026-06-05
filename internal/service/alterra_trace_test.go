package service

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GTDGit/gtd_gateway/internal/models"
	"github.com/GTDGit/gtd_gateway/pkg/alterra"
)

func alterraTestPrivateKeyPEM(t *testing.T) string {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}

	block := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	}

	return string(pem.EncodeToMemory(block))
}

func alterraTestClient(t *testing.T, baseURL string) *alterra.Client {
	t.Helper()

	client, err := alterra.NewClient(alterra.Config{
		BaseURL:       baseURL,
		ClientID:      "test-client",
		PrivateKeyPEM: alterraTestPrivateKeyPEM(t),
	})
	if err != nil {
		t.Fatalf("new alterra client: %v", err)
	}

	return client
}

func TestApplyProviderTraceStoresInitialOnlyOnce(t *testing.T) {
	t.Parallel()

	trx := &models.Transaction{}

	first := &ProviderResponse{
		RawResponse: []byte(`{"response_code":"10","status":"Pending"}`),
		HTTPStatus:  http.StatusCreated,
	}
	second := &ProviderResponse{
		RawResponse: []byte(`{"response_code":"00","status":"Success"}`),
		HTTPStatus:  http.StatusOK,
	}

	applyProviderTrace(trx, first)
	applyProviderTrace(trx, second)

	if got := string(trx.ProviderInitialResponse); got != `{"response_code":"10","status":"Pending"}` {
		t.Fatalf("ProviderInitialResponse = %q", got)
	}
	if got := string(trx.ProviderResponse); got != `{"response_code":"00","status":"Success"}` {
		t.Fatalf("ProviderResponse = %q", got)
	}
	if trx.ProviderInitialHTTPStatus == nil || *trx.ProviderInitialHTTPStatus != http.StatusCreated {
		t.Fatalf("ProviderInitialHTTPStatus = %#v, want %d", trx.ProviderInitialHTTPStatus, http.StatusCreated)
	}
	if trx.ProviderHTTPStatus == nil || *trx.ProviderHTTPStatus != http.StatusOK {
		t.Fatalf("ProviderHTTPStatus = %#v, want %d", trx.ProviderHTTPStatus, http.StatusOK)
	}
}

func TestAlterraCheckStatusPreservesRawTrace(t *testing.T) {
	t.Parallel()

	rawBody := `{"transaction_id":777,"type":"utility","created_at":1775754717,"updated_at":1775754817,"customer_id":"01428800700","customer_name":"PLN TEST","order_id":"UAT-PLN-0001","price":50000,"status":"Success","response_code":"00","amount":50000,"admin":0,"product":{"product_id":25,"product_code":"25","type":"utility","label":"PLN 50.000","operator":"pln","nominal":50000,"price":50000,"enabled":true},"data":{"reference_no":"REF-777"}}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v5/transaction/777" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, rawBody)
	}))
	defer server.Close()

	client := alterraTestClient(t, server.URL+"/api")
	provider := NewAlterraProviderClient(client, nil)

	resp, err := provider.CheckStatus(context.Background(), "777")
	if err != nil {
		t.Fatalf("CheckStatus() error = %v", err)
	}

	if resp.HTTPStatus != http.StatusOK {
		t.Fatalf("HTTPStatus = %d, want %d", resp.HTTPStatus, http.StatusOK)
	}
	if got := string(resp.RawResponse); got != rawBody {
		t.Fatalf("RawResponse = %q, want %q", got, rawBody)
	}
	if resp.ProviderRefID != "777" {
		t.Fatalf("ProviderRefID = %q, want %q", resp.ProviderRefID, "777")
	}
	if resp.RC != "00" || resp.Status != "Success" {
		t.Fatalf("unexpected response metadata: RC=%q status=%q", resp.RC, resp.Status)
	}
	if resp.ResponseTime < 0 || resp.ResponseTime > 5*time.Second {
		t.Fatalf("unexpected response time: %s", resp.ResponseTime)
	}
}
