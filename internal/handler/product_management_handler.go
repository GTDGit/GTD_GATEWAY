package handler

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/GTDGit/gtd_gateway/internal/service"
	"github.com/GTDGit/gtd_gateway/internal/utils"
)

// ProductManagementHandler handles product CRUD HTTP endpoints.
type ProductManagementHandler struct {
	productMgmtService *service.ProductManagementService
}

// NewProductManagementHandler constructs a ProductManagementHandler.
func NewProductManagementHandler(productMgmtService *service.ProductManagementService) *ProductManagementHandler {
	return &ProductManagementHandler{productMgmtService: productMgmtService}
}

// ListProducts handles GET /v1/admin/products
func (h *ProductManagementHandler) ListProducts(c *gin.Context) {
	// Parse query parameters
	filter := &service.ListProductsFilter{
		Type:     c.Query("type"),
		Category: c.Query("category"),
		Brand:    c.Query("brand"),
		Search:   c.Query("search"),
		Page:     1,
		Limit:    50,
	}
	if v := c.Query("variantId"); v != "" {
		if id, err := strconv.Atoi(v); err == nil {
			filter.VariantID = &id
		}
	}

	if page := c.Query("page"); page != "" {
		if p, err := strconv.Atoi(page); err == nil {
			filter.Page = p
		}
	}
	if limit := c.Query("limit"); limit != "" {
		if l, err := strconv.Atoi(limit); err == nil {
			filter.Limit = l
		}
	}
	if isActive := c.Query("isActive"); isActive != "" {
		active := isActive == "true"
		filter.IsActive = &active
	}

	result, err := h.productMgmtService.ListProducts(filter)
	if err != nil {
		utils.Error(c, 500, "INTERNAL_ERROR", "Failed to retrieve products")
		return
	}

	c.JSON(200, gin.H{
		"success": true,
		"code":    200,
		"message": "Products retrieved",
		"data":    result.Products,
		"meta": gin.H{
			"requestId": c.GetString("requestId"),
			"timestamp": utils.NowISO(),
			"pagination": gin.H{
				"page":       result.Page,
				"limit":      result.Limit,
				"totalItems": result.TotalItems,
				"totalPages": result.TotalPages,
			},
		},
	})
}

// CreateProduct handles POST /v1/admin/products
func (h *ProductManagementHandler) CreateProduct(c *gin.Context) {
	var req service.CreateProductRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, 400, "INVALID_REQUEST", "Invalid request body")
		return
	}

	product, err := h.productMgmtService.CreateProduct(c.Request.Context(), &req)
	if err != nil {
		if err.Error() == "sku_code already exists" {
			utils.Error(c, 400, "SKU_EXISTS", err.Error())
			return
		}
		if err.Error() == "type must be 'prepaid' or 'postpaid'" {
			utils.Error(c, 400, "INVALID_TYPE", err.Error())
			return
		}
		if strings.Contains(err.Error(), "invalid") || strings.Contains(err.Error(), "required") || strings.Contains(err.Error(), "not found in master") {
			utils.Error(c, 400, "INVALID_REQUEST", err.Error())
			return
		}
		utils.Error(c, 500, "INTERNAL_ERROR", "Failed to create product")
		return
	}

	utils.Success(c, 201, "Product created successfully", product)
}

// GetProduct handles GET /v1/admin/products/:id
func (h *ProductManagementHandler) GetProduct(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.Error(c, 400, "INVALID_ID", "Invalid product ID")
		return
	}

	product, err := h.productMgmtService.GetProduct(id)
	if err != nil {
		utils.Error(c, 404, "PRODUCT_NOT_FOUND", "Product not found")
		return
	}

	utils.Success(c, 200, "Product retrieved", product)
}

// UpdateProduct handles PUT /v1/admin/products/:id
func (h *ProductManagementHandler) UpdateProduct(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.Error(c, 400, "INVALID_ID", "Invalid product ID")
		return
	}

	var req service.UpdateProductRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, 400, "INVALID_REQUEST", "Invalid request body")
		return
	}

	product, err := h.productMgmtService.UpdateProduct(id, &req)
	if err != nil {
		if err.Error() == "product not found" {
			utils.Error(c, 404, "PRODUCT_NOT_FOUND", err.Error())
			return
		}
		if err.Error() == "sku_code already exists" {
			utils.Error(c, 400, "SKU_EXISTS", err.Error())
			return
		}
		if err.Error() == "type must be 'prepaid' or 'postpaid'" {
			utils.Error(c, 400, "INVALID_TYPE", err.Error())
			return
		}
		if strings.Contains(err.Error(), "invalid") || strings.Contains(err.Error(), "required") || strings.Contains(err.Error(), "not found in master") {
			utils.Error(c, 400, "INVALID_REQUEST", err.Error())
			return
		}
		utils.Error(c, 500, "INTERNAL_ERROR", "Failed to update product")
		return
	}

	utils.Success(c, 200, "Product updated successfully", product)
}

// DeleteProduct handles DELETE /v1/admin/products/:id
func (h *ProductManagementHandler) DeleteProduct(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.Error(c, 400, "INVALID_ID", "Invalid product ID")
		return
	}

	if err := h.productMgmtService.DeleteProduct(id); err != nil {
		if err.Error() == "product not found" {
			utils.Error(c, 404, "PRODUCT_NOT_FOUND", err.Error())
			return
		}
		utils.Error(c, 500, "INTERNAL_ERROR", "Failed to delete product")
		return
	}

	utils.Success(c, 200, "Product deleted successfully", nil)
}

