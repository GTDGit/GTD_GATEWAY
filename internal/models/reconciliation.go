package models

import "time"

// ----------------------------------------------------------------------------
// PaymentReconciliation is the gateway read/write model for the
// payment_reconciliations table (schema owned by the api service, migration
// 000061). A row exists when an inbound provider webhook disagreed with the
// authoritative provider inquiry; the admin panel lists open rows and resolves
// them.
// ----------------------------------------------------------------------------

type ReconReason string

const (
	ReconReasonStatusMismatch       ReconReason = "status_mismatch"
	ReconReasonAmountMismatch       ReconReason = "amount_mismatch"
	ReconReasonStatusAmountMismatch ReconReason = "status_amount_mismatch"
)

type ReconStatus string

const (
	ReconStatusOpen     ReconStatus = "open"
	ReconStatusResolved ReconStatus = "resolved"
)

type PaymentReconciliation struct {
	ID             int64              `db:"id" json:"id"`
	PaymentID      string             `db:"payment_id" json:"paymentId"`
	Provider       PaymentProvider    `db:"provider" json:"provider"`
	Reason         ReconReason        `db:"reason" json:"reason"`
	WebhookStatus  *string            `db:"webhook_status" json:"webhookStatus,omitempty"`
	InquiryStatus  *string            `db:"inquiry_status" json:"inquiryStatus,omitempty"`
	WebhookAmount  *int64             `db:"webhook_amount" json:"webhookAmount,omitempty"`
	InquiryAmount  *int64             `db:"inquiry_amount" json:"inquiryAmount,omitempty"`
	ExpectedAmount *int64             `db:"expected_amount" json:"expectedAmount,omitempty"`
	WebhookPayload NullableRawMessage `db:"webhook_payload" json:"webhookPayload,omitempty"`
	InquiryPayload NullableRawMessage `db:"inquiry_payload" json:"inquiryPayload,omitempty"`
	Status         ReconStatus        `db:"status" json:"status"`
	ResolvedStatus *string            `db:"resolved_status" json:"resolvedStatus,omitempty"`
	ResolvedBy     *string            `db:"resolved_by" json:"resolvedBy,omitempty"`
	ResolutionNote *string            `db:"resolution_note" json:"resolutionNote,omitempty"`
	CreatedAt      time.Time          `db:"created_at" json:"createdAt"`
	UpdatedAt      time.Time          `db:"updated_at" json:"updatedAt"`
	ResolvedAt     *time.Time         `db:"resolved_at" json:"resolvedAt,omitempty"`
}
