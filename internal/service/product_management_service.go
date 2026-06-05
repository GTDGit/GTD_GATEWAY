package service

import (
	"context"
	"database/sql"
	"errors"

	"github.com/GTDGit/gtd_gateway/internal/models"
	"github.com/GTDGit/gtd_gateway/internal/repository"
)

// ProductManagementService handles product CRUD operations.
type ProductManagementService struct {
	productRepo   *repository.ProductRepository
	skuRepo       *repository.SKURepository
	masterService *ProductMasterService
}

// NewProductManagementService constructs a ProductManagementService.
func NewProductManagementService(productRepo *repository.ProductRepository, skuRepo *repository.SKURepository, masterService *ProductMasterService) *ProductManagementService {
	return &ProductManagementService{
		productRepo:   productRepo,
		skuRepo:       skuRepo,
		masterService: masterService,
	}
}

// CreateProductRequest represents the request to create a new product.
type CreateProductRequest struct {
	SKUCode     string `json:"skuCode" binding:"required"`
	Name        string `json:"name" binding:"required"`
	Category    string `json:"category" binding:"required"`
	Brand       string `json:"brand" binding:"required"`
	Type        string `json:"type" binding:"required"` // "prepaid" or "postpaid" only
	VariantID   *int   `json:"variantId"`             // optional: Reguler, Pulsa Transfer, etc
	Admin       int    `json:"admin"`
	Commission  int    `json:"commission"`
	Description string `json:"description"`
}

// UpdateProductRequest represents the request to update a product.
type UpdateProductRequest struct {
	SKUCode     string `json:"skuCode"`
	Name        string `json:"name"`
	Category    string `json:"category"`
	Brand       string `json:"brand"`
	Type        string `json:"type"`      // prepaid or postpaid
	VariantID   *int   `json:"variantId"` // optional
	Admin       int    `json:"admin"`
	Commission  int    `json:"commission"`
	Description string `json:"description"`
	IsActive    *bool  `json:"isActive"`
}

// CreateSKURequest and UpdateSKURequest unchanged
type CreateSKURequest struct {
	DigiSKUCode    string `json:"digiSkuCode" binding:"required"`
	SellerName     string `json:"sellerName"`
	Priority       int    `json:"priority" binding:"required"`
	Price          int    `json:"price" binding:"required"`
	IsActive       bool   `json:"isActive"`
	SupportMulti   bool   `json:"supportMulti"`
	UnlimitedStock bool   `json:"unlimitedStock"`
	Stock          int    `json:"stock"`
	CutOffStart    string `json:"cutOffStart"`
	CutOffEnd      string `json:"cutOffEnd"`
}

type UpdateSKURequest struct {
	DigiSKUCode    string `json:"digiSkuCode"`
	SellerName     string `json:"sellerName"`
	Priority       *int   `json:"priority"`
	Price          *int   `json:"price"`
	IsActive       *bool  `json:"isActive"`
	SupportMulti   *bool  `json:"supportMulti"`
	UnlimitedStock *bool  `json:"unlimitedStock"`
	Stock          *int   `json:"stock"`
	CutOffStart    string `json:"cutOffStart"`
	CutOffEnd      string `json:"cutOffEnd"`
}

// CreateProduct creates a new product.
func (s *ProductManagementService) CreateProduct(ctx context.Context, req *CreateProductRequest) (*models.Product, error) {
	if req.Type != "prepaid" && req.Type != "postpaid" {
		return nil, errors.New("type must be 'prepaid' or 'postpaid'")
	}
	if err := s.masterService.ValidateCategoryBrandVariant(req.Category, req.Brand, req.VariantID); err != nil {
		return nil, err
	}
	existing, _ := s.productRepo.GetBySKUCode(req.SKUCode)
	if existing != nil {
		return nil, errors.New("sku_code already exists")
	}

	product := &models.Product{
		SkuCode:     req.SKUCode,
		Name:        req.Name,
		Category:    req.Category,
		Brand:       req.Brand,
		Type:        models.ProductType(req.Type),
		VariantID:   req.VariantID,
		Admin:       req.Admin,
		Commission:  req.Commission,
		Description: req.Description,
		IsActive:    false,
	}

	if err := s.productRepo.Create(product); err != nil {
		return nil, err
	}
	return product, nil
}

