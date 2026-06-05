package service

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/lib/pq"
	"github.com/rs/zerolog/log"

	"github.com/GTDGit/gtd_gateway/internal/models"
	"github.com/GTDGit/gtd_gateway/internal/repository"
	"github.com/GTDGit/gtd_gateway/pkg/bnc"
)

const (
	briSourceBankCode               = "002"
	bncSourceBankCode               = "490"
	disbursementInquiryExpiry       = 30 * time.Minute
	disbursementMinAmount     int64 = 10000
	disbursementInterbankFee  int64 = 2500
	transferStatusLookback          = 30 * 24 * time.Hour
)

type snapTransferClient interface {
	ExternalAccountInquiry(ctx context.Context, bankCode, accountNo string) (*bnc.AccountInquiryResponse, error)
	InternalAccountInquiry(ctx context.Context, accountNo string) (*bnc.AccountInquiryResponse, error)
	InterbankTransfer(ctx context.Context, req bnc.TransferRequest) (*bnc.TransferResponse, error)
	IntrabankTransfer(ctx context.Context, req bnc.TransferRequest) (*bnc.TransferResponse, error)
	TransferStatus(ctx context.Context, req bnc.TransferStatusRequest) (*bnc.TransferStatusResponse, error)
}

var transferPurposeDescriptions = map[string]string{
	"01": "Investasi",
	"02": "Pemindahan Dana",
	"03": "Pembelian",
	"99": "Lainnya",
}

type TransferServiceError struct {
	HTTPStatus int
	Code       string
	Message    string
	Err        error
}

func (e *TransferServiceError) Error() string {
	if e == nil {
		return ""
	}
	if e.Err == nil {
		return e.Code + ": " + e.Message
	}
	return e.Code + ": " + e.Message + ": " + e.Err.Error()
}

