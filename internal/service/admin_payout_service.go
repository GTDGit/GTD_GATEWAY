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

// AdminPayoutService exposes admin-level operations on disbursement payouts.
type AdminPayoutService struct {
	payoutRepo *repository.PayoutRepository
}

func NewAdminPayoutService(payoutRepo *repository.PayoutRepository) *AdminPayoutService {
	return &AdminPayoutService{payoutRepo: payoutRepo}
}

type AdminListPayoutsRequest struct {
	Status      *string `form:"status"`
	MethodType  *string `form:"methodType"`
	Provider    *string `form:"provider"`
	ChannelCode *string `form:"channelCode"`
	ClientID    *int    `form:"clientId"`
	IsSandbox   *bool   `form:"isSandbox"`
	StartDate   *string `form:"startDate"`
	EndDate     *string `form:"endDate"`
	Search      *string `form:"search"`
	Page        int     `form:"page"`
	Limit       int     `form:"limit"`
}

type AdminPayoutView struct {
	ID                  int     `json:"id"`
	PayoutID            string  `json:"payoutId"`
	ReferenceID         string  `json:"referenceId"`
	ClientID            int     `json:"clientId"`
	IsSandbox           bool    `json:"isSandbox"`
	MethodType          string  `json:"methodType"`
	ChannelCode         string  `json:"channelCode"`
	TransferType        *string `json:"transferType,omitempty"`
	Provider            string  `json:"provider"`
	BankCode            string  `json:"bankCode"`
	BankName            *string `json:"bankName,omitempty"`
	AccountNumber       string  `json:"accountNumber"`
	AccountName         *string `json:"accountName,omitempty"`
	SourceBankCode      *string `json:"sourceBankCode,omitempty"`
	SourceAccountNumber *string `json:"sourceAccountNumber,omitempty"`
	Amount              int64   `json:"amount"`
	Fee                 int64   `json:"fee"`
	SendAmount          int64   `json:"sendAmount"`
	TotalAmount         int64   `json:"totalAmount"`
	FeePaidBy           string  `json:"feePaidBy"`
	Status              string  `json:"status"`
	FailedReason        *string `json:"failedReason,omitempty"`
	FailedCode          *string `json:"failedCode,omitempty"`
	PurposeCode         *string `json:"purposeCode,omitempty"`
	Remark              *string `json:"remark,omitempty"`
	Description         *string `json:"description,omitempty"`
	CustomerName        *string `json:"customerName,omitempty"`
	CustomerEmail       *string `json:"customerEmail,omitempty"`
	CustomerPhone       *string `json:"customerPhone,omitempty"`
	ProviderRef         *string `json:"providerRef,omitempty"`
	ProviderData        any     `json:"providerData,omitempty"`
	CallbackURL         *string `json:"callbackUrl,omitempty"`
	CallbackSent        bool    `json:"callbackSent"`
	CallbackSentAt      *string `json:"callbackSentAt,omitempty"`
	CallbackAttempts    int     `json:"callbackAttempts"`
	CreatedAt           string  `json:"createdAt"`
	CompletedAt         *string `json:"completedAt,omitempty"`
	FailedAt            *string `json:"failedAt,omitempty"`
	UpdatedAt           string  `json:"updatedAt"`
}

type AdminListPayoutsResponse struct {
	Payouts    []AdminPayoutView `json:"payouts"`
	Pagination PaginationMeta    `json:"pagination"`
}

type AdminPayoutCallbackView struct {
	ID               int     `json:"id"`
	Provider         string  `json:"provider"`
	ProviderRef      *string `json:"providerRef,omitempty"`
	Signature        *string `json:"signature,omitempty"`
	IsValidSignature bool    `json:"isValidSignature"`
	PayoutID         *string `json:"payoutId,omitempty"`
	Status           *string `json:"status,omitempty"`
	IsProcessed      bool    `json:"isProcessed"`
	ProcessedAt      *string `json:"processedAt,omitempty"`
	ProcessError     *string `json:"processError,omitempty"`
	Payload          any     `json:"payload,omitempty"`
	CreatedAt        string  `json:"createdAt"`
}

