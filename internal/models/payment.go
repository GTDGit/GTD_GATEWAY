package models

import "time"

// ----------------------------------------------------------------------------
// Enums (match migration 000001 enum definitions).
// ----------------------------------------------------------------------------

type PaymentType string

const (
	PaymentTypeVA      PaymentType = "VA"
	PaymentTypeEwallet PaymentType = "EWALLET"
	PaymentTypeQRIS    PaymentType = "QRIS"
	PaymentTypeRetail  PaymentType = "RETAIL"
)

type PaymentStatus string

const (
	PaymentStatusPending       PaymentStatus = "Pending"
	PaymentStatusPaid          PaymentStatus = "Paid"
	PaymentStatusExpired       PaymentStatus = "Expired"
	PaymentStatusCancelled     PaymentStatus = "Cancelled"
	PaymentStatusFailed        PaymentStatus = "Failed"
	PaymentStatusRefunded      PaymentStatus = "Refunded"
	PaymentStatusPartialRefund PaymentStatus = "Partial_Refund"
)

// IsFinal reports whether the status is terminal (callbacks must not fire again).
func (s PaymentStatus) IsFinal() bool {
	switch s {
	case PaymentStatusPaid, PaymentStatusExpired, PaymentStatusCancelled,
		PaymentStatusFailed, PaymentStatusRefunded, PaymentStatusPartialRefund:
		return true
	}
	return false
}

type PaymentProvider string

const (
	ProviderPakailink     PaymentProvider = "pakailink"
	ProviderDanaDirect    PaymentProvider = "dana_direct"
	ProviderMidtrans      PaymentProvider = "midtrans"
	ProviderXendit        PaymentProvider = "xendit"
	ProviderBRIDirect     PaymentProvider = "bri_direct"
	ProviderBNIDirect     PaymentProvider = "bni_direct"
	ProviderMandiriDirect PaymentProvider = "mandiri_direct"
	ProviderBCADirect     PaymentProvider = "bca_direct"
	ProviderBNCDirect     PaymentProvider = "bnc_direct"
	ProviderOVODirect     PaymentProvider = "ovo_direct"
)

type FeeType string

const (
	FeeTypeFlat    FeeType = "flat"
	FeeTypePercent FeeType = "percent"
)

type RefundStatus string

const (
	RefundStatusPending    RefundStatus = "Pending"
	RefundStatusProcessing RefundStatus = "Processing"
	RefundStatusSuccess    RefundStatus = "Success"
	RefundStatusFailed     RefundStatus = "Failed"
)

// ----------------------------------------------------------------------------
// PaymentMethod mirrors the payment_methods table.
// ----------------------------------------------------------------------------

type PaymentMethod struct {
	ID                 int                `db:"id" json:"id"`
	Type               PaymentType        `db:"type" json:"type"`
	Code               string             `db:"code" json:"code"`
	Name               string             `db:"name" json:"name"`
	Provider           PaymentProvider    `db:"provider" json:"provider"`
	FeeType            FeeType            `db:"fee_type" json:"feeType"`
	FeeFlat            int                `db:"fee_flat" json:"feeFlat"`
	FeePercent         float64            `db:"fee_percent" json:"feePercent"`
	FeeMin             int                `db:"fee_min" json:"feeMin"`
	FeeMax             int                `db:"fee_max" json:"feeMax"`
	MinAmount          int                `db:"min_amount" json:"minAmount"`
	MaxAmount          int                `db:"max_amount" json:"maxAmount"`
	ExpiredDuration    int                `db:"expired_duration" json:"expiredDuration"`
	LogoURL            *string            `db:"logo_url" json:"logoUrl,omitempty"`
	DisplayOrder       int                `db:"display_order" json:"displayOrder"`
	PaymentInstruction NullableRawMessage `db:"payment_instruction" json:"paymentInstruction,omitempty"`
	IsActive           bool               `db:"is_active" json:"isActive"`
	IsMaintenance      bool               `db:"is_maintenance" json:"isMaintenance"`
	MaintenanceMessage *string            `db:"maintenance_message" json:"maintenanceMessage,omitempty"`
	CreatedAt          time.Time          `db:"created_at" json:"createdAt"`
	UpdatedAt          time.Time          `db:"updated_at" json:"updatedAt"`
}

