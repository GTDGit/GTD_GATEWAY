package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/GTDGit/gtd_gateway/internal/models"
)

// PayoutFilter is the admin-facing list filter for payouts.
type PayoutFilter struct {
	Status      string
	MethodType  string
	Provider    string
	ChannelCode string
	ClientID    int
	IsSandbox   *bool
	CreatedFrom *time.Time
	CreatedTo   *time.Time
	Search      string
}

// PayoutStats summarizes payout aggregates for admin dashboards.
type PayoutStats struct {
	Total           int   `db:"total" json:"total"`
	TotalSuccess    int   `db:"total_success" json:"totalSuccess"`
	TotalProcessing int   `db:"total_processing" json:"totalProcessing"`
	TotalPending    int   `db:"total_pending" json:"totalPending"`
	TotalFailed     int   `db:"total_failed" json:"totalFailed"`
	TotalVolume     int64 `db:"total_volume" json:"totalVolume"`
}

type PayoutRepository struct {
	db *sqlx.DB
}

func NewPayoutRepository(db *sqlx.DB) *PayoutRepository {
	return &PayoutRepository{db: db}
}

// ---------------------------------------------------------------------------
// Routes (per method_type provider routing)
// ---------------------------------------------------------------------------

// ListRoutes returns all routes ordered by method_type, priority.
func (r *PayoutRepository) ListRoutes(ctx context.Context) ([]models.PayoutRoute, error) {
	const q = `
		SELECT id, method_type, provider, priority, is_active, is_maintenance,
		       maintenance_message, created_at, updated_at
		FROM payout_routes
		ORDER BY method_type, priority ASC`
	rows := []models.PayoutRoute{}
	if err := r.db.SelectContext(ctx, &rows, q); err != nil {
		return nil, err
	}
	return rows, nil
}

// GetRouteByID returns a single route by primary key.
func (r *PayoutRepository) GetRouteByID(ctx context.Context, id int) (*models.PayoutRoute, error) {
	const q = `
		SELECT id, method_type, provider, priority, is_active, is_maintenance,
		       maintenance_message, created_at, updated_at
		FROM payout_routes
		WHERE id = $1
		LIMIT 1`
	var route models.PayoutRoute
	if err := r.db.GetContext(ctx, &route, q, id); err != nil {
		return nil, err
	}
	return &route, nil
}

// UpdateRoute updates the mutable fields of a payout route.
func (r *PayoutRepository) UpdateRoute(ctx context.Context, route *models.PayoutRoute) error {
	const q = `
		UPDATE payout_routes
		SET priority = $2,
		    is_active = $3,
		    is_maintenance = $4,
		    maintenance_message = $5,
		    updated_at = NOW()
		WHERE id = $1
		RETURNING updated_at`
	return r.db.QueryRowContext(
		ctx, q,
		route.ID,
		route.Priority,
		route.IsActive,
		route.IsMaintenance,
		route.MaintenanceMessage,
	).Scan(&route.UpdatedAt)
}

// ---------------------------------------------------------------------------
// Payouts
// ---------------------------------------------------------------------------

const payoutColumns = `id, payout_id, reference_id, client_id, is_sandbox, method_type, channel_code,
		transfer_type, provider, bank_code, bank_name, account_number, account_name,
		source_bank_code, source_account_number, amount, fee, send_amount, total_amount, fee_paid_by,
		status, failed_reason, failed_code, purpose_code, remark, description,
		customer_name, customer_email, customer_phone,
		inquiry_id, provider_ref, provider_data, callback_url, callback_sent, callback_sent_at,
		callback_attempts, created_at, completed_at, failed_at, updated_at`

// GetPayoutByID returns a single payout by primary key.
func (r *PayoutRepository) GetPayoutByID(ctx context.Context, id int) (*models.Payout, error) {
	q := `SELECT ` + payoutColumns + ` FROM payouts WHERE id = $1 LIMIT 1`
	var p models.Payout
	if err := r.db.GetContext(ctx, &p, q, id); err != nil {
		return nil, err
	}
	return &p, nil
}