// AdminPayoutRouteView is the admin-facing route shape (per method_type provider).
type AdminPayoutRouteView struct {
	ID                 int     `json:"id"`
	MethodType         string  `json:"methodType"`
	Provider           string  `json:"provider"`
	Priority           int     `json:"priority"`
	IsActive           bool    `json:"isActive"`
	IsMaintenance      bool    `json:"isMaintenance"`
	MaintenanceMessage *string `json:"maintenanceMessage,omitempty"`
	CreatedAt          string  `json:"createdAt"`
	UpdatedAt          string  `json:"updatedAt"`
}

// AdminUpdatePayoutRouteRequest carries editable route fields. Pointer fields
// are only applied when present.
type AdminUpdatePayoutRouteRequest struct {
	Priority           *int    `json:"priority"`
	IsActive           *bool   `json:"isActive"`
	IsMaintenance      *bool   `json:"isMaintenance"`
	MaintenanceMessage *string `json:"maintenanceMessage"`
}

func (s *AdminPayoutService) ListPayouts(ctx context.Context, req AdminListPayoutsRequest) (*AdminListPayoutsResponse, error) {
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.Limit <= 0 || req.Limit > 200 {
		req.Limit = 20
	}
	filter := buildPayoutFilter(req)
	offset := (req.Page - 1) * req.Limit
	rows, total, err := s.payoutRepo.ListPayouts(ctx, filter, req.Limit, offset)
	if err != nil {
		return nil, err
	}
	views := make([]AdminPayoutView, 0, len(rows))
	for i := range rows {
		views = append(views, payoutToAdminView(&rows[i]))
	}
	totalPages := 0
	if req.Limit > 0 {
		totalPages = (total + req.Limit - 1) / req.Limit
	}
	return &AdminListPayoutsResponse{
		Payouts: views,
		Pagination: PaginationMeta{
			Page:       req.Page,
			Limit:      req.Limit,
			TotalItems: total,
			TotalPages: totalPages,
		},
	}, nil
}

func (s *AdminPayoutService) Stats(ctx context.Context, req AdminListPayoutsRequest) (*repository.PayoutStats, error) {
	filter := buildPayoutFilter(req)
	return s.payoutRepo.Stats(ctx, filter)
}

func (s *AdminPayoutService) GetPayout(ctx context.Context, id int) (*AdminPayoutView, error) {
	p, err := s.payoutRepo.GetPayoutByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, newPaymentError(404, "PAYOUT_NOT_FOUND", "Payout not found", nil)
		}
		return nil, err
	}
	view := payoutToAdminView(p)
	return &view, nil
}

func (s *AdminPayoutService) ListCallbacks(ctx context.Context, id int) ([]AdminPayoutCallbackView, error) {
	p, err := s.payoutRepo.GetPayoutByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, newPaymentError(404, "PAYOUT_NOT_FOUND", "Payout not found", nil)
		}
		return nil, err
	}
	rows, err := s.payoutRepo.ListCallbacksByPayoutID(ctx, p.PayoutID)
	if err != nil {
		return nil, err
	}
	out := make([]AdminPayoutCallbackView, 0, len(rows))
	for i := range rows {
		out = append(out, payoutCallbackToView(&rows[i]))
	}
	return out, nil
}

// ListRoutes returns all payout routes grouped (ordered) by method_type/priority.
func (s *AdminPayoutService) ListRoutes(ctx context.Context) ([]AdminPayoutRouteView, error) {
	rows, err := s.payoutRepo.ListRoutes(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]AdminPayoutRouteView, 0, len(rows))
	for i := range rows {
		out = append(out, payoutRouteToView(&rows[i]))
	}
	return out, nil
}

// UpdateRoute applies the editable fields of a payout route.
func (s *AdminPayoutService) UpdateRoute(ctx context.Context, id int, req AdminUpdatePayoutRouteRequest) (*AdminPayoutRouteView, error) {
	route, err := s.payoutRepo.GetRouteByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, newPaymentError(404, "ROUTE_NOT_FOUND", "Payout route not found", nil)
		}
		return nil, err
	}
	if req.Priority != nil {
		route.Priority = *req.Priority
	}
	if req.IsActive != nil {
		route.IsActive = *req.IsActive
	}
	if req.IsMaintenance != nil {
		route.IsMaintenance = *req.IsMaintenance
	}
	if req.MaintenanceMessage != nil {
		msg := *req.MaintenanceMessage
		if msg == "" {
			route.MaintenanceMessage = nil
		} else {
			route.MaintenanceMessage = &msg
		}
	}
	if err := s.payoutRepo.UpdateRoute(ctx, route); err != nil {
		return nil, err
	}
	view := payoutRouteToView(route)
	return &view, nil
}

