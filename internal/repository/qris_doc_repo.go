package repository

import (
	"context"
	"database/sql"

	"github.com/jmoiron/sqlx"

	"github.com/GTDGit/gtd_gateway/internal/models"
)

// QRISDocRepository provides READ access to the file portal tables
// (file_bundles, file_items, file_access_logs) owned by the standalone files
// portal. The gateway only lists/inspects bundles and may force-close (revoke)
// one; it never creates bundles or files (upload lives in the portal).
type QRISDocRepository struct {
	db *sqlx.DB
}

// NewQRISDocRepository constructs a QRISDocRepository.
func NewQRISDocRepository(db *sqlx.DB) *QRISDocRepository {
	return &QRISDocRepository{db: db}
}

const qrisDocBundleColumns = `id, token, title, note, access_mode, status,
    created_by, confirmed_at, expires_at, created_at, updated_at`

const qrisDocFileColumns = `id, bundle_id, token, doc_name, file_name,
    content_type, size_bytes, storage_key, checksum, created_at`

func scanBundle(scanner interface{ Scan(dest ...any) error }, b *models.QRISDocBundle) error {
	return scanner.Scan(
		&b.ID, &b.Token, &b.Title, &b.Note, &b.AccessMode, &b.Status,
		&b.CreatedBy, &b.ConfirmedAt, &b.ExpiresAt, &b.CreatedAt, &b.UpdatedAt,
	)
}

func scanFile(scanner interface{ Scan(dest ...any) error }, f *models.QRISDocFile) error {
	return scanner.Scan(
		&f.ID, &f.BundleID, &f.Token, &f.DocName, &f.FileName,
		&f.ContentType, &f.SizeBytes, &f.StorageKey, &f.Checksum, &f.CreatedAt,
	)
}

// GetBundleByToken fetches a bundle by its link token. Returns sql.ErrNoRows
// when not found.
func (r *QRISDocRepository) GetBundleByToken(ctx context.Context, token string) (*models.QRISDocBundle, error) {
	query := `SELECT ` + qrisDocBundleColumns + ` FROM file_bundles WHERE token = $1 LIMIT 1`
	row := r.db.QueryRowxContext(ctx, query, token)
	var b models.QRISDocBundle
	if err := scanBundle(row, &b); err != nil {
		return nil, err
	}
	return &b, nil
}

// GetBundleByID fetches a bundle by numeric id.
func (r *QRISDocRepository) GetBundleByID(ctx context.Context, id int) (*models.QRISDocBundle, error) {
	query := `SELECT ` + qrisDocBundleColumns + ` FROM file_bundles WHERE id = $1 LIMIT 1`
	row := r.db.QueryRowxContext(ctx, query, id)
	var b models.QRISDocBundle
	if err := scanBundle(row, &b); err != nil {
		return nil, err
	}
	return &b, nil
}

// ListFilesByBundle returns all files belonging to a bundle, oldest first.
func (r *QRISDocRepository) ListFilesByBundle(ctx context.Context, bundleID int) ([]models.QRISDocFile, error) {
	query := `SELECT ` + qrisDocFileColumns + ` FROM file_items WHERE bundle_id = $1 ORDER BY created_at`
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

// ListBundles returns bundles newest-first for the admin list view (without files).
func (r *QRISDocRepository) ListBundles(ctx context.Context, limit, offset int) ([]models.QRISDocBundle, int, error) {
	var total int
	if err := r.db.QueryRowxContext(ctx, `SELECT COUNT(*) FROM file_bundles`).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `SELECT ` + qrisDocBundleColumns + ` FROM file_bundles
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
		`UPDATE file_bundles
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
