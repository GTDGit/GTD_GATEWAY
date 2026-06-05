package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/GTDGit/gtd_gateway/internal/models"
	"github.com/GTDGit/gtd_gateway/internal/repository"
	"github.com/GTDGit/gtd_gateway/internal/utils"
)

// PPOBProviderHandler handles PPOB provider management HTTP endpoints.
type PPOBProviderHandler struct {
	providerRepo *repository.PPOBProviderRepository
}

// NewPPOBProviderHandler constructs a PPOBProviderHandler.
func NewPPOBProviderHandler(providerRepo *repository.PPOBProviderRepository) *PPOBProviderHandler {
	return &PPOBProviderHandler{providerRepo: providerRepo}
}

// ============================================
// Provider Endpoints
// ============================================

// ListProviders handles GET /v1/admin/ppob/providers
func (h *PPOBProviderHandler) ListProviders(c *gin.Context) {
	activeOnly := c.Query("active") == "true"

	providers, err := h.providerRepo.GetAllProviders(activeOnly)
	if err != nil {
		utils.Error(c, 500, "INTERNAL_ERROR", "Failed to retrieve providers")
		return
	}

	utils.Success(c, 200, "Providers retrieved", providers)
}

// GetProvider handles GET /v1/admin/ppob/providers/:id
func (h *PPOBProviderHandler) GetProvider(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.Error(c, 400, "INVALID_ID", "Invalid provider ID")
		return
	}

	provider, err := h.providerRepo.GetProviderByID(id)
	if err != nil {
		utils.Error(c, 404, "NOT_FOUND", "Provider not found")
		return
	}

	utils.Success(c, 200, "Provider retrieved", provider)
}

