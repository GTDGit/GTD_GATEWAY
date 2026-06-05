package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/GTDGit/gtd_gateway/internal/models"
	"github.com/GTDGit/gtd_gateway/internal/repository"
)

// ProviderTransactionType represents the type of transaction
type ProviderTransactionType string

const (
	ProviderTrxPrepaid ProviderTransactionType = "prepaid"
	ProviderTrxInquiry ProviderTransactionType = "inquiry"
	ProviderTrxPayment ProviderTransactionType = "payment"
)

// ProviderRequest represents a unified request to any provider
type ProviderRequest struct {
	RefID                  string                  `json:"refId"`
	SKUCode                string                  `json:"skuCode"`
	CustomerNo             string                  `json:"customerNo"`
	Amount                 int                     `json:"amount,omitempty"`
	Type                   ProviderTransactionType `json:"type"`
	IsSandbox              bool                    `json:"isSandbox"`
	Extra                  map[string]any          `json:"extra,omitempty"` // Provider-specific fields
	ExcludedProviderSKUIDs map[int]bool            `json:"-"`

	// ForceProvider forces using a specific provider (for payment after inquiry)
	ForceProvider models.ProviderCode `json:"forceProvider,omitempty"`
	// InquiryRefID is the provider's ref ID from inquiry (required for payment)
	InquiryRefID string `json:"inquiryRefId,omitempty"`
}

// ProviderResponse represents a unified response from any provider
type ProviderResponse struct {
	Success        bool            `json:"success"`
	Pending        bool            `json:"pending"`
	RefID          string          `json:"refId"`
	ProviderRefID  string          `json:"providerRefId"`
	HTTPStatus     int             `json:"httpStatus,omitempty"`
	Phase          string          `json:"phase,omitempty"`
	Status         string          `json:"status"`
	RC             string          `json:"rc"`
	Message        string          `json:"message"`
	ProviderStatus string          `json:"providerStatus,omitempty"`
	SerialNumber   string          `json:"serialNumber,omitempty"`
	CustomerName   string          `json:"customerName,omitempty"`
	Amount         int             `json:"amount,omitempty"`
	Admin          int             `json:"admin,omitempty"`
	Description    json.RawMessage `json:"description,omitempty"`
	RawResponse    json.RawMessage `json:"rawResponse,omitempty"`
	NeedsRetry     bool            `json:"needsRetry"` // RC 49 equivalent - needs new ref_id
	ResponseTime   time.Duration   `json:"responseTime"`
	PublicCode     string          `json:"-"`
	PublicMessage  string          `json:"-"`
	PublicHTTPCode int             `json:"-"`
}

// PPOBProvider interface that all providers must implement
type PPOBProviderClient interface {
	// Code returns the provider code
	Code() models.ProviderCode

	// Topup processes a prepaid transaction
	Topup(ctx context.Context, req *ProviderRequest) (*ProviderResponse, error)

	// Inquiry checks a postpaid bill
	Inquiry(ctx context.Context, req *ProviderRequest) (*ProviderResponse, error)

	// Payment pays a postpaid bill
	Payment(ctx context.Context, req *ProviderRequest) (*ProviderResponse, error)

	// CheckStatus checks transaction status
	CheckStatus(ctx context.Context, refID string) (*ProviderResponse, error)

	// GetPriceList fetches current prices (for sync)
	GetPriceList(ctx context.Context, category string) ([]ProviderProduct, error)

	// IsHealthy returns whether the provider is currently healthy
	IsHealthy() bool
}

// ProviderProduct represents a product from provider's price list
type ProviderProduct struct {
	SKUCode     string `json:"skuCode"`
	ProductName string `json:"productName"`
	Category    string `json:"category"`
	Brand       string `json:"brand"`
	Price       int    `json:"price"`
	Admin       *int   `json:"admin,omitempty"`
	IsActive    bool   `json:"isActive"`
	Stock       *int   `json:"stock,omitempty"`
}

// ProviderRouter handles provider selection and fallback logic
type ProviderRouter struct {
	providerRepo *repository.PPOBProviderRepository
	providers    map[models.ProviderCode]PPOBProviderClient
}

// NewProviderRouter creates a new ProviderRouter
func NewProviderRouter(providerRepo *repository.PPOBProviderRepository) *ProviderRouter {
	return &ProviderRouter{
		providerRepo: providerRepo,
		providers:    make(map[models.ProviderCode]PPOBProviderClient),
	}
}

