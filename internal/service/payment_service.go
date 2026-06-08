package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/GTDGit/gtd_gateway/internal/models"
	"github.com/GTDGit/gtd_gateway/internal/repository"
	"github.com/GTDGit/gtd_gateway/internal/sse"
)

// ----------------------------------------------------------------------------
// Errors
// ----------------------------------------------------------------------------

type PaymentServiceError struct {
	HTTPStatus int
	Code       string
	Message    string
	Err        error
}

func (e *PaymentServiceError) Error() string {
	if e == nil {
		return ""
	}
	if e.Err == nil {
		return e.Code + ": " + e.Message
	}
	return e.Code + ": " + e.Message + ": " + e.Err.Error()
}

func (e *PaymentServiceError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func newPaymentError(httpStatus int, code, message string, err error) *PaymentServiceError {
	return &PaymentServiceError{HTTPStatus: httpStatus, Code: code, Message: message, Err: err}
}

// ----------------------------------------------------------------------------
// Request/response DTOs
// ----------------------------------------------------------------------------

type CreatePaymentRequest struct {
	ReferenceID   string          `json:"referenceId"`
	PaymentType   string          `json:"paymentType"`
	PaymentCode   string          `json:"paymentCode"`
	Amount        int64           `json:"amount"`
	ExpiredAt     string          `json:"expiredAt,omitempty"`
	CustomerName  string          `json:"customerName,omitempty"`
	CustomerEmail string          `json:"customerEmail,omitempty"`
	CustomerPhone string          `json:"customerPhone,omitempty"`
	Description   string          `json:"description,omitempty"`
	CallbackURL   string          `json:"callbackUrl,omitempty"`
	ReturnURL     string          `json:"returnUrl,omitempty"`
	Metadata      json.RawMessage `json:"metadata,omitempty"`
}

// PaymentResponse is the shape returned on create/get endpoints.
type PaymentResponse struct {
	PaymentID          string          `json:"paymentId"`
	ReferenceID        string          `json:"referenceId"`
	PaymentType        string          `json:"paymentType"`
	PaymentCode        string          `json:"paymentCode"`
	Provider           string          `json:"provider"`
	Amount             int64           `json:"amount"`
	Fee                int64           `json:"fee"`
	TotalAmount        int64           `json:"totalAmount"`
	Status             string          `json:"status"`
	PaymentDetail      json.RawMessage `json:"paymentDetail,omitempty"`
	PaymentInstruction json.RawMessage `json:"paymentInstruction,omitempty"`
	ProviderRef        string          `json:"providerRef,omitempty"`
	CustomerName       string          `json:"customerName,omitempty"`
	CustomerEmail      string          `json:"customerEmail,omitempty"`
	CustomerPhone      string          `json:"customerPhone,omitempty"`
	Description        string          `json:"description,omitempty"`
	ExpiredAt          string          `json:"expiredAt"`
	PaidAt             string          `json:"paidAt,omitempty"`
	CancelledAt        string          `json:"cancelledAt,omitempty"`
	CreatedAt          string          `json:"createdAt"`
}

// MethodsResponse groups active methods by payment type for the list endpoint.
type MethodsResponse struct {
	VA      []MethodEntry `json:"va"`
	Ewallet []MethodEntry `json:"ewallet"`
	QRIS    []MethodEntry `json:"qris"`
	Retail  []MethodEntry `json:"retail"`
}

type MethodEntry struct {
	ID              int    `json:"id"`
	Code            string `json:"code"`
	Name            string `json:"name"`
	Provider        string `json:"provider"`
	FeeType         string `json:"feeType"`
	FeeFlat         int    `json:"feeFlat"`
	FeePercent      float64 `json:"feePercent"`
	FeeMin          int    `json:"feeMin"`
	FeeMax          int    `json:"feeMax"`
	MinAmount       int    `json:"minAmount"`
	MaxAmount       int    `json:"maxAmount"`
	ExpiredDuration int    `json:"expiredDuration"`
	LogoURL         string `json:"logoUrl,omitempty"`
	IsMaintenance   bool   `json:"isMaintenance"`
}

// ----------------------------------------------------------------------------
// Service
// ----------------------------------------------------------------------------

type PaymentService struct {
	paymentRepo *repository.PaymentRepository
	clientRepo  *repository.ClientRepository
	router      *PaymentProviderRouter
	callbackSvc *PaymentCallbackService
	notifier    sse.PaymentNotifier
}

func NewPaymentService(
	paymentRepo *repository.PaymentRepository,
	clientRepo *repository.ClientRepository,
	router *PaymentProviderRouter,
	callbackSvc *PaymentCallbackService,
) *PaymentService {
	return &PaymentService{
		paymentRepo: paymentRepo,
		clientRepo:  clientRepo,
		router:      router,
		callbackSvc: callbackSvc,
	}
}

func (s *PaymentService) SetNotifier(notifier sse.PaymentNotifier) {
	s.notifier = notifier
}

// ----------------------------------------------------------------------------
// Method listing
// ----------------------------------------------------------------------------

func (s *PaymentService) ListMethods(ctx context.Context) (*MethodsResponse, error) {
	methods, err := s.paymentRepo.ListActiveMethods(ctx)
	if err != nil {
		return nil, err
	}
	resp := &MethodsResponse{}
	for i := range methods {
		m := methods[i]
		entry := MethodEntry{
			ID:              m.ID,
			Code:            m.Code,
			Name:            m.Name,
			Provider:        string(m.Provider),
			FeeType:         string(m.FeeType),
			FeeFlat:         m.FeeFlat,
			FeePercent:      m.FeePercent,
			FeeMin:          m.FeeMin,
			FeeMax:          m.FeeMax,
			MinAmount:       m.MinAmount,
			MaxAmount:       m.MaxAmount,
			ExpiredDuration: m.ExpiredDuration,
			IsMaintenance:   m.IsMaintenance,
		}
		if m.LogoURL != nil {
			entry.LogoURL = *m.LogoURL
		}
		switch m.Type {
		case models.PaymentTypeVA:
			resp.VA = append(resp.VA, entry)
		case models.PaymentTypeEwallet:
			resp.Ewallet = append(resp.Ewallet, entry)
		case models.PaymentTypeQRIS:
			resp.QRIS = append(resp.QRIS, entry)
		case models.PaymentTypeRetail:
			resp.Retail = append(resp.Retail, entry)
		}
	}
	return resp, nil
}

// ----------------------------------------------------------------------------
// Create
// ----------------------------------------------------------------------------

func (s *PaymentService) CreatePayment(ctx context.Context, req *CreatePaymentRequest, client *models.Client, isSandbox bool) (*PaymentResponse, error) {
	if req == nil {
		return nil, newPaymentError(400, "MISSING_FIELD", "Invalid request body", nil)
	}
	if client == nil {
		return nil, newPaymentError(401, "INVALID_TOKEN", "Unauthorized", nil)
	}

	req.ReferenceID = strings.TrimSpace(req.ReferenceID)
	req.PaymentType = strings.ToUpper(strings.TrimSpace(req.PaymentType))
	req.PaymentCode = strings.TrimSpace(req.PaymentCode)

	if req.ReferenceID == "" {
		return nil, newPaymentError(400, "MISSING_FIELD", "referenceId is required", nil)
	}
	if req.PaymentType == "" || req.PaymentCode == "" {
		return nil, newPaymentError(400, "MISSING_FIELD", "paymentType and paymentCode are required", nil)
	}
	if req.Amount <= 0 {
		return nil, newPaymentError(400, "INVALID_AMOUNT", "amount must be positive", nil)
	}

	// Idempotency: return existing record for identical referenceId.
	if existing, err := s.paymentRepo.GetByReferenceID(ctx, client.ID, req.ReferenceID); err == nil && existing != nil {
		return s.buildResponse(existing), nil
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	method, err := s.paymentRepo.GetMethodByTypeCode(ctx, models.PaymentType(req.PaymentType), req.PaymentCode)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, newPaymentError(404, "PAYMENT_METHOD_NOT_FOUND", "Payment method not found", nil)
		}
		return nil, err
	}
	if !method.IsActive {
		return nil, newPaymentError(400, "PAYMENT_METHOD_INACTIVE", "Payment method is not active", nil)
	}
	if method.IsMaintenance {
		msg := "Payment method is under maintenance"
		if method.MaintenanceMessage != nil && *method.MaintenanceMessage != "" {
			msg = *method.MaintenanceMessage
		}
		return nil, newPaymentError(503, "PAYMENT_METHOD_MAINTENANCE", msg, nil)
	}
	if method.MinAmount > 0 && req.Amount < int64(method.MinAmount) {
		return nil, newPaymentError(400, "AMOUNT_TOO_LOW", fmt.Sprintf("amount must be >= %d", method.MinAmount), nil)
	}
	if method.MaxAmount > 0 && req.Amount > int64(method.MaxAmount) {
		return nil, newPaymentError(400, "AMOUNT_TOO_HIGH", fmt.Sprintf("amount must be <= %d", method.MaxAmount), nil)
	}

	fee := method.CalculateFee(req.Amount)
	totalAmount := req.Amount + fee
	expiredAt := resolveExpiredAt(req.ExpiredAt, method.ExpiredDuration)

	metadata := models.NullableRawMessage(req.Metadata)

	payment := &models.Payment{
		ReferenceID:     req.ReferenceID,
		ClientID:        client.ID,
		PaymentMethodID: method.ID,
		IsSandbox:       isSandbox,
		PaymentType:     method.Type,
		PaymentCode:     method.Code,
		Provider:        method.Provider,
		Amount:          req.Amount,
		Fee:             fee,
		TotalAmount:     totalAmount,
		Status:          models.PaymentStatusPending,
		ExpiredAt:       expiredAt,
		Metadata:        metadata,
	}
	if req.CustomerName != "" {
		v := req.CustomerName
		payment.CustomerName = &v
	}
	if req.CustomerEmail != "" {
		v := req.CustomerEmail
		payment.CustomerEmail = &v
	}
	if req.CustomerPhone != "" {
		v := req.CustomerPhone
		payment.CustomerPhone = &v
	}
	if req.Description != "" {
		v := req.Description
		payment.Description = &v
	}

	if err := s.createPaymentWithGeneratedID(ctx, payment); err != nil {
		return nil, err
	}

	// Resolve adapter and execute provider-side creation.
	provider, err := s.router.Get(method.Provider)
	if err != nil {
		// Mark as failed since we cannot reach the provider.
		s.markFailed(ctx, payment, "PROVIDER_UNAVAILABLE", err.Error())
		return nil, err
	}

	providerReq := &PaymentCreateRequest{
		Type:          method.Type,
		Code:          method.Code,
		BankCode:      method.Code,
		PartnerRef:    payment.PaymentID,
		Amount:        req.Amount,
		Fee:           fee,
		TotalAmount:   totalAmount,
		ExpiredAt:     expiredAt,
		Description:   req.Description,
		CustomerName:  req.CustomerName,
		CustomerEmail: req.CustomerEmail,
		CustomerPhone: req.CustomerPhone,
		CallbackURL:   req.CallbackURL,
		ReturnURL:     req.ReturnURL,
	}
	start := time.Now()
	providerResp, providerErr := provider.CreatePayment(ctx, method, providerReq)
	elapsed := int(time.Since(start) / time.Millisecond)
	logEntry := &models.PaymentLog{
		PaymentID: payment.ID,
		Action:    "create",
		Provider:  method.Provider,
		IsSuccess: providerErr == nil,
	}
	reqBody, _ := json.Marshal(providerReq)
	logEntry.Request = models.NullableRawMessage(reqBody)
	if providerResp != nil {
		logEntry.Response = models.NullableRawMessage(providerResp.RawResponse)
	}
	if providerErr != nil {
		msg := providerErr.Error()
		logEntry.ErrorMessage = &msg
		code := "PROVIDER_ERROR"
		if svcErr, ok := providerErr.(*PaymentServiceError); ok {
			code = svcErr.Code
		}
		logEntry.ErrorCode = &code
	}
	now := time.Now()
	logEntry.ResponseAt = &now
	logEntry.ResponseTimeMs = &elapsed
	if err := s.paymentRepo.CreatePaymentLog(ctx, logEntry); err != nil {
		log.Warn().Err(err).Msg("payment: store log")
	}

	if providerErr != nil {
		s.markFailed(ctx, payment, "PROVIDER_ERROR", providerErr.Error())
		return nil, providerErr
	}

	detailJSON, _ := json.Marshal(providerResp.Normalized)
	payment.PaymentDetail = models.NullableRawMessage(detailJSON)
	if providerResp.ProviderRef != "" {
		v := providerResp.ProviderRef
		payment.ProviderRef = &v
	}
	payment.ProviderData = mergeProviderData(payment.ProviderData, "create", providerResp.RawResponse)
	payment.PaymentInstruction = renderInstruction(method, providerResp.Normalized, req.Amount)

	if err := s.paymentRepo.UpdatePayment(ctx, payment); err != nil {
		return nil, err
	}

	if s.notifier != nil {
		s.notifier.NotifyPaymentCreated(payment)
	}
	go s.EnqueueCallback(context.Background(), payment, "payment.pending")

	return s.buildResponse(payment), nil
}

