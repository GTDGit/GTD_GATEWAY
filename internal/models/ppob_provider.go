package models

import (
	"encoding/json"
	"time"
)

// ProviderCode identifies the PPOB provider
type ProviderCode string

const (
	ProviderKiosbank  ProviderCode = "kiosbank"
	ProviderAlterra   ProviderCode = "alterra"
	ProviderBRI       ProviderCode = "bri"
	ProviderDigiflazz ProviderCode = "digiflazz"
)

// PPOBProvider represents a PPOB provider in the system
type PPOBProvider struct {
	ID        int             `db:"id" json:"id"`
	Code      ProviderCode    `db:"code" json:"code"`
	Name      string          `db:"name" json:"name"`
	IsActive  bool            `db:"is_active" json:"isActive"`
	IsBackup  bool            `db:"is_backup" json:"isBackup"`
	Priority  int             `db:"priority" json:"priority"`
	Config    json.RawMessage `db:"config" json:"config,omitempty"`
	CreatedAt time.Time       `db:"created_at" json:"-"`
	UpdatedAt time.Time       `db:"updated_at" json:"updatedAt"`
}

// PPOBProviderSKU maps our products to provider's SKUs
type PPOBProviderSKU struct {
	ID                  int        `db:"id" json:"id"`
	ProviderID          int        `db:"provider_id" json:"providerId"`
	ProductID           int        `db:"product_id" json:"productId"`
	ProviderSKUCode     string     `db:"provider_sku_code" json:"providerSkuCode"`
	ProviderProductName string     `db:"provider_product_name" json:"providerProductName,omitempty"`
	Price               int        `db:"price" json:"price"`
	Admin               int        `db:"admin" json:"admin"`
	Commission          int        `db:"commission" json:"commission"`
	IsActive            bool       `db:"is_active" json:"isActive"`
	IsAvailable         bool       `db:"is_available" json:"isAvailable"`
	Stock               *int       `db:"stock" json:"stock,omitempty"`
	LastSyncAt          *time.Time `db:"last_sync_at" json:"lastSyncAt,omitempty"`
	SyncError           *string    `db:"sync_error" json:"syncError,omitempty"`
	CreatedAt           time.Time  `db:"created_at" json:"-"`
	UpdatedAt           time.Time  `db:"updated_at" json:"updatedAt"`

	// Joined fields
	ProviderCode ProviderCode `db:"provider_code" json:"providerCode,omitempty"`
	ProviderName string       `db:"provider_name" json:"providerName,omitempty"`
	ProductName  string       `db:"product_name" json:"productName,omitempty"`
	SkuCode      string       `db:"sku_code" json:"skuCode,omitempty"`
	ProductType  string       `db:"product_type" json:"productType,omitempty"`
	IsBackup     bool         `db:"is_backup" json:"isBackup,omitempty"`
}

// EffectiveAdmin returns admin minus commission
func (s PPOBProviderSKU) EffectiveAdmin() int {
	return s.Admin - s.Commission
}

// PPOBProviderHealth tracks provider performance
type PPOBProviderHealth struct {
	ID                int        `db:"id" json:"id"`
	ProviderID        int        `db:"provider_id" json:"providerId"`
	TotalRequests     int        `db:"total_requests" json:"totalRequests"`
	SuccessCount      int        `db:"success_count" json:"successCount"`
	FailedCount       int        `db:"failed_count" json:"failedCount"`
	LastSuccessAt     *time.Time `db:"last_success_at" json:"lastSuccessAt,omitempty"`
	LastFailureAt     *time.Time `db:"last_failure_at" json:"lastFailureAt,omitempty"`
	LastFailureReason *string    `db:"last_failure_reason" json:"lastFailureReason,omitempty"`
	AvgResponseTimeMs int        `db:"avg_response_time_ms" json:"avgResponseTimeMs"`
	HealthScore       float64    `db:"health_score" json:"healthScore"`
	Date              time.Time  `db:"date" json:"date"`
	CreatedAt         time.Time  `db:"created_at" json:"-"`
	UpdatedAt         time.Time  `db:"updated_at" json:"updatedAt"`

	// Joined fields
	ProviderCode ProviderCode `db:"provider_code" json:"providerCode,omitempty"`
	ProviderName string       `db:"provider_name" json:"providerName,omitempty"`
}

// PPOBProviderCallback stores provider callback data
type PPOBProviderCallback struct {
	ID            int             `db:"id" json:"id"`
	ProviderID    int             `db:"provider_id" json:"providerId"`
	ProviderCode  ProviderCode    `db:"-" json:"providerCode"` // Used internally, not in DB directly
	ProviderRefID string          `db:"provider_ref_id" json:"providerRefId"`
	TransactionID int             `db:"transaction_id" json:"transactionId"`
	Payload       json.RawMessage `db:"payload" json:"payload"`
	RC            string          `db:"-" json:"rc"` // Extracted RC code
	Status        *string         `db:"status" json:"status,omitempty"`
	Message       *string         `db:"message" json:"message,omitempty"`
	IsProcessed   bool            `db:"is_processed" json:"isProcessed"`
	ProcessedAt   *time.Time      `db:"processed_at" json:"processedAt,omitempty"`
	ProcessError  *string         `db:"process_error" json:"processError,omitempty"`
	CreatedAt     time.Time       `db:"created_at" json:"createdAt"`
}

// ProviderOption represents a provider option for transaction execution
type ProviderOption struct {
	ProviderID      int          `db:"provider_id" json:"providerId"`
	ProviderCode    ProviderCode `db:"provider_code" json:"providerCode"`
	ProviderSKUID   int          `db:"provider_sku_id" json:"providerSkuId"`
	ProviderSKUCode string       `db:"provider_sku_code" json:"providerSkuCode"`
	Price           int          `db:"price" json:"price"`
	Admin           int          `db:"admin" json:"admin"`
	Commission      int          `db:"commission" json:"commission"`
	IsBackup        bool         `db:"is_backup" json:"isBackup"`
}

// EffectiveAdmin returns admin minus commission (what customer effectively pays in admin)
func (o ProviderOption) EffectiveAdmin() int {
	return o.Admin - o.Commission
}

// ProductWithBestPrice represents product with best price from all providers
type ProductWithBestPrice struct {
	ID            int         `db:"id" json:"id"`
	SkuCode       string      `db:"sku_code" json:"skuCode"`
	Name          string      `db:"name" json:"productName"`
	Category      string      `db:"category" json:"category"`
	Brand         string      `db:"brand" json:"brand"`
	Type          ProductType `db:"type" json:"type"`
	Admin         int         `db:"admin" json:"admin"`
	BestPrice     *int        `db:"best_price" json:"price"`
	BestAdmin     *int        `db:"best_admin" json:"providerAdmin,omitempty"`
	IsActive      bool        `db:"is_active" json:"productStatus"`
	Description   string      `db:"description" json:"description,omitempty"`
	ProviderCount int         `db:"provider_count" json:"providerCount,omitempty"`
}
