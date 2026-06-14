package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/GTDGit/gtd_gateway/internal/models"
	"github.com/GTDGit/gtd_gateway/internal/repository"
	"github.com/GTDGit/gtd_gateway/internal/storage"
)

// Errors surfaced to handlers for mapping to HTTP codes.
var (
	ErrQRISDocNoFiles     = errors.New("no files provided")
	ErrQRISDocBadDocType  = errors.New("invalid doc type")
	ErrQRISDocBundleEmpty = errors.New("merchant name is required")
)

// allowedDocTypes constrains the doc_type field. "extra" covers the one extra
// photo Nobu sometimes requests beyond the three standard documents.
var allowedDocTypes = map[models.QRISDocType]bool{
	models.QRISDocTypeKTP:              true,
	models.QRISDocTypeSelfieKTP:        true,
	models.QRISDocTypeBusinessLocation: true,
	models.QRISDocTypeExtra:            true,
}

// QRISDocFileInput is one file to store, already decoded to raw bytes by the
// handler (from base64 JSON or multipart form-data).
type QRISDocFileInput struct {
	DocType     models.QRISDocType
	FileName    string
	ContentType string
	Data        []byte
}

// CreateBundleInput is the service-level upload request.
type CreateBundleInput struct {
	MerchantName   string
	QRISMerchantID *int
	Note           *string
	CreatedBy      *string
	Files          []QRISDocFileInput
}

// QRISDocService owns bundle creation (upload to private S3 + DB rows), link
// building, listing, and revocation. Files are never made public; the link only
// works through the token-gated portal.
type QRISDocService struct {
	repo      *repository.QRISDocRepository
	store     storage.Storage
	keyPrefix string
	baseURL   string // public portal base, e.g. https://files.qris.gtd.co.id
}

// NewQRISDocService constructs the service. store may be nil when S3 is not
// configured; in that case uploads return a clear error instead of panicking.
func NewQRISDocService(repo *repository.QRISDocRepository, store storage.Storage, keyPrefix, baseURL string) *QRISDocService {
	return &QRISDocService{
		repo:      repo,
		store:     store,
		keyPrefix: strings.Trim(keyPrefix, "/"),
		baseURL:   strings.TrimRight(baseURL, "/"),
	}
}

// BundleLink builds the public portal URL for a bundle token.
func (s *QRISDocService) BundleLink(token string) string {
	if s.baseURL == "" {
		return "/b/" + token
	}
	return s.baseURL + "/b/" + token
}

// objectKey composes the private S3 key for a file.
func (s *QRISDocService) objectKey(bundleToken, fileToken string) string {
	parts := []string{}
	if s.keyPrefix != "" {
		parts = append(parts, s.keyPrefix)
	}
	parts = append(parts, bundleToken, fileToken)
	return strings.Join(parts, "/")
}

// CreateBundle validates input, creates the bundle row (DB generates the
// token), uploads each file to private S3, and records file rows. On a storage
// failure mid-way the already-uploaded objects are best-effort cleaned up and
// the error is returned; the bundle row is left for the retention sweep since
// it carries no usable files.
func (s *QRISDocService) CreateBundle(ctx context.Context, in *CreateBundleInput) (*models.QRISDocBundle, error) {
	if strings.TrimSpace(in.MerchantName) == "" {
		return nil, ErrQRISDocBundleEmpty
	}
	if len(in.Files) == 0 {
		return nil, ErrQRISDocNoFiles
	}
	for _, f := range in.Files {
		if !allowedDocTypes[f.DocType] {
			return nil, fmt.Errorf("%w: %q", ErrQRISDocBadDocType, f.DocType)
		}
		if len(f.Data) == 0 {
			return nil, fmt.Errorf("file %q (%s) is empty", f.FileName, f.DocType)
		}
	}
	if s.store == nil {
		return nil, errors.New("storage not configured: set S3_BUCKET and credentials")
	}

	bundle := &models.QRISDocBundle{
		MerchantName:   strings.TrimSpace(in.MerchantName),
		QRISMerchantID: in.QRISMerchantID,
		Note:           in.Note,
		CreatedBy:      in.CreatedBy,
	}
	if err := s.repo.CreateBundle(ctx, bundle); err != nil {
		return nil, fmt.Errorf("create bundle: %w", err)
	}

	var uploadedKeys []string
	for _, fin := range in.Files {
		// Generate the file token in Go so the S3 key is known before insert —
		// no placeholder/update dance. The DB token default is only a fallback.
		fileToken := uuid.NewString()
		finalKey := s.objectKey(bundle.Token, fileToken)

		sum := sha256.Sum256(fin.Data)
		checksum := hex.EncodeToString(sum[:])

		fileRow := &models.QRISDocFile{
			BundleID:    bundle.ID,
			Token:       fileToken,
			DocType:     fin.DocType,
			FileName:    sanitizeFileName(fin.FileName),
			ContentType: defaultContentType(fin.ContentType),
			SizeBytes:   int64(len(fin.Data)),
			StorageKey:  finalKey,
			Checksum:    &checksum,
		}

		// Upload to private S3 first; only persist the row if the bytes landed.
		if err := s.store.Put(ctx, finalKey, fileRow.ContentType, fin.Data); err != nil {
			s.cleanup(ctx, uploadedKeys)
			return nil, fmt.Errorf("upload file: %w", err)
		}
		uploadedKeys = append(uploadedKeys, finalKey)

		if err := s.repo.CreateFile(ctx, fileRow); err != nil {
			s.cleanup(ctx, uploadedKeys)
			return nil, fmt.Errorf("create file row: %w", err)
		}
		bundle.Files = append(bundle.Files, *fileRow)
	}

	return bundle, nil
}

// cleanup best-effort deletes uploaded objects after a mid-batch failure.
func (s *QRISDocService) cleanup(ctx context.Context, keys []string) {
	for _, k := range keys {
		_ = s.store.Delete(ctx, k)
	}
}

// GetBundleWithFiles loads a bundle by token plus its files (admin view).
func (s *QRISDocService) GetBundleWithFiles(ctx context.Context, token string) (*models.QRISDocBundle, error) {
	b, err := s.repo.GetBundleByToken(ctx, token)
	if err != nil {
		return nil, err
	}
	files, err := s.repo.ListFilesByBundle(ctx, b.ID)
	if err != nil {
		return nil, err
	}
	b.Files = files
	return b, nil
}

// ListBundles returns a page of bundles for the admin list view.
func (s *QRISDocService) ListBundles(ctx context.Context, limit, offset int) ([]models.QRISDocBundle, int, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	return s.repo.ListBundles(ctx, limit, offset)
}

// RevokeBundleByToken revokes a bundle by its link token (admin can force-close
// a link; same effect as Nobu confirming download).
func (s *QRISDocService) RevokeBundleByToken(ctx context.Context, token string) error {
	return s.repo.RevokeBundleByToken(ctx, token)
}

func sanitizeFileName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "\\", "_")
	if name == "" {
		return "file"
	}
	if len(name) > 255 {
		name = name[:255]
	}
	return name
}

func defaultContentType(ct string) string {
	ct = strings.TrimSpace(ct)
	if ct == "" {
		return "application/octet-stream"
	}
	return ct
}
