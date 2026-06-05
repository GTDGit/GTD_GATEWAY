package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/GTDGit/gtd_gateway/internal/models"
	"github.com/GTDGit/gtd_gateway/internal/repository"
	"github.com/GTDGit/gtd_gateway/pkg/kiosbank"
)

// KiosbankProviderClient implements PPOBProviderClient for Kiosbank
type KiosbankProviderClient struct {
	prodClient   *kiosbank.Client
	devClient    *kiosbank.Client
	trxRepo      *repository.TransactionRepository
	callbackRepo *repository.CallbackRepository
	providerRepo *repository.PPOBProviderRepository
	healthy      bool
	healthMu     sync.RWMutex
}

// NewKiosbankProviderClient creates a new Kiosbank provider client
func NewKiosbankProviderClient(prodClient, devClient *kiosbank.Client, trxRepo *repository.TransactionRepository, callbackRepo *repository.CallbackRepository, providerRepo *repository.PPOBProviderRepository) *KiosbankProviderClient {
	return &KiosbankProviderClient{
		prodClient:   prodClient,
		devClient:    devClient,
		trxRepo:      trxRepo,
		callbackRepo: callbackRepo,
		providerRepo: providerRepo,
		healthy:      true,
	}
}

// Code returns the provider code
func (c *KiosbankProviderClient) Code() models.ProviderCode {
	return models.ProviderKiosbank
}

// getClient returns the appropriate client based on sandbox mode
func (c *KiosbankProviderClient) getClient(isSandbox bool) *kiosbank.Client {
	if isSandbox {
		return c.devClient
	}
	return c.prodClient
}

// Topup processes a prepaid transaction via SinglePayment
func (c *KiosbankProviderClient) Topup(ctx context.Context, req *ProviderRequest) (*ProviderResponse, error) {
	client := c.getClient(req.IsSandbox)
	startTime := time.Now()

	price := req.Amount
	admin, _ := intValueOK(req.Extra["admin"])
	total := price + admin

	req.RefID = resolveKiosbankReferenceID(req.RefID)

	resp, err := client.SinglePayment(ctx, req.SKUCode, req.CustomerNo, req.RefID, price, admin, total)
	responseTime := time.Since(startTime)
	if err != nil {
		c.markUnhealthy()
		if kiosbank.IsTransportUncertainError(err) {
			return pendingTransportResponse(req.RefID, responseTime, err), nil
		}
		return nil, err
	}

	c.markHealthy()
	return c.convertSinglePaymentResponse(resp, req.RefID, price, admin, responseTime), nil
}

// Inquiry checks a postpaid bill
func (c *KiosbankProviderClient) Inquiry(ctx context.Context, req *ProviderRequest) (*ProviderResponse, error) {
	client := c.getClient(req.IsSandbox)
	startTime := time.Now()

	req.Extra = normalizeKiosbankRequestData(req.Extra)
	periode := stringFromKeys(req.Extra, "periode")
	if kiosbankRequiresPeriode(req.SKUCode) && periode == "" {
		return kiosbankValidationResponse(req.RefID, "missing required Kiosbank field: periode"), nil
	}

	req.RefID = resolveKiosbankReferenceID(req.RefID)

	resp, err := client.Inquiry(ctx, req.SKUCode, req.CustomerNo, req.RefID, periode)
	responseTime := time.Since(startTime)
	if err != nil {
		c.markUnhealthy()
		return nil, err
	}

	c.markHealthy()
	return c.convertInquiryResponse(resp, req.RefID, responseTime), nil
}

// Payment pays a postpaid bill
func (c *KiosbankProviderClient) Payment(ctx context.Context, req *ProviderRequest) (*ProviderResponse, error) {
	client := c.getClient(req.IsSandbox)
	startTime := time.Now()

	tagihan := req.Amount
	admin, _ := intValueOK(req.Extra["admin"])
	total := tagihan + admin

	req.Extra = normalizeKiosbankRequestData(req.Extra)
	noHandphone := stringFromKeys(req.Extra, "noHandphone", "noHanphone")
	nama := stringFromKeys(req.Extra, "nama")
	kode := stringFromKeys(req.Extra, "kode")

	if kiosbankRequiresNoHandphone(req.SKUCode) && noHandphone == "" {
		return kiosbankValidationResponse(req.RefID, "missing required Kiosbank field: noHandphone"), nil
	}
	if kiosbankRequiresPackageFields(req.SKUCode) {
		if nama == "" {
			return kiosbankValidationResponse(req.RefID, "missing required Kiosbank field: nama"), nil
		}
		if kode == "" {
			return kiosbankValidationResponse(req.RefID, "missing required Kiosbank field: kode"), nil
		}
	}

	req.RefID = resolveKiosbankReferenceID(req.RefID)

	resp, err := client.Payment(ctx, req.SKUCode, req.CustomerNo, req.RefID, tagihan, admin, total, noHandphone, nama, kode)
	responseTime := time.Since(startTime)
	if err != nil {
		c.markUnhealthy()
		if kiosbank.IsTransportUncertainError(err) {
			return pendingTransportResponse(req.RefID, responseTime, err), nil
		}
		return nil, err
	}

	c.markHealthy()
	return c.convertPaymentResponse(resp, req.RefID, tagihan, admin, responseTime), nil
}

