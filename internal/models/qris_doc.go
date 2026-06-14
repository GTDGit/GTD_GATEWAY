package models

import "time"

// ----------------------------------------------------------------------------
// QRIS document portal (shared DB, migration 000065).
//
// A bundle is one shareable, token-gated link delivering a merchant's
// onboarding documents (KTP, selfie with KTP, business-location photo, plus one
// extra photo) to Nobu. The gateway owns upload + bundle creation; the
// files-qris portal service owns token-validated viewing/download/confirm.
// Files are ALWAYS private in S3 — never served via a public URL.
// ----------------------------------------------------------------------------

// QRISDocStatus is the lifecycle state of a bundle.
type QRISDocStatus string

const (
	QRISDocStatusActive  QRISDocStatus = "active"  // link is usable
	QRISDocStatusRevoked QRISDocStatus = "revoked" // confirmed/closed — all access returns 403
)

// QRISDocType classifies a single file within a bundle.
type QRISDocType string

const (
	QRISDocTypeKTP              QRISDocType = "ktp"
	QRISDocTypeSelfieKTP        QRISDocType = "selfie_ktp"
	QRISDocTypeBusinessLocation QRISDocType = "business_location"
	QRISDocTypeExtra            QRISDocType = "extra"
)

// QRISDocBundle is one shareable link for one merchant.
type QRISDocBundle struct {
	ID             int            `db:"id" json:"id"`
	Token          string         `db:"token" json:"token"`
	MerchantName   string         `db:"merchant_name" json:"merchantName"`
	QRISMerchantID *int           `db:"qris_merchant_id" json:"qrisMerchantId,omitempty"`
	Status         QRISDocStatus  `db:"status" json:"status"`
	Note           *string        `db:"note" json:"note,omitempty"`
	CreatedBy      *string        `db:"created_by" json:"createdBy,omitempty"`
	ConfirmedAt    *time.Time     `db:"confirmed_at" json:"confirmedAt,omitempty"`
	ExpiresAt      *time.Time     `db:"expires_at" json:"expiresAt,omitempty"`
	CreatedAt      time.Time      `db:"created_at" json:"createdAt"`
	UpdatedAt      time.Time      `db:"updated_at" json:"updatedAt"`

	// Files is populated on reads that join the bundle's files. Not a column.
	Files []QRISDocFile `db:"-" json:"files,omitempty"`
}

// QRISDocFile is one stored document within a bundle.
type QRISDocFile struct {
	ID          int         `db:"id" json:"id"`
	BundleID    int         `db:"bundle_id" json:"bundleId"`
	Token       string      `db:"token" json:"token"`
	DocType     QRISDocType `db:"doc_type" json:"docType"`
	FileName    string      `db:"file_name" json:"fileName"`
	ContentType string      `db:"content_type" json:"contentType"`
	SizeBytes   int64       `db:"size_bytes" json:"sizeBytes"`
	StorageKey  string      `db:"storage_key" json:"-"` // private S3 key — never exposed in JSON
	Checksum    *string     `db:"checksum" json:"checksum,omitempty"`
	CreatedAt   time.Time   `db:"created_at" json:"createdAt"`
}

// QRISDocAccessAction enumerates audited portal actions (PDP trail).
type QRISDocAccessAction string

const (
	QRISDocActionView      QRISDocAccessAction = "view"
	QRISDocActionDownload  QRISDocAccessAction = "download"
	QRISDocActionConfirm   QRISDocAccessAction = "confirm"
	QRISDocActionForbidden QRISDocAccessAction = "forbidden"
)

// QRISDocAccessLog is one PDP audit entry. bundle_id/file_id are nullable so a
// forbidden attempt on an unknown token can still be recorded.
type QRISDocAccessLog struct {
	ID        int                 `db:"id" json:"id"`
	BundleID  *int                `db:"bundle_id" json:"bundleId,omitempty"`
	FileID    *int                `db:"file_id" json:"fileId,omitempty"`
	Action    QRISDocAccessAction `db:"action" json:"action"`
	IP        *string             `db:"ip" json:"ip,omitempty"`
	UserAgent *string             `db:"user_agent" json:"userAgent,omitempty"`
	Detail    *string             `db:"detail" json:"detail,omitempty"`
	CreatedAt time.Time           `db:"created_at" json:"createdAt"`
}