// GetProduct retrieves a product by ID.
func (s *ProductManagementService) GetProduct(id int) (*models.Product, error) {
	product, err := s.productRepo.GetByID(id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.New("product not found")
		}
		return nil, err
	}
	return product, nil
}

// UpdateProduct updates a product.
func (s *ProductManagementService) UpdateProduct(id int, req *UpdateProductRequest) (*models.Product, error) {
	product, err := s.productRepo.GetByID(id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.New("product not found")
		}
		return nil, err
	}

	if req.SKUCode != "" {
		if req.SKUCode != product.SkuCode {
			existing, _ := s.productRepo.GetBySKUCode(req.SKUCode)
			if existing != nil {
				return nil, errors.New("sku_code already exists")
			}
		}
		product.SkuCode = req.SKUCode
	}
	if req.Name != "" {
		product.Name = req.Name
	}
	if req.Category != "" {
		if err := s.masterService.ValidateCategoryBrandVariant(req.Category, product.Brand, product.VariantID); err != nil {
			return nil, err
		}
		product.Category = req.Category
	}
	if req.Brand != "" {
		if err := s.masterService.ValidateCategoryBrandVariant(product.Category, req.Brand, product.VariantID); err != nil {
			return nil, err
		}
		product.Brand = req.Brand
	}
	if req.Type != "" {
		if req.Type != "prepaid" && req.Type != "postpaid" {
			return nil, errors.New("type must be 'prepaid' or 'postpaid'")
		}
		product.Type = models.ProductType(req.Type)
	}
	if req.VariantID != nil {
		variantID := req.VariantID
		if *variantID == 0 {
			variantID = nil
		}
		if err := s.masterService.ValidateCategoryBrandVariant(product.Category, product.Brand, variantID); err != nil {
			return nil, err
		}
		product.VariantID = variantID
	}
	if req.Admin > 0 {
		product.Admin = req.Admin
	}
	if req.Commission > 0 {
		product.Commission = req.Commission
	}
	if req.Description != "" {
		product.Description = req.Description
	}
	if req.IsActive != nil {
		product.IsActive = *req.IsActive
	}

	if err := s.productRepo.Update(product); err != nil {
		return nil, err
	}
	return product, nil
}

// DeleteProduct deletes a product.
func (s *ProductManagementService) DeleteProduct(id int) error {
	product, err := s.productRepo.GetByID(id)
	if err != nil {
		if err == sql.ErrNoRows {
			return errors.New("product not found")
		}
		return err
	}
	return s.productRepo.Delete(product.ID)
}

// CreateSKU, UpdateSKU, DeleteSKU, GetProductSKUs - unchanged logic
func (s *ProductManagementService) CreateSKU(productID int, req *CreateSKURequest) (*models.SKU, error) {
	if req.Priority < 1 || req.Priority > 3 {
		return nil, errors.New("priority must be 1, 2, or 3")
	}
	_, err := s.productRepo.GetByID(productID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.New("product not found")
		}
		return nil, err
	}
	existing, _ := s.skuRepo.GetByDigiSKUCode(req.DigiSKUCode)
	if existing != nil {
		return nil, errors.New("digi_sku_code already exists")
	}
	sku := &models.SKU{
		ProductID: productID, DigiSkuCode: req.DigiSKUCode, SellerName: req.SellerName,
		Priority: req.Priority, Price: req.Price, IsActive: req.IsActive,
		SupportMulti: req.SupportMulti, UnlimitedStock: req.UnlimitedStock, Stock: req.Stock,
		CutOffStart: req.CutOffStart, CutOffEnd: req.CutOffEnd,
	}
	if err := s.skuRepo.Create(sku); err != nil {
		return nil, err
	}
	return sku, nil
}