func buildPayoutFilter(req AdminListPayoutsRequest) repository.PayoutFilter {
	f := repository.PayoutFilter{}
	if req.Status != nil {
		f.Status = *req.Status
	}
	if req.MethodType != nil {
		f.MethodType = *req.MethodType
	}
	if req.Provider != nil {
		f.Provider = *req.Provider
	}
	if req.ChannelCode != nil {
		f.ChannelCode = *req.ChannelCode
	}
	if req.ClientID != nil {
		f.ClientID = *req.ClientID
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

func payoutToAdminView(p *models.Payout) AdminPayoutView {
	view := AdminPayoutView{
		ID:                  p.ID,
		PayoutID:            p.PayoutID,
		ReferenceID:         p.ReferenceID,
		ClientID:            p.ClientID,
		IsSandbox:           p.IsSandbox,
		MethodType:          string(p.MethodType),
		ChannelCode:         p.ChannelCode,
		Provider:            string(p.Provider),
		BankCode:            p.BankCode,
		BankName:            p.BankName,
		AccountNumber:       p.AccountNumber,
		AccountName:         p.AccountName,
		SourceBankCode:      p.SourceBankCode,
		SourceAccountNumber: p.SourceAccountNumber,
		Amount:              p.Amount,
		Fee:                 p.Fee,
		SendAmount:          p.SendAmount,
		TotalAmount:         p.TotalAmount,
		FeePaidBy:           string(p.FeePaidBy),
		Status:              string(p.Status),
		FailedReason:        p.FailedReason,
		FailedCode:          p.FailedCode,
		PurposeCode:         p.PurposeCode,
		Remark:              p.Remark,
		Description:         p.Description,
		CustomerName:        p.CustomerName,
		CustomerEmail:       p.CustomerEmail,
		CustomerPhone:       p.CustomerPhone,
		ProviderRef:         p.ProviderRef,
		CallbackURL:         p.CallbackURL,
		CallbackSent:        p.CallbackSent,
		CallbackAttempts:    p.CallbackAttempts,
		CreatedAt:           formatPaymentTime(p.CreatedAt),
		UpdatedAt:           formatPaymentTime(p.UpdatedAt),
	}
	if p.TransferType != nil {
		tt := string(*p.TransferType)
		view.TransferType = &tt
	}
	if p.CallbackSentAt != nil {
		ts := formatPaymentTime(*p.CallbackSentAt)
		view.CallbackSentAt = &ts
	}
	if p.CompletedAt != nil {
		ts := formatPaymentTime(*p.CompletedAt)
		view.CompletedAt = &ts
	}
	if p.FailedAt != nil {
		ts := formatPaymentTime(*p.FailedAt)
		view.FailedAt = &ts
	}
	view.ProviderData = payoutRawToAny(p.ProviderData)
	return view
}

func payoutCallbackToView(c *models.PayoutCallback) AdminPayoutCallbackView {
	view := AdminPayoutCallbackView{
		ID:               c.ID,
		Provider:         string(c.Provider),
		ProviderRef:      c.ProviderRef,
		Signature:        c.Signature,
		IsValidSignature: c.IsValidSignature,
		PayoutID:         c.PayoutID,
		Status:           c.Status,
		IsProcessed:      c.IsProcessed,
		ProcessError:     c.ProcessError,
		CreatedAt:        formatPaymentTime(c.CreatedAt),
	}
	if c.ProcessedAt != nil {
		ts := formatPaymentTime(*c.ProcessedAt)
		view.ProcessedAt = &ts
	}
	view.Payload = payoutRawToAny(c.Payload)
	return view
}

func payoutRouteToView(r *models.PayoutRoute) AdminPayoutRouteView {
	return AdminPayoutRouteView{
		ID:                 r.ID,
		MethodType:         string(r.MethodType),
		Provider:           string(r.Provider),
		Priority:           r.Priority,
		IsActive:           r.IsActive,
		IsMaintenance:      r.IsMaintenance,
		MaintenanceMessage: r.MaintenanceMessage,
		CreatedAt:          formatPaymentTime(r.CreatedAt),
		UpdatedAt:          formatPaymentTime(r.UpdatedAt),
	}
}

func payoutRawToAny(raw models.NullableRawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw)
	}
	return v
}
