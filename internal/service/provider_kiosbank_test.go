package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/GTDGit/gtd_gateway/internal/models"
	"github.com/GTDGit/gtd_gateway/pkg/kiosbank"
)

func TestParseKiosbankDataBPJSInquiry(t *testing.T) {
	t.Parallel()

	raw := json.RawMessage(`{
		"idPelanggan":"8888800020839792",
		"nama":"NURHASANAH",
		"premi":"25500",
		"adminBank":"2500",
		"total":"79000"
	}`)

	parsed := parseKiosbankData(raw)
	if parsed.CustomerName != "NURHASANAH" {
		t.Fatalf("CustomerName = %q", parsed.CustomerName)
	}
	if parsed.Admin != 2500 {
		t.Fatalf("Admin = %d, want %d", parsed.Admin, 2500)
	}
	if parsed.Amount != 76500 {
		t.Fatalf("Amount = %d, want %d", parsed.Amount, 76500)
	}
}

func TestConvertKiosbankPaymentResponseFallsBackToRequestedAmount(t *testing.T) {
	t.Parallel()

	provider := &KiosbankProviderClient{}
	resp := &kiosbank.PaymentResponse{
		BaseResponse: kiosbank.BaseResponse{RC: kiosbank.RCSuccess},
		ProductID:    "100302",
		Data: json.RawMessage(`{
			"AB":"000000003000",
			"TT":"000000003000",
			"TK":"66806385904760518886",
			"NM":"JAMALUDDIN"
		}`),
	}

	converted := provider.convertPaymentResponse(resp, "123456789012", 20000, 3000, time.Second)
	if converted.Amount != 20000 {
		t.Fatalf("Amount = %d, want %d", converted.Amount, 20000)
	}
	if converted.Admin != 3000 {
		t.Fatalf("Admin = %d, want %d", converted.Admin, 3000)
	}
	if converted.SerialNumber != "66806385904760518886" {
		t.Fatalf("SerialNumber = %q", converted.SerialNumber)
	}
	if converted.ProviderRefID != "123456789012" {
		t.Fatalf("ProviderRefID = %q", converted.ProviderRefID)
	}
}

func TestBuildKiosbankCheckStatusInputUsesLoggedRequestData(t *testing.T) {
	t.Parallel()

	logRequest := json.RawMessage(`{
		"provider":"kiosbank",
		"ref_id":"760864752227",
		"amount":149000,
		"admin":0,
		"extra":{
			"noHanphone":"08123",
			"nama":"(2101TN) Spotv Timnas",
			"kode":"2101TN"
		}
	}`)

	trx := &models.Transaction{
		TransactionID: "GRB-20260413-000001",
		ProviderRefID: func() *string { s := "760864752227"; return &s }(),
		CreatedAt:     time.Now(),
	}
	logs := []models.TransactionLog{{Request: logRequest}}

	input := buildKiosbankCheckStatusInput(trx, logs)
	if input.ReferenceID != "760864752227" {
		t.Fatalf("ReferenceID = %q", input.ReferenceID)
	}
	if input.Tagihan != 149000 || input.Admin != 0 || input.Total != 149000 {
		t.Fatalf("unexpected amounts: %#v", input)
	}
	if input.NoHandphone != "08123" || input.Nama != "(2101TN) Spotv Timnas" || input.Kode != "2101TN" {
		t.Fatalf("unexpected extra fields: %#v", input)
	}
}

func TestBuildKiosbankCheckStatusInputPrefersWireRequest(t *testing.T) {
	t.Parallel()

	logRequest := json.RawMessage(`{
		"provider":"kiosbank",
		"ref_id":"ignored",
		"wire_request":{
			"referenceID":"760864752227",
			"tagihan":149000,
			"admin":2500,
			"total":151500,
			"noHanphone":"08123",
			"nama":"(2101TN) Spotv Timnas",
			"kode":"2101TN"
		}
	}`)

	trx := &models.Transaction{
		TransactionID: "GRB-20260413-000001",
		ProviderRefID: func() *string { s := "760864752227"; return &s }(),
		CreatedAt:     time.Now(),
	}
	logs := []models.TransactionLog{{Request: logRequest}}

	input := buildKiosbankCheckStatusInput(trx, logs)
	if input.ReferenceID != "760864752227" {
		t.Fatalf("ReferenceID = %q", input.ReferenceID)
	}
	if input.Tagihan != 149000 || input.Admin != 2500 || input.Total != 151500 {
		t.Fatalf("unexpected amounts: %#v", input)
	}
}

func TestKiosbankValidationMatchesLiveDocs(t *testing.T) {
	t.Parallel()

	provider := &KiosbankProviderClient{}

	inquiryResp, err := provider.Inquiry(context.Background(), &ProviderRequest{
		RefID:      "GRB-20260414-000001",
		SKUCode:    "900001",
		CustomerNo: "1234567890",
		Type:       ProviderTrxInquiry,
	})
	if err != nil {
		t.Fatalf("Inquiry() error = %v", err)
	}
	if inquiryResp.RC != kiosbank.RCFormatError {
		t.Fatalf("Inquiry RC = %q, want %q", inquiryResp.RC, kiosbank.RCFormatError)
	}

	paymentResp, err := provider.Payment(context.Background(), &ProviderRequest{
		RefID:      "760864752227",
		SKUCode:    "900001",
		CustomerNo: "1234567890",
		Amount:     10000,
		Type:       ProviderTrxPayment,
		Extra:      map[string]any{"admin": 2500},
	})
	if err != nil {
		t.Fatalf("Payment() error = %v", err)
	}
	if paymentResp.Message != "missing required Kiosbank field: noHandphone" {
		t.Fatalf("unexpected payment validation message: %q", paymentResp.Message)
	}

	packageResp, err := provider.Payment(context.Background(), &ProviderRequest{
		RefID:      "760864752228",
		SKUCode:    "550031",
		CustomerNo: "1234567890",
		Amount:     10000,
		Type:       ProviderTrxPayment,
		Extra:      map[string]any{"admin": 2500},
	})
	if err != nil {
		t.Fatalf("Payment(package) error = %v", err)
	}
	if packageResp.Message != "missing required Kiosbank field: nama" {
		t.Fatalf("unexpected package validation message: %q", packageResp.Message)
	}
}

func TestConvertKiosbankAsyncPaymentResponseUsesAsyncRCClassification(t *testing.T) {
	t.Parallel()

	provider := &KiosbankProviderClient{}
	resp := &kiosbank.PaymentResponse{
		BaseResponse: kiosbank.BaseResponse{RC: kiosbank.RCProcessing},
		ProductID:    "550031",
		Data: json.RawMessage(`{
			"harga":"50000",
			"status":"Transaksi sedang diproses",
			"kodeVoucher":"VG-123"
		}`),
	}

	converted := provider.convertAsyncPaymentResponse(resp, "123456789012", 50000, 0, time.Second)
	if !converted.Pending {
		t.Fatalf("Pending = %v, want true", converted.Pending)
	}
	if converted.Status != "Pending" {
		t.Fatalf("Status = %q, want Pending", converted.Status)
	}
	if converted.ProviderStatus != "Transaksi sedang diproses" {
		t.Fatalf("ProviderStatus = %q", converted.ProviderStatus)
	}
	if converted.SerialNumber != "VG-123" {
		t.Fatalf("SerialNumber = %q, want VG-123", converted.SerialNumber)
	}
}
