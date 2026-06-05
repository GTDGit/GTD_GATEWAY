package models

import (
	"database/sql/driver"
	"encoding/json"
	"time"
)

type TransactionType string
type TransactionStatus string

const (
	TrxTypePrepaid TransactionType = "prepaid"
	TrxTypeInquiry TransactionType = "inquiry"
	TrxTypePayment TransactionType = "payment"
)

const (
	StatusProcessing TransactionStatus = "Processing"
	StatusSuccess    TransactionStatus = "Success"
	StatusPending    TransactionStatus = "Pending"
	StatusFailed     TransactionStatus = "Failed"
)

// NullableRawMessage handles NULL values for JSONB columns.
type NullableRawMessage json.RawMessage

// Scan implements sql.Scanner interface for NULL-safe JSONB scanning.
//
// The []byte returned by database/sql drivers is owned by the driver and
// may be reused on the next Scan call. We must copy it so subsequent rows
// don't overwrite the bytes we hold a pointer to.
func (n *NullableRawMessage) Scan(value interface{}) error {
	if value == nil {
		*n = nil
		return nil
	}
	switch v := value.(type) {
	case []byte:
		buf := make([]byte, len(v))
		copy(buf, v)
		*n = NullableRawMessage(buf)
	case string:
		*n = NullableRawMessage(v)
	}
	return nil
}

// Value implements driver.Valuer interface.
func (n NullableRawMessage) Value() (driver.Value, error) {
	if len(n) == 0 {
		return nil, nil
	}
	return []byte(n), nil
}

// MarshalJSON implements json.Marshaler.
func (n NullableRawMessage) MarshalJSON() ([]byte, error) {
	if len(n) == 0 {
		return []byte("null"), nil
	}
	return json.RawMessage(n).MarshalJSON()
}

// UnmarshalJSON implements json.Unmarshaler.
func (n *NullableRawMessage) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*n = nil
		return nil
	}
	*n = NullableRawMessage(data)
	return nil
}

// Transaction captures the lifecycle information for a customer transaction.
// Many fields are optional to accommodate different transaction types.
type Transaction struct {
	ID            int                `db:"id" json:"-"`
	TransactionID string             `db:"transaction_id" json:"transactionId"`
	ReferenceID   string             `db:"reference_id" json:"referenceId"`
	ClientID      int                `db:"client_id" json:"-"`
	ProductID     int                `db:"product_id" json:"-"`
	SkuID         *int               `db:"sku_id" json:"-"`
	SkuCode       string             `db:"-" json:"skuCode,omitempty"` // Product SKU code (from JOIN)
	DigiSkuCode   *string            `db:"-" json:"-"`                 // Digiflazz SKU code used (from JOIN)
	IsSandbox     bool               `db:"is_sandbox" json:"-"`
	CustomerNo    string             `db:"customer_no" json:"customerNo"`
	CustomerName  *string            `db:"customer_name" json:"customerName,omitempty"`
	Type          TransactionType    `db:"type" json:"type"`
	Status        TransactionStatus  `db:"status" json:"status"`
	SerialNumber  *string            `db:"serial_number" json:"serialNumber,omitempty"`
	Amount        *int               `db:"amount" json:"amount,omitempty"`
	Admin         int                `db:"admin" json:"admin,omitempty"`
	Period        *string            `db:"period" json:"period,omitempty"`
	Description   NullableRawMessage `db:"description" json:"description,omitempty"`
	FailedReason  *string            `db:"failed_reason" json:"failedReason,omitempty"`
	FailedCode    *string            `db:"failed_code" json:"failedCode,omitempty"`
	RetryCount    int                `db:"retry_count" json:"retryCount,omitempty"`
	MaxRetry      int                `db:"max_retry" json:"-"`
	NextRetryAt   *time.Time         `db:"next_retry_at" json:"nextRetryAt,omitempty"`
	ExpiredAt     *time.Time         `db:"expired_at" json:"expiredAt,omitempty"`
	InquiryID     *int               `db:"inquiry_id" json:"-"`
	DigiRefID     *string            `db:"digi_ref_id" json:"-"`
	CallbackSent  bool               `db:"callback_sent" json:"-"`
	CallbackAt    *time.Time         `db:"callback_sent_at" json:"-"`
	CreatedAt     time.Time          `db:"created_at" json:"createdAt"`
	ProcessedAt   *time.Time         `db:"processed_at" json:"processedAt,omitempty"`
	UpdatedAt     time.Time          `db:"updated_at" json:"-"`

	// Price tracking: buy_price = actual cost from provider, sell_price = price shown to client
	BuyPrice  *int `db:"buy_price" json:"-"`
	SellPrice *int `db:"sell_price" json:"price,omitempty"`

	// Multi-provider fields
	ProviderID                *int               `db:"provider_id" json:"-"`
	ProviderSKUID             *int               `db:"provider_sku_id" json:"-"`
	ProviderCode              *string            `db:"provider_code" json:"providerCode,omitempty"` // Populated from JOIN with ppob_providers
	ProviderRefID             *string            `db:"provider_ref_id" json:"-"`
	ProviderInitialResponse   NullableRawMessage `db:"provider_initial_response" json:"-"`
	ProviderResponse          NullableRawMessage `db:"provider_response" json:"-"`
	ProviderInitialHTTPStatus *int               `db:"provider_initial_http_status" json:"-"`
	ProviderHTTPStatus        *int               `db:"provider_http_status" json:"-"`
}