func (e *TransferServiceError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

type CreateTransferInquiryRequest struct {
	BankCode      string `json:"bankCode"`
	AccountNumber string `json:"accountNumber"`
}

type CreateTransferRequest struct {
	ReferenceID   string `json:"referenceId"`
	InquiryID     string `json:"inquiryId"`
	BankCode      string `json:"bankCode"`
	AccountNumber string `json:"accountNumber"`
	AccountName   string `json:"accountName"`
	Amount        int64  `json:"amount"`
	Purpose       string `json:"purpose"`
	Remark        string `json:"remark"`
}

type TransferService struct {
	transferRepo           *repository.TransferRepository
	bankRepo               *repository.BankCodeRepository
	bncClient              snapTransferClient
	briClient              snapTransferClient
	pakailinkClient        snapTransferClient
	callbackSvc            *TransferCallbackService
	bncSourceAccount       string
	briSourceAccount       string
	pakailinkSourceLabel   string
}

func NewTransferService(
	transferRepo *repository.TransferRepository,
	bankRepo *repository.BankCodeRepository,
	bncClient snapTransferClient,
	briClient snapTransferClient,
	callbackSvc *TransferCallbackService,
	bncSourceAccount string,
	briSourceAccount string,
) *TransferService {
	return &TransferService{
		transferRepo:     transferRepo,
		bankRepo:         bankRepo,
		bncClient:        bncClient,
		briClient:        briClient,
		callbackSvc:      callbackSvc,
		bncSourceAccount: strings.TrimSpace(bncSourceAccount),
		briSourceAccount: strings.TrimSpace(briSourceAccount),
	}
}

// SetPakailinkClient registers a PakaiLink disbursement adapter. When set, the
// service routes ALL bank transfers via PakaiLink in preference to BNC/BRI.
// sourceLabel is the merchant-facing source identifier persisted on transfers
// (PakaiLink does not expose a bank account; we use the partner ID/label).
func (s *TransferService) SetPakailinkClient(client snapTransferClient, sourceLabel string) {
	s.pakailinkClient = client
	s.pakailinkSourceLabel = strings.TrimSpace(sourceLabel)
}

func (s *TransferService) Available() bool {
	if s == nil {
		return false
	}
	if s.pakailinkClient != nil {
		return true
	}
	return (s.bncClient != nil && s.bncSourceAccount != "") || (s.briClient != nil && s.briSourceAccount != "")
}

func (s *TransferService) Inquiry(
	ctx context.Context,
	req *CreateTransferInquiryRequest,
	client *models.Client,
	isSandbox bool,
) (*models.TransferInquiryResponse, error) {
	if !s.Available() {
		return nil, newTransferError(503, "DISBURSEMENT_UNAVAILABLE", "Disbursement provider is not configured", nil)
	}
	if client == nil {
		return nil, newTransferError(401, "INVALID_TOKEN", "Unauthorized", nil)
	}
	if req == nil {
		return nil, newTransferError(400, "MISSING_FIELD", "Invalid request body", nil)
	}

	bank, transferType, provider, err := s.validateInquiryRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	var (
		providerClient snapTransferClient
		inquiryResp *bnc.AccountInquiryResponse
		rawResp     json.RawMessage
	)
	providerClient, err = s.clientForProvider(provider)
	if err != nil {
		return nil, err
	}

	switch transferType {
	case models.TransferTypeIntrabank:
		inquiryResp, err = providerClient.InternalAccountInquiry(ctx, req.AccountNumber)
	default:
		inquiryResp, err = providerClient.ExternalAccountInquiry(ctx, bank.Code, req.AccountNumber)
	}
	if err != nil {
		return nil, mapInquiryError(err)
	}
	rawResp = inquiryResp.RawResponse

	accountName := strings.TrimSpace(inquiryResp.BeneficiaryAccountName)
	bankName := strings.TrimSpace(bank.Name)
	if inquiryResp.BeneficiaryBankName != "" {
		bankName = strings.TrimSpace(inquiryResp.BeneficiaryBankName)
	}

	inquiry := &models.TransferInquiry{
		ClientID:      client.ID,
		IsSandbox:     isSandbox,
		BankCode:      bank.Code,
		BankName:      stringPtr(bankName),
		AccountNumber: req.AccountNumber,
		AccountName:   stringPtr(accountName),
		TransferType:  transferType,
		Provider:      provider,
		ProviderRef:   stringPtr(strings.TrimSpace(inquiryResp.ReferenceNo)),
		ProviderData:  mergeTransferProviderData(nil, "inquiry", rawResp),
		ExpiredAt:     time.Now().Add(disbursementInquiryExpiry),
	}

	if err := s.createInquiryWithGeneratedID(ctx, inquiry); err != nil {
		return nil, err
	}

	return &models.TransferInquiryResponse{
		BankCode:      bank.Code,
		BankShortName: bank.ShortName,
		BankName:      bankName,
		AccountNumber: req.AccountNumber,
		AccountName:   accountName,
		TransferType:  string(transferType),
		InquiryID:     inquiry.InquiryID,
		ExpiredAt:     formatTransferTime(inquiry.ExpiredAt),
	}, nil
}

func (s *TransferService) Execute(
	ctx context.Context,
	req *CreateTransferRequest,
	client *models.Client,
	isSandbox bool,
) (*models.TransferResponse, error) {
	if !s.Available() {
		return nil, newTransferError(503, "DISBURSEMENT_UNAVAILABLE", "Disbursement provider is not configured", nil)
	}
	if client == nil {
		return nil, newTransferError(401, "INVALID_TOKEN", "Unauthorized", nil)
	}
	if req == nil {
		return nil, newTransferError(400, "MISSING_FIELD", "Invalid request body", nil)
	}

	if err := s.validateTransferRequest(req); err != nil {
		return nil, err
	}

	if existing, err := s.transferRepo.GetTransferByReferenceID(ctx, client.ID, req.ReferenceID); err == nil && existing != nil {
		return nil, newTransferError(400, "DUPLICATE_REFERENCE_ID", "Reference ID already exists", nil)
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	inquiry, err := s.transferRepo.GetInquiryByInquiryID(ctx, req.InquiryID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, newTransferError(400, "INQUIRY_NOT_FOUND", "Inquiry ID not found", nil)
		}
		return nil, err
	}

	if inquiry.ClientID != client.ID {
		return nil, newTransferError(400, "INQUIRY_NOT_FOUND", "Inquiry ID not found", nil)
	}
	if time.Now().After(inquiry.ExpiredAt) {
		return nil, newTransferError(400, "INQUIRY_EXPIRED", "Inquiry has expired", nil)
	}
	if inquiry.BankCode != req.BankCode ||
		inquiry.AccountNumber != req.AccountNumber ||
		normalizeComparableString(derefString(inquiry.AccountName)) != normalizeComparableString(req.AccountName) {
		return nil, newTransferError(400, "ACCOUNT_MISMATCH", "Account data does not match inquiry", nil)
	}
	if inquiry.TransferType == models.TransferTypeInterbank && req.Purpose == "" {
		return nil, newTransferError(400, "PURPOSE_REQUIRED", "Purpose is required for interbank transfer", nil)
	}
	if req.Purpose != "" && !isValidTransferPurpose(req.Purpose) {
		return nil, newTransferError(400, "INVALID_PURPOSE", "Invalid purpose code", nil)
	}

	bank, err := s.bankRepo.GetByCode(ctx, req.BankCode)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, newTransferError(400, "INVALID_BANK_CODE", "Invalid bank code", nil)
		}
		return nil, err
	}

	fee := int64(0)
	if inquiry.TransferType == models.TransferTypeInterbank {
		fee = disbursementInterbankFee
	}

	sourceBankCode, sourceAccount, err := s.sourceAccountForProvider(inquiry.Provider)
	if err != nil {
		return nil, err
	}

	transfer := &models.Transfer{
		ReferenceID:         req.ReferenceID,
		ClientID:            client.ID,
		IsSandbox:           isSandbox,
		TransferType:        inquiry.TransferType,
		Provider:            inquiry.Provider,
		BankCode:            bank.Code,
		BankName:            stringPtr(bank.Name),
		AccountNumber:       req.AccountNumber,
		AccountName:         stringPtr(strings.TrimSpace(req.AccountName)),
		SourceBankCode:      sourceBankCode,
		SourceAccountNumber: sourceAccount,
		Amount:              req.Amount,
		Fee:                 fee,
		TotalAmount:         req.Amount + fee,
		Status:              models.TransferStatusProcessing,
		PurposeCode:         stringPtr(strings.TrimSpace(req.Purpose)),
		Remark:              stringPtr(strings.TrimSpace(req.Remark)),
		InquiryRowID:        intPtr(inquiry.ID),
		CallbackSent:        false,
	}

	if err := s.createTransferWithGeneratedID(ctx, transfer); err != nil {
		return nil, err
	}

	providerReq := bnc.TransferRequest{
		PartnerReferenceNo:     transfer.TransferID,
		Amount:                 transfer.Amount,
		BeneficiaryAccountNo:   transfer.AccountNumber,
		BeneficiaryBankCode:    transfer.BankCode,
		BeneficiaryAccountName: derefString(transfer.AccountName),
		Remark:                 strings.TrimSpace(req.Remark),
		PurposeCode:            strings.TrimSpace(req.Purpose),
		TransactionDate:        transfer.CreatedAt,
	}

	providerClient, err := s.clientForProvider(inquiry.Provider)
	if err != nil {
		return nil, err
	}

	var providerResp *bnc.TransferResponse
	switch inquiry.TransferType {
	case models.TransferTypeIntrabank:
		providerResp, err = providerClient.IntrabankTransfer(ctx, providerReq)
	default:
		providerResp, err = providerClient.InterbankTransfer(ctx, providerReq)
	}
	if err != nil {
		if isUncertainTransferError(err) {
			log.Warn().
				Err(err).
				Str("transfer_id", transfer.TransferID).
				Str("reference_id", transfer.ReferenceID).
				Msg("transfer submission uncertain, keeping transfer pending")

			transfer.Status = models.TransferStatusPending
			transfer.ProviderData = mergeTransferProviderData(transfer.ProviderData, "submit_error", transferErrorPayload(err))
			if updateErr := s.transferRepo.UpdateTransfer(ctx, transfer); updateErr != nil {
				return nil, updateErr
			}
			return s.buildTransferResponse(ctx, transfer, bank)
		}

		appErr := mapTransferSubmitError(err)
		now := time.Now()
		transfer.Status = models.TransferStatusFailed
		transfer.FailedReason = stringPtr(appErr.Message)
		transfer.FailedCode = stringPtr(providerErrorCode(err))
		transfer.FailedAt = &now
		transfer.CallbackSent = true
		transfer.ProviderData = mergeTransferProviderData(transfer.ProviderData, "submit_error", transferErrorPayload(err))
		if updateErr := s.transferRepo.UpdateTransfer(ctx, transfer); updateErr != nil {
			return nil, updateErr
		}
		return nil, appErr
	}

	transfer.ProviderRef = stringPtr(strings.TrimSpace(providerResp.ReferenceNo))
	transfer.ProviderData = mergeTransferProviderData(transfer.ProviderData, "submit", providerResp.RawResponse)
	transfer.Status = models.TransferStatusProcessing
	transfer.FailedReason = nil
	transfer.FailedCode = nil
	transfer.FailedAt = nil

	if err := s.transferRepo.UpdateTransfer(ctx, transfer); err != nil {
		return nil, err
	}

	return s.buildTransferResponse(ctx, transfer, bank)
}

