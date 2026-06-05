package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/GTDGit/gtd_gateway/internal/models"
	"github.com/GTDGit/gtd_gateway/internal/repository"
)

type TransferCallbackService struct {
	clientRepo *repository.ClientRepository
	bankRepo   *repository.BankCodeRepository
	httpClient *http.Client
}

func NewTransferCallbackService(clientRepo *repository.ClientRepository, bankRepo *repository.BankCodeRepository) *TransferCallbackService {
	return &TransferCallbackService{
		clientRepo: clientRepo,
		bankRepo:   bankRepo,
		httpClient: &http.Client{Timeout: 20 * time.Second},
	}
}

// Send returns handled=true when no retry is needed anymore.
func (s *TransferCallbackService) Send(ctx context.Context, transfer *models.Transfer) (bool, error) {
	if transfer == nil {
		return true, nil
	}
	if transfer.Status != models.TransferStatusSuccess && transfer.Status != models.TransferStatusFailed {
		return false, nil
	}

	client, err := s.clientRepo.GetByID(transfer.ClientID)
	if err != nil {
		return false, err
	}
	if client == nil || client.CallbackURL == "" {
		return true, nil
	}

	payload, event, err := s.buildPayload(ctx, transfer)
	if err != nil {
		return false, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, client.CallbackURL, bytes.NewReader(payload))
	if err != nil {
		return false, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GTD-Signature", "sha256="+signTransferCallback(payload, client.CallbackSecret))
	req.Header.Set("X-GTD-Event", event)
	req.Header.Set("X-GTD-Timestamp", formatTransferTime(time.Now()))
	req.Header.Set("X-GTD-Request-Id", newTransferCallbackRequestID())

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	_, _ = io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("transfer callback returned status %d", resp.StatusCode)
	}
	return true, nil
}

func (s *TransferCallbackService) buildPayload(ctx context.Context, transfer *models.Transfer) ([]byte, string, error) {
	bankShortName := ""
	if bank, err := s.bankRepo.GetByCode(ctx, transfer.BankCode); err == nil && bank != nil {
		bankShortName = bank.ShortName
	}

	event := "transfer.failed"
	if transfer.Status == models.TransferStatusSuccess {
		event = "transfer.success"
	}

	payload := map[string]any{
		"event": event,
		"data": map[string]any{
			"transferId":         transfer.TransferID,
			"referenceId":        transfer.ReferenceID,
			"status":             string(transfer.Status),
			"transferType":       string(transfer.TransferType),
			"route":              transferRoute(transfer.TransferType),
			"bankCode":           transfer.BankCode,
			"bankShortName":      bankShortName,
			"bankName":           derefString(transfer.BankName),
			"accountNumber":      transfer.AccountNumber,
			"accountName":        derefString(transfer.AccountName),
			"amount":             transfer.Amount,
			"fee":                transfer.Fee,
			"totalAmount":        transfer.TotalAmount,
			"purpose":            derefString(transfer.PurposeCode),
			"purposeDescription": transferPurposeDescription(derefString(transfer.PurposeCode)),
			"remark":             derefString(transfer.Remark),
			"providerRef":        derefString(transfer.ProviderRef),
			"createdAt":          formatTransferTime(transfer.CreatedAt),
		},
		"timestamp": formatTransferTime(time.Now()),
	}

	if transfer.CompletedAt != nil {
		payload["data"].(map[string]any)["completedAt"] = formatTransferTime(*transfer.CompletedAt)
	}
	if transfer.FailedAt != nil {
		payload["data"].(map[string]any)["failedAt"] = formatTransferTime(*transfer.FailedAt)
	}
	if transfer.FailedReason != nil {
		payload["data"].(map[string]any)["failedReason"] = *transfer.FailedReason
	}
	if transfer.FailedCode != nil {
		payload["data"].(map[string]any)["failedCode"] = *transfer.FailedCode
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, "", err
	}
	return raw, event, nil
}

func signTransferCallback(payload []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

func newTransferCallbackRequestID() string {
	buf := make([]byte, 8)
	_, _ = rand.Read(buf)
	return "tf_cb_" + hex.EncodeToString(buf)
}
