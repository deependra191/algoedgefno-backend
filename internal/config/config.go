package config

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

const (
	EnvProduction  Environment = "production"
	EnvStaging     Environment = "staging"
	EnvDevelopment Environment = "development"
	EnvTest        Environment = "test"

	envVarAppEnv         = "APP_ENV"
	envVarAutoMigrate    = "AUTO_MIGRATE"
	envVarDatabaseURL    = "DATABASE_URL"
	envVarPort           = "PORT"
	envVarDBHost         = "DB_HOST"
	envVarDBPort         = "DB_PORT"
	envVarDBUser         = "DB_USER"
	envVarDBPassword     = "DB_PASSWORD"
	envVarDBName         = "DB_NAME"
	envVarJWTSecret      = "JWT_SECRET"
	envVarAppSecretToken = "APP_SECRET_TOKEN"
	envVarMigrationsPath = "MIGRATIONS_PATH"
	envVarNSEUserAgent   = "NSE_USER_AGENT"
	envVarNSEAcceptHTML  = "NSE_ACCEPT_HTML"

	envVarBacktestEnabled    = "BACKTEST_ENABLED"
	envVarBacktestMaxDays    = "BACKTEST_MAX_DAYS"
	envVarBacktestMaxCandles = "BACKTEST_MAX_CANDLES"
	envVarSyncEnabled        = "SYNC_ENABLED"
	envVarLiveTickEnabled    = "LIVE_TICK_ENABLED"

	// Operational defaults — safe to use as config fallbacks.
	defaultPort           = "8080"
	defaultDBHost         = "localhost"
	defaultDBPort         = "5432"
	defaultMigrationsPath = "file://migrations"
	defaultNSEUserAgent   = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"
	defaultNSEAcceptHTML  = "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"

	// defaultBacktestMaxDays and defaultBacktestMaxCandles are conservative production limits.
	// Staging env files should set lower values. 0 means no limit.
	defaultBacktestEnabled    = true
	defaultBacktestMaxDays    = 730
	defaultBacktestMaxCandles = 20000
	defaultSyncEnabled        = true
	defaultLiveTickEnabled    = false
)

// Environment identifies the runtime environment selected by APP_ENV.
type Environment string

// Config contains environment-backed application configuration.
type Config struct {
	Port           string
	DatabaseURL    string
	DBHost         string
	DBPort         string
	DBUser         string
	DBPass         string
	DBName         string
	JWTSecret      string
	AppSecretToken string
	Env            Environment
	MigrationsPath string
	AutoMigrate    bool

	// NSE provider HTTP fingerprinting headers.
	// NSE blocks requests that don't look like a real browser.
	// Override via NSE_USER_AGENT / NSE_ACCEPT_HTML if NSE tightens their checks.
	NSEUserAgent  string
	NSEAcceptHTML string

	// Kill switches and cost-control limits.
	// BacktestEnabled gates all backtest submissions; set false to disable globally.
	// BacktestMaxDays is the maximum date range in days; 0 means no limit.
	// BacktestMaxCandles is the maximum candle count per run (signal or trade, whichever is larger); 0 means no limit.
	// SyncEnabled gates NSE EOD sync; set false to disable without code changes.
	// LiveTickEnabled is a no-op placeholder for Phase 3 live tick streaming.
	BacktestEnabled    bool
	BacktestMaxDays    int
	BacktestMaxCandles int
	SyncEnabled        bool
	LiveTickEnabled    bool
}

// Load reads environment-backed configuration, including a local .env file when present.
func Load() *Config {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	cfg, err := newFromEnv(os.LookupEnv)
	if err != nil {
		log.Fatal(err)
	}
	return cfg
}

// ValidateStartupIdentity checks environment and database identity guardrails before startup.
func (cfg *Config) ValidateStartupIdentity() error {
	if cfg == nil {
		return fmt.Errorf("config is required")
	}

	switch cfg.Env {
	case EnvProduction:
		return cfg.validateProductionIdentity()
	case EnvStaging:
		return cfg.validateStagingIdentity()
	case EnvDevelopment, EnvTest:
		return nil
	default:
		return fmt.Errorf("unsupported environment %q", cfg.Env)
	}
}

// ShouldRunMigrations reports whether app startup should automatically apply migrations.
func (cfg *Config) ShouldRunMigrations() bool {
	return cfg != nil && cfg.Env != EnvProduction && cfg.AutoMigrate
}