func (s *TransferService) GetTransfer(ctx context.Context, transferID string, clientID int) (*models.TransferResponse, error) {
	transfer, err := s.transferRepo.GetTransferByTransferID(ctx, strings.TrimSpace(transferID))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, newTransferError(404, "TRANSFER_NOT_FOUND", "Transfer not found", nil)
		}
		return nil, err
	}
	if transfer.ClientID != clientID {
		return nil, newTransferError(404, "TRANSFER_NOT_FOUND", "Transfer not found", nil)
	}

	if s.Available() &&
		(transfer.Status == models.TransferStatusProcessing || transfer.Status == models.TransferStatusPending) &&
		time.Since(transfer.UpdatedAt) >= 15*time.Second {
		if _, err := s.refreshTransferStatus(ctx, transfer, 0); err != nil {
			log.Warn().Err(err).Str("transfer_id", transfer.TransferID).Msg("failed to refresh transfer status on read")
		}
	}

	var bank *models.BankCode
	bank, err = s.bankRepo.GetByCode(ctx, transfer.BankCode)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	return s.buildTransferResponse(ctx, transfer, bank)
}

func (s *TransferService) ProcessPendingTransfers(ctx context.Context, staleAfter, maxAge time.Duration, limit int) error {
	if !s.Available() {
		return nil
	}
	if limit <= 0 {
		limit = 50
	}

	updatedBefore := time.Now().Add(-staleAfter)
	createdAfter := time.Now().Add(-transferStatusLookback)
	transfers, err := s.transferRepo.ListTransfersForStatusCheck(ctx, updatedBefore, createdAfter, limit)
	if err != nil {
		return err
	}

	for i := range transfers {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if _, err := s.refreshTransferStatus(ctx, &transfers[i], maxAge); err != nil {
			log.Warn().Err(err).Str("transfer_id", transfers[i].TransferID).Msg("failed to reconcile transfer status")
		}
	}

	return nil
}

