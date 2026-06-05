package service

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"

	"github.com/GTDGit/gtd_gateway/internal/cache"
	"github.com/GTDGit/gtd_gateway/internal/models"
	"github.com/GTDGit/gtd_gateway/internal/repository"
	"github.com/GTDGit/gtd_gateway/internal/sse"
	"github.com/GTDGit/gtd_gateway/internal/utils"
	"github.com/GTDGit/gtd_gateway/pkg/digiflazz"
)

// isDuplicateKeyError checks if the error is a PostgreSQL unique constraint violation.
func isDuplicateKeyError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	// PostgreSQL unique violation error codes/messages
	return strings.Contains(errStr, "duplicate key") ||
		strings.Contains(errStr, "unique constraint") ||
		strings.Contains(errStr, "23505") // PostgreSQL error code for unique_violation
}

// TransactionService contains business logic for transactions.
type TransactionService struct {
	trxRepo        *repository.TransactionRepository
	productRepo    *repository.ProductRepository
	skuRepo        *repository.SKURepository
	callbackRepo   *repository.CallbackRepository
	digiflazzProd  *digiflazz.Client
	digiflazzDev   *digiflazz.Client
	productSvc     *ProductService
	callbackSvc    *CallbackService
	sandboxMapper  *SandboxMapper
	inquiryCache   *cache.InquiryCache
	providerRouter *ProviderRouter         // Multi-provider router (optional)
	notifier       sse.TransactionNotifier // SSE notifier (optional)
}

// NewTransactionService constructs a TransactionService.
func NewTransactionService(
	trxRepo *repository.TransactionRepository,
	productRepo *repository.ProductRepository,
	skuRepo *repository.SKURepository,
	callbackRepo *repository.CallbackRepository,
	digiProd *digiflazz.Client,
	digiDev *digiflazz.Client,
	productSvc *ProductService,
	callbackSvc *CallbackService,
	inquiryCache *cache.InquiryCache,
) *TransactionService {
	return &TransactionService{
		trxRepo:       trxRepo,
		productRepo:   productRepo,
		skuRepo:       skuRepo,
		callbackRepo:  callbackRepo,
		digiflazzProd: digiProd,
		digiflazzDev:  digiDev,
		productSvc:    productSvc,
		callbackSvc:   callbackSvc,
		sandboxMapper: NewSandboxMapper(),
		inquiryCache:  inquiryCache,
	}
}

// SetProviderRouter sets the multi-provider router for the transaction service
func (s *TransactionService) SetProviderRouter(router *ProviderRouter) {
	s.providerRouter = router
}

// SetNotifier sets the SSE notifier for real-time transaction updates
func (s *TransactionService) SetNotifier(notifier sse.TransactionNotifier) {
	s.notifier = notifier
}

// getDigiflazzClient returns the appropriate Digiflazz client based on sandbox mode.
func (s *TransactionService) getDigiflazzClient(isSandbox bool) *digiflazz.Client {
	if isSandbox {
		return s.digiflazzDev
	}
	return s.digiflazzProd
}

// CreateTransactionRequest input
type CreateTransactionRequest struct {
	ReferenceID   string         `json:"referenceId" binding:"required"`
	SkuCode       string         `json:"skuCode" binding:"required"`
	CustomerNo    string         `json:"customerNo" binding:"required"`
	Type          string         `json:"type" binding:"required,oneof=prepaid inquiry payment"`
	TransactionID string         `json:"transactionId"` // Required for payment
	Provider      string         `json:"provider"`      // Optional: force specific provider (kiosbank, alterra, digiflazz)
	Data          map[string]any `json:"data,omitempty"`
}

// CreateTransaction routes processing based on req.Type.
func (s *TransactionService) CreateTransaction(ctx context.Context, req *CreateTransactionRequest, client *models.Client, isSandbox bool) (*models.Transaction, error) {
	switch req.Type {
	case "prepaid":
		return s.processPrepaid(ctx, req, client, isSandbox)
	case "inquiry":
		return s.processInquiry(ctx, req, client, isSandbox)
	case "payment":
		return s.processPayment(ctx, req, client, isSandbox)
	default:
		return nil, utils.ErrInvalidType
	}
}

// processPrepaid handles prepaid top-up workflow.
func (s *TransactionService) processPrepaid(ctx context.Context, req *CreateTransactionRequest, client *models.Client, isSandbox bool) (*models.Transaction, error) {
	// 1. Validate referenceId unique
	exists, err := s.trxRepo.ExistsReferenceID(client.ID, req.ReferenceID)
	if err == nil && exists {
		return nil, utils.ErrDuplicateReferenceID
	} else if err != nil {
		log.Error().Err(err).Msg("ExistsReferenceID failed")
	}

	// 2. Get product
	product, err := s.productRepo.GetBySKUCode(req.SkuCode)
	if err != nil || product == nil {
		return nil, utils.ErrInvalidSKU
	}

	// 3. Generate transaction ID
	trxID, err := s.trxRepo.GenerateTransactionID()
	if err != nil {
		return nil, err
	}

	// 4. Determine sell_price (cheapest provider price = what client sees)
	var sellPrice *int
	if s.providerRouter != nil && !isSandbox {
		if bestPrice, _, err := s.providerRouter.GetBestPrice(product.ID); err == nil && bestPrice != nil {
			sellPrice = bestPrice
		}
	}
	if sellPrice == nil && product.MinPrice != nil && *product.MinPrice > 0 {
		sellPrice = product.MinPrice
	}

	// 5. Create transaction record
	trx := &models.Transaction{
		TransactionID: trxID,
		ReferenceID:   req.ReferenceID,
		ClientID:      client.ID,
		ProductID:     product.ID,
		SkuCode:       product.SkuCode,
		CustomerNo:    req.CustomerNo,
		Type:          models.TrxTypePrepaid,
		Status:        models.StatusProcessing,
		IsSandbox:     isSandbox,
		SellPrice:     sellPrice,
	}

	if err := s.trxRepo.Create(trx); err != nil {
		// Check for duplicate reference_id (unique constraint violation)
		if isDuplicateKeyError(err) {
			return nil, utils.ErrDuplicateReferenceID
		}
		return nil, err
	}

	if s.notifier != nil {
		s.notifier.NotifyTransactionCreated(trx)
	}

	// 6. Try multi-provider routing if available
	if s.providerRouter != nil && !isSandbox {
		var providers []models.ProviderOption
		var provErr error
		if req.Provider != "" {
			providers, provErr = s.providerRouter.GetProviderOptionsAll(product.ID)
		} else {
			providers, provErr = s.providerRouter.GetProviderOptions(product.ID)
		}
		if provErr == nil && len(providers) > 0 {
			return s.executeWithProviderRouter(ctx, trx, ProviderTrxPrepaid, req.Provider, nil)
		}
		// No providers configured, fallback to legacy Digiflazz flow
		log.Debug().Int("product_id", product.ID).Msg("No multi-provider SKUs, using legacy Digiflazz flow")
	}

	// 7. Legacy flow: Get available SKUs and try each
	skus, err := s.productSvc.GetAvailableSKUs(product.ID)
	if err != nil || len(skus) == 0 {
		return s.handleAllSKUsFailed(trx)
	}

	return s.tryAllSKUs(ctx, trx, skus, isSandbox, 0)
}

