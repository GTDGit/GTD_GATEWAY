package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/GTDGit/gtd_gateway/internal/models"
	"github.com/GTDGit/gtd_gateway/internal/repository"
)

// AdminPaymentService exposes admin-level operations on payments, methods,
// refunds, and callback logs. It wraps the PaymentRepository alongside the
// core PaymentService to delegate stateful transitions.
type AdminPaymentService struct {
	paymentRepo *repository.PaymentRepository
	clientRepo  *repository.ClientRepository
	paymentSvc  *PaymentService
	callbackSvc *PaymentCallbackService
}

func NewAdminPaymentService(
	paymentRepo *repository.PaymentRepository,
	clientRepo *repository.ClientRepository,
	paymentSvc *PaymentService,
	callbackSvc *PaymentCallbackService,
) *AdminPaymentService {
	return &AdminPaymentService{
		paymentRepo: paymentRepo,
		clientRepo:  clientRepo,
		paymentSvc:  paymentSvc,
		callbackSvc: callbackSvc,
	}
}

// ---------------------------------------------------------------------------
// Filter + pagination DTOs
// ---------------------------------------------------------------------------

type AdminListPaymentsRequest struct {
	Status      *string `form:"status"`
	Type        *string `form:"type"`
	Provider    *string `form:"provider"`
	ClientID    *int    `form:"clientId"`
	PaymentID   *string `form:"paymentId"`
	ReferenceID *string `form:"referenceId"`
	IsSandbox   *bool   `form:"isSandbox"`
	StartDate   *string `form:"startDate"`
	EndDate     *string `form:"endDate"`
	Search      *string `form:"search"`
	Page        int     `form:"page"`
	Limit       int     `form:"limit"`
}

type AdminPaymentView struct {
	ID                 int     `json:"id"`
	PaymentID          string  `json:"paymentId"`
	ReferenceID        string  `json:"referenceId"`
	ClientID           int     `json:"clientId"`
	PaymentMethodID    int     `json:"paymentMethodId"`
	IsSandbox          bool    `json:"isSandbox"`
	PaymentType        string  `json:"paymentType"`
	PaymentCode        string  `json:"paymentCode"`
	Provider           string  `json:"provider"`
	Amount             int64   `json:"amount"`
	Fee                int64   `json:"fee"`
	TotalAmount        int64   `json:"totalAmount"`
	Status             string  `json:"status"`
	CustomerName       *string `json:"customerName,omitempty"`
	CustomerEmail      *string `json:"customerEmail,omitempty"`
	CustomerPhone      *string `json:"customerPhone,omitempty"`
	ProviderRef        *string `json:"providerRef,omitempty"`
	PaymentDetail      any     `json:"paymentDetail,omitempty"`
	PaymentInstruction any     `json:"paymentInstruction,omitempty"`
	ProviderData       any     `json:"providerData,omitempty"`
	Metadata           any     `json:"metadata,omitempty"`
	Description        *string `json:"description,omitempty"`
	CallbackSent       bool    `json:"callbackSent"`
	CallbackAttempts   int     `json:"callbackAttempts"`
	ExpiredAt          string  `json:"expiredAt"`
	CreatedAt          string  `json:"createdAt"`
	PaidAt             *string `json:"paidAt,omitempty"`
	CancelledAt        *string `json:"cancelledAt,omitempty"`
	UpdatedAt          string  `json:"updatedAt"`
}

type AdminListPaymentsResponse struct {
	Payments   []AdminPaymentView `json:"payments"`
	Pagination PaginationMeta     `json:"pagination"`
}

type AdminRefundRequest struct {
	Amount int64  `json:"amount" binding:"required"`
	Reason string `json:"reason"`
}

type AdminUpdateMethodRequest struct {
	Provider           *string         `json:"provider"`
	FeeType            *string         `json:"feeType"`
	FeeFlat            *int            `json:"feeFlat"`
	FeePercent         *float64        `json:"feePercent"`
	FeeMin             *int            `json:"feeMin"`
	FeeMax             *int            `json:"feeMax"`
	MinAmount          *int            `json:"minAmount"`
	MaxAmount          *int            `json:"maxAmount"`
	ExpiredDuration    *int            `json:"expiredDuration"`
	LogoURL            *string         `json:"logoUrl"`
	DisplayOrder       *int            `json:"displayOrder"`
	PaymentInstruction json.RawMessage `json:"paymentInstruction"`
	IsActive           *bool           `json:"isActive"`
	IsMaintenance      *bool           `json:"isMaintenance"`
	MaintenanceMessage *string         `json:"maintenanceMessage"`
}

// ---------------------------------------------------------------------------
// Payments
// ---------------------------------------------------------------------------

