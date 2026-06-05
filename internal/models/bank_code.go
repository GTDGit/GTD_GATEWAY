package models

import "time"

type BankCode struct {
	ID                  int       `json:"id" db:"id"`
	Code                string    `json:"code" db:"code"`
	ShortName           string    `json:"shortName" db:"short_name"`
	Name                string    `json:"name" db:"name"`
	SwiftCode           *string   `json:"swiftCode,omitempty" db:"swift_code"`
	SupportVA           bool      `json:"support_va" db:"support_va"`
	SupportDisbursement bool      `json:"support_disbursement" db:"support_disbursement"`
	IsActive            bool      `json:"is_active" db:"is_active"`
	CreatedAt           time.Time `json:"created_at" db:"created_at"`
	UpdatedAt           time.Time `json:"updated_at" db:"updated_at"`
}

// BankCodeItem is the public API response item for GET /v1/bank-codes
type BankCodeItem struct {
	Name      string  `json:"name"`
	ShortName string  `json:"shortName"`
	Code      string  `json:"code"`
	SwiftCode *string `json:"swiftCode,omitempty"`
}
