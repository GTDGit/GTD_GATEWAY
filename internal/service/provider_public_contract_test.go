package service

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/GTDGit/gtd_gateway/internal/models"
)

func TestCanonicalFailureForResponseAlterraProductClosed(t *testing.T) {
	t.Parallel()

	raw, _ := json.Marshal(map[string]any{
		"error": map[string]any{
			"code":    "403:product_closed",
			"message": "Product closed",
		},
	})
	resp := &ProviderResponse{
		Status:      string(models.StatusFailed),
		RC:          "403:product_closed",
		Message:     "Product closed",
		HTTPStatus:  400,
		RawResponse: raw,
	}

	failure := CanonicalFailureForResponse(string(models.ProviderAlterra), ProviderFailurePhaseInitialPayment, resp)
	if failure.Code != ProviderFailureProductUnavailable {
		t.Fatalf("Code = %q, want %q", failure.Code, ProviderFailureProductUnavailable)
	}
	if failure.HTTPStatus != 503 {
		t.Fatalf("HTTPStatus = %d, want 503", failure.HTTPStatus)
	}
}

func TestCanonicalFailureForResponseKiosbankInquiryInvalidProduct(t *testing.T) {
	t.Parallel()

	resp := &ProviderResponse{
		Status:  string(models.StatusFailed),
		RC:      "15",
		Message: "PRODUK TIDAK DIKENAL",
	}

	failure := CanonicalFailureForResponse(string(models.ProviderKiosbank), ProviderFailurePhaseInquiry, resp)
	if failure.Code != ProviderFailureUpstreamRequestInvalid {
		t.Fatalf("Code = %q, want %q", failure.Code, ProviderFailureUpstreamRequestInvalid)
	}
	if failure.HTTPStatus != 502 {
		t.Fatalf("HTTPStatus = %d, want 502", failure.HTTPStatus)
	}
}

func TestBuildFinalFailureResponseFromAttemptsUsesPrecedence(t *testing.T) {
	t.Parallel()

	attempts := []ProviderAttempt{
		{
			Provider: &models.ProviderOption{ProviderCode: models.ProviderAlterra},
			Response: &ProviderResponse{Status: string(models.StatusFailed), RC: "99", Message: "General Error"},
		},
		{
			Provider: &models.ProviderOption{ProviderCode: models.ProviderKiosbank},
			Response: &ProviderResponse{Status: string(models.StatusFailed), RC: "65", Message: "Nomor pelanggan salah"},
		},
	}

	resp := BuildFinalFailureResponseFromAttempts(attempts, ProviderFailurePhaseInitialPayment)
	if resp == nil {
		t.Fatal("expected response")
	}
	if resp.PublicCode != ProviderFailureInvalidCustomer {
		t.Fatalf("PublicCode = %q, want %q", resp.PublicCode, ProviderFailureInvalidCustomer)
	}
}

func TestProviderResponseFromErrorKiosbankInquiryTimeoutIsFailed(t *testing.T) {
	t.Parallel()

	resp := providerResponseFromError(string(models.ProviderKiosbank), ProviderFailurePhaseInquiry, errors.New("context deadline exceeded"))
	if resp == nil {
		t.Fatal("expected response")
	}
	if resp.Pending {
		t.Fatal("expected failed inquiry response")
	}
	if resp.PublicCode != ProviderFailureProviderTimeout {
		t.Fatalf("PublicCode = %q, want %q", resp.PublicCode, ProviderFailureProviderTimeout)
	}
}

func TestSanitizePublicProviderDescriptionRemovesTransportLeak(t *testing.T) {
	t.Parallel()

	raw := json.RawMessage(`{"transport_error":"Post https://development.kiosbank.com:4432/Services/Inquiry: context deadline exceeded","phase":"inquiry"}`)
	sanitized := SanitizePublicProviderDescription(raw)
	if len(sanitized) == 0 {
		t.Fatal("expected sanitized payload")
	}
	if string(sanitized) != `{"phase":"inquiry"}` {
		t.Fatalf("Sanitized = %s, want %s", sanitized, `{"phase":"inquiry"}`)
	}
}
