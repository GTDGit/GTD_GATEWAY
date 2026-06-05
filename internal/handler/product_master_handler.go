package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/GTDGit/gtd_gateway/internal/service"
	"github.com/GTDGit/gtd_gateway/internal/utils"
)

// ProductMasterHandler handles CRUD for categories, brands, variants.
type ProductMasterHandler struct {
	svc *service.ProductMasterService
}

// NewProductMasterHandler creates a new ProductMasterHandler.
func NewProductMasterHandler(svc *service.ProductMasterService) *ProductMasterHandler {
	return &ProductMasterHandler{svc: svc}
}

// --- Categories ---

func (h *ProductMasterHandler) ListCategories(c *gin.Context) {
	list, err := h.svc.ListCategories()
	if err != nil {
		utils.Error(c, 500, "INTERNAL_ERROR", "Failed to list categories")
		return
	}
	utils.Success(c, 200, "Categories retrieved", list)
}

func (h *ProductMasterHandler) CreateCategory(c *gin.Context) {
	var req struct {
		Name         string `json:"name" binding:"required"`
		DisplayOrder int    `json:"displayOrder"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, 400, "INVALID_REQUEST", "Invalid request body")
		return
	}
	cat, err := h.svc.CreateCategory(c.Request.Context(), req.Name, req.DisplayOrder)
	if err != nil {
		if err.Error() == "category name is required" || err.Error() == "category already exists" {
			utils.Error(c, 400, "INVALID_REQUEST", err.Error())
			return
		}
		utils.Error(c, 500, "INTERNAL_ERROR", "Failed to create category")
		return
	}
	utils.Success(c, 201, "Category created", cat)
}

func (h *ProductMasterHandler) UpdateCategory(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.Error(c, 400, "INVALID_ID", "Invalid category ID")
		return
	}
	var req struct {
		Name         string `json:"name" binding:"required"`
		DisplayOrder int    `json:"displayOrder"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, 400, "INVALID_REQUEST", "Invalid request body")
		return
	}
	if err := h.svc.UpdateCategory(c.Request.Context(), id, req.Name, req.DisplayOrder); err != nil {
		if err.Error() == "category not found" {
			utils.Error(c, 404, "NOT_FOUND", err.Error())
			return
		}
		if err.Error() == "category name is required" || err.Error() == "category name already used by another" {
			utils.Error(c, 400, "INVALID_REQUEST", err.Error())
			return
		}
		utils.Error(c, 500, "INTERNAL_ERROR", "Failed to update category")
		return
	}
	utils.Success(c, 200, "Category updated", nil)
}

func (h *ProductMasterHandler) DeleteCategory(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.Error(c, 400, "INVALID_ID", "Invalid category ID")
		return
	}
	if err := h.svc.DeleteCategory(c.Request.Context(), id); err != nil {
		if err.Error() == "category not found" {
			utils.Error(c, 404, "NOT_FOUND", err.Error())
			return
		}
		if err.Error() == "cannot delete: category is used by products" {
			utils.Error(c, 400, "IN_USE", err.Error())
			return
		}
		utils.Error(c, 500, "INTERNAL_ERROR", "Failed to delete category")
		return
	}
	utils.Success(c, 200, "Category deleted", nil)
}

// --- Brands ---

func (h *ProductMasterHandler) ListBrands(c *gin.Context) {
	list, err := h.svc.ListBrands()
	if err != nil {
		utils.Error(c, 500, "INTERNAL_ERROR", "Failed to list brands")
		return
	}
	utils.Success(c, 200, "Brands retrieved", list)
}

