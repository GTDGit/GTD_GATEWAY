package models

import (
	"encoding/json"
	"time"
)

// TransactionLog stores upstream transaction request/response log entries.
type TransactionLog struct {
	ID             int             `db:"id"`
	TransactionID  int             `db:"transaction_id"`
	SkuID          *int            `db:"sku_id"`
	DigiRefID      string          `db:"digi_ref_id"`
	Request        json.RawMessage `db:"request"`
	Response       json.RawMessage `db:"response"`
	RC             *string         `db:"rc"`
	Status         *string         `db:"status"`
	Message        *string         `db:"message"`
	ProviderID     *int            `db:"provider_id"`
	ProviderSKUID  *int            `db:"provider_sku_id"`
	CreatedAt      time.Time       `db:"created_at"`
	ResponseAt     *time.Time      `db:"response_at"`
	ResponseTimeMs *int            `db:"response_time_ms"`
}

// CallbackLog stores outgoing webhook attempts to client systems.
type CallbackLog struct {
	ID            int             `db:"id"`
	ClientID      int             `db:"client_id"`
	TransactionID *int            `db:"transaction_id"`
	PaymentID     *int            `db:"payment_id"`
	Event         string          `db:"event"`
	Payload       json.RawMessage `db:"payload"`
	Attempt       int             `db:"attempt"`
	MaxAttempts   int             `db:"max_attempts"`
	HTTPStatus    *int            `db:"http_status"`
	ResponseBody  *string         `db:"response_body"`
	ResponseTime  *int            `db:"response_time_ms"`
	IsDelivered   bool            `db:"is_delivered"`
	ErrorMessage  *string         `db:"error_message"`
	CreatedAt     time.Time       `db:"created_at"`
	NextRetryAt   *time.Time      `db:"next_retry_at"`
	DeliveredAt   *time.Time      `db:"delivered_at"`
}

// DigiflazzCallback stores raw callback payload from Digiflazz.
type DigiflazzCallback struct {
	ID           int             `db:"id"`
	DigiRefID    string          `db:"digi_ref_id"`
	Payload      json.RawMessage `db:"payload"`
	RC           *string         `db:"rc"`
	Status       *string         `db:"status"`
	SerialNumber *string         `db:"serial_number"`
	Message      *string         `db:"message"`
	IsProcessed  bool            `db:"is_processed"`
	ProcessedAt  *time.Time      `db:"processed_at"`
	ProcessError *string         `db:"process_error"`
	CreatedAt    time.Time       `db:"created_at"`
}
