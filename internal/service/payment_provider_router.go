package service

import (
	"sync"

	"github.com/GTDGit/gtd_gateway/internal/models"
)

// PaymentProviderRouter dispatches to provider-specific adapters.
type PaymentProviderRouter struct {
	mu      sync.RWMutex
	clients map[models.PaymentProvider]PaymentProviderClient
}

func NewPaymentProviderRouter() *PaymentProviderRouter {
	return &PaymentProviderRouter{clients: map[models.PaymentProvider]PaymentProviderClient{}}
}

func (r *PaymentProviderRouter) Register(c PaymentProviderClient) {
	if c == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.clients[c.Code()] = c
}

// Get returns the adapter for a provider or a 503 service error when none
// is registered.
func (r *PaymentProviderRouter) Get(code models.PaymentProvider) (PaymentProviderClient, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.clients[code]
	if !ok || c == nil {
		return nil, newPaymentError(503, "PAYMENT_PROVIDER_UNAVAILABLE", "Payment provider is not configured", nil)
	}
	return c, nil
}

// Has reports whether a provider adapter is registered.
func (r *PaymentProviderRouter) Has(code models.PaymentProvider) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.clients[code]
	return ok
}

// Providers returns the list of registered provider codes.
func (r *PaymentProviderRouter) Providers() []models.PaymentProvider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]models.PaymentProvider, 0, len(r.clients))
	for k := range r.clients {
		out = append(out, k)
	}
	return out
}
