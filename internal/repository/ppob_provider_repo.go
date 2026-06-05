package repository

import (
	"database/sql"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/GTDGit/gtd_gateway/internal/models"
)

// PPOBProviderRepository handles data access for multi-provider PPOB system.
type PPOBProviderRepository struct {
	db *sqlx.DB
}

// NewPPOBProviderRepository creates a new PPOBProviderRepository.
func NewPPOBProviderRepository(db *sqlx.DB) *PPOBProviderRepository {
	return &PPOBProviderRepository{db: db}
}

// ============================================
// Provider CRUD
// ============================================

// GetAllProviders returns all providers.
func (r *PPOBProviderRepository) GetAllProviders(activeOnly bool) ([]models.PPOBProvider, error) {
	q := `SELECT * FROM ppob_providers`
	if activeOnly {
		q += ` WHERE is_active = true`
	}
	q += ` ORDER BY is_backup ASC, priority ASC`

	var providers []models.PPOBProvider
	if err := r.db.Select(&providers, q); err != nil {
		return nil, err
	}
	return providers, nil
}

// GetProviderByCode returns a provider by code.
func (r *PPOBProviderRepository) GetProviderByCode(code models.ProviderCode) (*models.PPOBProvider, error) {
	const q = `SELECT * FROM ppob_providers WHERE code = $1 LIMIT 1`
	var p models.PPOBProvider
	if err := r.db.Get(&p, q, code); err != nil {
		return nil, err
	}
	return &p, nil
}

// GetProviderByID returns a provider by ID.
func (r *PPOBProviderRepository) GetProviderByID(id int) (*models.PPOBProvider, error) {
	const q = `SELECT * FROM ppob_providers WHERE id = $1 LIMIT 1`
	var p models.PPOBProvider
	if err := r.db.Get(&p, q, id); err != nil {
		return nil, err
	}
	return &p, nil
}

// UpdateProviderStatus updates provider active status.
func (r *PPOBProviderRepository) UpdateProviderStatus(id int, isActive bool) error {
	const q = `UPDATE ppob_providers SET is_active = $2, updated_at = NOW() WHERE id = $1`
	_, err := r.db.Exec(q, id, isActive)
	return err
}

// ============================================
// Provider SKU CRUD
// ============================================

// CreateProviderSKU creates a new provider SKU mapping.
func (r *PPOBProviderRepository) CreateProviderSKU(sku *models.PPOBProviderSKU) error {
	const q = `
		INSERT INTO ppob_provider_skus
			(provider_id, product_id, provider_sku_code, provider_product_name, price, admin, commission, is_active, is_available)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, created_at, updated_at`

	return r.db.QueryRowx(q,
		sku.ProviderID,
		sku.ProductID,
		sku.ProviderSKUCode,
		sku.ProviderProductName,
		sku.Price,
		sku.Admin,
		sku.Commission,
		sku.IsActive,
		sku.IsAvailable,
	).Scan(&sku.ID, &sku.CreatedAt, &sku.UpdatedAt)
}

// UpdateProviderSKU updates a provider SKU mapping.
func (r *PPOBProviderRepository) UpdateProviderSKU(sku *models.PPOBProviderSKU) error {
	const q = `
		UPDATE ppob_provider_skus SET
			provider_sku_code = $2,
			provider_product_name = $3,
			price = $4,
			admin = $5,
			commission = $6,
			is_active = $7,
			is_available = $8,
			updated_at = NOW()
		WHERE id = $1`

	_, err := r.db.Exec(q,
		sku.ID,
		sku.ProviderSKUCode,
		sku.ProviderProductName,
		sku.Price,
		sku.Admin,
		sku.Commission,
		sku.IsActive,
		sku.IsAvailable,
	)
	return err
}

// UpdateProviderSKUPrice updates price, optionally admin, and sync timestamp.
func (r *PPOBProviderRepository) UpdateProviderSKUPrice(id int, price int, admin *int, isAvailable bool) error {
	const q = `
		UPDATE ppob_provider_skus SET 
			price = $2, 
			admin = COALESCE($3, admin), 
			is_available = $4, 
			last_sync_at = NOW(),
			sync_error = NULL,
			updated_at = NOW() 
		WHERE id = $1`
	_, err := r.db.Exec(q, id, price, admin, isAvailable)
	return err
}

