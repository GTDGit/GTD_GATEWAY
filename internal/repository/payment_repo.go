package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/GTDGit/gtd_gateway/internal/models"
)

type PaymentRepository struct {
	db *sqlx.DB
}

func NewPaymentRepository(db *sqlx.DB) *PaymentRepository {
	return &PaymentRepository{db: db}
}

func nullablePaymentJSON(v models.NullableRawMessage) any {
	if len(v) == 0 {
		return nil
	}
	return []byte(v)
}

// ----------------------------------------------------------------------------
// Payment CRUD
// ----------------------------------------------------------------------------

const paymentColumns = `id, payment_id, reference_id, client_id, payment_method_id, is_sandbox,
    payment_type, payment_code, provider, amount, fee, total_amount,
    customer_name, customer_email, customer_phone, status,
    payment_detail, payment_instruction, sender_bank, sender_name, sender_account,
    provider_ref, provider_data, callback_type, description, metadata,
    callback_sent, callback_sent_at, callback_attempts, expired_at,
    created_at, paid_at, cancelled_at, updated_at`

func (r *PaymentRepository) CreatePayment(ctx context.Context, p *models.Payment) error {
	const q = `INSERT INTO payments (
        payment_id, reference_id, client_id, payment_method_id, is_sandbox,
        payment_type, payment_code, provider, amount, fee, total_amount,
        customer_name, customer_email, customer_phone, status,
        payment_detail, payment_instruction, sender_bank, sender_name, sender_account,
        provider_ref, provider_data, callback_type, description, metadata,
        callback_sent, callback_sent_at, callback_attempts, expired_at
    ) VALUES (
        $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11,
        $12, $13, $14, $15, $16, $17, $18, $19, $20,
        $21, $22, $23, $24, $25, $26, $27, $28, $29
    ) RETURNING id, created_at, updated_at`

	return r.db.QueryRowContext(ctx, q,
		p.PaymentID, p.ReferenceID, p.ClientID, p.PaymentMethodID, p.IsSandbox,
		p.PaymentType, p.PaymentCode, p.Provider, p.Amount, p.Fee, p.TotalAmount,
		p.CustomerName, p.CustomerEmail, p.CustomerPhone, p.Status,
		nullablePaymentJSON(p.PaymentDetail), nullablePaymentJSON(p.PaymentInstruction),
		p.SenderBank, p.SenderName, p.SenderAccount,
		p.ProviderRef, nullablePaymentJSON(p.ProviderData),
		p.CallbackType, p.Description, nullablePaymentJSON(p.Metadata),
		p.CallbackSent, p.CallbackSentAt, p.CallbackAttempts, p.ExpiredAt,
	).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
}

// UpdatePayment persists a full row update. Callers are expected to re-read if
// they need the generated updated_at reflected in-memory.
func (r *PaymentRepository) UpdatePayment(ctx context.Context, p *models.Payment) error {
	const q = `UPDATE payments SET
        status = $2,
        payment_detail = $3,
        payment_instruction = $4,
        sender_bank = $5,
        sender_name = $6,
        sender_account = $7,
        provider_ref = $8,
        provider_data = $9,
        callback_type = $10,
        description = $11,
        metadata = $12,
        callback_sent = $13,
        callback_sent_at = $14,
        callback_attempts = $15,
        paid_at = $16,
        cancelled_at = $17,
        expired_at = $18,
        updated_at = NOW()
    WHERE id = $1
    RETURNING updated_at`
	return r.db.QueryRowContext(ctx, q,
		p.ID,
		p.Status,
		nullablePaymentJSON(p.PaymentDetail),
		nullablePaymentJSON(p.PaymentInstruction),
		p.SenderBank, p.SenderName, p.SenderAccount,
		p.ProviderRef, nullablePaymentJSON(p.ProviderData),
		p.CallbackType, p.Description, nullablePaymentJSON(p.Metadata),
		p.CallbackSent, p.CallbackSentAt, p.CallbackAttempts,
		p.PaidAt, p.CancelledAt, p.ExpiredAt,
	).Scan(&p.UpdatedAt)
}

func (r *PaymentRepository) selectPayment(ctx context.Context, where string, args ...any) (*models.Payment, error) {
	q := `SELECT ` + paymentColumns + ` FROM payments WHERE ` + where + ` LIMIT 1`
	var p models.Payment
	if err := r.db.GetContext(ctx, &p, q, args...); err != nil {
		if err == sql.ErrNoRows {
			return nil, sql.ErrNoRows
		}
		return nil, err
	}
	return &p, nil
}

