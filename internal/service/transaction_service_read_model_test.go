package service

import (
	"testing"
	"time"

	"github.com/GTDGit/gtd_gateway/internal/cache"
	"github.com/GTDGit/gtd_gateway/internal/models"
)

func TestCachedInquiryToTransactionIncludesReadModelFields(t *testing.T) {
	t.Parallel()

	svc := &TransactionService{}
	cachedAt := time.Date(2026, time.April, 15, 19, 15, 0, 0, time.FixedZone("WIB", 7*3600))
	expiredAt := cachedAt.Add(10 * time.Minute)

	data := &cache.InquiryData{
		TransactionID:         "GRB-20260415-123456",
		ReferenceID:           "REF-001",
		ClientID:              99,
		ProductID:             77,
		CustomerNo:            "551600530024",
		SKUCode:               "2101001",
		Amount:                25000,
		Admin:                 2500,
		CustomerName:          "  PLN TEST  ",
		ExpiredAt:             expiredAt,
		CachedAt:              cachedAt,
		ProviderCode:          "kiosbank",
		ProviderID:            1,
		ProviderSKUID:         88,
		ProviderTransactionID: "760864752227",
		Status:                string(models.StatusFailed),
		FailedCode:            "04",
		FailedReason:          "   No Response From Biller   ",
	}

	trx := svc.cachedInquiryToTransaction(data, data.ClientID, data.ProductID)

	if trx.CreatedAt != cachedAt {
		t.Fatalf("CreatedAt = %v, want %v", trx.CreatedAt, cachedAt)
	}
	if trx.ProcessedAt == nil || !trx.ProcessedAt.Equal(cachedAt) {
		t.Fatalf("ProcessedAt = %v, want %v", trx.ProcessedAt, cachedAt)
	}
	if trx.ProviderCode == nil || *trx.ProviderCode != "kiosbank" {
		t.Fatalf("ProviderCode = %v", trx.ProviderCode)
	}
	if trx.ProviderID == nil || *trx.ProviderID != 1 {
		t.Fatalf("ProviderID = %v", trx.ProviderID)
	}
	if trx.ProviderSKUID == nil || *trx.ProviderSKUID != 88 {
		t.Fatalf("ProviderSKUID = %v", trx.ProviderSKUID)
	}
	if trx.ProviderRefID == nil || *trx.ProviderRefID != "760864752227" {
		t.Fatalf("ProviderRefID = %v", trx.ProviderRefID)
	}
	if trx.FailedReason == nil || *trx.FailedReason != "No Response From Biller" {
		t.Fatalf("FailedReason = %v", trx.FailedReason)
	}
	if trx.CustomerName == nil || *trx.CustomerName != "PLN TEST" {
		t.Fatalf("CustomerName = %v", trx.CustomerName)
	}
}
