package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/GTDGit/gtd_gateway/internal/service"
	"github.com/GTDGit/gtd_gateway/internal/utils"
)

// QRISHandler exposes admin CRUD over static-QRIS merchants and read-only
// listing of successful QRIS payments. Pakailink register/generate is delegated
// to the api service through the QRISService proxy.
type QRISHandler struct {
	qrisSvc *service.QRISService
}

func NewQRISHandler(qrisSvc *service.QRISService) *QRISHandler {
	return &QRISHandler{qrisSvc: qrisSvc}
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

// RequestPakailinkQR drives the api-side generate flow and persists the result.
func (h *QRISHandler) RequestPakailinkQR(c *gin.Context) {
	id, ok := intParam(c, "id")
	if !ok {
		return
	}
	m, err := h.qrisSvc.RequestPakailinkQR(c.Request.Context(), id)
	if err != nil {
		h.handleError(c, err)
		return
	}
	utils.Success(c, http.StatusOK, "QRIS generated", m)
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
