package digiflazz

// TopupRequest represents a prepaid top-up request to Digiflazz.
type TopupRequest struct {
	Username     string `json:"username"`
	BuyerSkuCode string `json:"buyer_sku_code"`
	CustomerNo   string `json:"customer_no"`
	RefID        string `json:"ref_id"`
	Sign         string `json:"sign"`
	Testing      bool   `json:"testing,omitempty"`
}

// InquiryRequest represents a postpaid inquiry request to Digiflazz.
type InquiryRequest struct {
	Commands     string `json:"commands"` // "inq-pasca"
	Username     string `json:"username"`
	BuyerSkuCode string `json:"buyer_sku_code"`
	CustomerNo   string `json:"customer_no"`
	RefID        string `json:"ref_id"`
	Sign         string `json:"sign"`
	Testing      bool   `json:"testing,omitempty"`
}

// PaymentRequest represents a postpaid payment request to Digiflazz.
type PaymentRequest struct {
	Commands     string `json:"commands"` // "pay-pasca"
	Username     string `json:"username"`
	BuyerSkuCode string `json:"buyer_sku_code"`
	CustomerNo   string `json:"customer_no"`
	RefID        string `json:"ref_id"`
	Sign         string `json:"sign"`
	Testing      bool   `json:"testing,omitempty"`
}

// PricelistRequest represents a price-list request.
type PricelistRequest struct {
	Cmd      string `json:"cmd"` // "prepaid" or "pasca"
	Username string `json:"username"`
	Sign     string `json:"sign"`
}

// BalanceRequest represents a balance (deposit) request.
type BalanceRequest struct {
	Cmd      string `json:"cmd"` // "deposit"
	Username string `json:"username"`
	Sign     string `json:"sign"`
}
