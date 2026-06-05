package pakailink

import "encoding/json"

// Amount is the SNAP BI money envelope: stringified number with 2 decimals.
type Amount struct {
	Value    string `json:"value"`
	Currency string `json:"currency"`
}

// APIError wraps a non-2xx or non-200 SNAP response.
type APIError struct {
	HTTPStatus      int
	ResponseCode    string
	ResponseMessage string
	RawResponse     json.RawMessage
}

func (e *APIError) Error() string {
	if e == nil {
		return ""
	}
	if e.ResponseCode == "" {
		return e.ResponseMessage
	}
	return e.ResponseCode + ": " + e.ResponseMessage
}

type tokenResponse struct {
	ResponseCode    string `json:"responseCode"`
	ResponseMessage string `json:"responseMessage"`
	AccessToken     string `json:"accessToken"`
	TokenType       string `json:"tokenType"`
	ExpiresIn       string `json:"expiresIn"`
}

// CreateVARequest is the business-level input for CreateVA.
type CreateVARequest struct {
	PartnerReferenceNo  string
	CustomerNo          string
	VirtualAccountName  string
	VirtualAccountPhone string
	VirtualAccountEmail string
	TotalAmount         int64
	BankCode            string
	CallbackURL         string
	ExpiredDate         string // ISO 8601 +07:00 (optional)
}

type CreateVAResponse struct {
	ResponseCode       string             `json:"responseCode"`
	ResponseMessage    string             `json:"responseMessage"`
	VirtualAccountData VirtualAccountData `json:"virtualAccountData"`
	RawResponse        json.RawMessage    `json:"-"`
}

type VirtualAccountData struct {
	PartnerReferenceNo  string         `json:"partnerReferenceNo"`
	CustomerNo          string         `json:"customerNo"`
	VirtualAccountNo    string         `json:"virtualAccountNo"`
	VirtualAccountName  string         `json:"virtualAccountName,omitempty"`
	VirtualAccountPhone string         `json:"virtualAccountPhone,omitempty"`
	VirtualAccountEmail string         `json:"virtualAccountEmail,omitempty"`
	ExpiredDate         string         `json:"expiredDate,omitempty"`
	TotalAmount         Amount         `json:"totalAmount"`
	AdditionalInfo      map[string]any `json:"additionalInfo,omitempty"`
}

type InquiryVAResponse struct {
	ResponseCode               string          `json:"responseCode"`
	ResponseMessage            string          `json:"responseMessage"`
	OriginalPartnerReferenceNo string          `json:"originalPartnerReferenceNo"`
	OriginalReferenceNo        string          `json:"originalReferenceNo,omitempty"`
	OriginalExternalID         string          `json:"originalExternalId,omitempty"`
	ServiceCode                string          `json:"serviceCode,omitempty"`
	LatestTransactionStatus    string          `json:"latestTransactionStatus"`
	TransactionStatusDesc      string          `json:"transactionStatusDesc,omitempty"`
	TransactionDate            string          `json:"transactionDate,omitempty"`
	PaidTime                   string          `json:"paidTime,omitempty"`
	Amount                     Amount          `json:"amount,omitempty"`
	AdditionalInfo             map[string]any  `json:"additionalInfo,omitempty"`
	RawResponse                json.RawMessage `json:"-"`
}

type DeleteVARequest struct {
	PartnerReferenceNo string
	CustomerNo         string
	VirtualAccountNo   string
	TrxID              string
}

type DeleteVAResponse struct {
	ResponseCode        string          `json:"responseCode"`
	ResponseMessage     string          `json:"responseMessage"`
	VirtualAccountData  map[string]any  `json:"virtualAccountData,omitempty"`
	RawResponse         json.RawMessage `json:"-"`
}

// GenerateQRRequest creates a QRIS MPM (Merchant Presented Mode) code.
type GenerateQRRequest struct {
	PartnerReferenceNo string
	Amount             int64
	TerminalID         string
	CallbackURL        string
	ExpiredDate        string // ISO 8601 (optional)
	MerchantName       string
	Description        string
}

type GenerateQRResponse struct {
	ResponseCode       string          `json:"responseCode"`
	ResponseMessage    string          `json:"responseMessage"`
	ReferenceNo        string          `json:"referenceNo,omitempty"`
	PartnerReferenceNo string          `json:"partnerReferenceNo,omitempty"`
	QRContent          string          `json:"qrContent,omitempty"`
	TerminalID         string          `json:"terminalId,omitempty"`
	AdditionalInfo     map[string]any  `json:"additionalInfo,omitempty"`
	RawResponse        json.RawMessage `json:"-"`
}

type InquiryQRResponse struct {
	ResponseCode               string          `json:"responseCode"`
	ResponseMessage            string          `json:"responseMessage"`
	OriginalPartnerReferenceNo string          `json:"originalPartnerReferenceNo"`
	OriginalReferenceNo        string          `json:"originalReferenceNo,omitempty"`
	LatestTransactionStatus    string          `json:"latestTransactionStatus"`
	TransactionStatusDesc      string          `json:"transactionStatusDesc,omitempty"`
	Amount                     Amount          `json:"amount,omitempty"`
	PaidTime                   string          `json:"paidTime,omitempty"`
	AdditionalInfo             map[string]any  `json:"additionalInfo,omitempty"`
	RawResponse                json.RawMessage `json:"-"`
}

// WebhookPayload is the combined shape covering VA payment + QRIS MPM notifications.
type WebhookPayload struct {
	TransactionData WebhookTransactionData `json:"transactionData"`
	RawBody         []byte                 `json:"-"`
}

type WebhookTransactionData struct {
	PaymentFlagStatus   string                 `json:"paymentFlagStatus"`
	PaymentFlagReason   map[string]string      `json:"paymentFlagReason,omitempty"`
	CustomerNo          string                 `json:"customerNo,omitempty"`
	VirtualAccountNo    string                 `json:"virtualAccountNo,omitempty"`
	VirtualAccountName  string                 `json:"virtualAccountName,omitempty"`
	PartnerReferenceNo  string                 `json:"partnerReferenceNo"`
	OriginalReferenceNo string                 `json:"originalReferenceNo,omitempty"`
	CallbackType        string                 `json:"callbackType,omitempty"`
	TrxDateTime         string                 `json:"trxDateTime,omitempty"`
	PaidAmount          Amount                 `json:"paidAmount,omitempty"`
	FeeAmount           Amount                 `json:"feeAmount,omitempty"`
	CreditBalance       Amount                 `json:"creditBalance,omitempty"`
	AdditionalInfo      map[string]any         `json:"additionalInfo,omitempty"`
	Extra               map[string]any         `json:"-"`
}
