package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/GTDGit/gtd_gateway/internal/models"
	"github.com/GTDGit/gtd_gateway/pkg/bri"
)

type BRIProviderClient struct {
	client        *bri.Client
	healthy       bool
	healthMu      sync.RWMutex
	denominations []int
}

func NewBRIProviderClient(client *bri.Client, denominations []int) *BRIProviderClient {
	if len(denominations) == 0 {
		denominations = []int{20000, 50000, 100000, 150000, 200000}
	}
	return &BRIProviderClient{
		client:        client,
		healthy:       client != nil,
		denominations: append([]int(nil), denominations...),
	}
}

func (c *BRIProviderClient) Code() models.ProviderCode {
	return models.ProviderBRI
}

func (c *BRIProviderClient) Topup(ctx context.Context, req *ProviderRequest) (*ProviderResponse, error) {
	if c.client == nil {
		return nil, fmt.Errorf("bri client not configured")
	}
	amount, err := parseBRIZZIAmount(req)
	if err != nil {
		return nil, err
	}

	startTime := time.Now()
	resp, err := c.client.BRIZZITopup(ctx, req.CustomerNo, amount)
	responseTime := time.Since(startTime)
	if err != nil {
		c.markUnhealthy()
		return nil, err
	}

	c.markHealthy()
	rawResp, _ := json.Marshal(resp)
	providerRefID := buildBRIZZICheckRef(resp.Data.Reff, req.CustomerNo, amount)

	return &ProviderResponse{
		Success:       resp.ResponseCode == "00",
		Pending:       isBRIZZIPending(resp.ResponseCode),
		RefID:         req.RefID,
		ProviderRefID: providerRefID,
		Status:        brizziStatus(resp.ResponseCode),
		RC:            resp.ResponseCode,
		Message:       nonEmptyString(resp.ResponseDescription, resp.ErrorCode),
		Amount:        amount,
		Description:   rawResp,
		RawResponse:   rawResp,
		ResponseTime:  responseTime,
	}, nil
}

func (c *BRIProviderClient) Inquiry(ctx context.Context, req *ProviderRequest) (*ProviderResponse, error) {
	if c.client == nil {
		return nil, fmt.Errorf("bri client not configured")
	}

	startTime := time.Now()
	resp, err := c.client.BRIZZIValidateCard(ctx, req.CustomerNo)
	responseTime := time.Since(startTime)
	if err != nil {
		c.markUnhealthy()
		return nil, err
	}

	c.markHealthy()
	rawResp, _ := json.Marshal(resp)

	return &ProviderResponse{
		Success:      resp.ResponseCode == "00",
		Pending:      false,
		RefID:        req.RefID,
		Status:       brizziStatus(resp.ResponseCode),
		RC:           resp.ResponseCode,
		Message:      resp.ResponseDescription,
		Description:  rawResp,
		RawResponse:  rawResp,
		ResponseTime: responseTime,
	}, nil
}

func (c *BRIProviderClient) Payment(ctx context.Context, req *ProviderRequest) (*ProviderResponse, error) {
	return c.Topup(ctx, req)
}

func (c *BRIProviderClient) CheckStatus(ctx context.Context, refID string) (*ProviderResponse, error) {
	if c.client == nil {
		return nil, fmt.Errorf("bri client not configured")
	}

	reff, cardNo, amount, err := parseBRIZZICheckRef(refID)
	if err != nil {
		return nil, err
	}

	startTime := time.Now()
	resp, err := c.client.BRIZZICheckTopupStatus(ctx, cardNo, amount, reff)
	responseTime := time.Since(startTime)
	if err != nil {
		c.markUnhealthy()
		return nil, err
	}

	c.markHealthy()
	rawResp, _ := json.Marshal(resp)

	return &ProviderResponse{
		Success:       resp.ResponseCode == "00" && !strings.EqualFold(strings.TrimSpace(resp.Data.Reversal), "TRUE"),
		Pending:       isBRIZZIPending(resp.ResponseCode),
		RefID:         refID,
		ProviderRefID: refID,
		Status:        brizziStatus(resp.ResponseCode),
		RC:            resp.ResponseCode,
		Message:       resp.ResponseDescription,
		Amount:        amount,
		Description:   rawResp,
		RawResponse:   rawResp,
		ResponseTime:  responseTime,
	}, nil
}

func (c *BRIProviderClient) GetPriceList(_ context.Context, category string) ([]ProviderProduct, error) {
	if category != "" && !strings.EqualFold(category, "e-money") && !strings.EqualFold(category, "emoney") {
		return []ProviderProduct{}, nil
	}

	products := make([]ProviderProduct, 0, len(c.denominations))
	for _, amount := range c.denominations {
		admin := 0
		products = append(products, ProviderProduct{
			SKUCode:     strconv.Itoa(amount),
			ProductName: fmt.Sprintf("BRIZZI Top Up %d", amount),
			Category:    "e-money",
			Brand:       "BRIZZI",
			Price:       amount,
			Admin:       &admin,
			IsActive:    true,
		})
	}
	return products, nil
}

func (c *BRIProviderClient) IsHealthy() bool {
	c.healthMu.RLock()
	defer c.healthMu.RUnlock()
	return c.healthy
}

func (c *BRIProviderClient) markHealthy() {
	c.healthMu.Lock()
	defer c.healthMu.Unlock()
	c.healthy = true
}

func (c *BRIProviderClient) markUnhealthy() {
	c.healthMu.Lock()
	defer c.healthMu.Unlock()
	c.healthy = false
}

func parseBRIZZIAmount(req *ProviderRequest) (int, error) {
	if req == nil {
		return 0, fmt.Errorf("provider request is required")
	}
	if req.Amount > 0 {
		return req.Amount, nil
	}
	amount, err := strconv.Atoi(strings.TrimSpace(req.SKUCode))
	if err == nil && amount > 0 {
		return amount, nil
	}
	if req.Extra != nil {
		switch v := req.Extra["amount"].(type) {
		case int:
			if v > 0 {
				return v, nil
			}
		case int64:
			if v > 0 {
				return int(v), nil
			}
		case float64:
			if v > 0 {
				return int(v), nil
			}
		case string:
			n, convErr := strconv.Atoi(strings.TrimSpace(v))
			if convErr == nil && n > 0 {
				return n, nil
			}
		}
	}
	return 0, fmt.Errorf("unable to determine BRIZZI topup amount from provider SKU code")
}

func buildBRIZZICheckRef(reff, cardNo string, amount int) string {
	return strings.TrimSpace(reff) + "|" + strings.TrimSpace(cardNo) + "|" + strconv.Itoa(amount)
}

func parseBRIZZICheckRef(value string) (string, string, int, error) {
	parts := strings.Split(strings.TrimSpace(value), "|")
	if len(parts) != 3 {
		return "", "", 0, fmt.Errorf("invalid BRIZZI provider reference format")
	}
	amount, err := strconv.Atoi(strings.TrimSpace(parts[2]))
	if err != nil {
		return "", "", 0, err
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), amount, nil
}

func isBRIZZIPending(code string) bool {
	switch strings.ToUpper(strings.TrimSpace(code)) {
	case "Q1", "Q4", "99", "0902":
		return true
	default:
		return false
	}
}

func brizziStatus(code string) string {
	if strings.TrimSpace(code) == "00" {
		return "Success"
	}
	if isBRIZZIPending(code) {
		return "Pending"
	}
	return "Failed"
}

func nonEmptyString(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