// ----------------------------------------------------------------------------
// Get / read path
// ----------------------------------------------------------------------------

func (s *PaymentService) GetPayment(ctx context.Context, paymentID string, clientID int) (*PaymentResponse, error) {
	paymentID = strings.TrimSpace(paymentID)
	if paymentID == "" {
		return nil, newPaymentError(400, "MISSING_FIELD", "paymentId is required", nil)
	}
	p, err := s.paymentRepo.GetByPaymentID(ctx, paymentID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, newPaymentError(404, "PAYMENT_NOT_FOUND", "Payment not found", nil)
		}
		return nil, err
	}
	if p.ClientID != clientID {
		return nil, newPaymentError(404, "PAYMENT_NOT_FOUND", "Payment not found", nil)
	}
	if p.Status == models.PaymentStatusPending && time.Since(p.UpdatedAt) > 15*time.Second {
		if refreshed, err := s.refreshStatus(ctx, p); err == nil && refreshed != nil {
			p = refreshed
		}
	}
	return s.buildResponse(p), nil
}

// ----------------------------------------------------------------------------
// Cancel
// ----------------------------------------------------------------------------

func (s *PaymentService) CancelPayment(ctx context.Context, paymentID string, clientID int, reason string) (*PaymentResponse, error) {
	p, err := s.paymentRepo.GetByPaymentID(ctx, strings.TrimSpace(paymentID))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, newPaymentError(404, "PAYMENT_NOT_FOUND", "Payment not found", nil)
		}
		return nil, err
	}
	if p.ClientID != clientID {
		return nil, newPaymentError(404, "PAYMENT_NOT_FOUND", "Payment not found", nil)
	}
	if p.Status != models.PaymentStatusPending {
		return nil, newPaymentError(400, "INVALID_PAYMENT_STATE", "Only pending payments can be cancelled", nil)
	}
	if provider, err := s.router.Get(p.Provider); err == nil {
		if _, err := provider.CancelPayment(ctx, p, reason); err != nil {
			log.Warn().Err(err).Str("paymentId", p.PaymentID).Msg("payment: provider cancel failed")
			// Continue — we still mark locally cancelled so merchant is consistent.
		}
	}
	now := time.Now()
	p.Status = models.PaymentStatusCancelled
	p.CancelledAt = &now
	if err := s.paymentRepo.UpdatePayment(ctx, p); err != nil {
		return nil, err
	}
	if s.notifier != nil {
		s.notifier.NotifyPaymentStatusChanged(p)
	}
	go s.EnqueueCallback(context.Background(), p, "payment.cancelled")
	return s.buildResponse(p), nil
}

