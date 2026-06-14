package handler

import (
	"encoding/base64"
	"errors"
	"io"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/GTDGit/gtd_gateway/internal/models"
	"github.com/GTDGit/gtd_gateway/internal/service"
	"github.com/GTDGit/gtd_gateway/internal/utils"
)

// maxFileBytes caps a single uploaded document at 15 MB (phone photos of KTP /
// selfie / storefront are well under this).
const maxFileBytes = 15 << 20

// QRISDocHandler exposes admin endpoints to upload QRIS onboarding documents
// and manage the resulting shareable bundle links.
type QRISDocHandler struct {
	svc *service.QRISDocService
}

// NewQRISDocHandler constructs a QRISDocHandler.
func NewQRISDocHandler(svc *service.QRISDocService) *QRISDocHandler {
	return &QRISDocHandler{svc: svc}
}

// base64FileReq is one file in a JSON upload body.
type base64FileReq struct {
	DocType     string `json:"docType" binding:"required"`     // ktp | selfie_ktp | business_location | extra
	FileName    string `json:"fileName" binding:"required"`
	ContentType string `json:"contentType"`
	DataBase64  string `json:"dataBase64" binding:"required"`  // raw base64 or data: URI
}

// createBundleJSONReq is the application/json upload shape.
type createBundleJSONReq struct {
	MerchantName   string          `json:"merchantName" binding:"required"`
	QRISMerchantID *int            `json:"qrisMerchantId"`
	Note           *string         `json:"note"`
	Files          []base64FileReq `json:"files" binding:"required"`
}

// CreateBundle handles POST /v1/admin/qris/documents.
//
// Accepts EITHER application/json with base64-encoded files, OR multipart
// form-data. In both cases the doc type and file name come from the request.
// Returns the bundle plus the shareable portal link.
func (h *QRISDocHandler) CreateBundle(c *gin.Context) {
	createdBy := optString(c.GetString("email"))

	in := &service.CreateBundleInput{CreatedBy: createdBy}

	contentType := c.ContentType()
	switch {
	case strings.HasPrefix(contentType, "multipart/form-data"):
		if err := h.parseMultipart(c, in); err != nil {
			utils.Error(c, 400, "INVALID_REQUEST", err.Error())
			return
		}
	case strings.HasPrefix(contentType, "application/json"):
		if err := h.parseJSON(c, in); err != nil {
			utils.Error(c, 400, "INVALID_REQUEST", err.Error())
			return
		}
	default:
		utils.Error(c, 415, "UNSUPPORTED_MEDIA_TYPE", "use application/json (base64) or multipart/form-data")
		return
	}

	bundle, err := h.svc.CreateBundle(c.Request.Context(), in)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrQRISDocNoFiles),
			errors.Is(err, service.ErrQRISDocBadDocType),
			errors.Is(err, service.ErrQRISDocBundleEmpty):
			utils.Error(c, 400, "INVALID_REQUEST", err.Error())
		default:
			utils.Error(c, 500, "INTERNAL_ERROR", "Failed to create document bundle")
		}
		return
	}

	utils.Success(c, 201, "Document bundle created", gin.H{
		"bundle": bundle,
		"link":   h.svc.BundleLink(bundle.Token),
	})
}

// parseJSON decodes a base64 JSON upload body into the service input.
func (h *QRISDocHandler) parseJSON(c *gin.Context, in *service.CreateBundleInput) error {
	var req createBundleJSONReq
	if err := c.ShouldBindJSON(&req); err != nil {
		return errors.New("invalid JSON body: merchantName and files are required")
	}
	in.MerchantName = req.MerchantName
	in.QRISMerchantID = req.QRISMerchantID
	in.Note = req.Note

	for _, f := range req.Files {
		data, err := decodeBase64(f.DataBase64)
		if err != nil {
			return errors.New("invalid base64 for file " + f.FileName)
		}
		if len(data) > maxFileBytes {
			return errors.New("file too large (max 15MB): " + f.FileName)
		}
		in.Files = append(in.Files, service.QRISDocFileInput{
			DocType:     models.QRISDocType(f.DocType),
			FileName:    f.FileName,
			ContentType: f.ContentType,
			Data:        data,
		})
	}
	return nil
}

// parseMultipart reads form-data files. Each file part's form field name is the
// doc type (ktp, selfie_ktp, business_location, extra); merchantName/note come
// from text fields.
func (h *QRISDocHandler) parseMultipart(c *gin.Context, in *service.CreateBundleInput) error {
	in.MerchantName = c.PostForm("merchantName")
	if note := c.PostForm("note"); note != "" {
		in.Note = &note
	}
	if v := c.PostForm("qrisMerchantId"); v != "" {
		if id, err := strconv.Atoi(v); err == nil {
			in.QRISMerchantID = &id
		}
	}

	form, err := c.MultipartForm()
	if err != nil {
		return errors.New("invalid multipart form")
	}

	for field, headers := range form.File {
		for _, fh := range headers {
			if fh.Size > maxFileBytes {
				return errors.New("file too large (max 15MB): " + fh.Filename)
			}
			file, err := fh.Open()
			if err != nil {
				return errors.New("cannot read uploaded file: " + fh.Filename)
			}
			data, err := io.ReadAll(io.LimitReader(file, maxFileBytes+1))
			file.Close()
			if err != nil {
				return errors.New("cannot read uploaded file: " + fh.Filename)
			}
			if len(data) > maxFileBytes {
				return errors.New("file too large (max 15MB): " + fh.Filename)
			}
			in.Files = append(in.Files, service.QRISDocFileInput{
				DocType:     models.QRISDocType(field),
				FileName:    fh.Filename,
				ContentType: fh.Header.Get("Content-Type"),
				Data:        data,
			})
		}
	}
	return nil
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
// operator force-close a link (same effect as Nobu confirming).
func (h *QRISDocHandler) RevokeBundle(c *gin.Context) {
	token := c.Param("token")
	if err := h.svc.RevokeBundleByToken(c.Request.Context(), token); err != nil {
		utils.Error(c, 404, "BUNDLE_NOT_FOUND", "Bundle not found")
		return
	}
	utils.Success(c, 200, "Bundle revoked", gin.H{"token": token, "status": "revoked"})
}

// decodeBase64 accepts a raw base64 string or a data: URI (data:<ct>;base64,<payload>).
func decodeBase64(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, ";base64,"); i != -1 {
		s = s[i+len(";base64,"):]
	} else if strings.HasPrefix(s, "data:") {
		if i := strings.Index(s, ","); i != -1 {
			s = s[i+1:]
		}
	}
	return base64.StdEncoding.DecodeString(s)
}

func optString(s string) *string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return &s
}