// PakailinkCallbackEvent is the parsed Service 44 callback delivered by
// PakaiLink for previously-pending transfers.
type PakailinkCallbackEvent struct {
	PaymentFlagStatus string // "00" success, "06" failed
	PartnerReference  string // maps to transfers.transfer_id
	ReferenceNo       string
	AccountNumber     string
	AccountName       string
	PaidAmount        int64
	FeeAmount         int64
	RawPayload        []byte
}

// ApplyPakailinkCallback updates the transfer state from a Service 44 callback.
// Returns the matched transfer (or nil) so the handler can decide on response.
func (s *TransferService) ApplyPakailinkCallback(ctx context.Context, ev PakailinkCallbackEvent) (*models.Transfer, error) {
	partnerRef := strings.TrimSpace(ev.PartnerReference)
	if partnerRef == "" {
		return nil, errors.New("partnerReferenceNo is required")
	}
	transfer, err := s.transferRepo.GetTransferByTransferID(ctx, partnerRef)
	if err != nil {
		return nil, err
	}

	transfer.ProviderData = mergeTransferProviderData(transfer.ProviderData, "callback", json.RawMessage(ev.RawPayload))
	if strings.TrimSpace(ev.ReferenceNo) != "" {
		transfer.ProviderRef = stringPtr(ev.ReferenceNo)
	}

	switch strings.TrimSpace(ev.PaymentFlagStatus) {
	case "00":
		if transfer.Status != models.TransferStatusSuccess {
			now := time.Now()
			transfer.Status = models.TransferStatusSuccess
			transfer.CompletedAt = &now
			transfer.FailedAt = nil
			transfer.FailedReason = nil
			transfer.FailedCode = nil
		}
	case "06":
		if transfer.Status != models.TransferStatusFailed {
			now := time.Now()
			transfer.Status = models.TransferStatusFailed
			transfer.FailedReason = stringPtr("Disbursement failed by provider")
			transfer.FailedCode = stringPtr(ev.PaymentFlagStatus)
			transfer.FailedAt = &now
			transfer.CompletedAt = nil
		}
	default:
		// "03" pending or anything else — keep current state.
	}

	if err := s.transferRepo.UpdateTransfer(ctx, transfer); err != nil {
		return transfer, err
	}

	if transfer.Status == models.TransferStatusSuccess || transfer.Status == models.TransferStatusFailed {
		s.trySendFinalCallback(ctx, transfer)
	}
	return transfer, nil
}

func (s *TransferService) RetryPendingCallbacks(ctx context.Context, limit int) error {
	if s.callbackSvc == nil {
		return nil
	}
	if limit <= 0 {
		limit = 50
	}

	transfers, err := s.transferRepo.ListFinalCallbackPending(ctx, limit)
	if err != nil {
		return err
	}

	for i := range transfers {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		s.trySendFinalCallback(ctx, &transfers[i])
	}

	return nil
}

func (s *TransferService) validateInquiryRequest(ctx context.Context, req *CreateTransferInquiryRequest) (*models.BankCode, models.TransferType, models.DisbursementProvider, error) {
	req.BankCode = strings.TrimSpace(req.BankCode)
	req.AccountNumber = strings.TrimSpace(req.AccountNumber)

	if req.BankCode == "" {
		return nil, "", "", newTransferError(400, "MISSING_FIELD", "bankCode is required", nil)
	}
	if req.AccountNumber == "" {
		return nil, "", "", newTransferError(400, "MISSING_FIELD", "accountNumber is required", nil)
	}
	if !isNumericString(req.AccountNumber) || len(req.AccountNumber) < 5 || len(req.AccountNumber) > 34 {
		return nil, "", "", newTransferError(400, "INVALID_ACCOUNT_NUMBER", "Invalid account number", nil)
	}

	bank, err := s.bankRepo.GetByCode(ctx, req.BankCode)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, "", "", newTransferError(400, "INVALID_BANK_CODE", "Invalid bank code", nil)
		}
		return nil, "", "", err
	}
	if !bank.SupportDisbursement {
		return nil, "", "", newTransferError(400, "INVALID_BANK_CODE", "Bank is not available for disbursement", nil)
	}

	transferType, provider, err := s.resolveTransferRoute(bank.Code)
	if err != nil {
		return nil, "", "", err
	}
	return bank, transferType, provider, nil
}