func (s *ProductManagementService) GetSKU(id int) (*models.SKU, error) {
	sku, err := s.skuRepo.GetByID(id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.New("sku not found")
		}
		return nil, err
	}
	return sku, nil
}

func (s *ProductManagementService) UpdateSKU(id int, req *UpdateSKURequest) (*models.SKU, error) {
	sku, err := s.skuRepo.GetByID(id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.New("sku not found")
		}
		return nil, err
	}
	if req.DigiSKUCode != "" {
		if req.DigiSKUCode != sku.DigiSkuCode {
			existing, _ := s.skuRepo.GetByDigiSKUCode(req.DigiSKUCode)
			if existing != nil {
				return nil, errors.New("digi_sku_code already exists")
			}
		}
		sku.DigiSkuCode = req.DigiSKUCode
	}
	if req.SellerName != "" {
		sku.SellerName = req.SellerName
	}
	if req.Priority != nil {
		if *req.Priority < 1 || *req.Priority > 3 {
			return nil, errors.New("priority must be 1, 2, or 3")
		}
		sku.Priority = *req.Priority
	}
	if req.Price != nil {
		sku.Price = *req.Price
	}
	if req.IsActive != nil {
		sku.IsActive = *req.IsActive
	}
	if req.SupportMulti != nil {
		sku.SupportMulti = *req.SupportMulti
	}
	if req.UnlimitedStock != nil {
		sku.UnlimitedStock = *req.UnlimitedStock
	}
	if req.Stock != nil {
		sku.Stock = *req.Stock
	}
	if req.CutOffStart != "" {
		sku.CutOffStart = req.CutOffStart
	}
	if req.CutOffEnd != "" {
		sku.CutOffEnd = req.CutOffEnd
	}
	if err := s.skuRepo.Update(sku); err != nil {
		return nil, err
	}
	return sku, nil
}

func (s *ProductManagementService) DeleteSKU(id int) error {
	sku, err := s.skuRepo.GetByID(id)
	if err != nil {
		if err == sql.ErrNoRows {
			return errors.New("sku not found")
		}
		return err
	}
	return s.skuRepo.Delete(sku.ID)
}

func (s *ProductManagementService) GetProductSKUs(productID int) ([]*models.SKU, error) {
	_, err := s.productRepo.GetByID(productID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.New("product not found")
		}
		return nil, err
	}
	skus, err := s.skuRepo.GetByProductID(productID)
	if err != nil {
		return nil, err
	}
	result := make([]*models.SKU, len(skus))
	for i := range skus {
		result[i] = &skus[i]
	}
	return result, nil
}

// ListProductsFilter - add VariantID
type ListProductsFilter struct {
	Type      string
	VariantID *int
	Category  string
	Brand     string
	Search    string
	IsActive  *bool
	Page      int
	Limit     int
}

func (s *ProductManagementService) ListProducts(filter *ListProductsFilter) (*repository.AdminProductResult, error) {
	repoFilter := &repository.AdminProductFilter{
		Type:      filter.Type,
		VariantID: filter.VariantID,
		Category:  filter.Category,
		Brand:     filter.Brand,
		Search:    filter.Search,
		IsActive:  filter.IsActive,
		Page:      filter.Page,
		Limit:     filter.Limit,
	}
	return s.productRepo.GetAllAdmin(repoFilter)
}

func (s *ProductManagementService) GetCategories(productType string) ([]string, error) {
	return s.productRepo.GetDistinctCategories(productType)
}

func (s *ProductManagementService) GetBrands(category, productType string) ([]string, error) {
	return s.productRepo.GetDistinctBrands(category, productType)
}

// GetVariants returns all product variants (Reguler, Pulsa Transfer, etc) for dropdown.
func (s *ProductManagementService) GetVariants() ([]repository.ProductVariant, error) {
	return s.masterService.ListVariants()
}
