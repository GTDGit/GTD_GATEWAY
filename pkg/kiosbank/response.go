package kiosbank

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// BaseResponse is common response fields
type BaseResponse struct {
	RC          string `json:"rc"`
	Description string `json:"description,omitempty"`
	MerchantID  string `json:"merchantID,omitempty"`
	SessionID   string `json:"sessionID,omitempty"`
}

// SignOnResponse is the response from Sign On
type SignOnResponse struct {
	BaseResponse
}

func (r *SignOnResponse) UnmarshalJSON(data []byte) error {
	type alias SignOnResponse
	aux := struct {
		alias          alias
		SessionIDUpper string `json:"SessionID"`
	}{}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	*r = SignOnResponse(aux.alias)
	if r.BaseResponse.SessionID == "" {
		r.BaseResponse.SessionID = aux.SessionIDUpper
	}
	return nil
}

// InquiryData contains inquiry result data
type InquiryData struct {
	IDPelanggan    string          `json:"idPelanggan"`
	Nama           string          `json:"nama"`
	TotalTagihan   string          `json:"totalTagihan,omitempty"`
	JumlahTagihan  string          `json:"jumlahTagihan,omitempty"`
	Admin          string          `json:"admin,omitempty"`
	Total          string          `json:"total,omitempty"`
	Period         string          `json:"periode,omitempty"`
	NoReferensi    string          `json:"noReferensi,omitempty"`
	Info           string          `json:"info,omitempty"`
	RincianTagihan json.RawMessage `json:"rincianTagihan,omitempty"`
}

// InquiryResponse is the response from Inquiry
type InquiryResponse struct {
	BaseResponse
	CustomerID  string          `json:"customerID"`
	ProductID   string          `json:"productID"`
	ReferenceID string          `json:"referenceID"`
	Data        json.RawMessage `json:"data"`
}

// PaymentData contains payment result data
type PaymentData struct {
	IDPelanggan  string          `json:"idPelanggan"`
	Nama         string          `json:"nama"`
	NoReferensi  string          `json:"noReferensi"`
	Tagihan      string          `json:"tagihan,omitempty"`
	Admin        string          `json:"admin,omitempty"`
	Total        string          `json:"total,omitempty"`
	Status       string          `json:"status,omitempty"`
	SerialNumber string          `json:"sn,omitempty"`
	Token        string          `json:"token,omitempty"`
	KWH          string          `json:"kwh,omitempty"`
	Info         json.RawMessage `json:"info,omitempty"`
}

// PaymentResponse is the response from Payment
type PaymentResponse struct {
	BaseResponse
	CustomerID  string          `json:"customerID"`
	ProductID   string          `json:"productID"`
	ReferenceID string          `json:"referenceID"`
	Data        json.RawMessage `json:"data"`
}

// SinglePaymentData contains single payment result data
type SinglePaymentData struct {
	IDPelanggan string `json:"idPelanggan"`
	Nama        string `json:"nama"`
	NoReferensi string `json:"noReferensi"`
	Harga       string `json:"harga"`
	Status      string `json:"status"`
}

// SinglePaymentResponse is the response from Single Payment
type SinglePaymentResponse struct {
	BaseResponse
	CustomerID  string          `json:"customerID"`
	ProductID   string          `json:"productID"`
	ReferenceID string          `json:"referenceID"`
	Data        json.RawMessage `json:"data"`
}

// CheckStatusResponse is the response from Check Status
type CheckStatusResponse struct {
	BaseResponse
	CustomerID  string          `json:"customerID"`
	ProductID   string          `json:"productID"`
	ReferenceID string          `json:"referenceID"`
	Data        json.RawMessage `json:"data"`
}

// toPaymentResponse converts CheckStatusResponse to PaymentResponse for reuse
func (r *CheckStatusResponse) ToPaymentResponse() *PaymentResponse {
	return &PaymentResponse{
		BaseResponse: r.BaseResponse,
		CustomerID:   r.CustomerID,
		ProductID:    r.ProductID,
		ReferenceID:  r.ReferenceID,
		Data:         r.Data,
	}
}

// PriceListItem represents a product in price list
// getDaftar-Harga returns: code, name, category, price, status
// getPulsa-Prabayar returns: code, name, price
type PriceListItem struct {
	Code     string `json:"code"`
	Name     string `json:"name"`
	Category string `json:"category,omitempty"`
	Price    string `json:"price,omitempty"`
	Status   string `json:"status,omitempty"`
}

func (p *PriceListItem) UnmarshalJSON(data []byte) error {
	type alias struct {
		Code     string          `json:"code"`
		Name     string          `json:"name"`
		Category string          `json:"category,omitempty"`
		Price    json.RawMessage `json:"price,omitempty"`
		Status   string          `json:"status,omitempty"`
	}

	var aux alias
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	p.Code = aux.Code
	p.Name = aux.Name
	p.Category = aux.Category
	p.Status = aux.Status

	if len(aux.Price) == 0 || bytes.Equal(aux.Price, []byte("null")) {
		p.Price = ""
		return nil
	}

	var asString string
	if err := json.Unmarshal(aux.Price, &asString); err == nil {
		p.Price = asString
		return nil
	}

	var asNumber json.Number
	if err := json.Unmarshal(aux.Price, &asNumber); err == nil {
		p.Price = asNumber.String()
		return nil
	}

	return fmt.Errorf("invalid price field: %s", string(aux.Price))
}

// PriceListResponse is the response from Price List
type PriceListResponse struct {
	BaseResponse
	Record []PriceListItem `json:"record"`
}

// CallbackPayload represents the callback from Kiosbank
type CallbackPayload struct {
	MerchantID  string          `json:"merchantID"`
	ProductID   string          `json:"productID"`
	CustomerID  string          `json:"customerID"`
	ReferenceID string          `json:"referenceID"`
	RC          string          `json:"rc"`
	Status      string          `json:"status"`
	Message     string          `json:"message,omitempty"`
	Data        json.RawMessage `json:"data,omitempty"`
}
