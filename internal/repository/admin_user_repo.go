package repository

import (
	"github.com/jmoiron/sqlx"

	"github.com/GTDGit/gtd_gateway/internal/models"
)

type AdminUserRepository struct {
	db *sqlx.DB
}

func NewAdminUserRepository(db *sqlx.DB) *AdminUserRepository {
	return &AdminUserRepository{db: db}
}

func (r *AdminUserRepository) GetByEmail(email string) (*models.AdminUser, error) {
	var user models.AdminUser
	err := r.db.Get(&user, `
		SELECT id, email, password_hash, name, role, is_active, last_login_at, created_at, updated_at 
		FROM admin_users 
		WHERE email = $1
	`, email)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *AdminUserRepository) Create(user *models.AdminUser) error {
	query := `
		INSERT INTO admin_users (email, password_hash, name, is_active)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at, updated_at
	`
	return r.db.QueryRow(query, user.Email, user.PasswordHash, user.Name, user.IsActive).
		Scan(&user.ID, &user.CreatedAt, &user.UpdatedAt)
}
