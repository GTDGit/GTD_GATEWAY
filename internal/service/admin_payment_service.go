package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"strings"
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

// AdminMethodView is a canonical payment method plus its ordered provider
// bindings (the Method_Provider_Mapping rows, priority ASC).
type AdminMethodView struct {
	models.PaymentMethod
	Providers []models.MethodProviderBinding `json:"providers"`
}

// AdminListMethodsResponse wraps the method list, each with its provider bindings.
type AdminListMethodsResponse struct {
	Methods []AdminMethodView `json:"methods"`
}

// AdminBindingUpdate is one ordered binding update in the providers PUT body.
// The provider identifies which binding row to update for the method.
type AdminBindingUpdate struct {
	Provider           string  `json:"provider"`
	Priority           int     `json:"priority"`
	IsActive           bool    `json:"isActive"`
	IsMaintenance      bool    `json:"isMaintenance"`
	MaintenanceMessage *string `json:"maintenanceMessage,omitempty"`
}

// AdminUpdateBindingsRequest is the body for PUT .../providers — the ordered
// set of bindings to apply for a method.
type AdminUpdateBindingsRequest struct {
	Providers []AdminBindingUpdate `json:"providers"`
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
	payment, err := s.paymentRepo.GetPaymentByID(ctx, target.PaymentID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return newPaymentError(404, "PAYMENT_NOT_FOUND", "Payment not found", nil)
		}
		return err
	}
	url := ""
	if payment.CallbackURL != nil {
		url = strings.TrimSpace(*payment.CallbackURL)
	}
	if url == "" {
		return newPaymentError(400, "CALLBACK_URL_MISSING", "Payment has no callback URL", nil)
	}
	client, err := s.clientRepo.GetByID(target.ClientID)
	if err != nil {
		return err
	}
	now := time.Now()
	target.NextRetryAt = &now
	_ = s.paymentRepo.UpdatePaymentCallbackLog(ctx, target)
	s.callbackSvc.AttemptDelivery(ctx, target, url, client.CallbackSecret)
	return nil
}

// ---------------------------------------------------------------------------
// Methods
// ---------------------------------------------------------------------------

func (s *AdminPaymentService) ListMethods(ctx context.Context) (*AdminListMethodsResponse, error) {
	methods, err := s.paymentRepo.ListAllMethods(ctx)
	if err != nil {
		return nil, err
	}
	views := make([]AdminMethodView, 0, len(methods))
	for i := range methods {
		m := methods[i]
		bindings, err := s.paymentRepo.GetMethodProvidersByTypeCode(ctx, m.Type, m.Code)
		if err != nil {
			return nil, err
		}
		views = append(views, AdminMethodView{PaymentMethod: m, Providers: bindings})
	}
	return &AdminListMethodsResponse{Methods: views}, nil
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

// ListProviders returns the provider bindings for the method identified by
// (type, code), ordered by priority ASC.
func (s *AdminPaymentService) ListProviders(ctx context.Context, paymentType, code string) ([]models.MethodProviderBinding, error) {
	t, c, err := normalizeMethodKey(paymentType, code)
	if err != nil {
		return nil, err
	}
	bindings, err := s.paymentRepo.GetMethodProvidersByTypeCode(ctx, t, c)
	if err != nil {
		return nil, err
	}
	if len(bindings) == 0 {
		// Distinguish a missing method from a method with no bindings.
		if _, mErr := s.paymentRepo.GetMethodByTypeCode(ctx, t, c); mErr != nil {
			if errors.Is(mErr, sql.ErrNoRows) {
				return nil, newPaymentError(404, "PAYMENT_METHOD_NOT_FOUND", "Payment method not found", nil)
			}
			return nil, mErr
		}
	}
	return bindings, nil
}

// UpdateProviders applies the ordered binding updates (priority, is_active,
// is_maintenance, maintenance_message) for the method identified by
// (type, code) and returns the refreshed, priority-ordered bindings.
func (s *AdminPaymentService) UpdateProviders(ctx context.Context, paymentType, code string, req AdminUpdateBindingsRequest) ([]models.MethodProviderBinding, error) {
	t, c, err := normalizeMethodKey(paymentType, code)
	if err != nil {
		return nil, err
	}
	existing, err := s.paymentRepo.GetMethodProvidersByTypeCode(ctx, t, c)
	if err != nil {
		return nil, err
	}
	if len(existing) == 0 {
		if _, mErr := s.paymentRepo.GetMethodByTypeCode(ctx, t, c); mErr != nil {
			if errors.Is(mErr, sql.ErrNoRows) {
				return nil, newPaymentError(404, "PAYMENT_METHOD_NOT_FOUND", "Payment method not found", nil)
			}
			return nil, mErr
		}
	}

	// Index existing bindings by provider for lookup.
	byProvider := make(map[models.PaymentProvider]*models.MethodProviderBinding, len(existing))
	for i := range existing {
		byProvider[existing[i].Provider] = &existing[i]
	}

	for _, u := range req.Providers {
		prov := models.PaymentProvider(strings.TrimSpace(u.Provider))
		binding, ok := byProvider[prov]
		if !ok {
			return nil, newPaymentError(400, "INVALID_PROVIDER_BINDING",
				"provider '"+u.Provider+"' is not bound to this payment method", nil)
		}
		binding.Priority = u.Priority
		binding.IsActive = u.IsActive
		binding.IsMaintenance = u.IsMaintenance
		if u.MaintenanceMessage != nil {
			v := *u.MaintenanceMessage
			binding.MaintenanceMessage = &v
		} else {
			binding.MaintenanceMessage = nil
		}
		if err := s.paymentRepo.UpdateMethodProviderBinding(ctx, binding); err != nil {
			return nil, err
		}
	}

	// Return the updated bindings ordered by priority ASC.
	sort.SliceStable(existing, func(i, j int) bool {
		if existing[i].Priority != existing[j].Priority {
			return existing[i].Priority < existing[j].Priority
		}
		return existing[i].ID < existing[j].ID
	})
	return existing, nil
}

// normalizeMethodKey upper-cases the type, trims the code, and validates the
// payment type against the known set.
func normalizeMethodKey(paymentType, code string) (models.PaymentType, string, error) {
	t := models.PaymentType(strings.ToUpper(strings.TrimSpace(paymentType)))
	c := strings.TrimSpace(code)
	switch t {
	case models.PaymentTypeVA, models.PaymentTypeEwallet, models.PaymentTypeQRIS, models.PaymentTypeRetail:
	default:
		return "", "", newPaymentError(400, "INVALID_PARAM", "Unknown payment method type: "+paymentType, nil)
	}
	if c == "" {
		return "", "", newPaymentError(400, "MISSING_FIELD", "payment method code is required", nil)
	}
	return t, c, nil
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