func (s *TransferService) validateTransferRequest(req *CreateTransferRequest) error {
	req.ReferenceID = strings.TrimSpace(req.ReferenceID)
	req.InquiryID = strings.TrimSpace(req.InquiryID)
	req.BankCode = strings.TrimSpace(req.BankCode)
	req.AccountNumber = strings.TrimSpace(req.AccountNumber)
	req.AccountName = strings.TrimSpace(req.AccountName)
	req.Purpose = strings.TrimSpace(req.Purpose)
	req.Remark = strings.TrimSpace(req.Remark)

	switch {
	case req.ReferenceID == "":
		return newTransferError(400, "MISSING_FIELD", "referenceId is required", nil)
	case req.InquiryID == "":
		return newTransferError(400, "MISSING_FIELD", "inquiryId is required", nil)
	case req.BankCode == "":
		return newTransferError(400, "MISSING_FIELD", "bankCode is required", nil)
	case req.AccountNumber == "":
		return newTransferError(400, "MISSING_FIELD", "accountNumber is required", nil)
	case req.AccountName == "":
		return newTransferError(400, "MISSING_FIELD", "accountName is required", nil)
	case req.Amount < disbursementMinAmount:
		return newTransferError(400, "AMOUNT_TOO_LOW", "Amount is below minimum transfer amount", nil)
	case len(req.Remark) > 50:
		return newTransferError(400, "INVALID_REMARK", "Remark must be 50 characters or fewer", nil)
	}

	if !isNumericString(req.AccountNumber) || len(req.AccountNumber) < 5 || len(req.AccountNumber) > 34 {
		return newTransferError(400, "INVALID_ACCOUNT_NUMBER", "Invalid account number", nil)
	}

	return nil
}

func (s *TransferService) createInquiryWithGeneratedID(ctx context.Context, inquiry *models.TransferInquiry) error {
	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		inquiry.InquiryID = newTransferPublicID("INQ")
		lastErr = s.transferRepo.CreateInquiry(ctx, inquiry)
		if lastErr == nil {
			return nil
		}
		if !isUniqueViolation(lastErr) {
			return lastErr
		}
	}
	return lastErr
}

func (s *TransferService) createTransferWithGeneratedID(ctx context.Context, transfer *models.Transfer) error {
	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		transfer.TransferID = newTransferPublicID("TRF")
		lastErr = s.transferRepo.CreateTransfer(ctx, transfer)
		if lastErr == nil {
			return nil
		}
		if !isUniqueViolation(lastErr) {
			if isReferenceUniqueViolation(lastErr) {
				return newTransferError(400, "DUPLICATE_REFERENCE_ID", "Reference ID already exists", lastErr)
			}
			return lastErr
		}
	}
	return lastErr
}