// RegisterProvider adds a provider client to the router
func (r *ProviderRouter) RegisterProvider(code models.ProviderCode, client PPOBProviderClient) {
	r.providers[code] = client
}

// GetClients returns a copy of the provider clients map
func (r *ProviderRouter) GetClients() map[models.ProviderCode]PPOBProviderClient {
	result := make(map[models.ProviderCode]PPOBProviderClient)
	for k, v := range r.providers {
		result[k] = v
	}
	return result
}

// GetAdapter returns the provider client for a given code, or nil if not found
func (r *ProviderRouter) GetAdapter(code string) PPOBProviderClient {
	return r.providers[models.ProviderCode(code)]
}

// ExecuteResult contains the result of a transaction execution
type ExecuteResult struct {
	Success        bool                    `json:"success"`
	Response       *ProviderResponse       `json:"response"`
	ProviderUsed   *models.ProviderOption  `json:"providerUsed"`
	ProvidersTried []models.ProviderOption `json:"providersTried"`
	Attempts       []ProviderAttempt       `json:"attempts,omitempty"`
	Error          error                   `json:"error,omitempty"`
}

// ProviderAttempt captures one concrete provider attempt, including the request shape used.
type ProviderAttempt struct {
	Provider *models.ProviderOption `json:"provider,omitempty"`
	Request  ProviderRequest        `json:"request"`
	Response *ProviderResponse      `json:"response,omitempty"`
	Error    string                 `json:"error,omitempty"`
}

