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

// minDevEnv returns the minimal env map for a development environment, avoiding repetition of
// required-field boilerplate in tests that focus on a single behaviour.
func minDevEnv() map[string]string {
	return map[string]string{
		"APP_ENV":          "development",
		"JWT_SECRET":       testJWTSecret,
		"APP_SECRET_TOKEN": testAppSecretToken,
		"DB_USER":          "algoedge_dev",
		"DB_PASSWORD":      testDBPassword,
		"DB_NAME":          "algoedgefno_dev",
	}
}

func TestValidateStartupIdentity_AllowsValidConfigs(t *testing.T) {
	tests := []struct {
		name string
		cfg  *Config
	}{
		{
			name: "development allows any credentials",
			cfg: &Config{
				Env:            EnvDevelopment,
				DBHost:         defaultDBHost,
				DBUser:         "local_dev_user",
				DBPass:         "local_dev_pass",
				DBName:         "local_dev_db",
				JWTSecret:      exampleSecretPlaceholder,
				AppSecretToken: exampleSecretPlaceholder,
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

func TestValidateStartupIdentity_ProductionRequiresMarkerInDBNameAndUser(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
	}{
		{
			name: "generic DB name and user",
			mutate: func(cfg *Config) {
				cfg.DBName = "algoedgefno"
				cfg.DBUser = "algoedgefno_app"
			},
		},
		{
			name: "generic DB name with prod user",
			mutate: func(cfg *Config) {
				cfg.DBName = "algoedgefno_live"
			},
		},
		{
			name: "prod DB name with generic user",
			mutate: func(cfg *Config) {
				cfg.DBUser = "algoedgefno_app"
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

func TestValidateStartupIdentity_StagingRequiresMarkerInDBNameAndUser(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
	}{
		{
			name: "empty DB name",
			mutate: func(cfg *Config) {
				cfg.DBName = ""
			},
		},
		{
			name: "empty DB user",
			mutate: func(cfg *Config) {
				cfg.DBUser = ""
			},
		},
		{
			name: "generic DB name and user",
			mutate: func(cfg *Config) {
				cfg.DBName = "algoedgefno"
				cfg.DBUser = "algoedgefno_app"
			},
		},
		{
			name: "staging DB name with generic user",
			mutate: func(cfg *Config) {
				cfg.DBUser = "algoedgefno_app"
			},
		},
		{
			name: "generic DB name with staging user",
			mutate: func(cfg *Config) {
				cfg.DBName = "algoedgefno"
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

func TestValidateStartupIdentity_StagingRejectsDefaultOrEmptySecrets(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
	}{
		{
			name: "empty app secret token",
			mutate: func(cfg *Config) {
				cfg.AppSecretToken = ""
			},
		},
		{
			name: "example app secret token",
			mutate: func(cfg *Config) {
				cfg.AppSecretToken = exampleSecretPlaceholder
			},
		},
		{
			name: "empty jwt secret",
			mutate: func(cfg *Config) {
				cfg.JWTSecret = ""
			},
		},
		{
			name: "example jwt secret",
			mutate: func(cfg *Config) {
				cfg.JWTSecret = exampleSecretPlaceholder
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

func TestValidateStartupIdentity_GenericDBHostAllowedWhenNameAndUserAreEnvironmentSpecific(t *testing.T) {
	tests := []struct {
		name string
		cfg  *Config
	}{
		{
			name: "production with generic host",
			cfg: func() *Config {
				cfg := validProductionConfig()
				cfg.DBHost = "postgres"
				return cfg
			}(),
		},
		{
			name: "staging with generic host",
			cfg: func() *Config {
				cfg := validStagingConfig()
				cfg.DBHost = "postgres"
				return cfg
			}(),
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
			name: "example app secret token",
			mutate: func(cfg *Config) {
				cfg.AppSecretToken = exampleSecretPlaceholder
			},
		},
		{
			name: "empty app secret token",
			mutate: func(cfg *Config) {
				cfg.AppSecretToken = ""
			},
		},
		{
			name: "example jwt secret",
			mutate: func(cfg *Config) {
				cfg.JWTSecret = exampleSecretPlaceholder
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

func TestValidateStartupIdentity_ProductionRejectsNonProdDBIdentityInUserAndName(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
	}{
		{
			name: "generic database user without prod marker",
			mutate: func(cfg *Config) {
				cfg.DBUser = "algoedge_app"
			},
		},
		{
			name: "generic database name without prod marker",
			mutate: func(cfg *Config) {
				cfg.DBName = "algoedgefno"
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
		{name: "empty migrations path", migrationsPath: ""},
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

func TestNewFromEnv_MissingOrBlankAppEnvFails(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
	}{
		{
			name: "APP_ENV missing",
			env:  map[string]string{},
		},
		{
			name: "APP_ENV blank",
			env:  map[string]string{"APP_ENV": "   "},
		},
		{
			name: "ENV=development without APP_ENV",
			env:  map[string]string{"ENV": "development"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newFromEnv(mapLookup(tt.env))
			if err == nil {
				t.Fatal("newFromEnv() error = nil, want error for missing APP_ENV")
			}
		})
	}
}

func TestNewFromEnv_InvalidAppEnvFails(t *testing.T) {
	_, err := newFromEnv(mapLookup(map[string]string{
		"APP_ENV": "unknown",
	}))
	if err == nil {
		t.Fatal("newFromEnv() error = nil, want error for invalid APP_ENV")
	}
}

func TestNewFromEnv_MissingSecretsFails(t *testing.T) {
	tests := []struct {
		name   string
		remove string
	}{
		{"missing JWT_SECRET", "JWT_SECRET"},
		{"blank JWT_SECRET", ""},
		{"missing APP_SECRET_TOKEN", "APP_SECRET_TOKEN"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := minDevEnv()
			if tt.remove != "" {
				delete(env, tt.remove)
			} else {
				env["JWT_SECRET"] = ""
			}
			_, err := newFromEnv(mapLookup(env))
			if err == nil {
				t.Fatalf("newFromEnv() error = nil, want error")
			}
		})
	}
}

func TestNewFromEnv_MissingDBFieldsFails(t *testing.T) {
	tests := []struct {
		name   string
		remove string
	}{
		{"missing DB_USER", "DB_USER"},
		{"missing DB_PASSWORD", "DB_PASSWORD"},
		{"missing DB_NAME", "DB_NAME"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := minDevEnv()
			delete(env, tt.remove)
			_, err := newFromEnv(mapLookup(env))
			if err == nil {
				t.Fatalf("newFromEnv() error = nil, want error for missing %s", tt.remove)
			}
		})
	}
}

func TestNewFromEnv_DatabaseURLSatisfiesDBFields(t *testing.T) {
	cfg, err := newFromEnv(mapLookup(map[string]string{
		"APP_ENV":          "development",
		"JWT_SECRET":       testJWTSecret,
		"APP_SECRET_TOKEN": testAppSecretToken,
		"DATABASE_URL":     "postgresql://dev_user:dev_pass@dev-db:5433/algoedgefno_dev?sslmode=disable",
	}))
	if err != nil {
		t.Fatalf("newFromEnv() error = %v", err)
	}
	if cfg.DBUser != "dev_user" {
		t.Fatalf("DBUser = %q, want dev_user", cfg.DBUser)
	}
	if cfg.DBPass != "dev_pass" {
		t.Fatalf("DBPass = %q, want dev_pass", cfg.DBPass)
	}
	if cfg.DBName != "algoedgefno_dev" {
		t.Fatalf("DBName = %q, want algoedgefno_dev", cfg.DBName)
	}
	if cfg.DBHost != "dev-db" {
		t.Fatalf("DBHost = %q, want dev-db", cfg.DBHost)
	}
	if cfg.DBPort != "5433" {
		t.Fatalf("DBPort = %q, want 5433", cfg.DBPort)
	}
}

func TestNewFromEnvParsesAPPEnvAndDatabaseURL(t *testing.T) {
	cfg, err := newFromEnv(mapLookup(map[string]string{
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
				"APP_ENV":          tt.env,
				"JWT_SECRET":       testJWTSecret,
				"APP_SECRET_TOKEN": testAppSecretToken,
				"DB_USER":          "algoedge_app",
				"DB_PASSWORD":      testDBPassword,
				"DB_NAME":          "algoedgefno",
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