// CheckStatus checks transaction status by looking up original transaction data
func (c *KiosbankProviderClient) CheckStatus(ctx context.Context, refID string) (*ProviderResponse, error) {
	trx, err := c.trxRepo.GetByProviderRefID(refID)
	if err != nil {
		return nil, fmt.Errorf("cannot check status: transaction not found for ref %s: %w", refID, err)
	}
	if trx.ProviderSKUID == nil {
		return nil, fmt.Errorf("cannot check status: no provider SKU ID for transaction %s", trx.TransactionID)
	}

	providerSKU, err := c.providerRepo.GetProviderSKUByID(*trx.ProviderSKUID)
	if err != nil {
		return nil, fmt.Errorf("cannot check status: failed to get provider SKU: %w", err)
	}

	var logs []models.TransactionLog
	if c.callbackRepo != nil {
		logs, _ = c.callbackRepo.GetLogsByTransactionID(trx.ID)
	}

	input := buildKiosbankCheckStatusInput(trx, logs)
	tglTransaksi := trx.CreatedAt.Format("2006-01-02")

	client := c.getClient(trx.IsSandbox)
	startTime := time.Now()

	resp, err := client.CheckStatus(
		ctx,
		providerSKU.ProviderSKUCode,
		trx.CustomerNo,
		input.ReferenceID,
		input.Tagihan,
		input.Admin,
		input.Total,
		tglTransaksi,
		input.NoHandphone,
		input.Nama,
		input.Kode,
	)
	responseTime := time.Since(startTime)
	if err != nil {
		c.markUnhealthy()
		return nil, err
	}

	c.markHealthy()
	return c.convertAsyncPaymentResponse(resp.ToPaymentResponse(), input.ReferenceID, input.Tagihan, input.Admin, responseTime), nil
}

// GetPriceList fetches current prices
func (c *KiosbankProviderClient) GetPriceList(ctx context.Context, category string) ([]ProviderProduct, error) {
	client := c.getClient(false)

	var products []ProviderProduct

	pulsaResp, err := client.GetPriceListPulsa(ctx)
	if err != nil {
		return nil, err
	}
	for _, p := range pulsaResp.Record {
		price := parseAmount(p.Price)
		products = append(products, ProviderProduct{
			SKUCode:     p.Code,
			ProductName: p.Name,
			Price:       price,
			IsActive:    true,
		})
	}

	generalResp, err := client.GetPriceList(ctx)
	if err != nil {
		return nil, err
	}
	for _, p := range generalResp.Record {
		if category != "" && !strings.EqualFold(p.Category, category) {
			continue
		}
		price := parseAmount(p.Price)
		isActive := true
		if p.Status != "" {
			isActive = strings.EqualFold(p.Status, "AKTIF")
		}
		products = append(products, ProviderProduct{
			SKUCode:     p.Code,
			ProductName: p.Name,
			Category:    p.Category,
			Price:       price,
			IsActive:    isActive,
		})
	}

	return products, nil
}

// IsHealthy returns whether the provider is healthy
func (c *KiosbankProviderClient) IsHealthy() bool {
	c.healthMu.RLock()
	defer c.healthMu.RUnlock()
	return c.healthy
}

// markHealthy marks the provider as healthy
func (c *KiosbankProviderClient) markHealthy() {
	c.healthMu.Lock()
	defer c.healthMu.Unlock()
	c.healthy = true
}

// markUnhealthy marks the provider as unhealthy
func (c *KiosbankProviderClient) markUnhealthy() {
	c.healthMu.Lock()
	defer c.healthMu.Unlock()
	c.healthy = false
}

