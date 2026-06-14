package repository

import (
	"context"
	"database/sql"

	"github.com/jmoiron/sqlx"

	"github.com/GTDGit/gtd_gateway/internal/models"
)

// QRISDocRepository provides data access for the QRIS document portal tables
// (qris_doc_bundles, qris_doc_files, qris_doc_access_logs), migration 000065.
type QRISDocRepository struct {
	db *sqlx.DB
}

// NewQRISDocRepository constructs a QRISDocRepository.
func NewQRISDocRepository(db *sqlx.DB) *QRISDocRepository {
	return &QRISDocRepository{db: db}
}

const qrisDocBundleColumns = `id, token, merchant_name, qris_merchant_id, status,
    note, created_by, confirmed_at, expires_at, created_at, updated_at`

const qrisDocFileColumns = `id, bundle_id, token, doc_type, file_name,
    content_type, size_bytes, storage_key, checksum, created_at`

func scanBundle(scanner interface{ Scan(dest ...any) error }, b *models.QRISDocBundle) error {
	return scanner.Scan(
		&b.ID, &b.Token, &b.MerchantName, &b.QRISMerchantID, &b.Status,
		&b.Note, &b.CreatedBy, &b.ConfirmedAt, &b.ExpiresAt, &b.CreatedAt, &b.UpdatedAt,
	)
}

func scanFile(scanner interface{ Scan(dest ...any) error }, f *models.QRISDocFile) error {
	return scanner.Scan(
		&f.ID, &f.BundleID, &f.Token, &f.DocType, &f.FileName,
		&f.ContentType, &f.SizeBytes, &f.StorageKey, &f.Checksum, &f.CreatedAt,
	)
}

// CreateBundle inserts a bundle. Token defaults in DB if b.Token is empty, so we
// let Postgres generate it and return everything.
func (r *QRISDocRepository) CreateBundle(ctx context.Context, b *models.QRISDocBundle) error {
	query := `INSERT INTO qris_doc_bundles
        (merchant_name, qris_merchant_id, note, created_by, expires_at)
        VALUES ($1, $2, $3, $4, $5)
        RETURNING ` + qrisDocBundleColumns
	row := r.db.QueryRowxContext(ctx, query,
		b.MerchantName, b.QRISMerchantID, b.Note, b.CreatedBy, b.ExpiresAt)
	return scanBundle(row, b)
}

// CreateFile inserts a file under a bundle and returns the generated row. The
// token is supplied by the caller (generated in Go so the S3 key is known
// before upload); the DB default only applies if it were omitted.
func (r *QRISDocRepository) CreateFile(ctx context.Context, f *models.QRISDocFile) error {
	query := `INSERT INTO qris_doc_files
        (bundle_id, token, doc_type, file_name, content_type, size_bytes, storage_key, checksum)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
        RETURNING ` + qrisDocFileColumns
	row := r.db.QueryRowxContext(ctx, query,
		f.BundleID, f.Token, f.DocType, f.FileName, f.ContentType, f.SizeBytes, f.StorageKey, f.Checksum)
	return scanFile(row, f)
}

// GetBundleByToken fetches a bundle by its link token. Returns sql.ErrNoRows
// when not found.
func (r *QRISDocRepository) GetBundleByToken(ctx context.Context, token string) (*models.QRISDocBundle, error) {
	query := `SELECT ` + qrisDocBundleColumns + ` FROM qris_doc_bundles WHERE token = $1 LIMIT 1`
	row := r.db.QueryRowxContext(ctx, query, token)
	var b models.QRISDocBundle
	if err := scanBundle(row, &b); err != nil {
		return nil, err
	}
	return &b, nil
}

// GetBundleByID fetches a bundle by numeric id.
func (r *QRISDocRepository) GetBundleByID(ctx context.Context, id int) (*models.QRISDocBundle, error) {
	query := `SELECT ` + qrisDocBundleColumns + ` FROM qris_doc_bundles WHERE id = $1 LIMIT 1`
	row := r.db.QueryRowxContext(ctx, query, id)
	var b models.QRISDocBundle
	if err := scanBundle(row, &b); err != nil {
		return nil, err
	}
	return &b, nil
}

// ListFilesByBundle returns all files belonging to a bundle, oldest first.
func (r *QRISDocRepository) ListFilesByBundle(ctx context.Context, bundleID int) ([]models.QRISDocFile, error) {
	query := `SELECT ` + qrisDocFileColumns + ` FROM qris_doc_files WHERE bundle_id = $1 ORDER BY created_at`
	rows, err := r.db.QueryxContext(ctx, query, bundleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []models.QRISDocFile
	for rows.Next() {
		var f models.QRISDocFile
		if err := scanFile(rows, &f); err != nil {
			return nil, err
		}
		files = append(files, f)
	}
	return files, rows.Err()
}

// GetFileByToken fetches a single file plus its parent bundle's token/status so
// the portal can enforce revocation without a second query.
func (r *QRISDocRepository) GetFileByToken(ctx context.Context, token string) (*models.QRISDocFile, error) {
	query := `SELECT ` + qrisDocFileColumns + ` FROM qris_doc_files WHERE token = $1 LIMIT 1`
	row := r.db.QueryRowxContext(ctx, query, token)
	var f models.QRISDocFile
	if err := scanFile(row, &f); err != nil {
		return nil, err
	}
	return &f, nil
}

// ListBundles returns bundles newest-first for the admin list view (without files).
func (r *QRISDocRepository) ListBundles(ctx context.Context, limit, offset int) ([]models.QRISDocBundle, int, error) {
	var total int
	if err := r.db.QueryRowxContext(ctx, `SELECT COUNT(*) FROM qris_doc_bundles`).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `SELECT ` + qrisDocBundleColumns + ` FROM qris_doc_bundles
        ORDER BY created_at DESC LIMIT $1 OFFSET $2`
	rows, err := r.db.QueryxContext(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var bundles []models.QRISDocBundle
	for rows.Next() {
		var b models.QRISDocBundle
		if err := scanBundle(rows, &b); err != nil {
			return nil, 0, err
		}
		bundles = append(bundles, b)
	}
	return bundles, total, rows.Err()
}

// RevokeBundleByToken marks a bundle revoked and stamps confirmed_at
// (idempotent: a second confirm keeps the first timestamp). Returns
// sql.ErrNoRows if no bundle has that token.
func (r *QRISDocRepository) RevokeBundleByToken(ctx context.Context, token string) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE qris_doc_bundles
         SET status = 'revoked',
             confirmed_at = COALESCE(confirmed_at, now()),
             updated_at = now()
         WHERE token = $1`, token)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// LogAccess appends a PDP audit row. Failures here must not block the user
// action, so callers typically log-and-continue on error.
func (r *QRISDocRepository) LogAccess(ctx context.Context, l *models.QRISDocAccessLog) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO qris_doc_access_logs (bundle_id, file_id, action, ip, user_agent, detail)
         VALUES ($1, $2, $3, $4, $5, $6)`,
		l.BundleID, l.FileID, l.Action, l.IP, l.UserAgent, l.Detail)
	return err
}
