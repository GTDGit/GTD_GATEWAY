package handler

import (
	"fmt"
	"io"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"

	"github.com/GTDGit/gtd_gateway/internal/sse"
	"github.com/GTDGit/gtd_gateway/internal/utils"
)

// SSEHandler handles Server-Sent Events for admin real-time updates.
type SSEHandler struct {
	hub *sse.Hub
}

// NewSSEHandler creates a new SSEHandler.
func NewSSEHandler(hub *sse.Hub) *SSEHandler {
	return &SSEHandler{hub: hub}
}

// Stream handles GET /v1/admin/sse?token=<jwt>
// EventSource API cannot set custom headers, so JWT is passed via query param.
func (h *SSEHandler) Stream(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		utils.Error(c, 401, "UNAUTHORIZED", "Missing token query parameter")
		return
	}

	claims, err := utils.ValidateJWT(token)
	if err != nil {
		utils.Error(c, 401, "INVALID_TOKEN", "Invalid or expired token")
		return
	}

	clientID := fmt.Sprintf("admin-%d-%d", claims.UserID, time.Now().UnixNano())

	// SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no") // Disable nginx buffering

	client := h.hub.Register(clientID)
	defer h.hub.Unregister(clientID)

	// Send initial connected event
	c.SSEvent("connected", gin.H{
		"clientId":  clientID,
		"message":   "SSE connection established",
		"timestamp": time.Now().Format(time.RFC3339),
	})
	c.Writer.Flush()

	log.Info().Str("client_id", clientID).Int("user_id", claims.UserID).Msg("Admin SSE stream started")

	// Stream events
	c.Stream(func(w io.Writer) bool {
		select {
		case data, ok := <-client.Events:
			if !ok {
				return false
			}
			c.SSEvent("transaction", string(data))
			return true
		case <-time.After(30 * time.Second):
			c.SSEvent("ping", gin.H{"timestamp": time.Now().Format(time.RFC3339)})
			return true
		case <-c.Request.Context().Done():
			return false
		}
	})
}
