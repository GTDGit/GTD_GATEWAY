package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/GTDGit/gtd_gateway/internal/models"
	"github.com/GTDGit/gtd_gateway/pkg/alterra"
)

// AlterraProviderClient implements PPOBProviderClient for Alterra
type AlterraProviderClient struct {
	prodClient    *alterra.Client
	devClient     *alterra.Client
	healthy       bool
	healthMu      sync.RWMutex
	lastUnhealthy time.Time
}

// NewAlterraProviderClient creates a new Alterra provider client
func NewAlterraProviderClient(prodClient, devClient *alterra.Client) *AlterraProviderClient {
	return &AlterraProviderClient{
		prodClient: prodClient,
		devClient:  devClient,
		healthy:    true,
	}
}

// Code returns the provider code
func (c *AlterraProviderClient) Code() models.ProviderCode {
	return models.ProviderAlterra
}

// getClient returns the appropriate client based on sandbox mode
func (c *AlterraProviderClient) getClient(isSandbox bool) *alterra.Client {
	if isSandbox && c.devClient != nil {
		return c.devClient
	}
	return c.prodClient
}

// Topup processes a prepaid transaction via Purchase
func (c *AlterraProviderClient) Topup(ctx context.Context, req *ProviderRequest) (*ProviderResponse, error) {
	client := c.getClient(req.IsSandbox)
	startTime := time.Now()

	// Parse product ID from SKU code
	productID, err := strconv.Atoi(req.SKUCode)
	if err != nil {
		return nil, err
	}

	// Get extra data if provided
	var data json.RawMessage
	if req.Extra != nil {
		data, _ = json.Marshal(sanitizeAlterraExtra(req.Extra))
	}

	resp, err := client.Purchase(ctx, req.CustomerNo, productID, req.RefID, data)
	responseTime := time.Since(startTime)

	if err != nil {
		c.markUnhealthy()
		return nil, err
	}

	c.markHealthy()
	return c.convertResponse(resp, req.RefID, responseTime), nil
}

// Inquiry checks a postpaid bill
func (c *AlterraProviderClient) Inquiry(ctx context.Context, req *ProviderRequest) (*ProviderResponse, error) {
	client := c.getClient(req.IsSandbox)
	startTime := time.Now()

	// Parse product ID from SKU code
	productID, err := strconv.Atoi(req.SKUCode)
	if err != nil {
		return nil, err
	}

	// Get extra data if provided
	var data json.RawMessage
	if req.Extra != nil {
		data, _ = json.Marshal(sanitizeAlterraExtra(req.Extra))
	}

	resp, err := client.Inquiry(ctx, req.CustomerNo, productID, req.RefID, data)
	responseTime := time.Since(startTime)

	if err != nil {
		c.markUnhealthy()
		return nil, err
	}

	c.markHealthy()
	return c.convertResponse(resp, req.RefID, responseTime), nil
}

// Payment pays a postpaid bill
func (c *AlterraProviderClient) Payment(ctx context.Context, req *ProviderRequest) (*ProviderResponse, error) {
	client := c.getClient(req.IsSandbox)
	startTime := time.Now()

	// Parse product ID from SKU code
	productID, err := strconv.Atoi(req.SKUCode)
	if err != nil {
		return nil, err
	}

	// Alterra purchase/payment for BPJS-style flows requires the inquiry reference
	// and now also supports forwarding payment_period when the provider asks for it.
	paymentData := buildAlterraPaymentData(req.Extra)
	data, _ := json.Marshal(paymentData)

	resp, err := client.Payment(ctx, req.CustomerNo, productID, req.RefID, data)
	responseTime := time.Since(startTime)

	if err != nil {
		c.markUnhealthy()
		return nil, err
	}

	c.markHealthy()
	return c.convertResponse(resp, req.RefID, responseTime), nil
}

// CheckStatus checks transaction status using Alterra's transaction ID
func (c *AlterraProviderClient) CheckStatus(ctx context.Context, refID string) (*ProviderResponse, error) {
	client := c.getClient(false)
	startTime := time.Now()

	// refID is Alterra's transaction ID (numeric string)
	trxID, err := strconv.Atoi(refID)
	if err != nil {
		return nil, fmt.Errorf("invalid alterra transaction id %q: %w", refID, err)
	}

	resp, err := client.GetTransactionByID(ctx, trxID)
	responseTime := time.Since(startTime)

	if err != nil {
		return nil, err
	}

	// Convert detail response to transaction response format
	trxResp := &alterra.TransactionResponse{
		TransactionID: resp.TransactionID,
		Type:          resp.Type,
		CreatedAt:     resp.CreatedAt,
		UpdatedAt:     resp.UpdatedAt,
		CustomerID:    resp.CustomerID,
		CustomerName:  resp.CustomerName,
		OrderID:       resp.OrderID,
		Price:         resp.Price,
		Status:        resp.Status,
		ResponseCode:  resp.ResponseCode,
		Amount:        resp.Amount,
		Admin:         resp.Admin,
		Product:       resp.Product,
		Data:          resp.Data,
		RawResponse:   resp.RawResponse,
		HTTPStatus:    resp.HTTPStatus,
	}

	return c.convertResponse(trxResp, refID, responseTime), nil
}

// GetPriceList fetches current prices
func (c *AlterraProviderClient) GetPriceList(ctx context.Context, category string) ([]ProviderProduct, error) {
	client := c.getClient(false)

	products, err := client.GetAllProducts(ctx)
	if err != nil {
		return nil, err
	}

	var result []ProviderProduct
	for _, p := range products {
		if category != "" && p.ProductType != category {
			continue
		}
		result = append(result, ProviderProduct{
			SKUCode:     strconv.Itoa(p.ProductID),
			ProductName: p.Label,
			Category:    p.ProductType,
			Brand:       p.Operator,
			Price:       p.Price,
			IsActive:    bool(p.Enable),
		})
	}

	return result, nil
}

