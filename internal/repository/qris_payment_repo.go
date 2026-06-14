package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/GTDGit/gtd_gateway/internal/models"
)

type QRISPaymentRepository struct {
	db *sqlx.DB
}

func NewQRISPaymentRepository(db *sqlx.DB) *QRISPaymentRepository {
	return &QRISPaymentRepository{db: db}
}

const qrisPaymentColumns = `id, qris_merchant_id, provider, reference_no, partner_reference_no,
    rrn, payment_reference_no, issuer_id, store_id, terminal_id,
    amount, fee_amount, nett_amount, payer_name, payer_phone, status, paid_at,
    raw_payload, created_at`

// QRISPaymentFilter is the admin-facing list filter for successful QRIS payments.
type QRISPaymentFilter struct {
	Provider       string
	QRISMerchantID int
	StoreID        string
	CreatedFrom    *time.Time
	CreatedTo      *time.Time
	Search         string
}

func (r *QRISPaymentRepository) List(ctx context.Context, f QRISPaymentFilter, limit, offset int) ([]models.QRISPayment, int, error) {
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
	if f.QRISMerchantID > 0 {
		add("qris_merchant_id = ?", f.QRISMerchantID)
	}
	if f.StoreID != "" {
		add("store_id = ?", f.StoreID)
	}
	if f.CreatedFrom != nil {
		add("created_at >= ?", *f.CreatedFrom)
	}
	if f.CreatedTo != nil {
		add("created_at <= ?", *f.CreatedTo)
	}
	if f.Search != "" {
		pattern := "%" + f.Search + "%"
		where = append(where, fmt.Sprintf("(reference_no ILIKE $%d OR payment_reference_no ILIKE $%d OR rrn ILIKE $%d OR payer_name ILIKE $%d)", idx, idx, idx, idx))
		args = append(args, pattern)
		idx++
	}

	whereClause := strings.Join(where, " AND ")
	countQ := `SELECT COUNT(*) FROM qris_payments WHERE ` + whereClause
	var total int
	if err := r.db.GetContext(ctx, &total, countQ, args...); err != nil {
		return nil, 0, err
	}

	q := `SELECT ` + qrisPaymentColumns + ` FROM qris_payments WHERE ` + whereClause +
		fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", idx, idx+1)
	args = append(args, limit, offset)

	rows := []models.QRISPayment{}
	if err := r.db.SelectContext(ctx, &rows, q, args...); err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}
