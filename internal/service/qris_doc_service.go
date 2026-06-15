package service

import (
	"context"
	"strings"

	"github.com/GTDGit/gtd_gateway/internal/models"
	"github.com/GTDGit/gtd_gateway/internal/repository"
)

// QRISDocService is the gateway's READ-ONLY view over the file portal. Upload
// and delivery live in the standalone portal (dev-files.gtd.co.id); here admins
// only list/inspect bundles, build the shareable link, and force-close
// (revoke) a bundle.
type QRISDocService struct {
	repo    *repository.QRISDocRepository
	baseURL string // public portal base, e.g. https://dev-files.gtd.co.id
}

// NewQRISDocService constructs the read-only service.
func NewQRISDocService(repo *repository.QRISDocRepository, baseURL string) *QRISDocService {
	return &QRISDocService{
		repo:    repo,
		baseURL: strings.TrimRight(baseURL, "/"),
	}
}

// BundleLink builds the public portal URL for a bundle token.
func (s *QRISDocService) BundleLink(token string) string {
	if s.baseURL == "" {
		return "/b/" + token
	}
	return s.baseURL + "/b/" + token
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

// RevokeBundleByToken revokes a bundle by its link token (admin force-close;
// same effect as the recipient confirming download).
func (s *QRISDocService) RevokeBundleByToken(ctx context.Context, token string) error {
	return s.repo.RevokeBundleByToken(ctx, token)
}
