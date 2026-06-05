package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/GTDGit/gtd_gateway/internal/models"
)

type BNCTransferNotificationAmount struct {
	Value    string `json:"value"`
	Currency string `json:"currency"`
}

type BNCTransferNotification struct {
	ResponseCode               string                        `json:"responseCode"`
	ResponseMessage            string                        `json:"responseMessage"`
	OriginalReferenceNo        string                        `json:"originalReferenceNo"`
	OriginalPartnerReferenceNo string                        `json:"originalPartnerReferenceNo"`
	ReferenceNo                string                        `json:"referenceNo"`
	PartnerReferenceNo         string                        `json:"partnerReferenceNo"`
	ServiceCode                string                        `json:"serviceCode"`
	TransactionDate            string                        `json:"transactionDate"`
	FinishedTime               string                        `json:"finishedTime"`
	BeneficiaryAccountNo       string                        `json:"beneficiaryAccountNo"`
	BeneficiaryBankCode        string                        `json:"beneficiaryBankCode"`
	SourceAccountNo            string                        `json:"sourceAccountNo"`
	LatestTransactionStatus    string                        `json:"latestTransactionStatus"`
	TransactionStatusDesc      string                        `json:"transactionStatusDesc"`
	Amount                     BNCTransferNotificationAmount `json:"amount"`
}

func (n *BNCTransferNotification) TransferID() string {
	return firstNonEmptyString(n.OriginalPartnerReferenceNo, n.PartnerReferenceNo)
}

func (n *BNCTransferNotification) ProviderRef() string {
	return firstNonEmptyString(n.OriginalReferenceNo, n.ReferenceNo)
}

func (s *TransferService) ApplyBNCNotification(
	ctx context.Context,
	notification *BNCTransferNotification,
	rawPayload json.RawMessage,
) error {
	if notification == nil {
		return newTransferError(400, "INVALID_NOTIFICATION", "Invalid transfer notification", nil)
	}

	transferID := strings.TrimSpace(notification.TransferID())
	if transferID == "" {
		return newTransferError(400, "INVALID_NOTIFICATION", "Missing transfer identifier", nil)
	}

	transfer, err := s.transferRepo.GetTransferByTransferID(ctx, transferID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return newTransferError(404, "TRANSFER_NOT_FOUND", "Transfer not found", err)
		}
		return err
	}

	if providerRef := strings.TrimSpace(notification.ProviderRef()); providerRef != "" {
		transfer.ProviderRef = stringPtr(providerRef)
	}
	transfer.ProviderData = mergeTransferProviderData(transfer.ProviderData, "callback", rawPayload)

	statusCode := strings.ToUpper(strings.TrimSpace(notification.LatestTransactionStatus))
	finalTime := parseBNCNotificationTime(notification.FinishedTime, notification.TransactionDate)

	switch statusCode {
	case "00", "SUCCESS":
		transfer.Status = models.TransferStatusSuccess
		transfer.CompletedAt = &finalTime
		transfer.FailedAt = nil
		transfer.FailedReason = nil
		transfer.FailedCode = nil
	case "06", "05", "FAILED", "CANCELLED":
		transfer.Status = models.TransferStatusFailed
		transfer.CompletedAt = nil
		transfer.FailedAt = &finalTime
		transfer.FailedReason = stringPtr(nonEmptyOrDefault(notification.TransactionStatusDesc, "Transfer failed"))
		transfer.FailedCode = stringPtr(nonEmptyOrDefault(notification.LatestTransactionStatus, notification.ResponseCode))
	case "03", "02", "01", "PENDING", "PAYING", "INITIATED":
		transfer.Status = models.TransferStatusPending
	default:
		transfer.Status = models.TransferStatusProcessing
	}

	if err := s.transferRepo.UpdateTransfer(ctx, transfer); err != nil {
		return err
	}

	if transfer.Status == models.TransferStatusSuccess || transfer.Status == models.TransferStatusFailed {
		s.trySendFinalCallback(ctx, transfer)
	}

	return nil
}

func parseBNCNotificationTime(values ...string) time.Time {
	for _, raw := range values {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if ts, err := time.Parse(time.RFC3339, raw); err == nil {
			return ts
		}
	}
	return time.Now()
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
