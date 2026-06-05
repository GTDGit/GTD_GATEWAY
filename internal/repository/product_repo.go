package repository

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"

	"github.com/GTDGit/gtd_gateway/internal/models"
)

// ProductRepository handles data access for products.
type ProductRepository struct {
	db *sqlx.DB
}

// NewProductRepository creates a new ProductRepository.
func NewProductRepository(db *sqlx.DB) *ProductRepository {
	return &ProductRepository{db: db}
}

// GetAll returns all active products with optional filters for type and category.
// When productType or category is an empty string, the filter is ignored respectively.
func (r *ProductRepository) GetAll(productType, category string) ([]models.Product, error) {
	const q = `
        SELECT * FROM products 
        WHERE ($1 = '' OR type::text = $1) 
        AND ($2 = '' OR category = $2)
        AND is_active = true
        ORDER BY category, brand, name`

	stmt, err := r.db.Preparex(q)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	var products []models.Product
	if err := stmt.Select(&products, productType, category); err != nil {
		return nil, err
	}
	return products, nil
}

// GetAllPaged returns active products with filters and pagination and also returns total count.
// Filters: productType (prepaid/postpaid), category, brand (exact), search (ILIKE on name).
// If a filter is empty it will be ignored. Page begins at 1.
func (r *ProductRepository) GetAllPaged(productType, category, brand, search string, page, limit int) ([]models.Product, int, error) {
	if page <= 0 {
		page = 1
	}
	if limit <= 0 {
		limit = 50
	}
	offset := (page - 1) * limit

	// Base WHERE clause
	const baseWhere = `WHERE ($1 = '' OR type::text = $1)
        AND ($2 = '' OR category = $2)
        AND ($3 = '' OR brand = $3)
        AND ($4 = '' OR name ILIKE '%%' || $4 || '%%')
        AND is_active = true`

	// Count total
	countQuery := `SELECT COUNT(1) FROM products ` + baseWhere
	var total int
	if err := r.db.Get(&total, countQuery, productType, category, brand, search); err != nil {
		return nil, 0, err
	}

	// Fetch page
	listQuery := `SELECT * FROM products ` + baseWhere + `
        ORDER BY category, brand, name LIMIT $5 OFFSET $6`
	var products []models.Product
	if err := r.db.Select(&products, listQuery, productType, category, brand, search, limit, offset); err != nil {
		return nil, 0, err
	}
	return products, total, nil
}

// GetBySKUCode returns a single product by sku_code.
func (r *ProductRepository) GetBySKUCode(skuCode string) (*models.Product, error) {
	const q = `SELECT * FROM products WHERE sku_code = $1 LIMIT 1`
	stmt, err := r.db.Preparex(q)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	var p models.Product
	if err := stmt.Get(&p, skuCode); err != nil {
		if err == sql.ErrNoRows {
			return nil, sql.ErrNoRows
		}
		return nil, err
	}
	return &p, nil
}

// GetByID returns a single product by id.
func (r *ProductRepository) GetByID(id int) (*models.Product, error) {
	const q = `SELECT * FROM products WHERE id = $1 LIMIT 1`
	stmt, err := r.db.Preparex(q)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	var p models.Product
	if err := stmt.Get(&p, id); err != nil {
		if err == sql.ErrNoRows {
			return nil, sql.ErrNoRows
		}
		return nil, err
	}
	return &p, nil
}

// Upsert inserts or updates a product by sku_code.
func (r *ProductRepository) Upsert(product *models.Product) error {
	const q = `
        INSERT INTO products (sku_code, name, category, brand, type, variant_id, admin, commission, description, is_active)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
        ON CONFLICT (sku_code) DO UPDATE SET
            name = EXCLUDED.name,
            category = EXCLUDED.category,
            brand = EXCLUDED.brand,
            type = EXCLUDED.type,
            variant_id = EXCLUDED.variant_id,
            admin = EXCLUDED.admin,
            commission = EXCLUDED.commission,
            description = EXCLUDED.description,
            is_active = EXCLUDED.is_active,
            updated_at = NOW()`

	stmt, err := r.db.Preparex(q)
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.Exec(
		product.SkuCode,
		product.Name,
		product.Category,
		product.Brand,
		product.Type,
		product.VariantID,
		product.Admin,
		product.Commission,
		product.Description,
		product.IsActive,
	)
	return err
}

