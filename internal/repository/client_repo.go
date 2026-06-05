package repository

import (
	"database/sql"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"

	"github.com/GTDGit/gtd_gateway/internal/models"
)

// ClientRepository provides data access methods for clients table.
type ClientRepository struct {
	db *sqlx.DB
}

// NewClientRepository creates a new ClientRepository.
func NewClientRepository(db *sqlx.DB) *ClientRepository {
	return &ClientRepository{db: db}
}

const clientColumns = `id, client_id, name, api_key, sandbox_key, callback_url, callback_secret,
    payment_callback_url, payment_callback_secret,
    ip_whitelist, scopes, is_active, created_at, updated_at`

func scanClient(scanner interface {
	Scan(dest ...any) error
}, c *models.Client) error {
	return scanner.Scan(
		&c.ID,
		&c.ClientID,
		&c.Name,
		&c.APIKey,
		&c.SandboxKey,
		&c.CallbackURL,
		&c.CallbackSecret,
		&c.PaymentCallbackURL,
		&c.PaymentCallbackSecret,
		pq.Array(&c.IPWhitelist),
		pq.Array(&c.Scopes),
		&c.IsActive,
		&c.CreatedAt,
		&c.UpdatedAt,
	)
}

// getBy is a small helper to fetch a single client by a specific column
// using a prepared statement. It ensures ip_whitelist is scanned via pq.Array.
func (r *ClientRepository) getBy(where string, arg any) (*models.Client, error) {
	const base = `SELECT ` + clientColumns + ` FROM clients WHERE `

	stmt, err := r.db.Preparex(base + where + " LIMIT 1")
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	row := stmt.QueryRowx(arg)
	var c models.Client
	if err := scanClient(row, &c); err != nil {
		if err == sql.ErrNoRows {
			return nil, sql.ErrNoRows
		}
		return nil, err
	}
	return &c, nil
}

// GetByAPIKey finds a client by production API key.
func (r *ClientRepository) GetByAPIKey(apiKey string) (*models.Client, error) {
	return r.getBy("api_key = $1", apiKey)
}

// GetBySandboxKey finds a client by sandbox key.
func (r *ClientRepository) GetBySandboxKey(sandboxKey string) (*models.Client, error) {
	return r.getBy("sandbox_key = $1", sandboxKey)
}

// GetByClientID finds a client by public client identifier.
func (r *ClientRepository) GetByClientID(clientID string) (*models.Client, error) {
	return r.getBy("client_id = $1", clientID)
}

// GetByID finds a client by numeric id.
func (r *ClientRepository) GetByID(id int) (*models.Client, error) {
	return r.getBy("id = $1", id)
}

// Create creates a new client.
func (r *ClientRepository) Create(client *models.Client) error {
	query := `INSERT INTO clients (
        client_id, name, api_key, sandbox_key, callback_url, callback_secret,
        payment_callback_url, payment_callback_secret, ip_whitelist, scopes, is_active
    ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
              RETURNING id, created_at, updated_at`

	return r.db.QueryRowx(query,
		client.ClientID,
		client.Name,
		client.APIKey,
		client.SandboxKey,
		client.CallbackURL,
		client.CallbackSecret,
		client.PaymentCallbackURL,
		client.PaymentCallbackSecret,
		pq.Array(client.IPWhitelist),
		pq.Array(client.Scopes),
		client.IsActive,
	).Scan(&client.ID, &client.CreatedAt, &client.UpdatedAt)
}

// Update updates an existing client.
func (r *ClientRepository) Update(client *models.Client) error {
	query := `UPDATE clients
              SET client_id = $1, name = $2, callback_url = $3, callback_secret = $4,
                  payment_callback_url = $5, payment_callback_secret = $6,
                  ip_whitelist = $7, scopes = $8, is_active = $9, api_key = $10, sandbox_key = $11
              WHERE id = $12
              RETURNING updated_at`

	return r.db.QueryRowx(query,
		client.ClientID,
		client.Name,
		client.CallbackURL,
		client.CallbackSecret,
		client.PaymentCallbackURL,
		client.PaymentCallbackSecret,
		pq.Array(client.IPWhitelist),
		pq.Array(client.Scopes),
		client.IsActive,
		client.APIKey,
		client.SandboxKey,
		client.ID,
	).Scan(&client.UpdatedAt)
}

// List retrieves all clients.
func (r *ClientRepository) List() ([]*models.Client, error) {
	query := `SELECT ` + clientColumns + ` FROM clients ORDER BY created_at DESC`

	rows, err := r.db.Queryx(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var clients []*models.Client
	for rows.Next() {
		var c models.Client
		if err := scanClient(rows, &c); err != nil {
			return nil, err
		}
		clients = append(clients, &c)
	}

	return clients, rows.Err()
}
