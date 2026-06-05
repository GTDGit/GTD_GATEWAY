package repository

import (
	"context"

	"github.com/GTDGit/gtd_gateway/internal/models"
	"github.com/jmoiron/sqlx"
)

type BankCodeRepository struct {
	db *sqlx.DB
}

func NewBankCodeRepository(db *sqlx.DB) *BankCodeRepository {
	return &BankCodeRepository{db: db}
}

func (r *BankCodeRepository) GetByCode(ctx context.Context, code string) (*models.BankCode, error) {
	query := `SELECT id, code, short_name, name, swift_code, support_va, support_disbursement, is_active, created_at, updated_at 
	          FROM bank_codes WHERE code = $1 AND is_active = true`

	var bank models.BankCode
	err := r.db.QueryRowContext(ctx, query, code).Scan(
		&bank.ID, &bank.Code, &bank.ShortName, &bank.Name, &bank.SwiftCode,
		&bank.SupportVA, &bank.SupportDisbursement, &bank.IsActive, &bank.CreatedAt, &bank.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &bank, nil
}

func (r *BankCodeRepository) GetAll(ctx context.Context) ([]models.BankCode, error) {
	query := `SELECT id, code, short_name, name, swift_code, support_va, support_disbursement, is_active, created_at, updated_at 
	          FROM bank_codes WHERE is_active = true ORDER BY code`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var banks []models.BankCode
	for rows.Next() {
		var bank models.BankCode
		if err := rows.Scan(&bank.ID, &bank.Code, &bank.ShortName, &bank.Name, &bank.SwiftCode,
			&bank.SupportVA, &bank.SupportDisbursement, &bank.IsActive, &bank.CreatedAt, &bank.UpdatedAt); err != nil {
			return nil, err
		}
		banks = append(banks, bank)
	}
	return banks, rows.Err()
}

// ListAll returns every bank including inactive ones, sorted by code (admin view).
func (r *BankCodeRepository) ListAll(ctx context.Context) ([]models.BankCode, error) {
	query := `SELECT id, code, short_name, name, swift_code, support_va, support_disbursement, is_active, created_at, updated_at
	          FROM bank_codes ORDER BY code`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var banks []models.BankCode
	for rows.Next() {
		var bank models.BankCode
		if err := rows.Scan(&bank.ID, &bank.Code, &bank.ShortName, &bank.Name, &bank.SwiftCode,
			&bank.SupportVA, &bank.SupportDisbursement, &bank.IsActive, &bank.CreatedAt, &bank.UpdatedAt); err != nil {
			return nil, err
		}
		banks = append(banks, bank)
	}
	return banks, rows.Err()
}

// GetByID returns a bank by primary key (admin view).
func (r *BankCodeRepository) GetByID(ctx context.Context, id int) (*models.BankCode, error) {
	query := `SELECT id, code, short_name, name, swift_code, support_va, support_disbursement, is_active, created_at, updated_at
	          FROM bank_codes WHERE id = $1`

	var bank models.BankCode
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&bank.ID, &bank.Code, &bank.ShortName, &bank.Name, &bank.SwiftCode,
		&bank.SupportVA, &bank.SupportDisbursement, &bank.IsActive, &bank.CreatedAt, &bank.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &bank, nil
}

// Update applies admin-editable fields to a bank row.
func (r *BankCodeRepository) Update(ctx context.Context, b *models.BankCode) error {
	query := `UPDATE bank_codes
	          SET short_name = $2, name = $3, swift_code = $4,
	              support_va = $5, support_disbursement = $6, is_active = $7,
	              updated_at = NOW()
	          WHERE id = $1
	          RETURNING updated_at`
	return r.db.QueryRowContext(ctx, query, b.ID, b.ShortName, b.Name, b.SwiftCode,
		b.SupportVA, b.SupportDisbursement, b.IsActive).Scan(&b.UpdatedAt)
}

func (r *BankCodeRepository) GetVABanks(ctx context.Context) ([]models.BankCode, error) {
	query := `SELECT id, code, short_name, name, swift_code, support_va, support_disbursement, is_active, created_at, updated_at 
	          FROM bank_codes WHERE is_active = true AND support_va = true ORDER BY code`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var banks []models.BankCode
	for rows.Next() {
		var bank models.BankCode
		if err := rows.Scan(&bank.ID, &bank.Code, &bank.ShortName, &bank.Name, &bank.SwiftCode,
			&bank.SupportVA, &bank.SupportDisbursement, &bank.IsActive, &bank.CreatedAt, &bank.UpdatedAt); err != nil {
			return nil, err
		}
		banks = append(banks, bank)
	}
	return banks, rows.Err()
}
