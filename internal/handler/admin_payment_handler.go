package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/GTDGit/gtd_gateway/internal/service"
	"github.com/GTDGit/gtd_gateway/internal/utils"
)

type AdminPaymentHandler struct {
	adminPaymentSvc *service.AdminPaymentService
}

func NewAdminPaymentHandler(adminPaymentSvc *service.AdminPaymentService) *AdminPaymentHandler {
	return &AdminPaymentHandler{adminPaymentSvc: adminPaymentSvc}
}

// ---------------------------------------------------------------------------
// Payments
// ---------------------------------------------------------------------------

func (h *AdminPaymentHandler) ListPayments(c *gin.Context) {
	req := bindAdminPaymentsRequest(c)
	resp, err := h.adminPaymentSvc.ListPayments(c.Request.Context(), req)
	if err != nil {
		h.handleError(c, err)
		return
	}
	utils.Success(c, http.StatusOK, "Payments retrieved", resp)
}

func (h *AdminPaymentHandler) Stats(c *gin.Context) {
	req := bindAdminPaymentsRequest(c)
	resp, err := h.adminPaymentSvc.Stats(c.Request.Context(), req)
	if err != nil {
		h.handleError(c, err)
		return
	}
	utils.Success(c, http.StatusOK, "Payment stats retrieved", resp)
}

func (h *AdminPaymentHandler) GetPayment(c *gin.Context) {
	id, ok := intParam(c, "id")
	if !ok {
		return
	}
	resp, err := h.adminPaymentSvc.GetPayment(c.Request.Context(), id)
	if err != nil {
		h.handleError(c, err)
		return
	}
	utils.Success(c, http.StatusOK, "Payment retrieved", resp)
}

func (h *AdminPaymentHandler) GetPaymentLogs(c *gin.Context) {
	id, ok := intParam(c, "id")
	if !ok {
		return
	}
	logs, err := h.adminPaymentSvc.GetPaymentLogs(c.Request.Context(), id)
	if err != nil {
		h.handleError(c, err)
		return
	}
	utils.Success(c, http.StatusOK, "Payment logs retrieved", logs)
}

func (h *AdminPaymentHandler) GetPaymentCallbacks(c *gin.Context) {
	id, ok := intParam(c, "id")
	if !ok {
		return
	}
	cbs, err := h.adminPaymentSvc.GetPaymentCallbacks(c.Request.Context(), id)
	if err != nil {
		h.handleError(c, err)
		return
	}
	utils.Success(c, http.StatusOK, "Provider callbacks retrieved", cbs)
}

func (h *AdminPaymentHandler) ListRefunds(c *gin.Context) {
	id, ok := intParam(c, "id")
	if !ok {
		return
	}
	rows, err := h.adminPaymentSvc.ListRefunds(c.Request.Context(), id)
	if err != nil {
		h.handleError(c, err)
		return
	}
	utils.Success(c, http.StatusOK, "Refunds retrieved", rows)
}

func (h *AdminPaymentHandler) ListCallbackLogs(c *gin.Context) {
	id, ok := intParam(c, "id")
	if !ok {
		return
	}
	rows, err := h.adminPaymentSvc.ListCallbackLogs(c.Request.Context(), id)
	if err != nil {
		h.handleError(c, err)
		return
	}
	utils.Success(c, http.StatusOK, "Callback logs retrieved", rows)
}

func (h *AdminPaymentHandler) RetryCallback(c *gin.Context) {
	id, ok := intParam(c, "id")
	if !ok {
		return
	}
	var body struct {
		LogID int `json:"logId"`
	}
	_ = c.ShouldBindJSON(&body)
	if err := h.adminPaymentSvc.RetryCallback(c.Request.Context(), id, body.LogID); err != nil {
		h.handleError(c, err)
		return
	}
	utils.Success(c, http.StatusOK, "Callback retry queued", gin.H{"retried": true})
}

func (h *AdminPaymentHandler) Refund(c *gin.Context) {
	id, ok := intParam(c, "id")
	if !ok {
		return
	}
	var req service.AdminRefundRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, http.StatusBadRequest, "MISSING_FIELD", "Invalid request body")
		return
	}
	refund, err := h.adminPaymentSvc.AdminRefund(c.Request.Context(), id, req)
	if err != nil {
		h.handleError(c, err)
		return
	}
	utils.Success(c, http.StatusCreated, "Refund submitted", refund)
}

// ---------------------------------------------------------------------------
// Methods
// ---------------------------------------------------------------------------

func (h *AdminPaymentHandler) ListMethods(c *gin.Context) {
	rows, err := h.adminPaymentSvc.ListMethods(c.Request.Context())
	if err != nil {
		h.handleError(c, err)
		return
	}
	utils.Success(c, http.StatusOK, "Payment methods retrieved", rows)
}

func (h *AdminPaymentHandler) UpdateMethod(c *gin.Context) {
	id, ok := intParam(c, "id")
	if !ok {
		return
	}
	var req service.AdminUpdateMethodRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, http.StatusBadRequest, "MISSING_FIELD", "Invalid request body")
		return
	}
	m, err := h.adminPaymentSvc.UpdateMethod(c.Request.Context(), id, req)
	if err != nil {
		h.handleError(c, err)
		return
	}
	utils.Success(c, http.StatusOK, "Payment method updated", m)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func bindAdminPaymentsRequest(c *gin.Context) service.AdminListPaymentsRequest {
	req := service.AdminListPaymentsRequest{}
	if v := c.Query("status"); v != "" {
		req.Status = &v
	}
	if v := c.Query("type"); v != "" {
		req.Type = &v
	}
	if v := c.Query("provider"); v != "" {
		req.Provider = &v
	}
	if v := c.Query("clientId"); v != "" {
		if id, err := strconv.Atoi(v); err == nil {
			req.ClientID = &id
		}
	}
	if v := c.Query("paymentId"); v != "" {
		req.PaymentID = &v
	}
	if v := c.Query("referenceId"); v != "" {
		req.ReferenceID = &v
	}
	if v := c.Query("isSandbox"); v != "" {
		b := v == "true"
		req.IsSandbox = &b
	}
	if v := c.Query("startDate"); v != "" {
		req.StartDate = &v
	}
	if v := c.Query("endDate"); v != "" {
		req.EndDate = &v
	}
	if v := c.Query("search"); v != "" {
		req.Search = &v
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
	return req
}

func intParam(c *gin.Context, name string) (int, bool) {
	raw := c.Param(name)
	id, err := strconv.Atoi(raw)
	if err != nil || id <= 0 {
		utils.Error(c, http.StatusBadRequest, "INVALID_PARAM", name+" must be a positive integer")
		return 0, false
	}
	return id, true
}

func (h *AdminPaymentHandler) handleError(c *gin.Context, err error) {
	var pe *service.PaymentServiceError
	if errors.As(err, &pe) {
		utils.Error(c, pe.HTTPStatus, pe.Code, pe.Message)
		return
	}
	utils.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error")
}