func (s *TransferService) refreshTransferStatus(ctx context.Context, transfer *models.Transfer, maxAge time.Duration) (bool, error) {
	if transfer == nil || !s.Available() {
		return false, nil
	}
	if transfer.Status == models.TransferStatusSuccess || transfer.Status == models.TransferStatusFailed {
		return false, nil
	}

	previousProviderRef := derefString(transfer.ProviderRef)
	previousProviderData := string(transfer.ProviderData)

	providerClient, err := s.clientForProvider(transfer.Provider)
	if err != nil {
		return false, nil
	}

	statusResp, err := providerClient.TransferStatus(ctx, bnc.TransferStatusRequest{
		OriginalPartnerReferenceNo: transfer.TransferID,
		ServiceCode:                providerServiceCode(transfer.TransferType),
		TransactionDate:            transfer.CreatedAt,
	})
	if err != nil {
		if maxAge > 0 && time.Since(transfer.CreatedAt) > maxAge {
			now := time.Now()
			transfer.Status = models.TransferStatusFailed
			transfer.FailedReason = stringPtr("Transfer timeout - unable to confirm final status with provider")
			transfer.FailedCode = stringPtr(providerErrorCode(err))
			transfer.FailedAt = &now
			transfer.ProviderData = mergeTransferProviderData(transfer.ProviderData, "status_error", transferErrorPayload(err))
			if updateErr := s.transferRepo.UpdateTransfer(ctx, transfer); updateErr != nil {
				return false, updateErr
			}
			s.trySendFinalCallback(ctx, transfer)
			return true, nil
		}

		transfer.ProviderData = mergeTransferProviderData(transfer.ProviderData, "status_error", transferErrorPayload(err))
		if updateErr := s.transferRepo.UpdateTransfer(ctx, transfer); updateErr != nil {
			return false, updateErr
		}
		return false, nil
	}

	transfer.ProviderData = mergeTransferProviderData(transfer.ProviderData, "status", statusResp.RawResponse)
	if strings.TrimSpace(statusResp.OriginalReferenceNo) != "" {
		transfer.ProviderRef = stringPtr(strings.TrimSpace(statusResp.OriginalReferenceNo))
	}

	changed := previousProviderRef != derefString(transfer.ProviderRef) || previousProviderData != string(transfer.ProviderData)
	switch statusResp.LatestTransactionStatus {
	case "00":
		if transfer.Status != models.TransferStatusSuccess {
			now := time.Now()
			transfer.Status = models.TransferStatusSuccess
			transfer.CompletedAt = &now
			transfer.FailedAt = nil
			transfer.FailedReason = nil
			transfer.FailedCode = nil
			changed = true
		}
	case "06":
		if transfer.Status != models.TransferStatusFailed {
			now := time.Now()
			transfer.Status = models.TransferStatusFailed
			transfer.FailedReason = stringPtr(nonEmptyOrDefault(statusResp.TransactionStatusDesc, "Transfer failed"))
			transfer.FailedCode = stringPtr(nonEmptyOrDefault(statusResp.LatestTransactionStatus, statusResp.ResponseCode))
			transfer.FailedAt = &now
			transfer.CompletedAt = nil
			changed = true
		}
	case "03":
		if transfer.Status != models.TransferStatusPending {
			transfer.Status = models.TransferStatusPending
			changed = true
		}
	default:
		if transfer.Status != models.TransferStatusProcessing {
			transfer.Status = models.TransferStatusProcessing
			changed = true
		}
	}

	if changed {
		if err := s.transferRepo.UpdateTransfer(ctx, transfer); err != nil {
			return false, err
		}
	}

	if transfer.Status == models.TransferStatusSuccess || transfer.Status == models.TransferStatusFailed {
		s.trySendFinalCallback(ctx, transfer)
	}

	return changed, nil
}

func (s *TransferService) trySendFinalCallback(ctx context.Context, transfer *models.Transfer) {
	if s.callbackSvc == nil || transfer == nil || transfer.CallbackSent {
		return
	}

	handled, err := s.callbackSvc.Send(ctx, transfer)
	if err != nil {
		log.Warn().Err(err).Str("transfer_id", transfer.TransferID).Msg("failed to deliver transfer callback")
		return
	}
	if !handled {
		return
	}
	if err := s.transferRepo.MarkCallbackSent(ctx, transfer.ID); err != nil {
		log.Warn().Err(err).Str("transfer_id", transfer.TransferID).Msg("failed to mark transfer callback as sent")
		return
	}
	now := time.Now()
	transfer.CallbackSent = true
	transfer.CallbackSentAt = &now
}

func (s *TransferService) buildTransferResponse(
	ctx context.Context,
	transfer *models.Transfer,
	bank *models.BankCode,
) (*models.TransferResponse, error) {
	if transfer == nil {
		return nil, newTransferError(404, "TRANSFER_NOT_FOUND", "Transfer not found", nil)
	}
	if bank == nil {
		var err error
		bank, err = s.bankRepo.GetByCode(ctx, transfer.BankCode)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	}

	resp := &models.TransferResponse{
		TransferID:         transfer.TransferID,
		ReferenceID:        transfer.ReferenceID,
		Status:             string(transfer.Status),
		TransferType:       string(transfer.TransferType),
		Route:              transferRoute(transfer.TransferType),
		BankCode:           transfer.BankCode,
		BankShortName:      "",
		BankName:           derefString(transfer.BankName),
		AccountNumber:      transfer.AccountNumber,
		AccountName:        derefString(transfer.AccountName),
		Amount:             transfer.Amount,
		Fee:                transfer.Fee,
		TotalAmount:        transfer.TotalAmount,
		Purpose:            derefString(transfer.PurposeCode),
		PurposeDescription: transferPurposeDescription(derefString(transfer.PurposeCode)),
		Remark:             derefString(transfer.Remark),
		ProviderRef:        derefString(transfer.ProviderRef),
		CreatedAt:          formatTransferTime(transfer.CreatedAt),
	}

	if bank != nil {
		resp.BankShortName = bank.ShortName
		if resp.BankName == "" {
			resp.BankName = bank.Name
		}
	}
	if transfer.CompletedAt != nil {
		resp.CompletedAt = formatTransferTime(*transfer.CompletedAt)
	}
	if transfer.FailedAt != nil {
		resp.FailedAt = formatTransferTime(*transfer.FailedAt)
	}
	if transfer.FailedReason != nil && transfer.Status == models.TransferStatusFailed {
		resp.FailedReason = *transfer.FailedReason
	}
	if transfer.FailedCode != nil && transfer.Status == models.TransferStatusFailed {
		resp.FailedCode = *transfer.FailedCode
	}

	return resp, nil
}

