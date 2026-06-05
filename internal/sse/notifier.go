package sse

import (
	"time"

	"github.com/GTDGit/gtd_gateway/internal/models"
)

// TransactionNotifier is the interface services use to emit transaction events.
type TransactionNotifier interface {
	NotifyTransactionCreated(trx *models.Transaction)
	NotifyTransactionStatusChanged(trx *models.Transaction)
}

// HubNotifier implements TransactionNotifier using the SSE Hub.
type HubNotifier struct {
	hub *Hub
}

// NewHubNotifier creates a notifier backed by the given Hub.
func NewHubNotifier(hub *Hub) *HubNotifier {
	return &HubNotifier{hub: hub}
}

func (n *HubNotifier) NotifyTransactionCreated(trx *models.Transaction) {
	if n.hub.ClientCount() == 0 {
		return
	}
	n.hub.Broadcast(transactionToEvent(EventTransactionCreated, trx))
}

func (n *HubNotifier) NotifyTransactionStatusChanged(trx *models.Transaction) {
	if n.hub.ClientCount() == 0 {
		return
	}
	n.hub.Broadcast(transactionToEvent(EventTransactionStatusChanged, trx))
}

func transactionToEvent(eventType EventType, trx *models.Transaction) *TransactionEvent {
	return &TransactionEvent{
		Event:         eventType,
		TransactionID: trx.TransactionID,
		ReferenceID:   trx.ReferenceID,
		CustomerNo:    trx.CustomerNo,
		SkuCode:       trx.SkuCode,
		Type:          string(trx.Type),
		Status:        string(trx.Status),
		ProviderCode:  trx.ProviderCode,
		FailedReason:  trx.FailedReason,
		Amount:        trx.Amount,
		BuyPrice:      trx.BuyPrice,
		SellPrice:     trx.SellPrice,
		Timestamp:     time.Now(),
	}
}

// NopNotifier is a no-op implementation for when SSE is not needed.
type NopNotifier struct{}

func (n *NopNotifier) NotifyTransactionCreated(trx *models.Transaction)       {}
func (n *NopNotifier) NotifyTransactionStatusChanged(trx *models.Transaction) {}
