package kiosbank

import (
	"context"
	"fmt"
)

type generalPriceListPrefix struct {
	PrefixID string
	Category string
}

var generalPriceListPrefixes = []generalPriceListPrefix{
	{PrefixID: "AAB.AAAA", Category: "STREAMING"},
	{PrefixID: "AAB.AAAB", Category: "STREAMING"},
	{PrefixID: "AAB.AAAC", Category: "STREAMING"},
	{PrefixID: "AAB.AAAD", Category: "STREAMING"},
	{PrefixID: "AAB.AAAE", Category: "STREAMING"},
	{PrefixID: "AAB.AAAG", Category: "STREAMING"},
	{PrefixID: "AAB.AAAH", Category: "STREAMING"},
	{PrefixID: "AAB.AAAI", Category: "STREAMING"},
	{PrefixID: "AAB.AAAJ", Category: "STREAMING"},
	{PrefixID: "AAB.AAAK", Category: "STREAMING"},
	{PrefixID: "AAB.AAAL", Category: "STREAMING"},
	{PrefixID: "AAB.AAAM", Category: "STREAMING"},
	{PrefixID: "AAB.AAAN", Category: "STREAMING"},
	{PrefixID: "AAB.AAAO", Category: "STREAMING"},
	{PrefixID: "AAB.AAAP", Category: "STREAMING"},
	{PrefixID: "AAB.AAAQ", Category: "STREAMING"},
	{PrefixID: "AAB.AAAR", Category: "STREAMING"},
	{PrefixID: "AAB.AAAS", Category: "STREAMING"},
	{PrefixID: "AAB.AAAT", Category: "STREAMING"},
	{PrefixID: "AAB.AAAU", Category: "STREAMING"},
	{PrefixID: "AAB.AAAV", Category: "STREAMING"},
	{PrefixID: "AAB.AAAW", Category: "STREAMING"},
	{PrefixID: "AAB.AAAX", Category: "STREAMING"},
	{PrefixID: "AAB.AAAY", Category: "STREAMING"},
	{PrefixID: "AAB.AAAZ", Category: "STREAMING"},
	{PrefixID: "AAB.AABA", Category: "STREAMING"},
	{PrefixID: "AAE.AAAA", Category: "PULSA"},
	{PrefixID: "AAE.AAAB", Category: "PULSA"},
	{PrefixID: "AAE.AAAC", Category: "PULSA"},
	{PrefixID: "AAE.AAAD", Category: "PULSA"},
	{PrefixID: "AAE.AAAE", Category: "PULSA"},
	{PrefixID: "AAE.AAAF", Category: "PULSA"},
	{PrefixID: "AAE.AAAG", Category: "PULSA"},
	{PrefixID: "AAE.AAAH", Category: "PULSA"},
	{PrefixID: "AAH.AAAL", Category: "TV BERBAYAR"},
	{PrefixID: "AAN.AAAA", Category: "VOUCHER"},
	{PrefixID: "AAN.AAAB", Category: "VOUCHER"},
	{PrefixID: "AAN.AAAC", Category: "VOUCHER"},
	{PrefixID: "AAO.AAAA", Category: "VOUCHER GAME"},
	{PrefixID: "AAO.AAAB", Category: "VOUCHER GAME"},
	{PrefixID: "AAO.AAAD", Category: "VOUCHER GAME"},
	{PrefixID: "AAO.AAAE", Category: "VOUCHER GAME"},
	{PrefixID: "AAO.AAAF", Category: "VOUCHER GAME"},
	{PrefixID: "AAO.AAAG", Category: "VOUCHER GAME"},
	{PrefixID: "AAO.AAAH", Category: "VOUCHER GAME"},
	{PrefixID: "AAO.AAAI", Category: "VOUCHER GAME"},
	{PrefixID: "AAO.AAAJ", Category: "VOUCHER GAME"},
	{PrefixID: "AAO.AAAK", Category: "VOUCHER GAME"},
	{PrefixID: "AAO.AAAL", Category: "VOUCHER GAME"},
	{PrefixID: "AAO.AAAN", Category: "VOUCHER GAME"},
	{PrefixID: "AAO.AAAO", Category: "VOUCHER GAME"},
	{PrefixID: "AAO.AAAP", Category: "VOUCHER GAME"},
	{PrefixID: "AAO.AAAR", Category: "VOUCHER GAME"},
	{PrefixID: "AAO.AAAS", Category: "VOUCHER GAME"},
	{PrefixID: "AAO.AAAT", Category: "VOUCHER GAME"},
	{PrefixID: "AAO.AAAU", Category: "VOUCHER GAME"},
	{PrefixID: "AAO.AAAV", Category: "VOUCHER GAME"},
	{PrefixID: "AAO.AAAW", Category: "VOUCHER GAME"},
	{PrefixID: "AAO.AAAX", Category: "VOUCHER GAME"},
	{PrefixID: "AAO.AAAY", Category: "VOUCHER GAME"},
	{PrefixID: "AAO.AAAZ", Category: "VOUCHER GAME"},
	{PrefixID: "AAO.AABA", Category: "VOUCHER GAME"},
}