func (s *TransferService) resolveTransferRoute(bankCode string) (models.TransferType, models.DisbursementProvider, error) {
	// PakaiLink, when configured, supersedes per-bank routing: every transfer
	// goes through Service 43 (interbank) regardless of the destination.
	if s.pakailinkClient != nil {
		return models.TransferTypeInterbank, models.DisbursementProviderPakaiLink, nil
	}
	switch strings.TrimSpace(bankCode) {
	case briSourceBankCode:
		if s.briClient == nil || s.briSourceAccount == "" {
			return "", "", newTransferError(503, "DISBURSEMENT_UNAVAILABLE", "BRI disbursement provider is not configured", nil)
		}
		return models.TransferTypeIntrabank, models.DisbursementProviderBRI, nil
	case bncSourceBankCode:
		if s.bncClient == nil || s.bncSourceAccount == "" {
			return "", "", newTransferError(503, "DISBURSEMENT_UNAVAILABLE", "BNC disbursement provider is not configured", nil)
		}
		return models.TransferTypeIntrabank, models.DisbursementProviderBNC, nil
	default:
		if s.bncClient == nil || s.bncSourceAccount == "" {
			return "", "", newTransferError(503, "DISBURSEMENT_UNAVAILABLE", "BNC disbursement provider is not configured", nil)
		}
		return models.TransferTypeInterbank, models.DisbursementProviderBNC, nil
	}
}

func (s *TransferService) clientForProvider(provider models.DisbursementProvider) (snapTransferClient, error) {
	switch provider {
	case models.DisbursementProviderPakaiLink:
		if s.pakailinkClient == nil {
			return nil, newTransferError(503, "DISBURSEMENT_UNAVAILABLE", "PakaiLink disbursement provider is not configured", nil)
		}
		return s.pakailinkClient, nil
	case models.DisbursementProviderBRI:
		if s.briClient == nil {
			return nil, newTransferError(503, "DISBURSEMENT_UNAVAILABLE", "BRI disbursement provider is not configured", nil)
		}
		return s.briClient, nil
	case models.DisbursementProviderBNC:
		if s.bncClient == nil {
			return nil, newTransferError(503, "DISBURSEMENT_UNAVAILABLE", "BNC disbursement provider is not configured", nil)
		}
		return s.bncClient, nil
	default:
		return nil, newTransferError(503, "DISBURSEMENT_UNAVAILABLE", "Selected disbursement provider is not configured", nil)
	}
}

func (s *TransferService) sourceAccountForProvider(provider models.DisbursementProvider) (string, string, error) {
	switch provider {
	case models.DisbursementProviderPakaiLink:
		// PakaiLink does not expose a source bank account; we persist the
		// configured partner label so admins can identify the funding source.
		label := s.pakailinkSourceLabel
		if label == "" {
			label = "PAKAILINK"
		}
		return "PAKAILINK", label, nil
	case models.DisbursementProviderBRI:
		if s.briSourceAccount == "" {
			return "", "", newTransferError(503, "DISBURSEMENT_UNAVAILABLE", "BRI source account is not configured", nil)
		}
		return briSourceBankCode, s.briSourceAccount, nil
	case models.DisbursementProviderBNC:
		if s.bncSourceAccount == "" {
			return "", "", newTransferError(503, "DISBURSEMENT_UNAVAILABLE", "BNC source account is not configured", nil)
		}
		return bncSourceBankCode, s.bncSourceAccount, nil
	default:
		return "", "", newTransferError(503, "DISBURSEMENT_UNAVAILABLE", "Selected source account is not configured", nil)
	}
}

func providerServiceCode(transferType models.TransferType) string {
	if transferType == models.TransferTypeIntrabank {
		return "17"
	}
	return "18"
}

func transferRoute(transferType models.TransferType) string {
	if transferType == models.TransferTypeInterbank {
		return "BIFAST"
	}
	return ""
}

func formatTransferTime(t time.Time) string {
	wib := time.FixedZone("WIB", 7*3600)
	return t.In(wib).Format("2006-01-02T15:04:05+07:00")
}

func transferPurposeDescription(code string) string {
	return transferPurposeDescriptions[strings.TrimSpace(code)]
}

func normalizeComparableString(v string) string {
	return strings.ToUpper(strings.Join(strings.Fields(strings.TrimSpace(v)), " "))
}

func derefString(v *string) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(*v)
}

func stringPtr(v string) *string {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	return &v
}

func intPtr(v int) *int {
	return &v
}

func newTransferError(httpStatus int, code, message string, err error) *TransferServiceError {
	return &TransferServiceError{
		HTTPStatus: httpStatus,
		Code:       code,
		Message:    message,
		Err:        err,
	}
}