// CalculateFee applies fee_type/flat/percent/min/max rules against the base amount.
func (m *PaymentMethod) CalculateFee(amount int64) int64 {
	var fee int64
	switch m.FeeType {
	case FeeTypeFlat:
		fee = int64(m.FeeFlat)
	case FeeTypePercent:
		// Round up so merchants never under-charge fees.
		raw := float64(amount) * m.FeePercent / 100.0
		fee = int64(raw)
		if float64(fee) < raw {
			fee++
		}
	}
	if m.FeeMin > 0 && fee < int64(m.FeeMin) {
		fee = int64(m.FeeMin)
	}
	if m.FeeMax > 0 && fee > int64(m.FeeMax) {
		fee = int64(m.FeeMax)
	}
	return fee
}

// ----------------------------------------------------------------------------
// Payment mirrors the payments table.
// ----------------------------------------------------------------------------

type Payment struct {
	ID                 int                `db:"id" json:"id"`
	PaymentID          string             `db:"payment_id" json:"paymentId"`
	ReferenceID        string             `db:"reference_id" json:"referenceId"`
	ClientID           int                `db:"client_id" json:"clientId"`
	PaymentMethodID    int                `db:"payment_method_id" json:"paymentMethodId"`
	IsSandbox          bool               `db:"is_sandbox" json:"isSandbox"`
	PaymentType        PaymentType        `db:"payment_type" json:"paymentType"`
	PaymentCode        string             `db:"payment_code" json:"paymentCode"`
	Provider           PaymentProvider    `db:"provider" json:"provider"`
	Amount             int64              `db:"amount" json:"amount"`
	Fee                int64              `db:"fee" json:"fee"`
	TotalAmount        int64              `db:"total_amount" json:"totalAmount"`
	CustomerName       *string            `db:"customer_name" json:"customerName,omitempty"`
	CustomerEmail      *string            `db:"customer_email" json:"customerEmail,omitempty"`
	CustomerPhone      *string            `db:"customer_phone" json:"customerPhone,omitempty"`
	Status             PaymentStatus      `db:"status" json:"status"`
	PaymentDetail      NullableRawMessage `db:"payment_detail" json:"paymentDetail"`
	PaymentInstruction NullableRawMessage `db:"payment_instruction" json:"paymentInstruction,omitempty"`
	SenderBank         *string            `db:"sender_bank" json:"senderBank,omitempty"`
	SenderName         *string            `db:"sender_name" json:"senderName,omitempty"`
	SenderAccount      *string            `db:"sender_account" json:"senderAccount,omitempty"`
	ProviderRef        *string            `db:"provider_ref" json:"providerRef,omitempty"`
	ProviderData       NullableRawMessage `db:"provider_data" json:"providerData,omitempty"`
	CallbackType       *string            `db:"callback_type" json:"callbackType,omitempty"`
	Description        *string            `db:"description" json:"description,omitempty"`
	Metadata           NullableRawMessage `db:"metadata" json:"metadata,omitempty"`
	CallbackSent       bool               `db:"callback_sent" json:"callbackSent"`
	CallbackSentAt     *time.Time         `db:"callback_sent_at" json:"callbackSentAt,omitempty"`
	CallbackAttempts   int                `db:"callback_attempts" json:"callbackAttempts"`
	ExpiredAt          time.Time          `db:"expired_at" json:"expiredAt"`
	CreatedAt          time.Time          `db:"created_at" json:"createdAt"`
	PaidAt             *time.Time         `db:"paid_at" json:"paidAt,omitempty"`
	CancelledAt        *time.Time         `db:"cancelled_at" json:"cancelledAt,omitempty"`
	UpdatedAt          time.Time          `db:"updated_at" json:"updatedAt"`
}

