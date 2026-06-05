package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/GTDGit/gtd_gateway/internal/models"
	"github.com/GTDGit/gtd_gateway/internal/repository"
	"github.com/GTDGit/gtd_gateway/internal/sse"
	"github.com/GTDGit/gtd_gateway/pkg/digiflazz"
)

// CallbackService handles outgoing callbacks to client systems and processing
// of incoming Digiflazz callbacks.
type CallbackService struct {
	clientRepo   *repository.ClientRepository
	callbackRepo *repository.CallbackRepository
	trxRepo      *repository.TransactionRepository
	httpClient   *http.Client
	// trxRetrier is set after initialization to avoid circular dependency
	trxRetrier TransactionRetrier
	notifier   sse.TransactionNotifier
}

// TransactionRetrier interface for retry functionality (avoids circular dependency)
type TransactionRetrier interface {
	RetryWithNextSKU(ctx context.Context, trx *models.Transaction, failedRC string, failedMessage string) (*models.Transaction, bool, error)
}

// ProviderFallbackRetrier retries prepaid transactions across provider mappings.
type ProviderFallbackRetrier interface {
	RetryWithNextProvider(ctx context.Context, trx *models.Transaction, failedRC string, failedMessage string) (*models.Transaction, bool, error)
}

// NewCallbackService constructs a CallbackService with a default HTTP client.
func NewCallbackService(clientRepo *repository.ClientRepository, callbackRepo *repository.CallbackRepository, trxRepo *repository.TransactionRepository) *CallbackService {
	return &CallbackService{
		clientRepo:   clientRepo,
		callbackRepo: callbackRepo,
		trxRepo:      trxRepo,
		httpClient: &http.Client{
			Timeout: 20 * time.Second,
		},
	}
}

// SetTransactionRetrier sets the transaction retrier (called after both services are created)
func (s *CallbackService) SetTransactionRetrier(retrier TransactionRetrier) {
	s.trxRetrier = retrier
}

// SetNotifier sets the SSE notifier for real-time transaction updates
func (s *CallbackService) SetNotifier(notifier sse.TransactionNotifier) {
	s.notifier = notifier
}

// SendCallback sends an HTTP POST webhook to the client's callback URL and logs the attempt.
// It schedules retries when delivery is not successful.
func (s *CallbackService) SendCallback(trx *models.Transaction, event string) error {
	if trx == nil {
		return nil
	}
	client, err := s.clientRepo.GetByID(trx.ClientID)
	if err != nil || client == nil || client.CallbackURL == "" {
		return err
	}

	payload := buildCallbackPayload(trx, event)
	signature := generateSignature(payload, client.CallbackSecret)

	req, err := http.NewRequest(http.MethodPost, client.CallbackURL, bytes.NewReader(payload))
	if err != nil {
		log.Error().Err(err).Msg("failed to create callback request")
		return err
	}
	reqID := generateRequestID()
	timestamp := time.Now().Format(time.RFC3339)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GTD-Signature", "sha256="+signature)
	req.Header.Set("X-GTD-Event", event)
	req.Header.Set("X-GTD-Timestamp", timestamp)
	req.Header.Set("X-GTD-Request-Id", reqID)

	resp, err := s.httpClient.Do(req)

	// read response body (best effort)
	var statusCode *int
	var respBody *string
	if resp != nil {
		defer resp.Body.Close()
		sc := resp.StatusCode
		statusCode = &sc
		bodyBytes, _ := io.ReadAll(resp.Body)
		bodyStr := string(bodyBytes)
		if bodyStr != "" {
			respBody = &bodyStr
		}
	}

	// Log attempt
	delivered := err == nil && resp != nil && resp.StatusCode == http.StatusOK
	logEntry := &models.CallbackLog{
		TransactionID: &trx.ID,
		ClientID:      client.ID,
		Event:         event,
		Payload:       json.RawMessage(payload),
		Attempt:       1,
		HTTPStatus:    statusCode,
		ResponseBody:  respBody,
		IsDelivered:   delivered,
	}
	if !logEntry.IsDelivered {
		next := s.getNextRetryTime(1)
		if !next.IsZero() {
			logEntry.NextRetryAt = &next
		}
	}
	if err := s.callbackRepo.CreateCallbackLog(logEntry); err != nil {
		log.Error().Err(err).Msg("failed to create callback log")
	}

	// Update transaction callback_sent status if delivered successfully
	if delivered && s.trxRepo != nil {
		now := time.Now()
		trx.CallbackSent = true
		trx.CallbackAt = &now
		if err := s.trxRepo.Update(trx); err != nil {
			log.Error().Err(err).Str("transactionId", trx.TransactionID).Msg("failed to update callback_sent status")
		}
	}

	// Schedule retry automatically handled by worker via RetryPendingCallbacks
	return nil
}

