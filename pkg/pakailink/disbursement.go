package pakailink

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

const (
	BankAccountInquiryPath = "/snap/v1.0/emoney/bank-account-inquiry"
	TransferBankPath       = "/snap/v1.0/emoney/transfer-bank"
	TransferStatusPath     = "/snap/v1.0/emoney/transfer-bank/status"

	DisbursementServiceCodeInquiry  = "42"
	DisbursementServiceCodeTransfer = "43"
	DisbursementServiceCodeCallback = "44"
	DisbursementServiceCodeStatus   = "45"

	TransferStatusSuccess = "00"
	TransferStatusPending = "03"
	TransferStatusFailed  = "06"
)

// BankAccountInquiryRequest mirrors PakaiLink Service 42 input.
type BankAccountInquiryRequest struct {
	PartnerReferenceNo      string
	BeneficiaryAccountNo    string
	BeneficiaryBankCode     string
}

// BankAccountInquiryResponse decodes the Service 42 response body.
type BankAccountInquiryResponse struct {
	ResponseCode             string          `json:"responseCode"`
	ResponseMessage          string          `json:"responseMessage"`
	SessionID                string          `json:"sessionId"`
	PartnerReferenceNo       string          `json:"partnerReferenceNo"`
	BeneficiaryAccountNumber string          `json:"beneficiaryAccountNumber"`
	BeneficiaryAccountName   string          `json:"beneficiaryAccountName"`
	BeneficiaryBankName      string          `json:"beneficiaryBankName"`
	AdditionalInfo           map[string]any  `json:"additionalInfo,omitempty"`
	RawResponse              json.RawMessage `json:"-"`
}

// TransferBankRequest mirrors PakaiLink Service 43 input.
type TransferBankRequest struct {
	PartnerReferenceNo       string
	BeneficiaryAccountNumber string
	BeneficiaryBankCode      string
	SessionID                string
	Amount                   int64
	CallbackURL              string
	Remark                   string
}

// TransferBankResponse decodes the Service 43 response body.
type TransferBankResponse struct {
	ResponseCode             string          `json:"responseCode"`
	ResponseMessage          string          `json:"responseMessage"`
	ReferenceNo              string          `json:"referenceNo"`
	PartnerReferenceNo       string          `json:"partnerReferenceNo"`
	BeneficiaryAccountNumber string          `json:"beneficiaryAccountNumber"`
	BeneficiaryAccountName   string          `json:"beneficiaryAccountName"`
	BeneficiaryBankName      string          `json:"beneficiaryBankName"`
	Amount                   Amount          `json:"amount"`
	FeeAmount                Amount          `json:"feeAmount"`
	AdditionalInfo           TransferAddInfo `json:"additionalInfo"`
	RawResponse              json.RawMessage `json:"-"`
}

// TransferAddInfo carries the transactionStatus + balance metadata returned in
// Service 43 / Service 45 responses.
type TransferAddInfo struct {
	TransactionStatus     string                `json:"transactionStatus"`
	TransactionStatusDesc TransactionStatusDesc `json:"transactionStatusDesc"`
	Balance               Amount                `json:"balance,omitempty"`
}

// TransactionStatusDesc holds the bilingual status description.
type TransactionStatusDesc struct {
	English   string `json:"english"`
	Indonesia string `json:"indonesia"`
}

// TransferStatusResponse decodes the Service 45 status inquiry response.
type TransferStatusResponse struct {
	ResponseCode                string                `json:"responseCode"`
	ResponseMessage             string                `json:"responseMessage"`
	OriginalPartnerReferenceNo  string                `json:"originalPartnerReferenceNo"`
	OriginalReferenceNo         string                `json:"originalReferenceNo"`
	OriginalExternalID          string                `json:"originalExternalId"`
	ServiceCode                 string                `json:"serviceCode"`
	TransactionDate             string                `json:"transactionDate"`
	Amount                      Amount                `json:"amount"`
	BeneficiaryAccountNumber    string                `json:"beneficiaryAccountNumber"`
	BeneficiaryAccountName      string                `json:"beneficiaryAccountName"`
	BeneficiaryBankCode         string                `json:"beneficiaryBankCode"`
	BeneficiaryBankName         string                `json:"beneficiaryBankName"`
	LatestTransactionStatus     string                `json:"latestTransactionStatus"`
	LatestTransactionStatusDesc TransactionStatusDesc `json:"latestTransactionStatusDesc"`
	AdditionalInfo              map[string]any        `json:"additionalInfo,omitempty"`
	RawResponse                 json.RawMessage       `json:"-"`
}

