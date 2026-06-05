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

// AdminTransferService exposes admin-level operations on disbursement transfers.
type AdminTransferService struct {
	transferRepo *repository.TransferRepository
}

func NewAdminTransferService(transferRepo *repository.TransferRepository) *AdminTransferService {
	return &AdminTransferService{transferRepo: transferRepo}
}

type AdminListTransfersRequest struct {
	Status    *string `form:"status"`
	Type      *string `form:"type"`
	Provider  *string `form:"provider"`
	BankCode  *string `form:"bankCode"`
	ClientID  *int    `form:"clientId"`
	IsSandbox *bool   `form:"isSandbox"`
	StartDate *string `form:"startDate"`
	EndDate   *string `form:"endDate"`
	Search    *string `form:"search"`
	Page      int     `form:"page"`
	Limit     int     `form:"limit"`
}

type AdminTransferView struct {
	ID                  int     `json:"id"`
	TransferID          string  `json:"transferId"`
	ReferenceID         string  `json:"referenceId"`
	ClientID            int     `json:"clientId"`
	IsSandbox           bool    `json:"isSandbox"`
	TransferType        string  `json:"transferType"`
	Provider            string  `json:"provider"`
	BankCode            string  `json:"bankCode"`
	BankName            *string `json:"bankName,omitempty"`
	AccountNumber       string  `json:"accountNumber"`
	AccountName         *string `json:"accountName,omitempty"`
	SourceBankCode      string  `json:"sourceBankCode"`
	SourceAccountNumber string  `json:"sourceAccountNumber"`
	Amount              int64   `json:"amount"`
	Fee                 int64   `json:"fee"`
	TotalAmount         int64   `json:"totalAmount"`
	Status              string  `json:"status"`
	FailedReason        *string `json:"failedReason,omitempty"`
	FailedCode          *string `json:"failedCode,omitempty"`
	PurposeCode         *string `json:"purposeCode,omitempty"`
	Remark              *string `json:"remark,omitempty"`
	ProviderRef         *string `json:"providerRef,omitempty"`
	ProviderData        any     `json:"providerData,omitempty"`
	CallbackSent        bool    `json:"callbackSent"`
	CallbackSentAt      *string `json:"callbackSentAt,omitempty"`
	CreatedAt           string  `json:"createdAt"`
	CompletedAt         *string `json:"completedAt,omitempty"`
	FailedAt            *string `json:"failedAt,omitempty"`
	UpdatedAt           string  `json:"updatedAt"`
}

type AdminListTransfersResponse struct {
	Transfers  []AdminTransferView `json:"transfers"`
	Pagination PaginationMeta      `json:"pagination"`
}

type AdminTransferCallbackView struct {
	ID               int     `json:"id"`
	Provider         string  `json:"provider"`
	ProviderRef      *string `json:"providerRef,omitempty"`
	Signature        *string `json:"signature,omitempty"`
	IsValidSignature bool    `json:"isValidSignature"`
	TransferID       *string `json:"transferId,omitempty"`
	Status           *string `json:"status,omitempty"`
	IsProcessed      bool    `json:"isProcessed"`
	ProcessedAt      *string `json:"processedAt,omitempty"`
	ProcessError     *string `json:"processError,omitempty"`
	Payload          any     `json:"payload,omitempty"`
	CreatedAt        string  `json:"createdAt"`
}

func (s *AdminTransferService) ListTransfers(ctx context.Context, req AdminListTransfersRequest) (*AdminListTransfersResponse, error) {
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.Limit <= 0 || req.Limit > 200 {
		req.Limit = 20
	}
	filter := buildTransferFilter(req)
	offset := (req.Page - 1) * req.Limit
	rows, total, err := s.transferRepo.ListTransfers(ctx, filter, req.Limit, offset)
	if err != nil {
		return nil, err
	}
	views := make([]AdminTransferView, 0, len(rows))
	for i := range rows {
		views = append(views, transferToAdminView(&rows[i]))
	}
	totalPages := 0
	if req.Limit > 0 {
		totalPages = (total + req.Limit - 1) / req.Limit
	}
	return &AdminListTransfersResponse{
		Transfers: views,
		Pagination: PaginationMeta{
			Page:       req.Page,
			Limit:      req.Limit,
			TotalItems: total,
			TotalPages: totalPages,
		},
	}, nil
}