// getNextRetryTime returns next retry time based on attempt number.
// Retry intervals: 30s, 1m, 5m, 30m, 2h
func (s *CallbackService) getNextRetryTime(attempt int) time.Time {
	intervals := []time.Duration{
		30 * time.Second,
		1 * time.Minute,
		5 * time.Minute,
		30 * time.Minute,
		2 * time.Hour,
	}
	if attempt >= len(intervals) {
		return time.Time{}
	}
	return time.Now().Add(intervals[attempt])
}

// RetryPendingCallbacks retries undelivered callbacks.
func (s *CallbackService) RetryPendingCallbacks() error {
	callbacks, err := s.callbackRepo.GetPendingCallbacks()
	if err != nil {
		return err
	}
	for i := range callbacks {
		cb := &callbacks[i]
		client, err := s.clientRepo.GetByID(cb.ClientID)
		if err != nil || client == nil || client.CallbackURL == "" {
			continue
		}
		req, err := http.NewRequest(http.MethodPost, client.CallbackURL, bytes.NewReader(cb.Payload))
		if err != nil {
			continue
		}
		// Recompute signature (payload unchanged)
		sig := generateSignature([]byte(cb.Payload), client.CallbackSecret)
		reqID := generateRequestID()
		timestamp := time.Now().Format(time.RFC3339)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-GTD-Signature", "sha256="+sig)
		req.Header.Set("X-GTD-Event", cb.Event)
		req.Header.Set("X-GTD-Timestamp", timestamp)
		req.Header.Set("X-GTD-Request-Id", reqID)

		resp, err := s.httpClient.Do(req)
		var statusCode *int
		var respBody *string
		if resp != nil {
			defer resp.Body.Close()
			sc := resp.StatusCode
			statusCode = &sc
			b, _ := io.ReadAll(resp.Body)
			bs := string(b)
			if bs != "" {
				respBody = &bs
			}
		}

		cb.Attempt++
		cb.HTTPStatus = statusCode
		cb.ResponseBody = respBody
		delivered := err == nil && resp != nil && resp.StatusCode == http.StatusOK
		cb.IsDelivered = delivered
		if !delivered {
			next := s.getNextRetryTime(cb.Attempt)
			if next.IsZero() {
				// No more retries
				cb.NextRetryAt = nil
			} else {
				cb.NextRetryAt = &next
			}
		} else {
			cb.NextRetryAt = nil
			// Update transaction callback_sent status
			if s.trxRepo != nil && cb.TransactionID != nil {
				s.trxRepo.MarkCallbackSent(*cb.TransactionID)
			}
		}

		if err := s.callbackRepo.UpdateCallbackLog(cb); err != nil {
			log.Error().Err(err).Msg("failed to update callback log")
		}
	}
	return nil
}

// ProcessDigiflazzCallback processes Digiflazz callback immediately.
// Stores callback for audit trail and processes it right away for fast response.
func (s *CallbackService) ProcessDigiflazzCallback(payload *digiflazz.CallbackPayload) error {
	if payload == nil {
		return nil
	}

	// 1. Persist raw callback for audit trail
	raw, _ := json.Marshal(payload)
	cb := &models.DigiflazzCallback{
		DigiRefID: payload.RefID,
		Payload:   json.RawMessage(raw),
		RC:        &payload.RC,
		Status:    &payload.Status,
		SerialNumber: func() *string {
			if payload.SN == "" {
				return nil
			}
			v := payload.SN
			return &v
		}(),
		Message: func() *string {
			if payload.Message == "" {
				return nil
			}
			v := payload.Message
			return &v
		}(),
		IsProcessed: false,
	}
	if err := s.callbackRepo.CreateDigiflazzCallback(cb); err != nil {
		log.Error().Err(err).Msg("failed to store digiflazz callback")
		// Continue processing even if storage fails
	}

	// 2. Process callback immediately
	s.processCallbackImmediate(cb, payload)

	return nil
}

