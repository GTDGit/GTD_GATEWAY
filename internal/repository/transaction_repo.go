package repository

import (
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/GTDGit/gtd_gateway/internal/models"
)

// TransactionRepository handles data access for transactions.
type TransactionRepository struct {
	db *sqlx.DB
}

// NewTransactionRepository creates a new TransactionRepository.
func NewTransactionRepository(db *sqlx.DB) *TransactionRepository {
	return &TransactionRepository{db: db}
}

const transactionSelectWithProvider = `
	SELECT t.*, pp.code AS provider_code
	FROM transactions t
	LEFT JOIN ppob_providers pp ON t.provider_id = pp.id`

// nullableJSON converts empty NullableRawMessage to nil for proper NULL handling in PostgreSQL.
func nullableJSON(v models.NullableRawMessage) interface{} {
	if len(v) == 0 {
		return nil
	}
	return []byte(v)
}

// Create inserts a new transaction row.
func (r *TransactionRepository) Create(trx *models.Transaction) error {
	const q = `
        INSERT INTO transactions (
            transaction_id, reference_id, client_id, product_id, sku_id, is_sandbox,
            customer_no, customer_name, type, status, serial_number, amount, admin,
            period, description, failed_reason, retry_count, next_retry_at, expired_at,
            inquiry_id, digi_ref_id, buy_price, sell_price,
            provider_id, provider_sku_id, provider_ref_id, provider_initial_response,
            provider_response, provider_initial_http_status, provider_http_status,
            created_at, processed_at
        ) VALUES (
            $1,$2,$3,$4,$5,$6,
            $7,$8,$9,$10,$11,$12,$13,
            $14,$15,$16,$17,$18,$19,
            $20,$21,$22,$23,
            $24,$25,$26,$27,
            $28,$29,$30,
            NOW(),$31
        ) RETURNING id, created_at, updated_at`

	return r.db.QueryRow(q,
		trx.TransactionID, trx.ReferenceID, trx.ClientID, trx.ProductID, trx.SkuID, trx.IsSandbox,
		trx.CustomerNo, trx.CustomerName, trx.Type, trx.Status, trx.SerialNumber, trx.Amount, trx.Admin,
		trx.Period, nullableJSON(trx.Description), trx.FailedReason, trx.RetryCount, trx.NextRetryAt, trx.ExpiredAt,
		trx.InquiryID, trx.DigiRefID, trx.BuyPrice, trx.SellPrice,
		trx.ProviderID, trx.ProviderSKUID, trx.ProviderRefID, nullableJSON(trx.ProviderInitialResponse),
		nullableJSON(trx.ProviderResponse), trx.ProviderInitialHTTPStatus, trx.ProviderHTTPStatus, trx.ProcessedAt,
	).Scan(&trx.ID, &trx.CreatedAt, &trx.UpdatedAt)
}

// Update updates an existing transaction identified by transaction_id.
func (r *TransactionRepository) Update(trx *models.Transaction) error {
	const q = `
        UPDATE transactions SET
            sku_id = $2,
            status = $3,
            serial_number = $4,
            amount = $5,
            admin = $6,
            period = $7,
            description = $8,
            failed_reason = $9,
            failed_code = $10,
            retry_count = $11,
            next_retry_at = $12,
            expired_at = $13,
            inquiry_id = $14,
            digi_ref_id = $15,
            callback_sent = $16,
            callback_sent_at = $17,
            processed_at = $18,
            buy_price = $19,
            sell_price = $20,
            provider_id = $21,
            provider_sku_id = $22,
            provider_ref_id = $23,
            provider_initial_response = $24,
            provider_response = $25,
            provider_initial_http_status = $26,
            provider_http_status = $27,
            updated_at = NOW()
        WHERE transaction_id = $1`

	stmt, err := r.db.Preparex(q)
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.Exec(
		trx.TransactionID,
		trx.SkuID,
		trx.Status,
		trx.SerialNumber,
		trx.Amount,
		trx.Admin,
		trx.Period,
		nullableJSON(trx.Description),
		trx.FailedReason,
		trx.FailedCode,
		trx.RetryCount,
		trx.NextRetryAt,
		trx.ExpiredAt,
		trx.InquiryID,
		trx.DigiRefID,
		trx.CallbackSent,
		trx.CallbackAt,
		trx.ProcessedAt,
		trx.BuyPrice,
		trx.SellPrice,
		trx.ProviderID,
		trx.ProviderSKUID,
		trx.ProviderRefID,
		nullableJSON(trx.ProviderInitialResponse),
		nullableJSON(trx.ProviderResponse),
		trx.ProviderInitialHTTPStatus,
		trx.ProviderHTTPStatus,
	)
	return err
}

