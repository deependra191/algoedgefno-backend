package config

import (
	"strings"
	"testing"
)

const (
	testAppSecretToken = "test-app-secret-token-not-the-example"
	testJWTSecret      = "test-jwt-secret-not-the-example"
	testDBPassword     = "test-db-password-not-the-example"
)

func TestValidateStartupIdentity_AllowsValidConfigs(t *testing.T) {
	tests := []struct {
		name string
		cfg  *Config
	}{
		{
			name: "development local defaults",
			cfg: &Config{
				Env:            EnvDevelopment,
				DBHost:         defaultDBHost,
				DBUser:         defaultDBUser,
				DBPass:         defaultDBPassword,
				DBName:         defaultDBName,
				JWTSecret:      defaultJWTSecret,
				AppSecretToken: defaultAppSecretToken,
				MigrationsPath: defaultMigrationsPath,
				AutoMigrate:    true,
			},
		},
		{
			name: "staging",
			cfg: &Config{
				Env:            EnvStaging,
				DBHost:         "postgres-staging",
				DBUser:         "algoedgefno_staging_app",
				DBPass:         testDBPassword,
				DBName:         "algoedgefno_staging",
				JWTSecret:      testJWTSecret,
				AppSecretToken: testAppSecretToken,
				MigrationsPath: defaultMigrationsPath,
				AutoMigrate:    false,
			},
		},
		{
			name: "production",
			cfg: &Config{
				Env:            EnvProduction,
				DBHost:         "postgres-prod",
				DBUser:         "algoedgefno_prod_app",
				DBPass:         testDBPassword,
				DBName:         "algoedgefno_prod",
				JWTSecret:      testJWTSecret,
				AppSecretToken: testAppSecretToken,
				MigrationsPath: "file:///opt/algoedgefno/migrations",
				AutoMigrate:    false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.cfg.ValidateStartupIdentity(); err != nil {
				t.Fatalf("ValidateStartupIdentity() error = %v", err)
			}
		})
	}
}

func TestValidateStartupIdentity_ProductionRejectsNonProductionDBIdentity(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
	}{
		{
			name: "staging database name",
			mutate: func(cfg *Config) {
				cfg.DBName = "algoedgefno_staging"
			},
		},
		{
			name: "development database user",
			mutate: func(cfg *Config) {
				cfg.DBUser = "algoedgefno_dev_app"
			},
		},
		{
			name: "test database host",
			mutate: func(cfg *Config) {
				cfg.DBHost = "postgres-test"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validProductionConfig()
			tt.mutate(cfg)

			err := cfg.ValidateStartupIdentity()
			if err == nil {
				t.Fatal("ValidateStartupIdentity() error = nil")
			}
			assertErrorDoesNotLeakSensitiveValues(t, err, cfg)
		})
	}
}

func TestValidateStartupIdentity_StagingRejectsProductionDBIdentity(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
	}{
		{
			name: "production database name",
			mutate: func(cfg *Config) {
				cfg.DBName = "algoedgefno_production"
			},
		},
		{
			name: "prod database user",
			mutate: func(cfg *Config) {
				cfg.DBUser = "algoedgefno_prod_app"
			},
		},
		{
			name: "prod database host",
			mutate: func(cfg *Config) {
				cfg.DBHost = "postgres-prod"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validStagingConfig()
			tt.mutate(cfg)

			err := cfg.ValidateStartupIdentity()
			if err == nil {
				t.Fatal("ValidateStartupIdentity() error = nil")
			}
			assertErrorDoesNotLeakSensitiveValues(t, err, cfg)
		})
	}
}

func TestValidateStartupIdentity_ProductionRejectsDefaultSecrets(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
	}{
		{
			name: "default app secret token",
			mutate: func(cfg *Config) {
				cfg.AppSecretToken = defaultAppSecretToken
			},
		},
		{
			name: "empty app secret token",
			mutate: func(cfg *Config) {
				cfg.AppSecretToken = ""
			},
		},
		{
			name: "default jwt secret",
			mutate: func(cfg *Config) {
				cfg.JWTSecret = defaultJWTSecret
			},
		},
		{
			name: "empty jwt secret",
			mutate: func(cfg *Config) {
				cfg.JWTSecret = ""
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validProductionConfig()
			tt.mutate(cfg)

			err := cfg.ValidateStartupIdentity()
			if err == nil {
				t.Fatal("ValidateStartupIdentity() error = nil")
			}
			assertErrorDoesNotLeakSensitiveValues(t, err, cfg)
		})
	}
}

func TestValidateStartupIdentity_ProductionRejectsUnsafeDefaultDBCredentials(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
	}{
		{
			name: "default database user",
			mutate: func(cfg *Config) {
				cfg.DBUser = defaultDBUser
			},
		},
		{
			name: "default database password",
			mutate: func(cfg *Config) {
				cfg.DBPass = defaultDBPassword
			},
		},
		{
			name: "default database name",
			mutate: func(cfg *Config) {
				cfg.DBName = defaultDBName
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validProductionConfig()
			tt.mutate(cfg)

			err := cfg.ValidateStartupIdentity()
			if err == nil {
				t.Fatal("ValidateStartupIdentity() error = nil")
			}
			assertErrorDoesNotLeakSensitiveValues(t, err, cfg)
		})
	}
}