func (h *ProductMasterHandler) CreateBrand(c *gin.Context) {
	var req struct {
		Name         string `json:"name" binding:"required"`
		DisplayOrder int    `json:"displayOrder"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, 400, "INVALID_REQUEST", "Invalid request body")
		return
	}
	brand, err := h.svc.CreateBrand(c.Request.Context(), req.Name, req.DisplayOrder)
	if err != nil {
		if err.Error() == "brand name is required" || err.Error() == "brand already exists" {
			utils.Error(c, 400, "INVALID_REQUEST", err.Error())
			return
		}
		utils.Error(c, 500, "INTERNAL_ERROR", "Failed to create brand")
		return
	}
	utils.Success(c, 201, "Brand created", brand)
}

func (h *ProductMasterHandler) UpdateBrand(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.Error(c, 400, "INVALID_ID", "Invalid brand ID")
		return
	}
	var req struct {
		Name         string `json:"name" binding:"required"`
		DisplayOrder int    `json:"displayOrder"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, 400, "INVALID_REQUEST", "Invalid request body")
		return
	}
	if err := h.svc.UpdateBrand(c.Request.Context(), id, req.Name, req.DisplayOrder); err != nil {
		if err.Error() == "brand not found" {
			utils.Error(c, 404, "NOT_FOUND", err.Error())
			return
		}
		if err.Error() == "brand name is required" || err.Error() == "brand name already used by another" {
			utils.Error(c, 400, "INVALID_REQUEST", err.Error())
			return
		}
		utils.Error(c, 500, "INTERNAL_ERROR", "Failed to update brand")
		return
	}
	utils.Success(c, 200, "Brand updated", nil)
}

func (h *ProductMasterHandler) DeleteBrand(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.Error(c, 400, "INVALID_ID", "Invalid brand ID")
		return
	}
	if err := h.svc.DeleteBrand(c.Request.Context(), id); err != nil {
		if err.Error() == "brand not found" {
			utils.Error(c, 404, "NOT_FOUND", err.Error())
			return
		}
		if err.Error() == "cannot delete: brand is used by products" {
			utils.Error(c, 400, "IN_USE", err.Error())
			return
		}
		utils.Error(c, 500, "INTERNAL_ERROR", "Failed to delete brand")
		return
	}
	utils.Success(c, 200, "Brand deleted", nil)
}

// --- Variants ---

func (h *ProductMasterHandler) ListVariants(c *gin.Context) {
	list, err := h.svc.ListVariants()
	if err != nil {
		utils.Error(c, 500, "INTERNAL_ERROR", "Failed to list variants")
		return
	}
	utils.Success(c, 200, "Variants retrieved", list)
}

func (h *ProductMasterHandler) CreateVariant(c *gin.Context) {
	var req struct {
		Name         string `json:"name" binding:"required"`
		DisplayOrder int    `json:"displayOrder"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, 400, "INVALID_REQUEST", "Invalid request body")
		return
	}
	v, err := h.svc.CreateVariant(c.Request.Context(), req.Name, req.DisplayOrder)
	if err != nil {
		if err.Error() == "variant name is required" || err.Error() == "variant already exists" {
			utils.Error(c, 400, "INVALID_REQUEST", err.Error())
			return
		}
		utils.Error(c, 500, "INTERNAL_ERROR", "Failed to create variant")
		return
	}
	utils.Success(c, 201, "Variant created", v)
}

func (h *ProductMasterHandler) UpdateVariant(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.Error(c, 400, "INVALID_ID", "Invalid variant ID")
		return
	}
	var req struct {
		Name         string `json:"name" binding:"required"`
		DisplayOrder int    `json:"displayOrder"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, 400, "INVALID_REQUEST", "Invalid request body")
		return
	}
	if err := h.svc.UpdateVariant(c.Request.Context(), id, req.Name, req.DisplayOrder); err != nil {
		if err.Error() == "variant not found" {
			utils.Error(c, 404, "NOT_FOUND", err.Error())
			return
		}
		if err.Error() == "variant name is required" || err.Error() == "variant name already used by another" {
			utils.Error(c, 400, "INVALID_REQUEST", err.Error())
			return
		}
		utils.Error(c, 500, "INTERNAL_ERROR", "Failed to update variant")
		return
	}
	utils.Success(c, 200, "Variant updated", nil)
}

func (h *ProductMasterHandler) DeleteVariant(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.Error(c, 400, "INVALID_ID", "Invalid variant ID")
		return
	}
	if err := h.svc.DeleteVariant(c.Request.Context(), id); err != nil {
		if err.Error() == "variant not found" {
			utils.Error(c, 404, "NOT_FOUND", err.Error())
			return
		}
		if err.Error() == "cannot delete: variant is used by products" {
			utils.Error(c, 400, "IN_USE", err.Error())
			return
		}
		utils.Error(c, 500, "INTERNAL_ERROR", "Failed to delete variant")
		return
	}
	utils.Success(c, 200, "Variant deleted", nil)
}
