package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/GTDGit/gtd_gateway/internal/service"
	"github.com/GTDGit/gtd_gateway/internal/utils"
)

// AdminTransactionHandler handles admin transaction HTTP endpoints.
type AdminTransactionHandler struct {
	adminTrxSvc *service.AdminTransactionService
}

// NewAdminTransactionHandler constructs an AdminTransactionHandler.
func NewAdminTransactionHandler(adminTrxSvc *service.AdminTransactionService) *AdminTransactionHandler {
	return &AdminTransactionHandler{adminTrxSvc: adminTrxSvc}
}

// ListTransactions handles GET /v1/admin/transactions
func (h *AdminTransactionHandler) ListTransactions(c *gin.Context) {
	var req service.ListTransactionsRequest

	// Parse query parameters
	if clientID := c.Query("clientId"); clientID != "" {
		if id, err := strconv.Atoi(clientID); err == nil {
			req.ClientID = &id
		}
	}
	if status := c.Query("status"); status != "" {
		req.Status = &status
	}
	if trxType := c.Query("type"); trxType != "" {
		req.Type = &trxType
	}
	if skuCode := c.Query("skuCode"); skuCode != "" {
		req.SkuCode = &skuCode
	}
	if customerNo := c.Query("customerNo"); customerNo != "" {
		req.CustomerNo = &customerNo
	}
	if referenceID := c.Query("referenceId"); referenceID != "" {
		req.ReferenceID = &referenceID
	}
	if transactionID := c.Query("transactionId"); transactionID != "" {
		req.TransactionID = &transactionID
	}
	if startDate := c.Query("startDate"); startDate != "" {
		req.StartDate = &startDate
	}
	if endDate := c.Query("endDate"); endDate != "" {
		req.EndDate = &endDate
	}
	if isSandbox := c.Query("isSandbox"); isSandbox != "" {
		val := isSandbox == "true"
		req.IsSandbox = &val
	}

	// Parse pagination
	if page := c.Query("page"); page != "" {
		if p, err := strconv.Atoi(page); err == nil {
			req.Page = p
		}
	}
	if req.Page < 1 {
		req.Page = 1
	}

	if limit := c.Query("limit"); limit != "" {
		if l, err := strconv.Atoi(limit); err == nil {
			req.Limit = l
		}
	}
	if req.Limit < 1 {
		req.Limit = 50
	}

	result, err := h.adminTrxSvc.ListTransactions(&req)
	if err != nil {
		utils.Error(c, 500, "INTERNAL_ERROR", "Failed to retrieve transactions")
		return
	}

	c.JSON(200, gin.H{
		"success": true,
		"code":    200,
		"message": "Transactions retrieved",
		"data":    result.Transactions,
		"meta": gin.H{
			"requestId": c.GetString("requestId"),
			"timestamp": utils.NowISO(),
			"pagination": gin.H{
				"page":       result.Pagination.Page,
				"limit":      result.Pagination.Limit,
				"totalItems": result.Pagination.TotalItems,
				"totalPages": result.Pagination.TotalPages,
			},
		},
	})
}

// GetTransaction handles GET /v1/admin/transactions/:id
func (h *AdminTransactionHandler) GetTransaction(c *gin.Context) {
	idOrTrxID := c.Param("id")
	if idOrTrxID == "" {
		utils.Error(c, 400, "INVALID_ID", "Transaction ID is required")
		return
	}

	trx, err := h.adminTrxSvc.GetTransaction(idOrTrxID)
	if err != nil {
		if err.Error() == "transaction not found" {
			utils.Error(c, 404, "TRANSACTION_NOT_FOUND", "Transaction not found")
			return
		}
		utils.Error(c, 500, "INTERNAL_ERROR", "Failed to retrieve transaction")
		return
	}

	utils.Success(c, 200, "Transaction retrieved", trx)
}

