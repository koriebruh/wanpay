package config

import (
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
	"github.com/samber/do/v2"
)

type Config struct {
	App       AppConfig       `toml:"app"`
	Database  DatabaseConfig  `toml:"database"`
	Redis     RedisConfig     `toml:"redis"`
	Logger    LoggerConfig    `toml:"logger"`
	Provider  ProviderConfig  `toml:"provider"`
	OTEL      OTELConfig      `toml:"otel"`
	Fee       FeeConfig       `toml:"fee"`
	TaskQueue TaskQueueConfig `toml:"taskqueue"`
	Admin     AdminConfig     `toml:"admin"`
}

type AdminConfig struct {
	JWTSecret            string `toml:"jwt_secret"`              // HMAC-SHA256 key for signing JWT tokens
	AccessTokenTTLHours  int    `toml:"access_token_ttl_hours"`  // default: 8
	RefreshTokenTTLHours int    `toml:"refresh_token_ttl_hours"` // default: 168 (7 days)
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

type TaskQueueConfig struct {
	Enabled     bool           `toml:"enabled"`
	Concurrency int            `toml:"concurrency"` // max concurrent workers; default 10
	Treasury    TreasuryConfig `toml:"treasury"`
}

type TreasuryConfig struct {
	CheckCron                string `toml:"check_cron"`                  // cron schedule for balance check
	TopupThresholdIDR        int64  `toml:"topup_threshold_idr"`         // trigger topup when provider balance < this
	TopupAmountIDR           int64  `toml:"topup_amount_idr"`            // amount to topup when threshold hit
	LargeCashoutThresholdIDR int64  `toml:"large_cashout_threshold_idr"` // single cashout that triggers immediate topup check
}

type ProviderConfig struct {
	Midtrans       MidtransConfig       `toml:"midtrans"`
	Xendit         XenditConfig         `toml:"xendit"`
	Doku           DokuConfig           `toml:"doku"`
	IPaymu         IPaymuConfig         `toml:"ipaymu"`
	CircuitBreaker CircuitBreakerConfig `toml:"circuit_breaker"`
}


type CircuitBreakerConfig struct {
	MaxRequests         uint32 `toml:"max_requests"`         // max requests in half-open state
	IntervalSeconds     int    `toml:"interval_seconds"`     // rolling window for failure counting
	TimeoutSeconds      int    `toml:"timeout_seconds"`      // seconds to stay open before half-open
	ConsecutiveFailures uint32 `toml:"consecutive_failures"` // failures before opening
}

type MidtransConfig struct {
	Enabled      bool     `toml:"enabled"`
	ServerKey    string   `toml:"server_key"`
	ClientKey    string   `toml:"client_key"`
	IsProduction bool     `toml:"is_production"`
	AllowedIPs   []string `toml:"allowed_ips"` // empty = accept all; set to Midtrans official IPs in production
}

type XenditConfig struct {
	Enabled      bool     `toml:"enabled"`
	SecretKey    string   `toml:"secret_key"`
	WebhookToken string   `toml:"webhook_token"`
	CallbackURL  string   `toml:"callback_url"`
	AllowedIPs   []string `toml:"allowed_ips"` // empty = accept all; set to Xendit official IPs in production
}

type DokuConfig struct {
	Enabled       bool     `toml:"enabled"`
	ClientID      string   `toml:"client_id"`
	SecretKey     string   `toml:"secret_key"`
	APIKey        string   `toml:"api_key"`
	PrivateKeyPEM string   `toml:"private_key"`
	IsProduction  bool     `toml:"is_production"`
	AllowedIPs    []string `toml:"allowed_ips"` // empty = accept all; set to DOKU official IPs in production
}

type IPaymuConfig struct {
	Enabled       bool     `toml:"enabled"`
	APIKey        string   `toml:"api_key"`
	VA            string   `toml:"va"`
	IsProduction  bool     `toml:"is_production"`
	NotifyURL     string   `toml:"notify_url"`
	WebhookSecret string   `toml:"webhook_secret"`
	AllowedIPs    []string `toml:"allowed_ips"` // empty = accept all; set to iPaymu official IPs in production
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