func (c *KiosbankProviderClient) convertInquiryResponse(resp *kiosbank.InquiryResponse, refID string, responseTime time.Duration) *ProviderResponse {
	rawResp := kiosbankRawEnvelope(resp)
	parsed := parseKiosbankData(resp.Data)
	class := kiosbank.ClassifyRC(resp.RC, kiosbank.ResponsePhaseInquiry)

	return &ProviderResponse{
		Success:        class == kiosbank.ResponseClassSuccess,
		Pending:        class == kiosbank.ResponseClassPending,
		RefID:          refID,
		ProviderRefID:  refID,
		HTTPStatus:     http.StatusOK,
		Phase:          string(kiosbank.ResponsePhaseInquiry),
		Status:         kiosbank.StatusFromClass(class),
		RC:             resp.RC,
		Message:        kiosbankMessage(resp.RC, resp.Description),
		ProviderStatus: parsed.StatusText,
		CustomerName:   parsed.CustomerName,
		Amount:         parsed.Amount,
		Admin:          parsed.Admin,
		Description:    compactRawJSON(resp.Data),
		RawResponse:    rawResp,
		NeedsRetry:     kiosbank.NeedsNewRefID(resp.RC),
		ResponseTime:   responseTime,
	}
}

func (c *KiosbankProviderClient) convertPaymentResponse(resp *kiosbank.PaymentResponse, refID string, requestedAmount, requestedAdmin int, responseTime time.Duration) *ProviderResponse {
	rawResp := kiosbankRawEnvelope(resp)
	parsed := parseKiosbankData(resp.Data)
	applyRequestedKiosbankAmounts(&parsed, requestedAmount, requestedAdmin)
	class := kiosbank.ClassifyRC(resp.RC, kiosbank.ResponsePhaseInitialPayment)

	return &ProviderResponse{
		Success:        class == kiosbank.ResponseClassSuccess,
		Pending:        class == kiosbank.ResponseClassPending,
		RefID:          refID,
		ProviderRefID:  refID,
		HTTPStatus:     http.StatusOK,
		Phase:          string(kiosbank.ResponsePhaseInitialPayment),
		Status:         kiosbank.StatusFromClass(class),
		RC:             resp.RC,
		Message:        kiosbankMessage(resp.RC, resp.Description),
		ProviderStatus: parsed.StatusText,
		SerialNumber:   parsed.SerialNumber,
		CustomerName:   parsed.CustomerName,
		Amount:         parsed.Amount,
		Admin:          parsed.Admin,
		Description:    compactRawJSON(resp.Data),
		RawResponse:    rawResp,
		NeedsRetry:     kiosbank.NeedsNewRefID(resp.RC),
		ResponseTime:   responseTime,
	}
}

func (c *KiosbankProviderClient) convertSinglePaymentResponse(resp *kiosbank.SinglePaymentResponse, refID string, requestedAmount, requestedAdmin int, responseTime time.Duration) *ProviderResponse {
	rawResp := kiosbankRawEnvelope(resp)
	parsed := parseKiosbankData(resp.Data)
	applyRequestedKiosbankAmounts(&parsed, requestedAmount, requestedAdmin)
	if parsed.SerialNumber == "" {
		parsed.SerialNumber = parsed.ReferenceNo
	}
	class := kiosbank.ClassifyRC(resp.RC, kiosbank.ResponsePhaseInitialPayment)

	return &ProviderResponse{
		Success:        class == kiosbank.ResponseClassSuccess,
		Pending:        class == kiosbank.ResponseClassPending,
		RefID:          refID,
		ProviderRefID:  refID,
		HTTPStatus:     http.StatusOK,
		Phase:          string(kiosbank.ResponsePhaseInitialPayment),
		Status:         kiosbank.StatusFromClass(class),
		RC:             resp.RC,
		Message:        kiosbankMessage(resp.RC, resp.Description),
		ProviderStatus: parsed.StatusText,
		SerialNumber:   parsed.SerialNumber,
		CustomerName:   parsed.CustomerName,
		Amount:         parsed.Amount,
		Admin:          parsed.Admin,
		Description:    compactRawJSON(resp.Data),
		RawResponse:    rawResp,
		NeedsRetry:     kiosbank.NeedsNewRefID(resp.RC),
		ResponseTime:   responseTime,
	}
}