func (s *AdminPaymentService) ListPayments(ctx context.Context, req AdminListPaymentsRequest) (*AdminListPaymentsResponse, error) {
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.Limit <= 0 || req.Limit > 200 {
		req.Limit = 20
	}
	filter := buildPaymentFilter(req)
	offset := (req.Page - 1) * req.Limit
	rows, total, err := s.paymentRepo.ListPayments(ctx, filter, req.Limit, offset)
	if err != nil {
		return nil, err
	}
	views := make([]AdminPaymentView, 0, len(rows))
	for i := range rows {
		views = append(views, paymentToAdminView(&rows[i]))
	}
	totalPages := 0
	if req.Limit > 0 {
		totalPages = (total + req.Limit - 1) / req.Limit
	}
	return &AdminListPaymentsResponse{
		Payments: views,
		Pagination: PaginationMeta{
			Page:       req.Page,
			Limit:      req.Limit,
			TotalItems: total,
			TotalPages: totalPages,
		},
	}, nil
}

func (s *AdminPaymentService) GetPayment(ctx context.Context, id int) (*AdminPaymentView, error) {
	p, err := s.paymentRepo.GetPaymentByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, newPaymentError(404, "PAYMENT_NOT_FOUND", "Payment not found", nil)
		}
		return nil, err
	}
	view := paymentToAdminView(p)
	return &view, nil
}

func (s *AdminPaymentService) GetPaymentLogs(ctx context.Context, paymentID int) ([]models.PaymentLog, error) {
	return s.paymentRepo.ListPaymentLogs(ctx, paymentID)
}

func (s *AdminPaymentService) GetPaymentCallbacks(ctx context.Context, paymentID int) ([]models.PaymentCallback, error) {
	p, err := s.paymentRepo.GetPaymentByID(ctx, paymentID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, newPaymentError(404, "PAYMENT_NOT_FOUND", "Payment not found", nil)
		}
		return nil, err
	}
	ref := ""
	if p.ProviderRef != nil {
		ref = *p.ProviderRef
	}
	if ref == "" {
		ref = p.PaymentID
	}
	return s.paymentRepo.ListPaymentCallbacksByProviderRef(ctx, p.Provider, ref)
}

func (s *AdminPaymentService) ListRefunds(ctx context.Context, paymentID int) ([]models.Refund, error) {
	return s.paymentRepo.ListRefundsByPaymentID(ctx, paymentID)
}

func (s *AdminPaymentService) ListCallbackLogs(ctx context.Context, paymentID int) ([]models.PaymentCallbackLog, error) {
	return s.paymentRepo.ListPaymentCallbackLogs(ctx, paymentID)
}

func (s *AdminPaymentService) Stats(ctx context.Context, req AdminListPaymentsRequest) (*repository.PaymentStats, error) {
	filter := buildPaymentFilter(req)
	return s.paymentRepo.Stats(ctx, filter)
}

// RetryCallback re-enqueues a pending callback log entry for immediate
// delivery. When logID is 0 the latest undelivered log for the payment is
// retried.
func (s *AdminPaymentService) RetryCallback(ctx context.Context, paymentID, logID int) error {
	if s.callbackSvc == nil {
		return newPaymentError(503, "CALLBACK_DISABLED", "Callback service not configured", nil)
	}
	logs, err := s.paymentRepo.ListPaymentCallbackLogs(ctx, paymentID)
	if err != nil {
		return err
	}
	var target *models.PaymentCallbackLog
	for i := range logs {
		row := &logs[i]
		if logID > 0 && row.ID != logID {
			continue
		}
		if row.IsDelivered {
			continue
		}
		target = row
		if logID > 0 {
			break
		}
	}
	if target == nil {
		return newPaymentError(404, "CALLBACK_NOT_FOUND", "No pending callback found to retry", nil)
	}
	client, err := s.clientRepo.GetByID(target.ClientID)
	if err != nil {
		return err
	}
	url, secret := client.EffectivePaymentCallback()
	if url == "" {
		return newPaymentError(400, "CALLBACK_URL_MISSING", "Client has no payment callback URL configured", nil)
	}
	now := time.Now()
	target.NextRetryAt = &now
	_ = s.paymentRepo.UpdatePaymentCallbackLog(ctx, target)
	s.callbackSvc.AttemptDelivery(ctx, target, url, secret)
	return nil
}

// AdminRefund bypasses client-scoping and delegates to PaymentService.RefundPayment.
func (s *AdminPaymentService) AdminRefund(ctx context.Context, paymentID int, req AdminRefundRequest) (*models.Refund, error) {
	p, err := s.paymentRepo.GetPaymentByID(ctx, paymentID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, newPaymentError(404, "PAYMENT_NOT_FOUND", "Payment not found", nil)
		}
		return nil, err
	}
	return s.paymentSvc.RefundPayment(ctx, p.PaymentID, 0, &CreateRefundRequest{
		Amount: req.Amount,
		Reason: req.Reason,
	})
}

// ---------------------------------------------------------------------------
// Methods
// ---------------------------------------------------------------------------

func (s *AdminPaymentService) ListMethods(ctx context.Context) ([]models.PaymentMethod, error) {
	return s.paymentRepo.ListAllMethods(ctx)
}

