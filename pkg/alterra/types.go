package alterra

import (
	"encoding/json"
	"strings"
)

// FlexBool handles JSON booleans that may come as strings ("true"/"false")
type FlexBool bool

func (fb *FlexBool) UnmarshalJSON(data []byte) error {
	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	switch v := raw.(type) {
	case bool:
		*fb = FlexBool(v)
	case string:
		*fb = FlexBool(strings.EqualFold(v, "true") || v == "1")
	case float64:
		*fb = FlexBool(v != 0)
	default:
		*fb = false
	}
	return nil
}

// Common Response

// ErrorResponse represents an error from Alterra
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail contains error information
type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Product Types

// Product represents a product in the catalog
type Product struct {
	ProductID   int      `json:"product_id"`
	Code        string   `json:"code"`
	Label       string   `json:"label"`
	ProductType string   `json:"product_type"`
	Operator    string   `json:"operator"`
	Nominal     int      `json:"nominal"`
	Price       int      `json:"price"`
	Enable      FlexBool `json:"enable"`
}

// ProductListResponse represents product list response
type ProductListResponse struct {
	TotalRecords int       `json:"total_records"`
	CurrentPage  int       `json:"current_page"`
	TotalPages   int       `json:"total_pages"`
	Data         []Product `json:"data"`
}

// Balance Types

// BalanceResponse represents balance response
type BalanceResponse struct {
	Balance int `json:"balance"`
}

// Transaction Types

// PurchaseRequest represents a purchase request
type PurchaseRequest struct {
	CustomerID string          `json:"customer_id"`
	ProductID  int             `json:"product_id"`
	OrderID    string          `json:"order_id"`
	Data       json.RawMessage `json:"data"`
}

// InquiryRequest represents an inquiry request
type InquiryRequest struct {
	CustomerID  string          `json:"customer_id"`
	InquiryType string          `json:"inquiry_type"`
	Data        json.RawMessage `json:"data"`
}

// PaymentRequest represents a payment request
type PaymentRequest struct {
	CustomerID string          `json:"customer_id"`
	ProductID  int             `json:"product_id"`
	OrderID    string          `json:"order_id"`
	Data       json.RawMessage `json:"data"`
}

// TransactionProduct represents product details in transaction response
type TransactionProduct struct {
	ProductID   int      `json:"product_id"`
	ProductCode string   `json:"product_code"`
	Type        string   `json:"type"`
	Label       string   `json:"label"`
	Operator    string   `json:"operator"`
	Nominal     int      `json:"nominal"`
	Price       int      `json:"price"`
	Enabled     FlexBool `json:"enabled"`
}

// TransactionData contains additional transaction data
type TransactionData struct {
	Nominal     int             `json:"nominal,omitempty"`
	Admin       int             `json:"admin,omitempty"`
	Identifier  json.RawMessage `json:"identifier,omitempty"`
	Token       string          `json:"token,omitempty"`
	KWH         string          `json:"kwh,omitempty"`
	Period      string          `json:"period,omitempty"`
	BillInfo    json.RawMessage `json:"bill_info,omitempty"`
	RefNumber   string          `json:"ref_number,omitempty"`
	ReferenceNo string          `json:"reference_no,omitempty"` // Inquiry reference (billing/utility)
	VendorRefNo string          `json:"vendor_refnum,omitempty"`
}

// TransactionResponse represents a transaction response
type TransactionResponse struct {
	TransactionID int                `json:"transaction_id"`
	Type          string             `json:"type"`
	CreatedAt     int64              `json:"created_at"`
	UpdatedAt     int64              `json:"updated_at"`
	CustomerID    string             `json:"customer_id"`
	CustomerName  string             `json:"customer_name,omitempty"`
	OrderID       string             `json:"order_id"`
	Price         int                `json:"price"`
	Status        string             `json:"status"`
	ResponseCode  string             `json:"response_code"`
	Amount        int                `json:"amount"`
	Admin         int                `json:"admin,omitempty"`
	ReferenceNo   string             `json:"reference_no,omitempty"` // Inquiry reference for subsequent purchase
	Code          string             `json:"code,omitempty"`         // Some Alterra business errors use top-level code
	Message       string             `json:"message,omitempty"`      // Some Alterra business errors use top-level message
	Product       TransactionProduct `json:"product"`
	Data          *TransactionData   `json:"data"`
	Error         *ErrorDetail       `json:"error"`
	RawResponse   json.RawMessage    `json:"-"`
	HTTPStatus    int                `json:"-"`
}

// TransactionDetailResponse represents transaction detail response
type TransactionDetailResponse struct {
	TransactionID int                `json:"transaction_id"`
	Type          string             `json:"type"`
	CreatedAt     int64              `json:"created_at"`
	UpdatedAt     int64              `json:"updated_at"`
	CustomerID    string             `json:"customer_id"`
	CustomerName  string             `json:"customer_name,omitempty"`
	OrderID       string             `json:"order_id"`
	Price         int                `json:"price"`
	Status        string             `json:"status"`
	ResponseCode  string             `json:"response_code"`
	Amount        int                `json:"amount"`
	Admin         int                `json:"admin,omitempty"`
	Code          string             `json:"code,omitempty"`
	Message       string             `json:"message,omitempty"`
	Product       TransactionProduct `json:"product"`
	Data          *TransactionData   `json:"data"`
	RawResponse   json.RawMessage    `json:"-"`
	HTTPStatus    int                `json:"-"`
}

// CallbackPayload represents callback from Alterra
type CallbackPayload struct {
	TransactionID string           `json:"transaction_id"`
	CustomerID    string           `json:"customer_id"`
	OrderID       string           `json:"order_id"`
	ResponseCode  string           `json:"response_code"`
	Product       json.RawMessage  `json:"product"`
	Created       int64            `json:"created"`
	Changed       int64            `json:"changed"`
	Price         int              `json:"price"`
	Data          *TransactionData `json:"data"`
}