func (c *KiosbankProviderClient) convertAsyncPaymentResponse(resp *kiosbank.PaymentResponse, refID string, requestedAmount, requestedAdmin int, responseTime time.Duration) *ProviderResponse {
	rawResp := kiosbankRawEnvelope(resp)
	parsed := parseKiosbankData(resp.Data)
	applyRequestedKiosbankAmounts(&parsed, requestedAmount, requestedAdmin)
	class := kiosbank.ClassifyRC(resp.RC, kiosbank.ResponsePhaseAsync)

	return &ProviderResponse{
		Success:        class == kiosbank.ResponseClassSuccess,
		Pending:        class == kiosbank.ResponseClassPending,
		RefID:          refID,
		ProviderRefID:  refID,
		HTTPStatus:     http.StatusOK,
		Phase:          string(kiosbank.ResponsePhaseAsync),
		Status:         kiosbank.StatusFromClass(class),
		RC:             resp.RC,
		Message:        kiosbankMessage(resp.RC, resp.Description),
		ProviderStatus: parsed.StatusText,
		SerialNumber:   parsed.SerialNumber,
		CustomerName:   parsed.CustomerName,
		Amount:         parsed.Amount,
		Admin:          parsed.Admin,
		Description:    compactRawJSON(resp.Data),
		RawResponse:    rawResp,
		NeedsRetry:     kiosbank.NeedsNewRefID(resp.RC),
		ResponseTime:   responseTime,
	}
}

type kiosbankParsedData struct {
	CustomerName string
	Amount       int
	Admin        int
	Total        int
	StatusText   string
	SerialNumber string
	ReferenceNo  string
	Data         map[string]any
}

type kiosbankCheckStatusInput struct {
	ReferenceID string
	Tagihan     int
	Admin       int
	Total       int
	NoHandphone string
	Nama        string
	Kode        string
}

func applyRequestedKiosbankAmounts(parsed *kiosbankParsedData, requestedAmount, requestedAdmin int) {
	if parsed == nil {
		return
	}
	if parsed.Admin == 0 {
		parsed.Admin = requestedAdmin
	}
	if requestedAmount > 0 && (parsed.Amount == 0 || (parsed.Total > 0 && parsed.Total == parsed.Admin && parsed.Amount == parsed.Admin)) {
		parsed.Amount = requestedAmount
	}
	if parsed.Total == 0 && parsed.Amount > 0 {
		parsed.Total = parsed.Amount + parsed.Admin
	}
}

func resolveKiosbankReferenceID(refID string) string {
	if kiosbank.IsNumericReferenceID(refID) {
		return refID
	}
	return kiosbank.GenerateReferenceID()
}

func parseKiosbankData(raw json.RawMessage) kiosbankParsedData {
	data := rawJSONObject(raw)

	admin := firstPositiveAmount(data, "admin", "adminBank", "biayaAdmin", "AB")
	total := firstPositiveAmount(data, "total", "totalBayar", "TT", "totalTagihan")
	amount := firstPositiveAmount(data, "tagihan", "TG", "jumlahPembelian", "harga", "nilaiBeliGas")
	if amount == 0 {
		amount = sumArrayAmounts(data, []string{"detail", "rincian", "rincianTagihan"}, []string{"premi", "tagihan", "jumlahPembayaran", "harga"})
	}
	if multiTagihan := sumMatchingAmounts(data, "tagihan"); multiTagihan > 0 {
		amount = multiTagihan
	}
	if amount == 0 && total > 0 {
		if admin > 0 && total > admin {
			amount = total - admin
		} else {
			amount = total
		}
	}

	referenceNo := stringFromMapKeys(data, "noReferensi", "RF")
	serialNumber := extractKiosbankReceiptSN(data)
	if serialNumber == "" {
		serialNumber = referenceNo
	}

	return kiosbankParsedData{
		CustomerName: stringFromMapKeys(data, "nama", "NM"),
		Amount:       amount,
		Admin:        admin,
		Total:        total,
		StatusText:   stringFromMapKeys(data, "status", "STATUS"),
		SerialNumber: serialNumber,
		ReferenceNo:  referenceNo,
		Data:         data,
	}
}