// IsHealthy returns whether the provider is healthy.
// Auto-recovers after 60 seconds of being unhealthy.
func (c *AlterraProviderClient) IsHealthy() bool {
	c.healthMu.RLock()
	healthy := c.healthy
	lastUnhealthy := c.lastUnhealthy
	c.healthMu.RUnlock()

	if !healthy && !lastUnhealthy.IsZero() && time.Since(lastUnhealthy) > 60*time.Second {
		c.markHealthy()
		return true
	}
	return healthy
}

// markHealthy marks the provider as healthy
func (c *AlterraProviderClient) markHealthy() {
	c.healthMu.Lock()
	defer c.healthMu.Unlock()
	c.healthy = true
}

// markUnhealthy marks the provider as unhealthy
func (c *AlterraProviderClient) markUnhealthy() {
	c.healthMu.Lock()
	defer c.healthMu.Unlock()
	c.healthy = false
	c.lastUnhealthy = time.Now()
}

// convertResponse converts Alterra response to unified format
func (c *AlterraProviderClient) convertResponse(resp *alterra.TransactionResponse, refID string, responseTime time.Duration) *ProviderResponse {
	rawResp := resp.RawResponse
	if len(rawResp) == 0 {
		rawResp, _ = json.Marshal(resp)
	}

	// Get reference_no from inquiry (top-level or data)
	referenceNo := resp.ReferenceNo
	if referenceNo == "" && resp.Data != nil {
		referenceNo = resp.Data.ReferenceNo
		if referenceNo == "" {
			referenceNo = resp.Data.RefNumber
		}
	}

	// Create description from data
	var description json.RawMessage
	if resp.Data != nil {
		desc := map[string]any{
			"nominal":     resp.Data.Nominal,
			"admin":       resp.Data.Admin,
			"token":       resp.Data.Token,
			"kwh":         resp.Data.KWH,
			"period":      resp.Data.Period,
			"refNumber":   resp.Data.RefNumber,
			"referenceNo": referenceNo,
		}
		if resp.Data.Identifier != nil {
			desc["identifier"] = resp.Data.Identifier
		}
		if resp.Data.BillInfo != nil {
			desc["billInfo"] = resp.Data.BillInfo
		}
		description, _ = json.Marshal(desc)
	} else if referenceNo != "" {
		desc := map[string]any{"referenceNo": referenceNo}
		description, _ = json.Marshal(desc)
	}

	// Get serial number from data
	serialNumber := ""
	if resp.Data != nil {
		if resp.Data.Token != "" {
			serialNumber = resp.Data.Token
		} else if resp.Data.VendorRefNo != "" {
			serialNumber = resp.Data.VendorRefNo
		} else if resp.Data.RefNumber != "" {
			serialNumber = resp.Data.RefNumber
		}
	}

	rc := alterraResponseCode(resp)
	message := alterraResponseMessage(resp, rc)

	providerRefID := ""
	if resp.TransactionID > 0 {
		providerRefID = strconv.Itoa(resp.TransactionID)
	}

	return &ProviderResponse{
		Success:       alterra.IsSuccess(resp.ResponseCode),
		Pending:       alterra.IsPending(resp.ResponseCode),
		RefID:         refID,
		ProviderRefID: providerRefID,
		HTTPStatus:    resp.HTTPStatus,
		Status:        resp.Status,
		RC:            rc,
		Message:       message,
		SerialNumber:  serialNumber,
		CustomerName:  resp.CustomerName,
		Amount:        resp.Amount,
		Admin:         resp.Admin,
		Description:   description,
		RawResponse:   rawResp,
		NeedsRetry:    alterra.NeedsNewRefID(resp.ResponseCode),
		ResponseTime:  responseTime,
	}
}

func alterraResponseCode(resp *alterra.TransactionResponse) string {
	if resp == nil {
		return ""
	}
	if resp.ResponseCode != "" {
		return resp.ResponseCode
	}
	if resp.Error != nil && resp.Error.Code != "" {
		return resp.Error.Code
	}
	if resp.Code != "" {
		return resp.Code
	}
	if resp.HTTPStatus > 0 {
		return strconv.Itoa(resp.HTTPStatus)
	}
	return ""
}

func alterraResponseMessage(resp *alterra.TransactionResponse, rc string) string {
	if resp == nil {
		return ""
	}
	if resp.Error != nil && resp.Error.Message != "" {
		return resp.Error.Message
	}
	if resp.Message != "" {
		return resp.Message
	}
	if desc := alterra.GetRCDescription(rc); desc != "Unknown error" {
		return desc
	}
	if resp.Status != "" {
		return resp.Status
	}
	return "Alterra transaction failed"
}

func sanitizeAlterraExtra(extra map[string]any) map[string]any {
	if len(extra) == 0 {
		return map[string]any{}
	}

	sanitized := make(map[string]any, len(extra))
	for key, value := range extra {
		switch key {
		case "admin", "commission":
			continue
		default:
			sanitized[key] = value
		}
	}
	return sanitized
}

func buildAlterraPaymentData(extra map[string]any) map[string]any {
	paymentData := make(map[string]any)
	if len(extra) == 0 {
		return paymentData
	}

	if refNo, ok := extra["reference_no"].(string); ok && refNo != "" {
		paymentData["reference_no"] = refNo
	}
	if paymentPeriod, ok := extra["payment_period"].(string); ok && paymentPeriod != "" {
		paymentData["payment_period"] = paymentPeriod
	}
	return paymentData
}