// UpdateProviderSKUSyncError marks sync error for a SKU.
func (r *PPOBProviderRepository) UpdateProviderSKUSyncError(id int, errMsg string) error {
	const q = `
		UPDATE ppob_provider_skus SET 
			sync_error = $2, 
			last_sync_at = NOW(),
			updated_at = NOW() 
		WHERE id = $1`
	_, err := r.db.Exec(q, id, errMsg)
	return err
}

// DeleteProviderSKU deletes a provider SKU mapping.
func (r *PPOBProviderRepository) DeleteProviderSKU(id int) error {
	const q = `DELETE FROM ppob_provider_skus WHERE id = $1`
	_, err := r.db.Exec(q, id)
	return err
}

// GetProviderSKUByID returns a provider SKU by ID.
func (r *PPOBProviderRepository) GetProviderSKUByID(id int) (*models.PPOBProviderSKU, error) {
	const q = `
		SELECT 
			ps.*,
			pr.code AS provider_code,
			pr.name AS provider_name,
			pr.is_backup,
			p.name AS product_name,
			p.sku_code
		FROM ppob_provider_skus ps
		JOIN ppob_providers pr ON ps.provider_id = pr.id
		JOIN products p ON ps.product_id = p.id
		WHERE ps.id = $1`

	var sku models.PPOBProviderSKU
	if err := r.db.Get(&sku, q, id); err != nil {
		return nil, err
	}
	return &sku, nil
}

// GetProviderSKUsByProduct returns all provider SKUs for a product.
func (r *PPOBProviderRepository) GetProviderSKUsByProduct(productID int, activeOnly bool) ([]models.PPOBProviderSKU, error) {
	q := `
		SELECT
			ps.*,
			pr.code AS provider_code,
			pr.name AS provider_name,
			pr.is_backup,
			p.name AS product_name,
			p.sku_code,
			p.type::text AS product_type
		FROM ppob_provider_skus ps
		JOIN ppob_providers pr ON ps.provider_id = pr.id
		JOIN products p ON ps.product_id = p.id
		WHERE ps.product_id = $1`

	if activeOnly {
		q += ` AND ps.is_active = true AND ps.is_available = true AND pr.is_active = true`
	}
	q += ` ORDER BY pr.is_backup ASC, ps.price ASC, pr.priority ASC`

	var skus []models.PPOBProviderSKU
	if err := r.db.Select(&skus, q, productID); err != nil {
		return nil, err
	}
	return skus, nil
}

// GetProviderSKUsByProvider returns all SKUs for a provider.
func (r *PPOBProviderRepository) GetProviderSKUsByProvider(providerID int) ([]models.PPOBProviderSKU, error) {
	const q = `
		SELECT
			ps.*,
			pr.code AS provider_code,
			pr.name AS provider_name,
			pr.is_backup,
			p.name AS product_name,
			p.sku_code,
			p.type::text AS product_type
		FROM ppob_provider_skus ps
		JOIN ppob_providers pr ON ps.provider_id = pr.id
		JOIN products p ON ps.product_id = p.id
		WHERE ps.provider_id = $1
		ORDER BY p.category, p.brand, p.name`

	var skus []models.PPOBProviderSKU
	if err := r.db.Select(&skus, q, providerID); err != nil {
		return nil, err
	}
	return skus, nil
}

// GetProviderSKUByProviderAndProduct finds SKU mapping by provider and product.
// Returns nil, nil if no mapping exists.
func (r *PPOBProviderRepository) GetProviderSKUByProviderAndProduct(providerID, productID int) (*models.PPOBProviderSKU, error) {
	const q = `
		SELECT 
			ps.*,
			pr.code AS provider_code,
			pr.name AS provider_name,
			pr.is_backup
		FROM ppob_provider_skus ps
		JOIN ppob_providers pr ON ps.provider_id = pr.id
		WHERE ps.provider_id = $1 AND ps.product_id = $2`

	var sku models.PPOBProviderSKU
	if err := r.db.Get(&sku, q, providerID, productID); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &sku, nil
}

