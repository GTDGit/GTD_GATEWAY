package bri

import "testing"

func TestDeriveBRIChannelIDUsesPartnerIDWhenMissing(t *testing.T) {
	got := deriveBRIChannelID("", "22210")
	if got != "22210" {
		t.Fatalf("expected channel ID to fallback to partner ID, got %q", got)
	}
}

func TestDeriveBRIChannelIDKeepsExplicitValidValue(t *testing.T) {
	got := deriveBRIChannelID("54321", "22210")
	if got != "54321" {
		t.Fatalf("expected explicit channel ID to be preserved, got %q", got)
	}
}

func TestDeriveBRIVAPartnerServiceIDPadsToEightChars(t *testing.T) {
	got := deriveBRIVAPartnerServiceID("", "22210")
	if got != "   22210" {
		t.Fatalf("expected left-padded partnerServiceId, got %q", got)
	}
}

func TestGenerateSNAPExternalIDUsesNineDigitsForBRIVA(t *testing.T) {
	got := generateSNAPExternalID("/snap/v1.0/transfer-va/create-va")
	if len(got) != 9 {
		t.Fatalf("expected 9-digit external ID, got %q", got)
	}
	if !isFixedNumeric(got, 9) {
		t.Fatalf("expected external ID to be numeric, got %q", got)
	}
}

func TestGenerateSNAPExternalIDUsesThirtySixCharsForIntrabank(t *testing.T) {
	got := generateSNAPExternalID("/intrabank/snap/v2.0/account-inquiry-internal")
	if len(got) != 36 {
		t.Fatalf("expected 36-char external ID, got %q", got)
	}
	for _, r := range got {
		if r < '0' || r > '9' {
			t.Fatalf("expected numeric external ID, got %q", got)
		}
	}
}
