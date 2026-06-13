package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/GTDGit/gtd_gateway/internal/service"
	"github.com/GTDGit/gtd_gateway/internal/utils"
)

type AdminPayoutHandler struct {
	svc *service.AdminPayoutService
}

func NewAdminPayoutHandler(svc *service.AdminPayoutService) *AdminPayoutHandler {
	return &AdminPayoutHandler{svc: svc}
}

func (h *AdminPayoutHandler) ListPayouts(c *gin.Context) {
	req := bindAdminPayoutsRequest(c)
	resp, err := h.svc.ListPayouts(c.Request.Context(), req)
	if err != nil {
		h.handleError(c, err)
		return
	}
	utils.Success(c, http.StatusOK, "Payouts retrieved", resp)
}

func (h *AdminPayoutHandler) Stats(c *gin.Context) {
	req := bindAdminPayoutsRequest(c)
	resp, err := h.svc.Stats(c.Request.Context(), req)
	if err != nil {
		h.handleError(c, err)
		return
	}
	utils.Success(c, http.StatusOK, "Payout stats retrieved", resp)
}

func (h *AdminPayoutHandler) GetPayout(c *gin.Context) {
	id, ok := intParam(c, "id")
	if !ok {
		return
	}
	resp, err := h.svc.GetPayout(c.Request.Context(), id)
	if err != nil {
		h.handleError(c, err)
		return
	}
	utils.Success(c, http.StatusOK, "Payout retrieved", resp)
}

func (h *AdminPayoutHandler) ListCallbacks(c *gin.Context) {
	id, ok := intParam(c, "id")
	if !ok {
		return
	}
	rows, err := h.svc.ListCallbacks(c.Request.Context(), id)
	if err != nil {
		h.handleError(c, err)
		return
	}
	utils.Success(c, http.StatusOK, "Provider callbacks retrieved", rows)
}

func (h *AdminPayoutHandler) ListRoutes(c *gin.Context) {
	rows, err := h.svc.ListRoutes(c.Request.Context())
	if err != nil {
		h.handleError(c, err)
		return
	}
	utils.Success(c, http.StatusOK, "Payout routes retrieved", rows)
}

func (h *AdminPayoutHandler) UpdateRoute(c *gin.Context) {
	id, ok := intParam(c, "id")
	if !ok {
		return
	}
	var req service.AdminUpdatePayoutRouteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body")
		return
	}
	resp, err := h.svc.UpdateRoute(c.Request.Context(), id, req)
	if err != nil {
		h.handleError(c, err)
		return
	}
	utils.Success(c, http.StatusOK, "Payout route updated", resp)
}

func bindAdminPayoutsRequest(c *gin.Context) service.AdminListPayoutsRequest {
	req := service.AdminListPayoutsRequest{}
	if v := c.Query("status"); v != "" {
		req.Status = &v
	}
	if v := c.Query("methodType"); v != "" {
		req.MethodType = &v
	}
	if v := c.Query("provider"); v != "" {
		req.Provider = &v
	}
	if v := c.Query("channelCode"); v != "" {
		req.ChannelCode = &v
	}
	if v := c.Query("clientId"); v != "" {
		if id, err := strconv.Atoi(v); err == nil {
			req.ClientID = &id
		}
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

func (h *AdminPayoutHandler) handleError(c *gin.Context, err error) {
	var pe *service.PaymentServiceError
	if errors.As(err, &pe) {
		utils.Error(c, pe.HTTPStatus, pe.Code, pe.Message)
		return
	}
	utils.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error")
}