func newFromEnv(lookup func(string) (string, bool)) (*Config, error) {
	rawEnv, err := requireEnvFrom(lookup, envVarAppEnv)
	if err != nil {
		return nil, err
	}
	env, err := parseEnvironment(rawEnv)
	if err != nil {
		return nil, err
	}

	autoMigrate, err := getBoolEnvFrom(lookup, envVarAutoMigrate, defaultAutoMigrate(env))
	if err != nil {
		return nil, err
	}

	jwtSecret, err := requireEnvFrom(lookup, envVarJWTSecret)
	if err != nil {
		return nil, err
	}

	appSecretToken, err := requireEnvFrom(lookup, envVarAppSecretToken)
	if err != nil {
		return nil, err
	}

	backtestEnabled, err := getBoolEnvFrom(lookup, envVarBacktestEnabled, defaultBacktestEnabled)
	if err != nil {
		return nil, err
	}
	backtestMaxDays, err := getIntEnvFrom(lookup, envVarBacktestMaxDays, defaultBacktestMaxDays)
	if err != nil {
		return nil, err
	}
	backtestMaxCandles, err := getIntEnvFrom(lookup, envVarBacktestMaxCandles, defaultBacktestMaxCandles)
	if err != nil {
		return nil, err
	}
	syncEnabled, err := getBoolEnvFrom(lookup, envVarSyncEnabled, defaultSyncEnabled)
	if err != nil {
		return nil, err
	}
	liveTickEnabled, err := getBoolEnvFrom(lookup, envVarLiveTickEnabled, defaultLiveTickEnabled)
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		Port:               getEnvFrom(lookup, envVarPort, defaultPort),
		DatabaseURL:        getEnvFrom(lookup, envVarDatabaseURL, ""),
		DBHost:             getEnvFrom(lookup, envVarDBHost, defaultDBHost),
		DBPort:             getEnvFrom(lookup, envVarDBPort, defaultDBPort),
		DBUser:             getEnvFrom(lookup, envVarDBUser, ""),
		DBPass:             getEnvFrom(lookup, envVarDBPassword, ""),
		DBName:             getEnvFrom(lookup, envVarDBName, ""),
		JWTSecret:          jwtSecret,
		AppSecretToken:     appSecretToken,
		Env:                env,
		MigrationsPath:     getEnvFrom(lookup, envVarMigrationsPath, defaultMigrationsPath),
		AutoMigrate:        autoMigrate,
		NSEUserAgent:       getEnvFrom(lookup, envVarNSEUserAgent, defaultNSEUserAgent),
		NSEAcceptHTML:      getEnvFrom(lookup, envVarNSEAcceptHTML, defaultNSEAcceptHTML),
		BacktestEnabled:    backtestEnabled,
		BacktestMaxDays:    backtestMaxDays,
		BacktestMaxCandles: backtestMaxCandles,
		SyncEnabled:        syncEnabled,
		LiveTickEnabled:    liveTickEnabled,
	}

	if cfg.DatabaseURL != "" {
		if err := cfg.applyDatabaseURL(); err != nil {
			return nil, err
		}
	}

	if strings.TrimSpace(cfg.DBUser) == "" {
		return nil, fmt.Errorf("%s is required (or supply DATABASE_URL)", envVarDBUser)
	}
	if strings.TrimSpace(cfg.DBPass) == "" {
		return nil, fmt.Errorf("%s is required (or supply DATABASE_URL)", envVarDBPassword)
	}
	if strings.TrimSpace(cfg.DBName) == "" {
		return nil, fmt.Errorf("%s is required (or supply DATABASE_URL)", envVarDBName)
	}

	return cfg, nil
}

// validateProductionIdentity enforces production-only guardrails: non-empty secrets,
// mandatory production markers in DB user/name, no non-prod markers anywhere,
// no auto-migrate, non-empty absolute migrations path.
func (cfg *Config) validateProductionIdentity() error {
	if cfg.AppSecretToken == "" {
		return fmt.Errorf("production APP_SECRET_TOKEN must be set")
	}
	if cfg.JWTSecret == "" {
		return fmt.Errorf("production JWT_SECRET must be set")
	}
	if matchesAnyIdentityPart([]string{cfg.DBName, cfg.DBUser, cfg.DBHost}, nonProductionIdentityMarkers()) {
		return fmt.Errorf("production database identity must not match staging, development, or test")
	}
	if !containsAnyMarker(cfg.DBName, productionIdentityMarkers()) {
		return fmt.Errorf("production DB_NAME must contain a production marker (prod or production)")
	}
	if !containsAnyMarker(cfg.DBUser, productionIdentityMarkers()) {
		return fmt.Errorf("production DB_USER must contain a production marker (prod or production)")
	}
	if cfg.AutoMigrate {
		return fmt.Errorf("production AUTO_MIGRATE must be disabled")
	}
	if cfg.MigrationsPath == "" || !isAbsoluteMigrationsPath(cfg.MigrationsPath) {
		return fmt.Errorf("production MIGRATIONS_PATH must be a non-empty absolute path")
	}
	return nil
}

