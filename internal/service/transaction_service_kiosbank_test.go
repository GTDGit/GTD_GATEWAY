package service

import (
	"testing"

	"github.com/GTDGit/gtd_gateway/internal/models"
)

func TestBuildProviderLogRequestIncludesKiosbankWireRequest(t *testing.T) {
	t.Parallel()

	req := &ProviderRequest{
		RefID:      "760864752227",
		SKUCode:    "550031",
		CustomerNo: "08123456789",
		Amount:     149000,
		Type:       ProviderTrxPayment,
		Extra: map[string]any{
			"admin":       2500,
			"noHandphone": "08123",
			"nama":        "(2101TN) Spotv Timnas",
			"kode":        "2101TN",
		},
	}
	opt := &models.ProviderOption{
		ProviderCode:    models.ProviderKiosbank,
		ProviderSKUCode: "550031",
		Admin:           2500,
	}

	logRequest := buildProviderLogRequest(opt, req)
	wireRequest, ok := logRequest["wire_request"].(map[string]any)
	if !ok {
		t.Fatalf("wire_request missing: %#v", logRequest)
	}
	if wireRequest["referenceID"] != "760864752227" {
		t.Fatalf("referenceID = %#v", wireRequest["referenceID"])
	}
	if wireRequest["tagihan"] != 149000 {
		t.Fatalf("tagihan = %#v", wireRequest["tagihan"])
	}
	if wireRequest["admin"] != 2500 {
		t.Fatalf("admin = %#v", wireRequest["admin"])
	}
	if wireRequest["total"] != 151500 {
		t.Fatalf("total = %#v", wireRequest["total"])
	}
	if wireRequest["noHanphone"] != "08123" {
		t.Fatalf("noHanphone = %#v", wireRequest["noHanphone"])
	}
}

func TestBuildProviderLogRequestSanitizesAlterraPaymentWireRequest(t *testing.T) {
	t.Parallel()

	req := &ProviderRequest{
		RefID:      "GRB-ALT-001",
		SKUCode:    "34",
		CustomerNo: "0000001430071801",
		Type:       ProviderTrxPayment,
		Extra: map[string]any{
			"admin":          0,
			"commission":     0,
			"reference_no":   "68752409",
			"payment_period": "01",
		},
	}
	opt := &models.ProviderOption{
		ProviderCode:    models.ProviderAlterra,
		ProviderSKUCode: "34",
	}

	logRequest := buildProviderLogRequest(opt, req)

	extra, ok := logRequest["extra"].(map[string]any)
	if !ok {
		t.Fatalf("extra missing: %#v", logRequest)
	}
	if extra["reference_no"] != "68752409" {
		t.Fatalf("reference_no = %#v", extra["reference_no"])
	}
	if extra["payment_period"] != "01" {
		t.Fatalf("payment_period = %#v", extra["payment_period"])
	}

	wireRequest, ok := logRequest["wire_request"].(map[string]any)
	if !ok {
		t.Fatalf("wire_request missing: %#v", logRequest)
	}
	data, ok := wireRequest["data"].(map[string]any)
	if !ok {
		t.Fatalf("wire_request.data missing: %#v", wireRequest)
	}
	if data["reference_no"] != "68752409" {
		t.Fatalf("reference_no = %#v", data["reference_no"])
	}
	if data["payment_period"] != "01" {
		t.Fatalf("payment_period = %#v", data["payment_period"])
	}
}
