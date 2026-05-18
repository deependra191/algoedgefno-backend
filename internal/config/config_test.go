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
				JWTSecret:      testJWTSecret,
				AppSecretToken: testAppSecretToken,
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

func TestValidateStartupIdentity_StagingRejectsEmptySecrets(t *testing.T) {
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
			name: "empty jwt secret",
			mutate: func(cfg *Config) {
				cfg.JWTSecret = ""
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

func TestValidateStartupIdentity_ProductionRejectsEmptySecrets(t *testing.T) {
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

func TestNewFromEnv_AppEnvRejectsAliasesAndUnknownValues(t *testing.T) {
	for _, appEnv := range []string{"prod", "stage", "dev", "local", "testing", "unknown", "DEVELOPMENT", "Production"} {
		t.Run(appEnv, func(t *testing.T) {
			_, err := newFromEnv(mapLookup(map[string]string{"APP_ENV": appEnv}))
			if err == nil {
				t.Fatalf("newFromEnv() error = nil for APP_ENV=%q, want error", appEnv)
			}
		})
	}
}

func TestNewFromEnv_AppEnvCanonicalValuesAccepted(t *testing.T) {
	for _, appEnv := range []string{"production", "staging", "development", "test"} {
		t.Run(appEnv, func(t *testing.T) {
			_, err := newFromEnv(mapLookup(map[string]string{
				"APP_ENV":          appEnv,
				"JWT_SECRET":       testJWTSecret,
				"APP_SECRET_TOKEN": testAppSecretToken,
				"DB_USER":          "algoedge_app",
				"DB_PASSWORD":      testDBPassword,
				"DB_NAME":          "algoedgefno",
			}))
			if err != nil {
				t.Fatalf("newFromEnv() error = %v for APP_ENV=%q", err, appEnv)
			}
		})
	}
}

func TestNewFromEnv_MissingSecretsFails(t *testing.T) {
	tests := []struct {
		name string
		env  func() map[string]string
	}{
		{
			name: "missing JWT_SECRET",
			env: func() map[string]string {
				m := minDevEnv()
				delete(m, "JWT_SECRET")
				return m
			},
		},
		{
			name: "blank JWT_SECRET",
			env: func() map[string]string {
				m := minDevEnv()
				m["JWT_SECRET"] = "   "
				return m
			},
		},
		{
			name: "missing APP_SECRET_TOKEN",
			env: func() map[string]string {
				m := minDevEnv()
				delete(m, "APP_SECRET_TOKEN")
				return m
			},
		},
		{
			name: "blank APP_SECRET_TOKEN",
			env: func() map[string]string {
				m := minDevEnv()
				m["APP_SECRET_TOKEN"] = "   "
				return m
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newFromEnv(mapLookup(tt.env()))
			if err == nil {
				t.Fatal("newFromEnv() error = nil, want error")
			}
		})
	}
}

func TestNewFromEnv_WhitespaceDBFieldsFails(t *testing.T) {
	for _, key := range []string{"DB_USER", "DB_PASSWORD", "DB_NAME"} {
		t.Run(key, func(t *testing.T) {
			env := minDevEnv()
			env[key] = "   "
			_, err := newFromEnv(mapLookup(env))
			if err == nil {
				t.Fatalf("newFromEnv() error = nil for whitespace-only %s", key)
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

func TestNewFromEnv_KillSwitchDefaults(t *testing.T) {
	cfg, err := newFromEnv(mapLookup(minDevEnv()))
	if err != nil {
		t.Fatalf("newFromEnv() error = %v", err)
	}
	if !cfg.BacktestEnabled {
		t.Error("BacktestEnabled default should be true")
	}
	if cfg.BacktestMaxDays != defaultBacktestMaxDays {
		t.Errorf("BacktestMaxDays default = %d, want %d", cfg.BacktestMaxDays, defaultBacktestMaxDays)
	}
	if cfg.BacktestMaxCandles != defaultBacktestMaxCandles {
		t.Errorf("BacktestMaxCandles default = %d, want %d", cfg.BacktestMaxCandles, defaultBacktestMaxCandles)
	}
	if !cfg.SyncEnabled {
		t.Error("SyncEnabled default should be true")
	}
	if len(cfg.TrustedProxies) != 0 {
		t.Errorf("TrustedProxies default = %v, want empty", cfg.TrustedProxies)
	}
}

func TestNewFromEnv_KillSwitchesCanBeOverridden(t *testing.T) {
	env := minDevEnv()
	env["BACKTEST_ENABLED"] = "false"
	env["BACKTEST_MAX_DAYS"] = "90"
	env["BACKTEST_MAX_CANDLES"] = "5000"
	env["SYNC_ENABLED"] = "false"

	cfg, err := newFromEnv(mapLookup(env))
	if err != nil {
		t.Fatalf("newFromEnv() error = %v", err)
	}
	if cfg.BacktestEnabled {
		t.Error("BacktestEnabled should be false")
	}
	if cfg.BacktestMaxDays != 90 {
		t.Errorf("BacktestMaxDays = %d, want 90", cfg.BacktestMaxDays)
	}
	if cfg.BacktestMaxCandles != 5000 {
		t.Errorf("BacktestMaxCandles = %d, want 5000", cfg.BacktestMaxCandles)
	}
	if cfg.SyncEnabled {
		t.Error("SyncEnabled should be false")
	}
}

func TestNewFromEnv_ZeroDisablesLimits(t *testing.T) {
	env := minDevEnv()
	env["BACKTEST_MAX_DAYS"] = "0"
	env["BACKTEST_MAX_CANDLES"] = "0"

	cfg, err := newFromEnv(mapLookup(env))
	if err != nil {
		t.Fatalf("newFromEnv() error = %v", err)
	}
	if cfg.BacktestMaxDays != 0 {
		t.Errorf("BacktestMaxDays = %d, want 0 (no limit)", cfg.BacktestMaxDays)
	}
	if cfg.BacktestMaxCandles != 0 {
		t.Errorf("BacktestMaxCandles = %d, want 0 (no limit)", cfg.BacktestMaxCandles)
	}
}

func TestNewFromEnv_InvalidKillSwitchValuesFail(t *testing.T) {
	tests := []struct {
		name string
		key  string
		val  string
	}{
		{"non-boolean BACKTEST_ENABLED", "BACKTEST_ENABLED", "yes"},
		{"non-integer BACKTEST_MAX_DAYS", "BACKTEST_MAX_DAYS", "one-year"},
		{"negative BACKTEST_MAX_DAYS", "BACKTEST_MAX_DAYS", "-1"},
		{"non-integer BACKTEST_MAX_CANDLES", "BACKTEST_MAX_CANDLES", "many"},
		{"non-boolean SYNC_ENABLED", "SYNC_ENABLED", "1.0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := minDevEnv()
			env[tt.key] = tt.val
			_, err := newFromEnv(mapLookup(env))
			if err == nil {
				t.Fatalf("newFromEnv() error = nil, want error for %s=%q", tt.key, tt.val)
			}
		})
	}
}

func TestNewFromEnv_ParsesTrustedProxies(t *testing.T) {
	env := minDevEnv()
	env["TRUSTED_PROXIES"] = "172.16.0.0/12, 10.0.0.0/8"

	cfg, err := newFromEnv(mapLookup(env))
	if err != nil {
		t.Fatalf("newFromEnv() error = %v", err)
	}

	assertStringSlice(t, cfg.TrustedProxies, []string{"172.16.0.0/12", "10.0.0.0/8"})
}

func assertStringSlice(t *testing.T, got, want []string) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("slice length = %d, want %d: got %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("slice[%d] = %q, want %q; got %v", i, got[i], want[i], got)
		}
	}
}

func TestNewFromEnv_DBSSLRequired_DefaultsToTrue(t *testing.T) {
	cfg, err := newFromEnv(mapLookup(minDevEnv()))
	if err != nil {
		t.Fatalf("newFromEnv() error = %v", err)
	}
	if !cfg.DBSSLRequired {
		t.Fatal("DBSSLRequired = false when DB_SSL_REQUIRED is unset; want true (fail-closed default)")
	}
}

func TestNewFromEnv_DBSSLRequired_HonorsEnvVar(t *testing.T) {
	tests := []struct {
		val  string
		want bool
	}{
		{"true", true},
		{"false", false},
		{"TRUE", true},
		{"FALSE", false},
		{"1", true},
		{"0", false},
	}
	for _, tt := range tests {
		t.Run(tt.val, func(t *testing.T) {
			env := minDevEnv()
			env["DB_SSL_REQUIRED"] = tt.val

			cfg, err := newFromEnv(mapLookup(env))
			if err != nil {
				t.Fatalf("newFromEnv() error = %v for DB_SSL_REQUIRED=%q", err, tt.val)
			}
			if cfg.DBSSLRequired != tt.want {
				t.Fatalf("DBSSLRequired = %v, want %v for DB_SSL_REQUIRED=%q", cfg.DBSSLRequired, tt.want, tt.val)
			}
		})
	}
}

func TestNewFromEnv_DBSSLRequired_RejectsInvalidValue(t *testing.T) {
	env := minDevEnv()
	env["DB_SSL_REQUIRED"] = "yes-please"

	_, err := newFromEnv(mapLookup(env))
	if err == nil {
		t.Fatal("newFromEnv() error = nil, want error for non-boolean DB_SSL_REQUIRED")
	}
}
