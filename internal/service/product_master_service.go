package service

import (
	"context"
	"errors"
	"strings"

	"github.com/GTDGit/gtd_gateway/internal/repository"
)

// ProductMasterService handles CRUD for product categories, brands, and variants.
type ProductMasterService struct {
	repo *repository.ProductMasterRepository
}

// NewProductMasterService creates a new ProductMasterService.
func NewProductMasterService(repo *repository.ProductMasterRepository) *ProductMasterService {
	return &ProductMasterService{repo: repo}
}

// --- Categories ---

func (s *ProductMasterService) ListCategories() ([]repository.ProductCategory, error) {
	return s.repo.ListCategories()
}

func (s *ProductMasterService) CreateCategory(ctx context.Context, name string, displayOrder int) (*repository.ProductCategory, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("category name is required")
	}
	existing, _ := s.repo.GetCategoryByName(name)
	if existing != nil {
		return nil, errors.New("category already exists")
	}
	return s.repo.CreateCategory(name, displayOrder)
}

func (s *ProductMasterService) UpdateCategory(ctx context.Context, id int, name string, displayOrder int) error {
	c, _ := s.repo.GetCategoryByID(id)
	if c == nil {
		return errors.New("category not found")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("category name is required")
	}
	existing, _ := s.repo.GetCategoryByName(name)
	if existing != nil && existing.ID != id {
		return errors.New("category name already used by another")
	}
	return s.repo.UpdateCategory(id, name, displayOrder)
}

func (s *ProductMasterService) DeleteCategory(ctx context.Context, id int) error {
	c, _ := s.repo.GetCategoryByID(id)
	if c == nil {
		return errors.New("category not found")
	}
	return s.repo.DeleteCategory(id)
}

// --- Brands ---

func (s *ProductMasterService) ListBrands() ([]repository.ProductBrand, error) {
	return s.repo.ListBrands()
}

func (s *ProductMasterService) CreateBrand(ctx context.Context, name string, displayOrder int) (*repository.ProductBrand, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("brand name is required")
	}
	existing, _ := s.repo.GetBrandByName(name)
	if existing != nil {
		return nil, errors.New("brand already exists")
	}
	return s.repo.CreateBrand(name, displayOrder)
}

func (s *ProductMasterService) UpdateBrand(ctx context.Context, id int, name string, displayOrder int) error {
	b, _ := s.repo.GetBrandByID(id)
	if b == nil {
		return errors.New("brand not found")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("brand name is required")
	}
	existing, _ := s.repo.GetBrandByName(name)
	if existing != nil && existing.ID != id {
		return errors.New("brand name already used by another")
	}
	return s.repo.UpdateBrand(id, name, displayOrder)
}

func (s *ProductMasterService) DeleteBrand(ctx context.Context, id int) error {
	b, _ := s.repo.GetBrandByID(id)
	if b == nil {
		return errors.New("brand not found")
	}
	return s.repo.DeleteBrand(id)
}

// --- Variants ---

func (s *ProductMasterService) ListVariants() ([]repository.ProductVariant, error) {
	return s.repo.ListVariants()
}

func (s *ProductMasterService) CreateVariant(ctx context.Context, name string, displayOrder int) (*repository.ProductVariant, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("variant name is required")
	}
	existing, _ := s.repo.GetVariantByName(name)
	if existing != nil {
		return nil, errors.New("variant already exists")
	}
	return s.repo.CreateVariant(name, displayOrder)
}

func (s *ProductMasterService) UpdateVariant(ctx context.Context, id int, name string, displayOrder int) error {
	v, _ := s.repo.GetVariantByID(id)
	if v == nil {
		return errors.New("variant not found")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("variant name is required")
	}
	existing, _ := s.repo.GetVariantByName(name)
	if existing != nil && existing.ID != id {
		return errors.New("variant name already used by another")
	}
	return s.repo.UpdateVariant(id, name, displayOrder)
}

func (s *ProductMasterService) DeleteVariant(ctx context.Context, id int) error {
	v, _ := s.repo.GetVariantByID(id)
	if v == nil {
		return errors.New("variant not found")
	}
	return s.repo.DeleteVariant(id)
}

// ValidateCategoryBrandVariant checks category and brand exist; variantID if provided must exist.
func (s *ProductMasterService) ValidateCategoryBrandVariant(category, brand string, variantID *int) error {
	if category == "" {
		return errors.New("category is required")
	}
	if brand == "" {
		return errors.New("brand is required")
	}
	cat, _ := s.repo.GetCategoryByName(category)
	if cat == nil {
		return errors.New("invalid category: not found in master list")
	}
	b, _ := s.repo.GetBrandByName(brand)
	if b == nil {
		return errors.New("invalid brand: not found in master list")
	}
	if variantID != nil && *variantID > 0 {
		v, _ := s.repo.GetVariantByID(*variantID)
		if v == nil {
			return errors.New("invalid variant: not found in master list")
		}
	}
	return nil
}
