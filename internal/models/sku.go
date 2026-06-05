package models

import "time"

// SKU represents a specific seller's stock-keeping unit entry for a product.
type SKU struct {
    ID             int       `db:"id" json:"id"`
    ProductID      int       `db:"product_id" json:"productId"`
    DigiSkuCode    string    `db:"digi_sku_code" json:"digiSkuCode"`
    SellerName     string    `db:"seller_name" json:"sellerName"`
    Priority       int       `db:"priority" json:"priority"`
    Price          int       `db:"price" json:"price"`
    IsActive       bool      `db:"is_active" json:"isActive"`
    SupportMulti   bool      `db:"support_multi" json:"supportMulti"`
    UnlimitedStock bool      `db:"unlimited_stock" json:"unlimitedStock"`
    Stock          int       `db:"stock" json:"stock"`
    CutOffStart    string    `db:"cut_off_start" json:"cutOffStart"` // TIME as string "HH:MM:SS"
    CutOffEnd      string    `db:"cut_off_end" json:"cutOffEnd"`
    CreatedAt      time.Time `db:"created_at" json:"-"`
    UpdatedAt      time.Time `db:"updated_at" json:"updatedAt"`
}
