package service

import (
	"net/http"
	"reflect"
	"testing"

	"github.com/GTDGit/gtd_gateway/pkg/alterra"
)

func TestAlterraResponseCodePrefersBusinessCode(t *testing.T) {
	t.Parallel()

	resp := &alterra.TransactionResponse{
		ResponseCode: "99",
		Error: &alterra.ErrorDetail{
			Code:    "406",
			Message: "Invalid parameter",
		},
		HTTPStatus: http.StatusNotAcceptable,
	}

	if got := alterraResponseCode(resp); got != "99" {
		t.Fatalf("alterraResponseCode() = %q, want %q", got, "99")
	}
}

func TestAlterraResponseCodeFallsBackToErrorCode(t *testing.T) {
	t.Parallel()

	resp := &alterra.TransactionResponse{
		Error: &alterra.ErrorDetail{
			Code:    "403",
			Message: "Product unavailable",
		},
		HTTPStatus: http.StatusForbidden,
	}

	if got := alterraResponseCode(resp); got != "403" {
		t.Fatalf("alterraResponseCode() = %q, want %q", got, "403")
	}
}

func TestAlterraResponseCodeFallsBackToTopLevelCode(t *testing.T) {
	t.Parallel()

	resp := &alterra.TransactionResponse{
		Code:       "403:product_closed",
		HTTPStatus: http.StatusBadRequest,
	}

	if got := alterraResponseCode(resp); got != "403:product_closed" {
		t.Fatalf("alterraResponseCode() = %q, want %q", got, "403:product_closed")
	}
}

func TestAlterraResponseMessageFallsBackToRCDescription(t *testing.T) {
	t.Parallel()

	resp := &alterra.TransactionResponse{
		ResponseCode: "99",
	}

	if got := alterraResponseMessage(resp, alterraResponseCode(resp)); got != "General Error" {
		t.Fatalf("alterraResponseMessage() = %q, want %q", got, "General Error")
	}
}

func TestAlterraResponseMessageFallsBackToTopLevelMessage(t *testing.T) {
	t.Parallel()

	resp := &alterra.TransactionResponse{
		Code:    "403:product_closed",
		Message: "Product closed",
	}

	if got := alterraResponseMessage(resp, alterraResponseCode(resp)); got != "Product closed" {
		t.Fatalf("alterraResponseMessage() = %q, want %q", got, "Product closed")
	}
}

func TestSanitizeAlterraExtraStripsInternalPricingFields(t *testing.T) {
	t.Parallel()

	input := map[string]any{
		"reference_no":   "REF-123",
		"payment_period": "02",
		"admin":          2500,
		"commission":     500,
	}

	got := sanitizeAlterraExtra(input)
	want := map[string]any{
		"reference_no":   "REF-123",
		"payment_period": "02",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sanitizeAlterraExtra() = %#v, want %#v", got, want)
	}
}

func TestBuildAlterraPaymentDataKeepsReferenceNoAndPaymentPeriod(t *testing.T) {
	t.Parallel()

	input := map[string]any{
		"reference_no":   "REF-123",
		"payment_period": "02",
		"admin":          2500,
	}

	got := buildAlterraPaymentData(input)
	want := map[string]any{
		"reference_no":   "REF-123",
		"payment_period": "02",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildAlterraPaymentData() = %#v, want %#v", got, want)
	}
}
