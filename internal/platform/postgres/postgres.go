// Package postgres provides a configured *sqlx.DB connection to our
// application database. Call New(cfg) once at startup; the returned *sqlx.DB
// is safe to share across goroutines and should be passed to every
// repository that needs database access.
//
// Connection pool settings (MaxOpenConns etc.) are tuned for a typical
// web app. If you're seeing "too many connections" errors in production,
// check your Postgres max_connections setting and adjust these values.
package postgres

import (
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/topboyasante/api-base/internal/config"
)

func New(cfg config.DBConfig) (*sqlx.DB, error) {
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s", cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.Name, cfg.SSLMode)
	db, err := sqlx.Connect("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("connect postgres: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return db, nil
}
