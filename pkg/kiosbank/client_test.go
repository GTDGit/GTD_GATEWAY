package kiosbank

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestGenerateReferenceID(t *testing.T) {
	t.Parallel()

	seen := make(map[string]struct{})
	for i := 0; i < 64; i++ {
		refID := GenerateReferenceID()
		if !IsNumericReferenceID(refID) {
			t.Fatalf("reference ID %q is not 12-digit numeric", refID)
		}
		if _, ok := seen[refID]; ok {
			t.Fatalf("duplicate reference ID generated: %q", refID)
		}
		seen[refID] = struct{}{}
	}
}

func TestGetPriceListPulsaRequestsAllPrefixes(t *testing.T) {
	t.Parallel()

	var (
		mu       sync.Mutex
		prefixes []string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			w.Header().Set("WWW-Authenticate", `Digest realm="test", nonce="nonce", opaque="opaque", qop="auth"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		switch r.URL.Path {
		case "/auth/Sign-On":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"rc":"00","SessionID":"SESSION"}`))
		case "/Services/getPulsa-Prabayar":
			var req PriceListRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			mu.Lock()
			prefixes = append(prefixes, req.PrefixID)
			mu.Unlock()

			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"rc":"00","record":[]}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL:      server.URL,
		MerchantID:   "MERCHANT",
		MerchantName: "Merchant Name",
		CounterID:    "1",
		AccountID:    "ACC",
		Mitra:        "DJI",
		Username:     "user",
		Password:     "pass",
	})

	if _, err := client.GetPriceListPulsa(context.Background()); err != nil {
		t.Fatalf("GetPriceListPulsa() error = %v", err)
	}

	sort.Strings(prefixes)
	expected := []string{"11", "21", "31", "41", "51", "81"}
	if len(prefixes) != len(expected) {
		t.Fatalf("prefix count = %d, want %d (%v)", len(prefixes), len(expected), prefixes)
	}
	for i, prefix := range expected {
		if prefixes[i] != prefix {
			t.Fatalf("prefixes[%d] = %q, want %q", i, prefixes[i], prefix)
		}
	}
}

func TestSignOnUsesLiveDocsContract(t *testing.T) {
	t.Parallel()

	var captured SignOnRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			w.Header().Set("WWW-Authenticate", `Digest realm="test", nonce="nonce", opaque="opaque", qop="auth"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		if r.URL.Path != "/auth/Sign-On" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}

		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"rc":"00","SessionID":"SESSION-LIVE"}`))
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL:      server.URL,
		MerchantID:   "MERCHANT",
		MerchantName: "Merchant Name",
		CounterID:    "1",
		AccountID:    "ACC",
		Mitra:        "DJI",
		Username:     "user",
		Password:     "pass",
	})

	resp, err := client.SignOn(context.Background())
	if err != nil {
		t.Fatalf("SignOn() error = %v", err)
	}
	if resp.SessionID != "SESSION-LIVE" {
		t.Fatalf("SessionID = %q, want SESSION-LIVE", resp.SessionID)
	}
	if captured.MerchantName != "Merchant Name" {
		t.Fatalf("MerchantName = %q, want Merchant Name", captured.MerchantName)
	}
}

func TestInquiryRetriesAfterSessionExpiredRC(t *testing.T) {
	t.Parallel()

	var (
		mu           sync.Mutex
		signOnCalls  int
		inquiryCalls int
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			w.Header().Set("WWW-Authenticate", `Digest realm="test", nonce="nonce", opaque="opaque", qop="auth"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/auth/Sign-On":
			mu.Lock()
			signOnCalls++
			current := signOnCalls
			mu.Unlock()
			_, _ = w.Write([]byte(`{"rc":"00","SessionID":"SESSION-` + string(rune('0'+current)) + `"}`))
		case "/Services/Inquiry":
			mu.Lock()
			inquiryCalls++
			current := inquiryCalls
			mu.Unlock()
			if current == 1 {
				_, _ = w.Write([]byte(`{"rc":"34","description":"Session expired","referenceID":"123456789012","data":{}}`))
				return
			}
			_, _ = w.Write([]byte(`{"rc":"00","description":"OK","referenceID":"123456789012","data":{"nama":"SAMPLE","total":"1000"}}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL:      server.URL,
		MerchantID:   "MERCHANT",
		MerchantName: "Merchant Name",
		CounterID:    "1",
		AccountID:    "ACC",
		Mitra:        "DJI",
		Username:     "user",
		Password:     "pass",
	})

	resp, err := client.Inquiry(context.Background(), "900001", "12345", "123456789012", "202601")
	if err != nil {
		t.Fatalf("Inquiry() error = %v", err)
	}
	if resp.RC != RCSuccess {
		t.Fatalf("Inquiry RC = %q, want %q", resp.RC, RCSuccess)
	}
	if signOnCalls != 2 {
		t.Fatalf("signOnCalls = %d, want 2", signOnCalls)
	}
	if inquiryCalls != 2 {
		t.Fatalf("inquiryCalls = %d, want 2", inquiryCalls)
	}
}

func TestSessionExpiryEndsSameDayWIB(t *testing.T) {
	t.Parallel()

	loc := time.FixedZone("WIB", 7*3600)
	now := time.Date(2026, time.April, 14, 10, 30, 0, 0, loc)
	expiry := sessionExpiry(now)

	expected := time.Date(2026, time.April, 14, 23, 59, 59, 0, loc)
	if !expiry.Equal(expected) {
		t.Fatalf("sessionExpiry() = %s, want %s", expiry, expected)
	}
}