// ----------------------------------------------------------------------------
// Workers hooks
// ----------------------------------------------------------------------------

// ProcessPendingPayments reconciles stale Pending payments by inquiring provider.
func (s *PaymentService) ProcessPendingPayments(ctx context.Context, staleAfter time.Duration, limit int) error {
	rows, err := s.paymentRepo.GetPendingPaymentsPastStale(ctx, staleAfter, limit)
	if err != nil {
		return err
	}
	for i := range rows {
		p := &rows[i]
		if _, err := s.refreshStatus(ctx, p); err != nil {
			log.Warn().Err(err).Str("paymentId", p.PaymentID).Msg("payment: refresh status")
		}
	}
	return nil
}

// ExpirePendingPayments marks Pending payments whose expired_at is in the past.
func (s *PaymentService) ExpirePendingPayments(ctx context.Context, limit int) error {
	rows, err := s.paymentRepo.GetExpiredPendingPayments(ctx, limit)
	if err != nil {
		return err
	}
	for i := range rows {
		p := &rows[i]
		p.Status = models.PaymentStatusExpired
		if err := s.paymentRepo.UpdatePayment(ctx, p); err != nil {
			log.Warn().Err(err).Str("paymentId", p.PaymentID).Msg("payment: mark expired")
			continue
		}
		if s.notifier != nil {
			s.notifier.NotifyPaymentStatusChanged(p)
		}
		go s.EnqueueCallback(context.Background(), p, "payment.expired")
	}
	return nil
}

