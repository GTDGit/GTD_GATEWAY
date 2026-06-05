package repository

import (
	"database/sql"
	"errors"

	"github.com/jmoiron/sqlx"
)

// ProductCategory model
type ProductCategory struct {
	ID           int    `db:"id"`
	Name         string `db:"name"`
	DisplayOrder int    `db:"display_order"`
}

// ProductBrand model
type ProductBrand struct {
	ID           int    `db:"id"`
	Name         string `db:"name"`
	DisplayOrder int    `db:"display_order"`
}

// ProductVariant model (Reguler, Pulsa Transfer, etc - NOT prepaid/postpaid)
type ProductVariant struct {
	ID           int    `db:"id"`
	Name         string `db:"name"`
	DisplayOrder int    `db:"display_order"`
}

// ProductMasterRepository handles product master data (categories, brands, variants).
type ProductMasterRepository struct {
	db *sqlx.DB
}

// NewProductMasterRepository creates a new ProductMasterRepository.
func NewProductMasterRepository(db *sqlx.DB) *ProductMasterRepository {
	return &ProductMasterRepository{db: db}
}

// --- Categories ---

func (r *ProductMasterRepository) ListCategories() ([]ProductCategory, error) {
	const q = `SELECT id, name, display_order FROM product_categories ORDER BY display_order, name`
	var out []ProductCategory
	if err := r.db.Select(&out, q); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *ProductMasterRepository) GetCategoryByID(id int) (*ProductCategory, error) {
	const q = `SELECT id, name, display_order FROM product_categories WHERE id = $1`
	var c ProductCategory
	if err := r.db.Get(&c, q, id); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &c, nil
}

func (r *ProductMasterRepository) GetCategoryByName(name string) (*ProductCategory, error) {
	const q = `SELECT id, name, display_order FROM product_categories WHERE name = $1`
	var c ProductCategory
	if err := r.db.Get(&c, q, name); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &c, nil
}

func (r *ProductMasterRepository) CreateCategory(name string, displayOrder int) (*ProductCategory, error) {
	const q = `INSERT INTO product_categories (name, display_order) VALUES ($1, $2) RETURNING id, name, display_order`
	var c ProductCategory
	if err := r.db.Get(&c, q, name, displayOrder); err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *ProductMasterRepository) UpdateCategory(id int, name string, displayOrder int) error {
	const q = `UPDATE product_categories SET name = $2, display_order = $3, updated_at = NOW() WHERE id = $1`
	_, err := r.db.Exec(q, id, name, displayOrder)
	return err
}

func (r *ProductMasterRepository) CountProductsByCategoryName(name string) (int, error) {
	const q = `SELECT COUNT(*) FROM products WHERE category = $1`
	var n int
	if err := r.db.Get(&n, q, name); err != nil {
		return 0, err
	}
	return n, nil
}

func (r *ProductMasterRepository) DeleteCategory(id int) error {
	c, _ := r.GetCategoryByID(id)
	if c == nil {
		return nil
	}
	n, _ := r.CountProductsByCategoryName(c.Name)
	if n > 0 {
		return errors.New("cannot delete: category is used by products")
	}
	const q = `DELETE FROM product_categories WHERE id = $1`
	_, err := r.db.Exec(q, id)
	return err
}

// --- Brands ---

func (r *ProductMasterRepository) ListBrands() ([]ProductBrand, error) {
	const q = `SELECT id, name, display_order FROM product_brands ORDER BY display_order, name`
	var out []ProductBrand
	if err := r.db.Select(&out, q); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *ProductMasterRepository) GetBrandByID(id int) (*ProductBrand, error) {
	const q = `SELECT id, name, display_order FROM product_brands WHERE id = $1`
	var b ProductBrand
	if err := r.db.Get(&b, q, id); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &b, nil
}

func (r *ProductMasterRepository) GetBrandByName(name string) (*ProductBrand, error) {
	const q = `SELECT id, name, display_order FROM product_brands WHERE name = $1`
	var b ProductBrand
	if err := r.db.Get(&b, q, name); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &b, nil
}

func (r *ProductMasterRepository) CreateBrand(name string, displayOrder int) (*ProductBrand, error) {
	const q = `INSERT INTO product_brands (name, display_order) VALUES ($1, $2) RETURNING id, name, display_order`
	var b ProductBrand
	if err := r.db.Get(&b, q, name, displayOrder); err != nil {
		return nil, err
	}
	return &b, nil
}

func (r *ProductMasterRepository) UpdateBrand(id int, name string, displayOrder int) error {
	const q = `UPDATE product_brands SET name = $2, display_order = $3, updated_at = NOW() WHERE id = $1`
	_, err := r.db.Exec(q, id, name, displayOrder)
	return err
}

func (r *ProductMasterRepository) CountProductsByBrandName(name string) (int, error) {
	const q = `SELECT COUNT(*) FROM products WHERE brand = $1`
	var n int
	if err := r.db.Get(&n, q, name); err != nil {
		return 0, err
	}
	return n, nil
}

func (r *ProductMasterRepository) DeleteBrand(id int) error {
	b, _ := r.GetBrandByID(id)
	if b == nil {
		return nil
	}
	n, _ := r.CountProductsByBrandName(b.Name)
	if n > 0 {
		return errors.New("cannot delete: brand is used by products")
	}
	const q = `DELETE FROM product_brands WHERE id = $1`
	_, err := r.db.Exec(q, id)
	return err
}

// --- Variants ---

func (r *ProductMasterRepository) ListVariants() ([]ProductVariant, error) {
	const q = `SELECT id, name, display_order FROM product_variants ORDER BY display_order, name`
	var out []ProductVariant
	if err := r.db.Select(&out, q); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *ProductMasterRepository) GetVariantByID(id int) (*ProductVariant, error) {
	const q = `SELECT id, name, display_order FROM product_variants WHERE id = $1`
	var v ProductVariant
	if err := r.db.Get(&v, q, id); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &v, nil
}

func (r *ProductMasterRepository) GetVariantByName(name string) (*ProductVariant, error) {
	const q = `SELECT id, name, display_order FROM product_variants WHERE name = $1`
	var v ProductVariant
	if err := r.db.Get(&v, q, name); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &v, nil
}

func (r *ProductMasterRepository) CreateVariant(name string, displayOrder int) (*ProductVariant, error) {
	const q = `INSERT INTO product_variants (name, display_order) VALUES ($1, $2) RETURNING id, name, display_order`
	var v ProductVariant
	if err := r.db.Get(&v, q, name, displayOrder); err != nil {
		return nil, err
	}
	return &v, nil
}

func (r *ProductMasterRepository) UpdateVariant(id int, name string, displayOrder int) error {
	const q = `UPDATE product_variants SET name = $2, display_order = $3, updated_at = NOW() WHERE id = $1`
	_, err := r.db.Exec(q, id, name, displayOrder)
	return err
}

func (r *ProductMasterRepository) CountProductsByVariantID(variantID int) (int, error) {
	const q = `SELECT COUNT(*) FROM products WHERE variant_id = $1`
	var n int
	if err := r.db.Get(&n, q, variantID); err != nil {
		return 0, err
	}
	return n, nil
}

func (r *ProductMasterRepository) DeleteVariant(id int) error {
	v, _ := r.GetVariantByID(id)
	if v == nil {
		return nil
	}
	n, _ := r.CountProductsByVariantID(v.ID)
	if n > 0 {
		return errors.New("cannot delete: variant is used by products")
	}
	const q = `DELETE FROM product_variants WHERE id = $1`
	_, err := r.db.Exec(q, id)
	return err
}
