package service

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/GTDGit/gtd_gateway/internal/models"
	"github.com/GTDGit/gtd_gateway/internal/repository"
)

// AdminReconciliationService lists payment reconciliations and resolves them.
// Resolving applies a final status to the underlying payment and forwards the
// outcome to the client using the same callback mechanism as the normal flow.
type AdminReconciliationService struct {
	reconRepo   *repository.ReconciliationRepository
	paymentRepo *repository.PaymentRepository
	paymentSvc  *PaymentService
}

func NewAdminReconciliationService(
	reconRepo *repository.ReconciliationRepository,
	paymentRepo *repository.PaymentRepository,
	paymentSvc *PaymentService,
) *AdminReconciliationService {
	return &AdminReconciliationService{
		reconRepo:   reconRepo,
		paymentRepo: paymentRepo,
		paymentSvc:  paymentSvc,
	}
}

// ---------------------------------------------------------------------------
// DTOs
// ---------------------------------------------------------------------------

type AdminListReconciliationsRequest struct {
	Status   *string `form:"status"`
	Provider *string `form:"provider"`
	Reason   *string `form:"reason"`
	Search   *string `form:"search"`
	Page     int     `form:"page"`
	Limit    int     `form:"limit"`
}

type AdminReconciliationView struct {
	ID             int64   `json:"id"`
	PaymentID      string  `json:"paymentId"`
	Provider       string  `json:"provider"`
	Reason         string  `json:"reason"`
	WebhookStatus  *string `json:"webhookStatus,omitempty"`
	InquiryStatus  *string `json:"inquiryStatus,omitempty"`
	WebhookAmount  *int64  `json:"webhookAmount,omitempty"`
	InquiryAmount  *int64  `json:"inquiryAmount,omitempty"`
	ExpectedAmount *int64  `json:"expectedAmount,omitempty"`
	WebhookPayload any     `json:"webhookPayload,omitempty"`
	InquiryPayload any     `json:"inquiryPayload,omitempty"`
	Status         string  `json:"status"`
	ResolvedStatus *string `json:"resolvedStatus,omitempty"`
	ResolvedBy     *string `json:"resolvedBy,omitempty"`
	ResolutionNote *string `json:"resolutionNote,omitempty"`
	CreatedAt      string  `json:"createdAt"`
	UpdatedAt      string  `json:"updatedAt"`
	ResolvedAt     *string `json:"resolvedAt,omitempty"`
}

type AdminListReconciliationsResponse struct {
	Reconciliations []AdminReconciliationView `json:"reconciliations"`
	Pagination      PaginationMeta            `json:"pagination"`
}

// ---------------------------------------------------------------------------
// List / Get
// ---------------------------------------------------------------------------

func (s *AdminReconciliationService) List(ctx context.Context, req AdminListReconciliationsRequest) (*AdminListReconciliationsResponse, error) {
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.Limit <= 0 || req.Limit > 200 {
		req.Limit = 20
	}
	filter := repository.ReconciliationFilter{}
	if req.Status != nil {
		filter.Status = *req.Status
	}
	if req.Provider != nil {
		filter.Provider = *req.Provider
	}
	if req.Reason != nil {
		filter.Reason = *req.Reason
	}
	if req.Search != nil {
		filter.Search = *req.Search
	}
	offset := (req.Page - 1) * req.Limit
	rows, total, err := s.reconRepo.List(ctx, filter, req.Limit, offset)
	if err != nil {
		return nil, err
	}
	views := make([]AdminReconciliationView, 0, len(rows))
	for i := range rows {
		views = append(views, reconToView(&rows[i]))
	}
	totalPages := 0
	if req.Limit > 0 {
		totalPages = (total + req.Limit - 1) / req.Limit
	}
	return &AdminListReconciliationsResponse{
		Reconciliations: views,
		Pagination: PaginationMeta{
			Page:       req.Page,
			Limit:      req.Limit,
			TotalItems: total,
			TotalPages: totalPages,
		},
	}, nil
}