// tryAllSKUs attempts transaction with each SKU until success/pending/fatal.
// CRITICAL: ref_id handling for Digiflazz idempotency:
// - Same ref_id to Digiflazz = safe (returns previous response)
// - Different ref_id = NEW transaction (dangerous if previous actually succeeded)
// We only increment refIDSuffix when we get a DEFINITE response requiring new ref_id (RC 49).
// refIDSuffixStart is used when retrying from callback to continue suffix numbering.
func (s *TransactionService) tryAllSKUs(ctx context.Context, trx *models.Transaction, skus []models.SKU, isSandbox bool, refIDSuffixStart int) (*models.Transaction, error) {
	refIDSuffix := refIDSuffixStart
	networkRetryCount := 0
	const maxNetworkRetries = 2 // Max retries per SKU on network error

	for i := 0; i < len(skus); i++ {
		sku := skus[i]

		// Generate Digiflazz ref_id - UNIQUE per SKU attempt
		digiRefID := trx.TransactionID
		if refIDSuffix > 0 {
			digiRefID = fmt.Sprintf("%s-%d", trx.TransactionID, refIDSuffix)
		}

		// Determine SKU and customer number to send to Digiflazz
		digiSKU := sku.DigiSkuCode
		digiCustomerNo := trx.CustomerNo

		// In sandbox mode, use test SKU and customer number
		if isSandbox {
			testSKU, testCustomerNo := s.sandboxMapper.GetTestMapping(sku.DigiSkuCode, trx.Type)
			digiSKU = testSKU
			digiCustomerNo = testCustomerNo
		}

		// CRITICAL: Store digiRefID BEFORE making API call for recovery
		trx.DigiRefID = &digiRefID
		trx.SkuID = &sku.ID
		if err := s.trxRepo.Update(trx); err != nil {
			log.Error().Err(err).Str("transaction_id", trx.TransactionID).Str("status", string(trx.Status)).Msg("CRITICAL: failed to update transaction in DB")
		}

		// Call Digiflazz with test data in sandbox mode, real data otherwise
		digi := s.getDigiflazzClient(isSandbox)
		resp, err := digi.Topup(ctx, digiSKU, digiCustomerNo, digiRefID, isSandbox)

		// Log attempt
		s.logAttempt(trx.ID, sku.ID, digiRefID, map[string]any{
			"buyer_sku_code": digiSKU,
			"customer_no":    digiCustomerNo,
			"ref_id":         digiRefID,
			"testing":        isSandbox,
		}, resp, err)

		if err != nil {
			// CRITICAL: Network error - DON'T change ref_id!
			// Digiflazz might have processed it. Retry with SAME ref_id is safe.
			log.Warn().
				Err(err).
				Str("transaction_id", trx.TransactionID).
				Str("digi_ref_id", digiRefID).
				Int("network_retry", networkRetryCount).
				Msg("Network error calling Digiflazz, will retry with same ref_id")

			networkRetryCount++
			if networkRetryCount <= maxNetworkRetries {
				// Wait briefly then retry with SAME ref_id (safe - Digiflazz idempotent)
				select {
				case <-ctx.Done():
					return s.handleAllSKUsFailed(trx)
				case <-time.After(5 * time.Second):
					i-- // Retry same SKU
					continue
				}
			}

			// Max network retries reached for this SKU, move to next SKU with new ref_id
			// This is necessary because we can't know if Digiflazz processed it
			log.Warn().
				Str("transaction_id", trx.TransactionID).
				Str("sku", sku.DigiSkuCode).
				Msg("Max network retries reached, switching to next SKU")

			refIDSuffix++
			networkRetryCount = 0 // Reset for next SKU
			continue
		}

		// Got response - reset network retry counter
		networkRetryCount = 0

		// Check RC
		switch {
		case digiflazz.IsSuccess(resp.RC):
			return s.handleSuccess(trx, &sku, resp)
		case digiflazz.IsPending(resp.RC):
			return s.handlePending(trx, &sku, resp)
		case digiflazz.IsFatal(resp.RC):
			return s.handleFatal(trx, resp)
		case digiflazz.NeedsNewRefID(resp.RC):
			// RC 49: Ref ID sudah terpakai - HARUS ganti ref_id
			log.Info().
				Str("transaction_id", trx.TransactionID).
				Str("rc", resp.RC).
				Msg("RC 49: Ref ID not unique, generating new suffix")
			refIDSuffix++
			i-- // Retry SAME SKU with new ref_id
			continue
		case digiflazz.IsRetryableWait(resp.RC):
			// RC 85/86: Need to wait before retrying on SAME SKU
			log.Info().
				Str("transaction_id", trx.TransactionID).
				Str("rc", resp.RC).
				Str("sku", sku.DigiSkuCode).
				Msg("Rate limited, waiting 60s before retry on same SKU")

			select {
			case <-ctx.Done():
				return s.handleAllSKUsFailed(trx)
			case <-time.After(60 * time.Second):
				// Retry same SKU - but need new ref_id because this ref_id was "used"
				refIDSuffix++
				i-- // Don't advance to next SKU, retry current one
				continue
			}
		case digiflazz.IsRetryableSwitchSKU(resp.RC):
			// Switch to next SKU with new ref_id
			log.Info().
				Str("transaction_id", trx.TransactionID).
				Str("rc", resp.RC).
				Str("sku", sku.DigiSkuCode).
				Msg("SKU failed, switching to next SKU")
			refIDSuffix++
			continue
		default:
			// Unknown RC, treat as retryable switch
			log.Warn().
				Str("transaction_id", trx.TransactionID).
				Str("rc", resp.RC).
				Msg("Unknown RC code, switching to next SKU")
			refIDSuffix++
			continue
		}
	}
	// All SKUs failed
	return s.handleAllSKUsFailed(trx)
}

// handleSuccess updates trx to success and dispatches callback.
func (s *TransactionService) handleSuccess(trx *models.Transaction, sku *models.SKU, resp *digiflazz.TransactionResponse) (*models.Transaction, error) {
	now := time.Now()
	trx.SkuID = &sku.ID
	trx.Status = models.StatusSuccess
	if resp.SN != "" {
		trx.SerialNumber = &resp.SN
	}
	trx.Amount = &resp.Price
	trx.BuyPrice = &resp.Price
	trx.ProcessedAt = &now
	if resp.RefID != "" {
		trx.DigiRefID = &resp.RefID
	}
	if err := s.persistTransactionUpdate(trx); err != nil {
		return nil, err
	}
	if s.notifier != nil {
		s.notifier.NotifyTransactionStatusChanged(trx)
	}

	// Send callback to client asynchronously
	go s.callbackSvc.SendCallback(trx, "transaction.success")
	return trx, nil
}

// handlePending updates trx to processing and stores digi ref id.
func (s *TransactionService) handlePending(trx *models.Transaction, sku *models.SKU, resp *digiflazz.TransactionResponse) (*models.Transaction, error) {
	trx.SkuID = &sku.ID
	trx.Status = models.StatusProcessing
	trx.Amount = &resp.Price
	if resp.RefID != "" {
		trx.DigiRefID = &resp.RefID
	}
	if err := s.persistTransactionUpdate(trx); err != nil {
		return nil, err
	}
	if s.notifier != nil {
		s.notifier.NotifyTransactionStatusChanged(trx)
	}
	return trx, nil
}

// handleFatal updates trx to failed and dispatches callback.
func (s *TransactionService) handleFatal(trx *models.Transaction, resp *digiflazz.TransactionResponse) (*models.Transaction, error) {
	now := time.Now()
	trx.Status = models.StatusFailed
	if resp.Message != "" {
		msg := resp.Message
		trx.FailedReason = &msg
	}
	if resp.RC != "" {
		rc := resp.RC
		trx.FailedCode = &rc
	}
	trx.ProcessedAt = &now
	if err := s.persistTransactionUpdate(trx); err != nil {
		return nil, err
	}
	if s.notifier != nil {
		s.notifier.NotifyTransactionStatusChanged(trx)
	}

	go s.callbackSvc.SendCallback(trx, "transaction.failed")
	return trx, nil
}

// handleAllSKUsFailed marks transaction as failed when all SKUs have been exhausted.
// This happens when all available SKUs return retryable errors - since we've already
// tried all sellers, there's no point in waiting. Mark as failed immediately.
func (s *TransactionService) handleAllSKUsFailed(trx *models.Transaction) (*models.Transaction, error) {
	now := time.Now()
	failure := GetCanonicalProviderFailure(ProviderFailureNoProviderAvailable)
	reason := failure.Message
	code := failure.Code
	trx.Status = models.StatusFailed
	trx.FailedReason = &reason
	trx.FailedCode = &code
	trx.ProcessedAt = &now
	trx.NextRetryAt = nil
	if err := s.persistTransactionUpdate(trx); err != nil {
		return nil, err
	}
	if s.notifier != nil {
		s.notifier.NotifyTransactionStatusChanged(trx)
	}

	go s.callbackSvc.SendCallback(trx, "transaction.failed")
	return trx, nil
}

func (s *TransactionService) persistTransactionUpdate(trx *models.Transaction) error {
	if err := s.trxRepo.Update(trx); err != nil {
		log.Error().
			Err(err).
			Str("transaction_id", trx.TransactionID).
			Str("status", string(trx.Status)).
			Msg("CRITICAL: failed to update transaction in DB")
		return err
	}
	return nil
}

func extractProviderErrorCode(message string) string {
	start := -1
	for i := 0; i < len(message); i++ {
		ch := message[i]
		if ch >= '0' && ch <= '9' {
			if start == -1 {
				start = i
			}
			continue
		}
		if start != -1 {
			token := message[start:i]
			if len(token) >= 2 && len(token) <= 3 {
				return token
			}
			start = -1
		}
	}
	if start != -1 {
		token := message[start:]
		if len(token) >= 2 && len(token) <= 3 {
			return token
		}
	}
	return ""
}

func providerResponseFromError(providerCode string, phase ProviderFailurePhase, err error) *ProviderResponse {
	if err == nil {
		return nil
	}

	message := strings.TrimSpace(err.Error())
	rc := extractProviderErrorCode(message)
	rawResp, _ := json.Marshal(map[string]any{
		"transport_error": message,
		"phase":           string(phase),
	})

	resp := &ProviderResponse{
		Status:      string(models.StatusFailed),
		RC:          rc,
		Message:     "Transaction could not be completed",
		RawResponse: rawResp,
	}

	switch providerCode {
	case string(models.ProviderAlterra):
		failureCode := ProviderFailureProviderUnavailable
		if looksLikeTimeoutMessage(message) {
			failureCode = ProviderFailureProviderTimeout
		}
		failure := GetCanonicalProviderFailure(failureCode)
		resp.Message = failure.Message
		resp.PublicCode = failure.Code
		resp.PublicMessage = failure.Message
		resp.PublicHTTPCode = failure.HTTPStatus
	case string(models.ProviderKiosbank):
		if phase == ProviderFailurePhaseInitialPayment || phase == ProviderFailurePhaseAsync {
			resp.Pending = true
			resp.Status = string(models.StatusPending)
			resp.Message = "Transaction is being processed"
			resp.Description = nil
			return resp
		}
		failureCode := ProviderFailureProviderUnavailable
		if looksLikeTimeoutMessage(message) {
			failureCode = ProviderFailureProviderTimeout
		}
		failure := GetCanonicalProviderFailure(failureCode)
		resp.Message = failure.Message
		resp.PublicCode = failure.Code
		resp.PublicMessage = failure.Message
		resp.PublicHTTPCode = failure.HTTPStatus
	default:
		failureCode := ProviderFailureProviderUnavailable
		if looksLikeTimeoutMessage(message) {
			failureCode = ProviderFailureProviderTimeout
		}
		failure := GetCanonicalProviderFailure(failureCode)
		resp.Message = failure.Message
		resp.PublicCode = failure.Code
		resp.PublicMessage = failure.Message
		resp.PublicHTTPCode = failure.HTTPStatus
	}

	return resp
}

