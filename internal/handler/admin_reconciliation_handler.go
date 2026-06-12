package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/GTDGit/gtd_gateway/internal/service"
	"github.com/GTDGit/gtd_gateway/internal/utils"
)

type AdminReconciliationHandler struct {
	svc *service.AdminReconciliationService
}

func NewAdminReconciliationHandler(svc *service.AdminReconciliationService) *AdminReconciliationHandler {
	return &AdminReconciliationHandler{svc: svc}
}

func (h *AdminReconciliationHandler) ListReconciliations(c *gin.Context) {
	var req service.AdminListReconciliationsRequest
	_ = c.ShouldBindQuery(&req)
	resp, err := h.svc.List(c.Request.Context(), req)
	if err != nil {
		h.handleError(c, err)
		return
	}
	utils.Success(c, http.StatusOK, "Reconciliations retrieved", resp)
}

func (h *AdminReconciliationHandler) GetReconciliation(c *gin.Context) {
	id, ok := int64Param(c, "id")
	if !ok {
		return
	}
	resp, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		h.handleError(c, err)
		return
	}
	utils.Success(c, http.StatusOK, "Reconciliation retrieved", resp)
}

func (h *AdminReconciliationHandler) ResolveReconciliation(c *gin.Context) {
	id, ok := int64Param(c, "id")
	if !ok {
		return
	}
	var body struct {
		Status string `json:"status"`
		Note   string `json:"note"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		utils.Error(c, http.StatusBadRequest, "INVALID_BODY", "Invalid request body")
		return
	}
	if body.Status == "" {
		utils.Error(c, http.StatusBadRequest, "MISSING_FIELD", "status is required")
		return
	}
	adminUser := adminIdentity(c)
	if err := h.svc.Resolve(c.Request.Context(), id, body.Status, adminUser, body.Note); err != nil {
		h.handleError(c, err)
		return
	}
	utils.Success(c, http.StatusOK, "Reconciliation resolved", gin.H{"resolved": true})
}

// int64Param parses a positive int64 path param, writing a 400 on failure.
func int64Param(c *gin.Context, name string) (int64, bool) {
	raw := c.Param(name)
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		utils.Error(c, http.StatusBadRequest, "INVALID_PARAM", name+" must be a positive integer")
		return 0, false
	}
	return id, true
}

// adminIdentity returns the resolving admin's email (set by JWT middleware),
// falling back to "admin" when unavailable.
func adminIdentity(c *gin.Context) string {
	if v, ok := c.Get("email"); ok {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return "admin"
}

func (h *AdminReconciliationHandler) handleError(c *gin.Context, err error) {
	var pe *service.PaymentServiceError
	if errors.As(err, &pe) {
		utils.Error(c, pe.HTTPStatus, pe.Code, pe.Message)
		return
	}
	utils.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error")
}