func mapInquiryError(err error) error {
	var apiErr *bnc.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ResponseCode {
		case "4041511", "4041611":
			return newTransferError(404, "ACCOUNT_NOT_FOUND", "Account not found", err)
		case "5001500", "5001600", "5041500", "5041600":
			return newTransferError(503, "BANK_UNAVAILABLE", "Bank is temporarily unavailable", err)
		case "4011500", "4011501", "4011600", "4011601":
			return newTransferError(503, "DISBURSEMENT_UNAVAILABLE", "Disbursement provider unavailable", err)
		default:
			if strings.HasPrefix(apiErr.ResponseCode, "4") {
				return newTransferError(400, "INVALID_REQUEST", nonEmptyOrDefault(apiErr.ResponseMessage, "Invalid inquiry request"), err)
			}
		}
	}
	return newTransferError(503, "BANK_UNAVAILABLE", "Bank is temporarily unavailable", err)
}

func mapTransferSubmitError(err error) *TransferServiceError {
	var apiErr *bnc.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ResponseCode {
		case "4031702", "4031802":
			return newTransferError(400, "AMOUNT_TOO_HIGH", "Amount exceeds provider limit", err)
		case "4031714", "4031814":
			return newTransferError(400, "INSUFFICIENT_BALANCE", "Disbursement balance is insufficient", err)
		case "4031718":
			return newTransferError(400, "ACCOUNT_NOT_FOUND", "Destination account is inactive", err)
		case "4041711", "4041811":
			return newTransferError(404, "ACCOUNT_NOT_FOUND", "Account not found", err)
		case "5031800":
			return newTransferError(503, "BIFAST_UNAVAILABLE", "BI-FAST is temporarily unavailable", err)
		case "4011700", "4011701", "4011800", "4011801":
			return newTransferError(503, "DISBURSEMENT_UNAVAILABLE", "Disbursement provider unavailable", err)
		case "4091700", "4091800":
			return newTransferError(400, "DUPLICATE_REFERENCE_ID", "Reference ID already exists", err)
		default:
			if strings.HasPrefix(apiErr.ResponseCode, "4") {
				return newTransferError(400, "INVALID_REQUEST", nonEmptyOrDefault(apiErr.ResponseMessage, "Invalid transfer request"), err)
			}
		}
	}
	return newTransferError(500, "INTERNAL_ERROR", "Internal server error", err)
}

func isUncertainTransferError(err error) bool {
	if err == nil {
		return false
	}
	var apiErr *bnc.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ResponseCode {
		case "5001700", "5001800", "5041700", "5041800", "4091700", "4091800":
			return true
		default:
			return false
		}
	}
	return true
}

func providerErrorCode(err error) string {
	var apiErr *bnc.APIError
	if errors.As(err, &apiErr) && apiErr.ResponseCode != "" {
		return apiErr.ResponseCode
	}
	return "UNKNOWN"
}

func mergeTransferProviderData(current models.NullableRawMessage, key string, payload any) models.NullableRawMessage {
	merged := map[string]any{}
	if len(current) > 0 {
		_ = json.Unmarshal(current, &merged)
	}
	if strings.TrimSpace(key) != "" && payload != nil {
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

func transferErrorPayload(err error) map[string]any {
	payload := map[string]any{
		"message": err.Error(),
	}
	var apiErr *bnc.APIError
	if errors.As(err, &apiErr) {
		payload["httpStatus"] = apiErr.HTTPStatus
		payload["responseCode"] = apiErr.ResponseCode
		payload["responseMessage"] = apiErr.ResponseMessage
		if len(apiErr.RawResponse) > 0 {
			payload["rawResponse"] = json.RawMessage(apiErr.RawResponse)
		}
	}
	return payload
}

func newTransferPublicID(prefix string) string {
	now := time.Now().In(time.FixedZone("WIB", 7*3600))
	return fmt.Sprintf("%s-%s-%06d", prefix, now.Format("20060102"), randomDigits(6))
}

func randomDigits(length int) int64 {
	if length <= 0 {
		return 0
	}
	max := int64(1)
	for i := 0; i < length; i++ {
		max *= 10
	}
	n, err := rand.Int(rand.Reader, big.NewInt(max))
	if err != nil {
		return time.Now().UnixNano() % max
	}
	return n.Int64()
}

func isUniqueViolation(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == "23505"
}

func isReferenceUniqueViolation(err error) bool {
	var pqErr *pq.Error
	if !errors.As(err, &pqErr) {
		return false
	}
	if pqErr.Code != "23505" {
		return false
	}
	return strings.Contains(strings.ToLower(pqErr.Constraint), "reference")
}

func isNumericString(v string) bool {
	for _, ch := range v {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return v != ""
}

func isValidTransferPurpose(v string) bool {
	_, ok := transferPurposeDescriptions[strings.TrimSpace(v)]
	return ok
}

func nonEmptyOrDefault(v, fallback string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return fallback
	}
	return v
}
