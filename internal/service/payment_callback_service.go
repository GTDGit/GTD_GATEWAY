package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/GTDGit/gtd_gateway/internal/models"
	"github.com/GTDGit/gtd_gateway/internal/repository"
)

const (
	paymentCallbackMaxAttempts = 5
	paymentCallbackSignatureHeader = "X-GTD-Payment-Signature"
)

// PaymentCallbackService delivers HMAC-signed webhooks to the merchant's
// configured URL and persists each delivery attempt to payment_callback_logs.
type PaymentCallbackService struct {
	paymentRepo *repository.PaymentRepository
	clientRepo  *repository.ClientRepository
	httpClient  *http.Client
}

func NewPaymentCallbackService(paymentRepo *repository.PaymentRepository, clientRepo *repository.ClientRepository) *PaymentCallbackService {
	return &PaymentCallbackService{
		paymentRepo: paymentRepo,
		clientRepo:  clientRepo,
		httpClient:  &http.Client{Timeout: 20 * time.Second},
	}
}

// EnqueueEvent creates a pending delivery row and attempts a first send. The
// worker picks up failed rows for retry.
func (s *PaymentCallbackService) EnqueueEvent(ctx context.Context, payment *models.Payment, event string) {
	if s == nil || payment == nil {
		return
	}
	client, err := s.clientRepo.GetByID(payment.ClientID)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			log.Warn().Err(err).Int("clientId", payment.ClientID).Msg("payment callback: load client")
		}
		return
	}
	url, secret := client.EffectivePaymentCallback()
	if url == "" {
		return
	}

	payload := buildPaymentCallbackPayload(payment, event)
	logRow := &models.PaymentCallbackLog{
		PaymentID:   payment.ID,
		ClientID:    client.ID,
		Event:       event,
		Payload:     models.NullableRawMessage(payload),
		Attempt:     0,
		MaxAttempts: paymentCallbackMaxAttempts,
	}
	if err := s.paymentRepo.CreatePaymentCallbackLog(ctx, logRow); err != nil {
		log.Warn().Err(err).Str("paymentId", payment.PaymentID).Msg("payment callback: insert log")
		return
	}
	s.AttemptDelivery(ctx, logRow, url, secret)
}

// RetryPendingCallbacks scans payment_callback_logs for undelivered rows whose
// next_retry_at has elapsed and re-attempts each one.
func (s *PaymentCallbackService) RetryPendingCallbacks(ctx context.Context, limit int) error {
	if s == nil {
		return nil
	}
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.paymentRepo.GetPendingPaymentCallbackLogs(ctx, limit)
	if err != nil {
		return err
	}
	for i := range rows {
		row := &rows[i]
		client, err := s.clientRepo.GetByID(row.ClientID)
		if err != nil {
			continue
		}
		url, secret := client.EffectivePaymentCallback()
		if url == "" {
			continue
		}
		s.AttemptDelivery(ctx, row, url, secret)
	}
	return nil
}

// AttemptDelivery sends one webhook attempt, updates the log row, and marks
// the payment delivered on 2xx. Exposed so admin retry endpoints can force an
// immediate send without waiting for the worker.
func (s *PaymentCallbackService) AttemptDelivery(ctx context.Context, row *models.PaymentCallbackLog, url, secret string) {
	payload := []byte(row.Payload)
	signature := hmacHexSHA256(payload, secret)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		s.markFailure(ctx, row, nil, nil, err.Error())
		return
	}
	reqID := genPaymentRequestID()
	ts := time.Now().UTC().Format(time.RFC3339)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(paymentCallbackSignatureHeader, "sha256="+signature)
	req.Header.Set("X-GTD-Event", row.Event)
	req.Header.Set("X-GTD-Timestamp", ts)
	req.Header.Set("X-GTD-Request-Id", reqID)

	start := time.Now()
	resp, err := s.httpClient.Do(req)
	elapsed := int(time.Since(start) / time.Millisecond)

	var status *int
	var body *string
	if resp != nil {
		defer resp.Body.Close()
		sc := resp.StatusCode
		status = &sc
		bodyBytes, _ := io.ReadAll(resp.Body)
		if len(bodyBytes) > 0 {
			b := string(bodyBytes)
			body = &b
		}
	}
	delivered := err == nil && resp != nil && resp.StatusCode >= 200 && resp.StatusCode < 300

	row.Attempt++
	row.HTTPStatus = status
	row.ResponseBody = body
	row.ResponseTimeMs = &elapsed
	row.IsDelivered = delivered
	if delivered {
		now := time.Now()
		row.DeliveredAt = &now
		row.NextRetryAt = nil
		row.ErrorMessage = nil
		if upErr := s.paymentRepo.UpdatePaymentCallbackLog(ctx, row); upErr != nil {
			log.Warn().Err(upErr).Msg("payment callback: update log")
		}
		if err := s.paymentRepo.MarkPaymentCallbackSent(ctx, row.PaymentID, now); err != nil {
			log.Warn().Err(err).Int("paymentId", row.PaymentID).Msg("payment callback: mark sent")
		}
		return
	}

	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	} else if status != nil {
		errMsg = "non-2xx response"
	}
	s.markFailure(ctx, row, status, body, errMsg)
}

