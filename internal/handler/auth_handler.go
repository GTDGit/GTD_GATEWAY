package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/GTDGit/gtd_gateway/internal/service"
	"github.com/GTDGit/gtd_gateway/internal/utils"
)

type AuthHandler struct {
	authService *service.AdminAuthService
}

func NewAuthHandler(authService *service.AdminAuthService) *AuthHandler {
	return &AuthHandler{authService: authService}
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req struct {
		Email    string `json:"email" binding:"required,email"`
		Password string `json:"password" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, 400, "INVALID_REQUEST", "Invalid request body")
		return
	}

	token, err := h.authService.Login(req.Email, req.Password)
	if err != nil {
		utils.Error(c, 401, "INVALID_CREDENTIALS", err.Error())
		return
	}

	utils.Success(c, 200, "Login successful", gin.H{
		"token": token,
	})
}
