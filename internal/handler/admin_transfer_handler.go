package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/GTDGit/gtd_gateway/internal/service"
	"github.com/GTDGit/gtd_gateway/internal/utils"
)

type AdminTransferHandler struct {
	svc *service.AdminTransferService
}

func NewAdminTransferHandler(svc *service.AdminTransferService) *AdminTransferHandler {
	return &AdminTransferHandler{svc: svc}
}

func (h *AdminTransferHandler) ListTransfers(c *gin.Context) {
	req := bindAdminTransfersRequest(c)
	resp, err := h.svc.ListTransfers(c.Request.Context(), req)
	if err != nil {
		h.handleError(c, err)
		return
	}
	utils.Success(c, http.StatusOK, "Transfers retrieved", resp)
}

func (h *AdminTransferHandler) Stats(c *gin.Context) {
	req := bindAdminTransfersRequest(c)
	resp, err := h.svc.Stats(c.Request.Context(), req)
	if err != nil {
		h.handleError(c, err)
		return
	}
	utils.Success(c, http.StatusOK, "Transfer stats retrieved", resp)
}

func (h *AdminTransferHandler) GetTransfer(c *gin.Context) {
	id, ok := intParam(c, "id")
	if !ok {
		return
	}
	resp, err := h.svc.GetTransfer(c.Request.Context(), id)
	if err != nil {
		h.handleError(c, err)
		return
	}
	utils.Success(c, http.StatusOK, "Transfer retrieved", resp)
}

func (h *AdminTransferHandler) ListCallbacks(c *gin.Context) {
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

func bindAdminTransfersRequest(c *gin.Context) service.AdminListTransfersRequest {
	req := service.AdminListTransfersRequest{}
	if v := c.Query("status"); v != "" {
		req.Status = &v
	}
	if v := c.Query("type"); v != "" {
		req.Type = &v
	}
	if v := c.Query("provider"); v != "" {
		req.Provider = &v
	}
	if v := c.Query("bankCode"); v != "" {
		req.BankCode = &v
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

func (h *AdminTransferHandler) handleError(c *gin.Context, err error) {
	var pe *service.PaymentServiceError
	if errors.As(err, &pe) {
		utils.Error(c, pe.HTTPStatus, pe.Code, pe.Message)
		return
	}
	utils.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error")
}
