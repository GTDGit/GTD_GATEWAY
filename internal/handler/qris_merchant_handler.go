package handler

import (
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/GTDGit/gtd_gateway/internal/service"
	"github.com/GTDGit/gtd_gateway/internal/utils"
)

// QRISHandler exposes admin CRUD over static-QRIS merchants and read-only
// listing of successful QRIS payments. Nobu onboarding (registrations, Excel
// batches, activation) is delegated to the api service through APIAdminProxy.
type QRISHandler struct {
	qrisSvc *service.QRISService
	proxy   *service.APIAdminProxy
}

func NewQRISHandler(qrisSvc *service.QRISService, proxy *service.APIAdminProxy) *QRISHandler {
	return &QRISHandler{qrisSvc: qrisSvc, proxy: proxy}
}

// ---------------------------------------------------------------------------
// Merchants
// ---------------------------------------------------------------------------

func (h *QRISHandler) ListMerchants(c *gin.Context) {
	req := service.QRISMerchantListRequest{
		Provider: c.Query("provider"),
		Status:   c.Query("status"),
		Search:   c.Query("search"),
	}
	if v := c.Query("clientId"); v != "" {
		if id, err := strconv.Atoi(v); err == nil {
			req.ClientID = &id
		}
	}
	if v := c.Query("page"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			req.Page = p
		}
	}
	if v := c.Query("limit"); v != "" {
		if l, err := strconv.Atoi(v); err == nil {
			req.Limit = l
		}
	}
	resp, err := h.qrisSvc.ListMerchants(c.Request.Context(), req)
	if err != nil {
		h.handleError(c, err)
		return
	}
	utils.Success(c, http.StatusOK, "Merchants retrieved", resp)
}

func (h *QRISHandler) GetMerchant(c *gin.Context) {
	id, ok := intParam(c, "id")
	if !ok {
		return
	}
	m, err := h.qrisSvc.GetMerchant(c.Request.Context(), id)
	if err != nil {
		h.handleError(c, err)
		return
	}
	utils.Success(c, http.StatusOK, "Merchant retrieved", m)
}

func (h *QRISHandler) CreateMerchant(c *gin.Context) {
	var req service.QRISMerchantUpsertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	m, err := h.qrisSvc.CreateMerchant(c.Request.Context(), req)
	if err != nil {
		h.handleError(c, err)
		return
	}
	utils.Success(c, http.StatusCreated, "Merchant created", m)
}

func (h *QRISHandler) UpdateMerchant(c *gin.Context) {
	id, ok := intParam(c, "id")
	if !ok {
		return
	}
	var req service.QRISMerchantUpsertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	m, err := h.qrisSvc.UpdateMerchant(c.Request.Context(), id, req)
	if err != nil {
		h.handleError(c, err)
		return
	}
	utils.Success(c, http.StatusOK, "Merchant updated", m)
}

// ---------------------------------------------------------------------------
// Nobu onboarding — proxied to the api service (admin JWT passed through)
// ---------------------------------------------------------------------------

// ListRegistrations → GET api /v1/admin/qris/registrations
func (h *QRISHandler) ListRegistrations(c *gin.Context) {
	h.forward(c, http.MethodGet, "/v1/admin/qris/registrations?"+c.Request.URL.RawQuery, nil)
}

// ActivateRegistration → POST api /v1/admin/qris/registrations/:id/activate
func (h *QRISHandler) ActivateRegistration(c *gin.Context) {
	id, ok := intParam(c, "id")
	if !ok {
		return
	}
	body, _ := io.ReadAll(c.Request.Body)
	h.forward(c, http.MethodPost, "/v1/admin/qris/registrations/"+strconv.Itoa(id)+"/activate", body)
}

// RejectRegistration → POST api /v1/admin/qris/registrations/:id/reject
func (h *QRISHandler) RejectRegistration(c *gin.Context) {
	id, ok := intParam(c, "id")
	if !ok {
		return
	}
	body, _ := io.ReadAll(c.Request.Body)
	h.forward(c, http.MethodPost, "/v1/admin/qris/registrations/"+strconv.Itoa(id)+"/reject", body)
}

// ListBatches → GET api /v1/admin/qris/batches
func (h *QRISHandler) ListBatches(c *gin.Context) {
	h.forward(c, http.MethodGet, "/v1/admin/qris/batches?"+c.Request.URL.RawQuery, nil)
}

// DownloadBatch → GET api /v1/admin/qris/batches/:id/download (binary passthrough)
func (h *QRISHandler) DownloadBatch(c *gin.Context) {
	id, ok := intParam(c, "id")
	if !ok {
		return
	}
	h.forward(c, http.MethodGet, "/v1/admin/qris/batches/"+strconv.Itoa(id)+"/download", nil)
}

// MarkBatchSent → POST api /v1/admin/qris/batches/:id/sent
func (h *QRISHandler) MarkBatchSent(c *gin.Context) {
	id, ok := intParam(c, "id")
	if !ok {
		return
	}
	h.forward(c, http.MethodPost, "/v1/admin/qris/batches/"+strconv.Itoa(id)+"/sent", nil)
}

// forward proxies the request to the api admin endpoint, passing the caller's
// Authorization header through (api + gateway share JWT_SECRET) and relaying the
// upstream status, content-type, and body verbatim.
func (h *QRISHandler) forward(c *gin.Context, method, path string, body []byte) {
	if h.proxy == nil || !h.proxy.Enabled() {
		utils.Error(c, http.StatusServiceUnavailable, "PROXY_UNAVAILABLE", "api admin proxy is not configured")
		return
	}
	resp, err := h.proxy.Forward(c.Request.Context(), method, path, c.GetHeader("Authorization"), body)
	if err != nil {
		h.handleError(c, err)
		return
	}
	contentType := resp.ContentType
	if contentType == "" {
		contentType = "application/json"
	}
	if resp.ContentDisposition != "" {
		c.Header("Content-Disposition", resp.ContentDisposition)
	}
	c.Data(resp.StatusCode, contentType, resp.Body)
}

// ---------------------------------------------------------------------------
// Payments
// ---------------------------------------------------------------------------

func (h *QRISHandler) ListPayments(c *gin.Context) {
	req := service.QRISPaymentListRequest{
		Provider:  c.Query("provider"),
		StoreID:   c.Query("storeId"),
		StartDate: c.Query("startDate"),
		EndDate:   c.Query("endDate"),
		Search:    c.Query("search"),
	}
	if v := c.Query("qrisMerchantId"); v != "" {
		if id, err := strconv.Atoi(v); err == nil {
			req.QRISMerchantID = &id
		}
	}
	if v := c.Query("page"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			req.Page = p
		}
	}
	if v := c.Query("limit"); v != "" {
		if l, err := strconv.Atoi(v); err == nil {
			req.Limit = l
		}
	}
	resp, err := h.qrisSvc.ListPayments(c.Request.Context(), req)
	if err != nil {
		h.handleError(c, err)
		return
	}
	utils.Success(c, http.StatusOK, "Payments retrieved", resp)
}

func (h *QRISHandler) handleError(c *gin.Context, err error) {
	var pe *service.PaymentServiceError
	if errors.As(err, &pe) {
		utils.Error(c, pe.HTTPStatus, pe.Code, pe.Message)
		return
	}
	utils.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error")
}
