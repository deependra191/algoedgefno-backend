package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	testJWTSecret  = "test-jwt-secret-not-the-example"
	testDBPassword = "test-db-password-not-the-example"
)

// minDevEnv returns the minimal env map for a development environment, avoiding repetition of
// required-field boilerplate in tests that focus on a single behaviour.
func minDevEnv() map[string]string {
	return map[string]string{
		"APP_ENV":     "development",
		"JWT_SECRET":  testJWTSecret,
		"DB_USER":     "algoedge_dev",
		"DB_PASSWORD": testDBPassword,
		"DB_NAME":     "algoedgefno_dev",
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
				"APP_ENV":     appEnv,
				"JWT_SECRET":  testJWTSecret,
				"DB_USER":     "algoedge_app",
				"DB_PASSWORD": testDBPassword,
				"DB_NAME":     "algoedgefno",
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
		"APP_ENV":      "development",
		"JWT_SECRET":   testJWTSecret,
		"DATABASE_URL": "postgresql://dev_user:dev_pass@dev-db:5433/algoedgefno_dev?sslmode=disable",
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
		"APP_ENV":      "staging",
		"DATABASE_URL": "postgresql://staging_user:staging_password@staging-db:6543/algoedgefno_staging?sslmode=disable",
		"AUTO_MIGRATE": "true",
		"JWT_SECRET":   testJWTSecret,
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
				"APP_ENV":     tt.env,
				"JWT_SECRET":  testJWTSecret,
				"DB_USER":     "algoedge_app",
				"DB_PASSWORD": testDBPassword,
				"DB_NAME":     "algoedgefno",
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
	for _, sensitive := range []string{cfg.DatabaseURL, cfg.DBPass, cfg.JWTSecret} {
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

func TestNewFromEnv_ParsesAllowedFirebaseUIDs(t *testing.T) {
	// ALLOWED_FIREBASE_UIDS is comma-separated; whitespace around entries is
	// trimmed. A single space-separated string must NOT be split into entries —
	// that would silently merge the staging test UIDs into one bogus UID.
	tests := []struct {
		name string
		val  string
		want []string
	}{
		{"single uid", "owner-uid", []string{"owner-uid"}},
		{"comma separated", "uid-a,uid-b,uid-denied,uid-conflict",
			[]string{"uid-a", "uid-b", "uid-denied", "uid-conflict"}},
		{"comma separated with spaces", "uid-a, uid-b , uid-c",
			[]string{"uid-a", "uid-b", "uid-c"}},
		{"space separated stays one entry", "uid-a uid-b uid-c",
			[]string{"uid-a uid-b uid-c"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := minDevEnv()
			env["ALLOWED_FIREBASE_UIDS"] = tt.val

			cfg, err := newFromEnv(mapLookup(env))
			if err != nil {
				t.Fatalf("newFromEnv() error = %v", err)
			}
			assertStringSlice(t, cfg.AllowedFirebaseUIDs, tt.want)
		})
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

// --- ValidateFirebaseAuthConfig tests ---

// validServerConfigStaging returns a Config that passes ValidateFirebaseAuthConfig in
// staging. It writes a temp credentials file because staging/prod now require a
// readable FIREBASE_CREDENTIALS_FILE (see ValidateFirebaseAuthConfig).
func validServerConfigStaging(t *testing.T) *Config {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "firebase-creds-*.json")
	if err != nil {
		t.Fatalf("create temp creds file: %v", err)
	}
	f.Close()
	return &Config{
		Env:                     EnvStaging,
		FirebaseProjectID:       "algoedgefno-staging",
		FirebaseWebAPIKey:       "test-web-api-key",
		FirebaseCredentialsFile: f.Name(),
		AllowedFirebaseUIDs:     []string{"uid-staging-1"},
	}
}

func TestValidateFirebaseAuthConfig_NilReturnsError(t *testing.T) {
	if err := ValidateFirebaseAuthConfig(nil); err == nil {
		t.Fatal("expected error for nil config")
	}
}

func TestValidateFirebaseAuthConfig_StagingRequiresWebAPIKey(t *testing.T) {
	cfg := validServerConfigStaging(t)
	cfg.FirebaseWebAPIKey = ""
	if err := ValidateFirebaseAuthConfig(cfg); err == nil {
		t.Fatal("expected error when FirebaseWebAPIKey is empty in staging")
	}
}

func TestValidateFirebaseAuthConfig_ProdRequiresWebAPIKey(t *testing.T) {
	cfg := validServerConfigStaging(t)
	cfg.Env = EnvProduction
	cfg.FirebaseWebAPIKey = ""
	if err := ValidateFirebaseAuthConfig(cfg); err == nil {
		t.Fatal("expected error when FirebaseWebAPIKey is empty in production")
	}
}

func TestValidateFirebaseAuthConfig_StagingRequiresNonEmptyAllowlist(t *testing.T) {
	cfg := validServerConfigStaging(t)
	cfg.AllowedFirebaseUIDs = nil
	if err := ValidateFirebaseAuthConfig(cfg); err == nil {
		t.Fatal("expected error for empty allowlist in staging")
	}
}

func TestValidateFirebaseAuthConfig_ProdRequiresNonEmptyAllowlist(t *testing.T) {
	cfg := validServerConfigStaging(t)
	cfg.Env = EnvProduction
	cfg.AllowedFirebaseUIDs = nil
	if err := ValidateFirebaseAuthConfig(cfg); err == nil {
		t.Fatal("expected error for empty allowlist in production")
	}
}

func TestValidateFirebaseAuthConfig_DevAllowsEmptyAllowlist(t *testing.T) {
	cfg := &Config{
		Env:                 EnvDevelopment,
		AllowedFirebaseUIDs: nil,
	}
	if err := ValidateFirebaseAuthConfig(cfg); err != nil {
		t.Fatalf("expected no error for empty allowlist in dev, got %v", err)
	}
}

func TestValidateFirebaseAuthConfig_TestAllowsEmptyAllowlist(t *testing.T) {
	cfg := &Config{
		Env:                 EnvTest,
		AllowedFirebaseUIDs: nil,
	}
	if err := ValidateFirebaseAuthConfig(cfg); err != nil {
		t.Fatalf("expected no error for empty allowlist in test, got %v", err)
	}
}

func TestValidateFirebaseAuthConfig_CredentialsFileRequiresProjectID(t *testing.T) {
	// Write a temp file so the readable-check passes.
	f, err := os.CreateTemp(t.TempDir(), "creds-*.json")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	f.Close()

	cfg := &Config{
		Env:                     EnvDevelopment,
		FirebaseCredentialsFile: f.Name(),
		FirebaseProjectID:       "", // missing
	}
	if err := ValidateFirebaseAuthConfig(cfg); err == nil {
		t.Fatal("expected error when FirebaseProjectID is empty but credentials file is set")
	}
}

func TestValidateFirebaseAuthConfig_UnreadableCredentialsFile(t *testing.T) {
	cfg := &Config{
		Env:                     EnvDevelopment,
		FirebaseCredentialsFile: "/nonexistent/path/to/creds.json",
		FirebaseProjectID:       "my-project",
	}
	if err := ValidateFirebaseAuthConfig(cfg); err == nil {
		t.Fatal("expected error for unreadable credentials file")
	}
}

func TestValidateFirebaseAuthConfig_ValidStagingConfig(t *testing.T) {
	cfg := validServerConfigStaging(t)
	if err := ValidateFirebaseAuthConfig(cfg); err != nil {
		t.Fatalf("expected no error for valid staging config, got %v", err)
	}
}

func TestValidateFirebaseAuthConfig_StagingRequiresCredentialsFile(t *testing.T) {
	cfg := validServerConfigStaging(t)
	cfg.FirebaseCredentialsFile = ""
	if err := ValidateFirebaseAuthConfig(cfg); err == nil {
		t.Fatal("expected error when FirebaseCredentialsFile is empty in staging")
	}
}

func TestValidateFirebaseAuthConfig_ProdRequiresCredentialsFile(t *testing.T) {
	cfg := validServerConfigStaging(t)
	cfg.Env = EnvProduction
	cfg.FirebaseCredentialsFile = ""
	if err := ValidateFirebaseAuthConfig(cfg); err == nil {
		t.Fatal("expected error when FirebaseCredentialsFile is empty in production")
	}
}

func TestValidateFirebaseAuthConfig_DevNoFirebaseAllPasses(t *testing.T) {
	cfg := &Config{
		Env: EnvDevelopment,
	}
	if err := ValidateFirebaseAuthConfig(cfg); err != nil {
		t.Fatalf("expected no error for dev with no Firebase config, got %v", err)
	}
}

// TestEnvExamples_FirebaseProjectIDsDiffer asserts that prod.env.example and
// staging.env.example contain non-empty, distinct FIREBASE_PROJECT_ID values.
// A shared project ID between environments would allow staging test UIDs to
// authenticate against production Firebase, defeating the per-environment UID
// separation. The test reads the example files relative to the module root so
// it catches accidental copy-paste before the files reach the VPS.
func TestEnvExamples_FirebaseProjectIDsDiffer(t *testing.T) {
	repoRoot := filepath.Join("..", "..", "deploy", "env")
	stagingID := readEnvExampleValue(t, filepath.Join(repoRoot, "staging.env.example"), "FIREBASE_PROJECT_ID")
	prodID := readEnvExampleValue(t, filepath.Join(repoRoot, "prod.env.example"), "FIREBASE_PROJECT_ID")

	if stagingID == "" {
		t.Fatal("staging.env.example: FIREBASE_PROJECT_ID is empty")
	}
	if prodID == "" {
		t.Fatal("prod.env.example: FIREBASE_PROJECT_ID is empty")
	}
	if stagingID == prodID {
		t.Fatalf("staging and prod FIREBASE_PROJECT_ID must differ, both are %q", stagingID)
	}
}

// readEnvExampleValue reads the first matching KEY=VALUE line from an env
// example file and returns the value portion. It is intentionally lenient
// about surrounding whitespace but rejects inline comments.
func readEnvExampleValue(t *testing.T, path, key string) string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()

	prefix := key + "="
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan %s: %v", path, err)
	}
	return ""
}