// ListPayouts returns payouts matching the filter plus the total count.
func (r *PayoutRepository) ListPayouts(ctx context.Context, f PayoutFilter, limit, offset int) ([]models.Payout, int, error) {
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
	if f.MethodType != "" {
		add("method_type = ?", f.MethodType)
	}
	if f.Provider != "" {
		add("provider = ?", f.Provider)
	}
	if f.ChannelCode != "" {
		add("channel_code = ?", f.ChannelCode)
	}
	if f.ClientID > 0 {
		add("client_id = ?", f.ClientID)
	}
	if f.IsSandbox != nil {
		add("is_sandbox = ?", *f.IsSandbox)
	}
	if f.CreatedFrom != nil {
		add("created_at >= ?", *f.CreatedFrom)
	}
	if f.CreatedTo != nil {
		add("created_at <= ?", *f.CreatedTo)
	}
	if f.Search != "" {
		pattern := "%" + f.Search + "%"
		where = append(where, fmt.Sprintf("(payout_id ILIKE $%d OR reference_id ILIKE $%d OR account_number ILIKE $%d OR provider_ref ILIKE $%d)", idx, idx, idx, idx))
		args = append(args, pattern)
		idx++
	}

	whereClause := strings.Join(where, " AND ")
	countQ := `SELECT COUNT(*) FROM payouts WHERE ` + whereClause
	var total int
	if err := r.db.GetContext(ctx, &total, countQ, args...); err != nil {
		return nil, 0, err
	}

	q := `SELECT ` + payoutColumns + ` FROM payouts WHERE ` + whereClause +
		fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", idx, idx+1)
	args = append(args, limit, offset)

	rows := []models.Payout{}
	if err := r.db.SelectContext(ctx, &rows, q, args...); err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// Stats returns aggregate counts and volume for the given filter.
func (r *PayoutRepository) Stats(ctx context.Context, f PayoutFilter) (*PayoutStats, error) {
	where := []string{"1=1"}
	args := []any{}
	idx := 1
	add := func(clause string, val any) {
		where = append(where, strings.ReplaceAll(clause, "?", fmt.Sprintf("$%d", idx)))
		args = append(args, val)
		idx++
	}
	if f.ClientID > 0 {
		add("client_id = ?", f.ClientID)
	}
	if f.IsSandbox != nil {
		add("is_sandbox = ?", *f.IsSandbox)
	}
	if f.CreatedFrom != nil {
		add("created_at >= ?", *f.CreatedFrom)
	}
	if f.CreatedTo != nil {
		add("created_at <= ?", *f.CreatedTo)
	}
	q := `SELECT
        COUNT(*) AS total,
        COUNT(*) FILTER (WHERE status = 'Success') AS total_success,
        COUNT(*) FILTER (WHERE status = 'Processing') AS total_processing,
        COUNT(*) FILTER (WHERE status = 'Pending') AS total_pending,
        COUNT(*) FILTER (WHERE status = 'Failed') AS total_failed,
        COALESCE(SUM(total_amount) FILTER (WHERE status = 'Success'), 0) AS total_volume
    FROM payouts WHERE ` + strings.Join(where, " AND ")
	var s PayoutStats
	if err := r.db.GetContext(ctx, &s, q, args...); err != nil {
		return nil, err
	}
	return &s, nil
}

// ---------------------------------------------------------------------------
// Callbacks
// ---------------------------------------------------------------------------

// ListCallbacksByPayoutID returns provider callbacks for the given payout reference.
func (r *PayoutRepository) ListCallbacksByPayoutID(ctx context.Context, payoutID string) ([]models.PayoutCallback, error) {
	const q = `
		SELECT id, provider, provider_ref, headers, payload, signature, is_valid_signature,
		       payout_id, status, is_processed, processed_at, process_error, created_at
		FROM payout_callbacks
		WHERE payout_id = $1
		ORDER BY created_at DESC`
	rows := []models.PayoutCallback{}
	if err := r.db.SelectContext(ctx, &rows, q, payoutID); err != nil {
		return nil, err
	}
	return rows, nil
}

// ---------------------------------------------------------------------------
// Payout methods catalog (per-channel name, fee, amount limits)
// ---------------------------------------------------------------------------

const payoutMethodColumns = `id, method_type, code, name, fee_type, fee_flat, fee_percent,
		fee_min, fee_max, min_amount, max_amount, logo_url, display_order,
		is_active, is_maintenance, maintenance_message, created_at, updated_at`

// ListMethods returns every payout_methods row ordered for display (admin view).
func (r *PayoutRepository) ListMethods(ctx context.Context) ([]models.PayoutMethodCatalog, error) {
	q := `SELECT ` + payoutMethodColumns + ` FROM payout_methods
		ORDER BY method_type, display_order ASC, id ASC`
	rows := []models.PayoutMethodCatalog{}
	if err := r.db.SelectContext(ctx, &rows, q); err != nil {
		return nil, err
	}
	return rows, nil
}

// GetMethodByID returns a single payout_methods row by primary key.
func (r *PayoutRepository) GetMethodByID(ctx context.Context, id int) (*models.PayoutMethodCatalog, error) {
	q := `SELECT ` + payoutMethodColumns + ` FROM payout_methods WHERE id = $1 LIMIT 1`
	var m models.PayoutMethodCatalog
	if err := r.db.GetContext(ctx, &m, q, id); err != nil {
		return nil, err
	}
	return &m, nil
}

// UpdateMethod mutates the editable fields of a payout_methods row.
func (r *PayoutRepository) UpdateMethod(ctx context.Context, m *models.PayoutMethodCatalog) error {
	const q = `
		UPDATE payout_methods
		SET name = $2,
		    fee_type = $3,
		    fee_flat = $4,
		    fee_percent = $5,
		    fee_min = $6,
		    fee_max = $7,
		    min_amount = $8,
		    max_amount = $9,
		    logo_url = $10,
		    display_order = $11,
		    is_active = $12,
		    is_maintenance = $13,
		    maintenance_message = $14,
		    updated_at = NOW()
		WHERE id = $1
		RETURNING updated_at`
	return r.db.QueryRowContext(ctx, q,
		m.ID, m.Name, m.FeeType, m.FeeFlat, m.FeePercent, m.FeeMin, m.FeeMax,
		m.MinAmount, m.MaxAmount, m.LogoURL, m.DisplayOrder,
		m.IsActive, m.IsMaintenance, m.MaintenanceMessage,
	).Scan(&m.UpdatedAt)
}
