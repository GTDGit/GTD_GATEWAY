package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/GTDGit/gtd_gateway/internal/models"
	"github.com/GTDGit/gtd_gateway/internal/service"
	"github.com/GTDGit/gtd_gateway/internal/utils"
)

// QRISDocHandler exposes admin endpoints to inspect file portal links and their
// status, and to force-close (revoke) a bundle. Upload lives in the standalone
// portal (dev-files.gtd.co.id); the gateway is read-only here.
type QRISDocHandler struct {
	svc *service.QRISDocService
}

// NewQRISDocHandler constructs a QRISDocHandler.
func NewQRISDocHandler(svc *service.QRISDocService) *QRISDocHandler {
	return &QRISDocHandler{svc: svc}
}

// ListBundles handles GET /v1/admin/qris/documents.
func (h *QRISDocHandler) ListBundles(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * limit

	bundles, total, err := h.svc.ListBundles(c.Request.Context(), limit, offset)
	if err != nil {
		utils.Error(c, 500, "INTERNAL_ERROR", "Failed to list bundles")
		return
	}

	// Attach the portal link to each bundle for admin convenience.
	type bundleWithLink struct {
		models.QRISDocBundle
		Link string `json:"link"`
	}
	out := make([]bundleWithLink, 0, len(bundles))
	for _, b := range bundles {
		out = append(out, bundleWithLink{QRISDocBundle: b, Link: h.svc.BundleLink(b.Token)})
	}

	utils.SuccessWithPagination(c, 200, "Bundles retrieved", out, page, limit, total)
}

// GetBundle handles GET /v1/admin/qris/documents/:token (with files).
func (h *QRISDocHandler) GetBundle(c *gin.Context) {
	token := c.Param("token")
	b, err := h.svc.GetBundleWithFiles(c.Request.Context(), token)
	if err != nil {
		utils.Error(c, 404, "BUNDLE_NOT_FOUND", "Bundle not found")
		return
	}
	utils.Success(c, 200, "Bundle retrieved", gin.H{
		"bundle": b,
		"link":   h.svc.BundleLink(b.Token),
	})
}

// RevokeBundle handles POST /v1/admin/qris/documents/:token/revoke — lets an
// operator force-close a link (same effect as the recipient confirming).
func (h *QRISDocHandler) RevokeBundle(c *gin.Context) {
	token := c.Param("token")
	if err := h.svc.RevokeBundleByToken(c.Request.Context(), token); err != nil {
		utils.Error(c, 404, "BUNDLE_NOT_FOUND", "Bundle not found")
		return
	}
	utils.Success(c, 200, "Bundle revoked", gin.H{"token": token, "status": "revoked"})
}
