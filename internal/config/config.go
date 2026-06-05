package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// Config holds all application configuration loaded from environment variables.
// It is the single source of truth for runtime parameters.
type Config struct {
	Port      string
	Env       string
	JWTSecret string

	DB           DatabaseConfig
	Redis        RedisConfig
	Digiflazz    DigiflazzConfig
	Worker       WorkerConfig
	Kiosbank     KiosbankConfig
	Alterra      AlterraConfig
	BRI          BRIConfig
	Disbursement DisbursementConfig
	Payment      PaymentConfig
}

// PaymentConfig aggregates provider credentials for the payment module.
type PaymentConfig struct {
	Pakailink PakailinkConfig
	Dana      DanaConfig
	Midtrans  MidtransConfig
	Xendit    XenditConfig
}

type PakailinkConfig struct {
	Env            string
	BaseURL        string
	ClientID       string
	ClientSecret   string
	PartnerID      string
	ChannelID      string
	PrivateKeyPath string
	PrivateKeyPEM  string
	CallbackURL    string
}

type DanaConfig struct {
	Env            string
	BaseURL        string
	MerchantID     string
	ClientID       string
	ClientSecret   string
	PartnerID      string
	PrivateKeyPath string
	PrivateKeyPEM  string
	CallbackURL    string
	ReturnURL      string
}

type MidtransConfig struct {
	Env           string
	BaseURL       string
	ServerKey     string
	ClientKey     string
	MerchantID    string
	WebhookSecret string
	CallbackURL   string
}