// GetAllProviderSKUsPaged returns all provider SKUs with pagination.
func (r *PPOBProviderRepository) GetAllProviderSKUsPaged(providerID int, search string, page, limit int) ([]models.PPOBProviderSKU, int, error) {
	if page <= 0 {
		page = 1
	}
	if limit <= 0 {
		limit = 50
	}
	offset := (page - 1) * limit

	baseWhere := `WHERE ($1 = 0 OR ps.provider_id = $1)
		AND ($2 = '' OR p.name ILIKE '%%' || $2 || '%%' OR p.sku_code ILIKE '%%' || $2 || '%%')`

	// Count
	countQ := `SELECT COUNT(1) FROM ppob_provider_skus ps
		JOIN products p ON ps.product_id = p.id ` + baseWhere
	var total int
	if err := r.db.Get(&total, countQ, providerID, search); err != nil {
		return nil, 0, err
	}

	// Fetch
	listQ := `
		SELECT
			ps.*,
			pr.code AS provider_code,
			pr.name AS provider_name,
			pr.is_backup,
			p.name AS product_name,
			p.sku_code,
			p.type::text AS product_type
		FROM ppob_provider_skus ps
		JOIN ppob_providers pr ON ps.provider_id = pr.id
		JOIN products p ON ps.product_id = p.id ` + baseWhere + `
		ORDER BY pr.name, p.category, p.brand, p.name
		LIMIT $3 OFFSET $4`

	var skus []models.PPOBProviderSKU
	if err := r.db.Select(&skus, listQ, providerID, search, limit, offset); err != nil {
		return nil, 0, err
	}
	return skus, total, nil
}

// ============================================
// Provider Selection for Transaction
// ============================================

// GetProvidersForProduct returns providers sorted by price for PREPAID transaction execution.
// Non-backup providers first (sorted by price ASC), then backup providers.
func (r *PPOBProviderRepository) GetProvidersForProduct(productID int) ([]models.ProviderOption, error) {
	const q = `
		SELECT 
			pr.id AS provider_id,
			pr.code AS provider_code,
			ps.id AS provider_sku_id,
			ps.provider_sku_code,
			ps.price,
			ps.admin,
			ps.commission,
			pr.is_backup
		FROM ppob_provider_skus ps
		JOIN ppob_providers pr ON ps.provider_id = pr.id
		WHERE ps.product_id = $1
		AND ps.is_active = true
		AND ps.is_available = true
		AND pr.is_active = true
		AND ps.price > 0
		ORDER BY pr.is_backup ASC, ps.price ASC, pr.priority ASC`

	var options []models.ProviderOption
	if err := r.db.Select(&options, q, productID); err != nil {
		return nil, err
	}
	return options, nil
}

// GetProvidersForProductPostpaid returns providers sorted by effective admin (admin - commission) for POSTPAID.
// Lower effective admin = better for postpaid because we earn more commission.
// Example: A admin=5000, comm=3500 → effective=1500 | B admin=3000, comm=1000 → effective=2000 | A wins
func (r *PPOBProviderRepository) GetProvidersForProductPostpaid(productID int) ([]models.ProviderOption, error) {
	const q = `
		SELECT 
			pr.id AS provider_id,
			pr.code AS provider_code,
			ps.id AS provider_sku_id,
			ps.provider_sku_code,
			ps.price,
			ps.admin,
			ps.commission,
			pr.is_backup
		FROM ppob_provider_skus ps
		JOIN ppob_providers pr ON ps.provider_id = pr.id
		WHERE ps.product_id = $1
		AND ps.is_active = true
		AND ps.is_available = true
		AND pr.is_active = true
		ORDER BY pr.is_backup ASC, (ps.admin - ps.commission) ASC, pr.priority ASC`

	var options []models.ProviderOption
	if err := r.db.Select(&options, q, productID); err != nil {
		return nil, err
	}
	return options, nil
}

// GetProvidersForProductAll returns providers including unavailable ones.
// Used when a specific provider is explicitly requested and availability should be ignored.
func (r *PPOBProviderRepository) GetProvidersForProductAll(productID int) ([]models.ProviderOption, error) {
	const q = `
		SELECT
			pr.id AS provider_id,
			pr.code AS provider_code,
			ps.id AS provider_sku_id,
			ps.provider_sku_code,
			ps.price,
			ps.admin,
			ps.commission,
			pr.is_backup
		FROM ppob_provider_skus ps
		JOIN ppob_providers pr ON ps.provider_id = pr.id
		WHERE ps.product_id = $1
		AND ps.is_active = true
		AND pr.is_active = true
		ORDER BY pr.is_backup ASC, (ps.admin - ps.commission) ASC, pr.priority ASC`

	var options []models.ProviderOption
	if err := r.db.Select(&options, q, productID); err != nil {
		return nil, err
	}
	return options, nil
}