func (s *AdminPaymentService) UpdateMethod(ctx context.Context, id int, req AdminUpdateMethodRequest) (*models.PaymentMethod, error) {
	m, err := s.paymentRepo.GetMethodByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, newPaymentError(404, "PAYMENT_METHOD_NOT_FOUND", "Payment method not found", nil)
		}
		return nil, err
	}
	if req.Provider != nil {
		m.Provider = models.PaymentProvider(*req.Provider)
	}
	if req.FeeType != nil {
		m.FeeType = models.FeeType(*req.FeeType)
	}
	if req.FeeFlat != nil {
		m.FeeFlat = *req.FeeFlat
	}
	if req.FeePercent != nil {
		m.FeePercent = *req.FeePercent
	}
	if req.FeeMin != nil {
		m.FeeMin = *req.FeeMin
	}
	if req.FeeMax != nil {
		m.FeeMax = *req.FeeMax
	}
	if req.MinAmount != nil {
		m.MinAmount = *req.MinAmount
	}
	if req.MaxAmount != nil {
		m.MaxAmount = *req.MaxAmount
	}
	if req.ExpiredDuration != nil {
		m.ExpiredDuration = *req.ExpiredDuration
	}
	if req.LogoURL != nil {
		v := *req.LogoURL
		m.LogoURL = &v
	}
	if req.DisplayOrder != nil {
		m.DisplayOrder = *req.DisplayOrder
	}
	if len(req.PaymentInstruction) > 0 {
		if !json.Valid(req.PaymentInstruction) {
			return nil, newPaymentError(400, "INVALID_PAYMENT_INSTRUCTION", "paymentInstruction must be valid JSON", nil)
		}
		m.PaymentInstruction = models.NullableRawMessage(req.PaymentInstruction)
	}
	if req.IsActive != nil {
		m.IsActive = *req.IsActive
	}
	if req.IsMaintenance != nil {
		m.IsMaintenance = *req.IsMaintenance
	}
	if req.MaintenanceMessage != nil {
		v := *req.MaintenanceMessage
		m.MaintenanceMessage = &v
	}
	if err := s.paymentRepo.UpdateMethod(ctx, m); err != nil {
		return nil, err
	}
	return m, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func buildPaymentFilter(req AdminListPaymentsRequest) repository.PaymentFilter {
	f := repository.PaymentFilter{}
	if req.Status != nil {
		f.Status = *req.Status
	}
	if req.Type != nil {
		f.Type = *req.Type
	}
	if req.Provider != nil {
		f.Provider = *req.Provider
	}
	if req.ClientID != nil {
		f.ClientID = *req.ClientID
	}
	if req.PaymentID != nil {
		f.PaymentID = *req.PaymentID
	}
	if req.ReferenceID != nil {
		f.ReferenceID = *req.ReferenceID
	}
	if req.IsSandbox != nil {
		v := *req.IsSandbox
		f.IsSandbox = &v
	}
	if req.Search != nil {
		f.Search = *req.Search
	}
	if req.StartDate != nil {
		if t, err := time.Parse(time.RFC3339, *req.StartDate); err == nil {
			f.CreatedFrom = &t
		} else if t, err := time.Parse("2006-01-02", *req.StartDate); err == nil {
			f.CreatedFrom = &t
		}
	}
	if req.EndDate != nil {
		if t, err := time.Parse(time.RFC3339, *req.EndDate); err == nil {
			f.CreatedTo = &t
		} else if t, err := time.Parse("2006-01-02", *req.EndDate); err == nil {
			end := t.Add(24*time.Hour - time.Second)
			f.CreatedTo = &end
		}
	}
	return f
}

func paymentToAdminView(p *models.Payment) AdminPaymentView {
	view := AdminPaymentView{
		ID:               p.ID,
		PaymentID:        p.PaymentID,
		ReferenceID:      p.ReferenceID,
		ClientID:         p.ClientID,
		PaymentMethodID:  p.PaymentMethodID,
		IsSandbox:        p.IsSandbox,
		PaymentType:      string(p.PaymentType),
		PaymentCode:      p.PaymentCode,
		Provider:         string(p.Provider),
		Amount:           p.Amount,
		Fee:              p.Fee,
		TotalAmount:      p.TotalAmount,
		Status:           string(p.Status),
		CustomerName:     p.CustomerName,
		CustomerEmail:    p.CustomerEmail,
		CustomerPhone:    p.CustomerPhone,
		ProviderRef:      p.ProviderRef,
		Description:      p.Description,
		CallbackSent:     p.CallbackSent,
		CallbackAttempts: p.CallbackAttempts,
		ExpiredAt:        formatPaymentTime(p.ExpiredAt),
		CreatedAt:        formatPaymentTime(p.CreatedAt),
		UpdatedAt:        formatPaymentTime(p.UpdatedAt),
	}
	if p.PaidAt != nil {
		ts := formatPaymentTime(*p.PaidAt)
		view.PaidAt = &ts
	}
	if p.CancelledAt != nil {
		ts := formatPaymentTime(*p.CancelledAt)
		view.CancelledAt = &ts
	}
	view.PaymentDetail = rawToAny(p.PaymentDetail)
	view.PaymentInstruction = rawToAny(p.PaymentInstruction)
	view.ProviderData = rawToAny(p.ProviderData)
	view.Metadata = rawToAny(p.Metadata)
	return view
}

func rawToAny(raw models.NullableRawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw)
	}
	return v
}