// ApplyWebhook processes an authenticated provider webhook. The provider
// layer is responsible for verifying signatures before calling this.
func (s *PaymentService) ApplyWebhook(ctx context.Context, provider models.PaymentProvider, partnerRef string, event PaymentWebhookEvent) error {
	p, err := s.paymentRepo.GetByPaymentID(ctx, partnerRef)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Try lookup by provider_ref as some providers echo their own id.
			p, err = s.paymentRepo.GetByProviderRef(ctx, provider, partnerRef)
			if err != nil {
				return err
			}
		} else {
			return err
		}
	}
	if p.Status.IsFinal() {
		// Idempotent: duplicate webhook.
		return nil
	}

	prevStatus := p.Status
	p.Status = event.Status
	now := time.Now()
	if event.Status == models.PaymentStatusPaid && p.PaidAt == nil {
		p.PaidAt = &now
	}
	if event.Status == models.PaymentStatusCancelled && p.CancelledAt == nil {
		p.CancelledAt = &now
	}
	if event.ProviderRef != "" {
		v := event.ProviderRef
		p.ProviderRef = &v
	}
	if len(event.RawPayload) > 0 {
		p.ProviderData = mergeProviderData(p.ProviderData, "webhook", event.RawPayload)
	}
	if err := s.paymentRepo.UpdatePayment(ctx, p); err != nil {
		return err
	}
	if prevStatus == p.Status {
		return nil
	}
	if s.notifier != nil {
		s.notifier.NotifyPaymentStatusChanged(p)
	}
	eventName := "payment." + strings.ToLower(string(p.Status))
	if p.Status == models.PaymentStatusPartialRefund {
		eventName = "payment.partial_refund"
	}
	go s.EnqueueCallback(context.Background(), p, eventName)
	return nil
}