// Execute tries to execute a transaction with providers in order of preference.
// - Prepaid: sorted by price ASC (cheapest first)
// - Postpaid (inquiry/payment): sorted by effective admin (admin - commission) ASC
// - ForceProvider: uses exact provider specified (for user preference or payment after inquiry)
// Flow: try best provider -> if fail, try next -> ... -> finally try backup (Digiflazz)
func (r *ProviderRouter) Execute(ctx context.Context, productID int, req *ProviderRequest) (*ExecuteResult, error) {
	result := &ExecuteResult{
		ProvidersTried: make([]models.ProviderOption, 0),
		Attempts:       make([]ProviderAttempt, 0),
	}

	// If ForceProvider is set, use only that provider
	if req.ForceProvider != "" {
		return r.executeWithProvider(ctx, productID, req, result)
	}

	// Get providers sorted appropriately based on transaction type
	var options []models.ProviderOption
	var err error

	switch req.Type {
	case ProviderTrxPrepaid:
		// Prepaid: sort by price ASC (cheapest first)
		options, err = r.providerRepo.GetProvidersForProduct(productID)
	case ProviderTrxInquiry, ProviderTrxPayment:
		// Postpaid: sort by effective admin (admin - commission) ASC
		options, err = r.providerRepo.GetProvidersForProductPostpaid(productID)
	default:
		return nil, fmt.Errorf("invalid transaction type: %s", req.Type)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to get providers: %w", err)
	}

	if len(options) == 0 {
		return nil, fmt.Errorf("no providers available for product %d", productID)
	}

	if len(req.ExcludedProviderSKUIDs) > 0 {
		filtered := make([]models.ProviderOption, 0, len(options))
		for _, opt := range options {
			if req.ExcludedProviderSKUIDs[opt.ProviderSKUID] {
				continue
			}
			filtered = append(filtered, opt)
		}
		options = filtered
	}

	if len(options) == 0 {
		return result, fmt.Errorf("no remaining providers available for product %d", productID)
	}

	log.Debug().
		Int("product_id", productID).
		Int("provider_count", len(options)).
		Str("type", string(req.Type)).
		Msg("Starting provider execution")

	refIDSuffix := 0
	baseRefID := req.RefID

	for _, opt := range options {
		// Get provider client
		client, ok := r.providers[opt.ProviderCode]
		if !ok {
			log.Warn().
				Str("provider", string(opt.ProviderCode)).
				Msg("Provider client not registered")
			continue
		}

		// Check if provider is healthy
		if !client.IsHealthy() {
			log.Warn().
				Str("provider", string(opt.ProviderCode)).
				Msg("Provider not healthy, skipping")
			continue
		}

		// Update ref ID for each attempt (to avoid duplicate issues)
		if refIDSuffix > 0 {
			req.RefID = fmt.Sprintf("%s-%d", baseRefID, refIDSuffix)
		} else {
			req.RefID = baseRefID
		}

		// Update SKU code and amounts for this provider
		req.SKUCode = opt.ProviderSKUCode
		req.Amount = opt.Price
		if req.Extra == nil {
			req.Extra = make(map[string]any)
		}
		req.Extra["admin"] = opt.Admin
		req.Extra["commission"] = opt.Commission

		result.ProvidersTried = append(result.ProvidersTried, opt)

		log.Info().
			Str("provider", string(opt.ProviderCode)).
			Str("sku_code", opt.ProviderSKUCode).
			Int("price", opt.Price).
			Int("admin", opt.Admin).
			Bool("is_backup", opt.IsBackup).
			Str("ref_id", req.RefID).
			Msg("Trying provider")

		reqSnapshot := cloneProviderRequest(req)
		startTime := time.Now()
		var resp *ProviderResponse

		// Execute transaction based on type
		switch req.Type {
		case ProviderTrxPrepaid:
			resp, err = client.Topup(ctx, req)
		case ProviderTrxInquiry:
			resp, err = client.Inquiry(ctx, req)
		case ProviderTrxPayment:
			resp, err = client.Payment(ctx, req)
		default:
			return nil, fmt.Errorf("invalid transaction type: %s", req.Type)
		}

		responseTime := time.Since(startTime)

		// Record health metrics
		success := err == nil && resp != nil && (resp.Success || resp.Pending)
		failureReason := ""
		if err != nil {
			failureReason = err.Error()
		} else if resp != nil && !resp.Success && !resp.Pending {
			failureReason = resp.Message
		}

		_ = r.providerRepo.RecordProviderRequest(
			opt.ProviderID,
			success,
			int(responseTime.Milliseconds()),
			failureReason,
		)

		// Handle network error - retry same provider with same ref_id is safe
		if err != nil {
			result.Attempts = append(result.Attempts, ProviderAttempt{
				Provider: providerOptionPtr(opt),
				Request:  reqSnapshot,
				Error:    err.Error(),
			})
			result.ProviderUsed = providerOptionPtr(opt)
			log.Warn().
				Err(err).
				Str("provider", string(opt.ProviderCode)).
				Str("ref_id", req.RefID).
				Msg("Network error, moving to next provider")
			refIDSuffix++
			continue
		}

		// Handle response
		if resp.Success {
			result.Attempts = append(result.Attempts, ProviderAttempt{
				Provider: providerOptionPtr(opt),
				Request:  reqSnapshot,
				Response: resp,
			})
			log.Info().
				Str("provider", string(opt.ProviderCode)).
				Str("ref_id", req.RefID).
				Str("status", resp.Status).
				Msg("Transaction successful")

			result.Success = true
			result.Response = resp
			result.ProviderUsed = &opt
			return result, nil
		}

		if resp.Pending {
			result.Attempts = append(result.Attempts, ProviderAttempt{
				Provider: providerOptionPtr(opt),
				Request:  reqSnapshot,
				Response: resp,
			})
			log.Info().
				Str("provider", string(opt.ProviderCode)).
				Str("ref_id", req.RefID).
				Str("status", resp.Status).
				Msg("Transaction pending")

			result.Success = false // Not fully successful yet
			result.Response = resp
			result.ProviderUsed = &opt
			return result, nil // Return pending - don't try other providers
		}

		// Transaction failed - try next provider
		result.Attempts = append(result.Attempts, ProviderAttempt{
			Provider: providerOptionPtr(opt),
			Request:  reqSnapshot,
			Response: resp,
		})
		result.Response = resp
		result.ProviderUsed = providerOptionPtr(opt)
		log.Warn().
			Str("provider", string(opt.ProviderCode)).
			Str("ref_id", req.RefID).
			Str("rc", resp.RC).
			Str("message", resp.Message).
			Bool("needs_new_ref_id", resp.NeedsRetry).
			Msg("Transaction failed, trying next provider")

		// For backup provider, we don't continue
		if opt.IsBackup {
			result.Response = resp
			result.ProviderUsed = &opt
			result.Error = fmt.Errorf("all providers failed: %s", resp.Message)
			return result, nil
		}

		// Increment ref_id suffix for next provider to avoid collision
		refIDSuffix++
	}

	// All providers failed
	return result, fmt.Errorf("all providers exhausted")
}

