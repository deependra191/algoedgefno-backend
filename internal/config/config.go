package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
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
		DBUser:         getEnv("DB_USER", "algoedge"),
		DBPass:         getEnv("DB_PASSWORD", "algoedge"),
		DBName:         getEnv("DB_NAME", "algoedgefno"),
		JWTSecret:      getEnv("JWT_SECRET", "change-this-in-production"),
		AppSecretToken: getEnv("APP_SECRET_TOKEN", ""),
		Env:            getEnv("ENV", "development"),
	}

	if cfg.Env == "production" {
		if cfg.AppSecretToken == "" {
			log.Fatal("APP_SECRET_TOKEN must be set in production")
		}
		if cfg.JWTSecret == "change-this-in-production" {
			log.Fatal("JWT_SECRET must be changed in production")
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