// GetStats handles GET /v1/admin/transactions/stats
func (h *AdminTransactionHandler) GetStats(c *gin.Context) {
	var clientID *int
	var startDate, endDate *string

	if cid := c.Query("clientId"); cid != "" {
		if id, err := strconv.Atoi(cid); err == nil {
			clientID = &id
		}
	}
	if sd := c.Query("startDate"); sd != "" {
		startDate = &sd
	}
	if ed := c.Query("endDate"); ed != "" {
		endDate = &ed
	}

	result, err := h.adminTrxSvc.GetStats(clientID, startDate, endDate)
	if err != nil {
		utils.Error(c, 500, "INTERNAL_ERROR", "Failed to retrieve statistics")
		return
	}

	stats := result.Stats

	// Calculate success rate
	successRate := float64(0)
	if stats.TotalTransactions > 0 {
		successRate = float64(stats.SuccessTransactions) / float64(stats.TotalTransactions) * 100
	}

	c.JSON(200, gin.H{
		"success": true,
		"code":    200,
		"message": "Statistics retrieved",
		"data": gin.H{
			"summary": gin.H{
				"totalTransactions":   stats.TotalTransactions,
				"successTransactions": stats.SuccessTransactions,
				"failedTransactions":  stats.FailedTransactions,
				"pendingTransactions": stats.PendingTransactions,
				"successRate":         successRate,
				"totalAmount":         stats.TotalAmount,
				"totalProfit":         0, // Note: Requires selling price tracking to calculate
			},
			"byStatus": gin.H{
				"Success":    stats.SuccessTransactions,
				"Failed":     stats.FailedTransactions,
				"Pending":    stats.PendingTransactions,
				"Processing": stats.ProcessingTransactions,
			},
			"byType": gin.H{
				"prepaid": stats.PrepaidCount,
				"inquiry": stats.InquiryCount,
				"payment": stats.PaymentCount,
			},
			"dailyTrend": result.DailyTrend,
		},
		"meta": gin.H{
			"requestId": c.GetString("requestId"),
			"timestamp": utils.NowISO(),
		},
	})
}

// GetTransactionLogs handles GET /v1/admin/transactions/:id/logs
func (h *AdminTransactionHandler) GetTransactionLogs(c *gin.Context) {
	transactionID := c.Param("id")
	if transactionID == "" {
		utils.Error(c, 400, "INVALID_ID", "Transaction ID is required")
		return
	}

	logs, err := h.adminTrxSvc.GetTransactionLogs(transactionID)
	if err != nil {
		if err.Error() == "transaction not found" {
			utils.Error(c, 404, "TRANSACTION_NOT_FOUND", "Transaction not found")
			return
		}
		utils.Error(c, 500, "INTERNAL_ERROR", "Failed to retrieve transaction logs")
		return
	}

	utils.Success(c, 200, "Transaction logs retrieved", logs)
}

// ManualRetry handles POST /v1/admin/transactions/:id/retry
func (h *AdminTransactionHandler) ManualRetry(c *gin.Context) {
	transactionID := c.Param("id")
	if transactionID == "" {
		utils.Error(c, 400, "INVALID_ID", "Transaction ID is required")
		return
	}

	trx, err := h.adminTrxSvc.ManualRetry(c.Request.Context(), transactionID)
	if err != nil {
		switch err.Error() {
		case "transaction not found":
			utils.Error(c, 404, "TRANSACTION_NOT_FOUND", "Transaction not found")
		case "transaction cannot be retried - status must be Pending or Processing":
			utils.Error(c, 400, "INVALID_STATUS", err.Error())
		case "manual retry not supported for this transaction type":
			utils.Error(c, 400, "UNSUPPORTED_OPERATION", err.Error())
		default:
			utils.Error(c, 500, "INTERNAL_ERROR", "Failed to retry transaction")
		}
		return
	}

	utils.Success(c, 200, "Transaction retry initiated", trx)
}