func TestValidateStartupIdentity_ProductionMigrationValidation(t *testing.T) {
	tests := []struct {
		name           string
		migrationsPath string
		autoMigrate    bool
	}{
		{name: "relative file URL", migrationsPath: defaultMigrationsPath},
		{name: "relative path", migrationsPath: "migrations"},
		{name: "auto migrate enabled", migrationsPath: "file:///opt/algoedgefno/migrations", autoMigrate: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validProductionConfig()
			cfg.MigrationsPath = tt.migrationsPath
			cfg.AutoMigrate = tt.autoMigrate

			err := cfg.ValidateStartupIdentity()
			if err == nil {
				t.Fatal("ValidateStartupIdentity() error = nil")
			}
			assertErrorDoesNotLeakSensitiveValues(t, err, cfg)
		})
	}
}

func TestNewFromEnvPrefersAPPEnvAndParsesDatabaseURL(t *testing.T) {
	cfg, err := newFromEnv(mapLookup(map[string]string{
		"ENV":              "production",
		"APP_ENV":          "staging",
		"DATABASE_URL":     "postgresql://staging_user:staging_password@staging-db:6543/algoedgefno_staging?sslmode=disable",
		"AUTO_MIGRATE":     "true",
		"JWT_SECRET":       testJWTSecret,
		"APP_SECRET_TOKEN": testAppSecretToken,
	}))
	if err != nil {
		t.Fatalf("newFromEnv() error = %v", err)
	}

	if cfg.Env != EnvStaging {
		t.Fatalf("Env = %q, want %q", cfg.Env, EnvStaging)
	}
	if cfg.DBHost != "staging-db" {
		t.Fatalf("DBHost = %q, want staging-db", cfg.DBHost)
	}
	if cfg.DBPort != "6543" {
		t.Fatalf("DBPort = %q, want 6543", cfg.DBPort)
	}
	if cfg.DBUser != "staging_user" {
		t.Fatalf("DBUser = %q, want staging_user", cfg.DBUser)
	}
	if cfg.DBPass != "staging_password" {
		t.Fatalf("DBPass = %q, want staging_password", cfg.DBPass)
	}
	if cfg.DBName != "algoedgefno_staging" {
		t.Fatalf("DBName = %q, want algoedgefno_staging", cfg.DBName)
	}
	if !cfg.AutoMigrate {
		t.Fatal("AutoMigrate = false, want true")
	}
}

func TestNewFromEnvAutoMigrateDefaults(t *testing.T) {
	tests := []struct {
		name string
		env  string
		want bool
	}{
		{name: "development", env: "development", want: true},
		{name: "test", env: "test", want: true},
		{name: "staging", env: "staging", want: false},
		{name: "production", env: "production", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := newFromEnv(mapLookup(map[string]string{
				"ENV": tt.env,
			}))
			if err != nil {
				t.Fatalf("newFromEnv() error = %v", err)
			}
			if cfg.AutoMigrate != tt.want {
				t.Fatalf("AutoMigrate = %v, want %v", cfg.AutoMigrate, tt.want)
			}
		})
	}
}

func validProductionConfig() *Config {
	return &Config{
		Env:            EnvProduction,
		DBHost:         "postgres-prod",
		DBUser:         "algoedgefno_prod_app",
		DBPass:         testDBPassword,
		DBName:         "algoedgefno_prod",
		JWTSecret:      testJWTSecret,
		AppSecretToken: testAppSecretToken,
		MigrationsPath: "file:///opt/algoedgefno/migrations",
		AutoMigrate:    false,
	}
}

func validStagingConfig() *Config {
	return &Config{
		Env:            EnvStaging,
		DBHost:         "postgres-staging",
		DBUser:         "algoedgefno_staging_app",
		DBPass:         testDBPassword,
		DBName:         "algoedgefno_staging",
		JWTSecret:      testJWTSecret,
		AppSecretToken: testAppSecretToken,
		MigrationsPath: defaultMigrationsPath,
		AutoMigrate:    false,
	}
}

func mapLookup(values map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		v, ok := values[key]
		return v, ok
	}
}

func assertErrorDoesNotLeakSensitiveValues(t *testing.T, err error, cfg *Config) {
	t.Helper()

	msg := err.Error()
	for _, sensitive := range []string{cfg.DatabaseURL, cfg.DBPass, cfg.AppSecretToken, cfg.JWTSecret} {
		if sensitive != "" && strings.Contains(msg, sensitive) {
			t.Fatalf("error leaked sensitive value %q in %q", sensitive, msg)
		}
	}
}