// processCallbackImmediate handles the callback processing logic immediately
func (s *CallbackService) processCallbackImmediate(cb *models.DigiflazzCallback, payload *digiflazz.CallbackPayload) {
	// Find transaction by digi_ref_id
	trx, err := s.trxRepo.GetByDigiRefID(payload.RefID)
	if err != nil {
		// Try base ref_id (without suffix)
		baseRefID := extractBaseRefID(payload.RefID)
		if baseRefID != payload.RefID {
			trx, err = s.trxRepo.GetByDigiRefID(baseRefID)
			if err != nil {
				trx, err = s.trxRepo.GetByTransactionID(baseRefID)
			}
		} else {
			trx, err = s.trxRepo.GetByTransactionID(payload.RefID)
		}
	}

	if err != nil || trx == nil {
		log.Warn().
			Str("digi_ref_id", payload.RefID).
			Msg("Transaction not found for Digiflazz callback, will retry via worker")
		return // Worker will retry later
	}

	// Skip if transaction is already in final state
	if trx.Status == models.StatusSuccess || trx.Status == models.StatusFailed {
		log.Debug().
			Str("transaction_id", trx.TransactionID).
			Str("status", string(trx.Status)).
			Msg("Transaction already in final state, skipping callback")
		s.markCallbackProcessed(cb.ID)
		return
	}

	rc := payload.RC
	now := time.Now()

	// Process based on RC code
	switch {
	case digiflazz.IsSuccess(rc):
		trx.Status = models.StatusSuccess
		if payload.SN != "" {
			trx.SerialNumber = &payload.SN
		}
		if payload.Price > 0 {
			trx.BuyPrice = &payload.Price
		}
		trx.ProcessedAt = &now
		trx.CallbackSent = false

		if err := s.trxRepo.Update(trx); err != nil {
			log.Error().Err(err).Str("transaction_id", trx.TransactionID).Msg("Failed to update transaction to success")
			return
		}
		if s.notifier != nil {
			s.notifier.NotifyTransactionStatusChanged(trx)
		}

		go s.SendCallback(trx, "transaction.success")
		log.Info().Str("transaction_id", trx.TransactionID).Msg("Transaction updated to Success from Digiflazz callback")

	case digiflazz.IsFatal(rc):
		msg := payload.Message
		trx.Status = models.StatusFailed
		trx.FailedReason = &msg
		trx.FailedCode = &rc
		trx.ProcessedAt = &now

		if err := s.trxRepo.Update(trx); err != nil {
			log.Error().Err(err).Str("transaction_id", trx.TransactionID).Msg("Failed to update transaction to failed")
			return
		}
		if s.notifier != nil {
			s.notifier.NotifyTransactionStatusChanged(trx)
		}

		go s.SendCallback(trx, "transaction.failed")
		log.Info().Str("transaction_id", trx.TransactionID).Str("rc", rc).Msg("Transaction updated to Failed (fatal RC)")

	case digiflazz.IsRetryable(rc):
		// Retryable RC - try with next SKU immediately
		if s.trxRetrier == nil {
			log.Warn().Str("transaction_id", trx.TransactionID).Msg("Transaction retrier not set, cannot retry")
			return
		}

		log.Info().
			Str("transaction_id", trx.TransactionID).
			Str("rc", rc).
			Str("message", payload.Message).
			Msg("Retryable RC, attempting immediate retry with next SKU")

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		result, shouldMarkFailed, err := s.trxRetrier.RetryWithNextSKU(ctx, trx, rc, payload.Message)
		if err != nil {
			log.Error().Err(err).Str("transaction_id", trx.TransactionID).Msg("Error during retry")
			return // Worker will retry later
		}

		if shouldMarkFailed {
			log.Info().Str("transaction_id", trx.TransactionID).Msg("All SKUs exhausted after retry")
		} else {
			log.Info().
				Str("transaction_id", trx.TransactionID).
				Str("status", string(result.Status)).
				Msg("Retry completed")
		}

	case digiflazz.IsPending(rc):
		log.Debug().Str("transaction_id", trx.TransactionID).Msg("Transaction still pending")
		// Don't mark as processed - will be picked up again if another callback comes

	default:
		// Unknown RC - treat as failed
		log.Warn().
			Str("transaction_id", trx.TransactionID).
			Str("rc", rc).
			Msg("Unknown RC code, treating as failed")

		msg := payload.Message
		trx.Status = models.StatusFailed
		trx.FailedReason = &msg
		trx.FailedCode = &rc
		trx.ProcessedAt = &now

		if err := s.trxRepo.Update(trx); err != nil {
			log.Error().Err(err).Str("transaction_id", trx.TransactionID).Msg("Failed to update transaction to failed")
			return
		}
		if s.notifier != nil {
			s.notifier.NotifyTransactionStatusChanged(trx)
		}

		go s.SendCallback(trx, "transaction.failed")
	}

	// Mark callback as processed
	s.markCallbackProcessed(cb.ID)
}

