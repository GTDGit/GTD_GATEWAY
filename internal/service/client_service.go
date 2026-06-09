package service

import (
	"context"
	"database/sql"
	"errors"

	"github.com/GTDGit/gtd_gateway/internal/models"
	"github.com/GTDGit/gtd_gateway/internal/repository"
	"github.com/GTDGit/gtd_gateway/internal/utils"
)

// validateScopes ensures every requested scope is recognized.
func validateScopes(scopes []string) error {
	for _, s := range scopes {
		if !models.IsValidScope(s) {
			return errors.New("invalid scope: " + s)
		}
	}
	return nil
}

// ClientService handles client business logic.
type ClientService struct {
	clientRepo *repository.ClientRepository
}

// NewClientService constructs a ClientService.
func NewClientService(clientRepo *repository.ClientRepository) *ClientService {
	return &ClientService{clientRepo: clientRepo}
}

// CreateClientRequest represents the request to create a new client.
type CreateClientRequest struct {
	ClientID    string   `json:"clientId" binding:"required"`
	Name        string   `json:"name" binding:"required"`
	CallbackURL string   `json:"callbackUrl" binding:"required"`
	IPWhitelist []string `json:"ipWhitelist"`
	Scopes      []string `json:"scopes"`
	IsActive    *bool    `json:"isActive"`
}

// UpdateClientRequest represents the request to update a client.
type UpdateClientRequest struct {
	ClientID    string   `json:"clientId"`
	Name        string   `json:"name"`
	CallbackURL string   `json:"callbackUrl"`
	IPWhitelist []string `json:"ipWhitelist"`
	Scopes      []string `json:"scopes"`
	IsActive    *bool    `json:"isActive"`
}

// CreateClient creates a new client with auto-generated keys.
func (s *ClientService) CreateClient(ctx context.Context, req *CreateClientRequest) (*models.Client, error) {
	// Check if client_id already exists
	existing, _ := s.clientRepo.GetByClientID(req.ClientID)
	if existing != nil {
		return nil, errors.New("client_id already exists")
	}

	// Generate keys
	liveKey, err := utils.GenerateLiveKey()
	if err != nil {
		return nil, err
	}

	sandboxKey, err := utils.GenerateSandboxKey()
	if err != nil {
		return nil, err
	}

	webhookSecret, err := utils.GenerateWebhookSecret()
	if err != nil {
		return nil, err
	}

	// Create client
	// default active true if not provided
	active := true
	if req.IsActive != nil {
		active = *req.IsActive
	}

	// Default to all scopes when not specified — preserves pre-scope behavior
	// for clients created via existing automation. Admin UI will pass explicit scopes.
	scopes := req.Scopes
	if len(scopes) == 0 {
		scopes = append([]string(nil), models.AllScopes...)
	}
	if err := validateScopes(scopes); err != nil {
		return nil, err
	}

	client := &models.Client{
		ClientID:       req.ClientID,
		Name:           req.Name,
		APIKey:         liveKey,
		SandboxKey:     sandboxKey,
		CallbackURL:    req.CallbackURL,
		CallbackSecret: webhookSecret,
		IPWhitelist:    req.IPWhitelist,
		Scopes:         scopes,
		IsActive:       active,
	}

	if err := s.clientRepo.Create(client); err != nil {
		return nil, err
	}

	return client, nil
}

// GetClient retrieves a client by ID.
func (s *ClientService) GetClient(id int) (*models.Client, error) {
	return s.clientRepo.GetByID(id)
}

// GetClientByClientID retrieves a client by client_id.
func (s *ClientService) GetClientByClientID(clientID string) (*models.Client, error) {
	client, err := s.clientRepo.GetByClientID(clientID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.New("client not found")
		}
		return nil, err
	}
	return client, nil
}

// ListClients retrieves all clients.
func (s *ClientService) ListClients() ([]*models.Client, error) {
	return s.clientRepo.List()
}

// UpdateClient updates a client.
func (s *ClientService) UpdateClient(id int, req *UpdateClientRequest) (*models.Client, error) {
	client, err := s.clientRepo.GetByID(id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.New("client not found")
		}
		return nil, err
	}

	// Update fields if provided
	if req.ClientID != "" {
		client.ClientID = req.ClientID
	}
	if req.Name != "" {
		client.Name = req.Name
	}
	if req.CallbackURL != "" {
		client.CallbackURL = req.CallbackURL
	}
	if req.IPWhitelist != nil {
		client.IPWhitelist = req.IPWhitelist
	}
	if req.Scopes != nil {
		if err := validateScopes(req.Scopes); err != nil {
			return nil, err
		}
		client.Scopes = req.Scopes
	}
	if req.IsActive != nil {
		client.IsActive = *req.IsActive
	}

	if err := s.clientRepo.Update(client); err != nil {
		return nil, err
	}

	return client, nil
}

// RegenerateKeys regenerates API keys for a client.
func (s *ClientService) RegenerateKeys(id int, keyType string) (*models.Client, error) {
	client, err := s.clientRepo.GetByID(id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.New("client not found")
		}
		return nil, err
	}

	switch keyType {
	case "live":
		newKey, err := utils.GenerateLiveKey()
		if err != nil {
			return nil, err
		}
		client.APIKey = newKey
	case "sandbox":
		newKey, err := utils.GenerateSandboxKey()
		if err != nil {
			return nil, err
		}
		client.SandboxKey = newKey
	case "webhook":
		newSecret, err := utils.GenerateWebhookSecret()
		if err != nil {
			return nil, err
		}
		client.CallbackSecret = newSecret
	default:
		return nil, errors.New("invalid key_type: must be 'live', 'sandbox', or 'webhook'")
	}

	if err := s.clientRepo.Update(client); err != nil {
		return nil, err
	}

	return client, nil
}