func (s *PaymentCallbackService) markFailure(ctx context.Context, row *models.PaymentCallbackLog, status *int, body *string, errMsg string) {
	if row.Attempt == 0 {
		row.Attempt = 1
	}
	if status != nil {
		row.HTTPStatus = status
	}
	if body != nil {
		row.ResponseBody = body
	}
	if errMsg != "" {
		row.ErrorMessage = &errMsg
	}
	next := paymentNextRetry(row.Attempt)
	if !next.IsZero() && row.Attempt < row.MaxAttempts {
		row.NextRetryAt = &next
	} else {
		row.NextRetryAt = nil
	}
	row.IsDelivered = false
	if err := s.paymentRepo.UpdatePaymentCallbackLog(ctx, row); err != nil {
		log.Warn().Err(err).Msg("payment callback: update log on failure")
	}
}

// paymentNextRetry applies the backoff schedule 30s / 1m / 5m / 30m / 2h.
func paymentNextRetry(attempt int) time.Time {
	intervals := []time.Duration{
		30 * time.Second,
		1 * time.Minute,
		5 * time.Minute,
		30 * time.Minute,
		2 * time.Hour,
	}
	if attempt <= 0 || attempt > len(intervals) {
		return time.Time{}
	}
	return time.Now().Add(intervals[attempt-1])
}

func hmacHexSHA256(payload []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

func genPaymentRequestID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "pcb_" + hex.EncodeToString(b)
}

// buildPaymentCallbackPayload renders the merchant-facing webhook body.
func buildPaymentCallbackPayload(p *models.Payment, event string) []byte {
	type data struct {
		PaymentID       string          `json:"paymentId"`
		ReferenceID     string          `json:"referenceId"`
		Type            string          `json:"type"`
		Status          string          `json:"status"`
		PaymentCode     string          `json:"paymentCode,omitempty"`
		Provider        string          `json:"provider,omitempty"`
		Amount          int64           `json:"amount"`
		Fee             int64           `json:"fee"`
		TotalAmount     int64           `json:"totalAmount"`
		CustomerName    *string         `json:"customerName,omitempty"`
		PaymentDetail   json.RawMessage `json:"paymentDetail,omitempty"`
		ProviderRef     *string         `json:"providerRef,omitempty"`
		PaidAt          *time.Time      `json:"paidAt,omitempty"`
		CancelledAt     *time.Time      `json:"cancelledAt,omitempty"`
		ExpiredAt       time.Time       `json:"expiredAt"`
		CreatedAt       time.Time       `json:"createdAt"`
	}
	type envelope struct {
		Event     string    `json:"event"`
		Data      data      `json:"data"`
		Timestamp time.Time `json:"timestamp"`
	}
	out := envelope{
		Event: event,
		Data: data{
			PaymentID:     p.PaymentID,
			ReferenceID:   p.ReferenceID,
			Type:          string(p.PaymentType),
			Status:        string(p.Status),
			PaymentCode:   p.PaymentCode,
			Provider:      string(p.Provider),
			Amount:        p.Amount,
			Fee:           p.Fee,
			TotalAmount:   p.TotalAmount,
			CustomerName:  p.CustomerName,
			PaymentDetail: json.RawMessage(p.PaymentDetail),
			ProviderRef:   p.ProviderRef,
			PaidAt:        p.PaidAt,
			CancelledAt:   p.CancelledAt,
			ExpiredAt:     p.ExpiredAt,
			CreatedAt:     p.CreatedAt,
		},
		Timestamp: time.Now().UTC(),
	}
	b, _ := json.Marshal(out)
	return b
}
