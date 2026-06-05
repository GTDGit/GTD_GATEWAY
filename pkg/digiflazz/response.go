package digiflazz

import "encoding/json"

// TransactionResponseWrapper wraps the transaction response from Digiflazz.
// Digiflazz always wraps the response in a "data" field.
type TransactionResponseWrapper struct {
	Data TransactionResponse `json:"data"`
}

// TransactionResponse represents the response from Digiflazz for
// topup/inquiry/payment transactions.
type TransactionResponse struct {
	RefID          string `json:"ref_id"`
	CustomerNo     string `json:"customer_no"`
	BuyerSkuCode   string `json:"buyer_sku_code"`
	Message        string `json:"message"`
	Status         string `json:"status"`
	RC             string `json:"rc"`
	SN             string `json:"sn,omitempty"`
	BuyerLastSaldo int    `json:"buyer_last_saldo"`
	Price          int    `json:"price"`
	Tele           string `json:"tele,omitempty"`

	// Postpaid specific
	CustomerName string          `json:"customer_name,omitempty"`
	Admin        int             `json:"admin,omitempty"`
	Selling      int             `json:"selling_price,omitempty"`
	Desc         json.RawMessage `json:"desc,omitempty"`
}

// PricelistResponse represents the pricelist payload.
type PricelistResponse struct {
	Data []PricelistItem `json:"data"`
}

// PricelistItem represents a single item in the Digiflazz pricelist.
type PricelistItem struct {
	ProductName         string `json:"product_name"`
	Category            string `json:"category"`
	Brand               string `json:"brand"`
	Type                string `json:"type,omitempty"` // Only for prepaid
	SellerName          string `json:"seller_name"`
	Price               int    `json:"price,omitempty"`      // Only for prepaid
	Admin               int    `json:"admin,omitempty"`      // Only for postpaid
	Commission          int    `json:"commission,omitempty"` // Only for postpaid
	BuyerSkuCode        string `json:"buyer_sku_code"`
	BuyerProductStatus  bool   `json:"buyer_product_status"`
	SellerProductStatus bool   `json:"seller_product_status"`
	UnlimitedStock      bool   `json:"unlimited_stock,omitempty"` // Only for prepaid
	Stock               int    `json:"stock,omitempty"`           // Only for prepaid
	Multi               bool   `json:"multi,omitempty"`           // Only for prepaid
	StartCutOff         string `json:"start_cut_off,omitempty"`   // Only for prepaid
	EndCutOff           string `json:"end_cut_off,omitempty"`     // Only for prepaid
	Desc                string `json:"desc"`
}

// BalanceResponse represents the balance (deposit) response from Digiflazz.
type BalanceResponse struct {
	Deposit int `json:"deposit"`
}

// CallbackPayload represents the payload Digiflazz posts to our webhook endpoint.
type CallbackPayload struct {
	RefID        string `json:"ref_id"`
	CustomerNo   string `json:"customer_no"`
	BuyerSkuCode string `json:"buyer_sku_code"`
	Message      string `json:"message"`
	Status       string `json:"status"`
	RC           string `json:"rc"`
	SN           string `json:"sn,omitempty"`
	Price        int    `json:"price"`

	// Postpaid
	CustomerName string          `json:"customer_name,omitempty"`
	Admin        int             `json:"admin,omitempty"`
	Desc         json.RawMessage `json:"desc,omitempty"`
}
