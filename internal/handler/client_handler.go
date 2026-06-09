package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/GTDGit/gtd_gateway/internal/service"
	"github.com/GTDGit/gtd_gateway/internal/utils"
)

// ClientHandler handles client management HTTP endpoints.
type ClientHandler struct {
	clientService *service.ClientService
}

// NewClientHandler constructs a ClientHandler.
func NewClientHandler(clientService *service.ClientService) *ClientHandler {
	return &ClientHandler{clientService: clientService}
}

// CreateClient handles POST /v1/admin/clients
func (h *ClientHandler) CreateClient(c *gin.Context) {
 var req service.CreateClientRequest
 if err := c.ShouldBindJSON(&req); err != nil {
     utils.Error(c, 400, "INVALID_REQUEST", "Invalid request body")
     return
 }

	client, err := h.clientService.CreateClient(c.Request.Context(), &req)
	if err != nil {
		if err.Error() == "client_id already exists" {
			utils.Error(c, 400, "CLIENT_EXISTS", err.Error())
			return
		}
		utils.Error(c, 500, "INTERNAL_ERROR", "Failed to create client")
		return
	}

	utils.Success(c, 201, "Client created successfully", client)
}

// GetClient handles GET /v1/admin/clients/:id
func (h *ClientHandler) GetClient(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.Error(c, 400, "INVALID_ID", "Invalid client ID")
		return
	}

	client, err := h.clientService.GetClient(id)
	if err != nil {
		utils.Error(c, 404, "CLIENT_NOT_FOUND", "Client not found")
		return
	}

	utils.Success(c, 200, "Client retrieved", client)
}

// GetClientByClientID handles GET /v1/admin/clients/by-client-id/:client_id
func (h *ClientHandler) GetClientByClientID(c *gin.Context) {
	clientID := c.Param("client_id")

	client, err := h.clientService.GetClientByClientID(clientID)
	if err != nil {
		utils.Error(c, 404, "CLIENT_NOT_FOUND", "Client not found")
		return
	}

	utils.Success(c, 200, "Client retrieved", client)
}

// ListClients handles GET /v1/admin/clients
func (h *ClientHandler) ListClients(c *gin.Context) {
	clients, err := h.clientService.ListClients()
	if err != nil {
		utils.Error(c, 500, "INTERNAL_ERROR", "Failed to retrieve clients")
		return
	}

	utils.Success(c, 200, "Clients retrieved", gin.H{
		"clients": clients,
		"total":   len(clients),
	})
}

// UpdateClient handles PUT /v1/admin/clients/:id
func (h *ClientHandler) UpdateClient(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.Error(c, 400, "INVALID_ID", "Invalid client ID")
		return
	}

 var req service.UpdateClientRequest
 if err := c.ShouldBindJSON(&req); err != nil {
     utils.Error(c, 400, "INVALID_REQUEST", "Invalid request body")
     return
 }

	client, err := h.clientService.UpdateClient(id, &req)
	if err != nil {
		if err.Error() == "client not found" {
			utils.Error(c, 404, "CLIENT_NOT_FOUND", err.Error())
			return
		}
		utils.Error(c, 500, "INTERNAL_ERROR", "Failed to update client")
		return
	}

	utils.Success(c, 200, "Client updated successfully", client)
}

// RegenerateKeys handles POST /v1/admin/clients/:id/regenerate
func (h *ClientHandler) RegenerateKeys(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.Error(c, 400, "INVALID_ID", "Invalid client ID")
		return
	}

	var req struct {
		KeyType string `json:"key_type" binding:"required"` // "live", "sandbox", or "webhook"
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, 400, "INVALID_REQUEST", "key_type is required")
		return
	}

	client, err := h.clientService.RegenerateKeys(id, req.KeyType)
	if err != nil {
		if err.Error() == "client not found" {
			utils.Error(c, 404, "CLIENT_NOT_FOUND", err.Error())
			return
		}
		if err.Error() == "invalid key_type: must be 'live', 'sandbox', or 'webhook'" {
			utils.Error(c, 400, "INVALID_KEY_TYPE", err.Error())
			return
		}
		utils.Error(c, 500, "INTERNAL_ERROR", "Failed to regenerate keys")
		return
	}

	utils.Success(c, 200, "Keys regenerated successfully", client)
}
