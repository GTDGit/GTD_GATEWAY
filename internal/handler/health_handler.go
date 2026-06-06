package handler

import (
	"time"

	"github.com/gin-gonic/gin"

	"github.com/GTDGit/gtd_gateway/internal/utils"
)

var startTime = time.Now()

// HealthHandler provides health endpoint.
type HealthHandler struct{}

// NewHealthHandler creates a new HealthHandler.
// The digiflazz parameter is kept for API compatibility but ignored.
func NewHealthHandler(_ interface{}) *HealthHandler {
	return &HealthHandler{}
}

// GetHealth responds with service status.
func (h *HealthHandler) GetHealth(c *gin.Context) {
	data := gin.H{
		"status":  "healthy",
		"version": "1.0.0",
		"uptime":  int(time.Since(startTime).Seconds()),
	}
	utils.Success(c, 200, "Service is healthy", data)
}