// PaymentWebhookEvent carries the normalized webhook outcome for ApplyWebhook.
type PaymentWebhookEvent struct {
	Status      models.PaymentStatus
	ProviderRef string
	PaidAmount  int64
	RawPayload  json.RawMessage
}

// ----------------------------------------------------------------------------
// Internals
// ----------------------------------------------------------------------------

func (s *PaymentService) createPaymentWithGeneratedID(ctx context.Context, p *models.Payment) error {
	var lastErr error
	for i := 0; i < 5; i++ {
		p.PaymentID = newPaymentPublicID("PAY")
		lastErr = s.paymentRepo.CreatePayment(ctx, p)
		if lastErr == nil {
			return nil
		}
		if !isUniqueViolation(lastErr) {
			if isReferenceUniqueViolation(lastErr) {
				return newPaymentError(400, "DUPLICATE_REFERENCE_ID", "Reference ID already exists", lastErr)
			}
			return lastErr
		}
	}
	return lastErr
}

func (s *PaymentService) markFailed(ctx context.Context, p *models.Payment, code, message string) {
	p.Status = models.PaymentStatusFailed
	msg := code + ": " + message
	if p.Description == nil {
		p.Description = &msg
	}
	_ = s.paymentRepo.UpdatePayment(ctx, p)
	if s.notifier != nil {
		s.notifier.NotifyPaymentStatusChanged(p)
	}
	go s.EnqueueCallback(context.Background(), p, "payment.failed")
}

func (s *PaymentService) refreshStatus(ctx context.Context, p *models.Payment) (*models.Payment, error) {
	if p.Status.IsFinal() {
		return p, nil
	}
	provider, err := s.router.Get(p.Provider)
	if err != nil {
		return p, nil
	}
	result, err := provider.InquiryPayment(ctx, p)
	if err != nil {
		return p, err
	}
	if result == nil {
		return p, nil
	}
	if result.ProviderRef != "" {
		v := result.ProviderRef
		p.ProviderRef = &v
	}
	if len(result.RawResponse) > 0 {
		p.ProviderData = mergeProviderData(p.ProviderData, "inquiry", result.RawResponse)
	}
	if result.Status != "" && result.Status != p.Status {
		prev := p.Status
		p.Status = result.Status
		now := time.Now()
		if result.Status == models.PaymentStatusPaid && p.PaidAt == nil {
			p.PaidAt = &now
		}
		if result.Status == models.PaymentStatusCancelled && p.CancelledAt == nil {
			p.CancelledAt = &now
		}
		if err := s.paymentRepo.UpdatePayment(ctx, p); err != nil {
			return p, err
		}
		if s.notifier != nil {
			s.notifier.NotifyPaymentStatusChanged(p)
		}
		if prev != p.Status {
			eventName := "payment." + strings.ToLower(string(p.Status))
			if p.Status == models.PaymentStatusPartialRefund {
				eventName = "payment.partial_refund"
			}
			go s.EnqueueCallback(context.Background(), p, eventName)
		}
	} else if err := s.paymentRepo.UpdatePayment(ctx, p); err != nil {
		return p, err
	}
	return p, nil
}

