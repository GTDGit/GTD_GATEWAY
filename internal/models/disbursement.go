package models

import "time"

// Payout (disbursement) domain types for the admin read model. Mirrors the
// api module's payout schema: routing is per method_type (BANK/EWALLET) with
// priority-ordered provider fallback.

// MethodType distinguishes a bank transfer from an e-wallet top-up.
type MethodType string

const (
	MethodTypeBank    MethodType = "BANK"
	MethodTypeEwallet MethodType = "EWALLET"
)

// TransferType is the internal bank routing hint (intrabank vs interbank).
type TransferType string

const (
	TransferTypeIntrabank TransferType = "INTRABANK"
	TransferTypeInterbank TransferType = "INTERBANK"
)

// PayoutStatus is the lifecycle status persisted on a payout.
type PayoutStatus string

const (
	PayoutStatusProcessing PayoutStatus = "Processing"
	PayoutStatusSuccess    PayoutStatus = "Success"
	PayoutStatusPending    PayoutStatus = "Pending"
	PayoutStatusFailed     PayoutStatus = "Failed"
)

// DisbursementProvider identifies a payout provider.
type DisbursementProvider string

const (
	DisbursementProviderBCA       DisbursementProvider = "bca_direct"
	DisbursementProviderBRI       DisbursementProvider = "bri_direct"
	DisbursementProviderBNI       DisbursementProvider = "bni_direct"
	DisbursementProviderMandiri   DisbursementProvider = "mandiri_direct"
	DisbursementProviderBNC       DisbursementProvider = "bnc_direct"
	DisbursementProviderPakaiLink DisbursementProvider = "pakailink"
	DisbursementProviderDANA      DisbursementProvider = "dana_direct"
)

// PayoutRoute binds a method_type to a provider with a priority for fallback.
type PayoutRoute struct {
	ID                 int                  `db:"id"`
	MethodType         MethodType           `db:"method_type"`
	Provider           DisbursementProvider `db:"provider"`
	Priority           int                  `db:"priority"`
	IsActive           bool                 `db:"is_active"`
	IsMaintenance      bool                 `db:"is_maintenance"`
	MaintenanceMessage *string              `db:"maintenance_message"`
	CreatedAt          time.Time            `db:"created_at"`
	UpdatedAt          time.Time            `db:"updated_at"`
}

// PayoutInquiry is a cached recipient validation created by the inquiry flow.
type PayoutInquiry struct {
	ID            int                  `db:"id"`
	InquiryID     string               `db:"inquiry_id"`
	ClientID      int                  `db:"client_id"`
	IsSandbox     bool                 `db:"is_sandbox"`
	MethodType    MethodType           `db:"method_type"`
	ChannelCode   string               `db:"channel_code"`
	BankCode      string               `db:"bank_code"`
	BankName      *string              `db:"bank_name"`
	AccountNumber string               `db:"account_number"`
	AccountName   *string              `db:"account_name"`
	TransferType  *TransferType        `db:"transfer_type"`
	Provider      DisbursementProvider `db:"provider"`
	ProviderRef   *string              `db:"provider_ref"`
	ProviderData  NullableRawMessage   `db:"provider_data"`
	ExpiredAt     time.Time            `db:"expired_at"`
	CreatedAt     time.Time            `db:"created_at"`
}

// Payout is a disbursement record (admin read model).
type Payout struct {
	ID                  int                  `db:"id"`
	PayoutID            string               `db:"payout_id"`
	ReferenceID         string               `db:"reference_id"`
	ClientID            int                  `db:"client_id"`
	IsSandbox           bool                 `db:"is_sandbox"`
	MethodType          MethodType           `db:"method_type"`
	ChannelCode         string               `db:"channel_code"`
	TransferType        *TransferType        `db:"transfer_type"`
	Provider            DisbursementProvider `db:"provider"`
	BankCode            string               `db:"bank_code"`
	BankName            *string              `db:"bank_name"`
	AccountNumber       string               `db:"account_number"`
	AccountName         *string              `db:"account_name"`
	SourceBankCode      *string              `db:"source_bank_code"`
	SourceAccountNumber *string              `db:"source_account_number"`
	Amount              int64                `db:"amount"`
	Fee                 int64                `db:"fee"`
	SendAmount          int64                `db:"send_amount"`
	TotalAmount         int64                `db:"total_amount"`
	FeePaidBy           FeePaidBy            `db:"fee_paid_by"`
	Status              PayoutStatus         `db:"status"`
	FailedReason        *string              `db:"failed_reason"`
	FailedCode          *string              `db:"failed_code"`
	PurposeCode         *string              `db:"purpose_code"`
	Remark              *string              `db:"remark"`
	Description         *string              `db:"description"`
	CustomerName        *string              `db:"customer_name"`
	CustomerEmail       *string              `db:"customer_email"`
	CustomerPhone       *string              `db:"customer_phone"`
	InquiryRowID        *int                 `db:"inquiry_id"`
	ProviderRef         *string              `db:"provider_ref"`
	ProviderData        NullableRawMessage   `db:"provider_data"`
	CallbackURL         *string              `db:"callback_url"`
	CallbackSent        bool                 `db:"callback_sent"`
	CallbackSentAt      *time.Time           `db:"callback_sent_at"`
	CallbackAttempts    int                  `db:"callback_attempts"`
	CreatedAt           time.Time            `db:"created_at"`
	CompletedAt         *time.Time           `db:"completed_at"`
	FailedAt            *time.Time           `db:"failed_at"`
	UpdatedAt           time.Time            `db:"updated_at"`
}

// PayoutCallback records an inbound provider callback for a payout.
type PayoutCallback struct {
	ID               int                  `db:"id"`
	Provider         DisbursementProvider `db:"provider"`
	ProviderRef      *string              `db:"provider_ref"`
	Headers          NullableRawMessage   `db:"headers"`
	Payload          NullableRawMessage   `db:"payload"`
	Signature        *string              `db:"signature"`
	IsValidSignature bool                 `db:"is_valid_signature"`
	PayoutID         *string              `db:"payout_id"`
	Status           *string              `db:"status"`
	IsProcessed      bool                 `db:"is_processed"`
	ProcessedAt      *time.Time           `db:"processed_at"`
	ProcessError     *string              `db:"process_error"`
	CreatedAt        time.Time            `db:"created_at"`
}