// executeWithProvider executes a transaction with a specific provider (user preference or payment after inquiry)
func (r *ProviderRouter) executeWithProvider(ctx context.Context, productID int, req *ProviderRequest, result *ExecuteResult) (*ExecuteResult, error) {
	// Get the specific provider
	client, ok := r.providers[req.ForceProvider]
	if !ok {
		return nil, fmt.Errorf("forced provider %s not registered", req.ForceProvider)
	}

	// Get provider options — include unavailable since this is a forced provider request
	options, err := r.providerRepo.GetProvidersForProductAll(productID)
	if err != nil {
		return nil, fmt.Errorf("failed to get providers: %w", err)
	}

	var opt *models.ProviderOption
	for i := range options {
		if options[i].ProviderCode == req.ForceProvider {
			opt = &options[i]
			break
		}
	}

	if opt == nil {
		return nil, fmt.Errorf("provider %s not available for product %d", req.ForceProvider, productID)
	}

	// Update SKU code and amounts for this provider
	req.SKUCode = opt.ProviderSKUCode
	req.Amount = opt.Price
	if req.Extra == nil {
		req.Extra = make(map[string]any)
	}
	req.Extra["admin"] = opt.Admin
	req.Extra["commission"] = opt.Commission

	log.Info().
		Str("provider", string(opt.ProviderCode)).
		Str("sku_code", opt.ProviderSKUCode).
		Int("price", opt.Price).
		Int("admin", opt.Admin).
		Str("type", string(req.Type)).
		Str("ref_id", req.RefID).
		Msg("Executing with forced provider")

	result.ProvidersTried = append(result.ProvidersTried, *opt)

	reqSnapshot := cloneProviderRequest(req)
	startTime := time.Now()
	var resp *ProviderResponse
	switch req.Type {
	case ProviderTrxPrepaid:
		resp, err = client.Topup(ctx, req)
	case ProviderTrxInquiry:
		resp, err = client.Inquiry(ctx, req)
	case ProviderTrxPayment:
		resp, err = client.Payment(ctx, req)
	}
	responseTime := time.Since(startTime)

	// Record health metrics
	success := err == nil && resp != nil && (resp.Success || resp.Pending)
	failureReason := ""
	if err != nil {
		failureReason = err.Error()
	} else if resp != nil && !resp.Success && !resp.Pending {
		failureReason = resp.Message
	}

	_ = r.providerRepo.RecordProviderRequest(
		opt.ProviderID,
		success,
		int(responseTime.Milliseconds()),
		failureReason,
	)

	if err != nil {
		result.Attempts = append(result.Attempts, ProviderAttempt{
			Provider: providerOptionPtr(*opt),
			Request:  reqSnapshot,
			Error:    err.Error(),
		})
		return nil, fmt.Errorf("%s failed with provider %s: %w", req.Type, req.ForceProvider, err)
	}

	result.Attempts = append(result.Attempts, ProviderAttempt{
		Provider: providerOptionPtr(*opt),
		Request:  reqSnapshot,
		Response: resp,
	})
	result.Response = resp
	result.ProviderUsed = opt
	result.Success = resp.Success

	return result, nil
}

func cloneProviderRequest(req *ProviderRequest) ProviderRequest {
	if req == nil {
		return ProviderRequest{}
	}

	cloned := *req
	cloned.Extra = cloneAnyMap(req.Extra)
	if len(req.ExcludedProviderSKUIDs) > 0 {
		cloned.ExcludedProviderSKUIDs = make(map[int]bool, len(req.ExcludedProviderSKUIDs))
		for k, v := range req.ExcludedProviderSKUIDs {
			cloned.ExcludedProviderSKUIDs[k] = v
		}
	}
	return cloned
}

func providerOptionPtr(opt models.ProviderOption) *models.ProviderOption {
	copyOpt := opt
	return &copyOpt
}

// GetBestPrice returns the best price for a product from non-backup providers
func (r *ProviderRouter) GetBestPrice(productID int) (*int, *int, error) {
	return r.providerRepo.GetBestPriceForProduct(productID)
}

// GetProviderOptions returns all available providers for a product sorted by price (for prepaid)
func (r *ProviderRouter) GetProviderOptions(productID int) ([]models.ProviderOption, error) {
	return r.providerRepo.GetProvidersForProduct(productID)
}

// GetProviderOptionsPostpaid returns providers sorted by effective admin (admin - commission) ASC
func (r *ProviderRouter) GetProviderOptionsPostpaid(productID int) ([]models.ProviderOption, error) {
	return r.providerRepo.GetProvidersForProductPostpaid(productID)
}

// GetProviderOptionsAll returns all providers including unavailable ones (for explicit provider requests)
func (r *ProviderRouter) GetProviderOptionsAll(productID int) ([]models.ProviderOption, error) {
	return r.providerRepo.GetProvidersForProductAll(productID)
}
