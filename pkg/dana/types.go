package dana

import "encoding/json"

// Amount is the SNAP BI money envelope: stringified number with 2 decimals.
type Amount struct {
	Value    string `json:"value"`
	Currency string `json:"currency"`
}

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

// PayMethod / PayOption constants sourced from docs/payment/dana_direct.md.
const (
	PayMethodBalance    = "BALANCE"
	PayMethodNetworkPay = "NETWORK_PAY"
	PayOptionQRIS       = "NETWORK_PAY_PG_QRIS"
	PayOptionOVO        = "NETWORK_PAY_PG_OVO"
	PayOptionGoPay      = "NETWORK_PAY_PG_GOPAY"
	PayOptionShopeePay  = "NETWORK_PAY_PG_SPAY"

	StatusSuccess   = "00"
	StatusCancelled = "05"
)

// CreateOrderRequest describes a DANA Direct order. The caller picks
// PayMethod/PayOption per the use case (DANA balance vs. QRIS).
type CreateOrderRequest struct {
	PartnerReferenceNo string
	MerchantID         string
	ExternalStoreID    string
	Amount             int64
	ValidUpTo          string // ISO 8601 +07:00
	NotificationURL    string
	ReturnURL          string
	PayMethod          string
	PayOption          string
	OrderTitle         string
	OrderScenario      string // default "API"
	MCC                string // default "5732"
}

type CreateOrderResponse struct {
	ResponseCode       string          `json:"responseCode"`
	ResponseMessage    string          `json:"responseMessage"`
	ReferenceNo        string          `json:"referenceNo"`
	PartnerReferenceNo string          `json:"partnerReferenceNo"`
	WebRedirectURL     string          `json:"webRedirectUrl,omitempty"`
	CheckoutURL        string          `json:"checkoutUrl,omitempty"`
	DeeplinkURL        string          `json:"deeplinkUrl,omitempty"`
	AdditionalInfo     map[string]any  `json:"additionalInfo,omitempty"`
	RawResponse        json.RawMessage `json:"-"`
}

// PaymentCode returns the QRIS string from additionalInfo.paymentCode.
func (r *CreateOrderResponse) PaymentCode() string {
	if r == nil {
		return ""
	}
	if v, ok := r.AdditionalInfo["paymentCode"].(string); ok {
		return v
	}
	return ""
}

type InquiryOrderResponse struct {
	ResponseCode               string          `json:"responseCode"`
	ResponseMessage            string          `json:"responseMessage"`
	OriginalPartnerReferenceNo string          `json:"originalPartnerReferenceNo"`
	OriginalReferenceNo        string          `json:"originalReferenceNo"`
	LatestTransactionStatus    string          `json:"latestTransactionStatus"`
	TransactionStatusDesc      string          `json:"transactionStatusDesc,omitempty"`
	Amount                     Amount          `json:"amount,omitempty"`
	AdditionalInfo             map[string]any  `json:"additionalInfo,omitempty"`
	RawResponse                json.RawMessage `json:"-"`
}

type CancelOrderRequest struct {
	PartnerReferenceNo string
	MerchantID         string
	Reason             string
}

type CancelOrderResponse struct {
	ResponseCode        string          `json:"responseCode"`
	ResponseMessage     string          `json:"responseMessage"`
	OriginalReferenceNo string          `json:"originalReferenceNo,omitempty"`
	CancelTime          string          `json:"cancelTime,omitempty"`
	RawResponse         json.RawMessage `json:"-"`
}

type RefundRequest struct {
	PartnerReferenceNo       string
	PartnerRefundNo          string
	MerchantID               string
	RefundAmount             int64
	Reason                   string
	OriginalPartnerReference string
	OriginalReferenceNo      string
}

type RefundResponse struct {
	ResponseCode       string          `json:"responseCode"`
	ResponseMessage    string          `json:"responseMessage"`
	RefundTime         string          `json:"refundTime,omitempty"`
	RefundNo           string          `json:"refundNo,omitempty"`
	PartnerRefundNo    string          `json:"partnerRefundNo,omitempty"`
	RefundAmount       Amount          `json:"refundAmount,omitempty"`
	AdditionalInfo     map[string]any  `json:"additionalInfo,omitempty"`
	RawResponse        json.RawMessage `json:"-"`
}

// WebhookPayload matches DANA's payment notification schema.
type WebhookPayload struct {
	OriginalPartnerReferenceNo string         `json:"originalPartnerReferenceNo"`
	OriginalReferenceNo        string         `json:"originalReferenceNo"`
	MerchantID                 string         `json:"merchantId"`
	Amount                     Amount         `json:"amount"`
	LatestTransactionStatus    string         `json:"latestTransactionStatus"`
	TransactionStatusDesc      string         `json:"transactionStatusDesc,omitempty"`
	CreatedTime                string         `json:"createdTime,omitempty"`
	FinishedTime               string         `json:"finishedTime,omitempty"`
	AdditionalInfo             map[string]any `json:"additionalInfo,omitempty"`
	RawBody                    []byte         `json:"-"`
}
