package repository

import (
	"encoding/json"

	"github.com/jmoiron/sqlx"

	"github.com/GTDGit/gtd_gateway/internal/models"
)

// CallbackRepository provides access to callback- and log-related tables.
type CallbackRepository struct {
	db *sqlx.DB
}

// NewCallbackRepository creates a new CallbackRepository.
func NewCallbackRepository(db *sqlx.DB) *CallbackRepository {
	return &CallbackRepository{db: db}
}

// nullableRawJSON converts empty json.RawMessage to nil for proper NULL handling in PostgreSQL.
func nullableRawJSON(v json.RawMessage) interface{} {
	if len(v) == 0 {
		return nil
	}
	return []byte(v)
}

// CreateTransactionLog inserts a new transaction log row.
func (r *CallbackRepository) CreateTransactionLog(log *models.TransactionLog) error {
	const q = `
        INSERT INTO transaction_logs (
            transaction_id, sku_id, digi_ref_id, request, response, rc, status, message,
            provider_id, provider_sku_id, created_at, response_at, response_time_ms
        ) VALUES (
            $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW(), $11, $12
        )`
	stmt, err := r.db.Preparex(q)
	if err != nil {
		return err
	}
	defer stmt.Close()
	_, err = stmt.Exec(
		log.TransactionID,
		log.SkuID,
		log.DigiRefID,
		nullableRawJSON(log.Request),
		nullableRawJSON(log.Response),
		log.RC,
		log.Status,
		log.Message,
		log.ProviderID,
		log.ProviderSKUID,
		log.ResponseAt,
		log.ResponseTimeMs,
	)
	return err
}

// GetLogsByTransactionID returns all logs for a transaction ordered by creation time.
func (r *CallbackRepository) GetLogsByTransactionID(transactionID int) ([]models.TransactionLog, error) {
	const q = `SELECT * FROM transaction_logs WHERE transaction_id = $1 ORDER BY created_at ASC`
	stmt, err := r.db.Preparex(q)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()
	var logs []models.TransactionLog
	if err := stmt.Select(&logs, transactionID); err != nil {
		return nil, err
	}
	return logs, nil
}

// CreateCallbackLog inserts a new callback log (to client).
func (r *CallbackRepository) CreateCallbackLog(log *models.CallbackLog) error {
	const q = `
        INSERT INTO callback_logs (
            transaction_id, client_id, event, payload, attempt, http_status, response_body, is_delivered, created_at, next_retry_at
        ) VALUES (
            $1,$2,$3,$4,$5,$6,$7,$8,NOW(),$9
        )`
	stmt, err := r.db.Preparex(q)
	if err != nil {
		return err
	}
	defer stmt.Close()
	_, err = stmt.Exec(
		log.TransactionID,
		log.ClientID,
		log.Event,
		log.Payload,
		log.Attempt,
		log.HTTPStatus,
		log.ResponseBody,
		log.IsDelivered,
		log.NextRetryAt,
	)
	return err
}

// UpdateCallbackLog updates an existing callback log row.
func (r *CallbackRepository) UpdateCallbackLog(log *models.CallbackLog) error {
	const q = `
        UPDATE callback_logs SET
            attempt = $2,
            http_status = $3,
            response_body = $4,
            is_delivered = $5,
            next_retry_at = $6
        WHERE id = $1`
	stmt, err := r.db.Preparex(q)
	if err != nil {
		return err
	}
	defer stmt.Close()
	_, err = stmt.Exec(
		log.ID,
		log.Attempt,
		log.HTTPStatus,
		log.ResponseBody,
		log.IsDelivered,
		log.NextRetryAt,
	)
	return err
}

// GetPendingCallbacks returns pending callback logs ready to deliver.
// Uses SKIP LOCKED to avoid duplicate processing by concurrent workers.
func (r *CallbackRepository) GetPendingCallbacks() ([]models.CallbackLog, error) {
	const q = `
        SELECT * FROM callback_logs
        WHERE is_delivered = false
          AND next_retry_at <= NOW()
          AND attempt < 5
        ORDER BY next_retry_at ASC
        FOR UPDATE SKIP LOCKED`
	stmt, err := r.db.Preparex(q)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()
	var logs []models.CallbackLog
	if err := stmt.Select(&logs); err != nil {
		return nil, err
	}
	return logs, nil
}

// MarkDelivered marks a callback as delivered.
func (r *CallbackRepository) MarkDelivered(id int) error {
	const q = `UPDATE callback_logs SET is_delivered = true WHERE id = $1`
	stmt, err := r.db.Preparex(q)
	if err != nil {
		return err
	}
	defer stmt.Close()
	_, err = stmt.Exec(id)
	return err
}

// CreateDigiflazzCallback inserts a digiflazz callback record.
func (r *CallbackRepository) CreateDigiflazzCallback(cb *models.DigiflazzCallback) error {
	const q = `
        INSERT INTO digiflazz_callbacks (
            digi_ref_id, payload, rc, status, serial_number, message, is_processed, processed_at, created_at
        ) VALUES (
            $1, $2, $3, $4, $5, $6, $7, $8, NOW()
        )`
	stmt, err := r.db.Preparex(q)
	if err != nil {
		return err
	}
	defer stmt.Close()
	_, err = stmt.Exec(
		cb.DigiRefID,
		cb.Payload,
		cb.RC,
		cb.Status,
		cb.SerialNumber,
		cb.Message,
		cb.IsProcessed,
		cb.ProcessedAt,
	)
	return err
}

// GetUnprocessedCallbacks returns digiflazz callbacks that are not processed yet.
func (r *CallbackRepository) GetUnprocessedCallbacks() ([]models.DigiflazzCallback, error) {
	const q = `
        SELECT * FROM digiflazz_callbacks
        WHERE is_processed = false
        ORDER BY id ASC
        FOR UPDATE SKIP LOCKED`
	stmt, err := r.db.Preparex(q)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()
	var list []models.DigiflazzCallback
	if err := stmt.Select(&list); err != nil {
		return nil, err
	}
	return list, nil
}

// MarkProcessed marks a digiflazz callback as processed and sets processed_at.
func (r *CallbackRepository) MarkProcessed(id int) error {
	const q = `UPDATE digiflazz_callbacks SET is_processed = true, processed_at = NOW() WHERE id = $1`
	stmt, err := r.db.Preparex(q)
	if err != nil {
		return err
	}
	defer stmt.Close()
	_, err = stmt.Exec(id)
	return err
}

// MarkProcessedWithError marks a digiflazz callback as processed with an error message.
func (r *CallbackRepository) MarkProcessedWithError(id int, errorMsg string) error {
	const q = `UPDATE digiflazz_callbacks SET is_processed = true, processed_at = NOW(), process_error = $2 WHERE id = $1`
	stmt, err := r.db.Preparex(q)
	if err != nil {
		return err
	}
	defer stmt.Close()
	_, err = stmt.Exec(id, errorMsg)
	return err
}
