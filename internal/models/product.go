package models

import "time"

// ProductType enumerates the supported product types (prepaid/postpaid only).
type ProductType string

const (
	ProductTypePrepaid  ProductType = "prepaid"
	ProductTypePostpaid ProductType = "postpaid"
)

// Product represents a product definition in the catalog.
// Type = prepaid/postpaid (transaction type). Variant = Reguler, Pulsa Transfer, etc (optional).
type Product struct {
	ID          int         `db:"id" json:"id"`
	SkuCode     string      `db:"sku_code" json:"skuCode"`
	Name        string      `db:"name" json:"productName"`
	Category    string      `db:"category" json:"category"`
	Brand       string      `db:"brand" json:"brand"`
	Type        ProductType `db:"type" json:"type"`
	VariantID   *int        `db:"variant_id" json:"variantId,omitempty"`
	VariantName string      `db:"variant_name" json:"variantName,omitempty"` // from JOIN product_variants
	Admin       int         `db:"admin" json:"admin,omitempty"`
	Commission  int         `db:"commission" json:"commission,omitempty"`
	Description string      `db:"description" json:"description"`
	IsActive    bool        `db:"is_active" json:"productStatus"`
	CreatedAt   time.Time   `db:"created_at" json:"-"`
	UpdatedAt   time.Time   `db:"updated_at" json:"updatedAt"`

	ProviderCount int  `db:"provider_count" json:"providerCount"`
	MinPrice      *int `db:"min_price" json:"minPrice,omitempty"`
	MinAdmin      *int `db:"min_admin" json:"minAdmin,omitempty"`
}
