package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/GTDGit/gtd_gateway/internal/models"
	"github.com/GTDGit/gtd_gateway/internal/repository"
	"github.com/GTDGit/gtd_gateway/internal/utils"
)

// QRISService implements admin CRUD over static-QRIS merchants and read-only
// listing of successful QRIS payments. Pakailink register/generate is delegated
// to the api service via PakailinkProxy (the gateway holds no provider client).
type QRISService struct {
	merchantRepo *repository.QRISMerchantRepository
	paymentRepo  *repository.QRISPaymentRepository
	proxy        *PakailinkProxy
}

func NewQRISService(
	merchantRepo *repository.QRISMerchantRepository,
	paymentRepo *repository.QRISPaymentRepository,
	proxy *PakailinkProxy,
) *QRISService {
	return &QRISService{merchantRepo: merchantRepo, paymentRepo: paymentRepo, proxy: proxy}
}

// ---------------------------------------------------------------------------
// DTOs
// ---------------------------------------------------------------------------

type QRISMerchantListRequest struct {
	Provider string
	ClientID *int
	Status   string
	Search   string
	Page     int
	Limit    int
}

type QRISMerchantUpsertRequest struct {
	ClientID   *int   `json:"clientId"`
	Provider   string `json:"provider"`
	StoreID    string `json:"storeId"`
	TerminalID string `json:"terminalId"`
	QRISString string `json:"qrisString"`
	Status     string `json:"status"`
	// Optional manual overrides (otherwise parsed from qrisString).
	MerchantName string `json:"merchantName"`
	MerchantCity string `json:"merchantCity"`
}

type QRISPaymentListRequest struct {
	Provider       string
	QRISMerchantID *int
	StoreID        string
	StartDate      string
	EndDate        string
	Search         string
	Page           int
	Limit          int
}

type QRISMerchantListResponse struct {
	Items      []models.QRISMerchant `json:"items"`
	Pagination PaginationMeta        `json:"pagination"`
}

type QRISPaymentListResponse struct {
	Items      []models.QRISPayment `json:"items"`
	Pagination PaginationMeta       `json:"pagination"`
}

// ---------------------------------------------------------------------------
// Merchants
// ---------------------------------------------------------------------------

func (s *QRISService) ListMerchants(ctx context.Context, req QRISMerchantListRequest) (*QRISMerchantListResponse, error) {
	page, limit := normalizePage(req.Page, req.Limit)
	filter := repository.QRISMerchantFilter{
		Provider: strings.TrimSpace(req.Provider),
		Status:   strings.TrimSpace(req.Status),
		Search:   strings.TrimSpace(req.Search),
	}
	if req.ClientID != nil {
		filter.ClientID = *req.ClientID
	}
	rows, total, err := s.merchantRepo.List(ctx, filter, limit, (page-1)*limit)
	if err != nil {
		return nil, newPaymentError(http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list merchants", err)
	}
	return &QRISMerchantListResponse{
		Items:      rows,
		Pagination: makePagination(page, limit, total),
	}, nil
}

func (s *QRISService) GetMerchant(ctx context.Context, id int) (*models.QRISMerchant, error) {
	m, err := s.merchantRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, newPaymentError(http.StatusNotFound, "NOT_FOUND", "merchant not found", nil)
		}
		return nil, newPaymentError(http.StatusInternalServerError, "INTERNAL_ERROR", "failed to load merchant", err)
	}
	return m, nil
}

func (s *QRISService) CreateMerchant(ctx context.Context, req QRISMerchantUpsertRequest) (*models.QRISMerchant, error) {
	provider := strings.TrimSpace(req.Provider)
	if provider != string(models.QRISProviderPakailink) && provider != string(models.QRISProviderNobu) {
		return nil, newPaymentError(http.StatusBadRequest, "INVALID_PROVIDER", "provider must be pakailink or nobu", nil)
	}
	storeID := strings.TrimSpace(req.StoreID)
	if storeID == "" {
		return nil, newPaymentError(http.StatusBadRequest, "MISSING_FIELD", "storeId is required", nil)
	}
	status := strings.TrimSpace(req.Status)
	if status == "" {
		status = "active"
	}

	m := &models.QRISMerchant{
		ClientID:   req.ClientID,
		Provider:   models.QRISProvider(provider),
		StoreID:    storeID,
		TerminalID: ptrIfNotBlank(req.TerminalID),
		QRISString: ptrIfNotBlank(req.QRISString),
		Status:     status,
	}
	applyParsedQR(m, req)

	if err := s.merchantRepo.Create(ctx, m); err != nil {
		if isUniqueViolation(err) {
			return nil, newPaymentError(http.StatusConflict, "DUPLICATE", "a merchant with this provider + storeId already exists", err)
		}
		return nil, newPaymentError(http.StatusInternalServerError, "INTERNAL_ERROR", "failed to create merchant", err)
	}
	return m, nil
}

