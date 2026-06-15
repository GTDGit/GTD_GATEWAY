package models

import "time"

// ----------------------------------------------------------------------------
// File portal (read model).
//
// The standalone files portal (dev-files.gtd.co.id) owns upload + delivery and
// writes these tables: file_bundles, file_items, file_access_logs. The gateway
// is READ-ONLY here — admins inspect links and their status and may force-close
// (revoke) a bundle. The gateway no longer uploads.
// ----------------------------------------------------------------------------

// QRISDocStatus is the lifecycle state of a bundle.
type QRISDocStatus string

const (
	QRISDocStatusActive  QRISDocStatus = "active"  // link is usable
	QRISDocStatusRevoked QRISDocStatus = "revoked" // confirmed/closed — all access returns 403
)

// QRISDocAccessMode controls a bundle's link lifetime.
type QRISDocAccessMode string

const (
	QRISDocAccessOpen QRISDocAccessMode = "open" // free repeat access until revoke/expiry
	QRISDocAccessOnce QRISDocAccessMode = "once" // revoked after the first download
)

// QRISDocBundle is one shareable link (row in file_bundles).
type QRISDocBundle struct {
	ID          int               `db:"id" json:"id"`
	Token       string            `db:"token" json:"token"`
	Title       string            `db:"title" json:"title"`
	Note        *string           `db:"note" json:"note,omitempty"`
	AccessMode  QRISDocAccessMode `db:"access_mode" json:"accessMode"`
	Status      QRISDocStatus     `db:"status" json:"status"`
	CreatedBy   *string           `db:"created_by" json:"createdBy,omitempty"`
	ConfirmedAt *time.Time        `db:"confirmed_at" json:"confirmedAt,omitempty"`
	ExpiresAt   *time.Time        `db:"expires_at" json:"expiresAt,omitempty"`
	CreatedAt   time.Time         `db:"created_at" json:"createdAt"`
	UpdatedAt   time.Time         `db:"updated_at" json:"updatedAt"`

	// Files is populated on reads that join the bundle's files. Not a column.
	Files []QRISDocFile `db:"-" json:"files,omitempty"`
}

// QRISDocFile is one stored file within a bundle (row in file_items).
type QRISDocFile struct {
	ID          int       `db:"id" json:"id"`
	BundleID    int       `db:"bundle_id" json:"bundleId"`
	Token       string    `db:"token" json:"token"`
	DocName     *string   `db:"doc_name" json:"docName,omitempty"`
	FileName    string    `db:"file_name" json:"fileName"`
	ContentType string    `db:"content_type" json:"contentType"`
	SizeBytes   int64     `db:"size_bytes" json:"sizeBytes"`
	StorageKey  string    `db:"storage_key" json:"-"` // private S3 key — never exposed in JSON
	Checksum    *string   `db:"checksum" json:"checksum,omitempty"`
	CreatedAt   time.Time `db:"created_at" json:"createdAt"`
}

// QRISDocAccessAction enumerates audited portal actions (PDP trail).
type QRISDocAccessAction string

const (
	QRISDocActionView      QRISDocAccessAction = "view"
	QRISDocActionDownload  QRISDocAccessAction = "download"
	QRISDocActionConfirm   QRISDocAccessAction = "confirm"
	QRISDocActionForbidden QRISDocAccessAction = "forbidden"
	QRISDocActionUpload    QRISDocAccessAction = "upload"
)

// QRISDocAccessLog is one PDP audit entry (row in file_access_logs).
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
