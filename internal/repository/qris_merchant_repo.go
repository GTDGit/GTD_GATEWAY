package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"

	"github.com/GTDGit/gtd_gateway/internal/models"
)

type QRISMerchantRepository struct {
	db *sqlx.DB
}

func NewQRISMerchantRepository(db *sqlx.DB) *QRISMerchantRepository {
	return &QRISMerchantRepository{db: db}
}

func nullableQRISMerchantJSON(v models.NullableRawMessage) any {
	if len(v) == 0 {
		return nil
	}
	return []byte(v)
}

const qrisMerchantColumns = `id, client_id, provider, merchant_name, merchant_city,
    merchant_category_code, nmid, store_id, terminal_id, qris_string, status,
    raw_provider_response, created_at, updated_at`

// QRISMerchantFilter is the admin-facing list filter.
type QRISMerchantFilter struct {
	Provider string
	ClientID int
	Status   string
	Search   string
}

func (r *QRISMerchantRepository) Create(ctx context.Context, m *models.QRISMerchant) error {
	const q = `INSERT INTO qris_merchants (
        client_id, provider, merchant_name, merchant_city, merchant_category_code,
        nmid, store_id, terminal_id, qris_string, status, raw_provider_response
    ) VALUES (
        $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
    ) RETURNING id, created_at, updated_at`
	return r.db.QueryRowContext(ctx, q,
		m.ClientID, m.Provider, m.MerchantName, m.MerchantCity, m.MerchantCategoryCode,
		m.NMID, m.StoreID, m.TerminalID, m.QRISString, m.Status,
		nullableQRISMerchantJSON(m.RawProviderResponse),
	).Scan(&m.ID, &m.CreatedAt, &m.UpdatedAt)
}

func (r *QRISMerchantRepository) Update(ctx context.Context, m *models.QRISMerchant) error {
	const q = `UPDATE qris_merchants SET
        client_id = $2,
        merchant_name = $3,
        merchant_city = $4,
        merchant_category_code = $5,
        nmid = $6,
        store_id = $7,
        terminal_id = $8,
        qris_string = $9,
        status = $10,
        raw_provider_response = $11,
        updated_at = NOW()
    WHERE id = $1
    RETURNING updated_at`
	return r.db.QueryRowContext(ctx, q,
		m.ID, m.ClientID, m.MerchantName, m.MerchantCity, m.MerchantCategoryCode,
		m.NMID, m.StoreID, m.TerminalID, m.QRISString, m.Status,
		nullableQRISMerchantJSON(m.RawProviderResponse),
	).Scan(&m.UpdatedAt)
}

func (r *QRISMerchantRepository) GetByID(ctx context.Context, id int) (*models.QRISMerchant, error) {
	q := `SELECT ` + qrisMerchantColumns + ` FROM qris_merchants WHERE id = $1 LIMIT 1`
	var m models.QRISMerchant
	if err := r.db.GetContext(ctx, &m, q, id); err != nil {
		if err == sql.ErrNoRows {
			return nil, sql.ErrNoRows
		}
		return nil, err
	}
	return &m, nil
}

func (r *QRISMerchantRepository) List(ctx context.Context, f QRISMerchantFilter, limit, offset int) ([]models.QRISMerchant, int, error) {
	where := []string{"1=1"}
	args := []any{}
	idx := 1
	add := func(clause string, val any) {
		where = append(where, strings.ReplaceAll(clause, "?", fmt.Sprintf("$%d", idx)))
		args = append(args, val)
		idx++
	}
	if f.Provider != "" {
		add("provider = ?", f.Provider)
	}
	if f.ClientID > 0 {
		add("client_id = ?", f.ClientID)
	}
	if f.Status != "" {
		add("status = ?", f.Status)
	}
	if f.Search != "" {
		pattern := "%" + f.Search + "%"
		where = append(where, fmt.Sprintf("(merchant_name ILIKE $%d OR store_id ILIKE $%d OR nmid ILIKE $%d)", idx, idx, idx))
		args = append(args, pattern)
		idx++
	}

	whereClause := strings.Join(where, " AND ")
	countQ := `SELECT COUNT(*) FROM qris_merchants WHERE ` + whereClause
	var total int
	if err := r.db.GetContext(ctx, &total, countQ, args...); err != nil {
		return nil, 0, err
	}

	q := `SELECT ` + qrisMerchantColumns + ` FROM qris_merchants WHERE ` + whereClause +
		fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", idx, idx+1)
	args = append(args, limit, offset)

	rows := []models.QRISMerchant{}
	if err := r.db.SelectContext(ctx, &rows, q, args...); err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}
