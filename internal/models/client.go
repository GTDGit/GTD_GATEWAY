package models

import "time"

// Client represents a registered API consumer of the Gerbang gateway.
// Sensitive keys are omitted from JSON responses for security.
type Client struct {
	ID             int       `db:"id" json:"id"`
	ClientID       string    `db:"client_id" json:"clientId"`
	Name           string    `db:"name" json:"name"`
	APIKey         string    `db:"api_key" json:"apiKey,omitempty"`
	SandboxKey     string    `db:"sandbox_key" json:"sandboxKey,omitempty"`
	CallbackURL    string    `db:"callback_url" json:"callbackUrl"`
	CallbackSecret string    `db:"callback_secret" json:"callbackSecret,omitempty"`
	IPWhitelist    []string  `db:"ip_whitelist" json:"ipWhitelist"`
	Scopes         []string  `db:"scopes" json:"scopes"`
	IsActive       bool      `db:"is_active" json:"isActive"`
	CreatedAt      time.Time `db:"created_at" json:"createdAt"`
	UpdatedAt      time.Time `db:"updated_at" json:"updatedAt"`
}