func buildKiosbankCheckStatusInput(trx *models.Transaction, logs []models.TransactionLog) kiosbankCheckStatusInput {
	input := kiosbankCheckStatusInput{
		ReferenceID: trx.TransactionID,
	}
	if trx.ProviderRefID != nil && *trx.ProviderRefID != "" {
		input.ReferenceID = *trx.ProviderRefID
	}
	if trx.Amount != nil {
		input.Tagihan = *trx.Amount
	}
	input.Admin = trx.Admin

	for i := len(logs) - 1; i >= 0; i-- {
		reqMap := rawJSONObject(logs[i].Request)
		if provider := stringFromMapKeys(reqMap, "provider"); provider != "" && provider != string(models.ProviderKiosbank) {
			continue
		}
		if wireRequest := rawJSONObjectFromAny(reqMap["wire_request"]); len(wireRequest) > 0 {
			if refID := stringFromMapKeys(wireRequest, "referenceID"); refID != "" {
				input.ReferenceID = refID
			}
			if amount, ok := intValueOK(wireRequest["tagihan"]); ok {
				input.Tagihan = amount
			}
			if admin, ok := intValueOK(wireRequest["admin"]); ok {
				input.Admin = admin
			}
			if total, ok := intValueOK(wireRequest["total"]); ok {
				input.Total = total
			}
			input.NoHandphone = stringFromKeys(wireRequest, "noHandphone", "noHanphone")
			input.Nama = stringFromKeys(wireRequest, "nama")
			input.Kode = stringFromKeys(wireRequest, "kode")
			break
		}
		if refID := stringFromMapKeys(reqMap, "ref_id"); refID != "" {
			input.ReferenceID = refID
		}
		if amount, ok := intValueOK(reqMap["amount"]); ok {
			input.Tagihan = amount
		}
		if admin, ok := intValueOK(reqMap["admin"]); ok {
			input.Admin = admin
		}
		if extra, ok := reqMap["extra"].(map[string]any); ok {
			extra = normalizeKiosbankRequestData(extra)
			input.NoHandphone = stringFromKeys(extra, "noHandphone", "noHanphone")
			input.Nama = stringFromKeys(extra, "nama")
			input.Kode = stringFromKeys(extra, "kode")
		}
		break
	}

	input.Total = input.Tagihan + input.Admin
	return input
}

func kiosbankValidationResponse(refID, message string) *ProviderResponse {
	refID = resolveKiosbankReferenceID(refID)
	return &ProviderResponse{
		RefID:         refID,
		ProviderRefID: refID,
		HTTPStatus:    http.StatusBadRequest,
		Phase:         string(kiosbank.ResponsePhaseInitialPayment),
		Status:        "Failed",
		RC:            kiosbank.RCFormatError,
		Message:       message,
	}
}

func kiosbankRawEnvelope(v any) json.RawMessage {
	raw, _ := json.Marshal(v)
	return raw
}

func compactRawJSON(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	cp := make([]byte, len(raw))
	copy(cp, raw)
	return cp
}

func rawJSONObject(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return map[string]any{}
	}
	return out
}

func rawJSONObjectFromAny(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	switch raw := v.(type) {
	case json.RawMessage:
		return rawJSONObject(raw)
	case []byte:
		return rawJSONObject(raw)
	case string:
		return rawJSONObject(json.RawMessage(raw))
	}
	return map[string]any{}
}

func cloneAnyMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return map[string]any{}
	}
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func mergeAnyMap(dst map[string]any, src map[string]any) {
	if dst == nil || len(src) == 0 {
		return
	}
	for k, v := range src {
		dst[k] = v
	}
}

func normalizeKiosbankRequestData(data map[string]any) map[string]any {
	normalized := cloneAnyMap(data)
	if normalized == nil {
		normalized = make(map[string]any)
	}

	if v := stringFromKeys(normalized, "periode"); v != "" {
		normalized["periode"] = v
	}
	if v := stringFromKeys(normalized, "noHandphone", "noHanphone"); v != "" {
		normalized["noHandphone"] = v
		normalized["noHanphone"] = v
	}
	if v := stringFromKeys(normalized, "nama"); v != "" {
		normalized["nama"] = v
	}
	if v := stringFromKeys(normalized, "kode"); v != "" {
		normalized["kode"] = v
	}

	return normalized
}

func kiosbankRequiresPeriode(productID string) bool {
	return productID == "900001"
}

func kiosbankRequiresNoHandphone(productID string) bool {
	return productID == "900001"
}

func kiosbankRequiresPackageFields(productID string) bool {
	return productID == "550031"
}

func kiosbankMessage(rc, description string) string {
	if description != "" {
		return description
	}
	return kiosbank.GetRCDescription(rc)
}

