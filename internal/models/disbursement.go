package models

import "time"

type TransferType string
type TransferStatus string
type DisbursementProvider string

const (
	TransferTypeIntrabank TransferType = "INTRABANK"
	TransferTypeInterbank TransferType = "INTERBANK"
)

const (
	TransferStatusProcessing TransferStatus = "Processing"
	TransferStatusSuccess    TransferStatus = "Success"
	TransferStatusPending    TransferStatus = "Pending"
	TransferStatusFailed     TransferStatus = "Failed"
)

const (
	DisbursementProviderBCA     DisbursementProvider = "bca_direct"
	DisbursementProviderBRI     DisbursementProvider = "bri_direct"
	DisbursementProviderBNI     DisbursementProvider = "bni_direct"
	DisbursementProviderMandiri DisbursementProvider = "mandiri_direct"
	DisbursementProviderBNC     DisbursementProvider = "bnc_direct"
	DisbursementProviderPakaiLink DisbursementProvider = "pakailink"
)

type TransferInquiry struct {
	ID            int                  `db:"id"`
	InquiryID     string               `db:"inquiry_id"`
	ClientID      int                  `db:"client_id"`
	IsSandbox     bool                 `db:"is_sandbox"`
	BankCode      string               `db:"bank_code"`
	BankName      *string              `db:"bank_name"`
	AccountNumber string               `db:"account_number"`
	AccountName   *string              `db:"account_name"`
	TransferType  TransferType         `db:"transfer_type"`
	Provider      DisbursementProvider `db:"provider"`
	ProviderRef   *string              `db:"provider_ref"`
	ProviderData  NullableRawMessage   `db:"provider_data"`
	ExpiredAt     time.Time            `db:"expired_at"`
	CreatedAt     time.Time            `db:"created_at"`
}

type Transfer struct {
	ID                  int                  `db:"id"`
	TransferID          string               `db:"transfer_id"`
	ReferenceID         string               `db:"reference_id"`
	ClientID            int                  `db:"client_id"`
	IsSandbox           bool                 `db:"is_sandbox"`
	TransferType        TransferType         `db:"transfer_type"`
	Provider            DisbursementProvider `db:"provider"`
	BankCode            string               `db:"bank_code"`
	BankName            *string              `db:"bank_name"`
	AccountNumber       string               `db:"account_number"`
	AccountName         *string              `db:"account_name"`
	SourceBankCode      string               `db:"source_bank_code"`
	SourceAccountNumber string               `db:"source_account_number"`
	Amount              int64                `db:"amount"`
	Fee                 int64                `db:"fee"`
	TotalAmount         int64                `db:"total_amount"`
	Status              TransferStatus       `db:"status"`
	FailedReason        *string              `db:"failed_reason"`
	FailedCode          *string              `db:"failed_code"`
	PurposeCode         *string              `db:"purpose_code"`
	Remark              *string              `db:"remark"`
	InquiryRowID        *int                 `db:"inquiry_id"`
	ProviderRef         *string              `db:"provider_ref"`
	ProviderData        NullableRawMessage   `db:"provider_data"`
	CallbackSent        bool                 `db:"callback_sent"`
	CallbackSentAt      *time.Time           `db:"callback_sent_at"`
	CreatedAt           time.Time            `db:"created_at"`
	CompletedAt         *time.Time           `db:"completed_at"`
	FailedAt            *time.Time           `db:"failed_at"`
	UpdatedAt           time.Time            `db:"updated_at"`
}

type TransferCallback struct {
	ID               int                  `db:"id"`
	Provider         DisbursementProvider `db:"provider"`
	ProviderRef      *string              `db:"provider_ref"`
	Headers          NullableRawMessage   `db:"headers"`
	Payload          NullableRawMessage   `db:"payload"`
	Signature        *string              `db:"signature"`
	IsValidSignature bool                 `db:"is_valid_signature"`
	TransferID       *string              `db:"transfer_id"`
	Status           *string              `db:"status"`
	IsProcessed      bool                 `db:"is_processed"`
	ProcessedAt      *time.Time           `db:"processed_at"`
	ProcessError     *string              `db:"process_error"`
	CreatedAt        time.Time            `db:"created_at"`
}

type TransferInquiryResponse struct {
	BankCode      string `json:"bankCode"`
	BankShortName string `json:"bankShortName"`
	BankName      string `json:"bankName"`
	AccountNumber string `json:"accountNumber"`
	AccountName   string `json:"accountName"`
	TransferType  string `json:"transferType"`
	InquiryID     string `json:"inquiryId"`
	ExpiredAt     string `json:"expiredAt"`
}

type TransferResponse struct {
	TransferID         string `json:"transferId"`
	ReferenceID        string `json:"referenceId"`
	Status             string `json:"status"`
	TransferType       string `json:"transferType"`
	Route              string `json:"route,omitempty"`
	BankCode           string `json:"bankCode"`
	BankShortName      string `json:"bankShortName"`
	BankName           string `json:"bankName"`
	AccountNumber      string `json:"accountNumber"`
	AccountName        string `json:"accountName"`
	Amount             int64  `json:"amount"`
	Fee                int64  `json:"fee"`
	TotalAmount        int64  `json:"totalAmount"`
	Purpose            string `json:"purpose,omitempty"`
	PurposeDescription string `json:"purposeDescription,omitempty"`
	Remark             string `json:"remark,omitempty"`
	ProviderRef        string `json:"providerRef,omitempty"`
	CreatedAt          string `json:"createdAt"`
	CompletedAt        string `json:"completedAt,omitempty"`
	FailedAt           string `json:"failedAt,omitempty"`
	FailedReason       string `json:"failedReason,omitempty"`
	FailedCode         string `json:"failedCode,omitempty"`
}