// processInquiry handles postpaid inquiry using Redis cache.
func (s *TransactionService) processInquiry(ctx context.Context, req *CreateTransactionRequest, client *models.Client, isSandbox bool) (*models.Transaction, error) {
	// Product must exist
	product, err := s.productRepo.GetBySKUCode(req.SkuCode)
	if err != nil || product == nil {
		return nil, utils.ErrInvalidSKU
	}

	// Check if inquiry already cached (same client, customer, sku, refId)
	cached, err := s.inquiryCache.GetByCacheKey(ctx, client.ID, req.CustomerNo, req.SkuCode, req.ReferenceID)
	if err == nil && cached != nil {
		log.Debug().Str("transactionId", cached.TransactionID).Msg("inquiry cache hit")
		// Return cached inquiry as transaction model
		return s.cachedInquiryToTransaction(cached, client.ID, product.ID), nil
	} else if err != nil && err != redis.Nil {
		log.Warn().Err(err).Msg("failed to get inquiry cache")
	}

	// Cache miss - generate new transaction ID
	trxID, err := s.trxRepo.GenerateTransactionID()
	if err != nil {
		return nil, err
	}

	// Expiration end of day WIB
	wib := time.FixedZone("WIB", 7*3600) // UTC+7
	nowWIB := time.Now().In(wib)
	eod := time.Date(nowWIB.Year(), nowWIB.Month(), nowWIB.Day(), 23, 59, 59, 0, wib)

	// Try multi-provider inquiry if available and not sandbox
	if s.providerRouter != nil && !isSandbox {
		var providers []models.ProviderOption
		var provErr error
		if req.Provider != "" {
			// When a specific provider is requested, include unavailable SKUs
			providers, provErr = s.providerRouter.GetProviderOptionsAll(product.ID)
		} else {
			providers, provErr = s.providerRouter.GetProviderOptionsPostpaid(product.ID)
		}
		if provErr == nil && len(providers) > 0 {
			return s.executeInquiryWithProviders(ctx, req, client, product, trxID, providers, eod)
		}
		log.Debug().Int("product_id", product.ID).Msg("No multi-provider SKUs for inquiry, using legacy Digiflazz flow")
	}

	// Legacy Digiflazz inquiry flow
	return s.executeInquiryWithDigiflazz(ctx, req, client, product, trxID, eod, isSandbox)
}

// processPayment handles postpaid payment after a successful inquiry.
func (s *TransactionService) processPayment(ctx context.Context, req *CreateTransactionRequest, client *models.Client, isSandbox bool) (*models.Transaction, error) {
	// 1. Get inquiry from Redis
	inquiryData, err := s.inquiryCache.GetByTransactionID(ctx, req.TransactionID)
	if err == redis.Nil {
		return nil, utils.ErrTransactionNotFound
	} else if err != nil {
		log.Error().Err(err).Str("transactionId", req.TransactionID).Msg("failed to get inquiry from cache")
		return nil, fmt.Errorf("failed to get inquiry: %w", err)
	}

	// 2. Validate
	if inquiryData.ReferenceID != req.ReferenceID {
		return nil, utils.ErrReferenceMismatch
	}
	if inquiryData.CustomerNo != req.CustomerNo {
		return nil, utils.ErrCustomerMismatch
	}
	if inquiryData.ClientID != client.ID {
		return nil, utils.ErrTransactionNotFound
	}
	if inquiryData.Status != "" && inquiryData.Status != string(models.StatusSuccess) {
		return nil, utils.ErrInvalidTransactionType
	}
	// Validate SKU code belongs to same product
	product, err := s.productRepo.GetBySKUCode(req.SkuCode)
	if err != nil || product == nil || product.ID != inquiryData.ProductID {
		return nil, utils.ErrSkuMismatch
	}
	if inquiryData.ExpiredAt.Before(time.Now()) {
		return nil, utils.ErrInquiryExpired
	}

	// 3. Create payment transaction in database (this one we store!)
	payTrxID, err := s.trxRepo.GenerateTransactionID()
	if err != nil {
		return nil, err
	}
	// sell_price for payment = the inquiry amount (what client was quoted)
	var sellPrice *int
	if inquiryData.Amount > 0 {
		sp := inquiryData.Amount
		sellPrice = &sp
	}
	payment := &models.Transaction{
		TransactionID: payTrxID,
		ReferenceID:   req.ReferenceID,
		ClientID:      client.ID,
		ProductID:     inquiryData.ProductID,
		SkuCode:       inquiryData.SKUCode,
		CustomerNo:    inquiryData.CustomerNo,
		Type:          models.TrxTypePayment,
		Status:        models.StatusProcessing,
		IsSandbox:     isSandbox,
		SellPrice:     sellPrice,
	}
	if err := s.trxRepo.Create(payment); err != nil {
		return nil, err
	}

	if s.notifier != nil {
		s.notifier.NotifyTransactionCreated(payment)
	}

	// 4. Route payment to the correct provider
	// If inquiry was handled by a multi-provider (ProviderCode is set), use that same provider.
	// Otherwise, fall back to legacy Digiflazz flow.
	if inquiryData.ProviderCode != "" && s.providerRouter != nil && !isSandbox {
		log.Info().
			Str("provider", inquiryData.ProviderCode).
			Str("inquiry_trx_id", inquiryData.TransactionID).
			Str("payment_trx_id", payTrxID).
			Msg("Routing payment to same provider as inquiry")
		return s.executePaymentWithProvider(ctx, payment, inquiryData, req)
	}

	// Legacy Digiflazz payment flow
	digiSKU := req.SkuCode
	digiCustomerNo := inquiryData.CustomerNo

	if isSandbox {
		testSKU, testCustomerNo := s.sandboxMapper.GetTestMapping(req.SkuCode, payment.Type)
		digiSKU = testSKU
		digiCustomerNo = testCustomerNo
	}

	refID := inquiryData.TransactionID
	digi := s.getDigiflazzClient(isSandbox)
	resp, err := digi.Payment(ctx, digiSKU, digiCustomerNo, refID, isSandbox)

	s.logAttempt(payment.ID, 0, refID, map[string]any{
		"buyer_sku_code": digiSKU,
		"customer_no":    digiCustomerNo,
		"ref_id":         refID,
		"testing":        isSandbox,
	}, resp, err)

	if err != nil {
		return s.handleAllSKUsFailed(payment)
	}

	if digiflazz.IsSuccess(resp.RC) {
		now := time.Now()
		payment.Status = models.StatusSuccess
		if resp.SN != "" {
			payment.SerialNumber = &resp.SN
		}
		payment.Amount = &resp.Price
		payment.BuyPrice = &resp.Price
		payment.ProcessedAt = &now
		payment.DigiRefID = &refID
		if err := s.persistTransactionUpdate(payment); err != nil {
			return nil, err
		}

		if err := s.inquiryCache.Delete(ctx, inquiryData); err != nil {
			log.Warn().Err(err).Str("transactionId", inquiryData.TransactionID).Msg("failed to delete inquiry cache")
		}

		if s.notifier != nil {
			s.notifier.NotifyTransactionStatusChanged(payment)
		}
		go s.callbackSvc.SendCallback(payment, "transaction.success")
		return payment, nil
	}

	if digiflazz.IsPending(resp.RC) {
		payment.Status = models.StatusProcessing
		payment.Amount = &resp.Price
		payment.DigiRefID = &refID
		if err := s.persistTransactionUpdate(payment); err != nil {
			return nil, err
		}
		if s.notifier != nil {
			s.notifier.NotifyTransactionStatusChanged(payment)
		}
		return payment, nil
	}

	// Fatal/other
	now := time.Now()
	payment.Status = models.StatusFailed
	msg := resp.Message
	payment.FailedReason = &msg
	payment.ProcessedAt = &now
	payment.DigiRefID = &refID
	if err := s.persistTransactionUpdate(payment); err != nil {
		return nil, err
	}
	if s.notifier != nil {
		s.notifier.NotifyTransactionStatusChanged(payment)
	}
	go s.callbackSvc.SendCallback(payment, "transaction.failed")
	return payment, nil
}

