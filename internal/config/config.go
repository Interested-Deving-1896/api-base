// Package config loads application settings from environment variables.
//
// Load is called once during startup in main.go. The returned Config struct
// is passed into internal/app.BuildApp which then hands the relevant pieces
// to each subsystem (database, redis, rate limiter, etc.).
//
// When you add a new configuration value, add it to three places:
//  1. The appropriate struct below (grouped by subsystem)
//  2. The Load function, reading from os.Getenv
//  3. The .env.example file at the project root
//
// Never read from os.Getenv anywhere else in the codebase. All config comes
// through this package.
package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	App       AppConfig
	DB        DBConfig
	Redis     RedisConfig
	RateLimit RateLimitConfig
}

type AppConfig struct {
	Env  string
	Port string
}

type DBConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	Name     string
	SSLMode  string
}

type RedisConfig struct {
	Addr     string
	Password string
}

type RateLimitConfig struct {
	PerIPHourly int
}

func Load() (*Config, error) {
	// In production, environment variables come from the orchestrator
	// (Kubernetes, systemd, Docker). In dev, we load from .env.
	_ = godotenv.Load() // best-effort; fine if the file doesn't exist

	perIP, err := strconv.Atoi(getEnvOrDefault("RATELIMIT_PER_IP_HOURLY", "1000"))
	if err != nil {
		return nil, fmt.Errorf("invalid RATELIMIT_PER_IP_HOURLY config: %w", err)
	}

	return &Config{
		App: AppConfig{
			Env:  getEnvOrDefault("APP_ENV", "development"),
			Port: getEnvOrDefault("APP_PORT", "8080"),
		},
		DB: DBConfig{
			Host:     mustGetEnv("DB_HOST"),
			Port:     mustGetEnv("DB_PORT"),
			User:     mustGetEnv("DB_USER"),
			Password: mustGetEnv("DB_PASSWORD"),
			Name:     mustGetEnv("DB_NAME"),
			SSLMode:  getEnvOrDefault("DB_SSLMODE", "disable"),
		},
		Redis: RedisConfig{
			Addr:     mustGetEnv("REDIS_ADDR"),
			Password: os.Getenv("REDIS_PASSWORD"),
		},
		RateLimit: RateLimitConfig{
			PerIPHourly: perIP,
		},
	}, nil
}

func mustGetEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		panic(fmt.Sprintf("required env var %s was not set", key))
	}
	return v
}

func getEnvOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
