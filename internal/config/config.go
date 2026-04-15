package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

const (
	EnvProduction    = "production"
	DefaultJWTSecret = "change-this-in-production"
	defaultDBUser    = "algoedge"
	defaultDBPass    = "algoedge"
	defaultDBName    = "algoedgefno"
)

type Config struct {
	Port           string
	DBHost         string
	DBPort         string
	DBUser         string
	DBPass         string
	DBName         string
	JWTSecret      string
	AppSecretToken string
	Env            string
}

func Load() *Config {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	cfg := &Config{
		Port:           getEnv("PORT", "8080"),
		DBHost:         getEnv("DB_HOST", "localhost"),
		DBPort:         getEnv("DB_PORT", "5432"),
		DBUser:         getEnv("DB_USER", defaultDBUser),
		DBPass:         getEnv("DB_PASSWORD", defaultDBPass),
		DBName:         getEnv("DB_NAME", defaultDBName),
		JWTSecret:      getEnv("JWT_SECRET", DefaultJWTSecret),
		AppSecretToken: getEnv("APP_SECRET_TOKEN", ""),
		Env:            getEnv("ENV", "development"),
	}

	if cfg.Env == EnvProduction {
		if cfg.AppSecretToken == "" {
			log.Fatal("APP_SECRET_TOKEN must be set in production")
		}
		if cfg.JWTSecret == DefaultJWTSecret {
			log.Fatal("JWT_SECRET must be changed in production")
		}
		if cfg.DBUser == defaultDBUser || cfg.DBPass == defaultDBPass || cfg.DBName == defaultDBName {
			log.Fatal("DB_USER, DB_PASSWORD and DB_NAME must be changed in production")
		}
	}

	return cfg
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
