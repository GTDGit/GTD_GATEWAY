package sse

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// EventType defines the SSE event name.
type EventType string

const (
	EventTransactionCreated       EventType = "transaction.created"
	EventTransactionStatusChanged EventType = "transaction.status_changed"
)

// TransactionEvent is the payload broadcast to admin SSE clients.
type TransactionEvent struct {
	Event         EventType `json:"event"`
	TransactionID string    `json:"transactionId"`
	ReferenceID   string    `json:"referenceId"`
	CustomerNo    string    `json:"customerNo"`
	SkuCode       string    `json:"skuCode"`
	Type          string    `json:"type"`
	Status        string    `json:"status"`
	ProviderCode  *string   `json:"providerCode,omitempty"`
	FailedReason  *string   `json:"failedReason,omitempty"`
	Amount        *int      `json:"amount,omitempty"`
	BuyPrice      *int      `json:"buyPrice,omitempty"`
	SellPrice     *int      `json:"sellPrice,omitempty"`
	Timestamp     time.Time `json:"timestamp"`
}

// Client represents a connected SSE admin client.
type Client struct {
	ID     string
	Events chan []byte
}

// Hub manages SSE client connections and broadcasts.
type Hub struct {
	mu      sync.RWMutex
	clients map[string]*Client
}

// NewHub creates a new SSE hub.
func NewHub() *Hub {
	return &Hub{
		clients: make(map[string]*Client),
	}
}

// Register adds a new client and returns it for streaming.
func (h *Hub) Register(clientID string) *Client {
	h.mu.Lock()
	defer h.mu.Unlock()

	c := &Client{
		ID:     clientID,
		Events: make(chan []byte, 64),
	}
	h.clients[clientID] = c
	log.Info().Str("client_id", clientID).Int("total_clients", len(h.clients)).Msg("SSE client connected")
	return c
}

// Unregister removes a client and closes its channel.
func (h *Hub) Unregister(clientID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if c, ok := h.clients[clientID]; ok {
		close(c.Events)
		delete(h.clients, clientID)
		log.Info().Str("client_id", clientID).Int("total_clients", len(h.clients)).Msg("SSE client disconnected")
	}
}

// Broadcast sends an event to all connected clients.
// Non-blocking: drops message if client buffer is full.
func (h *Hub) Broadcast(event *TransactionEvent) {
	data, err := json.Marshal(event)
	if err != nil {
		log.Error().Err(err).Msg("Failed to marshal SSE event")
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, c := range h.clients {
		select {
		case c.Events <- data:
		default:
			log.Warn().Str("client_id", c.ID).Msg("SSE client buffer full, dropping event")
		}
	}
}

// BroadcastRaw sends an already-serialized payload to all connected clients
// without re-marshaling. Used to fan out events received from the Redis
// pub/sub bridge. Non-blocking: drops message if client buffer is full.
func (h *Hub) BroadcastRaw(data []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, c := range h.clients {
		select {
		case c.Events <- data:
		default:
			log.Warn().Str("client_id", c.ID).Msg("SSE client buffer full, dropping event")
		}
	}
}

// ClientCount returns the number of connected clients.
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}
