package middleware

import (
    "fmt"
    "time"

    "github.com/gin-gonic/gin"
    "github.com/google/uuid"
    "github.com/rs/zerolog/log"
)

// LoggingMiddleware logs basic request/response details and injects a request_id into context.
func LoggingMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        start := time.Now()
        path := c.Request.URL.Path

        // Generate request ID
        requestID := uuid.New().String()[:8]
        c.Set("request_id", requestID)

        // Process request
        c.Next()

        // Log after response
        latency := time.Since(start)
        status := c.Writer.Status()
        clientIP := c.ClientIP()
        clientID := c.GetInt("client_id")

        log.Info().
            Str("request_id", requestID).
            Str("method", c.Request.Method).
            Str("path", path).
            Int("status", status).
            Dur("latency", latency).
            Str("ip", clientIP).
            Str("client_id", fmt.Sprintf("%d", clientID)).
            Msg("HTTP Request")
    }
}
