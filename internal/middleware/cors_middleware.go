package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// CORSMiddleware handles Cross-Origin Resource Sharing (CORS) headers.
func CORSMiddleware() gin.HandlerFunc {
	allowedOrigins := map[string]bool{
		"http://localhost:3000":       true,
		"http://127.0.0.1:3000":       true,
		"https://admin.gtd.co.id":     true,
		"https://gtd.co.id":           true,
		"https://www.admin.gtd.co.id": true,
		"https://www.gtd.co.id":       true,
	}

	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		originNorm := strings.TrimSuffix(origin, "/")

		if allowedOrigins[originNorm] {
			c.Header("Access-Control-Allow-Origin", origin)
		}

		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With, X-Api-Key, X-Client-Id")
		c.Header("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE, PATCH")
		c.Header("Access-Control-Max-Age", "86400")

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}