// UpdateStatus sets the active flag of a product.
func (r *ProductRepository) UpdateStatus(id int, isActive bool) error {
	const q = `UPDATE products SET is_active = $2, updated_at = NOW() WHERE id = $1`
	stmt, err := r.db.Preparex(q)
	if err != nil {
		return err
	}
	defer stmt.Close()
	_, err = stmt.Exec(id, isActive)
	return err
}

// Create creates a new product.
func (r *ProductRepository) Create(product *models.Product) error {
	query := `INSERT INTO products (sku_code, name, category, brand, type, variant_id, admin, commission, description, is_active)
              VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
              RETURNING id, created_at, updated_at`

	return r.db.QueryRowx(query,
		product.SkuCode,
		product.Name,
		product.Category,
		product.Brand,
		product.Type,
		product.VariantID,
		product.Admin,
		product.Commission,
		product.Description,
		product.IsActive,
	).Scan(&product.ID, &product.CreatedAt, &product.UpdatedAt)
}

// Update updates an existing product.
func (r *ProductRepository) Update(product *models.Product) error {
	query := `UPDATE products
              SET sku_code = $1, name = $2, category = $3, brand = $4,
                  type = $5, variant_id = $6, admin = $7, commission = $8, description = $9, is_active = $10
              WHERE id = $11
              RETURNING updated_at`

	return r.db.QueryRowx(query,
		product.SkuCode,
		product.Name,
		product.Category,
		product.Brand,
		product.Type,
		product.VariantID,
		product.Admin,
		product.Commission,
		product.Description,
		product.IsActive,
		product.ID,
	).Scan(&product.UpdatedAt)
}

// Delete deletes a product by ID.
func (r *ProductRepository) Delete(id int) error {
	query := `DELETE FROM products WHERE id = $1`
	_, err := r.db.Exec(query, id)
	return err
}

// AdminProductFilter holds filters for admin product queries.
type AdminProductFilter struct {
	Type      string // prepaid or postpaid
	VariantID *int   // optional variant filter
	Category  string
	Brand     string
	Search    string
	IsActive  *bool
	Page      int
	Limit     int
}

// AdminProductResult contains paginated product results for admin.
type AdminProductResult struct {
	Products   []models.Product
	TotalItems int
	TotalPages int
	Page       int
	Limit      int
}

