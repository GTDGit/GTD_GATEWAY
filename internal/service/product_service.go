package service

import (
	"time"

	"github.com/GTDGit/gtd_gateway/internal/models"
	"github.com/GTDGit/gtd_gateway/internal/repository"
)

// ProductService provides product-related business logic.
type ProductService struct {
	productRepo  *repository.ProductRepository
	skuRepo      *repository.SKURepository
	providerRepo *repository.PPOBProviderRepository
}

// NewProductService constructs a ProductService.
func NewProductService(productRepo *repository.ProductRepository, skuRepo *repository.SKURepository) *ProductService {
	return &ProductService{productRepo: productRepo, skuRepo: skuRepo}
}

// NewProductServiceWithProviders constructs a ProductService with multi-provider support.
func NewProductServiceWithProviders(productRepo *repository.ProductRepository, skuRepo *repository.SKURepository, providerRepo *repository.PPOBProviderRepository) *ProductService {
	return &ProductService{productRepo: productRepo, skuRepo: skuRepo, providerRepo: providerRepo}
}

// ProductResponse is the outward-facing payload for product listing.
type ProductResponse struct {
	SkuCode       string    `json:"skuCode"`
	Name          string    `json:"name"`
	Category      string    `json:"category"`
	Brand         string    `json:"brand"`
	Type          string    `json:"type"`
	Price         int       `json:"price,omitempty"`
	Admin         int       `json:"admin,omitempty"`
	Commission    int       `json:"commission,omitempty"`
	IsActive      bool      `json:"isActive"`
	Description   string    `json:"description"`
	ProviderCount int       `json:"providerCount,omitempty"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

// GetProducts returns products with filters and pagination.
// If multi-provider is enabled, returns best price from all providers.
// Otherwise falls back to the main SKU price (priority=1).
func (s *ProductService) GetProducts(productType, category, brand, search string, page, limit int) ([]ProductResponse, int, error) {
	// Use multi-provider pricing if available
	if s.providerRepo != nil {
		return s.getProductsWithBestPrice(productType, category, brand, search, page, limit)
	}

	// Fallback to legacy SKU-based pricing
	return s.getProductsLegacy(productType, category, brand, search, page, limit)
}

// getProductsWithBestPrice returns products with best price from multi-provider system
func (s *ProductService) getProductsWithBestPrice(productType, category, brand, search string, page, limit int) ([]ProductResponse, int, error) {
	products, total, err := s.providerRepo.GetProductsWithBestPrice(productType, category, brand, search, page, limit)
	if err != nil {
		return nil, 0, err
	}

	result := make([]ProductResponse, 0, len(products))
	for _, p := range products {
		price := 0
		admin := 0
		if p.BestPrice != nil {
			price = *p.BestPrice
		}
		if p.BestAdmin != nil {
			admin = *p.BestAdmin
		}

		result = append(result, ProductResponse{
			SkuCode:       p.SkuCode,
			Name:          p.Name,
			Category:      p.Category,
			Brand:         p.Brand,
			Type:          string(p.Type),
			Price:         price,
			Admin:         admin,
			IsActive:      p.IsActive,
			Description:   p.Description,
			ProviderCount: p.ProviderCount,
		})
	}
	return result, total, nil
}

// getProductsLegacy returns products with main SKU price (legacy method)
func (s *ProductService) getProductsLegacy(productType, category, brand, search string, page, limit int) ([]ProductResponse, int, error) {
	products, total, err := s.productRepo.GetAllPaged(productType, category, brand, search, page, limit)
	if err != nil {
		return nil, 0, err
	}

	result := make([]ProductResponse, 0, len(products))
	for _, p := range products {
		price, _ := s.skuRepo.GetMainSKUPrice(p.ID)
		result = append(result, ProductResponse{
			SkuCode:     p.SkuCode,
			Name:        p.Name,
			Category:    p.Category,
			Brand:       p.Brand,
			Type:        string(p.Type),
			Price:       price,
			Admin:       p.Admin,
			Commission:  p.Commission,
			IsActive:    p.IsActive,
			Description: p.Description,
			UpdatedAt:   p.UpdatedAt,
		})
	}
	return result, total, nil
}

// GetAvailableSKUs returns SKUs that are not in cutoff at the current WIB time.
func (s *ProductService) GetAvailableSKUs(productID int) ([]models.SKU, error) {
	wib := time.FixedZone("WIB", 7*3600) // UTC+7
	currentTime := time.Now().In(wib).Format("15:04:05")
	return s.skuRepo.GetAvailableSKUs(productID, currentTime)
}

// GetProductBySkuCode returns a product by sku code.
func (s *ProductService) GetProductBySkuCode(skuCode string) (*models.Product, error) {
	return s.productRepo.GetBySKUCode(skuCode)
}

// GetProductByID returns a product by ID.
func (s *ProductService) GetProductByID(productID int) (*models.Product, error) {
	return s.productRepo.GetByID(productID)
}