// GetByTransactionID returns transaction by transaction_id.
func (r *TransactionRepository) GetByTransactionID(transactionID string) (*models.Transaction, error) {
	const q = transactionSelectWithProvider + ` WHERE t.transaction_id = $1 LIMIT 1`
	stmt, err := r.db.Preparex(q)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()
	var t models.Transaction
	if err := stmt.Get(&t, transactionID); err != nil {
		if err == sql.ErrNoRows {
			return nil, sql.ErrNoRows
		}
		return nil, err
	}
	return &t, nil
}

// GetByProviderRefID returns transaction by provider_ref_id.
func (r *TransactionRepository) GetByProviderRefID(providerRefID string) (*models.Transaction, error) {
	const q = transactionSelectWithProvider + ` WHERE t.provider_ref_id = $1 LIMIT 1`
	stmt, err := r.db.Preparex(q)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()
	var t models.Transaction
	if err := stmt.Get(&t, providerRefID); err != nil {
		if err == sql.ErrNoRows {
			return nil, sql.ErrNoRows
		}
		return nil, err
	}
	return &t, nil
}

// GetByReferenceID returns transaction by client_id and reference_id.
func (r *TransactionRepository) GetByReferenceID(clientID int, referenceID string) (*models.Transaction, error) {
	const q = transactionSelectWithProvider + ` WHERE t.client_id = $1 AND t.reference_id = $2 LIMIT 1`
	stmt, err := r.db.Preparex(q)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()
	var t models.Transaction
	if err := stmt.Get(&t, clientID, referenceID); err != nil {
		if err == sql.ErrNoRows {
			return nil, sql.ErrNoRows
		}
		return nil, err
	}
	return &t, nil
}

// GetByDigiRefID returns transaction by digi_ref_id.
func (r *TransactionRepository) GetByDigiRefID(digiRefID string) (*models.Transaction, error) {
	const q = transactionSelectWithProvider + ` WHERE t.digi_ref_id = $1 LIMIT 1`
	stmt, err := r.db.Preparex(q)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()
	var t models.Transaction
	if err := stmt.Get(&t, digiRefID); err != nil {
		if err == sql.ErrNoRows {
			return nil, sql.ErrNoRows
		}
		return nil, err
	}
	return &t, nil
}

// GetAllPendingTransactions returns all transactions in Pending status.
// With the new logic, transactions should not stay in Pending state,
// so this is used for cleanup purposes.
func (r *TransactionRepository) GetAllPendingTransactions() ([]models.Transaction, error) {
	const q = `
        SELECT * FROM transactions
        WHERE status = 'Pending'
        ORDER BY created_at ASC
        FOR UPDATE SKIP LOCKED`

	stmt, err := r.db.Preparex(q)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()
	var list []models.Transaction
	if err := stmt.Select(&list); err != nil {
		return nil, err
	}
	return list, nil
}

