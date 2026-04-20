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
	Storage   StorageConfig
	Payment   PaymentConfig
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

type StorageConfig struct {
	Provider string                       // "local" | "s3" | ...
	Options  map[string]map[string]string // provider-specific: path, bucket, region, keys...
}

type PaymentConfig struct {
	CardProvider        string // e.g. "stripe"
	MobileMoneyProvider string // e.g. "paystack"

	// Options per provider, keyed by provider name.
	// Example: Options["stripe"]["secret_key"]
	Options map[string]map[string]string
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
		Storage: StorageConfig{
			Provider: getEnvOrDefault("STORAGE_PROVIDER", "local"),
			Options:  buildStorageOptions(),
		},
		Payment: PaymentConfig{
			CardProvider:        getEnvOrDefault("PAYMENT_CARD_PROVIDER", "stripe"),
			MobileMoneyProvider: getEnvOrDefault("PAYMENT_MOBILE_MONEY_PROVIDER", "paystack"),
			Options:             buildPaymentOptions(),
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

// buildStorageOptions loads options for every storage provider whose env is
// present. STORAGE_PROVIDER picks the default; other providers stay loaded so
// individual services can opt into them via the registry.
//
// "local" is always available — handy as a dev/test fallback even when no
// env is set.
func buildStorageOptions() map[string]map[string]string {
	opts := map[string]map[string]string{
		"local": {
			"path":     getEnvOrDefault("STORAGE_LOCAL_PATH", "./uploads"),
			"base_url": getEnvOrDefault("STORAGE_LOCAL_BASE_URL", "/files"),
		},
	}

	if bucket := os.Getenv("STORAGE_S3_BUCKET"); bucket != "" {
		opts["s3"] = map[string]string{
			"bucket":     bucket,
			"region":     os.Getenv("STORAGE_S3_REGION"),
			"access_key": os.Getenv("STORAGE_S3_ACCESS_KEY"),
			"secret_key": os.Getenv("STORAGE_S3_SECRET_KEY"),
			"endpoint":   os.Getenv("STORAGE_S3_ENDPOINT"),
		}
	}

	if accountID := os.Getenv("STORAGE_R2_ACCOUNT_ID"); accountID != "" {
		opts["r2"] = map[string]string{
			"account_id": accountID,
			"bucket":     os.Getenv("STORAGE_R2_BUCKET"),
			"access_key": os.Getenv("STORAGE_R2_ACCESS_KEY"),
			"secret_key": os.Getenv("STORAGE_R2_SECRET_KEY"),
			"public_url": os.Getenv("STORAGE_R2_PUBLIC_URL"),
		}
	}

	return opts
}

// buildPaymentOptions follows the same rule as storage: load whichever
// processors have credentials present, regardless of which one is the default.
func buildPaymentOptions() map[string]map[string]string {
	opts := map[string]map[string]string{}

	if key := os.Getenv("STRIPE_SECRET_KEY"); key != "" {
		opts["stripe"] = map[string]string{
			"secret_key":     key,
			"webhook_secret": os.Getenv("STRIPE_WEBHOOK_SECRET"),
		}
	}

	if key := os.Getenv("PAYSTACK_SECRET_KEY"); key != "" {
		opts["paystack"] = map[string]string{
			"secret_key":     key,
			"webhook_secret": os.Getenv("PAYSTACK_WEBHOOK_SECRET"),
		}
	}

	return opts
}
