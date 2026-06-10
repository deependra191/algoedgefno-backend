package main

import (
	"testing"

	"github.com/deependra191/algoedgefno-backend/internal/config"
)

func TestValidateLocalConfigRejectsUnsafeEnvironmentsAndDatabaseIdentity(t *testing.T) {
	tests := []struct {
		name string
		cfg  config.Config
	}{
		{
			name: "staging env",
			cfg:  localConfig(config.EnvStaging, "algoedgefno", "algoedge", localHostName),
		},
		{
			name: "production-like database name",
			cfg:  localConfig(config.EnvDevelopment, "algoedgefno_prod", "algoedge", localHostName),
		},
		{
			name: "staging-like database user",
			cfg:  localConfig(config.EnvDevelopment, "algoedgefno", "algoedge_stage", localHostName),
		},
		{
			name: "vps-like connection string",
			cfg:  localConfig(config.EnvDevelopment, "algoedgefno", "algoedge", "db.internal.vps"),
		},
		{
			name: "non-loopback host",
			cfg:  localConfig(config.EnvDevelopment, "algoedgefno", "algoedge", "10.0.0.10"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateLocalConfig(&tt.cfg); err == nil {
				t.Fatal("expected local guard to reject config")
			}
		})
	}
}

func TestValidateLocalConfigAllowsDevelopmentLoopback(t *testing.T) {
	cfg := localConfig(config.EnvDevelopment, "algoedgefno", "algoedge", localHostName)
	if err := validateLocalConfig(&cfg); err != nil {
		t.Fatalf("unexpected guard error: %v", err)
	}
}

func localConfig(env config.Environment, dbName, dbUser, dbHost string) config.Config {
	return config.Config{
		Env:    env,
		DBName: dbName,
		DBUser: dbUser,
		DBHost: dbHost,
	}
}