// EnqueueCallback forwards to the callback service if configured.
func (s *PaymentService) EnqueueCallback(ctx context.Context, p *models.Payment, event string) {
	if s == nil || s.callbackSvc == nil {
		return
	}
	s.callbackSvc.EnqueueEvent(ctx, p, event)
}

func (s *PaymentService) buildResponse(p *models.Payment) *PaymentResponse {
	resp := &PaymentResponse{
		PaymentID:          p.PaymentID,
		ReferenceID:        p.ReferenceID,
		PaymentType:        string(p.PaymentType),
		PaymentCode:        p.PaymentCode,
		Provider:           string(p.Provider),
		Amount:             p.Amount,
		Fee:                p.Fee,
		TotalAmount:        p.TotalAmount,
		Status:             string(p.Status),
		PaymentDetail:      json.RawMessage(p.PaymentDetail),
		PaymentInstruction: json.RawMessage(p.PaymentInstruction),
		ExpiredAt:          formatPaymentTime(p.ExpiredAt),
		CreatedAt:          formatPaymentTime(p.CreatedAt),
	}
	if p.ProviderRef != nil {
		resp.ProviderRef = *p.ProviderRef
	}
	if p.CustomerName != nil {
		resp.CustomerName = *p.CustomerName
	}
	if p.CustomerEmail != nil {
		resp.CustomerEmail = *p.CustomerEmail
	}
	if p.CustomerPhone != nil {
		resp.CustomerPhone = *p.CustomerPhone
	}
	if p.Description != nil {
		resp.Description = *p.Description
	}
	if p.PaidAt != nil {
		resp.PaidAt = formatPaymentTime(*p.PaidAt)
	}
	if p.CancelledAt != nil {
		resp.CancelledAt = formatPaymentTime(*p.CancelledAt)
	}
	return resp
}

// ----------------------------------------------------------------------------
// Helpers
// ----------------------------------------------------------------------------

func resolveExpiredAt(requested string, maxDurationSec int) time.Time {
	now := time.Now()
	var maxExp time.Time
	if maxDurationSec > 0 {
		maxExp = now.Add(time.Duration(maxDurationSec) * time.Second)
	}
	if strings.TrimSpace(requested) != "" {
		if t, err := time.Parse(time.RFC3339, requested); err == nil {
			if t.After(now) {
				if !maxExp.IsZero() && t.After(maxExp) {
					return maxExp
				}
				return t
			}
		}
	}
	if !maxExp.IsZero() {
		return maxExp
	}
	return now.Add(24 * time.Hour)
}

func mergeProviderData(current models.NullableRawMessage, key string, payload json.RawMessage) models.NullableRawMessage {
	merged := map[string]json.RawMessage{}
	if len(current) > 0 {
		_ = json.Unmarshal(current, &merged)
	}
	if strings.TrimSpace(key) != "" && len(payload) > 0 {
		merged[key] = payload
	}
	if len(merged) == 0 {
		return nil
	}
	raw, err := json.Marshal(merged)
	if err != nil {
		return current
	}
	return models.NullableRawMessage(raw)
}

// renderInstruction takes the method's template JSON and substitutes runtime
// placeholders like {VA_NUMBER}, {PAYMENT_CODE}, {AMOUNT}.
func renderInstruction(method *models.PaymentMethod, norm PaymentDetailNormalized, amount int64) models.NullableRawMessage {
	if len(method.PaymentInstruction) == 0 {
		return nil
	}
	tmpl := string(method.PaymentInstruction)
	replace := map[string]string{
		"{VA_NUMBER}":    norm.VANumber,
		"{PAYMENT_CODE}": norm.PaymentCode,
		"{QR_STRING}":    norm.QRString,
		"{AMOUNT}":       fmt.Sprintf("%d", amount),
	}
	for k, v := range replace {
		tmpl = strings.ReplaceAll(tmpl, k, v)
	}
	return models.NullableRawMessage(tmpl)
}

func formatPaymentTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	wib := time.FixedZone("WIB", 7*3600)
	return t.In(wib).Format("2006-01-02T15:04:05+07:00")
}

func newPaymentPublicID(prefix string) string {
	now := time.Now().In(time.FixedZone("WIB", 7*3600))
	return fmt.Sprintf("%s-%s-%06d", prefix, now.Format("20060102"), randomDigits(6))
}
