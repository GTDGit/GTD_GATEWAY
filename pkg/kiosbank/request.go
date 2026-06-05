package kiosbank

// SignOnRequest is the request for Sign On
type SignOnRequest struct {
	MerchantID   string `json:"merchantID"`
	MerchantName string `json:"merchantName"`
	CounterID    string `json:"counterID"`
	AccountID    string `json:"accountID"`
	Mitra        string `json:"mitra"`
}

// InquiryRequest is the request for Inquiry
type InquiryRequest struct {
	SessionID   string `json:"sessionID"`
	MerchantID  string `json:"merchantID"`
	ProductID   string `json:"productID"`
	CustomerID  string `json:"customerID"`
	ReferenceID string `json:"referenceID"`
	Periode     string `json:"periode,omitempty"` // BPJS: "01"-"12" (months)
}

// PaymentRequest is the request for Payment
type PaymentRequest struct {
	SessionID   string `json:"sessionID"`
	MerchantID  string `json:"merchantID"`
	ProductID   string `json:"productID"`
	CustomerID  string `json:"customerID"`
	ReferenceID string `json:"referenceID"`
	Tagihan     string `json:"tagihan"`              // 12 digits
	Admin       string `json:"admin"`                // 12 digits
	Total       string `json:"total"`                // 12 digits
	NoHandphone string `json:"noHanphone,omitempty"` // BPJS: nomor HP peserta
	Nama        string `json:"nama,omitempty"`       // K-Vision Paket: nama paket
	Kode        string `json:"kode,omitempty"`       // K-Vision Paket: kode paket
}

// SinglePaymentRequest is the request for Single Payment (prepaid)
type SinglePaymentRequest struct {
	SessionID   string `json:"sessionID"`
	MerchantID  string `json:"merchantID"`
	ProductID   string `json:"productID"`
	CustomerID  string `json:"customerID"`
	ReferenceID string `json:"referenceID"`
	Tagihan     string `json:"tagihan"` // 12 digits
	Admin       string `json:"admin"`   // 12 digits
	Total       string `json:"total"`   // 12 digits
}

// CheckStatusRequest is the request for Check Status
type CheckStatusRequest struct {
	SessionID    string `json:"sessionID"`
	MerchantID   string `json:"merchantID"`
	ProductID    string `json:"productID"`
	CustomerID   string `json:"customerID"`
	ReferenceID  string `json:"referenceID"`
	Tagihan      string `json:"tagihan"`
	Admin        string `json:"admin"`
	Total        string `json:"total"`
	NoHandphone  string `json:"noHanphone,omitempty"`
	Nama         string `json:"nama,omitempty"`
	Kode         string `json:"kode,omitempty"`
	TglTransaksi string `json:"tglTransaksi"` // YYYY-MM-DD, required
}

// PriceListRequest is the request for price list
type PriceListRequest struct {
	SessionID  string `json:"sessionID"`
	MerchantID string `json:"merchantID"`
	PrefixID   string `json:"prefixID,omitempty"`
}
