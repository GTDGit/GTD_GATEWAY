package alterra

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
)

func testPrivateKeyPEM(t *testing.T) string {
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

func testClient(t *testing.T, baseURL string) *Client {
	t.Helper()

	client, err := NewClient(Config{
		BaseURL:       baseURL,
		ClientID:      "test-client",
		PrivateKeyPEM: testPrivateKeyPEM(t),
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	return client
}

func TestBuildRequestURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		base string
		path string
		want string
	}{
		{
			name: "base without api suffix",
			base: "https://example.com",
			path: "/api/v5/transaction/purchase",
			want: "https://example.com/api/v5/transaction/purchase",
		},
		{
			name: "base already includes api suffix",
			base: "https://example.com/api",
			path: "/api/v5/transaction/purchase",
			want: "https://example.com/api/v5/transaction/purchase",
		},
		{
			name: "base with trailing slash",
			base: "https://example.com/api/",
			path: "/api/v5/transaction/777",
			want: "https://example.com/api/v5/transaction/777",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := buildRequestURL(tc.base, tc.path); got != tc.want {
				t.Fatalf("buildRequestURL() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPurchasePreservesBusinessErrorResponse(t *testing.T) {
	t.Parallel()

	rawBody := `{"transaction_id":1907470,"type":"mobile","created_at":1775754717,"updated_at":1775754717,"customer_id":"878891149161214","order_id":"GRB-20260410-296462","price":49000,"status":"Failed","response_code":"20","amount":0,"product":{"product_id":11,"product_code":"11","type":"mobile","label":"XL Rp. 50,000","operator":"xl","nominal":50000,"price":49000,"enabled":false},"data":null,"error":{"code":"406","message":"Invalid parameter"}}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v5/transaction/purchase" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotAcceptable)
		_, _ = io.WriteString(w, rawBody)
	}))
	defer server.Close()

	client := testClient(t, server.URL+"/api")
	resp, err := client.Purchase(context.Background(), "878891149161214", 11, "GRB-20260410-296462", nil)
	if err != nil {
		t.Fatalf("Purchase() error = %v", err)
	}

	if resp.HTTPStatus != http.StatusNotAcceptable {
		t.Fatalf("HTTPStatus = %d, want %d", resp.HTTPStatus, http.StatusNotAcceptable)
	}
	if resp.ResponseCode != "20" {
		t.Fatalf("ResponseCode = %q, want %q", resp.ResponseCode, "20")
	}
	if resp.Error == nil || resp.Error.Message != "Invalid parameter" {
		t.Fatalf("Error = %#v, want message %q", resp.Error, "Invalid parameter")
	}
	if got := string(resp.RawResponse); got != rawBody {
		t.Fatalf("RawResponse = %q, want %q", got, rawBody)
	}
}

func TestPurchasePreservesErrorEnvelopeOnHTTP403(t *testing.T) {
	t.Parallel()

	rawBody := `{"error":{"code":"403","message":"Product unavailable"}}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, rawBody)
	}))
	defer server.Close()

	client := testClient(t, server.URL)
	resp, err := client.Purchase(context.Background(), "08123456789", 27, "ORD-403", nil)
	if err != nil {
		t.Fatalf("Purchase() error = %v", err)
	}

	if resp.HTTPStatus != http.StatusForbidden {
		t.Fatalf("HTTPStatus = %d, want %d", resp.HTTPStatus, http.StatusForbidden)
	}
	if resp.Error == nil || resp.Error.Code != "403" || resp.Error.Message != "Product unavailable" {
		t.Fatalf("Error = %#v, want code/message 403/Product unavailable", resp.Error)
	}
	if got := string(resp.RawResponse); got != rawBody {
		t.Fatalf("RawResponse = %q, want %q", got, rawBody)
	}
}