// validateStagingIdentity enforces staging guardrails: non-empty secrets,
// mandatory staging markers in DB user/name, no production markers anywhere.
func (cfg *Config) validateStagingIdentity() error {
	if cfg.AppSecretToken == "" {
		return fmt.Errorf("staging APP_SECRET_TOKEN must be set")
	}
	if cfg.JWTSecret == "" {
		return fmt.Errorf("staging JWT_SECRET must be set")
	}
	if matchesAnyIdentityPart([]string{cfg.DBName, cfg.DBUser, cfg.DBHost}, productionIdentityMarkers()) {
		return fmt.Errorf("staging database identity must not match production")
	}
	if !containsAnyMarker(cfg.DBName, stagingIdentityMarkers()) {
		return fmt.Errorf("staging DB_NAME must contain a staging marker (staging or stage)")
	}
	if !containsAnyMarker(cfg.DBUser, stagingIdentityMarkers()) {
		return fmt.Errorf("staging DB_USER must contain a staging marker (staging or stage)")
	}
	return nil
}

func parseEnvironment(raw string) (Environment, error) {
	env := Environment(strings.TrimSpace(raw))

	switch env {
	case EnvProduction, EnvStaging, EnvDevelopment, EnvTest:
		return env, nil
	default:
		return "", fmt.Errorf("APP_ENV %q is not valid: use one of production, staging, development, test", raw)
	}
}

func defaultAutoMigrate(env Environment) bool {
	return env == EnvDevelopment || env == EnvTest
}

func requireEnvFrom(lookup func(string) (string, bool), key string) (string, error) {
	v, ok := lookup(key)
	if !ok || strings.TrimSpace(v) == "" {
		return "", fmt.Errorf("%s is required", key)
	}
	return v, nil
}

func getBoolEnvFrom(lookup func(string) (string, bool), key string, fallback bool) (bool, error) {
	v, ok := lookup(key)
	if !ok || strings.TrimSpace(v) == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean", key)
	}
	return parsed, nil
}

func getIntEnvFrom(lookup func(string) (string, bool), key string, fallback int) (int, error) {
	v, ok := lookup(key)
	if !ok || strings.TrimSpace(v) == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || parsed < 0 {
		return 0, fmt.Errorf("%s must be a non-negative integer", key)
	}
	return parsed, nil
}

func (cfg *Config) applyDatabaseURL() error {
	u, err := url.Parse(cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("DATABASE_URL is invalid")
	}
	if u.Scheme != "postgres" && u.Scheme != "postgresql" {
		return fmt.Errorf("DATABASE_URL must use postgres or postgresql scheme")
	}

	if u.Hostname() != "" {
		cfg.DBHost = u.Hostname()
	}
	if u.Port() != "" {
		cfg.DBPort = u.Port()
	}
	if u.User != nil {
		cfg.DBUser = u.User.Username()
		if pass, ok := u.User.Password(); ok {
			cfg.DBPass = pass
		}
	}
	if name := strings.TrimPrefix(u.Path, "/"); name != "" {
		cfg.DBName = name
	}
	return nil
}

func nonProductionIdentityMarkers() []string {
	return []string{"staging", "stage", "development", "dev", "test", "testing"}
}

func productionIdentityMarkers() []string {
	return []string{"production", "prod"}
}

func stagingIdentityMarkers() []string {
	return []string{"staging", "stage"}
}

func matchesAnyIdentityPart(parts []string, markers []string) bool {
	for _, part := range parts {
		if containsAnyMarker(part, markers) {
			return true
		}
	}
	return false
}

func containsAnyMarker(part string, markers []string) bool {
	normalized := strings.ToLower(strings.TrimSpace(part))
	if normalized == "" {
		return false
	}
	for _, marker := range markers {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

func isAbsoluteMigrationsPath(path string) bool {
	if strings.HasPrefix(path, "file://") {
		u, err := url.Parse(path)
		if err != nil {
			return false
		}
		return u.Host == "" && filepath.IsAbs(u.Path)
	}
	return filepath.IsAbs(path)
}

func getEnvFrom(lookup func(string) (string, bool), key, fallback string) string {
	if v, ok := lookup(key); ok && v != "" {
		return v
	}
	return fallback
}
