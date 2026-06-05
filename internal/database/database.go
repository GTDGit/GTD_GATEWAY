package database

import (
    "context"
    "database/sql"
    "errors"
    "fmt"
    "net/url"
    "time"

    "github.com/jmoiron/sqlx"
    _ "github.com/lib/pq" // PostgreSQL driver

    appconfig "github.com/GTDGit/gtd_gateway/internal/config"
)

// Connect establishes a PostgreSQL connection using the provided configuration.
// It applies a small retry strategy to handle transient bootstrapping issues
// (e.g., DB container starting up). The returned *sqlx.DB has pool settings
// pre-configured and is pinged before returning.
func Connect(cfg *appconfig.DatabaseConfig) (*sqlx.DB, error) {
    if cfg == nil {
        return nil, errors.New("nil database config")
    }

    dsn := fmt.Sprintf(
        "postgres://%s:%s@%s:%s/%s?sslmode=%s",
        url.QueryEscape(cfg.User), url.QueryEscape(cfg.Password), cfg.Host, cfg.Port, cfg.Name, cfg.SSLMode,
    )

    // When a CA bundle path is configured (e.g., for TLS-verified RDS
    // connections), append it as the sslrootcert parameter. Leaving
    // SSLRootCert empty preserves the prior DSN exactly.
    if cfg.SSLRootCert != "" {
        dsn += "&sslrootcert=" + url.QueryEscape(cfg.SSLRootCert)
    }

    // Retry policy: up to 5 attempts, exponential backoff starting at 500ms.
    const (
        maxAttempts = 5
        baseDelay   = 500 * time.Millisecond
    )

    var db *sqlx.DB
    var lastErr error
    for attempt := 1; attempt <= maxAttempts; attempt++ {
        db, lastErr = sqlx.Open("postgres", dsn)
        if lastErr != nil {
            // Wait then retry opening.
            sleepWithBackoff(attempt, baseDelay)
            continue
        }

        // Pool settings
        setPool(db.DB)

        // Ping with timeout to validate the connection.
        ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
        lastErr = db.PingContext(ctx)
        cancel()
        if lastErr == nil {
            return db, nil
        }

        // Close and retry on ping failure.
        _ = db.Close()
        sleepWithBackoff(attempt, baseDelay)
    }

    return nil, fmt.Errorf("failed to connect to database after %d attempts: %w", maxAttempts, lastErr)
}

// setPool configures the connection pool for the database.
func setPool(db *sql.DB) {
    db.SetMaxOpenConns(25)
    db.SetMaxIdleConns(5)
    db.SetConnMaxLifetime(5 * time.Minute)
}

// sleepWithBackoff sleeps for an exponentially increasing duration.
func sleepWithBackoff(attempt int, base time.Duration) {
    // Simple exponential backoff: base * 2^(attempt-1), capped to 5s.
    d := base << (attempt - 1)
    if d > 5*time.Second {
        d = 5 * time.Second
    }
    time.Sleep(d)
}