type XenditConfig struct {
	Env          string
	BaseURL      string
	APIKey       string
	APIVersion   string
	WebhookToken string
	CallbackURL  string
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

// DigiflazzConfig contains credentials and secrets for Digiflazz integration.
type DigiflazzConfig struct {
	Username       string
	KeyProduction  string
	KeyDevelopment string
	WebhookSecret  string
}

// WorkerConfig contains interval configuration for background workers.
type WorkerConfig struct {
	SyncInterval              time.Duration
	RetryInterval             time.Duration
	CallbackInterval          time.Duration
	DigiflazzCallbackInterval time.Duration
	StatusCheckInterval       time.Duration
	StatusCheckStaleAfter     time.Duration
	StatusCheckMaxAge         time.Duration
	PaymentStatusInterval     time.Duration
	PaymentStatusStaleAfter   time.Duration
	PaymentExpiryInterval     time.Duration
	PaymentCallbackInterval   time.Duration
}

// KiosbankConfig contains credentials for Kiosbank PPOB provider
type KiosbankConfig struct {
	BaseURL                       string
	MerchantID                    string
	MerchantName                  string
	CounterID                     string
	AccountID                     string
	Mitra                         string
	Username                      string
	Password                      string
	InsecureSkipVerify            bool
	DevelopmentURL                string
	DevelopmentInsecureSkipVerify bool
	StatusCheckMinAge             time.Duration
	StatusCheckMaxAge             time.Duration
	DevelopmentCreds              KiosbankCredentialConfig
}

// KiosbankCredentialConfig contains environment-specific Kiosbank credentials.
type KiosbankCredentialConfig struct {
	MerchantID   string
	MerchantName string
	CounterID    string
	AccountID    string
	Mitra        string
	Username     string
	Password     string
}

// AlterraConfig contains credentials for Alterra PPOB provider
type AlterraConfig struct {
	BaseURL           string
	ClientID          string
	PrivateKeyPath    string // Path to RSA private key file
	PrivateKeyPEM     string // RSA private key PEM content (alternative to path)
	CallbackPublicKey string // Alterra's public key PEM for verifying callback signatures
}

// BRIConfig contains configuration for BRI SNAP BI and BRIZZI integrations.
type BRIConfig struct {
	Env                    string
	BaseURL                string
	ClientID               string
	ClientSecret           string
	PartnerID              string
	ChannelID              string
	BRIVANumber            string
	CompanyCode            string
	SourceAccount          string
	PrivateKeyPath         string
	VACallbackURL          string
	DisbCallbackURL        string
	ConnectorClientKey     string
	ConnectorPublicKeyPath string
	ConnectorPublicKeyPEM  string
	BRIZZIUsername         string
	BRIZZIDenominations    []int
}

// DisbursementConfig contains provider configuration for bank transfers.
type DisbursementConfig struct {
	BNC       BNCConfig
	Pakailink PakailinkDisbursementConfig
}

// PakailinkDisbursementConfig holds the disbursement-specific callback URL for
// PakaiLink Service 44. Credentials & signing keys are reused from the payment
// PakaiLink configuration to avoid splitting one account into two key pairs.
type PakailinkDisbursementConfig struct {
	Enabled     bool
	CallbackURL string
	SourceLabel string
}

// BNCConfig contains configuration for Bank Neo disbursement integration.
type BNCConfig struct {
	Env                    string
	BaseURL                string
	ClientID               string
	ClientSecret           string
	PartnerID              string
	ChannelID              string
	SourceAccount          string
	PrivateKeyPath         string
	DisbCallbackURL        string
	ConnectorClientKey     string
	ConnectorPublicKeyPath string
	ConnectorPublicKeyPEM  string
}

// Load reads configuration from environment variables. If a .env file exists
// in the working directory, it will be loaded first. It returns a populated
// Config or an error with a human-friendly message.
func Load() (*Config, error) {
	// Load .env if present; ignore error if file is missing so that production
	// environments relying solely on real environment variables keep working.
	_ = godotenv.Load()

	cfg := &Config{}
	var err error

	// Server
	cfg.Port = getEnv("PORT", "8080")
	cfg.Env = getEnv("ENV", "development")
	cfg.JWTSecret = getEnv("JWT_SECRET", "")

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

	// Digiflazz
	cfg.Digiflazz = DigiflazzConfig{
		Username:       getEnv("DIGIFLAZZ_USERNAME", ""),
		KeyProduction:  getEnv("DIGIFLAZZ_KEY_PRODUCTION", ""),
		KeyDevelopment: getEnv("DIGIFLAZZ_KEY_DEVELOPMENT", ""),
		WebhookSecret:  getEnv("DIGIFLAZZ_WEBHOOK_SECRET", ""),
	}

	// Kiosbank PPOB Provider
	kiosbankBaseURL := getEnv("KIOSBANK_BASE_URL", "https://transaksi.kiosbank.com:17109")
	kiosbankDevBaseURL := getEnv("KIOSBANK_DEV_BASE_URL", "https://development.kiosbank.com:4432")
	cfg.Kiosbank = KiosbankConfig{
		BaseURL:                       kiosbankBaseURL,
		MerchantID:                    getEnv("KIOSBANK_MERCHANT_ID", ""),
		MerchantName:                  getEnv("KIOSBANK_MERCHANT_NAME", ""),
		CounterID:                     getEnv("KIOSBANK_COUNTER_ID", ""),
		AccountID:                     getEnv("KIOSBANK_ACCOUNT_ID", ""),
		Mitra:                         getEnv("KIOSBANK_MITRA", ""),
		Username:                      getEnv("KIOSBANK_USERNAME", ""),
		Password:                      getEnv("KIOSBANK_PASSWORD", ""),
		InsecureSkipVerify:            getEnvBool("KIOSBANK_INSECURE_SKIP_VERIFY", defaultKiosbankInsecureSkipVerify(kiosbankBaseURL)),
		DevelopmentURL:                kiosbankDevBaseURL,
		DevelopmentInsecureSkipVerify: getEnvBool("KIOSBANK_DEV_INSECURE_SKIP_VERIFY", defaultKiosbankInsecureSkipVerify(kiosbankDevBaseURL)),
		StatusCheckMinAge:             5 * time.Minute,
		StatusCheckMaxAge:             72 * time.Hour,
		DevelopmentCreds: KiosbankCredentialConfig{
			MerchantID:   getEnv("KIOSBANK_DEV_MERCHANT_ID", getEnv("KIOSBANK_MERCHANT_ID", "")),
			MerchantName: getEnv("KIOSBANK_DEV_MERCHANT_NAME", getEnv("KIOSBANK_MERCHANT_NAME", "")),
			CounterID:    getEnv("KIOSBANK_DEV_COUNTER_ID", getEnv("KIOSBANK_COUNTER_ID", "")),
			AccountID:    getEnv("KIOSBANK_DEV_ACCOUNT_ID", getEnv("KIOSBANK_ACCOUNT_ID", "")),
			Mitra:        getEnv("KIOSBANK_DEV_MITRA", getEnv("KIOSBANK_MITRA", "")),
			Username:     getEnv("KIOSBANK_DEV_USERNAME", getEnv("KIOSBANK_USERNAME", "")),
			Password:     getEnv("KIOSBANK_DEV_PASSWORD", getEnv("KIOSBANK_PASSWORD", "")),
		},
	}
	if cfg.Kiosbank.StatusCheckMinAge, err = parseDurationEnv("KIOSBANK_STATUS_CHECK_MIN_AGE", cfg.Kiosbank.StatusCheckMinAge.String()); err != nil {
		return nil, fmt.Errorf("invalid KIOSBANK_STATUS_CHECK_MIN_AGE: %w", err)
	}
	if cfg.Kiosbank.StatusCheckMaxAge, err = parseDurationEnv("KIOSBANK_STATUS_CHECK_MAX_AGE", cfg.Kiosbank.StatusCheckMaxAge.String()); err != nil {
		return nil, fmt.Errorf("invalid KIOSBANK_STATUS_CHECK_MAX_AGE: %w", err)
	}

	// Alterra PPOB Provider
	cfg.Alterra = AlterraConfig{
		BaseURL:           getEnv("ALTERRA_BASE_URL", "https://horven-api.sumpahpalapa.com"),
		ClientID:          getEnv("ALTERRA_CLIENT_ID", ""),
		PrivateKeyPath:    getEnv("ALTERRA_PRIVATE_KEY_PATH", ""),
		PrivateKeyPEM:     getEnv("ALTERRA_PRIVATE_KEY_PEM", ""),
		CallbackPublicKey: getEnv("ALTERRA_CALLBACK_PUBLIC_KEY", ""),
	}

	// BRI SNAP BI / BRIZZI
	cfg.BRI = BRIConfig{
		Env:                    getEnv("BRI_ENV", "SANDBOX"),
		BaseURL:                getEnv("BRI_BASE_URL", ""),
		ClientID:               getEnv("BRI_CLIENT_ID", ""),
		ClientSecret:           getEnv("BRI_CLIENT_SECRET", ""),
		PartnerID:              getEnv("BRI_PARTNER_ID", ""),
		ChannelID:              getEnv("BRI_CHANNEL_ID", ""),
		BRIVANumber:            getEnv("BRI_BRIVA_NUMBER", ""),
		CompanyCode:            getEnv("BRI_COMPANY_CODE", ""),
		SourceAccount:          getEnv("BRI_SOURCE_ACCOUNT", ""),
		PrivateKeyPath:         getEnv("BRI_PRIVATE_KEY_PATH", ""),
		VACallbackURL:          getEnv("BRI_VA_CALLBACK_URL", ""),
		DisbCallbackURL:        getEnv("BRI_DISB_CALLBACK_URL", ""),
		ConnectorClientKey:     getEnv("BRI_CONNECTOR_CLIENT_KEY", ""),
		ConnectorPublicKeyPath: getEnv("BRI_CONNECTOR_PUBLIC_KEY_PATH", ""),
		ConnectorPublicKeyPEM:  getEnv("BRI_CONNECTOR_PUBLIC_KEY_PEM", ""),
		BRIZZIUsername:         getEnv("BRI_BRIZZI_USERNAME", ""),
		BRIZZIDenominations:    getEnvIntList("BRI_BRIZZI_DENOMINATIONS", []int{20000, 50000, 100000, 150000, 200000}),
	}
	if cfg.BRI.BaseURL == "" {
		switch cfg.BRI.Env {
		case "PRODUCTION":
			cfg.BRI.BaseURL = "https://partner.api.bri.co.id"
		default:
			cfg.BRI.BaseURL = "https://sandbox.partner.api.bri.co.id"
		}
	}

	// Disbursement providers
	cfg.Disbursement = DisbursementConfig{
		BNC: BNCConfig{
			Env:                    getEnv("BNC_ENV", "SANDBOX"),
			BaseURL:                getEnv("BNC_BASE_URL", ""),
			ClientID:               getEnv("BNC_CLIENT_ID", ""),
			ClientSecret:           getEnv("BNC_CLIENT_SECRET", ""),
			PartnerID:              getEnv("BNC_PARTNER_ID", ""),
			ChannelID:              getEnv("BNC_CHANNEL_ID", ""),
			SourceAccount:          getEnv("BNC_SOURCE_ACCOUNT", ""),
			PrivateKeyPath:         getEnv("BNC_PRIVATE_KEY_PATH", ""),
			DisbCallbackURL:        getEnv("BNC_DISB_CALLBACK_URL", ""),
			ConnectorClientKey:     getEnv("BNC_CONNECTOR_CLIENT_KEY", ""),
			ConnectorPublicKeyPath: getEnv("BNC_CONNECTOR_PUBLIC_KEY_PATH", ""),
			ConnectorPublicKeyPEM:  getEnv("BNC_CONNECTOR_PUBLIC_KEY_PEM", ""),
		},
		Pakailink: PakailinkDisbursementConfig{
			Enabled:     getEnv("PAKAILINK_DISBURSEMENT_ENABLED", "false") == "true",
			CallbackURL: getEnv("PAKAILINK_DISBURSEMENT_CALLBACK_URL", ""),
			SourceLabel: getEnv("PAKAILINK_DISBURSEMENT_SOURCE_LABEL", ""),
		},
	}
	if cfg.Disbursement.BNC.BaseURL == "" {
		switch cfg.Disbursement.BNC.Env {
		case "PRODUCTION":
			cfg.Disbursement.BNC.BaseURL = "https://api.bankneo.co.id"
		default:
			cfg.Disbursement.BNC.BaseURL = "https://sandbox.bankneo.co.id"
		}
	}

	// Workers (durations)
	if cfg.Worker.SyncInterval, err = parseDurationEnv("SYNC_INTERVAL", "15m"); err != nil {
		return nil, fmt.Errorf("invalid SYNC_INTERVAL: %w", err)
	}
	if cfg.Worker.RetryInterval, err = parseDurationEnv("RETRY_INTERVAL", "15m"); err != nil {
		return nil, fmt.Errorf("invalid RETRY_INTERVAL: %w", err)
	}
	if cfg.Worker.CallbackInterval, err = parseDurationEnv("CALLBACK_RETRY_INTERVAL", "1m"); err != nil {
		return nil, fmt.Errorf("invalid CALLBACK_RETRY_INTERVAL: %w", err)
	}
	if cfg.Worker.DigiflazzCallbackInterval, err = parseDurationEnv("DIGIFLAZZ_CALLBACK_INTERVAL", "30s"); err != nil {
		return nil, fmt.Errorf("invalid DIGIFLAZZ_CALLBACK_INTERVAL: %w", err)
	}
	if cfg.Worker.StatusCheckInterval, err = parseDurationEnv("STATUS_CHECK_INTERVAL", "10s"); err != nil {
		return nil, fmt.Errorf("invalid STATUS_CHECK_INTERVAL: %w", err)
	}
	if cfg.Worker.StatusCheckStaleAfter, err = parseDurationEnv("STATUS_CHECK_STALE_AFTER", "10s"); err != nil {
		return nil, fmt.Errorf("invalid STATUS_CHECK_STALE_AFTER: %w", err)
	}
	if cfg.Worker.StatusCheckMaxAge, err = parseDurationEnv("STATUS_CHECK_MAX_AGE", "5m"); err != nil {
		return nil, fmt.Errorf("invalid STATUS_CHECK_MAX_AGE: %w", err)
	}
	if cfg.Worker.PaymentStatusInterval, err = parseDurationEnv("PAYMENT_STATUS_INTERVAL", "10s"); err != nil {
		return nil, fmt.Errorf("invalid PAYMENT_STATUS_INTERVAL: %w", err)
	}
	if cfg.Worker.PaymentStatusStaleAfter, err = parseDurationEnv("PAYMENT_STATUS_STALE_AFTER", "30s"); err != nil {
		return nil, fmt.Errorf("invalid PAYMENT_STATUS_STALE_AFTER: %w", err)
	}
	if cfg.Worker.PaymentExpiryInterval, err = parseDurationEnv("PAYMENT_EXPIRY_INTERVAL", "1m"); err != nil {
		return nil, fmt.Errorf("invalid PAYMENT_EXPIRY_INTERVAL: %w", err)
	}
	if cfg.Worker.PaymentCallbackInterval, err = parseDurationEnv("PAYMENT_CALLBACK_INTERVAL", "30s"); err != nil {
		return nil, fmt.Errorf("invalid PAYMENT_CALLBACK_INTERVAL: %w", err)
	}

	// Payment providers
	cfg.Payment = PaymentConfig{
		Pakailink: PakailinkConfig{
			Env:            getEnv("PAKAILINK_ENV", "SANDBOX"),
			BaseURL:        getEnv("PAKAILINK_BASE_URL", ""),
			ClientID:       getEnv("PAKAILINK_CLIENT_ID", ""),
			ClientSecret:   getEnv("PAKAILINK_CLIENT_SECRET", ""),
			PartnerID:      getEnv("PAKAILINK_PARTNER_ID", ""),
			ChannelID:      getEnv("PAKAILINK_CHANNEL_ID", ""),
			PrivateKeyPath: getEnv("PAKAILINK_PRIVATE_KEY_PATH", ""),
			PrivateKeyPEM:  getEnv("PAKAILINK_PRIVATE_KEY_PEM", ""),
			CallbackURL:    getEnv("PAKAILINK_CALLBACK_URL", ""),
		},
		Dana: DanaConfig{
			Env:            getEnv("DANA_ENV", "SANDBOX"),
			BaseURL:        getEnv("DANA_BASE_URL", ""),
			MerchantID:     getEnv("DANA_MERCHANT_ID", ""),
			ClientID:       getEnv("DANA_CLIENT_ID", ""),
			ClientSecret:   getEnv("DANA_CLIENT_SECRET", ""),
			PartnerID:      getEnv("DANA_PARTNER_ID", ""),
			PrivateKeyPath: getEnv("DANA_PRIVATE_KEY_PATH", ""),
			PrivateKeyPEM:  getEnv("DANA_PRIVATE_KEY_PEM", ""),
			CallbackURL:    getEnv("DANA_CALLBACK_URL", ""),
			ReturnURL:      getEnv("DANA_RETURN_URL", ""),
		},
		Midtrans: MidtransConfig{
			Env:           getEnv("MIDTRANS_ENV", "SANDBOX"),
			BaseURL:       getEnv("MIDTRANS_BASE_URL", ""),
			ServerKey:     getEnv("MIDTRANS_SERVER_KEY", ""),
			ClientKey:     getEnv("MIDTRANS_CLIENT_KEY", ""),
			MerchantID:    getEnv("MIDTRANS_MERCHANT_ID", ""),
			WebhookSecret: getEnv("MIDTRANS_WEBHOOK_SECRET", ""),
			CallbackURL:   getEnv("MIDTRANS_CALLBACK_URL", ""),
		},
		Xendit: XenditConfig{
			Env:          getEnv("XENDIT_ENV", "SANDBOX"),
			BaseURL:      getEnv("XENDIT_BASE_URL", ""),
			APIKey:       getEnv("XENDIT_API_KEY", ""),
			APIVersion:   getEnv("XENDIT_API_VERSION", "2024-11-11"),
			WebhookToken: getEnv("XENDIT_WEBHOOK_TOKEN", ""),
			CallbackURL:  getEnv("XENDIT_CALLBACK_URL", ""),
		},
	}
	if cfg.Payment.Midtrans.BaseURL == "" {
		switch cfg.Payment.Midtrans.Env {
		case "PRODUCTION":
			cfg.Payment.Midtrans.BaseURL = "https://api.midtrans.com"
		default:
			cfg.Payment.Midtrans.BaseURL = "https://api.sandbox.midtrans.com"
		}
	}
	if cfg.Payment.Xendit.BaseURL == "" {
		cfg.Payment.Xendit.BaseURL = "https://api.xendit.co"
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

// getEnvBool parses a boolean environment variable.
func getEnvBool(key string, def bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	switch strings.ToLower(v) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return def
	}
}

// getEnvIntList parses a comma-separated integer environment variable.
func getEnvIntList(key string, def []int) []int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return append([]int(nil), def...)
	}

	parts := strings.Split(v, ",")
	values := make([]int, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		n, err := strconv.Atoi(part)
		if err != nil {
			return append([]int(nil), def...)
		}
		values = append(values, n)
	}
	if len(values) == 0 {
		return append([]int(nil), def...)
	}
	return values
}

// parseDurationEnv reads an environment variable and parses it as time.Duration.
// If the variable is empty, it falls back to the provided default value.
func parseDurationEnv(key, def string) (time.Duration, error) {
	raw := getEnv(key, def)
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, err
	}
	if d < 0 {
		return 0, fmt.Errorf("duration must be >= 0")
	}
	return d, nil
}

func defaultKiosbankInsecureSkipVerify(baseURL string) bool {
	return strings.Contains(strings.ToLower(baseURL), "development.kiosbank.com")
}