// CreateSKU handles POST /v1/admin/products/:id/skus
func (h *ProductManagementHandler) CreateSKU(c *gin.Context) {
	productID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.Error(c, 400, "INVALID_ID", "Invalid product ID")
		return
	}

	var req service.CreateSKURequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, 400, "INVALID_REQUEST", "Invalid request body")
		return
	}

	sku, err := h.productMgmtService.CreateSKU(productID, &req)
	if err != nil {
		if err.Error() == "product not found" {
			utils.Error(c, 404, "PRODUCT_NOT_FOUND", err.Error())
			return
		}
		if err.Error() == "digi_sku_code already exists" {
			utils.Error(c, 400, "SKU_EXISTS", err.Error())
			return
		}
		if err.Error() == "priority must be 1, 2, or 3" {
			utils.Error(c, 400, "INVALID_PRIORITY", err.Error())
			return
		}
		utils.Error(c, 500, "INTERNAL_ERROR", "Failed to create SKU")
		return
	}

	utils.Success(c, 201, "SKU created successfully", sku)
}

// GetProductSKUs handles GET /v1/admin/products/:id/skus
func (h *ProductManagementHandler) GetProductSKUs(c *gin.Context) {
	productID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.Error(c, 400, "INVALID_ID", "Invalid product ID")
		return
	}

	skus, err := h.productMgmtService.GetProductSKUs(productID)
	if err != nil {
		if err.Error() == "product not found" {
			utils.Error(c, 404, "PRODUCT_NOT_FOUND", err.Error())
			return
		}
		utils.Error(c, 500, "INTERNAL_ERROR", "Failed to retrieve SKUs")
		return
	}

	utils.Success(c, 200, "SKUs retrieved", gin.H{
		"skus":  skus,
		"total": len(skus),
	})
}

// GetSKU handles GET /v1/admin/skus/:id
func (h *ProductManagementHandler) GetSKU(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.Error(c, 400, "INVALID_ID", "Invalid SKU ID")
		return
	}

	sku, err := h.productMgmtService.GetSKU(id)
	if err != nil {
		utils.Error(c, 404, "SKU_NOT_FOUND", "SKU not found")
		return
	}

	utils.Success(c, 200, "SKU retrieved", sku)
}

// UpdateSKU handles PUT /v1/admin/skus/:id
func (h *ProductManagementHandler) UpdateSKU(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.Error(c, 400, "INVALID_ID", "Invalid SKU ID")
		return
	}

	var req service.UpdateSKURequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, 400, "INVALID_REQUEST", "Invalid request body")
		return
	}

	sku, err := h.productMgmtService.UpdateSKU(id, &req)
	if err != nil {
		if err.Error() == "sku not found" {
			utils.Error(c, 404, "SKU_NOT_FOUND", err.Error())
			return
		}
		if err.Error() == "digi_sku_code already exists" {
			utils.Error(c, 400, "SKU_EXISTS", err.Error())
			return
		}
		if err.Error() == "priority must be 1, 2, or 3" {
			utils.Error(c, 400, "INVALID_PRIORITY", err.Error())
			return
		}
		utils.Error(c, 500, "INTERNAL_ERROR", "Failed to update SKU")
		return
	}

	utils.Success(c, 200, "SKU updated successfully", sku)
}

// DeleteSKU handles DELETE /v1/admin/skus/:id
func (h *ProductManagementHandler) DeleteSKU(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.Error(c, 400, "INVALID_ID", "Invalid SKU ID")
		return
	}

	if err := h.productMgmtService.DeleteSKU(id); err != nil {
		if err.Error() == "sku not found" {
			utils.Error(c, 404, "SKU_NOT_FOUND", err.Error())
			return
		}
		utils.Error(c, 500, "INTERNAL_ERROR", "Failed to delete SKU")
		return
	}

	utils.Success(c, 200, "SKU deleted successfully", nil)
}

// GetCategories handles GET /v1/admin/products/categories?type=prepaid
func (h *ProductManagementHandler) GetCategories(c *gin.Context) {
	categories, err := h.productMgmtService.GetCategories(c.Query("type"))
	if err != nil {
		utils.Error(c, 500, "INTERNAL_ERROR", "Failed to retrieve categories")
		return
	}

	utils.Success(c, 200, "Categories retrieved", categories)
}

// GetBrands handles GET /v1/admin/products/brands?category=Pulsa&type=prepaid
func (h *ProductManagementHandler) GetBrands(c *gin.Context) {
	brands, err := h.productMgmtService.GetBrands(c.Query("category"), c.Query("type"))
	if err != nil {
		utils.Error(c, 500, "INTERNAL_ERROR", "Failed to retrieve brands")
		return
	}
	utils.Success(c, 200, "Brands retrieved", brands)
}

// GetVariants handles GET /v1/admin/products/variants
func (h *ProductManagementHandler) GetVariants(c *gin.Context) {
	variants, err := h.productMgmtService.GetVariants()
	if err != nil {
		utils.Error(c, 500, "INTERNAL_ERROR", "Failed to retrieve variants")
		return
	}
	utils.Success(c, 200, "Variants retrieved", variants)
}
