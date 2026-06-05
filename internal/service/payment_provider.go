package service

import (
	"context"
	"encoding/json"
	"time"

	"github.com/GTDGit/gtd_gateway/internal/models"
)

// PaymentCreateRequest is the unified provider-agnostic create payload.
type PaymentCreateRequest struct {
	Type           models.PaymentType
	Code           string
	BankCode       string // VA bank code, e.g. "014", "451"
	PartnerRef     string // public PaymentID (PAY-…)
	Amount         int64
	Fee            int64
	TotalAmount    int64
	ExpiredAt      time.Time
	Description    string
	CustomerName   string
	CustomerEmail  string
	CustomerPhone  string
	CallbackURL    string
	ReturnURL      string
	Metadata       map[string]any
}

// PaymentDetailNormalized is the union shape copied into payment.payment_detail.
type PaymentDetailNormalized struct {
	// VA
	BankCode            string `json:"bankCode,omitempty"`
	BankName            string `json:"bankName,omitempty"`
	VANumber            string `json:"vaNumber,omitempty"`
	AccountName         string `json:"accountName,omitempty"`
	// EWALLET/QRIS
	CheckoutURL string `json:"checkoutUrl,omitempty"`
	MobileWebURL string `json:"mobileWebUrl,omitempty"`
	Deeplink    string `json:"deeplink,omitempty"`
	QRCodeURL   string `json:"qrCodeUrl,omitempty"`
	QRString    string `json:"qrString,omitempty"`
	QRImageURL  string `json:"qrImageUrl,omitempty"`
	// RETAIL
	RetailName  string `json:"retailName,omitempty"`
	PaymentCode string `json:"paymentCode,omitempty"`
	// Shared
	ProviderReferenceNo string `json:"providerReferenceNo,omitempty"`
	Provider            string `json:"provider,omitempty"`
}

// PaymentCreateResponse is returned by all provider adapters.
type PaymentCreateResponse struct {
	ProviderRef string
	Normalized  PaymentDetailNormalized
	RawResponse json.RawMessage
}

// PaymentInquiryResult reports the latest known status from a provider.
type PaymentInquiryResult struct {
	Status      models.PaymentStatus
	ProviderRef string
	PaidAmount  int64
	RawResponse json.RawMessage
}

// PaymentCancelResult reports the outcome of a cancel call.
type PaymentCancelResult struct {
	Cancelled   bool
	RawResponse json.RawMessage
}

// PaymentRefundResult reports the outcome of a refund call.
type PaymentRefundResult struct {
	ProviderRef string
	Succeeded   bool
	RawResponse json.RawMessage
}

// PaymentProviderClient is implemented by provider-specific adapters.
type PaymentProviderClient interface {
	Code() models.PaymentProvider
	CreatePayment(ctx context.Context, method *models.PaymentMethod, req *PaymentCreateRequest) (*PaymentCreateResponse, error)
	InquiryPayment(ctx context.Context, payment *models.Payment) (*PaymentInquiryResult, error)
	CancelPayment(ctx context.Context, payment *models.Payment, reason string) (*PaymentCancelResult, error)
	RefundPayment(ctx context.Context, payment *models.Payment, refund *models.Refund) (*PaymentRefundResult, error)
}