// SignOn performs authentication and returns session ID
func (c *Client) SignOn(ctx context.Context) (*SignOnResponse, error) {
	req := SignOnRequest{
		MerchantID:   c.config.MerchantID,
		MerchantName: c.config.MerchantName,
		CounterID:    c.config.CounterID,
		AccountID:    c.config.AccountID,
		Mitra:        c.config.Mitra,
	}

	var resp SignOnResponse
	if err := c.doRequest(ctx, "/auth/Sign-On", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Inquiry checks customer bill information
func (c *Client) Inquiry(ctx context.Context, productID, customerID, referenceID, periode string) (*InquiryResponse, error) {
	for attempt := 0; attempt < 2; attempt++ {
		sessionID, err := c.ensureSession(ctx)
		if err != nil {
			return nil, err
		}

		req := InquiryRequest{
			SessionID:   sessionID,
			MerchantID:  c.config.MerchantID,
			ProductID:   productID,
			CustomerID:  customerID,
			ReferenceID: formatReferenceID(referenceID),
			Periode:     periode,
		}

		var resp InquiryResponse
		if err := c.doRequest(ctx, "/Services/Inquiry", req, &resp); err != nil {
			return nil, &TransportError{Err: err}
		}
		if !IsSessionExpired(resp.RC) || attempt == 1 {
			return &resp, nil
		}
		c.sessionMu.Lock()
		c.clearSession()
		c.sessionMu.Unlock()
	}
	return nil, fmt.Errorf("inquiry retry exhausted")
}

// Payment pays a postpaid bill
func (c *Client) Payment(ctx context.Context, productID, customerID, referenceID string, tagihan, admin, total int, noHandphone, nama, kode string) (*PaymentResponse, error) {
	for attempt := 0; attempt < 2; attempt++ {
		sessionID, err := c.ensureSession(ctx)
		if err != nil {
			return nil, err
		}

		req := PaymentRequest{
			SessionID:   sessionID,
			MerchantID:  c.config.MerchantID,
			ProductID:   productID,
			CustomerID:  customerID,
			ReferenceID: formatReferenceID(referenceID),
			Tagihan:     formatAmount(tagihan),
			Admin:       formatAmount(admin),
			Total:       formatAmount(total),
			NoHandphone: noHandphone,
			Nama:        nama,
			Kode:        kode,
		}

		var resp PaymentResponse
		if err := c.doRequest(ctx, "/Services/Payment", req, &resp); err != nil {
			return nil, &TransportError{Err: err}
		}
		if !IsSessionExpired(resp.RC) || attempt == 1 {
			return &resp, nil
		}
		c.sessionMu.Lock()
		c.clearSession()
		c.sessionMu.Unlock()
	}
	return nil, fmt.Errorf("payment retry exhausted")
}

// SinglePayment performs prepaid transaction (pulsa/data)
func (c *Client) SinglePayment(ctx context.Context, productID, customerID, referenceID string, tagihan, admin, total int) (*SinglePaymentResponse, error) {
	for attempt := 0; attempt < 2; attempt++ {
		sessionID, err := c.ensureSession(ctx)
		if err != nil {
			return nil, err
		}

		req := SinglePaymentRequest{
			SessionID:   sessionID,
			MerchantID:  c.config.MerchantID,
			ProductID:   productID,
			CustomerID:  customerID,
			ReferenceID: formatReferenceID(referenceID),
			Tagihan:     formatAmount(tagihan),
			Admin:       formatAmount(admin),
			Total:       formatAmount(total),
		}

		var resp SinglePaymentResponse
		if err := c.doRequest(ctx, "/Services/SinglePayment", req, &resp); err != nil {
			return nil, &TransportError{Err: err}
		}
		if !IsSessionExpired(resp.RC) || attempt == 1 {
			return &resp, nil
		}
		c.sessionMu.Lock()
		c.clearSession()
		c.sessionMu.Unlock()
	}
	return nil, fmt.Errorf("single payment retry exhausted")
}

// CheckStatus checks transaction status
// tglTransaksi must be YYYY-MM-DD format (date of original payment)
func (c *Client) CheckStatus(ctx context.Context, productID, customerID, referenceID string, tagihan, admin, total int, tglTransaksi, noHandphone, nama, kode string) (*CheckStatusResponse, error) {
	for attempt := 0; attempt < 2; attempt++ {
		sessionID, err := c.ensureSession(ctx)
		if err != nil {
			return nil, err
		}

		req := CheckStatusRequest{
			SessionID:    sessionID,
			MerchantID:   c.config.MerchantID,
			ProductID:    productID,
			CustomerID:   customerID,
			ReferenceID:  formatReferenceID(referenceID),
			Tagihan:      formatAmount(tagihan),
			Admin:        formatAmount(admin),
			Total:        formatAmount(total),
			NoHandphone:  noHandphone,
			Nama:         nama,
			Kode:         kode,
			TglTransaksi: tglTransaksi,
		}

		var resp CheckStatusResponse
		if err := c.doRequest(ctx, "/Services/Check-Status", req, &resp); err != nil {
			return nil, &TransportError{Err: err}
		}
		if !IsSessionExpired(resp.RC) || attempt == 1 {
			return &resp, nil
		}
		c.sessionMu.Lock()
		c.clearSession()
		c.sessionMu.Unlock()
	}
	return nil, fmt.Errorf("check status retry exhausted")
}

// GetPriceListPulsa gets prepaid (pulsa/data) price list
func (c *Client) GetPriceListPulsa(ctx context.Context) (*PriceListResponse, error) {
	sessionID, err := c.ensureSession(ctx)
	if err != nil {
		return nil, err
	}

	prefixes := []string{"11", "21", "31", "41", "51", "81"}
	merged := make([]PriceListItem, 0)
	seen := make(map[string]struct{})

	for _, prefixID := range prefixes {
		req := PriceListRequest{
			SessionID:  sessionID,
			MerchantID: c.config.MerchantID,
			PrefixID:   prefixID,
		}

		var resp PriceListResponse
		if err := c.doRequest(ctx, "/Services/getPulsa-Prabayar", req, &resp); err != nil {
			return nil, fmt.Errorf("prefix %s: %w", prefixID, err)
		}
		for _, item := range resp.Record {
			if _, ok := seen[item.Code]; ok {
				continue
			}
			seen[item.Code] = struct{}{}
			merged = append(merged, item)
		}
	}

	return &PriceListResponse{
		BaseResponse: BaseResponse{RC: RCSuccess},
		Record:       merged,
	}, nil
}

// GetPriceList gets general price list
func (c *Client) GetPriceList(ctx context.Context) (*PriceListResponse, error) {
	sessionID, err := c.ensureSession(ctx)
	if err != nil {
		return nil, err
	}

	merged := make([]PriceListItem, 0, len(generalPriceListPrefixes))
	seen := make(map[string]struct{})

	for _, prefix := range generalPriceListPrefixes {
		req := PriceListRequest{
			SessionID:  sessionID,
			MerchantID: c.config.MerchantID,
			PrefixID:   prefix.PrefixID,
		}

		var resp PriceListResponse
		if err := c.doRequest(ctx, "/Services/getHargaByProductID", req, &resp); err != nil {
			return nil, fmt.Errorf("prefix %s: %w", prefix.PrefixID, err)
		}

		for _, item := range resp.Record {
			if _, ok := seen[item.Code]; ok {
				continue
			}
			seen[item.Code] = struct{}{}
			if item.Category == "" {
				item.Category = prefix.Category
			}
			if item.Status == "" {
				item.Status = "AKTIF"
			}
			merged = append(merged, item)
		}
	}

	return &PriceListResponse{
		BaseResponse: BaseResponse{RC: RCSuccess},
		Record:       merged,
	}, nil
}
