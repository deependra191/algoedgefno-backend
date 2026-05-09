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

	envVarAppEnv          = "APP_ENV"
	envVarEnv             = "ENV"
	envVarAutoMigrate     = "AUTO_MIGRATE"
	envVarDatabaseURL     = "DATABASE_URL"
	defaultJWTSecret      = "change-this-in-production"
	defaultAppSecretToken = "change-this-in-production"
	defaultDBHost         = "localhost"
	defaultDBPort         = "5432"
	defaultDBUser         = "algoedge"
	defaultDBPassword     = "algoedge"
	defaultDBName         = "algoedgefno"
	defaultMigrationsPath = "file://migrations"
)

// Environment identifies the runtime environment selected by ENV or APP_ENV.
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
	env, err := parseEnvironment(getEnvFrom(lookup, envVarAppEnv, getEnvFrom(lookup, envVarEnv, string(EnvDevelopment))))
	if err != nil {
		return nil, err
	}

	autoMigrate, err := getBoolEnvFrom(lookup, envVarAutoMigrate, defaultAutoMigrate(env))
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		Port:           getEnvFrom(lookup, "PORT", "8080"),
		DatabaseURL:    getEnvFrom(lookup, envVarDatabaseURL, ""),
		DBHost:         getEnvFrom(lookup, "DB_HOST", defaultDBHost),
		DBPort:         getEnvFrom(lookup, "DB_PORT", defaultDBPort),
		DBUser:         getEnvFrom(lookup, "DB_USER", ""),
		DBPass:         getEnvFrom(lookup, "DB_PASSWORD", ""),
		DBName:         getEnvFrom(lookup, "DB_NAME", ""),
		JWTSecret:      getEnvFrom(lookup, "JWT_SECRET", defaultJWTSecret),
		AppSecretToken: getEnvFrom(lookup, "APP_SECRET_TOKEN", ""),
		Env:            env,
		MigrationsPath: getEnvFrom(lookup, "MIGRATIONS_PATH", defaultMigrationsPath),
		AutoMigrate:    autoMigrate,
		NSEUserAgent:   getEnvFrom(lookup, "NSE_USER_AGENT", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"),
		NSEAcceptHTML:  getEnvFrom(lookup, "NSE_ACCEPT_HTML", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"),
	}

	if cfg.DatabaseURL != "" {
		if err := cfg.applyDatabaseURL(); err != nil {
			return nil, err
		}
	}

	return cfg, nil
}

func (cfg *Config) validateProductionIdentity() error {
	if cfg.AppSecretToken == "" || cfg.AppSecretToken == defaultAppSecretToken {
		return fmt.Errorf("production APP_SECRET_TOKEN must be set to a non-example value")
	}
	if cfg.JWTSecret == "" || cfg.JWTSecret == defaultJWTSecret {
		return fmt.Errorf("production JWT_SECRET must be set to a non-example value")
	}
	if cfg.DBUser == "" || cfg.DBPass == "" || cfg.DBName == "" {
		return fmt.Errorf("production DB_USER, DB_PASSWORD, and DB_NAME must be set")
	}
	if cfg.DBUser == defaultDBUser {
		return fmt.Errorf("production DB_USER must not use the local example default")
	}
	if cfg.DBPass == defaultDBPassword {
		return fmt.Errorf("production DB_PASSWORD must not use the local example default")
	}
	if cfg.DBName == defaultDBName {
		return fmt.Errorf("production DB_NAME must not use the local example default")
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
	if cfg.MigrationsPath != "" && !isAbsoluteMigrationsPath(cfg.MigrationsPath) {
		return fmt.Errorf("production MIGRATIONS_PATH must be an absolute path")
	}
	return nil
}

func (cfg *Config) validateStagingIdentity() error {
	if cfg.AppSecretToken == "" || cfg.AppSecretToken == defaultAppSecretToken {
		return fmt.Errorf("staging APP_SECRET_TOKEN must be set to a non-example value")
	}
	if cfg.JWTSecret == "" || cfg.JWTSecret == defaultJWTSecret {
		return fmt.Errorf("staging JWT_SECRET must be set to a non-example value")
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
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "production", "prod":
		return EnvProduction, nil
	case "staging", "stage":
		return EnvStaging, nil
	case "development", "dev", "local":
		return EnvDevelopment, nil
	case "test", "testing":
		return EnvTest, nil
	default:
		return "", fmt.Errorf("unsupported environment %q", raw)
	}
}

func defaultAutoMigrate(env Environment) bool {
	return env == EnvDevelopment || env == EnvTest
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