// UpdateProviderStatus handles PATCH /v1/admin/ppob/providers/:id/status
func (h *PPOBProviderHandler) UpdateProviderStatus(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.Error(c, 400, "INVALID_ID", "Invalid provider ID")
		return
	}

	var req struct {
		IsActive bool `json:"isActive"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, 400, "INVALID_REQUEST", "Invalid request body")
		return
	}

	if err := h.providerRepo.UpdateProviderStatus(id, req.IsActive); err != nil {
		utils.Error(c, 500, "INTERNAL_ERROR", "Failed to update provider status")
		return
	}

	utils.Success(c, 200, "Provider status updated", nil)
}

// ============================================
// Provider SKU Endpoints
// ============================================

// CreateProviderSKURequest represents request for creating provider SKU
type CreateProviderSKURequest struct {
	ProviderID          int    `json:"-"` // Set from URL param
	ProductID           int    `json:"productId" binding:"required"`
	ProviderSKUCode     string `json:"providerSkuCode" binding:"required"`
	ProviderProductName string `json:"providerProductName"`
	Price               int    `json:"price"`
	Admin               int    `json:"admin"`
	Commission          int    `json:"commission"`
	IsActive            bool   `json:"isActive"`
}

// ListProviderSKUs handles GET /v1/admin/ppob/providers/:id/skus
func (h *PPOBProviderHandler) ListProviderSKUs(c *gin.Context) {
	providerID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.Error(c, 400, "INVALID_ID", "Invalid provider ID")
		return
	}

	search := c.Query("search")
	page := 1
	limit := 50

	if p := c.Query("page"); p != "" {
		page, _ = strconv.Atoi(p)
	}
	if l := c.Query("limit"); l != "" {
		limit, _ = strconv.Atoi(l)
	}

	skus, total, err := h.providerRepo.GetAllProviderSKUsPaged(providerID, search, page, limit)
	if err != nil {
		utils.Error(c, 500, "INTERNAL_ERROR", "Failed to retrieve provider SKUs")
		return
	}

	totalPages := (total + limit - 1) / limit

	c.JSON(200, gin.H{
		"success": true,
		"code":    200,
		"message": "Provider SKUs retrieved",
		"data":    skus,
		"meta": gin.H{
			"requestId": c.GetString("requestId"),
			"timestamp": utils.NowISO(),
			"pagination": gin.H{
				"page":       page,
				"limit":      limit,
				"totalItems": total,
				"totalPages": totalPages,
			},
		},
	})
}

// GetProviderSKUsByProduct handles GET /v1/admin/ppob/products/:productId/provider-skus
func (h *PPOBProviderHandler) GetProviderSKUsByProduct(c *gin.Context) {
	productID, err := strconv.Atoi(c.Param("productId"))
	if err != nil {
		utils.Error(c, 400, "INVALID_ID", "Invalid product ID")
		return
	}

	activeOnly := c.Query("active") == "true"

	skus, err := h.providerRepo.GetProviderSKUsByProduct(productID, activeOnly)
	if err != nil {
		utils.Error(c, 500, "INTERNAL_ERROR", "Failed to retrieve provider SKUs")
		return
	}

	utils.Success(c, 200, "Provider SKUs retrieved", skus)
}

// GetProviderSKU handles GET /v1/admin/ppob/provider-skus/:id
func (h *PPOBProviderHandler) GetProviderSKU(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.Error(c, 400, "INVALID_ID", "Invalid SKU ID")
		return
	}

	sku, err := h.providerRepo.GetProviderSKUByID(id)
	if err != nil {
		utils.Error(c, 404, "NOT_FOUND", "Provider SKU not found")
		return
	}

	utils.Success(c, 200, "Provider SKU retrieved", sku)
}

// CreateProviderSKU handles POST /v1/admin/ppob/providers/:id/skus
func (h *PPOBProviderHandler) CreateProviderSKU(c *gin.Context) {
	providerID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.Error(c, 400, "INVALID_ID", "Invalid provider ID")
		return
	}

	var req CreateProviderSKURequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, 400, "INVALID_REQUEST", "Invalid request body")
		return
	}

	// Use provider ID from URL param
	req.ProviderID = providerID

	// Check if mapping already exists
	existing, err := h.providerRepo.GetProviderSKUByProviderAndProduct(req.ProviderID, req.ProductID)
	if err != nil {
		utils.Error(c, 500, "INTERNAL_ERROR", "Failed to check existing SKU mapping")
		return
	}
	if existing != nil {
		utils.Error(c, 400, "DUPLICATE", "Provider SKU mapping already exists for this product")
		return
	}

	sku := &models.PPOBProviderSKU{
		ProviderID:          req.ProviderID,
		ProductID:           req.ProductID,
		ProviderSKUCode:     req.ProviderSKUCode,
		ProviderProductName: req.ProviderProductName,
		Price:               req.Price,
		Admin:               req.Admin,
		Commission:          req.Commission,
		IsActive:            req.IsActive,
		IsAvailable:         true,
	}

	if err := h.providerRepo.CreateProviderSKU(sku); err != nil {
		utils.Error(c, 500, "INTERNAL_ERROR", "Failed to create provider SKU")
		return
	}

	utils.Success(c, 201, "Provider SKU created", sku)
}

// UpdateProviderSKURequest represents request for updating provider SKU
type UpdateProviderSKURequest struct {
	ProviderSKUCode     string `json:"providerSkuCode"`
	ProviderProductName string `json:"providerProductName"`
	Price               int    `json:"price"`
	Admin               int    `json:"admin"`
	Commission          int    `json:"commission"`
	IsActive            bool   `json:"isActive"`
	IsAvailable         bool   `json:"isAvailable"`
}

// UpdateProviderSKU handles PUT /v1/admin/ppob/provider-skus/:id
func (h *PPOBProviderHandler) UpdateProviderSKU(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.Error(c, 400, "INVALID_ID", "Invalid SKU ID")
		return
	}

	var req UpdateProviderSKURequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, 400, "INVALID_REQUEST", "Invalid request body")
		return
	}

	sku, err := h.providerRepo.GetProviderSKUByID(id)
	if err != nil {
		utils.Error(c, 404, "NOT_FOUND", "Provider SKU not found")
		return
	}

	// Update fields
	sku.ProviderSKUCode = req.ProviderSKUCode
	sku.ProviderProductName = req.ProviderProductName
	sku.Price = req.Price
	sku.Admin = req.Admin
	sku.Commission = req.Commission
	sku.IsActive = req.IsActive
	sku.IsAvailable = req.IsAvailable

	if err := h.providerRepo.UpdateProviderSKU(sku); err != nil {
		utils.Error(c, 500, "INTERNAL_ERROR", "Failed to update provider SKU")
		return
	}

	utils.Success(c, 200, "Provider SKU updated", sku)
}

// DeleteProviderSKU handles DELETE /v1/admin/ppob/provider-skus/:id
func (h *PPOBProviderHandler) DeleteProviderSKU(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.Error(c, 400, "INVALID_ID", "Invalid SKU ID")
		return
	}

	if err := h.providerRepo.DeleteProviderSKU(id); err != nil {
		utils.Error(c, 500, "INTERNAL_ERROR", "Failed to delete provider SKU")
		return
	}

	utils.Success(c, 200, "Provider SKU deleted", nil)
}

// ============================================
// Provider Health Endpoints
// ============================================

// GetProviderHealth handles GET /v1/admin/ppob/providers/:id/health
func (h *PPOBProviderHandler) GetProviderHealth(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.Error(c, 400, "INVALID_ID", "Invalid provider ID")
		return
	}

	health, err := h.providerRepo.GetProviderHealth(id)
	if err != nil {
		utils.Error(c, 500, "INTERNAL_ERROR", "Failed to retrieve provider health")
		return
	}

	if health == nil {
		// No health data yet for today
		utils.Success(c, 200, "No health data available", nil)
		return
	}

	utils.Success(c, 200, "Provider health retrieved", health)
}

// GetAllProviderHealthToday handles GET /v1/admin/ppob/providers/health
func (h *PPOBProviderHandler) GetAllProviderHealthToday(c *gin.Context) {
	health, err := h.providerRepo.GetAllProviderHealthToday()
	if err != nil {
		utils.Error(c, 500, "INTERNAL_ERROR", "Failed to retrieve provider health")
		return
	}

	utils.Success(c, 200, "Provider health retrieved", health)
}
