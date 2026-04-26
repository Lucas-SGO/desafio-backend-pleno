package config

import (
	"log"
	"os"
)

type Config struct {
	Port          string
	DatabaseURL   string
	RedisURL      string
	WebhookSecret string
	CPFHMACSecret string
	JWTSecret     string
}

func Load() Config {
	cfg := Config{
		Port:          getEnv("APP_PORT", "8080"),
		DatabaseURL:   requireEnv("DATABASE_URL"),
		RedisURL:      requireEnv("REDIS_URL"),
		WebhookSecret: requireEnv("WEBHOOK_SECRET"),
		CPFHMACSecret: requireEnv("CPF_HMAC_SECRET"),
		JWTSecret:     os.Getenv("JWT_SECRET"),
	}
	return cfg
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func requireEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("required environment variable %s is not set", key)
	}
	return v
}
