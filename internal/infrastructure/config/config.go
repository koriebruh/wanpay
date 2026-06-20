package config

import (
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
	"github.com/samber/do/v2"
)

type Config struct {
	App      AppConfig      `toml:"app"`
	Database DatabaseConfig `toml:"database"`
	Redis    RedisConfig    `toml:"redis"`
	Logger   LoggerConfig   `toml:"logger"`
	Provider ProviderConfig `toml:"provider"`
	OTEL     OTELConfig     `toml:"otel"`
	Fee      FeeConfig      `toml:"fee"`
}

type OTELConfig struct {
	Enabled     bool    `toml:"enabled"`
	Endpoint    string  `toml:"endpoint"` // Jaeger OTLP gRPC: host:port
	ServiceName string  `toml:"service_name"`
	SampleRatio float64 `toml:"sample_ratio"` // 1.0 = always, 0.1 = 10%
}

type AppConfig struct {
	Name        string         `toml:"name"`
	Env         string         `toml:"env"`
	Port        string         `toml:"port"`
	BaseURL     string         `toml:"base_url"`
	CallbackURL string         `toml:"callback_url"`
	Shutdown    ShutdownConfig `toml:"shutdown"`
	HTTP        HTTPConfig     `toml:"http"`
}

type HTTPConfig struct {
	RequestTimeoutSeconds int      `toml:"request_timeout_seconds"`
	MaxBodySize           string   `toml:"max_body_size"`
	CORSAllowOrigins      []string `toml:"cors_allow_origins"`
}

type ShutdownConfig struct {
	TimeoutSeconds int `toml:"timeout_seconds"`
}

type DatabaseConfig struct {
	DSN                    string `toml:"dsn"`
	MigrateDSN             string `toml:"migrate_dsn"` // direct Postgres (bypasses PgBouncer — advisory locks)
	MaxOpenConns           int    `toml:"max_open_conns"`
	MaxIdleConns           int    `toml:"max_idle_conns"`
	ConnMaxLifetimeMinutes int    `toml:"conn_max_lifetime_minutes"`
}

type RedisConfig struct {
	Enabled  bool   `toml:"enabled"`
	Addr     string `toml:"addr"`
	Password string `toml:"password"`
	DB       int    `toml:"db"`
}

type LoggerConfig struct {
	Level    string `toml:"level"`
	Encoding string `toml:"encoding"`
	Output   string `toml:"output"`
}

type ProviderConfig struct {
	Midtrans       MidtransConfig       `toml:"midtrans"`
	Xendit         XenditConfig         `toml:"xendit"`
	Doku           DokuConfig           `toml:"doku"`
	CircuitBreaker CircuitBreakerConfig `toml:"circuit_breaker"`
}

type CircuitBreakerConfig struct {
	MaxRequests         uint32 `toml:"max_requests"`         // max requests in half-open state
	IntervalSeconds     int    `toml:"interval_seconds"`     // rolling window for failure counting
	TimeoutSeconds      int    `toml:"timeout_seconds"`      // seconds to stay open before half-open
	ConsecutiveFailures uint32 `toml:"consecutive_failures"` // failures before opening
}

type MidtransConfig struct {
	ServerKey    string `toml:"server_key"`
	ClientKey    string `toml:"client_key"`
	IsProduction bool   `toml:"is_production"`
}

type XenditConfig struct {
	SecretKey    string `toml:"secret_key"`
	WebhookToken string `toml:"webhook_token"`
	CallbackURL  string `toml:"callback_url"` // required for QRIS — Xendit POSTs payment events here
}

type DokuConfig struct {
	ClientID      string `toml:"client_id"`
	SecretKey     string `toml:"secret_key"`    // Active Secret Key from dashboard
	APIKey        string `toml:"api_key"`       // API Key from dashboard — used for HMAC-SHA512 request signatures
	PrivateKeyPEM string `toml:"private_key"`   // RSA PKCS8 PEM — used for SHA256withRSA B2B access token signature
}

// FeeConfig holds platform-wide margin settings applied on top of each merchant's FeeConfig.
// This is Wanpey's revenue layer — separate from what merchants are individually contracted to pay.
type FeeConfig struct {
	Margin MarginConfig `toml:"margin"`
}

// MarginConfig is the platform margin added to every transaction when Enabled = true.
// The margin is deducted from the merchant's settlement (FeeBearer is always merchant).
// Per-method config allows different margin types for VA (typically flat) vs QRIS (typically %).
type MarginConfig struct {
	Enabled      bool         `toml:"enabled"`
	VA           MethodMargin `toml:"va"`
	QRIS         MethodMargin `toml:"qris"`
	Disbursement MethodMargin `toml:"disbursement"`
}

// MethodMargin defines the platform margin for a single payment method.
// Type must be "flat" or "percentage". Only the field matching Type is used.
type MethodMargin struct {
	Type       string  `toml:"type"`       // "flat" | "percentage"
	FlatIDR    int64   `toml:"flat_idr"`   // used when Type = "flat"
	Percentage float64 `toml:"percentage"` // e.g. 0.3 = 0.3%, used when Type = "percentage"
}

// Provide registers Config as a lazy singleton in the DI injector.
func Provide(i do.Injector) {
	do.Provide(i, func(i do.Injector) (*Config, error) {
		return Load()
	})
}

// Load reads and parses the config file. Exported for use by the migrate CLI command.
func Load() (*Config, error) {
	path := configPath()

	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, fmt.Errorf("decode config %q: %w", path, err)
	}
	return &cfg, nil
}

// configPath resolves the TOML file path.
// Precedence: CONFIG_PATH env var → ./config.toml
func configPath() string {
	if p := os.Getenv("CONFIG_PATH"); p != "" {
		return p
	}
	return ".config.toml"
}