// GetStaleProcessingTransactions returns Processing transactions older than the given duration.
// Finds both legacy Digiflazz transactions and multi-provider transactions.
// Used to re-check status by calling the appropriate provider.
func (r *TransactionRepository) GetStaleProcessingTransactions(staleAfter time.Duration) ([]models.Transaction, error) {
	const q = `
        SELECT t.*, pp.code AS provider_code
        FROM transactions t
        LEFT JOIN ppob_providers pp ON t.provider_id = pp.id
        WHERE t.status = 'Processing'
          AND t.created_at < NOW() - $1::interval
          AND (
            (t.type = 'prepaid' AND t.digi_ref_id IS NOT NULL)
            OR (t.provider_id IS NOT NULL AND t.provider_ref_id IS NOT NULL)
          )
        ORDER BY t.created_at ASC
        LIMIT 50
        FOR UPDATE OF t SKIP LOCKED`

	stmt, err := r.db.Preparex(q)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	// Convert duration to PostgreSQL interval string (e.g., "10 seconds")
	intervalStr := fmt.Sprintf("%d seconds", int(staleAfter.Seconds()))

	var list []models.Transaction
	if err := stmt.Select(&list, intervalStr); err != nil {
		return nil, err
	}
	return list, nil
}

// ExistsReferenceID checks if a client has already used a reference_id.
func (r *TransactionRepository) ExistsReferenceID(clientID int, referenceID string) (bool, error) {
	const q = `SELECT EXISTS(SELECT 1 FROM transactions WHERE client_id = $1 AND reference_id = $2)`
	stmt, err := r.db.Preparex(q)
	if err != nil {
		return false, err
	}
	defer stmt.Close()
	var exists bool
	if err := stmt.Get(&exists, clientID, referenceID); err != nil {
		return false, err
	}
	return exists, nil
}

// GenerateTransactionID returns an ID like GRB-YYYYMMDD-NNNNNN using Asia/Jakarta date.
func (r *TransactionRepository) GenerateTransactionID() (string, error) {
	// Get date string in Asia/Jakarta from DB to avoid TZ mismatches.
	const dateQ = `SELECT TO_CHAR(NOW() AT TIME ZONE 'Asia/Jakarta', 'YYYYMMDD')`
	var ymd string
	if err := r.db.Get(&ymd, dateQ); err != nil {
		return "", err
	}

	const existsQ = `SELECT EXISTS(SELECT 1 FROM transactions WHERE transaction_id = $1)`
	for attempt := 0; attempt < 10; attempt++ {
		n, err := rand.Int(rand.Reader, big.NewInt(1000000))
		if err != nil {
			return "", err
		}

		candidate := fmt.Sprintf("GRB-%s-%06d", ymd, n.Int64())
		var exists bool
		if err := r.db.Get(&exists, existsQ, candidate); err != nil {
			return "", err
		}
		if !exists {
			return candidate, nil
		}
	}

	return "", errors.New("failed to generate unique transaction id")
}

// GetInquiryForPayment returns the inquiry transaction eligible for linking to a payment.
// Validations: type='inquiry', status='Success', expired_at > NOW(), and not already used by a payment.
func (r *TransactionRepository) GetInquiryForPayment(transactionID string) (*models.Transaction, error) {
	const q = transactionSelectWithProvider + `
        WHERE t.transaction_id = $1
          AND t.type = 'inquiry'
          AND t.status = 'Success'
          AND t.expired_at > NOW()
          AND NOT EXISTS (
              SELECT 1 FROM transactions p WHERE p.inquiry_id = t.id
          )
        LIMIT 1`

	stmt, err := r.db.Preparex(q)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()
	var inq models.Transaction
	if err := stmt.Get(&inq, transactionID); err != nil {
		if err == sql.ErrNoRows {
			return nil, sql.ErrNoRows
		}
		return nil, err
	}
	return &inq, nil
}

// MarkInquiryAsPaid links an inquiry to a payment by setting payment.inquiry_id = inquiryID.
func (r *TransactionRepository) MarkInquiryAsPaid(inquiryID int, paymentID int) error {
	const q = `UPDATE transactions SET inquiry_id = $1, updated_at = NOW() WHERE id = $2`
	stmt, err := r.db.Preparex(q)
	if err != nil {
		return err
	}
	defer stmt.Close()
	_, err = stmt.Exec(inquiryID, paymentID)
	return err
}

// AdminTransactionFilter holds filters for admin transaction queries.
type AdminTransactionFilter struct {
	ClientID      *int
	Status        *string
	Type          *string
	SkuCode       *string
	CustomerNo    *string
	ReferenceID   *string
	TransactionID *string
	StartDate     *string
	EndDate       *string
	IsSandbox     *bool
	Page          int
	Limit         int
}