// GetBestPriceForProduct returns the best (lowest) price from non-backup providers.
func (r *PPOBProviderRepository) GetBestPriceForProduct(productID int) (*int, *int, error) {
	const q = `
		SELECT ps.price, ps.admin
		FROM ppob_provider_skus ps
		JOIN ppob_providers pr ON ps.provider_id = pr.id
		WHERE ps.product_id = $1
		AND ps.is_active = true
		AND ps.is_available = true
		AND pr.is_active = true
		AND pr.is_backup = false
		AND ps.price > 0
		ORDER BY ps.price ASC
		LIMIT 1`

	var price, admin int
	if err := r.db.QueryRowx(q, productID).Scan(&price, &admin); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	return &price, &admin, nil
}

// ============================================
// Products with Best Price
// ============================================

// GetProductsWithBestPrice returns products with their best price from non-backup providers.
func (r *PPOBProviderRepository) GetProductsWithBestPrice(productType, category, brand, search string, page, limit int) ([]models.ProductWithBestPrice, int, error) {
	if page <= 0 {
		page = 1
	}
	if limit <= 0 {
		limit = 50
	}
	offset := (page - 1) * limit

	baseWhere := `WHERE p.is_active = true
		AND ($1 = '' OR p.type::text = $1)
		AND ($2 = '' OR p.category = $2)
		AND ($3 = '' OR p.brand = $3)
		AND ($4 = '' OR p.name ILIKE '%%' || $4 || '%%')`

	// Count
	countQ := `SELECT COUNT(1) FROM products p ` + baseWhere
	var total int
	if err := r.db.Get(&total, countQ, productType, category, brand, search); err != nil {
		return nil, 0, err
	}

	// Fetch with best price
	listQ := `
		SELECT
			p.id,
			p.sku_code,
			p.name,
			p.category,
			p.brand,
			p.type,
			p.admin,
			p.is_active,
			p.description,
			(
				SELECT MIN(ps.price)
				FROM ppob_provider_skus ps
				JOIN ppob_providers pr ON ps.provider_id = pr.id
				WHERE ps.product_id = p.id
				AND ps.is_active = true
				AND ps.is_available = true
				AND pr.is_active = true
				AND pr.is_backup = false
				AND ps.price > 0
			) AS best_price,
			(
				SELECT ps.admin
				FROM ppob_provider_skus ps
				JOIN ppob_providers pr ON ps.provider_id = pr.id
				WHERE ps.product_id = p.id
				AND ps.is_active = true
				AND ps.is_available = true
				AND pr.is_active = true
				AND pr.is_backup = false
				AND ps.price > 0
				ORDER BY ps.price ASC
				LIMIT 1
			) AS best_admin,
			(
				SELECT COUNT(DISTINCT ps.provider_id)
				FROM ppob_provider_skus ps
				JOIN ppob_providers pr ON ps.provider_id = pr.id
				WHERE ps.product_id = p.id
				AND ps.is_active = true
				AND ps.is_available = true
				AND pr.is_active = true
				AND pr.is_backup = false
			) AS provider_count
		FROM products p ` + baseWhere + `
		ORDER BY p.category, p.brand, p.name
		LIMIT $5 OFFSET $6`

	var products []models.ProductWithBestPrice
	if err := r.db.Select(&products, listQ, productType, category, brand, search, limit, offset); err != nil {
		return nil, 0, err
	}
	return products, total, nil
}

// ============================================
// Provider Health
// ============================================

