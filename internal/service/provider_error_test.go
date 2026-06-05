package service

import (
	"errors"
	"testing"

	"github.com/GTDGit/gtd_gateway/internal/models"
)

func TestProviderResponseFromErrorUsesAlterraHTTPCode(t *testing.T) {
	t.Parallel()

	resp := providerResponseFromError(string(models.ProviderAlterra), ProviderFailurePhaseInitialPayment, errors.New("http error: 403"))
	if resp == nil {
		t.Fatal("expected provider response")
	}
	if resp.RC != "403" {
		t.Fatalf("RC = %q, want %q", resp.RC, "403")
	}
	if resp.PublicCode != ProviderFailureProviderUnavailable {
		t.Fatalf("PublicCode = %q, want %q", resp.PublicCode, ProviderFailureProviderUnavailable)
	}
	if resp.Pending {
		t.Fatal("expected failed response, got pending")
	}
}

func TestProviderResponseFromErrorKeepsKiosbankTransportPending(t *testing.T) {
	t.Parallel()

	resp := providerResponseFromError(string(models.ProviderKiosbank), ProviderFailurePhaseInitialPayment, errors.New("request failed: tls handshake timeout"))
	if resp == nil {
		t.Fatal("expected provider response")
	}
	if !resp.Pending {
		t.Fatal("expected pending response")
	}
	if resp.Message != "Transaction is being processed" {
		t.Fatalf("Message = %q, want %q", resp.Message, "Transaction is being processed")
	}
}
