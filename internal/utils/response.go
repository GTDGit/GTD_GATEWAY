package utils

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Response defines the standard API response envelope.
type Response struct {
	Success bool        `json:"success"`
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
	Error   *ErrorInfo  `json:"error,omitempty"`
	Meta    Meta        `json:"meta"`
}

// ErrorInfo provides details for error responses.
type ErrorInfo struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Meta contains request-scoped metadata.
type Meta struct {
	RequestID  string      `json:"requestId"`
	Timestamp  string      `json:"timestamp"`
	Pagination *Pagination `json:"pagination,omitempty"`
}

// Pagination holds pagination metadata for list responses.
type Pagination struct {
	Page       int `json:"page"`
	Limit      int `json:"limit"`
	TotalItems int `json:"totalItems"`
	TotalPages int `json:"totalPages"`
}

// Success writes a success response with the standard envelope.
func Success(c *gin.Context, code int, message string, data interface{}) {
	c.JSON(code, Response{
		Success: true,
		Code:    code,
		Message: message,
		Data:    data,
		Meta: Meta{
			RequestID: getRequestID(c),
			Timestamp: time.Now().Format(time.RFC3339),
		},
	})
}

// SuccessWithPagination writes a success response with pagination metadata.
func SuccessWithPagination(c *gin.Context, code int, message string, data interface{}, page, limit, totalItems int) {
	// safety defaults
	if page <= 0 {
		page = 1
	}
	if limit <= 0 {
		limit = 50
	}
	totalPages := 0
	if limit > 0 {
		totalPages = (totalItems + limit - 1) / limit
	}
	c.JSON(code, Response{
		Success: true,
		Code:    code,
		Message: message,
		Data:    data,
		Meta: Meta{
			RequestID: getRequestID(c),
			Timestamp: time.Now().Format(time.RFC3339),
			Pagination: &Pagination{
				Page:       page,
				Limit:      limit,
				TotalItems: totalItems,
				TotalPages: totalPages,
			},
		},
	})
}

// Error writes an error response with provided API error code and message.
func Error(c *gin.Context, code int, errCode, message string) {
	c.JSON(code, Response{
		Success: false,
		Code:    code,
		Message: message,
		Error: &ErrorInfo{
			Code:    errCode,
			Message: message,
		},
		Meta: Meta{
			RequestID: getRequestID(c),
			Timestamp: time.Now().Format(time.RFC3339),
		},
	})
}

// ErrorWithData writes an error response that still includes a data payload.
func ErrorWithData(c *gin.Context, code int, message, errCode, errMessage string, data interface{}) {
	c.JSON(code, Response{
		Success: false,
		Code:    code,
		Message: message,
		Data:    data,
		Error: &ErrorInfo{
			Code:    errCode,
			Message: errMessage,
		},
		Meta: Meta{
			RequestID: getRequestID(c),
			Timestamp: time.Now().Format(time.RFC3339),
		},
	})
}

func getRequestID(c *gin.Context) string {
	if id := c.GetString("request_id"); id != "" {
		return id
	}
	return uuid.New().String()[:8]
}

// NowISO returns the current time in ISO 8601 format with WIB timezone.
func NowISO() string {
	wib := time.FixedZone("WIB", 7*3600)
	return time.Now().In(wib).Format("2006-01-02T15:04:05+07:00")
}