func TestEnsureSessionAcceptsSessionIDWithoutRC(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			w.Header().Set("WWW-Authenticate", `Digest realm="test", nonce="nonce", opaque="opaque", qop="auth"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/auth/Sign-On":
			_, _ = w.Write([]byte(`{"SessionID":"SESSION-WITHOUT-RC"}`))
		case "/Services/getPulsa-Prabayar":
			_, _ = w.Write([]byte(`{"rc":"00","record":[]}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL:      server.URL,
		MerchantID:   "MERCHANT",
		MerchantName: "Merchant Name",
		CounterID:    "1",
		AccountID:    "ACC",
		Mitra:        "DJI",
		Username:     "user",
		Password:     "pass",
	})

	if _, err := client.GetPriceListPulsa(context.Background()); err != nil {
		t.Fatalf("GetPriceListPulsa() error = %v", err)
	}
}

func TestEnsureSessionFailsWithoutSessionID(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			w.Header().Set("WWW-Authenticate", `Digest realm="test", nonce="nonce", opaque="opaque", qop="auth"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"rc":"00"}`))
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL:      server.URL,
		MerchantID:   "MERCHANT",
		MerchantName: "Merchant Name",
		CounterID:    "1",
		AccountID:    "ACC",
		Mitra:        "DJI",
		Username:     "user",
		Password:     "pass",
	})

	_, err := client.GetPriceListPulsa(context.Background())
	if err == nil {
		t.Fatal("expected error when SessionID is missing")
	}
	if !strings.Contains(err.Error(), "missing SessionID") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetPriceListUsesLiveDocsGeneralEndpoint(t *testing.T) {
	t.Parallel()

	var (
		mu       sync.Mutex
		prefixes []string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			w.Header().Set("WWW-Authenticate", `Digest realm="test", nonce="nonce", opaque="opaque", qop="auth"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/auth/Sign-On":
			_, _ = w.Write([]byte(`{"SessionID":"SESSION-LIVE"}`))
		case "/Services/getHargaByProductID":
			var req PriceListRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			if req.PrefixID == "" {
				t.Fatal("expected prefixID in general price list request")
			}
			mu.Lock()
			prefixes = append(prefixes, req.PrefixID)
			mu.Unlock()

			_, _ = w.Write([]byte(`{"rc":"00","record":[{"code":"` + req.PrefixID + `-1","name":"Sample","price":1000}]}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL:      server.URL,
		MerchantID:   "MERCHANT",
		MerchantName: "Merchant Name",
		CounterID:    "1",
		AccountID:    "ACC",
		Mitra:        "DJI",
		Username:     "user",
		Password:     "pass",
	})

	resp, err := client.GetPriceList(context.Background())
	if err != nil {
		t.Fatalf("GetPriceList() error = %v", err)
	}
	if len(resp.Record) != len(generalPriceListPrefixes) {
		t.Fatalf("record count = %d, want %d", len(resp.Record), len(generalPriceListPrefixes))
	}

	sort.Strings(prefixes)
	expected := make([]string, 0, len(generalPriceListPrefixes))
	for _, prefix := range generalPriceListPrefixes {
		expected = append(expected, prefix.PrefixID)
	}
	sort.Strings(expected)
	for i := range expected {
		if prefixes[i] != expected[i] {
			t.Fatalf("prefixes[%d] = %q, want %q", i, prefixes[i], expected[i])
		}
	}

	for _, item := range resp.Record {
		if item.Category == "" {
			t.Fatalf("expected category to be populated for %+v", item)
		}
		if item.Status != "AKTIF" {
			t.Fatalf("expected default AKTIF status for %+v", item)
		}
		if item.Price != "1000" {
			t.Fatalf("expected numeric price to normalize to string, got %+v", item)
		}
	}

	expectedPulsePrefixes := []string{"AAE.AAAA", "AAE.AAAB", "AAE.AAAC", "AAE.AAAD", "AAE.AAAE", "AAE.AAAF", "AAE.AAAG", "AAE.AAAH"}
	for _, prefix := range expectedPulsePrefixes {
		found := false
		for _, configured := range generalPriceListPrefixes {
			if configured.PrefixID == prefix && configured.Category == "PULSA" {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected general price list prefixes to include %s for PULSA", prefix)
		}
	}
}

func TestClientSupportsInsecureSkipVerifyForBrokenTLS(t *testing.T) {
	t.Parallel()

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			w.Header().Set("WWW-Authenticate", `Digest realm="test", nonce="nonce", opaque="opaque", qop="auth"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/auth/Sign-On":
			_, _ = w.Write([]byte(`{"SessionID":"TLS-SESSION"}`))
		case "/Services/getPulsa-Prabayar":
			_, _ = w.Write([]byte(`{"rc":"00","record":[]}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	secureClient := NewClient(Config{
		BaseURL:      server.URL,
		MerchantID:   "MERCHANT",
		MerchantName: "Merchant Name",
		CounterID:    "1",
		AccountID:    "ACC",
		Mitra:        "DJI",
		Username:     "user",
		Password:     "pass",
	})

	if _, err := secureClient.GetPriceListPulsa(context.Background()); err == nil {
		t.Fatal("expected TLS verification error without insecure skip verify")
	}

	insecureClient := NewClient(Config{
		BaseURL:            server.URL,
		MerchantID:         "MERCHANT",
		MerchantName:       "Merchant Name",
		CounterID:          "1",
		AccountID:          "ACC",
		Mitra:              "DJI",
		Username:           "user",
		Password:           "pass",
		InsecureSkipVerify: true,
	})

	if _, err := insecureClient.GetPriceListPulsa(context.Background()); err != nil {
		t.Fatalf("GetPriceListPulsa() with insecure TLS error = %v", err)
	}
}