func (s *AdminReconciliationService) Get(ctx context.Context, id int64) (*AdminReconciliationView, error) {
	rec, err := s.reconRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, newPaymentError(404, "RECONCILIATION_NOT_FOUND", "Reconciliation not found", nil)
		}
		return nil, err
	}
	view := reconToView(rec)
	return &view, nil
}

// ---------------------------------------------------------------------------
// Resolve
// ---------------------------------------------------------------------------

// Resolve applies the operator-chosen final status to the payment, forwards the
// outcome to the client, and closes the reconciliation. Only open rows with a
// terminal target status are accepted.
func (s *AdminReconciliationService) Resolve(ctx context.Context, id int64, resolvedStatus, adminUser, note string) error {
	status := models.PaymentStatus(strings.TrimSpace(resolvedStatus))
	if !status.IsFinal() {
		return newPaymentError(400, "INVALID_STATUS", "resolvedStatus must be a final status (Success/Failed/Expired/Cancelled)", nil)
	}

	rec, err := s.reconRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return newPaymentError(404, "RECONCILIATION_NOT_FOUND", "Reconciliation not found", nil)
		}
		return err
	}
	if rec.Status != models.ReconStatusOpen {
		return newPaymentError(400, "ALREADY_RESOLVED", "Reconciliation is already resolved", nil)
	}

	payment, err := s.paymentRepo.GetByPaymentID(ctx, rec.PaymentID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return newPaymentError(404, "PAYMENT_NOT_FOUND", "Payment not found", nil)
		}
		return err
	}

	prevStatus := payment.Status
	payment.Status = status
	now := time.Now()
	if status == models.PaymentStatusSuccess && payment.PaidAt == nil {
		payment.PaidAt = &now
	}
	if status == models.PaymentStatusCancelled && payment.CancelledAt == nil {
		payment.CancelledAt = &now
	}
	if err := s.paymentRepo.UpdatePayment(ctx, payment); err != nil {
		return err
	}

	// Forward the resolved outcome to the client using the standard callback
	// path (only when the status actually changed, to avoid a spurious event).
	if prevStatus != payment.Status && s.paymentSvc != nil {
		if s.paymentSvc.notifier != nil {
			s.paymentSvc.notifier.NotifyPaymentStatusChanged(payment)
		}
		eventName := "payment." + strings.ToLower(string(payment.Status))
		s.paymentSvc.EnqueueCallback(ctx, payment, eventName)
	}

	if err := s.reconRepo.ResolveByID(ctx, id, string(status), adminUser, note); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return newPaymentError(409, "ALREADY_RESOLVED", "Reconciliation was resolved concurrently", nil)
		}
		return err
	}
	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func reconToView(r *models.PaymentReconciliation) AdminReconciliationView {
	view := AdminReconciliationView{
		ID:             r.ID,
		PaymentID:      r.PaymentID,
		Provider:       string(r.Provider),
		Reason:         string(r.Reason),
		WebhookStatus:  r.WebhookStatus,
		InquiryStatus:  r.InquiryStatus,
		WebhookAmount:  r.WebhookAmount,
		InquiryAmount:  r.InquiryAmount,
		ExpectedAmount: r.ExpectedAmount,
		Status:         string(r.Status),
		ResolvedStatus: r.ResolvedStatus,
		ResolvedBy:     r.ResolvedBy,
		ResolutionNote: r.ResolutionNote,
		CreatedAt:      formatPaymentTime(r.CreatedAt),
		UpdatedAt:      formatPaymentTime(r.UpdatedAt),
	}
	view.WebhookPayload = rawToAny(r.WebhookPayload)
	view.InquiryPayload = rawToAny(r.InquiryPayload)
	if r.ResolvedAt != nil {
		ts := formatPaymentTime(*r.ResolvedAt)
		view.ResolvedAt = &ts
	}
	return view
}
