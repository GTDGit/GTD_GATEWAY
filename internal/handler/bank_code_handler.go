package handler

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/GTDGit/gtd_gateway/internal/models"
	"github.com/GTDGit/gtd_gateway/internal/repository"
	"github.com/GTDGit/gtd_gateway/internal/utils"
)

// BankCodeHandler handles bank code HTTP requests.
type BankCodeHandler struct {
	repo *repository.BankCodeRepository
}

// NewBankCodeHandler creates a new BankCodeHandler.
func NewBankCodeHandler(repo *repository.BankCodeRepository) *BankCodeHandler {
	return &BankCodeHandler{repo: repo}
}

// GetBankCodes returns all active bank codes.
// GET /v1/bank-codes
func (h *BankCodeHandler) GetBankCodes(c *gin.Context) {
	ctx := c.Request.Context()

	banks, err := h.repo.GetAll(ctx)
	if err != nil {
		utils.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to retrieve bank codes")
		return
	}

	items := make([]models.BankCodeItem, 0, len(banks))
	for _, b := range banks {
		items = append(items, models.BankCodeItem{
			Name:      b.Name,
			ShortName: b.ShortName,
			Code:      b.Code,
			SwiftCode: b.SwiftCode,
		})
	}

	utils.Success(c, http.StatusOK, "Bank codes retrieved successfully", items)
}

// AdminListBankCodes returns every bank including inactive (admin view).
// GET /v1/admin/bank-codes
func (h *BankCodeHandler) AdminListBankCodes(c *gin.Context) {
	banks, err := h.repo.ListAll(c.Request.Context())
	if err != nil {
		utils.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to retrieve bank codes")
		return
	}
	if banks == nil {
		banks = []models.BankCode{}
	}
	utils.Success(c, http.StatusOK, "Bank codes retrieved", banks)
}

// AdminUpdateBankCodeRequest is the editable subset of a bank record.
type AdminUpdateBankCodeRequest struct {
	ShortName           *string `json:"shortName"`
	Name                *string `json:"name"`
	SwiftCode           *string `json:"swiftCode"`
	SupportVA           *bool   `json:"supportVa"`
	SupportDisbursement *bool   `json:"supportDisbursement"`
	IsActive            *bool   `json:"isActive"`
}

// AdminUpdateBankCode applies edits to a bank row.
// PUT /v1/admin/bank-codes/:id
func (h *BankCodeHandler) AdminUpdateBankCode(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil || id <= 0 {
		utils.Error(c, http.StatusBadRequest, "INVALID_PARAM", "id must be a positive integer")
		return
	}
	var req AdminUpdateBankCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, http.StatusBadRequest, "MISSING_FIELD", "Invalid request body")
		return
	}
	bank, err := h.repo.GetByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			utils.Error(c, http.StatusNotFound, "BANK_NOT_FOUND", "Bank not found")
			return
		}
		utils.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to load bank")
		return
	}
	if req.ShortName != nil {
		bank.ShortName = *req.ShortName
	}
	if req.Name != nil {
		bank.Name = *req.Name
	}
	if req.SwiftCode != nil {
		v := *req.SwiftCode
		if v == "" {
			bank.SwiftCode = nil
		} else {
			bank.SwiftCode = &v
		}
	}
	if req.SupportVA != nil {
		bank.SupportVA = *req.SupportVA
	}
	if req.SupportDisbursement != nil {
		bank.SupportDisbursement = *req.SupportDisbursement
	}
	if req.IsActive != nil {
		bank.IsActive = *req.IsActive
	}
	if err := h.repo.Update(c.Request.Context(), bank); err != nil {
		utils.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to update bank")
		return
	}
	utils.Success(c, http.StatusOK, "Bank updated", bank)
}