// RecordProviderRequest records a request to a provider for health tracking.
func (r *PPOBProviderRepository) RecordProviderRequest(providerID int, success bool, responseTimeMs int, failureReason string) error {
	const q = `
		INSERT INTO ppob_provider_health 
			(provider_id, total_requests, success_count, failed_count, last_success_at, last_failure_at, last_failure_reason, avg_response_time_ms, date)
		VALUES ($1, 1, $2, $3, $4, $5, $6, $7, CURRENT_DATE)
		ON CONFLICT (provider_id, date) DO UPDATE SET
			total_requests = ppob_provider_health.total_requests + 1,
			success_count = ppob_provider_health.success_count + $2,
			failed_count = ppob_provider_health.failed_count + $3,
			last_success_at = CASE WHEN $2 = 1 THEN NOW() ELSE ppob_provider_health.last_success_at END,
			last_failure_at = CASE WHEN $3 = 1 THEN NOW() ELSE ppob_provider_health.last_failure_at END,
			last_failure_reason = CASE WHEN $3 = 1 THEN $6 ELSE ppob_provider_health.last_failure_reason END,
			avg_response_time_ms = (ppob_provider_health.avg_response_time_ms * ppob_provider_health.total_requests + $7) / (ppob_provider_health.total_requests + 1),
			health_score = (ppob_provider_health.success_count + $2)::DECIMAL / (ppob_provider_health.total_requests + 1) * 100,
			updated_at = NOW()`

	var successCount, failedCount int
	var lastSuccessAt, lastFailureAt *time.Time
	now := time.Now()

	if success {
		successCount = 1
		lastSuccessAt = &now
	} else {
		failedCount = 1
		lastFailureAt = &now
	}

	_, err := r.db.Exec(q, providerID, successCount, failedCount, lastSuccessAt, lastFailureAt, failureReason, responseTimeMs)
	return err
}

// GetProviderHealth returns health stats for a provider (today).
func (r *PPOBProviderRepository) GetProviderHealth(providerID int) (*models.PPOBProviderHealth, error) {
	const q = `
		SELECT h.*, pr.code AS provider_code, pr.name AS provider_name
		FROM ppob_provider_health h
		JOIN ppob_providers pr ON h.provider_id = pr.id
		WHERE h.provider_id = $1 AND h.date = CURRENT_DATE`

	var health models.PPOBProviderHealth
	if err := r.db.Get(&health, q, providerID); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &health, nil
}

// GetAllProviderHealthToday returns health stats for all providers today.
func (r *PPOBProviderRepository) GetAllProviderHealthToday() ([]models.PPOBProviderHealth, error) {
	const q = `
		SELECT h.*, pr.code AS provider_code, pr.name AS provider_name
		FROM ppob_provider_health h
		JOIN ppob_providers pr ON h.provider_id = pr.id
		WHERE h.date = CURRENT_DATE
		ORDER BY pr.is_backup ASC, h.health_score DESC`

	var health []models.PPOBProviderHealth
	if err := r.db.Select(&health, q); err != nil {
		return nil, err
	}
	return health, nil
}

// ============================================
// Provider Callbacks
// ============================================

// CreateProviderCallback saves a provider callback.
func (r *PPOBProviderRepository) CreateProviderCallback(cb *models.PPOBProviderCallback) error {
	const q = `
		INSERT INTO ppob_provider_callbacks 
			(provider_id, provider_ref_id, transaction_id, payload, status, message)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at`

	return r.db.QueryRowx(q,
		cb.ProviderID,
		cb.ProviderRefID,
		cb.TransactionID,
		cb.Payload,
		cb.Status,
		cb.Message,
	).Scan(&cb.ID, &cb.CreatedAt)
}

// UpdateProviderCallbackProcessed marks a callback as processed.
func (r *PPOBProviderRepository) UpdateProviderCallbackProcessed(id int, processed bool) error {
	const q = `UPDATE ppob_provider_callbacks SET is_processed = $2, processed_at = NOW() WHERE id = $1`
	_, err := r.db.Exec(q, id, processed)
	return err
}

// GetUnprocessedCallbacks returns unprocessed callbacks.
func (r *PPOBProviderRepository) GetUnprocessedCallbacks(limit int) ([]models.PPOBProviderCallback, error) {
	const q = `
		SELECT * FROM ppob_provider_callbacks 
		WHERE is_processed = false 
		ORDER BY created_at ASC 
		LIMIT $1`

	var callbacks []models.PPOBProviderCallback
	if err := r.db.Select(&callbacks, q, limit); err != nil {
		return nil, err
	}
	return callbacks, nil
}

// MarkCallbackProcessed marks a callback as processed.
func (r *PPOBProviderRepository) MarkCallbackProcessed(id int, processError string) error {
	const q = `
		UPDATE ppob_provider_callbacks SET 
			is_processed = true, 
			processed_at = NOW(), 
			process_error = $2 
		WHERE id = $1`
	_, err := r.db.Exec(q, id, processError)
	return err
}
