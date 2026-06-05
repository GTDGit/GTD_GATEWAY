package repository

import (
	"database/sql"

	"github.com/jmoiron/sqlx"

	"github.com/GTDGit/gtd_gateway/internal/models"
)

// SKURepository handles data access for skus.
type SKURepository struct {
	db *sqlx.DB
}

// NewSKURepository creates a new SKURepository.
func NewSKURepository(db *sqlx.DB) *SKURepository {
	return &SKURepository{db: db}
}

// GetByProductID returns SKUs for a product (any status).
func (r *SKURepository) GetByProductID(productID int) ([]models.SKU, error) {
	const q = `SELECT * FROM skus WHERE product_id = $1 ORDER BY priority ASC`
	stmt, err := r.db.Preparex(q)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	var skus []models.SKU
	if err := stmt.Select(&skus, productID); err != nil {
		return nil, err
	}
	return skus, nil
}

// GetAvailableSKUs returns active SKUs considering cutoff windows against currentTime (HH:MM:SS).
func (r *SKURepository) GetAvailableSKUs(productID int, currentTime string) ([]models.SKU, error) {
	// Cutoff logic per spec: allow SKUs where either no cutoff or now is NOT within cutoff.
	const q = `
        SELECT * FROM skus
        WHERE product_id = $1
          AND is_active = true
          AND (
              (cut_off_start = '00:00:00' AND cut_off_end = '00:00:00')
              OR
              (cut_off_start < cut_off_end 
               AND NOT ($2::time >= cut_off_start AND $2::time <= cut_off_end))
              OR
              (cut_off_start > cut_off_end 
               AND NOT ($2::time >= cut_off_start OR $2::time <= cut_off_end))
          )
        ORDER BY priority ASC`

	stmt, err := r.db.Preparex(q)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	var skus []models.SKU
	if err := stmt.Select(&skus, productID, currentTime); err != nil {
		return nil, err
	}
	return skus, nil
}

// GetByDigiSKUCode returns a single SKU by digi_sku_code.
func (r *SKURepository) GetByDigiSKUCode(digiSkuCode string) (*models.SKU, error) {
	const q = `SELECT * FROM skus WHERE digi_sku_code = $1 LIMIT 1`
	stmt, err := r.db.Preparex(q)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()
	var sku models.SKU
	if err := stmt.Get(&sku, digiSkuCode); err != nil {
		if err == sql.ErrNoRows {
			return nil, sql.ErrNoRows
		}
		return nil, err
	}
	return &sku, nil
}

// GetByID returns a single SKU by id.
func (r *SKURepository) GetByID(id int) (*models.SKU, error) {
	const q = `SELECT * FROM skus WHERE id = $1 LIMIT 1`
	stmt, err := r.db.Preparex(q)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()
	var sku models.SKU
	if err := stmt.Get(&sku, id); err != nil {
		if err == sql.ErrNoRows {
			return nil, sql.ErrNoRows
		}
		return nil, err
	}
	return &sku, nil
}

// Upsert inserts or updates a SKU by digi_sku_code unique constraint.
func (r *SKURepository) Upsert(sku *models.SKU) error {
	const q = `
        INSERT INTO skus (product_id, digi_sku_code, seller_name, priority, price, is_active, support_multi, unlimited_stock, stock, cut_off_start, cut_off_end)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
        ON CONFLICT (digi_sku_code) DO UPDATE SET
            product_id = EXCLUDED.product_id,
            seller_name = EXCLUDED.seller_name,
            priority = EXCLUDED.priority,
            price = EXCLUDED.price,
            is_active = EXCLUDED.is_active,
            support_multi = EXCLUDED.support_multi,
            unlimited_stock = EXCLUDED.unlimited_stock,
            stock = EXCLUDED.stock,
            cut_off_start = EXCLUDED.cut_off_start,
            cut_off_end = EXCLUDED.cut_off_end,
            updated_at = NOW()`

	stmt, err := r.db.Preparex(q)
	if err != nil {
		return err
	}
	defer stmt.Close()
	_, err = stmt.Exec(
		sku.ProductID,
		sku.DigiSkuCode,
		sku.SellerName,
		sku.Priority,
		sku.Price,
		sku.IsActive,
		sku.SupportMulti,
		sku.UnlimitedStock,
		sku.Stock,
		sku.CutOffStart,
		sku.CutOffEnd,
	)
	return err
}

// UpdateStatus updates the active flag of a SKU.
func (r *SKURepository) UpdateStatus(id int, isActive bool) error {
	const q = `UPDATE skus SET is_active = $2, updated_at = NOW() WHERE id = $1`
	stmt, err := r.db.Preparex(q)
	if err != nil {
		return err
	}
	defer stmt.Close()
	_, err = stmt.Exec(id, isActive)
	return err
}

// GetMainSKUPrice returns price of priority=1 active SKU for a product.
func (r *SKURepository) GetMainSKUPrice(productID int) (int, error) {
	const q = `SELECT price FROM skus WHERE product_id = $1 AND priority = 1 AND is_active = true LIMIT 1`
	stmt, err := r.db.Preparex(q)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()
	var price int
	if err := stmt.Get(&price, productID); err != nil {
		if err == sql.ErrNoRows {
			return 0, sql.ErrNoRows
		}
		return 0, err
	}
	return price, nil
}

// Create creates a new SKU.
func (r *SKURepository) Create(sku *models.SKU) error {
	query := `INSERT INTO skus (product_id, digi_sku_code, seller_name, priority, price, is_active, support_multi, unlimited_stock, stock, cut_off_start, cut_off_end)
              VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
              RETURNING id, created_at, updated_at`

	return r.db.QueryRowx(query,
		sku.ProductID,
		sku.DigiSkuCode,
		sku.SellerName,
		sku.Priority,
		sku.Price,
		sku.IsActive,
		sku.SupportMulti,
		sku.UnlimitedStock,
		sku.Stock,
		sku.CutOffStart,
		sku.CutOffEnd,
	).Scan(&sku.ID, &sku.CreatedAt, &sku.UpdatedAt)
}

// Update updates an existing SKU.
func (r *SKURepository) Update(sku *models.SKU) error {
	query := `UPDATE skus
              SET digi_sku_code = $1, seller_name = $2, priority = $3, price = $4,
                  is_active = $5, support_multi = $6, unlimited_stock = $7, stock = $8,
                  cut_off_start = $9, cut_off_end = $10
              WHERE id = $11
              RETURNING updated_at`

	return r.db.QueryRowx(query,
		sku.DigiSkuCode,
		sku.SellerName,
		sku.Priority,
		sku.Price,
		sku.IsActive,
		sku.SupportMulti,
		sku.UnlimitedStock,
		sku.Stock,
		sku.CutOffStart,
		sku.CutOffEnd,
		sku.ID,
	).Scan(&sku.UpdatedAt)
}

// Delete deletes a SKU by ID.
func (r *SKURepository) Delete(id int) error {
	query := `DELETE FROM skus WHERE id = $1`
	_, err := r.db.Exec(query, id)
	return err
}