// DisbursementCallback is the Service 44 callback envelope.
type DisbursementCallback struct {
	TransactionData DisbursementCallbackData `json:"transactionData"`
}

// DisbursementCallbackData holds the actual fields delivered in Service 44.
type DisbursementCallbackData struct {
	PaymentFlagStatus  string                `json:"paymentFlagStatus"`
	PaymentFlagReason  TransactionStatusDesc `json:"paymentFlagReason"`
	PartnerReferenceNo string                `json:"partnerReferenceNo"`
	AccountNumber      string                `json:"accountNumber"`
	AccountName        string                `json:"accountName"`
	ReferenceNo        string                `json:"referenceNo"`
	PaidAmount         Amount                `json:"paidAmount"`
	FeeAmount          Amount                `json:"feeAmount"`
	AdditionalInfo     map[string]any        `json:"additionalInfo,omitempty"`
}

// BankAccountInquiry calls Service 42 for beneficiary lookup.
func (c *Client) BankAccountInquiry(ctx context.Context, req BankAccountInquiryRequest) (*BankAccountInquiryResponse, error) {
	body := map[string]any{
		"partnerReferenceNo":       req.PartnerReferenceNo,
		"beneficiaryAccountNumber": req.BeneficiaryAccountNo,
		"additionalInfo": map[string]any{
			"beneficiaryBankCode": req.BeneficiaryBankCode,
		},
	}

	var resp BankAccountInquiryResponse
	raw, err := c.doSNAPRequest(ctx, http.MethodPost, BankAccountInquiryPath, body, &resp)
	if err != nil {
		return nil, err
	}
	resp.RawResponse = raw
	return &resp, nil
}

// TransferBank calls Service 43 to submit a disbursement transfer.
func (c *Client) TransferBank(ctx context.Context, req TransferBankRequest) (*TransferBankResponse, error) {
	add := map[string]any{}
	if req.CallbackURL != "" {
		add["callbackUrl"] = req.CallbackURL
	}
	if req.Remark != "" {
		add["remark"] = req.Remark
	}

	body := map[string]any{
		"partnerReferenceNo":       req.PartnerReferenceNo,
		"beneficiaryAccountNumber": req.BeneficiaryAccountNumber,
		"beneficiaryBankCode":      req.BeneficiaryBankCode,
		"amount": Amount{
			Value:    formatAmount(req.Amount),
			Currency: "IDR",
		},
		"additionalInfo": add,
	}
	if req.SessionID != "" {
		body["sessionId"] = req.SessionID
	}

	var resp TransferBankResponse
	raw, err := c.doSNAPRequest(ctx, http.MethodPost, TransferBankPath, body, &resp)
	if err != nil {
		return nil, err
	}
	resp.RawResponse = raw
	return &resp, nil
}

// TransferStatus calls Service 45 for the latest transfer status.
func (c *Client) TransferStatus(ctx context.Context, originalPartnerReferenceNo string) (*TransferStatusResponse, error) {
	body := map[string]any{
		"originalPartnerReferenceNo": originalPartnerReferenceNo,
	}
	var resp TransferStatusResponse
	raw, err := c.doSNAPRequest(ctx, http.MethodPost, TransferStatusPath, body, &resp)
	if err != nil {
		return nil, err
	}
	resp.RawResponse = raw
	return &resp, nil
}

// DisbursementCallbackTimestamp parses the X-TIMESTAMP header into time.Time.
// Returns the zero time on any parse error so callers can apply policy
// (e.g. reject signatures older than N minutes).
func DisbursementCallbackTimestamp(value string) time.Time {
	t, err := time.Parse("2006-01-02T15:04:05-07:00", value)
	if err != nil {
		return time.Time{}
	}
	return t
}