// GetAllAdmin returns all products for admin with filters and pagination (includes inactive).
func (r *ProductRepository) GetAllAdmin(filter *AdminProductFilter) (*AdminProductResult, error) {
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.Limit <= 0 {
		filter.Limit = 50
	}
	if filter.Limit > 100 {
		filter.Limit = 100
	}
	offset := (filter.Page - 1) * filter.Limit

	// Build dynamic WHERE clause
	baseWhere := `WHERE 1=1`
	args := []interface{}{}
	argIdx := 1

	if filter.Type != "" {
		baseWhere += fmt.Sprintf(" AND p.type::text = $%d", argIdx)
		args = append(args, filter.Type)
		argIdx++
	}
	if filter.VariantID != nil {
		baseWhere += fmt.Sprintf(" AND p.variant_id = $%d", argIdx)
		args = append(args, *filter.VariantID)
		argIdx++
	}
	if filter.Category != "" {
		baseWhere += fmt.Sprintf(" AND p.category ILIKE $%d", argIdx)
		args = append(args, "%"+filter.Category+"%")
		argIdx++
	}
	if filter.Brand != "" {
		baseWhere += fmt.Sprintf(" AND p.brand ILIKE $%d", argIdx)
		args = append(args, "%"+filter.Brand+"%")
		argIdx++
	}
	if filter.Search != "" {
		baseWhere += fmt.Sprintf(" AND (p.name ILIKE $%d OR p.sku_code ILIKE $%d)", argIdx, argIdx)
		args = append(args, "%"+filter.Search+"%")
		argIdx++
	}
	if filter.IsActive != nil {
		baseWhere += fmt.Sprintf(" AND p.is_active = $%d", argIdx)
		args = append(args, *filter.IsActive)
		argIdx++
	}

	// Count total
	countQuery := `SELECT COUNT(1) FROM products p ` + baseWhere
	var total int
	if err := r.db.Get(&total, countQuery, args...); err != nil {
		return nil, err
	}

	totalPages := (total + filter.Limit - 1) / filter.Limit

	// Fetch page with provider count, min price
	// Note: variant_name omitted to avoid requiring product_variants table (migration 020)
	listQuery := fmt.Sprintf(`
		SELECT
			p.*,
			COALESCE(ps.provider_count, 0) AS provider_count,
			ps.min_price,
			ps.min_admin
		FROM products p
		LEFT JOIN (
			SELECT
				psk.product_id,
				COUNT(DISTINCT psk.provider_id) AS provider_count,
				MIN(CASE WHEN psk.price > 0 THEN psk.price ELSE NULL END) AS min_price,
				MIN(CASE WHEN psk.admin > 0 THEN (psk.admin - psk.commission) ELSE NULL END) AS min_admin
			FROM ppob_provider_skus psk
			JOIN ppob_providers ppr ON psk.provider_id = ppr.id
			WHERE psk.is_active = true AND psk.is_available = true AND ppr.is_active = true
			GROUP BY psk.product_id
		) ps ON ps.product_id = p.id
		WHERE 1=1 %s
		ORDER BY p.category, p.brand, COALESCE(ps.min_price, 999999999), p.name
		LIMIT $%d OFFSET $%d`,
		strings.Replace(baseWhere, "WHERE 1=1", "", 1), argIdx, argIdx+1)
	args = append(args, filter.Limit, offset)

	var products []models.Product
	if err := r.db.Select(&products, listQuery, args...); err != nil {
		return nil, err
	}

	return &AdminProductResult{
		Products:   products,
		TotalItems: total,
		TotalPages: totalPages,
		Page:       filter.Page,
		Limit:      filter.Limit,
	}, nil
}

// GetDistinctCategories returns all distinct categories, optionally filtered by type.
func (r *ProductRepository) GetDistinctCategories(productType string) ([]string, error) {
	q := `SELECT DISTINCT category FROM products WHERE category != ''`
	args := []interface{}{}
	argIdx := 1
	if productType != "" {
		q += ` AND type = $` + fmt.Sprintf("%d", argIdx)
		args = append(args, productType)
		argIdx++
	}
	q += ` ORDER BY category`

	var categories []string
	if err := r.db.Select(&categories, q, args...); err != nil {
		return nil, err
	}
	return categories, nil
}

// GetDistinctBrands returns all distinct brands, optionally filtered by category and type.
func (r *ProductRepository) GetDistinctBrands(category, productType string) ([]string, error) {
	q := `SELECT DISTINCT brand FROM products WHERE brand != ''`
	args := []interface{}{}
	argIdx := 1
	if category != "" {
		q += ` AND category = $` + fmt.Sprintf("%d", argIdx)
		args = append(args, category)
		argIdx++
	}
	if productType != "" {
		q += ` AND type = $` + fmt.Sprintf("%d", argIdx)
		args = append(args, productType)
		argIdx++
	}
	q += ` ORDER BY brand`

	var brands []string
	if err := r.db.Select(&brands, q, args...); err != nil {
		return nil, err
	}
	return brands, nil
}
