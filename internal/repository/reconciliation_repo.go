package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"

	"github.com/GTDGit/gtd_gateway/internal/models"
)

type ReconciliationRepository struct {
	db *sqlx.DB
}

func NewReconciliationRepository(db *sqlx.DB) *ReconciliationRepository {
	return &ReconciliationRepository{db: db}
}

const reconciliationColumns = `id, payment_id, provider, reason, webhook_status, inquiry_status,
    webhook_amount, inquiry_amount, expected_amount, webhook_payload, inquiry_payload,
    status, resolved_status, resolved_by, resolution_note, created_at, updated_at, resolved_at`

// ReconciliationFilter is the admin-facing list filter.
type ReconciliationFilter struct {
	Status   string
	Provider string
	Reason   string
	Search   string // matches payment_id
}

// List returns reconciliations matching the filter plus the total count.
func (r *ReconciliationRepository) List(ctx context.Context, f ReconciliationFilter, limit, offset int) ([]models.PaymentReconciliation, int, error) {
	where := []string{"1=1"}
	args := []any{}
	idx := 1
	add := func(clause string, val any) {
		where = append(where, strings.ReplaceAll(clause, "?", fmt.Sprintf("$%d", idx)))
		args = append(args, val)
		idx++
	}
	if f.Status != "" {
		add("status = ?", f.Status)
	}
	if f.Provider != "" {
		add("provider = ?", f.Provider)
	}
	if f.Reason != "" {
		add("reason = ?", f.Reason)
	}
	if f.Search != "" {
		add("payment_id ILIKE ?", "%"+f.Search+"%")
	}

	whereClause := strings.Join(where, " AND ")
	countQ := `SELECT COUNT(*) FROM payment_reconciliations WHERE ` + whereClause
	var total int
	if err := r.db.GetContext(ctx, &total, countQ, args...); err != nil {
		return nil, 0, err
	}

	q := `SELECT ` + reconciliationColumns + ` FROM payment_reconciliations WHERE ` + whereClause +
		fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", idx, idx+1)
	args = append(args, limit, offset)

	rows := []models.PaymentReconciliation{}
	if err := r.db.SelectContext(ctx, &rows, q, args...); err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// GetByID returns a single reconciliation, or sql.ErrNoRows when absent.
func (r *ReconciliationRepository) GetByID(ctx context.Context, id int64) (*models.PaymentReconciliation, error) {
	q := `SELECT ` + reconciliationColumns + ` FROM payment_reconciliations WHERE id = $1 LIMIT 1`
	var rec models.PaymentReconciliation
	if err := r.db.GetContext(ctx, &rec, q, id); err != nil {
		if err == sql.ErrNoRows {
			return nil, sql.ErrNoRows
		}
		return nil, err
	}
	return &rec, nil
}

// ResolveByID marks an open reconciliation resolved. Returns sql.ErrNoRows when
// the row does not exist or was already resolved.
func (r *ReconciliationRepository) ResolveByID(ctx context.Context, id int64, resolvedStatus, resolvedBy, note string) error {
	const q = `UPDATE payment_reconciliations SET
        status = 'resolved',
        resolved_status = $2,
        resolved_by = $3,
        resolution_note = NULLIF($4, ''),
        resolved_at = NOW(),
        updated_at = NOW()
    WHERE id = $1 AND status = 'open'`
	res, err := r.db.ExecContext(ctx, q, id, resolvedStatus, resolvedBy, note)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
