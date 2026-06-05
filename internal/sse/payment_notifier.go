package sse

import (
	"encoding/json"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/GTDGit/gtd_gateway/internal/models"
)

// Payment SSE events.
const (
	EventPaymentCreated       EventType = "payment.created"
	EventPaymentStatusChanged EventType = "payment.status_changed"
)

// PaymentEvent is broadcast to admin SSE clients on payment state changes.
type PaymentEvent struct {
	Event       EventType `json:"event"`
	PaymentID   string    `json:"paymentId"`
	ReferenceID string    `json:"referenceId"`
	ClientID    int       `json:"clientId"`
	Type        string    `json:"type"`
	Status      string    `json:"status"`
	Provider    string    `json:"provider"`
	Amount      int64     `json:"amount"`
	TotalAmount int64     `json:"totalAmount"`
	Timestamp   time.Time `json:"timestamp"`
}

// PaymentNotifier is implemented by services that emit payment-state events.
type PaymentNotifier interface {
	NotifyPaymentCreated(p *models.Payment)
	NotifyPaymentStatusChanged(p *models.Payment)
}

// NopPaymentNotifier is a no-op implementation.
type NopPaymentNotifier struct{}

func (n *NopPaymentNotifier) NotifyPaymentCreated(p *models.Payment)       {}
func (n *NopPaymentNotifier) NotifyPaymentStatusChanged(p *models.Payment) {}

// NotifyPaymentCreated broadcasts a payment.created event to connected clients.
func (n *HubNotifier) NotifyPaymentCreated(p *models.Payment) {
	if n.hub.ClientCount() == 0 {
		return
	}
	n.hub.BroadcastPayment(paymentToEvent(EventPaymentCreated, p))
}

// NotifyPaymentStatusChanged broadcasts a payment.status_changed event.
func (n *HubNotifier) NotifyPaymentStatusChanged(p *models.Payment) {
	if n.hub.ClientCount() == 0 {
		return
	}
	n.hub.BroadcastPayment(paymentToEvent(EventPaymentStatusChanged, p))
}

// BroadcastPayment serializes a payment event and pushes it to every client.
func (h *Hub) BroadcastPayment(event *PaymentEvent) {
	data, err := json.Marshal(event)
	if err != nil {
		log.Error().Err(err).Msg("failed to marshal payment SSE event")
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, c := range h.clients {
		select {
		case c.Events <- data:
		default:
			log.Warn().Str("client_id", c.ID).Msg("SSE client buffer full, dropping payment event")
		}
	}
}

func paymentToEvent(eventType EventType, p *models.Payment) *PaymentEvent {
	if p == nil {
		return &PaymentEvent{Event: eventType, Timestamp: time.Now()}
	}
	return &PaymentEvent{
		Event:       eventType,
		PaymentID:   p.PaymentID,
		ReferenceID: p.ReferenceID,
		ClientID:    p.ClientID,
		Type:        string(p.PaymentType),
		Status:      string(p.Status),
		Provider:    string(p.Provider),
		Amount:      p.Amount,
		TotalAmount: p.TotalAmount,
		Timestamp:   time.Now(),
	}
}