func (s *AdminTransferService) Stats(ctx context.Context, req AdminListTransfersRequest) (*repository.TransferStats, error) {
	filter := buildTransferFilter(req)
	return s.transferRepo.Stats(ctx, filter)
}

func (s *AdminTransferService) GetTransfer(ctx context.Context, id int) (*AdminTransferView, error) {
	t, err := s.transferRepo.GetTransferByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, newPaymentError(404, "TRANSFER_NOT_FOUND", "Transfer not found", nil)
		}
		return nil, err
	}
	view := transferToAdminView(t)
	return &view, nil
}

func (s *AdminTransferService) ListCallbacks(ctx context.Context, id int) ([]AdminTransferCallbackView, error) {
	t, err := s.transferRepo.GetTransferByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, newPaymentError(404, "TRANSFER_NOT_FOUND", "Transfer not found", nil)
		}
		return nil, err
	}
	rows, err := s.transferRepo.ListCallbacksByTransferID(ctx, t.TransferID)
	if err != nil {
		return nil, err
	}
	out := make([]AdminTransferCallbackView, 0, len(rows))
	for i := range rows {
		out = append(out, transferCallbackToView(&rows[i]))
	}
	return out, nil
}

func buildTransferFilter(req AdminListTransfersRequest) repository.TransferFilter {
	f := repository.TransferFilter{}
	if req.Status != nil {
		f.Status = *req.Status
	}
	if req.Type != nil {
		f.Type = *req.Type
	}
	if req.Provider != nil {
		f.Provider = *req.Provider
	}
	if req.BankCode != nil {
		f.BankCode = *req.BankCode
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

func transferToAdminView(t *models.Transfer) AdminTransferView {
	view := AdminTransferView{
		ID:                  t.ID,
		TransferID:          t.TransferID,
		ReferenceID:         t.ReferenceID,
		ClientID:            t.ClientID,
		IsSandbox:           t.IsSandbox,
		TransferType:        string(t.TransferType),
		Provider:            string(t.Provider),
		BankCode:            t.BankCode,
		BankName:            t.BankName,
		AccountNumber:       t.AccountNumber,
		AccountName:         t.AccountName,
		SourceBankCode:      t.SourceBankCode,
		SourceAccountNumber: t.SourceAccountNumber,
		Amount:              t.Amount,
		Fee:                 t.Fee,
		TotalAmount:         t.TotalAmount,
		Status:              string(t.Status),
		FailedReason:        t.FailedReason,
		FailedCode:          t.FailedCode,
		PurposeCode:         t.PurposeCode,
		Remark:              t.Remark,
		ProviderRef:         t.ProviderRef,
		CallbackSent:        t.CallbackSent,
		CreatedAt:           formatPaymentTime(t.CreatedAt),
		UpdatedAt:           formatPaymentTime(t.UpdatedAt),
	}
	if t.CallbackSentAt != nil {
		ts := formatPaymentTime(*t.CallbackSentAt)
		view.CallbackSentAt = &ts
	}
	if t.CompletedAt != nil {
		ts := formatPaymentTime(*t.CompletedAt)
		view.CompletedAt = &ts
	}
	if t.FailedAt != nil {
		ts := formatPaymentTime(*t.FailedAt)
		view.FailedAt = &ts
	}
	view.ProviderData = transferRawToAny(t.ProviderData)
	return view
}

func transferCallbackToView(c *models.TransferCallback) AdminTransferCallbackView {
	view := AdminTransferCallbackView{
		ID:               c.ID,
		Provider:         string(c.Provider),
		ProviderRef:      c.ProviderRef,
		Signature:        c.Signature,
		IsValidSignature: c.IsValidSignature,
		TransferID:       c.TransferID,
		Status:           c.Status,
		IsProcessed:      c.IsProcessed,
		ProcessError:     c.ProcessError,
		CreatedAt:        formatPaymentTime(c.CreatedAt),
	}
	if c.ProcessedAt != nil {
		ts := formatPaymentTime(*c.ProcessedAt)
		view.ProcessedAt = &ts
	}
	view.Payload = transferRawToAny(c.Payload)
	return view
}

func transferRawToAny(raw models.NullableRawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw)
	}
	return v
}