func (s *QRISService) UpdateMerchant(ctx context.Context, id int, req QRISMerchantUpsertRequest) (*models.QRISMerchant, error) {
	m, err := s.GetMerchant(ctx, id)
	if err != nil {
		return nil, err
	}
	if v := strings.TrimSpace(req.StoreID); v != "" {
		m.StoreID = v
	}
	if req.ClientID != nil {
		m.ClientID = req.ClientID
	}
	m.TerminalID = ptrIfNotBlank(req.TerminalID)
	if v := strings.TrimSpace(req.Status); v != "" {
		m.Status = v
	}
	if v := strings.TrimSpace(req.QRISString); v != "" {
		m.QRISString = &v
	}
	applyParsedQR(m, req)

	if err := s.merchantRepo.Update(ctx, m); err != nil {
		if isUniqueViolation(err) {
			return nil, newPaymentError(http.StatusConflict, "DUPLICATE", "a merchant with this provider + storeId already exists", err)
		}
		return nil, newPaymentError(http.StatusInternalServerError, "INTERNAL_ERROR", "failed to update merchant", err)
	}
	return m, nil
}

// RequestPakailinkQR drives the api-side register+generate flow and persists the
// returned QR string + parsed fields onto the merchant.
func (s *QRISService) RequestPakailinkQR(ctx context.Context, id int) (*models.QRISMerchant, error) {
	m, err := s.GetMerchant(ctx, id)
	if err != nil {
		return nil, err
	}
	if m.Provider != models.QRISProviderPakailink {
		return nil, newPaymentError(http.StatusBadRequest, "INVALID_PROVIDER", "QR request is only available for Pakailink merchants", nil)
	}
	if s.proxy == nil || !s.proxy.Enabled() {
		return nil, newPaymentError(http.StatusServiceUnavailable, "PROXY_UNAVAILABLE", "api internal proxy is not configured", nil)
	}

	genResp, err := s.proxy.Generate(ctx, PakailinkGenerateRequest{
		StoreID:      m.StoreID,
		TerminalID:   strFromPtr(m.TerminalID),
		MerchantName: strFromPtr(m.MerchantName),
	})
	if err != nil {
		return nil, err
	}

	qr := strings.TrimSpace(genResp.QRContent)
	if qr == "" {
		return nil, newPaymentError(http.StatusBadGateway, "PROVIDER_ERROR", "Pakailink returned an empty QR string", nil)
	}
	m.QRISString = &qr
	if raw, mErr := json.Marshal(genResp); mErr == nil {
		m.RawProviderResponse = models.NullableRawMessage(raw)
	}
	if info, perr := utils.ParseQRIS(qr); perr == nil {
		applyQRISInfo(m, info)
	}

	if err := s.merchantRepo.Update(ctx, m); err != nil {
		return nil, newPaymentError(http.StatusInternalServerError, "INTERNAL_ERROR", "failed to persist generated QR", err)
	}
	return m, nil
}

// ---------------------------------------------------------------------------
// Payments
// ---------------------------------------------------------------------------

func (s *QRISService) ListPayments(ctx context.Context, req QRISPaymentListRequest) (*QRISPaymentListResponse, error) {
	page, limit := normalizePage(req.Page, req.Limit)
	filter := repository.QRISPaymentFilter{
		Provider: strings.TrimSpace(req.Provider),
		StoreID:  strings.TrimSpace(req.StoreID),
		Search:   strings.TrimSpace(req.Search),
	}
	if req.QRISMerchantID != nil {
		filter.QRISMerchantID = *req.QRISMerchantID
	}
	if t := parseFilterDate(req.StartDate); t != nil {
		filter.CreatedFrom = t
	}
	if t := parseFilterDate(req.EndDate); t != nil {
		filter.CreatedTo = t
	}
	rows, total, err := s.paymentRepo.List(ctx, filter, limit, (page-1)*limit)
	if err != nil {
		return nil, newPaymentError(http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list payments", err)
	}
	return &QRISPaymentListResponse{
		Items:      rows,
		Pagination: makePagination(page, limit, total),
	}, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func applyParsedQR(m *models.QRISMerchant, req QRISMerchantUpsertRequest) {
	// Manual overrides win when provided.
	if v := strings.TrimSpace(req.MerchantName); v != "" {
		m.MerchantName = &v
	}
	if v := strings.TrimSpace(req.MerchantCity); v != "" {
		m.MerchantCity = &v
	}
	qr := strings.TrimSpace(req.QRISString)
	if qr == "" {
		return
	}
	if info, err := utils.ParseQRIS(qr); err == nil {
		applyQRISInfo(m, info)
	}
}

func applyQRISInfo(m *models.QRISMerchant, info utils.QRISInfo) {
	if info.NMID != "" {
		m.NMID = &info.NMID
	}
	if info.TerminalID != "" {
		m.TerminalID = &info.TerminalID
	}
	if info.MerchantCategoryCode != "" {
		m.MerchantCategoryCode = &info.MerchantCategoryCode
	}
	if info.MerchantName != "" && (m.MerchantName == nil || *m.MerchantName == "") {
		name := info.MerchantName
		m.MerchantName = &name
	}
	if info.MerchantCity != "" && (m.MerchantCity == nil || *m.MerchantCity == "") {
		city := info.MerchantCity
		m.MerchantCity = &city
	}
}

func normalizePage(page, limit int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}
	return page, limit
}

func makePagination(page, limit, total int) PaginationMeta {
	totalPages := 0
	if limit > 0 {
		totalPages = (total + limit - 1) / limit
	}
	return PaginationMeta{Page: page, Limit: limit, TotalItems: total, TotalPages: totalPages}
}

func parseFilterDate(v string) *time.Time {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return &t
	}
	if t, err := time.Parse("2006-01-02", v); err == nil {
		return &t
	}
	return nil
}

func ptrIfNotBlank(v string) *string {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	return &v
}

func strFromPtr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
