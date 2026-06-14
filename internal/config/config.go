package config

import (
	"errors"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

// Config holds all application configuration loaded from environment variables.
// It is the single source of truth for runtime parameters.
//
// Provider-specific config blocks (Digiflazz, Kiosbank, Alterra, BRI,
// Disbursement, Payment, Worker) have been removed: the Gateway runs in
// admin-read/view-only mode and does not instantiate any provider clients.
type Config struct {
	Port      string
	Env       string
	JWTSecret string

	// Service-to-service: the gateway proxies Pakailink QRIS register/generate
	// to the api service (which owns the provider client). These are empty when
	// the proxy is not configured, in which case those admin actions return 503.
	APIInternalURL   string
	InternalAPIToken string

	DB      DatabaseConfig
	Redis   RedisConfig
	Storage StorageConfig

	// FilesBaseURL is the public base of the QRIS document portal
	// (e.g. https://files.qris.gtd.co.id). The gateway uses it to build the
	// shareable bundle link returned after an upload. No trailing slash.
	FilesBaseURL string
}

// StorageConfig contains S3 (Jakarta / ap-southeast-3) parameters for the QRIS
// document portal. Files are ALWAYS private — the bucket must block all public
// access; objects are only ever streamed through a token-validating handler.
type StorageConfig struct {
	Region    string // ap-southeast-3 (Jakarta) for data residency
	Bucket    string
	Endpoint  string // optional custom endpoint (e.g. MinIO for local dev); empty = AWS default
	AccessKey string
	SecretKey string
	KeyPrefix string // optional object key prefix, e.g. "qris-docs"
}

// DatabaseConfig contains PostgreSQL connection parameters.
type DatabaseConfig struct {
	Host        string
	Port        string
	User        string
	Password    string
	Name        string
	SSLMode     string
	SSLRootCert string
}

// RedisConfig contains Redis connection parameters.
type RedisConfig struct {
	Host     string
	Port     string
	Password string
	DB       int
}

// Load reads configuration from environment variables. If a .env file exists
// in the working directory, it will be loaded first. It returns a populated
// Config or an error with a human-friendly message.
func Load() (*Config, error) {
	// Load .env if present; ignore error if file is missing so that production
	// environments relying solely on real environment variables keep working.
	_ = godotenv.Load()

	cfg := &Config{}

	// Server
	cfg.Port = getEnv("PORT", "8080")
	cfg.Env = getEnv("ENV", "development")
	cfg.JWTSecret = getEnv("JWT_SECRET", "")
	cfg.APIInternalURL = getEnv("API_INTERNAL_URL", "")
	cfg.InternalAPIToken = getEnv("INTERNAL_API_TOKEN", "")
	cfg.FilesBaseURL = strings.TrimRight(getEnv("FILES_BASE_URL", ""), "/")

	// Database
	cfg.DB = DatabaseConfig{
		Host:        getEnv("DB_HOST", ""),
		Port:        getEnv("DB_PORT", "5432"),
		User:        getEnv("DB_USER", ""),
		Password:    getEnv("DB_PASSWORD", ""),
		Name:        getEnv("DB_NAME", ""),
		SSLMode:     getEnv("DB_SSLMODE", "disable"),
		SSLRootCert: getEnv("DB_SSLROOTCERT", ""),
	}

	// Redis
	cfg.Redis = RedisConfig{
		Host:     getEnv("REDIS_HOST", "redis"),
		Port:     getEnv("REDIS_PORT", "6379"),
		Password: getEnv("REDIS_PASSWORD", ""),
		DB:       getEnvInt("REDIS_DB", 0),
	}

	// Storage (S3 Jakarta) for the QRIS document portal.
	cfg.Storage = StorageConfig{
		Region:    getEnv("FILES_S3_REGION", "ap-southeast-3"),
		Bucket:    getEnv("FILES_S3_BUCKET", ""),
		Endpoint:  getEnv("FILES_S3_ENDPOINT", ""),
		AccessKey: getEnv("FILES_S3_ACCESS_KEY", ""),
		SecretKey: getEnv("FILES_S3_SECRET_KEY", ""),
		KeyPrefix: getEnv("FILES_S3_KEY_PREFIX", "qris-docs"),
	}

	// Basic validation for DB parameters — keeps messages concise and helpful.
	if cfg.DB.Host == "" || cfg.DB.User == "" || cfg.DB.Name == "" {
		return nil, errors.New("database configuration incomplete: ensure DB_HOST, DB_USER, and DB_NAME are set")
	}

	// Validate JWT_SECRET
	if cfg.JWTSecret == "" {
		return nil, errors.New("JWT_SECRET must be set for authentication")
	}

	return cfg, nil
}

// getEnv returns the value of an environment variable or a default if empty.
func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// getEnvInt returns the value of an environment variable as an integer or a default if empty/invalid.
func getEnvInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return i
}