func stringFromKeys(data map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := stringValue(data[key]); value != "" {
			return value
		}
	}
	return ""
}

func stringFromMapKeys(data map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := stringValue(data[key]); value != "" {
			return value
		}
	}
	return ""
}

func stringValue(v any) string {
	switch value := v.(type) {
	case string:
		return strings.TrimSpace(value)
	case fmt.Stringer:
		return strings.TrimSpace(value.String())
	case float64:
		if value == float64(int64(value)) {
			return strconv.FormatInt(int64(value), 10)
		}
	case int:
		return strconv.Itoa(value)
	case int64:
		return strconv.FormatInt(value, 10)
	case json.Number:
		return value.String()
	}
	return ""
}

func firstPositiveAmount(data map[string]any, keys ...string) int {
	for _, key := range keys {
		if amount := parseAmountAny(data[key]); amount > 0 {
			return amount
		}
	}
	return 0
}

func sumMatchingAmounts(data map[string]any, prefix string) int {
	total := 0
	found := false
	for key, value := range data {
		lower := strings.ToLower(key)
		if lower == prefix || !strings.HasPrefix(lower, prefix) {
			continue
		}
		if amount := parseAmountAny(value); amount > 0 {
			total += amount
			found = true
		}
	}
	if !found {
		return 0
	}
	return total
}

func sumArrayAmounts(data map[string]any, arrayKeys []string, fieldKeys []string) int {
	for _, arrayKey := range arrayKeys {
		items, ok := data[arrayKey].([]any)
		if !ok {
			continue
		}
		total := 0
		found := false
		for _, item := range items {
			itemMap, ok := item.(map[string]any)
			if !ok {
				continue
			}
			for _, fieldKey := range fieldKeys {
				if amount := parseAmountAny(itemMap[fieldKey]); amount > 0 {
					total += amount
					found = true
					break
				}
			}
		}
		if found {
			return total
		}
	}
	return 0
}

func parseAmountAny(v any) int {
	switch value := v.(type) {
	case string:
		return parseAmount(value)
	case float64:
		return int(value)
	case int:
		return value
	case int64:
		return int(value)
	case json.Number:
		if i, err := value.Int64(); err == nil {
			return int(i)
		}
	}
	return 0
}

func intValueOK(v any) (int, bool) {
	switch value := v.(type) {
	case int:
		return value, true
	case int64:
		return int(value), true
	case float64:
		return int(value), true
	case json.Number:
		if i, err := value.Int64(); err == nil {
			return int(i), true
		}
	case string:
		if value == "" {
			return 0, false
		}
		return parseAmount(value), true
	}
	return 0, false
}

// extractKiosbankReceiptSN extracts serial number from Kiosbank payment data.
func extractKiosbankReceiptSN(data map[string]any) string {
	if tk := stringFromMapKeys(data, "TK", "token"); tk != "" {
		return tk
	}
	if sn := stringFromMapKeys(data, "sn", "kodeVoucher"); sn != "" {
		return sn
	}
	if nr := stringFromMapKeys(data, "noReferensi", "RF"); nr != "" {
		return nr
	}
	return ""
}

// parseAmount converts Kiosbank amount strings to int.
func parseAmount(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}

	s = strings.TrimPrefix(s, "Rp")
	s = strings.TrimSpace(s)
	if idx := strings.Index(s, ","); idx >= 0 {
		s = s[:idx]
	}
	s = strings.ReplaceAll(s, ".", "")
	s = strings.ReplaceAll(s, " ", "")
	if s == "" {
		return 0
	}

	v, err := strconv.Atoi(s)
	if err != nil {
		log.Warn().Str("input", s).Err(err).Msg("parseAmount: failed to parse amount string")
		return 0
	}
	return v
}

// getStatusFromRC converts RC to status string
func pendingTransportResponse(refID string, responseTime time.Duration, err error) *ProviderResponse {
	raw := map[string]any{
		"transport_error": err.Error(),
		"phase":           string(kiosbank.ResponsePhaseInitialPayment),
	}
	rawResp, _ := json.Marshal(raw)
	return &ProviderResponse{
		Pending:       true,
		RefID:         refID,
		ProviderRefID: refID,
		Phase:         string(kiosbank.ResponsePhaseInitialPayment),
		Status:        kiosbank.StatusFromClass(kiosbank.ResponseClassPending),
		Message:       "Transaction is being processed",
		RawResponse:   rawResp,
		ResponseTime:  responseTime,
	}
}
