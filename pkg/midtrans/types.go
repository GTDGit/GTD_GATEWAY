package midtrans

import "encoding/json"

type APIError struct {
	HTTPStatus    int
	StatusCode    string
	StatusMessage string
	RawResponse   json.RawMessage
}

func (e *APIError) Error() string {
	if e == nil {
		return ""
	}
	if e.StatusCode == "" {
		return e.StatusMessage
	}
	return e.StatusCode + ": " + e.StatusMessage
}

// Payment type constants.
const (
	PaymentTypeGoPay     = "gopay"
	PaymentTypeShopeePay = "shopeepay"

	StatusPending    = "pending"
	StatusSettlement = "settlement"
	StatusCapture    = "capture"
	StatusExpire     = "expire"
	StatusDeny       = "deny"
	StatusCancel     = "cancel"
	StatusRefund     = "refund"
)

type TransactionDetails struct {
	OrderID     string `json:"order_id"`
	GrossAmount int64  `json:"gross_amount"`
}

type CustomerDetails struct {
	FirstName string `json:"first_name,omitempty"`
	LastName  string `json:"last_name,omitempty"`
	Email     string `json:"email,omitempty"`
	Phone     string `json:"phone,omitempty"`
}

type ItemDetail struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Price    int64  `json:"price"`
	Quantity int    `json:"quantity"`
}

type GoPayOptions struct {
	EnableCallback  bool   `json:"enable_callback"`
	CallbackURL     string `json:"callback_url,omitempty"`
	ExpiryDuration  int    `json:"expiry_duration,omitempty"`
	ExpiryUnit      string `json:"expiry_unit,omitempty"`
}

type ShopeePayOptions struct {
	CallbackURL string `json:"callback_url"`
}

type ChargeRequest struct {
	PaymentType        string              `json:"payment_type"`
	TransactionDetails TransactionDetails  `json:"transaction_details"`
	CustomerDetails    *CustomerDetails    `json:"customer_details,omitempty"`
	ItemDetails        []ItemDetail        `json:"item_details,omitempty"`
	GoPay              *GoPayOptions       `json:"gopay,omitempty"`
	ShopeePay          *ShopeePayOptions   `json:"shopeepay,omitempty"`
	CustomExpiry       *CustomExpiry       `json:"custom_expiry,omitempty"`
}

type CustomExpiry struct {
	OrderTime      string `json:"order_time,omitempty"`
	ExpiryDuration int    `json:"expiry_duration,omitempty"`
	Unit           string `json:"unit,omitempty"`
}

type Action struct {
	Name   string `json:"name"`
	Method string `json:"method"`
	URL    string `json:"url"`
}

type ChargeResponse struct {
	StatusCode        string          `json:"status_code"`
	StatusMessage     string          `json:"status_message"`
	TransactionID     string          `json:"transaction_id"`
	OrderID           string          `json:"order_id"`
	GrossAmount       string          `json:"gross_amount"`
	PaymentType       string          `json:"payment_type"`
	TransactionTime   string          `json:"transaction_time"`
	TransactionStatus string          `json:"transaction_status"`
	FraudStatus       string          `json:"fraud_status,omitempty"`
	Actions           []Action        `json:"actions,omitempty"`
	SettlementTime    string          `json:"settlement_time,omitempty"`
	RawResponse       json.RawMessage `json:"-"`
}

// Action returns the URL of the first action with the given name (empty when absent).
func (c *ChargeResponse) Action(name string) string {
	if c == nil {
		return ""
	}
	for _, a := range c.Actions {
		if a.Name == name {
			return a.URL
		}
	}
	return ""
}

type StatusResponse struct {
	StatusCode        string          `json:"status_code"`
	StatusMessage     string          `json:"status_message"`
	TransactionID     string          `json:"transaction_id"`
	OrderID           string          `json:"order_id"`
	GrossAmount       string          `json:"gross_amount"`
	PaymentType       string          `json:"payment_type"`
	TransactionTime   string          `json:"transaction_time"`
	TransactionStatus string          `json:"transaction_status"`
	SettlementTime    string          `json:"settlement_time,omitempty"`
	FraudStatus       string          `json:"fraud_status,omitempty"`
	RawResponse       json.RawMessage `json:"-"`
}

type CancelResponse struct {
	StatusCode        string          `json:"status_code"`
	StatusMessage     string          `json:"status_message"`
	TransactionID     string          `json:"transaction_id"`
	OrderID           string          `json:"order_id"`
	GrossAmount       string          `json:"gross_amount,omitempty"`
	PaymentType       string          `json:"payment_type,omitempty"`
	TransactionStatus string          `json:"transaction_status,omitempty"`
	RawResponse       json.RawMessage `json:"-"`
}

type RefundRequest struct {
	RefundKey string `json:"refund_key,omitempty"`
	Amount    int64  `json:"amount"`
	Reason    string `json:"reason,omitempty"`
}

type RefundResponse struct {
	StatusCode        string          `json:"status_code"`
	StatusMessage     string          `json:"status_message"`
	TransactionID     string          `json:"transaction_id"`
	OrderID           string          `json:"order_id"`
	RefundAmount      string          `json:"refund_amount,omitempty"`
	TransactionStatus string          `json:"transaction_status,omitempty"`
	RawResponse       json.RawMessage `json:"-"`
}

// WebhookPayload models the Midtrans notification body used for signature
// verification and idempotent processing.
type WebhookPayload struct {
	TransactionTime   string `json:"transaction_time"`
	TransactionStatus string `json:"transaction_status"`
	TransactionID     string `json:"transaction_id"`
	StatusMessage     string `json:"status_message"`
	StatusCode        string `json:"status_code"`
	SignatureKey      string `json:"signature_key"`
	PaymentType       string `json:"payment_type"`
	OrderID           string `json:"order_id"`
	MerchantID        string `json:"merchant_id"`
	GrossAmount       string `json:"gross_amount"`
	Currency          string `json:"currency"`
	FraudStatus       string `json:"fraud_status,omitempty"`
	SettlementTime    string `json:"settlement_time,omitempty"`
	RawBody           []byte `json:"-"`
}