// GetTransaction retrieves a transaction visible to the given client.
func (s *TransactionService) GetTransaction(transactionID string, clientID int) (*models.Transaction, error) {
	trx, err := s.trxRepo.GetByTransactionID(transactionID)
	if err == nil && trx != nil {
		if trx.ClientID != clientID {
			return nil, utils.ErrTransactionNotFound
		}
		return trx, nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if s.inquiryCache == nil {
		return nil, utils.ErrTransactionNotFound
	}

	cached, cacheErr := s.inquiryCache.GetByTransactionID(context.Background(), transactionID)
	if cacheErr == nil && cached != nil {
		if cached.ClientID != clientID {
			return nil, utils.ErrTransactionNotFound
		}
		return s.cachedInquiryToTransaction(cached, cached.ClientID, cached.ProductID), nil
	}
	if cacheErr != nil && cacheErr != redis.Nil {
		return nil, cacheErr
	}

	return nil, utils.ErrTransactionNotFound
}

// RetryTransaction retries a pending/processing transaction.
// CRITICAL: Must check if there's a pending transaction at any provider first to avoid duplicates.
func (s *TransactionService) RetryTransaction(ctx context.Context, trx *models.Transaction) (*models.Transaction, error) {
	// If transaction is Processing with an active provider ref, don't retry - wait for callback.
	if trx.Status == models.StatusProcessing {
		if (trx.DigiRefID != nil && *trx.DigiRefID != "") ||
			(trx.ProviderRefID != nil && *trx.ProviderRefID != "") {
			providerName := "provider"
			if trx.ProviderCode != nil && *trx.ProviderCode != "" {
				providerName = *trx.ProviderCode
			} else if trx.DigiRefID != nil && *trx.DigiRefID != "" {
				providerName = "digiflazz"
			}
			log.Info().
				Str("transaction_id", trx.TransactionID).
				Str("provider", providerName).
				Msg("Transaction is processing at provider, cannot retry - wait for callback")
			return trx, fmt.Errorf("transaction is processing at %s, please wait for callback", providerName)
		}
	}

	// Use provider router for multi-provider transactions (non-sandbox, prepaid)
	if s.providerRouter != nil && !trx.IsSandbox && trx.Type == "prepaid" {
		return s.executeWithProviderRouter(ctx, trx, ProviderTrxPrepaid, "", nil)
	}

	// Legacy Digiflazz-only path (sandbox or no provider router)
	skus, err := s.productSvc.GetAvailableSKUs(trx.ProductID)
	if err != nil || len(skus) == 0 {
		return s.handleAllSKUsFailed(trx)
	}

	// Start retry with suffix based on existing digi_ref_id to avoid collision
	return s.tryAllSKUsWithOffset(ctx, trx, skus, trx.IsSandbox, s.extractRefIDSuffix(trx.DigiRefID)+1)
}

// extractRefIDSuffix extracts the numeric suffix from a digi_ref_id.
// "GRB-20250203-000001" returns 0, "GRB-20250203-000001-3" returns 3.
func (s *TransactionService) extractRefIDSuffix(digiRefID *string) int {
	if digiRefID == nil || *digiRefID == "" {
		return 0
	}
	ref := *digiRefID
	// Count dashes - format is GRB-YYYYMMDD-NNNNNN[-suffix]
	dashCount := 0
	lastDashPos := -1
	for i, c := range ref {
		if c == '-' {
			dashCount++
			lastDashPos = i
		}
	}
	if dashCount < 3 || lastDashPos <= 0 {
		return 0 // No suffix
	}
	suffixStr := ref[lastDashPos+1:]
	var suffix int
	fmt.Sscanf(suffixStr, "%d", &suffix)
	return suffix
}

// tryAllSKUsWithOffset is like tryAllSKUs but starts with a specific refIDSuffix offset.
func (s *TransactionService) tryAllSKUsWithOffset(ctx context.Context, trx *models.Transaction, skus []models.SKU, isSandbox bool, startSuffix int) (*models.Transaction, error) {
	refIDSuffix := startSuffix
	networkRetryCount := 0
	const maxNetworkRetries = 2

	log.Info().
		Str("transaction_id", trx.TransactionID).
		Int("start_suffix", startSuffix).
		Msg("Starting retry with offset")

	for i := 0; i < len(skus); i++ {
		sku := skus[i]

		digiRefID := trx.TransactionID
		if refIDSuffix > 0 {
			digiRefID = fmt.Sprintf("%s-%d", trx.TransactionID, refIDSuffix)
		}

		digiSKU := sku.DigiSkuCode
		digiCustomerNo := trx.CustomerNo

		if isSandbox {
			testSKU, testCustomerNo := s.sandboxMapper.GetTestMapping(sku.DigiSkuCode, trx.Type)
			digiSKU = testSKU
			digiCustomerNo = testCustomerNo
		}

		trx.DigiRefID = &digiRefID
		trx.SkuID = &sku.ID
		if err := s.trxRepo.Update(trx); err != nil {
			log.Error().Err(err).Str("transaction_id", trx.TransactionID).Str("status", string(trx.Status)).Msg("CRITICAL: failed to update transaction in DB")
		}

		digi := s.getDigiflazzClient(isSandbox)
		resp, err := digi.Topup(ctx, digiSKU, digiCustomerNo, digiRefID, isSandbox)

		s.logAttempt(trx.ID, sku.ID, digiRefID, map[string]any{
			"buyer_sku_code": digiSKU,
			"customer_no":    digiCustomerNo,
			"ref_id":         digiRefID,
			"testing":        isSandbox,
			"is_retry":       true,
		}, resp, err)

		if err != nil {
			log.Warn().Err(err).Str("transaction_id", trx.TransactionID).Str("digi_ref_id", digiRefID).Msg("Network error on retry")
			networkRetryCount++
			if networkRetryCount <= maxNetworkRetries {
				select {
				case <-ctx.Done():
					return s.handleAllSKUsFailed(trx)
				case <-time.After(5 * time.Second):
					i--
					continue
				}
			}
			refIDSuffix++
			networkRetryCount = 0
			continue
		}

		networkRetryCount = 0

		switch {
		case digiflazz.IsSuccess(resp.RC):
			return s.handleSuccess(trx, &sku, resp)
		case digiflazz.IsPending(resp.RC):
			return s.handlePending(trx, &sku, resp)
		case digiflazz.IsFatal(resp.RC):
			return s.handleFatal(trx, resp)
		case digiflazz.NeedsNewRefID(resp.RC):
			refIDSuffix++
			i--
			continue
		case digiflazz.IsRetryableWait(resp.RC):
			select {
			case <-ctx.Done():
				return s.handleAllSKUsFailed(trx)
			case <-time.After(60 * time.Second):
				refIDSuffix++
				i--
				continue
			}
		case digiflazz.IsRetryableSwitchSKU(resp.RC):
			refIDSuffix++
			continue
		default:
			refIDSuffix++
			continue
		}
	}
	return s.handleAllSKUsFailed(trx)
}

// logAttempt writes a transaction log entry.
func (s *TransactionService) logAttempt(trxID int, skuID int, digiRefID string, request any, resp *digiflazz.TransactionResponse, err error) {
	reqJSON, _ := json.Marshal(request)
	var respJSON []byte
	var rcPtr, statusPtr, messagePtr *string
	var responseAt *time.Time
	if resp != nil {
		respJSON, _ = json.Marshal(resp)
		if resp.RC != "" {
			rc := resp.RC
			rcPtr = &rc
		}
		if resp.Status != "" {
			st := resp.Status
			statusPtr = &st
		}
		if resp.Message != "" {
			msg := resp.Message
			messagePtr = &msg
		}
		now := time.Now()
		responseAt = &now
	} else if err != nil {
		msg := err.Error()
		messagePtr = &msg
	}
	var skuIDPtr *int
	if skuID > 0 {
		skuIDPtr = &skuID
	}
	logEntry := &models.TransactionLog{
		TransactionID: trxID,
		SkuID:         skuIDPtr,
		DigiRefID:     digiRefID,
		Request:       json.RawMessage(reqJSON),
		Response:      json.RawMessage(respJSON),
		RC:            rcPtr,
		Status:        statusPtr,
		Message:       messagePtr,
		ResponseAt:    responseAt,
	}
	if err := s.callbackRepo.CreateTransactionLog(logEntry); err != nil {
		log.Error().Err(err).Msg("failed to create transaction log")
	}
}

// logProviderAttempt logs a provider transaction attempt to the transaction_logs table.
func (s *TransactionService) logProviderAttempt(trxID int, opt *models.ProviderOption, refID string, request any, resp *ProviderResponse, err error) {
	reqJSON, _ := json.Marshal(request)
	var respJSON []byte
	var rcPtr, statusPtr, messagePtr *string
	var responseAt *time.Time
	var responseTimeMs *int
	if resp != nil {
		respJSON, _ = json.Marshal(resp)
		if resp.RC != "" {
			rc := resp.RC
			rcPtr = &rc
		}
		if resp.Status != "" {
			st := resp.Status
			statusPtr = &st
		}
		if resp.Message != "" {
			msg := resp.Message
			messagePtr = &msg
		}
		if resp.ResponseTime > 0 {
			ms := int(resp.ResponseTime.Milliseconds())
			responseTimeMs = &ms
		}
		now := time.Now()
		responseAt = &now
	} else if err != nil {
		msg := err.Error()
		messagePtr = &msg
	}
	logEntry := &models.TransactionLog{
		TransactionID:  trxID,
		DigiRefID:      refID,
		Request:        json.RawMessage(reqJSON),
		Response:       json.RawMessage(respJSON),
		RC:             rcPtr,
		Status:         statusPtr,
		Message:        messagePtr,
		ResponseAt:     responseAt,
		ResponseTimeMs: responseTimeMs,
	}
	if opt != nil {
		providerID := opt.ProviderID
		providerSKUID := opt.ProviderSKUID
		logEntry.ProviderID = &providerID
		logEntry.ProviderSKUID = &providerSKUID
	}
	if err := s.callbackRepo.CreateTransactionLog(logEntry); err != nil {
		log.Error().Err(err).Msg("failed to create transaction log")
	}
}

func (s *TransactionService) logProviderAttempts(trxID int, result *ExecuteResult) {
	if result == nil {
		return
	}

	for _, attempt := range result.Attempts {
		var request any
		if attempt.Provider != nil {
			request = buildProviderLogRequest(attempt.Provider, &attempt.Request)
		} else {
			request = buildProviderLogRequest(nil, &attempt.Request)
		}

		reqJSON, _ := json.Marshal(request)
		var respJSON []byte
		var rcPtr, statusPtr, messagePtr *string
		var responseAt *time.Time
		var responseTimeMs *int
		if attempt.Response != nil {
			respJSON, _ = json.Marshal(attempt.Response)
			if attempt.Response.RC != "" {
				rc := attempt.Response.RC
				rcPtr = &rc
			}
			if attempt.Response.Status != "" {
				status := attempt.Response.Status
				statusPtr = &status
			}
			if attempt.Response.Message != "" {
				msg := attempt.Response.Message
				messagePtr = &msg
			}
			if attempt.Response.ResponseTime > 0 {
				ms := int(attempt.Response.ResponseTime.Milliseconds())
				responseTimeMs = &ms
			}
			now := time.Now()
			responseAt = &now
		} else if attempt.Error != "" {
			msg := attempt.Error
			messagePtr = &msg
		}

		logEntry := &models.TransactionLog{
			TransactionID:  trxID,
			DigiRefID:      attempt.Request.RefID,
			Request:        json.RawMessage(reqJSON),
			Response:       json.RawMessage(respJSON),
			RC:             rcPtr,
			Status:         statusPtr,
			Message:        messagePtr,
			ResponseAt:     responseAt,
			ResponseTimeMs: responseTimeMs,
		}
		if attempt.Provider != nil {
			providerID := attempt.Provider.ProviderID
			providerSKUID := attempt.Provider.ProviderSKUID
			logEntry.ProviderID = &providerID
			logEntry.ProviderSKUID = &providerSKUID
		}
		if err := s.callbackRepo.CreateTransactionLog(logEntry); err != nil {
			log.Error().Err(err).Msg("failed to create transaction log")
		}
	}
}

func buildProviderLogRequest(opt *models.ProviderOption, req *ProviderRequest) map[string]any {
	logRequest := map[string]any{
		"ref_id": req.RefID,
		"type":   req.Type,
	}
	if req.CustomerNo != "" {
		logRequest["customer_no"] = req.CustomerNo
	}
	if req.SKUCode != "" {
		logRequest["sku_code"] = req.SKUCode
	}
	if req.Amount > 0 {
		logRequest["amount"] = req.Amount
	}
	if len(req.Extra) > 0 {
		extra := cloneAnyMap(req.Extra)
		if opt != nil && opt.ProviderCode == models.ProviderAlterra {
			extra = buildAlterraLogExtra(req)
		}
		logRequest["extra"] = extra
	}
	if opt != nil {
		logRequest["provider"] = string(opt.ProviderCode)
		if opt.ProviderSKUCode != "" {
			logRequest["provider_sku_code"] = opt.ProviderSKUCode
		}
		logRequest["admin"] = opt.Admin
	}
	if logRequest["provider"] == string(models.ProviderKiosbank) {
		if wireRequest := buildKiosbankWireRequest(req, opt); len(wireRequest) > 0 {
			logRequest["wire_request"] = wireRequest
		}
	}
	if logRequest["provider"] == string(models.ProviderAlterra) {
		if wireRequest := buildAlterraWireRequest(req); len(wireRequest) > 0 {
			logRequest["wire_request"] = wireRequest
		}
	}
	return logRequest
}

func buildAlterraLogExtra(req *ProviderRequest) map[string]any {
	if req == nil {
		return nil
	}

	extra := cloneAnyMap(req.Extra)
	delete(extra, "admin")
	delete(extra, "commission")

	if req.Type == ProviderTrxPayment {
		return buildAlterraPaymentData(extra)
	}

	return extra
}

func buildAlterraWireRequest(req *ProviderRequest) map[string]any {
	if req == nil {
		return nil
	}

	productID := req.SKUCode
	if intID, ok := intValueOK(req.SKUCode); ok {
		productID = fmt.Sprintf("%d", intID)
	}

	wireRequest := map[string]any{
		"customer_id": req.CustomerNo,
	}

	switch req.Type {
	case ProviderTrxInquiry:
		wireRequest["inquiry_type"] = "Customer_information"
		data := map[string]any{
			"product_id": productID,
		}
		for key, value := range buildAlterraLogExtra(req) {
			data[key] = value
		}
		wireRequest["data"] = data
	case ProviderTrxPrepaid:
		wireRequest["product_id"] = productID
		wireRequest["order_id"] = req.RefID
		wireRequest["data"] = buildAlterraLogExtra(req)
	case ProviderTrxPayment:
		wireRequest["product_id"] = productID
		wireRequest["order_id"] = req.RefID
		wireRequest["data"] = buildAlterraLogExtra(req)
	}

	return wireRequest
}

func buildKiosbankWireRequest(req *ProviderRequest, opt *models.ProviderOption) map[string]any {
	if req == nil {
		return nil
	}

	wireRequest := map[string]any{
		"referenceID": req.RefID,
		"customerID":  req.CustomerNo,
		"productID":   req.SKUCode,
	}

	extra := normalizeKiosbankRequestData(cloneAnyMap(req.Extra))
	admin, ok := intValueOK(extra["admin"])
	if !ok && opt != nil {
		admin = opt.Admin
	}

	switch req.Type {
	case ProviderTrxInquiry:
		if periode := stringFromKeys(extra, "periode"); periode != "" {
			wireRequest["periode"] = periode
		}
	case ProviderTrxPrepaid, ProviderTrxPayment:
		wireRequest["tagihan"] = req.Amount
		wireRequest["admin"] = admin
		wireRequest["total"] = req.Amount + admin
		if noHandphone := stringFromKeys(extra, "noHandphone", "noHanphone"); noHandphone != "" {
			wireRequest["noHanphone"] = noHandphone
		}
		if nama := stringFromKeys(extra, "nama"); nama != "" {
			wireRequest["nama"] = nama
		}
		if kode := stringFromKeys(extra, "kode"); kode != "" {
			wireRequest["kode"] = kode
		}
	}

	return wireRequest
}

// cachedInquiryToTransaction converts cached inquiry data to transaction model.
func (s *TransactionService) cachedInquiryToTransaction(data *cache.InquiryData, clientID, productID int) *models.Transaction {
	status := models.StatusSuccess
	if data.Status != "" {
		status = models.TransactionStatus(data.Status)
	}
	amount := data.Amount
	expiredAt := data.ExpiredAt
	createdAt := data.CachedAt
	if createdAt.IsZero() {
		createdAt = time.Now()
	}
	var failedCode *string
	if data.FailedCode != "" {
		failedCode = &data.FailedCode
	}
	var failedReason *string
	if reason := strings.TrimSpace(data.FailedReason); reason != "" {
		failedReason = &reason
	}
	var customerName *string
	if name := strings.TrimSpace(data.CustomerName); name != "" {
		customerName = &name
	}
	var providerCode *string
	if code := strings.TrimSpace(data.ProviderCode); code != "" {
		providerCode = &code
	}
	var providerID *int
	if data.ProviderID > 0 {
		providerID = &data.ProviderID
	}
	var providerSKUID *int
	if data.ProviderSKUID > 0 {
		providerSKUID = &data.ProviderSKUID
	}
	var processedAt *time.Time
	if status == models.StatusSuccess || status == models.StatusFailed {
		processed := createdAt
		processedAt = &processed
	}

	return &models.Transaction{
		TransactionID: data.TransactionID,
		ReferenceID:   data.ReferenceID,
		ClientID:      clientID,
		ProductID:     productID,
		SkuCode:       data.SKUCode,
		CustomerNo:    data.CustomerNo,
		Type:          models.TrxTypeInquiry,
		Status:        status,
		Amount:        &amount,
		Admin:         data.Admin,
		CustomerName:  customerName,
		Description:   models.NullableRawMessage(SanitizePublicProviderDescription(data.Description)),
		FailedCode:    failedCode,
		FailedReason:  failedReason,
		ProviderID:    providerID,
		ProviderSKUID: providerSKUID,
		ProviderCode:  providerCode,
		ProviderRefID: func() *string {
			if data.ProviderTransactionID == "" {
				return nil
			}
			ref := data.ProviderTransactionID
			return &ref
		}(),
		ProviderResponse: models.NullableRawMessage(data.ProviderResponse),
		ExpiredAt:        &expiredAt,
		CreatedAt:        createdAt,
		ProcessedAt:      processedAt,
	}
}

// safeMarshalRaw returns json.RawMessage from a RawMessage or nil if empty.
func safeMarshalRaw(v json.RawMessage) json.RawMessage {
	if len(v) == 0 || bytes.Equal(v, []byte("null")) {
		return nil
	}
	// Ensure it's a copy
	cp := make([]byte, len(v))
	copy(cp, v)
	return json.RawMessage(cp)
}

func applyProviderTrace(trx *models.Transaction, resp *ProviderResponse) {
	if trx == nil || resp == nil {
		return
	}

	if len(resp.RawResponse) > 0 {
		raw := models.NullableRawMessage(resp.RawResponse)
		if len(trx.ProviderInitialResponse) == 0 {
			trx.ProviderInitialResponse = raw
		}
		trx.ProviderResponse = raw
	}

	if resp.HTTPStatus > 0 {
		httpStatus := resp.HTTPStatus
		if trx.ProviderInitialHTTPStatus == nil {
			trx.ProviderInitialHTTPStatus = &httpStatus
		}
		trx.ProviderHTTPStatus = &httpStatus
	}
}

// RetryWithNextSKU retries a transaction with the next available SKU.
// Called by callback worker when Digiflazz returns a retryable RC code.
// Returns (transaction, shouldMarkFailed, error)
// - shouldMarkFailed=true means all SKUs exhausted, mark as failed
// - shouldMarkFailed=false means either success/pending or error occurred
func (s *TransactionService) RetryWithNextSKU(ctx context.Context, trx *models.Transaction, failedRC string, failedMessage string) (*models.Transaction, bool, error) {
	log.Info().
		Str("transaction_id", trx.TransactionID).
		Str("failed_rc", failedRC).
		Str("failed_message", failedMessage).
		Msg("Retrying transaction with next SKU from callback")

	// Get available SKUs for this product (use WIB timezone for availability window)
	wib := time.FixedZone("WIB", 7*3600)
	currentTime := time.Now().In(wib).Format("15:04:05")
	skus, err := s.skuRepo.GetAvailableSKUs(trx.ProductID, currentTime)
	if err != nil || len(skus) == 0 {
		log.Error().Err(err).Int("product_id", trx.ProductID).Msg("No available SKUs for retry")
		// Call handleAllSKUsFailed to update transaction and send callback
		result, _ := s.handleAllSKUsFailed(trx)
		return result, true, nil // Mark as failed
	}

	// Get previous attempts from transaction_logs to find which SKUs were already tried
	logs, err := s.callbackRepo.GetLogsByTransactionID(trx.ID)
	if err != nil {
		log.Error().Err(err).Int("transaction_id", trx.ID).Msg("Failed to get transaction logs for retry")
		return trx, false, err
	}

	// Build set of already-tried SKU IDs
	triedSKUs := make(map[int]bool)
	for _, l := range logs {
		if l.SkuID != nil && *l.SkuID > 0 {
			triedSKUs[*l.SkuID] = true
		}
	}

	log.Debug().
		Int("total_skus", len(skus)).
		Int("tried_skus", len(triedSKUs)).
		Interface("tried_sku_ids", triedSKUs).
		Msg("SKU retry status")

	// Find next untried SKUs
	var nextSKUs []models.SKU
	for _, sku := range skus {
		if !triedSKUs[sku.ID] {
			nextSKUs = append(nextSKUs, sku)
		}
	}

	if len(nextSKUs) == 0 {
		log.Info().Str("transaction_id", trx.TransactionID).Msg("All SKUs exhausted, marking as failed")
		// Call handleAllSKUsFailed to update transaction and send callback
		result, _ := s.handleAllSKUsFailed(trx)
		return result, true, nil // All SKUs tried
	}

	log.Info().
		Str("transaction_id", trx.TransactionID).
		Int("remaining_skus", len(nextSKUs)).
		Str("next_sku", nextSKUs[0].DigiSkuCode).
		Msg("Attempting retry with remaining SKUs")

	// Calculate new ref_id suffix based on number of previous attempts
	refIDSuffixStart := len(logs)
	if refIDSuffixStart == 0 {
		refIDSuffixStart = 1 // Start from 1 if no logs (shouldn't happen but safety)
	}

	// Use the same tryAllSKUs logic with remaining SKUs
	result, err := s.tryAllSKUs(ctx, trx, nextSKUs, trx.IsSandbox, refIDSuffixStart)
	if err != nil {
		return result, false, err
	}

	// Check if result is in final state
	if result.Status == models.StatusFailed {
		return result, true, nil // All SKUs exhausted (handleAllSKUsFailed was called)
	}

	return result, false, nil // Success or Pending
}

// RetryWithNextProvider retries a prepaid multi-provider transaction with the next untried provider.
// It returns handled=true when this method has fully handled the failure path, either by retrying
// another provider or finalizing the transaction as failed.
func (s *TransactionService) RetryWithNextProvider(ctx context.Context, trx *models.Transaction, failedRC string, failedMessage string) (*models.Transaction, bool, error) {
	if trx == nil || trx.Type != models.TrxTypePrepaid || trx.IsSandbox || s.providerRouter == nil {
		return trx, false, nil
	}

	excluded, err := s.getTriedProviderSKUs(trx.ID)
	if err != nil {
		return trx, false, err
	}

	options, err := s.providerRouter.GetProviderOptions(trx.ProductID)
	if err != nil {
		return trx, false, err
	}

	hasRemaining := false
	for _, opt := range options {
		if !excluded[opt.ProviderSKUID] {
			hasRemaining = true
			break
		}
	}

	if !hasRemaining {
		resp := exhaustedProviderResponse(failedRC, failedMessage)
		result, err := s.handleProviderFailed(trx, resp)
		return result, true, err
	}

	trx.Status = models.StatusProcessing
	trx.FailedCode = nil
	trx.FailedReason = nil
	trx.ProcessedAt = nil

	result, err := s.executeWithProviderRouter(ctx, trx, ProviderTrxPrepaid, "", excluded)
	return result, true, err
}

// executeWithProviderRouter executes a transaction using the multi-provider router.
func (s *TransactionService) executeWithProviderRouter(ctx context.Context, trx *models.Transaction, trxType ProviderTransactionType, forceProvider string, excludedProviderSKUs map[int]bool) (*models.Transaction, error) {
	if s.providerRouter == nil {
		return nil, fmt.Errorf("provider router not configured")
	}

	// Build provider request
	req := &ProviderRequest{
		RefID:                  trx.TransactionID,
		CustomerNo:             trx.CustomerNo,
		Type:                   trxType,
		IsSandbox:              trx.IsSandbox,
		ForceProvider:          models.ProviderCode(forceProvider),
		ExcludedProviderSKUIDs: excludedProviderSKUs,
	}

	// Execute with provider router
	result, err := s.providerRouter.Execute(ctx, trx.ProductID, req)
	s.logProviderAttempts(trx.ID, result)
	phase := ProviderFailurePhaseInitialPayment
	if trxType == ProviderTrxInquiry {
		phase = ProviderFailurePhaseInquiry
	}
	if err != nil {
		log.Error().Err(err).Str("transaction_id", trx.TransactionID).Msg("Provider router execution failed")
		var attempts []ProviderAttempt
		if result != nil {
			attempts = result.Attempts
		}
		if resp := BuildFinalFailureResponseFromAttempts(attempts, phase); resp != nil {
			applyAttemptProvider(trx, latestAttemptOption(result))
			applyProviderTrace(trx, resp)
			if resp.ProviderRefID != "" {
				trx.ProviderRefID = &resp.ProviderRefID
			}
			return s.handleProviderFailedForPhase(trx, resp, phase)
		}
		return s.handleAllSKUsFailed(trx)
	}

	// Store provider info
	applyAttemptProvider(trx, result.ProviderUsed)

	if result.Response == nil {
		log.Error().Str("transaction_id", trx.TransactionID).Msg("Provider returned nil response")
		return s.handleAllSKUsFailed(trx)
	}

	// Store provider reference ID
	if result.Response.ProviderRefID != "" {
		trx.ProviderRefID = &result.Response.ProviderRefID
	}

	// Store raw response
	applyProviderTrace(trx, result.Response)

	// Handle response based on status
	if result.Response.Success {
		return s.handleProviderSuccess(trx, result.Response)
	}

	if result.Response.Pending {
		return s.handleProviderPending(trx, result.Response)
	}

	// Failed
	return s.handleProviderFailedForPhase(trx, result.Response, phase)
}

// handleProviderSuccess handles a successful provider response
func (s *TransactionService) handleProviderSuccess(trx *models.Transaction, resp *ProviderResponse) (*models.Transaction, error) {
	now := time.Now()
	trx.Status = models.StatusSuccess
	trx.FailedCode = nil
	trx.FailedReason = nil
	if resp.SerialNumber != "" {
		trx.SerialNumber = &resp.SerialNumber
	}
	if resp.Amount > 0 {
		trx.Amount = &resp.Amount
		trx.BuyPrice = &resp.Amount
	}
	if resp.CustomerName != "" {
		trx.CustomerName = &resp.CustomerName
	}
	if desc := SanitizePublicProviderDescription(resp.Description); len(desc) > 0 {
		trx.Description = models.NullableRawMessage(desc)
	}
	trx.ProcessedAt = &now
	if err := s.persistTransactionUpdate(trx); err != nil {
		return nil, err
	}
	if s.notifier != nil {
		s.notifier.NotifyTransactionStatusChanged(trx)
	}

	go s.callbackSvc.SendCallback(trx, "transaction.success")
	return trx, nil
}

// handleProviderPending handles a pending provider response
func (s *TransactionService) handleProviderPending(trx *models.Transaction, resp *ProviderResponse) (*models.Transaction, error) {
	trx.Status = models.StatusProcessing
	trx.FailedCode = nil
	trx.FailedReason = nil
	if resp.Amount > 0 {
		trx.Amount = &resp.Amount
	}
	if resp.CustomerName != "" {
		trx.CustomerName = &resp.CustomerName
	}
	if desc := SanitizePublicProviderDescription(resp.Description); len(desc) > 0 {
		trx.Description = models.NullableRawMessage(desc)
	}
	if err := s.persistTransactionUpdate(trx); err != nil {
		return nil, err
	}
	if s.notifier != nil {
		s.notifier.NotifyTransactionStatusChanged(trx)
	}
	return trx, nil
}

// handleProviderFailed handles a failed provider response
func (s *TransactionService) handleProviderFailed(trx *models.Transaction, resp *ProviderResponse) (*models.Transaction, error) {
	return s.handleProviderFailedForPhase(trx, resp, ProviderFailurePhaseForTransactionType(trx.Type))
}

func (s *TransactionService) handleProviderFailedForPhase(trx *models.Transaction, resp *ProviderResponse, phase ProviderFailurePhase) (*models.Transaction, error) {
	now := time.Now()
	trx.Status = models.StatusFailed
	trx.NextRetryAt = nil
	if resp != nil {
		ApplyCanonicalFailureToTransaction(trx, providerCodeForTransaction(trx), phase, resp)
	}
	trx.ProcessedAt = &now
	if err := s.persistTransactionUpdate(trx); err != nil {
		return nil, err
	}
	if s.notifier != nil {
		s.notifier.NotifyTransactionStatusChanged(trx)
	}

	go s.callbackSvc.SendCallback(trx, "transaction.failed")
	return trx, nil
}

func (s *TransactionService) getTriedProviderSKUs(trxID int) (map[int]bool, error) {
	logs, err := s.callbackRepo.GetLogsByTransactionID(trxID)
	if err != nil {
		return nil, err
	}

	tried := make(map[int]bool)
	for _, l := range logs {
		if l.ProviderSKUID != nil && *l.ProviderSKUID > 0 {
			tried[*l.ProviderSKUID] = true
		}
	}
	return tried, nil
}

func exhaustedProviderResponse(failedRC, failedMessage string) *ProviderResponse {
	message := failedMessage
	if message == "" {
		message = GetCanonicalProviderFailure(ProviderFailureNoProviderAvailable).Message
	}
	resp := &ProviderResponse{
		Status:  string(models.StatusFailed),
		RC:      failedRC,
		Message: message,
	}
	if failedRC == "" {
		failure := GetCanonicalProviderFailure(ProviderFailureNoProviderAvailable)
		resp.PublicCode = failure.Code
		resp.PublicMessage = failure.Message
		resp.PublicHTTPCode = failure.HTTPStatus
	}
	return resp
}

func exhaustedProviderResponseFromResult(result *ExecuteResult) *ProviderResponse {
	if result == nil {
		return nil
	}
	return BuildFinalFailureResponseFromAttempts(result.Attempts, ProviderFailurePhaseInitialPayment)
}

func latestAttemptOption(result *ExecuteResult) *models.ProviderOption {
	if result == nil {
		return nil
	}
	if result.ProviderUsed != nil {
		return result.ProviderUsed
	}
	for i := len(result.Attempts) - 1; i >= 0; i-- {
		if result.Attempts[i].Provider != nil {
			return result.Attempts[i].Provider
		}
	}
	return nil
}

func applyAttemptProvider(trx *models.Transaction, opt *models.ProviderOption) {
	if trx == nil || opt == nil {
		return
	}
	providerID := opt.ProviderID
	trx.ProviderID = &providerID
	providerSKUID := opt.ProviderSKUID
	trx.ProviderSKUID = &providerSKUID
	providerCode := string(opt.ProviderCode)
	trx.ProviderCode = &providerCode
	trx.Admin = opt.Admin
	if opt.Price > 0 && trx.Amount == nil {
		amount := opt.Price
		trx.Amount = &amount
	}
}

func providerCodeForTransaction(trx *models.Transaction) string {
	if trx == nil || trx.ProviderCode == nil {
		return ""
	}
	return strings.TrimSpace(*trx.ProviderCode)
}

// executeInquiryWithProviders tries inquiry across multiple providers in price order.
// On success, caches the inquiry data WITH provider info so payment routes to the same provider.
// If req.Provider is set, only that provider is used (user preference).
func (s *TransactionService) executeInquiryWithProviders(
	ctx context.Context,
	req *CreateTransactionRequest,
	client *models.Client,
	product *models.Product,
	trxID string,
	providers []models.ProviderOption,
	eod time.Time,
) (*models.Transaction, error) {
	// Filter providers if user specifies a preferred provider
	if req.Provider != "" {
		filtered := make([]models.ProviderOption, 0)
		for _, opt := range providers {
			if string(opt.ProviderCode) == req.Provider {
				filtered = append(filtered, opt)
				break
			}
		}
		if len(filtered) == 0 {
			log.Warn().Str("provider", req.Provider).Int("product_id", product.ID).Msg("Requested provider not available for this product")
			// Fall through to all providers
		} else {
			providers = filtered
		}
	}

	attempts := make([]ProviderAttempt, 0, len(providers))

	for _, opt := range providers {
		adapter := s.providerRouter.GetAdapter(string(opt.ProviderCode))
		if adapter == nil {
			log.Warn().Str("provider", string(opt.ProviderCode)).Msg("Provider adapter not found for inquiry")
			continue
		}
		if !adapter.IsHealthy() && req.Provider == "" {
			log.Warn().Str("provider", string(opt.ProviderCode)).Msg("Provider not healthy, skipping inquiry")
			continue
		}

		provReq := &ProviderRequest{
			RefID:      trxID,
			SKUCode:    opt.ProviderSKUCode,
			CustomerNo: req.CustomerNo,
			Type:       ProviderTrxInquiry,
			IsSandbox:  false,
			Extra:      cloneAnyMap(req.Data),
		}
		if opt.ProviderCode == models.ProviderKiosbank {
			provReq.Extra = normalizeKiosbankRequestData(provReq.Extra)
		}

		log.Info().
			Str("provider", string(opt.ProviderCode)).
			Str("sku_code", opt.ProviderSKUCode).
			Str("ref_id", trxID).
			Msg("Trying inquiry with provider")

		reqSnapshot := cloneProviderRequest(provReq)
		resp, err := adapter.Inquiry(ctx, provReq)
		if err != nil {
			attempts = append(attempts, ProviderAttempt{
				Provider: providerOptionPtr(opt),
				Request:  reqSnapshot,
				Error:    err.Error(),
			})
			log.Warn().Err(err).Str("provider", string(opt.ProviderCode)).Msg("Inquiry network error, trying next provider")
			continue
		}

		attempts = append(attempts, ProviderAttempt{
			Provider: providerOptionPtr(opt),
			Request:  reqSnapshot,
			Response: resp,
		})

		if resp.Success {
			// Extract provider reference_no from description (needed for payment)
			providerRefNo := ""
			if len(resp.Description) > 0 {
				var descMap map[string]any
				if err := json.Unmarshal(resp.Description, &descMap); err == nil {
					if rn, ok := descMap["referenceNo"].(string); ok && rn != "" {
						providerRefNo = rn
					} else if rn, ok := descMap["refNumber"].(string); ok && rn != "" {
						providerRefNo = rn
					} else if rn, ok := descMap["noReferensi"].(string); ok && rn != "" {
						providerRefNo = rn
					}
				}
			}

			expiredAt := eod
			if opt.ProviderCode == models.ProviderKiosbank {
				expiredAt = time.Now().Add(10 * time.Minute)
			}

			// Cache inquiry with provider info
			inquiryData := &cache.InquiryData{
				TransactionID:         trxID,
				ReferenceID:           req.ReferenceID,
				ClientID:              client.ID,
				ProductID:             product.ID,
				CustomerNo:            req.CustomerNo,
				SKUCode:               req.SkuCode,
				Amount:                resp.Amount,
				Admin:                 resp.Admin,
				CustomerName:          resp.CustomerName,
				Description:           SanitizePublicProviderDescription(resp.Description),
				ExpiredAt:             expiredAt,
				ProviderCode:          string(opt.ProviderCode),
				ProviderSKUCode:       opt.ProviderSKUCode,
				ProviderID:            opt.ProviderID,
				ProviderSKUID:         opt.ProviderSKUID,
				ProviderRefNo:         providerRefNo,
				ProviderResponse:      safeMarshalRaw(resp.RawResponse),
				ProviderHTTPStatus:    resp.HTTPStatus,
				ProviderTransactionID: resp.ProviderRefID,
				ProviderExtra:         cloneAnyMap(provReq.Extra),
				Status:                string(models.StatusSuccess),
			}

			if err := s.inquiryCache.Set(ctx, inquiryData); err != nil {
				log.Error().Err(err).Msg("failed to cache inquiry")
			}

			log.Info().
				Str("provider", string(opt.ProviderCode)).
				Str("transaction_id", trxID).
				Int("amount", resp.Amount).
				Msg("Inquiry successful with provider")

			return s.cachedInquiryToTransaction(inquiryData, client.ID, product.ID), nil
		}

		// Not successful - this is a valid biller response (e.g., wrong number, timeout)
		// Return as failed inquiry instead of trying next providers
		log.Warn().
			Str("provider", string(opt.ProviderCode)).
			Str("rc", resp.RC).
			Str("message", resp.Message).
			Msg("Inquiry failed with provider (biller error)")

		failure := CanonicalFailureForResponse(string(opt.ProviderCode), ProviderFailurePhaseInquiry, resp)
		inquiryData := &cache.InquiryData{
			TransactionID:         trxID,
			ReferenceID:           req.ReferenceID,
			ClientID:              client.ID,
			ProductID:             product.ID,
			SKUCode:               req.SkuCode,
			CustomerNo:            req.CustomerNo,
			Amount:                resp.Amount,
			Admin:                 resp.Admin,
			CustomerName:          resp.CustomerName,
			Description:           SanitizePublicProviderDescription(resp.Description),
			ExpiredAt:             eod,
			ProviderCode:          string(opt.ProviderCode),
			ProviderSKUCode:       opt.ProviderSKUCode,
			ProviderID:            opt.ProviderID,
			ProviderSKUID:         opt.ProviderSKUID,
			ProviderResponse:      safeMarshalRaw(resp.RawResponse),
			ProviderHTTPStatus:    resp.HTTPStatus,
			ProviderTransactionID: resp.ProviderRefID,
			ProviderExtra:         cloneAnyMap(provReq.Extra),
			Status:                string(models.StatusFailed),
			FailedReason:          failure.Message,
			FailedCode:            failure.Code,
		}
		if err := s.inquiryCache.SetPrimaryOnly(ctx, inquiryData); err != nil {
			log.Warn().Err(err).Str("transaction_id", trxID).Msg("failed to cache failed inquiry")
		}

		return s.cachedInquiryToTransaction(inquiryData, client.ID, product.ID), nil
	}

	// All providers failed (network or transport errors).
	failureResp := BuildFinalFailureResponseFromAttempts(attempts, ProviderFailurePhaseInquiry)
	failure := CanonicalFailureForResponse("", ProviderFailurePhaseInquiry, failureResp)
	var lastOpt *models.ProviderOption
	if len(attempts) > 0 {
		lastOpt = attempts[len(attempts)-1].Provider
	}
	inquiryData := &cache.InquiryData{
		TransactionID: trxID,
		ReferenceID:   req.ReferenceID,
		ClientID:      client.ID,
		ProductID:     product.ID,
		SKUCode:       req.SkuCode,
		CustomerNo:    req.CustomerNo,
		ExpiredAt:     eod,
		Status:        string(models.StatusFailed),
		FailedReason:  failure.Message,
		FailedCode:    failure.Code,
	}
	if lastOpt != nil {
		inquiryData.ProviderCode = string(lastOpt.ProviderCode)
		inquiryData.ProviderSKUCode = lastOpt.ProviderSKUCode
		inquiryData.ProviderID = lastOpt.ProviderID
		inquiryData.ProviderSKUID = lastOpt.ProviderSKUID
	}
	if failureResp != nil {
		inquiryData.ProviderResponse = safeMarshalRaw(failureResp.RawResponse)
		inquiryData.ProviderHTTPStatus = failureResp.HTTPStatus
		inquiryData.ProviderTransactionID = failureResp.ProviderRefID
		inquiryData.Description = SanitizePublicProviderDescription(failureResp.Description)
	}
	if err := s.inquiryCache.SetPrimaryOnly(ctx, inquiryData); err != nil {
		log.Warn().Err(err).Str("transaction_id", trxID).Msg("failed to cache failed inquiry")
	}
	return s.cachedInquiryToTransaction(inquiryData, client.ID, product.ID), nil
}

// executeInquiryWithDigiflazz runs the legacy Digiflazz inquiry flow.
func (s *TransactionService) executeInquiryWithDigiflazz(
	ctx context.Context,
	req *CreateTransactionRequest,
	client *models.Client,
	product *models.Product,
	trxID string,
	eod time.Time,
	isSandbox bool,
) (*models.Transaction, error) {
	digiSKU := req.SkuCode
	digiCustomerNo := req.CustomerNo

	if isSandbox {
		testSKU, testCustomerNo := s.sandboxMapper.GetTestMapping(req.SkuCode, models.TrxTypeInquiry)
		digiSKU = testSKU
		digiCustomerNo = testCustomerNo
	}

	digi := s.getDigiflazzClient(isSandbox)
	if digi == nil {
		return nil, fmt.Errorf("no provider available for inquiry (Digiflazz client not configured)")
	}
	resp, err := digi.Inquiry(ctx, digiSKU, digiCustomerNo, trxID, isSandbox)

	log.Info().
		Str("transactionId", trxID).
		Str("buyer_sku_code", digiSKU).
		Str("customer_no", digiCustomerNo).
		Bool("sandbox", isSandbox).
		Msg("inquiry request to digiflazz (fallback)")

	if err != nil {
		log.Error().Err(err).Str("transactionId", trxID).Msg("inquiry failed")
		return nil, fmt.Errorf("inquiry failed: %w", err)
	}

	if !digiflazz.IsSuccess(resp.RC) {
		log.Warn().Str("rc", resp.RC).Str("message", resp.Message).Msg("inquiry not successful")
		return nil, fmt.Errorf("inquiry failed: %s", resp.Message)
	}

	inquiryData := &cache.InquiryData{
		TransactionID: trxID,
		ReferenceID:   req.ReferenceID,
		ClientID:      client.ID,
		ProductID:     product.ID,
		CustomerNo:    req.CustomerNo,
		SKUCode:       req.SkuCode,
		Amount:        resp.Price,
		Admin:         resp.Admin,
		CustomerName:  resp.CustomerName,
		Description:   resp.Desc,
		ExpiredAt:     eod,
		// ProviderCode left empty = legacy Digiflazz
	}

	if err := s.inquiryCache.Set(ctx, inquiryData); err != nil {
		log.Error().Err(err).Msg("failed to cache inquiry")
	}

	return s.cachedInquiryToTransaction(inquiryData, client.ID, product.ID), nil
}

// executePaymentWithProvider executes payment using a specific provider (from inquiry cache).
func (s *TransactionService) executePaymentWithProvider(
	ctx context.Context,
	payment *models.Transaction,
	inquiryData *cache.InquiryData,
	req *CreateTransactionRequest,
) (*models.Transaction, error) {
	adapter := s.providerRouter.GetAdapter(inquiryData.ProviderCode)
	if adapter == nil {
		log.Warn().Str("provider", inquiryData.ProviderCode).Msg("Provider adapter not found for payment, falling back to Digiflazz")
		return nil, fmt.Errorf("provider adapter not found: %s", inquiryData.ProviderCode)
	}

	extra := cloneAnyMap(inquiryData.ProviderExtra)
	if len(req.Data) > 0 {
		mergeAnyMap(extra, req.Data)
	}
	if inquiryData.ProviderRefNo != "" {
		extra["reference_no"] = inquiryData.ProviderRefNo
	}
	extra["admin"] = inquiryData.Admin
	if inquiryData.ProviderCode == string(models.ProviderKiosbank) {
		extra = normalizeKiosbankRequestData(extra)
	}

	refID := inquiryData.TransactionID
	if inquiryData.ProviderCode == string(models.ProviderKiosbank) && inquiryData.ProviderTransactionID != "" {
		refID = inquiryData.ProviderTransactionID
	}

	provReq := &ProviderRequest{
		RefID:      refID,
		SKUCode:    inquiryData.ProviderSKUCode,
		CustomerNo: inquiryData.CustomerNo,
		Amount:     inquiryData.Amount,
		Type:       ProviderTrxPayment,
		IsSandbox:  payment.IsSandbox,
		Extra:      extra,
	}

	// Store provider info on the payment transaction
	providerID := inquiryData.ProviderID
	payment.ProviderID = &providerID
	providerSKUID := inquiryData.ProviderSKUID
	payment.ProviderSKUID = &providerSKUID
	providerCode := inquiryData.ProviderCode
	payment.ProviderCode = &providerCode
	payment.Admin = inquiryData.Admin
	if inquiryData.Amount > 0 {
		payment.Amount = &inquiryData.Amount
	}
	if inquiryData.ProviderCode == string(models.ProviderKiosbank) && provReq.RefID != "" {
		payment.ProviderRefID = &provReq.RefID
	}

	log.Info().
		Str("provider", inquiryData.ProviderCode).
		Str("sku_code", inquiryData.ProviderSKUCode).
		Str("ref_id", provReq.RefID).
		Str("payment_trx_id", payment.TransactionID).
		Msg("Executing payment with provider")

	resp, err := adapter.Payment(ctx, provReq)

	// Log attempt
	paymentProviderOption := &models.ProviderOption{
		ProviderID:      inquiryData.ProviderID,
		ProviderCode:    models.ProviderCode(inquiryData.ProviderCode),
		ProviderSKUID:   inquiryData.ProviderSKUID,
		ProviderSKUCode: inquiryData.ProviderSKUCode,
		Price:           inquiryData.Amount,
		Admin:           inquiryData.Admin,
	}
	s.logProviderAttempt(payment.ID, paymentProviderOption, provReq.RefID, buildProviderLogRequest(paymentProviderOption, provReq), resp, err)

	if err != nil {
		log.Error().Err(err).Str("provider", inquiryData.ProviderCode).Msg("Payment provider error")
		errResp := providerResponseFromError(inquiryData.ProviderCode, ProviderFailurePhaseInitialPayment, err)
		if errResp != nil && errResp.Pending {
			payment.Status = models.StatusProcessing
			if err := s.persistTransactionUpdate(payment); err != nil {
				return nil, err
			}
			if s.notifier != nil {
				s.notifier.NotifyTransactionStatusChanged(payment)
			}
			return payment, nil
		}
		return s.handleProviderFailed(payment, errResp)
	}

	// Store provider reference ID
	if resp.ProviderRefID != "" {
		payment.ProviderRefID = &resp.ProviderRefID
	}
	applyProviderTrace(payment, resp)

	if resp.Success {
		now := time.Now()
		payment.Status = models.StatusSuccess
		payment.FailedCode = nil
		payment.FailedReason = nil
		if resp.SerialNumber != "" {
			payment.SerialNumber = &resp.SerialNumber
		}
		if resp.Amount > 0 {
			payment.Amount = &resp.Amount
			payment.BuyPrice = &resp.Amount
		}
		if desc := SanitizePublicProviderDescription(resp.Description); len(desc) > 0 {
			payment.Description = models.NullableRawMessage(desc)
		}
		payment.ProcessedAt = &now
		if err := s.persistTransactionUpdate(payment); err != nil {
			return nil, err
		}

		// Delete inquiry from cache (already paid)
		if err := s.inquiryCache.Delete(ctx, inquiryData); err != nil {
			log.Warn().Err(err).Msg("failed to delete inquiry cache after payment")
		}

		if s.notifier != nil {
			s.notifier.NotifyTransactionStatusChanged(payment)
		}
		go s.callbackSvc.SendCallback(payment, "transaction.success")
		return payment, nil
	}

	if resp.Pending {
		payment.Status = models.StatusProcessing
		payment.FailedCode = nil
		payment.FailedReason = nil
		if resp.Amount > 0 {
			payment.Amount = &resp.Amount
		}
		if desc := SanitizePublicProviderDescription(resp.Description); len(desc) > 0 {
			payment.Description = models.NullableRawMessage(desc)
		}
		if err := s.persistTransactionUpdate(payment); err != nil {
			return nil, err
		}
		if s.notifier != nil {
			s.notifier.NotifyTransactionStatusChanged(payment)
		}
		return payment, nil
	}

	// Fatal/failed
	return s.handleProviderFailed(payment, resp)
}