// AdminTransactionResult contains paginated transaction results.
type AdminTransactionResult struct {
	Transactions []models.Transaction
	TotalItems   int
	TotalPages   int
	Page         int
	Limit        int
}

// GetAllAdmin returns transactions for admin with filters and pagination.
func (r *TransactionRepository) GetAllAdmin(filter *AdminTransactionFilter) (*AdminTransactionResult, error) {
	// Build base query with JOINs to get product sku_code and digi_sku_code
	baseQ := `FROM transactions t
              JOIN products p ON t.product_id = p.id
              LEFT JOIN skus s ON t.sku_id = s.id
              LEFT JOIN clients c ON t.client_id = c.id
              LEFT JOIN ppob_providers pp ON t.provider_id = pp.id
              WHERE 1=1`

	args := []interface{}{}
	argIdx := 1

	// Add filters
	if filter.ClientID != nil {
		baseQ += fmt.Sprintf(" AND t.client_id = $%d", argIdx)
		args = append(args, *filter.ClientID)
		argIdx++
	}
	if filter.Status != nil && *filter.Status != "" {
		baseQ += fmt.Sprintf(" AND t.status = $%d", argIdx)
		args = append(args, *filter.Status)
		argIdx++
	}
	if filter.Type != nil && *filter.Type != "" {
		baseQ += fmt.Sprintf(" AND t.type = $%d", argIdx)
		args = append(args, *filter.Type)
		argIdx++
	}
	if filter.SkuCode != nil && *filter.SkuCode != "" {
		baseQ += fmt.Sprintf(" AND p.sku_code = $%d", argIdx)
		args = append(args, *filter.SkuCode)
		argIdx++
	}
	if filter.CustomerNo != nil && *filter.CustomerNo != "" {
		baseQ += fmt.Sprintf(" AND t.customer_no ILIKE $%d", argIdx)
		args = append(args, "%"+*filter.CustomerNo+"%")
		argIdx++
	}
	if filter.ReferenceID != nil && *filter.ReferenceID != "" {
		baseQ += fmt.Sprintf(" AND t.reference_id ILIKE $%d", argIdx)
		args = append(args, "%"+*filter.ReferenceID+"%")
		argIdx++
	}
	if filter.TransactionID != nil && *filter.TransactionID != "" {
		baseQ += fmt.Sprintf(" AND t.transaction_id ILIKE $%d", argIdx)
		args = append(args, "%"+*filter.TransactionID+"%")
		argIdx++
	}
	if filter.StartDate != nil && *filter.StartDate != "" {
		baseQ += fmt.Sprintf(" AND t.created_at >= $%d::date", argIdx)
		args = append(args, *filter.StartDate)
		argIdx++
	}
	if filter.EndDate != nil && *filter.EndDate != "" {
		baseQ += fmt.Sprintf(" AND t.created_at < ($%d::date + interval '1 day')", argIdx)
		args = append(args, *filter.EndDate)
		argIdx++
	}
	if filter.IsSandbox != nil {
		baseQ += fmt.Sprintf(" AND t.is_sandbox = $%d", argIdx)
		args = append(args, *filter.IsSandbox)
		argIdx++
	}

	// Count total
	countQ := "SELECT COUNT(*) " + baseQ
	var total int
	if err := r.db.Get(&total, countQ, args...); err != nil {
		return nil, err
	}

	// Calculate pagination
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.Limit < 1 {
		filter.Limit = 50
	}
	if filter.Limit > 100 {
		filter.Limit = 100
	}
	offset := (filter.Page - 1) * filter.Limit
	totalPages := (total + filter.Limit - 1) / filter.Limit

	// Select with pagination - include product sku_code, digi_sku_code, and provider info
	selectQ := fmt.Sprintf(`
		SELECT
			t.id, t.transaction_id, t.reference_id, t.client_id, t.product_id, t.sku_id,
			t.is_sandbox, t.customer_no, t.customer_name, t.type, t.status,
			t.serial_number, t.amount, t.admin, t.period, t.description,
			t.failed_reason, t.failed_code, t.retry_count, t.max_retry,
			t.next_retry_at, t.expired_at, t.inquiry_id, t.digi_ref_id,
			t.buy_price, t.sell_price,
			t.provider_id, t.provider_ref_id,
			pp.code AS provider_code,
			t.callback_sent, t.callback_sent_at, t.created_at, t.processed_at, t.updated_at,
			p.sku_code AS product_sku_code,
			s.digi_sku_code AS digi_sku_code
		%s
		ORDER BY t.created_at DESC LIMIT $%d OFFSET $%d`, baseQ, argIdx, argIdx+1)
	args = append(args, filter.Limit, offset)

	rows, err := r.db.Queryx(selectQ, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var transactions []models.Transaction
	for rows.Next() {
		var trx transactionWithJoins
		if err := rows.StructScan(&trx); err != nil {
			return nil, err
		}
		// Map to Transaction model
		t := trx.toTransaction()
		transactions = append(transactions, t)
	}

	return &AdminTransactionResult{
		Transactions: transactions,
		TotalItems:   total,
		TotalPages:   totalPages,
		Page:         filter.Page,
		Limit:        filter.Limit,
	}, nil
}

// transactionWithJoins is a helper struct for scanning transactions with joined fields.
type transactionWithJoins struct {
	ID             int                      `db:"id"`
	TransactionID  string                   `db:"transaction_id"`
	ReferenceID    string                   `db:"reference_id"`
	ClientID       int                      `db:"client_id"`
	ProductID      int                      `db:"product_id"`
	SkuID          *int                     `db:"sku_id"`
	IsSandbox      bool                     `db:"is_sandbox"`
	CustomerNo     string                   `db:"customer_no"`
	CustomerName   *string                  `db:"customer_name"`
	Type           models.TransactionType   `db:"type"`
	Status         models.TransactionStatus `db:"status"`
	SerialNumber   *string                  `db:"serial_number"`
	Amount         *int                     `db:"amount"`
	Admin          int                      `db:"admin"`
	Period         *string                  `db:"period"`
	Description    []byte                   `db:"description"`
	FailedReason   *string                  `db:"failed_reason"`
	FailedCode     *string                  `db:"failed_code"`
	RetryCount     int                      `db:"retry_count"`
	MaxRetry       int                      `db:"max_retry"`
	NextRetryAt    *time.Time               `db:"next_retry_at"`
	ExpiredAt      *time.Time               `db:"expired_at"`
	InquiryID      *int                     `db:"inquiry_id"`
	DigiRefID      *string                  `db:"digi_ref_id"`
	BuyPrice       *int                     `db:"buy_price"`
	SellPrice      *int                     `db:"sell_price"`
	ProviderID     *int                     `db:"provider_id"`
	ProviderRefID  *string                  `db:"provider_ref_id"`
	ProviderCode   *string                  `db:"provider_code"`
	CallbackSent   bool                     `db:"callback_sent"`
	CallbackAt     *time.Time               `db:"callback_sent_at"`
	CreatedAt      time.Time                `db:"created_at"`
	ProcessedAt    *time.Time               `db:"processed_at"`
	UpdatedAt      time.Time                `db:"updated_at"`
	ProductSkuCode string                   `db:"product_sku_code"`
	DigiSkuCode    *string                  `db:"digi_sku_code"`
}

func (t *transactionWithJoins) toTransaction() models.Transaction {
	return models.Transaction{
		ID:            t.ID,
		TransactionID: t.TransactionID,
		ReferenceID:   t.ReferenceID,
		ClientID:      t.ClientID,
		ProductID:     t.ProductID,
		SkuID:         t.SkuID,
		SkuCode:       t.ProductSkuCode,
		DigiSkuCode:   t.DigiSkuCode,
		IsSandbox:     t.IsSandbox,
		CustomerNo:    t.CustomerNo,
		CustomerName:  t.CustomerName,
		Type:          t.Type,
		Status:        t.Status,
		SerialNumber:  t.SerialNumber,
		Amount:        t.Amount,
		Admin:         t.Admin,
		Period:        t.Period,
		Description:   t.Description,
		FailedReason:  t.FailedReason,
		FailedCode:    t.FailedCode,
		RetryCount:    t.RetryCount,
		MaxRetry:      t.MaxRetry,
		NextRetryAt:   t.NextRetryAt,
		ExpiredAt:     t.ExpiredAt,
		InquiryID:     t.InquiryID,
		DigiRefID:     t.DigiRefID,
		BuyPrice:      t.BuyPrice,
		SellPrice:     t.SellPrice,
		ProviderID:    t.ProviderID,
		ProviderRefID: t.ProviderRefID,
		ProviderCode:  t.ProviderCode,
		CallbackSent:  t.CallbackSent,
		CallbackAt:    t.CallbackAt,
		CreatedAt:     t.CreatedAt,
		ProcessedAt:   t.ProcessedAt,
		UpdatedAt:     t.UpdatedAt,
	}
}

// AdminTransactionStats contains transaction statistics.
// AdminTransactionStats contains transaction statistics.
type AdminTransactionStats struct {
	TotalTransactions      int   `db:"total_transactions" json:"totalTransactions"`
	SuccessTransactions    int   `db:"success_transactions" json:"successTransactions"`
	FailedTransactions     int   `db:"failed_transactions" json:"failedTransactions"`
	PendingTransactions    int   `db:"pending_transactions" json:"pendingTransactions"`
	ProcessingTransactions int   `db:"processing_transactions" json:"processingTransactions"`
	TotalAmount            int64 `db:"total_amount" json:"totalAmount"`
	// ByType breakdown
	PrepaidCount int `db:"prepaid_count" json:"prepaidCount"`
	InquiryCount int `db:"inquiry_count" json:"inquiryCount"`
	PaymentCount int `db:"payment_count" json:"paymentCount"`
}

// DailyTrend represents daily transaction statistics.
type DailyTrend struct {
	Date    string `db:"date" json:"date"`
	Total   int    `db:"total" json:"total"`
	Success int    `db:"success" json:"success"`
	Failed  int    `db:"failed" json:"failed"`
	Amount  int64  `db:"amount" json:"amount"`
}

// GetAdminStats returns transaction statistics for admin.
func (r *TransactionRepository) GetAdminStats(clientID *int, startDate, endDate *string) (*AdminTransactionStats, error) {
	q := `SELECT
            COUNT(*) as total_transactions,
            COUNT(*) FILTER (WHERE status = 'Success') as success_transactions,
            COUNT(*) FILTER (WHERE status = 'Failed') as failed_transactions,
            COUNT(*) FILTER (WHERE status = 'Pending') as pending_transactions,
            COUNT(*) FILTER (WHERE status = 'Processing') as processing_transactions,
            COALESCE(SUM(amount) FILTER (WHERE status = 'Success'), 0) as total_amount,
            COUNT(*) FILTER (WHERE type = 'prepaid') as prepaid_count,
            COUNT(*) FILTER (WHERE type = 'inquiry') as inquiry_count,
            COUNT(*) FILTER (WHERE type = 'payment') as payment_count
          FROM transactions
          WHERE 1=1`

	args := []interface{}{}
	argIdx := 1

	if clientID != nil {
		q += fmt.Sprintf(" AND client_id = $%d", argIdx)
		args = append(args, *clientID)
		argIdx++
	}
	if startDate != nil && *startDate != "" {
		q += fmt.Sprintf(" AND created_at >= $%d::date", argIdx)
		args = append(args, *startDate)
		argIdx++
	}
	if endDate != nil && *endDate != "" {
		q += fmt.Sprintf(" AND created_at < ($%d::date + interval '1 day')", argIdx)
		args = append(args, *endDate)
		argIdx++
	}

	var stats AdminTransactionStats
	if err := r.db.Get(&stats, q, args...); err != nil {
		return nil, err
	}
	return &stats, nil
}

// GetDailyTrend returns daily transaction statistics for the given period.
func (r *TransactionRepository) GetDailyTrend(clientID *int, startDate, endDate *string) ([]DailyTrend, error) {
	q := `SELECT
            TO_CHAR(created_at AT TIME ZONE 'Asia/Jakarta', 'YYYY-MM-DD') as date,
            COUNT(*) as total,
            COUNT(*) FILTER (WHERE status = 'Success') as success,
            COUNT(*) FILTER (WHERE status = 'Failed') as failed,
            COALESCE(SUM(amount) FILTER (WHERE status = 'Success'), 0) as amount
          FROM transactions
          WHERE 1=1`

	args := []interface{}{}
	argIdx := 1

	if clientID != nil {
		q += fmt.Sprintf(" AND client_id = $%d", argIdx)
		args = append(args, *clientID)
		argIdx++
	}
	if startDate != nil && *startDate != "" {
		q += fmt.Sprintf(" AND created_at >= $%d::date", argIdx)
		args = append(args, *startDate)
		argIdx++
	}
	if endDate != nil && *endDate != "" {
		q += fmt.Sprintf(" AND created_at < ($%d::date + interval '1 day')", argIdx)
		args = append(args, *endDate)
		argIdx++
	}

	q += " GROUP BY TO_CHAR(created_at AT TIME ZONE 'Asia/Jakarta', 'YYYY-MM-DD') ORDER BY date DESC LIMIT 30"

	var trends []DailyTrend
	if err := r.db.Select(&trends, q, args...); err != nil {
		return nil, err
	}
	return trends, nil
}

// GetByIDAdmin returns a transaction by ID for admin (no client filtering).
func (r *TransactionRepository) GetByIDAdmin(id int) (*models.Transaction, error) {
	const q = `SELECT t.*, p.sku_code as "sku_code" 
               FROM transactions t
               JOIN products p ON t.product_id = p.id
               WHERE t.id = $1 LIMIT 1`
	var trx models.Transaction
	if err := r.db.Get(&trx, q, id); err != nil {
		if err == sql.ErrNoRows {
			return nil, sql.ErrNoRows
		}
		return nil, err
	}
	return &trx, nil
}

// GetByTransactionIDAdmin returns a transaction by transaction_id for admin (no client filtering).
func (r *TransactionRepository) GetByTransactionIDAdmin(transactionID string) (*models.Transaction, error) {
	const q = `
		SELECT
			t.id, t.transaction_id, t.reference_id, t.client_id, t.product_id, t.sku_id,
			t.is_sandbox, t.customer_no, t.customer_name, t.type, t.status,
			t.serial_number, t.amount, t.admin, t.period, t.description,
			t.failed_reason, t.failed_code, t.retry_count, t.max_retry,
			t.next_retry_at, t.expired_at, t.inquiry_id, t.digi_ref_id,
			t.buy_price, t.sell_price,
			t.provider_id, t.provider_ref_id,
			pp.code AS provider_code,
			t.callback_sent, t.callback_sent_at, t.created_at, t.processed_at, t.updated_at,
			p.sku_code AS product_sku_code,
			s.digi_sku_code AS digi_sku_code
		FROM transactions t
		JOIN products p ON t.product_id = p.id
		LEFT JOIN skus s ON t.sku_id = s.id
		LEFT JOIN ppob_providers pp ON t.provider_id = pp.id
		WHERE t.transaction_id = $1 LIMIT 1`

	var trx transactionWithJoins
	if err := r.db.Get(&trx, q, transactionID); err != nil {
		if err == sql.ErrNoRows {
			return nil, sql.ErrNoRows
		}
		return nil, err
	}
	result := trx.toTransaction()
	return &result, nil
}

// MarkCallbackSent marks a transaction's callback as sent by its internal ID.
func (r *TransactionRepository) MarkCallbackSent(id int) error {
	const q = `UPDATE transactions SET callback_sent = true, callback_sent_at = NOW(), updated_at = NOW() WHERE id = $1`
	_, err := r.db.Exec(q, id)
	return err
}
