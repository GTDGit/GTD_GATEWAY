package xendit

import "encoding/json"

type APIError struct {
	HTTPStatus int
	ErrorCode  string `json:"error_code"`
	Message    string `json:"message"`
	Errors     []any  `json:"errors,omitempty"`
	RawResponse json.RawMessage
}

func (e *APIError) Error() string {
	if e == nil {
		return ""
	}
	if e.ErrorCode == "" {
		return e.Message
	}
	return e.ErrorCode + ": " + e.Message
}

const (
	ChannelAlfamart  = "ALFAMART"
	ChannelIndomaret = "INDOMARET"

	StatusAccepting = "ACCEPTING_PAYMENTS"
	StatusSucceeded = "SUCCEEDED"
	StatusExpired   = "EXPIRED"
	StatusFailed    = "FAILED"
	StatusCanceled  = "CANCELED"

	EventCapture  = "payment.capture"
	EventFailure  = "payment.failure"
	EventAuthorization = "payment.authorization"
)

type PaymentRequestChannelProperties struct {
	PayerName   string `json:"payer_name,omitempty"`
	ExpiresAt   string `json:"expires_at,omitempty"`
	PaymentCode string `json:"payment_code,omitempty"`
}

type PaymentRequestCreate struct {
	ReferenceID       string                          `json:"reference_id"`
	Type              string                          `json:"type"`
	Country           string                          `json:"country"`
	Currency          string                          `json:"currency"`
	ChannelCode       string                          `json:"channel_code"`
	RequestAmount     int64                           `json:"request_amount"`
	ChannelProperties PaymentRequestChannelProperties `json:"channel_properties"`
	CaptureMethod     string                          `json:"capture_method,omitempty"`
	Description       string                          `json:"description,omitempty"`
	Metadata          map[string]any                  `json:"metadata,omitempty"`
}

type PaymentRequest struct {
	BusinessID        string                          `json:"business_id"`
	ReferenceID       string                          `json:"reference_id"`
	PaymentRequestID  string                          `json:"payment_request_id"`
	LatestPaymentID   string                          `json:"latest_payment_id,omitempty"`
	Type              string                          `json:"type"`
	Country           string                          `json:"country"`
	Currency          string                          `json:"currency"`
	RequestAmount     int64                           `json:"request_amount"`
	CaptureMethod     string                          `json:"capture_method"`
	ChannelCode       string                          `json:"channel_code"`
	ChannelProperties PaymentRequestChannelProperties `json:"channel_properties"`
	Status            string                          `json:"status"`
	Description       string                          `json:"description"`
	Created           string                          `json:"created"`
	Updated           string                          `json:"updated"`
	FailureCode       string                          `json:"failure_code,omitempty"`
	Actions           []any                           `json:"actions,omitempty"`
	RawResponse       json.RawMessage                 `json:"-"`
}

type RefundCreate struct {
	Amount      int64          `json:"amount"`
	ReferenceID string         `json:"reference_id,omitempty"`
	Reason      string         `json:"reason"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type Refund struct {
	RefundID         string          `json:"refund_id"`
	PaymentRequestID string          `json:"payment_request_id,omitempty"`
	PaymentID        string          `json:"payment_id,omitempty"`
	ReferenceID      string          `json:"reference_id,omitempty"`
	Currency         string          `json:"currency"`
	Amount           int64           `json:"amount"`
	Status           string          `json:"status"`
	Reason           string          `json:"reason"`
	Created          string          `json:"created"`
	Updated          string          `json:"updated"`
	RawResponse      json.RawMessage `json:"-"`
}

type WebhookPayload struct {
	Event      string         `json:"event"`
	BusinessID string         `json:"business_id"`
	Created    string         `json:"created"`
	Data       WebhookData    `json:"data"`
	RawBody    []byte         `json:"-"`
}

type WebhookData struct {
	PaymentID        string         `json:"payment_id"`
	PaymentRequestID string         `json:"payment_request_id"`
	ReferenceID      string         `json:"reference_id"`
	Status           string         `json:"status"`
	RequestAmount    int64          `json:"request_amount"`
	ChannelCode      string         `json:"channel_code"`
	Country          string         `json:"country"`
	Currency         string         `json:"currency"`
	Description      string         `json:"description,omitempty"`
	FailureCode      string         `json:"failure_code,omitempty"`
	Created          string         `json:"created"`
	Updated          string         `json:"updated"`
	Metadata         map[string]any `json:"metadata,omitempty"`
}
