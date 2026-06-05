package main

import (
	"testing"

	"github.com/GTDGit/gtd_gateway/internal/config"
)

func TestKiosbankClientConfigUsesDevelopmentOverrides(t *testing.T) {
	t.Parallel()

	cfg := config.KiosbankConfig{
		BaseURL:                       "https://prod.example",
		MerchantID:                    "PROD",
		MerchantName:                  "Prod Merchant",
		CounterID:                     "1",
		AccountID:                     "A",
		Mitra:                         "DJI",
		Username:                      "prod-user",
		Password:                      "prod-pass",
		InsecureSkipVerify:            true,
		DevelopmentURL:                "https://dev.example",
		DevelopmentInsecureSkipVerify: true,
		DevelopmentCreds: config.KiosbankCredentialConfig{
			MerchantID:   "DEV",
			MerchantName: "Dev Merchant",
			CounterID:    "2",
			AccountID:    "B",
			Mitra:        "KB",
			Username:     "dev-user",
			Password:     "dev-pass",
		},
	}

	prod := kiosbankClientConfig(cfg, false)
	dev := kiosbankClientConfig(cfg, true)

	if prod.BaseURL != "https://prod.example" || prod.Username != "prod-user" || prod.MerchantName != "Prod Merchant" || !prod.InsecureSkipVerify {
		t.Fatalf("unexpected prod config: %#v", prod)
	}
	if dev.BaseURL != "https://dev.example" || dev.Username != "dev-user" || dev.MerchantID != "DEV" || dev.MerchantName != "Dev Merchant" || !dev.InsecureSkipVerify {
		t.Fatalf("unexpected dev config: %#v", dev)
	}
}

func TestKiosbankClientConfigFallsBackToProductionCredentials(t *testing.T) {
	t.Parallel()

	cfg := config.KiosbankConfig{
		BaseURL:        "https://prod.example",
		DevelopmentURL: "https://dev.example",
		MerchantID:     "PROD",
		MerchantName:   "Prod Merchant",
		CounterID:      "1",
		AccountID:      "A",
		Mitra:          "DJI",
		Username:       "prod-user",
		Password:       "prod-pass",
	}

	dev := kiosbankClientConfig(cfg, true)
	if dev.Username != "prod-user" || dev.Password != "prod-pass" || dev.MerchantID != "PROD" || dev.MerchantName != "Prod Merchant" {
		t.Fatalf("unexpected fallback dev config: %#v", dev)
	}
}

func TestKiosbankClientConfigFallsBackMerchantNameToMerchantID(t *testing.T) {
	t.Parallel()

	cfg := config.KiosbankConfig{
		BaseURL:        "https://prod.example",
		DevelopmentURL: "https://dev.example",
		MerchantID:     "PROD",
		CounterID:      "1",
		AccountID:      "A",
		Mitra:          "DJI",
		Username:       "prod-user",
		Password:       "prod-pass",
		DevelopmentCreds: config.KiosbankCredentialConfig{
			MerchantID: "DEV",
			Username:   "dev-user",
			Password:   "dev-pass",
		},
	}

	prod := kiosbankClientConfig(cfg, false)
	dev := kiosbankClientConfig(cfg, true)

	if prod.MerchantName != "PROD" {
		t.Fatalf("prod MerchantName = %q, want PROD", prod.MerchantName)
	}
	if dev.MerchantName != "DEV" {
		t.Fatalf("dev MerchantName = %q, want DEV", dev.MerchantName)
	}
}