func (r *PaymentRepository) GetPaymentByID(ctx context.Context, id int) (*models.Payment, error) {
	return r.selectPayment(ctx, "id = $1", id)
}

func (r *PaymentRepository) GetByPaymentID(ctx context.Context, paymentID string) (*models.Payment, error) {
	return r.selectPayment(ctx, "payment_id = $1", paymentID)
}

func (r *PaymentRepository) GetByReferenceID(ctx context.Context, clientID int, referenceID string) (*models.Payment, error) {
	return r.selectPayment(ctx, "client_id = $1 AND reference_id = $2", clientID, referenceID)
}

func (r *PaymentRepository) GetByProviderRef(ctx context.Context, provider models.PaymentProvider, providerRef string) (*models.Payment, error) {
	return r.selectPayment(ctx, "provider = $1 AND provider_ref = $2", provider, providerRef)
}

// PaymentFilter is the admin-facing list filter.
type PaymentFilter struct {
	Status       string
	Type         string
	Provider     string
	ClientID     int
	PaymentID    string
	ReferenceID  string
	IsSandbox    *bool
	CreatedFrom  *time.Time
	CreatedTo    *time.Time
	Search       string
}

// ListPayments returns payments matching the filter plus the total count.
func (r *PaymentRepository) ListPayments(ctx context.Context, f PaymentFilter, limit, offset int) ([]models.Payment, int, error) {
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
	if f.Type != "" {
		add("payment_type = ?", f.Type)
	}
	if f.Provider != "" {
		add("provider = ?", f.Provider)
	}
	if f.ClientID > 0 {
		add("client_id = ?", f.ClientID)
	}
	if f.PaymentID != "" {
		add("payment_id = ?", f.PaymentID)
	}
	if f.ReferenceID != "" {
		add("reference_id = ?", f.ReferenceID)
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
		where = append(where, fmt.Sprintf("(payment_id ILIKE $%d OR reference_id ILIKE $%d OR provider_ref ILIKE $%d)", idx, idx, idx))
		args = append(args, pattern)
		idx++
	}

	whereClause := strings.Join(where, " AND ")
	countQ := `SELECT COUNT(*) FROM payments WHERE ` + whereClause
	var total int
	if err := r.db.GetContext(ctx, &total, countQ, args...); err != nil {
		return nil, 0, err
	}

	q := `SELECT ` + paymentColumns + ` FROM payments WHERE ` + whereClause +
		fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", idx, idx+1)
	args = append(args, limit, offset)

	rows := []models.Payment{}
	if err := r.db.SelectContext(ctx, &rows, q, args...); err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// GetPendingPaymentsPastStale returns Pending payments whose updated_at is older than staleAfter.
func (r *PaymentRepository) GetPendingPaymentsPastStale(ctx context.Context, staleAfter time.Duration, limit int) ([]models.Payment, error) {
	q := `SELECT ` + paymentColumns + ` FROM payments
    WHERE status = 'Pending' AND updated_at < NOW() - $1::interval
    ORDER BY updated_at ASC LIMIT $2`
	rows := []models.Payment{}
	if err := r.db.SelectContext(ctx, &rows, q, fmt.Sprintf("%d seconds", int(staleAfter.Seconds())), limit); err != nil {
		return nil, err
	}
	return rows, nil
}

// GetExpiredPendingPayments returns Pending payments whose expired_at is in the past.
func (r *PaymentRepository) GetExpiredPendingPayments(ctx context.Context, limit int) ([]models.Payment, error) {
	q := `SELECT ` + paymentColumns + ` FROM payments
    WHERE status = 'Pending' AND expired_at < NOW()
    ORDER BY expired_at ASC LIMIT $1`
	rows := []models.Payment{}
	if err := r.db.SelectContext(ctx, &rows, q, limit); err != nil {
		return nil, err
	}
	return rows, nil
}

// MarkPaymentCallbackSent sets callback_sent=true and timestamps + increments the attempt counter.
func (r *PaymentRepository) MarkPaymentCallbackSent(ctx context.Context, paymentID int, sentAt time.Time) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE payments SET callback_sent = true, callback_sent_at = $2, callback_attempts = callback_attempts + 1, updated_at = NOW() WHERE id = $1`,
		paymentID, sentAt)
	return err
}

// ----------------------------------------------------------------------------
// PaymentMethod access
// ----------------------------------------------------------------------------

const methodColumns = `id, type, code, name, provider, provider_display_name, fee_type, fee_flat, fee_percent,
    fee_min, fee_max, min_amount, max_amount, expired_duration, logo_url,
    display_order, payment_instruction, is_active, is_maintenance,
    maintenance_message, created_at, updated_at`

func (r *PaymentRepository) GetMethodByTypeCode(ctx context.Context, paymentType models.PaymentType, code string) (*models.PaymentMethod, error) {
	q := `SELECT ` + methodColumns + ` FROM payment_methods WHERE type = $1 AND code = $2 LIMIT 1`
	var m models.PaymentMethod
	if err := r.db.GetContext(ctx, &m, q, paymentType, code); err != nil {
		if err == sql.ErrNoRows {
			return nil, sql.ErrNoRows
		}
		return nil, err
	}
	return &m, nil
}

func (r *PaymentRepository) GetMethodByID(ctx context.Context, id int) (*models.PaymentMethod, error) {
	q := `SELECT ` + methodColumns + ` FROM payment_methods WHERE id = $1 LIMIT 1`
	var m models.PaymentMethod
	if err := r.db.GetContext(ctx, &m, q, id); err != nil {
		if err == sql.ErrNoRows {
			return nil, sql.ErrNoRows
		}
		return nil, err
	}
	return &m, nil
}

func (r *PaymentRepository) ListActiveMethods(ctx context.Context) ([]models.PaymentMethod, error) {
	q := `SELECT ` + methodColumns + ` FROM payment_methods WHERE is_active = true ORDER BY display_order ASC, id ASC`
	rows := []models.PaymentMethod{}
	if err := r.db.SelectContext(ctx, &rows, q); err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *PaymentRepository) ListAllMethods(ctx context.Context) ([]models.PaymentMethod, error) {
	q := `SELECT ` + methodColumns + ` FROM payment_methods ORDER BY display_order ASC, id ASC`
	rows := []models.PaymentMethod{}
	if err := r.db.SelectContext(ctx, &rows, q); err != nil {
		return nil, err
	}
	return rows, nil
}

// UpdateMethod persists admin-editable fields of a payment method.
func (r *PaymentRepository) UpdateMethod(ctx context.Context, m *models.PaymentMethod) error {
	q := `UPDATE payment_methods SET
        provider = $2, provider_display_name = $3, fee_type = $4, fee_flat = $5, fee_percent = $6,
        fee_min = $7, fee_max = $8, min_amount = $9, max_amount = $10,
        expired_duration = $11, logo_url = $12, display_order = $13,
        payment_instruction = $14, is_active = $15, is_maintenance = $16,
        maintenance_message = $17, updated_at = NOW()
    WHERE id = $1 RETURNING updated_at`
	return r.db.QueryRowContext(ctx, q,
		m.ID, m.Provider, m.ProviderDisplayName, m.FeeType, m.FeeFlat, m.FeePercent,
		m.FeeMin, m.FeeMax, m.MinAmount, m.MaxAmount,
		m.ExpiredDuration, m.LogoURL, m.DisplayOrder,
		nullablePaymentJSON(m.PaymentInstruction), m.IsActive, m.IsMaintenance,
		m.MaintenanceMessage,
	).Scan(&m.UpdatedAt)
}

// ----------------------------------------------------------------------------
// Logs
// ----------------------------------------------------------------------------

func (r *PaymentRepository) CreatePaymentLog(ctx context.Context, log *models.PaymentLog) error {
	q := `INSERT INTO payment_logs (
        payment_id, action, provider, request, response, is_success,
        error_code, error_message, response_at, response_time_ms
    ) VALUES (
        $1, $2, $3, $4, $5, $6, $7, $8, $9, $10
    ) RETURNING id, created_at`
	return r.db.QueryRowContext(ctx, q,
		log.PaymentID, log.Action, log.Provider,
		nullablePaymentJSON(log.Request), nullablePaymentJSON(log.Response),
		log.IsSuccess, log.ErrorCode, log.ErrorMessage,
		log.ResponseAt, log.ResponseTimeMs,
	).Scan(&log.ID, &log.CreatedAt)
}

func (r *PaymentRepository) ListPaymentLogs(ctx context.Context, paymentID int) ([]models.PaymentLog, error) {
	q := `SELECT id, payment_id, action, provider, request, response, is_success,
        error_code, error_message, created_at, response_at, response_time_ms
    FROM payment_logs WHERE payment_id = $1 ORDER BY created_at ASC`
	rows := []models.PaymentLog{}
	if err := r.db.SelectContext(ctx, &rows, q, paymentID); err != nil {
		return nil, err
	}
	return rows, nil
}

// ----------------------------------------------------------------------------
// Provider webhook audit (pre-existing table)
// ----------------------------------------------------------------------------

func (r *PaymentRepository) CreatePaymentCallback(ctx context.Context, callback *models.PaymentCallback) error {
	const q = `
		INSERT INTO payment_callbacks (
			provider, provider_ref, headers, payload, signature, is_valid_signature,
			payment_id, status, paid_amount, is_processed, processed_at, process_error
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9, $10, $11, $12
		)
		RETURNING id, created_at`

	return r.db.QueryRowContext(
		ctx,
		q,
		callback.Provider,
		callback.ProviderRef,
		nullablePaymentJSON(callback.Headers),
		nullablePaymentJSON(callback.Payload),
		callback.Signature,
		callback.IsValidSignature,
		callback.PaymentID,
		callback.Status,
		callback.PaidAmount,
		callback.IsProcessed,
		callback.ProcessedAt,
		callback.ProcessError,
	).Scan(&callback.ID, &callback.CreatedAt)
}

func (r *PaymentRepository) UpdatePaymentCallbackProcessed(ctx context.Context, callbackID int, isProcessed bool, processError *string) error {
	const q = `
		UPDATE payment_callbacks
		SET is_processed = $2,
		    processed_at = CASE WHEN $2 THEN NOW() ELSE NULL END,
		    process_error = $3
		WHERE id = $1`

	_, err := r.db.ExecContext(ctx, q, callbackID, isProcessed, processError)
	return err
}

func (r *PaymentRepository) UpdatePaymentCallbackSignature(ctx context.Context, callbackID int, isValidSignature bool) error {
	const q = `
		UPDATE payment_callbacks
		SET is_valid_signature = $2
		WHERE id = $1`

	_, err := r.db.ExecContext(ctx, q, callbackID, isValidSignature)
	return err
}

func (r *PaymentRepository) ListPaymentCallbacksByProviderRef(ctx context.Context, provider models.PaymentProvider, providerRef string) ([]models.PaymentCallback, error) {
	q := `SELECT id, provider, provider_ref, headers, payload, signature, is_valid_signature,
        payment_id, status, paid_amount, is_processed, processed_at, process_error, created_at
    FROM payment_callbacks WHERE provider = $1 AND provider_ref = $2 ORDER BY created_at DESC`
	rows := []models.PaymentCallback{}
	if err := r.db.SelectContext(ctx, &rows, q, provider, providerRef); err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *PaymentRepository) CountProcessedCallbacksByRef(ctx context.Context, provider models.PaymentProvider, providerRef, status string) (int, error) {
	q := `SELECT COUNT(*) FROM payment_callbacks
    WHERE provider = $1 AND provider_ref = $2 AND is_processed = true AND status = $3`
	var n int
	err := r.db.GetContext(ctx, &n, q, provider, providerRef, status)
	return n, err
}

// ----------------------------------------------------------------------------
// Outbound client webhook audit (payment_callback_logs)
// ----------------------------------------------------------------------------

func (r *PaymentRepository) CreatePaymentCallbackLog(ctx context.Context, log *models.PaymentCallbackLog) error {
	q := `INSERT INTO payment_callback_logs (
        payment_id, client_id, event, payload, attempt, max_attempts,
        http_status, response_body, response_time_ms, is_delivered, error_message,
        next_retry_at, delivered_at
    ) VALUES (
        $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13
    ) RETURNING id, created_at, updated_at`
	return r.db.QueryRowContext(ctx, q,
		log.PaymentID, log.ClientID, log.Event, nullablePaymentJSON(log.Payload),
		log.Attempt, log.MaxAttempts, log.HTTPStatus, log.ResponseBody,
		log.ResponseTimeMs, log.IsDelivered, log.ErrorMessage,
		log.NextRetryAt, log.DeliveredAt,
	).Scan(&log.ID, &log.CreatedAt, &log.UpdatedAt)
}

func (r *PaymentRepository) UpdatePaymentCallbackLog(ctx context.Context, log *models.PaymentCallbackLog) error {
	q := `UPDATE payment_callback_logs SET
        attempt = $2, http_status = $3, response_body = $4, response_time_ms = $5,
        is_delivered = $6, error_message = $7, next_retry_at = $8, delivered_at = $9,
        updated_at = NOW()
    WHERE id = $1 RETURNING updated_at`
	return r.db.QueryRowContext(ctx, q,
		log.ID, log.Attempt, log.HTTPStatus, log.ResponseBody, log.ResponseTimeMs,
		log.IsDelivered, log.ErrorMessage, log.NextRetryAt, log.DeliveredAt,
	).Scan(&log.UpdatedAt)
}

func (r *PaymentRepository) GetPaymentCallbackLogByID(ctx context.Context, id int) (*models.PaymentCallbackLog, error) {
	q := `SELECT id, payment_id, client_id, event, payload, attempt, max_attempts,
        http_status, response_body, response_time_ms, is_delivered, error_message,
        next_retry_at, delivered_at, created_at, updated_at
    FROM payment_callback_logs WHERE id = $1 LIMIT 1`
	var log models.PaymentCallbackLog
	if err := r.db.GetContext(ctx, &log, q, id); err != nil {
		if err == sql.ErrNoRows {
			return nil, sql.ErrNoRows
		}
		return nil, err
	}
	return &log, nil
}

func (r *PaymentRepository) ListPaymentCallbackLogs(ctx context.Context, paymentID int) ([]models.PaymentCallbackLog, error) {
	q := `SELECT id, payment_id, client_id, event, payload, attempt, max_attempts,
        http_status, response_body, response_time_ms, is_delivered, error_message,
        next_retry_at, delivered_at, created_at, updated_at
    FROM payment_callback_logs WHERE payment_id = $1 ORDER BY created_at ASC`
	rows := []models.PaymentCallbackLog{}
	if err := r.db.SelectContext(ctx, &rows, q, paymentID); err != nil {
		return nil, err
	}
	return rows, nil
}

// GetPendingPaymentCallbackLogs returns logs that are ready for retry.
func (r *PaymentRepository) GetPendingPaymentCallbackLogs(ctx context.Context, limit int) ([]models.PaymentCallbackLog, error) {
	q := `SELECT id, payment_id, client_id, event, payload, attempt, max_attempts,
        http_status, response_body, response_time_ms, is_delivered, error_message,
        next_retry_at, delivered_at, created_at, updated_at
    FROM payment_callback_logs
    WHERE is_delivered = false AND attempt < max_attempts AND (next_retry_at IS NULL OR next_retry_at <= NOW())
    ORDER BY next_retry_at NULLS FIRST, created_at ASC
    LIMIT $1`
	rows := []models.PaymentCallbackLog{}
	if err := r.db.SelectContext(ctx, &rows, q, limit); err != nil {
		return nil, err
	}
	return rows, nil
}

// PaymentStats returns aggregated counts for the admin dashboard.
type PaymentStats struct {
	Total        int   `db:"total" json:"total"`
	TotalPaid    int   `db:"total_paid" json:"totalPaid"`
	TotalPending int   `db:"total_pending" json:"totalPending"`
	TotalExpired int   `db:"total_expired" json:"totalExpired"`
	TotalFailed  int   `db:"total_failed" json:"totalFailed"`
	TotalVolume  int64 `db:"total_volume" json:"totalVolume"`
}

func (r *PaymentRepository) Stats(ctx context.Context, f PaymentFilter) (*PaymentStats, error) {
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
        COUNT(*) FILTER (WHERE status = 'Paid') AS total_paid,
        COUNT(*) FILTER (WHERE status = 'Pending') AS total_pending,
        COUNT(*) FILTER (WHERE status = 'Expired') AS total_expired,
        COUNT(*) FILTER (WHERE status = 'Failed') AS total_failed,
        COALESCE(SUM(total_amount) FILTER (WHERE status = 'Paid'), 0) AS total_volume
    FROM payments WHERE ` + strings.Join(where, " AND ")
	var s PaymentStats
	if err := r.db.GetContext(ctx, &s, q, args...); err != nil {
		return nil, err
	}
	return &s, nil
}