// markCallbackProcessed marks the callback as processed in the database
func (s *CallbackService) markCallbackProcessed(id int) {
	if err := s.callbackRepo.MarkProcessed(id); err != nil {
		log.Error().Err(err).Int("id", id).Msg("Failed to mark callback as processed")
	}
}

// extractBaseRefID extracts the base transaction ID from a ref_id that might have a suffix.
func extractBaseRefID(refID string) string {
	dashCount := 0
	lastDashPos := -1
	for i, c := range refID {
		if c == '-' {
			dashCount++
			if dashCount == 3 {
				lastDashPos = i
				break
			}
		}
	}
	if lastDashPos > 0 {
		return refID[:lastDashPos]
	}
	return refID
}

// buildCallbackPayload constructs the JSON payload sent to clients.
func buildCallbackPayload(trx *models.Transaction, event string) []byte {
	type dataPayload struct {
		TransactionID string      `json:"transactionId"`
		ReferenceID   string      `json:"referenceId,omitempty"`
		SkuCode       string      `json:"skuCode,omitempty"`
		CustomerNo    string      `json:"customerNo,omitempty"`
		CustomerName  *string     `json:"customerName,omitempty"`
		Type          string      `json:"type,omitempty"`
		Status        string      `json:"status"`
		SerialNumber  *string     `json:"serialNumber,omitempty"`
		Price         *int        `json:"price,omitempty"`
		Admin         int         `json:"admin,omitempty"`
		Period        *string     `json:"period,omitempty"`
		Description   interface{} `json:"description,omitempty"`
		FailedReason  *string     `json:"failedReason,omitempty"`
		FailedCode    *string     `json:"failedCode,omitempty"`
		CreatedAt     time.Time   `json:"createdAt"`
		ProcessedAt   *time.Time  `json:"processedAt,omitempty"`
	}
	type payload struct {
		Event     string      `json:"event"`
		Data      dataPayload `json:"data"`
		Timestamp string      `json:"timestamp"`
	}
	var desc any
	if len(trx.Description) > 0 {
		_ = json.Unmarshal(trx.Description, &desc)
	}
	p := payload{
		Event: event,
		Data: dataPayload{
			TransactionID: trx.TransactionID,
			ReferenceID:   trx.ReferenceID,
			SkuCode:       trx.SkuCode,
			CustomerNo:    trx.CustomerNo,
			CustomerName:  trx.CustomerName,
			Type:          string(trx.Type),
			Status:        string(trx.Status),
			SerialNumber:  trx.SerialNumber,
			Price:         trx.Amount,
			Admin:         trx.Admin,
			Period:        trx.Period,
			Description:   desc,
			FailedReason:  trx.FailedReason,
			FailedCode:    trx.FailedCode,
			CreatedAt:     trx.CreatedAt,
			ProcessedAt:   trx.ProcessedAt,
		},
		Timestamp: time.Now().Format(time.RFC3339),
	}
	b, _ := json.Marshal(p)
	return b
}

// generateSignature creates HMAC-SHA256 signature of payload using secret.
func generateSignature(payload []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

// generateRequestID creates a unique request ID for callback tracking.
func generateRequestID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "cb_" + hex.EncodeToString(b)
}
