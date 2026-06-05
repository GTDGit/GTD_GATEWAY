package service

import (
	"errors"

	"github.com/rs/zerolog/log"
	"golang.org/x/crypto/bcrypt"

	"github.com/GTDGit/gtd_gateway/internal/models"
	"github.com/GTDGit/gtd_gateway/internal/repository"
	"github.com/GTDGit/gtd_gateway/internal/utils"
)

type AdminAuthService struct {
	adminRepo *repository.AdminUserRepository
}

func NewAdminAuthService(adminRepo *repository.AdminUserRepository) *AdminAuthService {
	return &AdminAuthService{adminRepo: adminRepo}
}

func (s *AdminAuthService) Login(email, password string) (string, error) {
	log.Debug().Str("email", email).Msg("Login attempt")

	user, err := s.adminRepo.GetByEmail(email)
	if err != nil {
		log.Error().Err(err).Str("email", email).Msg("Failed to get user by email")
		return "", errors.New("invalid credentials")
	}

	log.Debug().
		Int("user_id", user.ID).
		Str("email", user.Email).
		Bool("is_active", user.IsActive).
		Str("password_hash", user.PasswordHash).
		Msg("User found")

	if !user.IsActive {
		log.Warn().Str("email", email).Msg("Account is inactive")
		return "", errors.New("account is inactive")
	}

	// Verify password using bcrypt
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		log.Error().Err(err).Str("email", email).Msg("Password verification failed")
		return "", errors.New("invalid credentials")
	}

	log.Info().Str("email", email).Msg("Login successful")

	token, err := utils.GenerateJWT(user.ID, user.Email)
	if err != nil {
		return "", err
	}

	return token, nil
}

func (s *AdminAuthService) CreateAdmin(email, password, name string) error {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	user := &models.AdminUser{
		Email:        email,
		PasswordHash: string(hashedPassword),
		Name:         name,
		IsActive:     true,
	}

	return s.adminRepo.Create(user)
}