// PaymentLog mirrors the payment_logs audit table.
type PaymentLog struct {
	ID             int                `db:"id" json:"id"`
	PaymentID      int                `db:"payment_id" json:"paymentId"`
	Action         string             `db:"action" json:"action"`
	Provider       PaymentProvider    `db:"provider" json:"provider"`
	Request        NullableRawMessage `db:"request" json:"request,omitempty"`
	Response       NullableRawMessage `db:"response" json:"response,omitempty"`
	IsSuccess      bool               `db:"is_success" json:"isSuccess"`
	ErrorCode      *string            `db:"error_code" json:"errorCode,omitempty"`
	ErrorMessage   *string            `db:"error_message" json:"errorMessage,omitempty"`
	CreatedAt      time.Time          `db:"created_at" json:"createdAt"`
	ResponseAt     *time.Time         `db:"response_at" json:"responseAt,omitempty"`
	ResponseTimeMs *int               `db:"response_time_ms" json:"responseTimeMs,omitempty"`
}

// Refund mirrors the refunds table.
type Refund struct {
	ID           int                `db:"id" json:"id"`
	RefundID     string             `db:"refund_id" json:"refundId"`
	PaymentID    int                `db:"payment_id" json:"paymentId"`
	Amount       int64              `db:"amount" json:"amount"`
	Status       RefundStatus       `db:"status" json:"status"`
	Reason       string             `db:"reason" json:"reason"`
	ProviderRef  *string            `db:"provider_ref" json:"providerRef,omitempty"`
	ProviderData NullableRawMessage `db:"provider_data" json:"providerData,omitempty"`
	CreatedAt    time.Time          `db:"created_at" json:"createdAt"`
	ProcessedAt  *time.Time         `db:"processed_at" json:"processedAt,omitempty"`
	UpdatedAt    time.Time          `db:"updated_at" json:"updatedAt"`
}

// PaymentCallback captures raw provider webhook payloads + verification result.
type PaymentCallback struct {
	ID               int                `db:"id" json:"id"`
	Provider         PaymentProvider    `db:"provider" json:"provider"`
	ProviderRef      *string            `db:"provider_ref" json:"providerRef,omitempty"`
	Headers          NullableRawMessage `db:"headers" json:"headers,omitempty"`
	Payload          NullableRawMessage `db:"payload" json:"payload"`
	Signature        *string            `db:"signature" json:"signature,omitempty"`
	IsValidSignature bool               `db:"is_valid_signature" json:"isValidSignature"`
	PaymentID        *string            `db:"payment_id" json:"paymentId,omitempty"`
	Status           *string            `db:"status" json:"status,omitempty"`
	PaidAmount       *int64             `db:"paid_amount" json:"paidAmount,omitempty"`
	IsProcessed      bool               `db:"is_processed" json:"isProcessed"`
	ProcessedAt      *time.Time         `db:"processed_at" json:"processedAt,omitempty"`
	ProcessError     *string            `db:"process_error" json:"processError,omitempty"`
	CreatedAt        time.Time          `db:"created_at" json:"createdAt"`
}

// PaymentCallbackLog mirrors payment_callback_logs — outbound client webhook audit.
type PaymentCallbackLog struct {
	ID             int                `db:"id" json:"id"`
	PaymentID      int                `db:"payment_id" json:"paymentId"`
	ClientID       int                `db:"client_id" json:"clientId"`
	Event          string             `db:"event" json:"event"`
	Payload        NullableRawMessage `db:"payload" json:"payload"`
	Attempt        int                `db:"attempt" json:"attempt"`
	MaxAttempts    int                `db:"max_attempts" json:"maxAttempts"`
	HTTPStatus     *int               `db:"http_status" json:"httpStatus,omitempty"`
	ResponseBody   *string            `db:"response_body" json:"responseBody,omitempty"`
	ResponseTimeMs *int               `db:"response_time_ms" json:"responseTimeMs,omitempty"`
	IsDelivered    bool               `db:"is_delivered" json:"isDelivered"`
	ErrorMessage   *string            `db:"error_message" json:"errorMessage,omitempty"`
	NextRetryAt    *time.Time         `db:"next_retry_at" json:"nextRetryAt,omitempty"`
	DeliveredAt    *time.Time         `db:"delivered_at" json:"deliveredAt,omitempty"`
	CreatedAt      time.Time          `db:"created_at" json:"createdAt"`
	UpdatedAt      time.Time          `db:"updated_at" json:"updatedAt"`
}
