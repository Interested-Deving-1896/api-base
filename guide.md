# Building a Modular Monolith in Go — A Walkthrough

This is a hands-on guide to building a production-grade Go backend from scratch. We'll build a simple Todo API, but the real goal is to learn the structure and patterns so you can apply them to any project.

By the end, you'll have:

- A Gin HTTP server with graceful shutdown
- Postgres and Redis wired up
- A consistent response envelope for every endpoint
- Request tracing via request IDs (end-to-end)
- Structured logging with `slog`
- Prometheus metrics
- Rate limiting and idempotency middleware
- A full Todo module with Create and GetByID endpoints
- Swagger documentation with a CI gate to keep it fresh
- Tests for every layer (service, repository, integration)

Take your time. Each step builds on the one before it. Run the verification at the end of each step before moving on — if something's broken, it's easier to find the bug in 20 lines than in 2,000.

---

## Table of Contents

1. [Prerequisites and Mental Model](#step-1-prerequisites-and-mental-model)
2. [Initialize the Project](#step-2-initialize-the-project)
3. [Install Dependencies](#step-3-install-dependencies)
4. [Config Loading](#step-4-config-loading)
5. [Request Context — The Thread That Ties Everything Together](#step-5-request-context)
6. [The Response Envelope](#step-6-the-response-envelope)
7. [Typed API Errors](#step-7-typed-api-errors)
8. [Observability — Logger and Metrics](#step-8-observability)
9. [Platform Layer — Postgres, Redis, Server, Validator](#step-9-platform-layer)
10. [Middleware Stack](#step-10-middleware-stack)
11. [Rate Limiting](#step-11-rate-limiting)
12. [Idempotency](#step-12-idempotency)
13. [Migrations](#step-13-migrations)
14. [The Todo Module](#step-14-the-todo-module)
15. [Wiring Everything Together](#step-15-wiring-everything-together)
16. [Main and Graceful Shutdown](#step-16-main-and-graceful-shutdown)
17. [Swagger Docs and the CI Gate](#step-17-swagger-docs-and-the-ci-gate)
18. [Running and Verifying](#step-18-running-and-verifying)
19. [Writing Tests](#step-19-writing-tests)
20. [What's Next](#step-20-whats-next)

---

## Step 1: Prerequisites and Mental Model

Before we type anything, let's get on the same page about what we're building and why.

### What you need installed

- Go 1.22 or newer
- Docker and Docker Compose (for Postgres and Redis locally)
- `make` (for running our build commands)
- `curl` or any HTTP client for testing

### The mental model

We're building a **modular monolith**. That means:

- One deployable binary — simple to run, simple to deploy.
- Internal modules with clear boundaries — each module is a self-contained feature area (users, todos, orders, etc.).
- Modules talk to each other through **interfaces**, never direct imports of internal packages. This is what keeps the monolith from turning into spaghetti.

Three ideas do most of the work in this structure. If you remember nothing else, remember these:

**Every request carries a request ID.** We generate it at the edge, store it in the Go `context.Context`, include it in every log line and every error response. When something breaks, that ID is how you trace exactly what happened.

**The response shape is a contract.** Every successful response, every error, every validation failure — they all return the same envelope. Clients can rely on this. Our handlers never call `c.JSON` directly; they go through helpers.

**DTOs at every boundary.** Domain models (the structs that represent real business concepts like `Todo`) never leave their module. At the HTTP edge, we convert them to DTOs — Data Transfer Objects — that carry JSON tags and validation rules. This keeps our internal model free to evolve without breaking the API.

Keep these three ideas in mind as we build. Every decision we make serves one of them.

---

## Step 2: Initialize the Project

Let's create the project and scaffold our directory structure. Starting with the layout upfront is worth the small effort — it's much easier than shuffling files later.

```bash
mkdir backend && cd backend
go mod init github.com/yourname/backend
git init
```

Now create the directory structure. We'll explain what each folder is for as we fill it in, but building the skeleton first helps you see the shape of the project.

```bash
mkdir -p cmd/api \
  internal/app \
  internal/config \
  internal/modules/todo/{handler,service,repository,dto,mapper,domain} \
  internal/platform/{postgres,redis,server,validator} \
  internal/platform/postgres/migrations \
  internal/observability/{logger,metrics} \
  internal/shared/{response,apierror,middleware,requestctx,ratelimit,idempotency} \
  api/docs \
  test/integration \
  scripts \
  .github/workflows
```

Quick tour of what we just created:

- `cmd/api/` — the entry point. Our `main.go` will live here.
- `internal/app/` — wires all the pieces together. Think of it as the project's assembly point.
- `internal/config/` — loads configuration from environment variables.
- `internal/modules/todo/` — our first business module. Each module has `handler/`, `service/`, `repository/`, `dto/`, `mapper/`, `domain/` as subfolders. Layered per module.
- `internal/platform/` — wrappers around infrastructure concerns (database, cache, HTTP server, validator). Kept thin.
- `internal/observability/` — logger and metrics. Set up once, used everywhere.
- `internal/shared/` — cross-cutting concerns: the response envelope, typed errors, middleware, rate limiting, idempotency.
- `api/docs/` — Swagger will generate files here. We add a `.gitkeep`.
- `test/integration/` — end-to-end HTTP tests that boot the whole app.
- `scripts/` — shell scripts for migrations and tests.
- `.github/workflows/` — our CI gate for keeping API docs fresh.

**Why `internal/`?** In Go, anything under `internal/` can only be imported by code in the same module. It's a compile-time guarantee that nobody outside our project can depend on our internals. This is the simplest way to enforce "these are our private building blocks."

Now create a `.gitignore` to keep junk out of version control:

```bash
cat > .gitignore <<'EOF'
# Binaries
/bin/
/backend

# Env
.env

# Uploads and temp files
/uploads/
/tmp/
*.log

# Test coverage
coverage.out

# IDEs
.idea/
.vscode/
*.swp
EOF
```

And a `.env.example` — this is the contract for what configuration our app expects. Future-you (and every new contributor) will thank you for keeping this accurate.

```bash
cat > .env.example <<'EOF'
APP_ENV=development
APP_PORT=8080

DB_HOST=localhost
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=postgres
DB_NAME=backend
DB_SSLMODE=disable

REDIS_ADDR=localhost:6379

RATELIMIT_PER_IP_HOURLY=1000
EOF

cp .env.example .env
```

Copy to `.env` so you have a working local config right away. The `.env` file is gitignored; `.env.example` is committed.

**Verify:** Run `git status` — you should see the structure created but no `.env` file being tracked.

---

## Step 3: Install Dependencies

Let's grab everything we need in one pass. Doing this upfront means we don't have to context-switch later when we're in the middle of thinking about business logic.

```bash
# HTTP server
go get github.com/gin-gonic/gin

# Database
go get github.com/jmoiron/sqlx
go get github.com/lib/pq

# Redis
go get github.com/redis/go-redis/v9

# Validation
go get github.com/go-playground/validator/v10

# Env loading
go get github.com/joho/godotenv

# UUIDs (for request IDs, record IDs, etc.)
go get github.com/google/uuid

# Prometheus metrics
go get github.com/prometheus/client_golang/prometheus
go get github.com/prometheus/client_golang/prometheus/promhttp

# Swagger docs
go install github.com/swaggo/swag/cmd/swag@latest
go get github.com/swaggo/gin-swagger
go get github.com/swaggo/files
```

Notice what's **not** on this list:

- No ORM. We use raw SQL with `sqlx` — it's boring and predictable, and you can read every query.
- No dependency injection framework. We wire things up in a plain Go function. It's 50 lines of code and anyone can read it.
- No logging library. `slog` is in the standard library as of Go 1.21 and it's great.
- No auth library yet. We'll add that when we actually need it.

The less magic in your stack, the less your team has to learn.

---

## Step 4: Config Loading

Every app needs configuration. We'll load it once at startup and pass it where it's needed. Nobody outside `config/` should ever touch `os.Getenv` directly — that's how you end up with config scattered across the codebase.

Create `internal/config/config.go`:

```go
// Package config loads application settings from environment variables.
//
// Load is called once during startup in main.go. The returned Config struct
// is passed into internal/app.BuildApp which then hands the relevant pieces
// to each subsystem (database, redis, rate limiter, etc.).
//
// When you add a new configuration value, add it to three places:
//   1. The appropriate struct below (grouped by subsystem)
//   2. The Load function, reading from os.Getenv
//   3. The .env.example file at the project root
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

func (d DBConfig) DSN() string {
	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		d.Host, d.Port, d.User, d.Password, d.Name, d.SSLMode)
}

type RedisConfig struct {
	Addr string
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
		return nil, fmt.Errorf("invalid RATELIMIT_PER_IP_HOURLY: %w", err)
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
			Addr: mustGetEnv("REDIS_ADDR"),
		},
		RateLimit: RateLimitConfig{
			PerIPHourly: perIP,
		},
	}, nil
}

func mustGetEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		panic(fmt.Sprintf("required env var %s not set", key))
	}
	return v
}

func getEnvOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
```

Why `mustGetEnv` for required values? If the database password is missing, we want to crash immediately on startup with a clear error — not run for 30 seconds then mysteriously fail on the first DB query. Fail fast.

**Verify:** `go build ./...` should succeed. The config package compiles even though nothing uses it yet.

---

## Step 5: Request Context

Before we build anything else, we need to establish how request-scoped data flows through our app. This is the foundation everything else depends on.

The idea: when a request comes in, we assign it a unique ID. That ID travels with the request through every function call. When we log something, the request ID is on the log line. When we return an error, the request ID is in the response. When the user reports a bug, they send us that ID, and we find exactly what happened.

Create `internal/shared/requestctx/requestctx.go`:

```go
// Package requestctx stores and retrieves request-scoped values from the
// Go context.Context.
//
// Anything you want to make available to any function handling a request —
// the request ID, the authenticated user, the trace ID — goes here.
//
// We use a private type for the context key (ctxKey) so that no other
// package can accidentally collide with us by using the same string key.
// This is the standard Go pattern for context values.
//
// Middleware is what populates these values (see internal/shared/middleware).
// Handlers and services retrieve them by calling RequestID(ctx), UserID(ctx),
// etc.
package requestctx

import "context"

type ctxKey string

const (
	requestIDKey ctxKey = "request_id"
	userIDKey    ctxKey = "user_id"
)

func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey, id)
}

func RequestID(ctx context.Context) string {
	if v, ok := ctx.Value(requestIDKey).(string); ok {
		return v
	}
	return ""
}

func WithUserID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, userIDKey, id)
}

func UserID(ctx context.Context) string {
	if v, ok := ctx.Value(userIDKey).(string); ok {
		return v
	}
	return ""
}
```

That's the whole file. Small but critical — everything else in our observability and error handling refers back to this.

**Why typed keys?** `ctxKey` is unexported. If some third-party package also puts a `"request_id"` string into context, it won't collide with ours because their key has a different underlying type.

---

## Step 6: The Response Envelope

Now we set up the contract for every API response. Before writing a single handler, we decide once what a response looks like — and every handler in the codebase will use it.

Create `internal/shared/response/response.go`:

```go
// Package response defines the shape every API response takes — success,
// error, or validation failure. Every handler writes its response through
// the helpers here (Success, Error, ValidationError). This is how we
// guarantee a consistent response format across the whole API.
//
// Handlers should never call c.JSON directly. If you find yourself wanting
// to, ask why — the helpers probably already handle your case, or we should
// add a new helper rather than diverge.
//
// The request_id field inside every error body is populated from context.
// That ID comes from the RequestID middleware, which sets it before any
// handler runs. If you see an error response without a request_id, the
// middleware isn't wired up correctly.
package response

import (
	"github.com/gin-gonic/gin"
	"github.com/yourname/backend/internal/shared/requestctx"
)

type Envelope struct {
	Success bool       `json:"success"`
	Data    any        `json:"data,omitempty"`
	Error   *ErrorBody `json:"error,omitempty"`
	Meta    *Meta      `json:"meta,omitempty"`
}

type ErrorBody struct {
	Code      string       `json:"code"`
	Message   string       `json:"message"`
	RequestID string       `json:"request_id"`
	Details   []FieldError `json:"details,omitempty"`
}

type FieldError struct {
	Field   string `json:"field"`
	Tag     string `json:"tag"`
	Message string `json:"message"`
}

type Meta struct {
	Page       int `json:"page,omitempty"`
	PageSize   int `json:"page_size,omitempty"`
	TotalItems int `json:"total_items,omitempty"`
}

func Success(c *gin.Context, status int, data any) {
	c.JSON(status, Envelope{Success: true, Data: data})
}

func Error(c *gin.Context, status int, code, message string) {
	c.JSON(status, Envelope{
		Success: false,
		Error: &ErrorBody{
			Code:      code,
			Message:   message,
			RequestID: requestctx.RequestID(c.Request.Context()),
		},
	})
}

func ValidationError(c *gin.Context, fields []FieldError) {
	c.JSON(422, Envelope{
		Success: false,
		Error: &ErrorBody{
			Code:      "VALIDATION_FAILED",
			Message:   "the request contains invalid data",
			RequestID: requestctx.RequestID(c.Request.Context()),
			Details:   fields,
		},
	})
}
```

Take a moment to look at what this does. Three functions. Every response in our entire API will go through one of them. If we ever need to change the response shape — add a `timestamp` field, rename `success` to `ok`, whatever — we change it here, once. No hunting through 50 handlers.

A success response looks like:
```json
{"success": true, "data": {"id": "abc", "title": "buy milk"}}
```

An error response looks like:
```json
{"success": false, "error": {"code": "NOT_FOUND", "message": "todo not found", "request_id": "req_abc123"}}
```

A validation error looks like:
```json
{
  "success": false,
  "error": {
    "code": "VALIDATION_FAILED",
    "message": "the request contains invalid data",
    "request_id": "req_abc123",
    "details": [{"field": "title", "tag": "required", "message": "title is required"}]
  }
}
```

**The `request_id` in every error is the single most valuable thing in this package.** When a user screenshots an error, that ID tells you exactly which log lines to grep for. Support tickets go from "can't reproduce" to "found it in 10 seconds."

**Replace `github.com/yourname/backend`** in the import with your actual module path (what you put in `go mod init`).

---

## Step 7: Typed API Errors

Our services will need to signal failure types to handlers — "not found," "conflict," "bad request" — in a way that middleware can translate to the right HTTP status. We define a small set of typed errors for this.

Create `internal/shared/apierror/apierror.go`:

```go
// Package apierror defines the typed errors that services return and
// middleware translates to HTTP responses.
//
// A service that wants to signal "not found" returns apierror.ErrNotFound.
// The handler catches the error with c.Error(err), and the ErrorHandler
// middleware (see internal/shared/middleware/errorhandler.go) unwraps it
// via apierror.As and sends the matching HTTP status through
// response.Error.
//
// When you need a new error type (e.g. ErrForbidden, ErrTooManyRequests),
// add it here with a clear Code, user-safe Message, and correct HTTPCode.
// Never put error codes anywhere else in the codebase — this is the single
// source of truth.
package apierror

import "errors"

type Error struct {
	Code     string
	Message  string
	HTTPCode int
}

func (e *Error) Error() string { return e.Message }

var (
	ErrNotFound   = &Error{Code: "NOT_FOUND", Message: "resource not found", HTTPCode: 404}
	ErrConflict   = &Error{Code: "CONFLICT", Message: "resource already exists", HTTPCode: 409}
	ErrBadRequest = &Error{Code: "BAD_REQUEST", Message: "bad request", HTTPCode: 400}
	ErrInternal   = &Error{Code: "INTERNAL", Message: "internal server error", HTTPCode: 500}
)

// As unwraps an error into an *apierror.Error if possible. Middleware uses
// this to decide the HTTP status for an error returned from a handler.
func As(err error) (*Error, bool) {
	var apiErr *Error
	if errors.As(err, &apiErr) {
		return apiErr, true
	}
	return nil, false
}
```

Services return these. Handlers add them to `c.Errors`. Middleware translates them. Clean separation.

---

## Step 8: Observability

Observability means: can you answer "what is my system doing right now?" and "what did it do five minutes ago?" We build two pieces: structured logging and Prometheus metrics.

### Logger

Create `internal/observability/logger/logger.go`:

```go
// Package logger wraps Go's standard slog library with a helper that
// automatically attaches request-scoped values (like request_id) to every
// log line.
//
// Call Init once at startup (main.go does this). After that, anywhere in
// the codebase that has a context.Context can call:
//
//     logger.FromContext(ctx).Info("user_created", "user_id", u.ID)
//
// and the resulting log line will include request_id automatically. This
// is how we trace a request across many functions without passing a
// logger down every call.
//
// In production we emit JSON (parseable by log aggregators). In dev we
// emit human-readable text. The format is chosen by the APP_ENV value.
package logger

import (
	"context"
	"log/slog"
	"os"

	"github.com/yourname/backend/internal/shared/requestctx"
)

var base *slog.Logger

func Init(env string) {
	var handler slog.Handler
	opts := &slog.HandlerOptions{Level: slog.LevelInfo}
	if env != "production" {
		opts.Level = slog.LevelDebug
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}
	base = slog.New(handler)
	slog.SetDefault(base)
}

// FromContext returns a logger with request_id (and any other request-scoped
// fields) pre-attached. Always prefer this over calling slog directly, so
// that every log line is automatically correlated to a request.
func FromContext(ctx context.Context) *slog.Logger {
	if base == nil {
		// Init wasn't called. Fall back to default so tests don't panic.
		return slog.Default()
	}
	l := base
	if rid := requestctx.RequestID(ctx); rid != "" {
		l = l.With("request_id", rid)
	}
	return l
}
```

### Metrics

Create `internal/observability/metrics/metrics.go`:

```go
// Package metrics exposes Prometheus metrics for HTTP requests.
//
// Three metrics are collected:
//   - http_requests_total: counter, how many requests we've served
//   - http_request_duration_seconds: histogram, how long each request took
//   - http_requests_in_flight: gauge, how many requests are currently being
//     processed
//
// Call Init once at startup to register the collectors. Then use:
//   - Middleware() on every route to collect measurements
//   - Handler() on the /metrics endpoint so Prometheus can scrape it
//
// IMPORTANT: the middleware uses c.FullPath() for the path label, which
// gives us the route template (/todos/:id) rather than the actual URL
// (/todos/abc-123). If we used the URL, every unique ID would create a
// new label combination and our metric memory would explode. Never change
// this without understanding why.
package metrics

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	requestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total HTTP requests by method, path, status",
		},
		[]string{"method", "path", "status"},
	)
	requestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "Request duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)
	inFlight = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "http_requests_in_flight",
			Help: "In-flight HTTP requests",
		},
	)
)

func Init() {
	prometheus.MustRegister(requestsTotal, requestDuration, inFlight)
}

func Handler() gin.HandlerFunc {
	h := promhttp.Handler()
	return func(c *gin.Context) { h.ServeHTTP(c.Writer, c.Request) }
}

func Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		inFlight.Inc()
		defer inFlight.Dec()

		c.Next()

		path := c.FullPath()
		if path == "" {
			path = "unknown"
		}
		status := strconv.Itoa(c.Writer.Status())
		requestsTotal.WithLabelValues(c.Request.Method, path, status).Inc()
		requestDuration.WithLabelValues(c.Request.Method, path).Observe(time.Since(start).Seconds())
	}
}
```

**Verify:** `go build ./...` — everything should still compile.

---

## Step 9: Platform Layer

The "platform" layer is our thin wrapper around infrastructure. Each file does one thing: connect to Postgres, or connect to Redis, or start an HTTP server. They exist so the rest of our code doesn't have to know the library-specific setup details.

### Postgres

Create `internal/platform/postgres/postgres.go`:

```go
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

	"github.com/yourname/backend/internal/config"
)

func New(cfg config.DBConfig) (*sqlx.DB, error) {
	db, err := sqlx.Connect("postgres", cfg.DSN())
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
```

### Redis

Create `internal/platform/redis/redis.go`:

```go
// Package redis provides a configured Redis client.
//
// We use Redis for rate limiting counters and (in the future) caching.
// The client is safe to share across goroutines. Call New once at startup
// and pass the result to anything that needs Redis.
package redis

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"

	"github.com/yourname/backend/internal/config"
)

func New(cfg config.RedisConfig) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{Addr: cfg.Addr})
	if err := client.Ping(context.Background()).Err(); err != nil {
		return nil, fmt.Errorf("ping redis: %w", err)
	}
	return client, nil
}
```

### Server (with graceful shutdown)

Create `internal/platform/server/server.go`:

```go
// Package server wraps *http.Server with a graceful shutdown helper.
//
// When the process receives SIGINT or SIGTERM, we want to:
//   1. Stop accepting new connections
//   2. Let in-flight requests complete (up to a timeout)
//   3. Close the server cleanly
//
// This matters in production because the orchestrator (Kubernetes,
// systemd) will SIGTERM our pod during deploys. Without graceful
// shutdown, in-flight requests get dropped and users see random errors.
//
// Usage: build a server with New, then call Run. Run blocks until the
// process receives a shutdown signal or the server errors.
package server

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type Server struct {
	httpServer *http.Server
}

func New(addr string, handler http.Handler) *Server {
	return &Server{
		httpServer: &http.Server{
			Addr:              addr,
			Handler:           handler,
			ReadHeaderTimeout: 10 * time.Second,
		},
	}
}

func (s *Server) Run() error {
	// Start listening in a goroutine so Run can also wait for signals.
	errCh := make(chan error, 1)
	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	// Wait for either a fatal server error or a shutdown signal.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		return err
	case <-sigCh:
		// Got shutdown signal; drain in-flight requests.
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return s.httpServer.Shutdown(ctx)
	}
}
```

### Validator

Create `internal/platform/validator/validator.go`:

```go
// Package validator wraps go-playground/validator with a helper that
// translates validation errors into our standard response.FieldError
// format.
//
// Handlers call c.ShouldBindJSON(&req), which internally runs the
// validator against the DTO's `binding:` struct tags. If validation
// fails, the handler passes the error to TranslateErrors and then to
// response.ValidationError so the client gets a structured, field-level
// error response.
package validator

import (
	"errors"
	"fmt"

	govalidator "github.com/go-playground/validator/v10"

	"github.com/yourname/backend/internal/shared/response"
)

func New() *govalidator.Validate {
	return govalidator.New()
}

// TranslateErrors converts a go-playground/validator error into our
// FieldError slice format. If the error isn't a ValidationErrors, returns nil.
func TranslateErrors(err error) []response.FieldError {
	var ve govalidator.ValidationErrors
	if !errors.As(err, &ve) {
		return nil
	}
	out := make([]response.FieldError, 0, len(ve))
	for _, fe := range ve {
		out = append(out, response.FieldError{
			Field:   fe.Field(),
			Tag:     fe.Tag(),
			Message: messageFor(fe),
		})
	}
	return out
}

func messageFor(fe govalidator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return fmt.Sprintf("%s is required", fe.Field())
	case "email":
		return fmt.Sprintf("%s must be a valid email", fe.Field())
	case "min":
		return fmt.Sprintf("%s must be at least %s characters", fe.Field(), fe.Param())
	case "max":
		return fmt.Sprintf("%s must be at most %s characters", fe.Field(), fe.Param())
	default:
		return fmt.Sprintf("%s failed %s validation", fe.Field(), fe.Tag())
	}
}
```

**Verify:** `go build ./...` still passes.

---

## Step 10: Middleware Stack

Middleware runs for every request in a defined order. Ours does five things: tag the request with an ID, log it, measure it, recover from panics, and translate errors to responses.

The order matters. Each piece builds on the last. Let's build them now.

### Request ID middleware

Create `internal/shared/middleware/requestid.go`:

```go
// Package middleware contains HTTP middleware that runs for every request.
// Each file in this package is one piece of middleware; wire.go decides the
// order they run in.
//
// RequestID must always be first. It generates (or accepts via header) a
// unique ID for the request, stores it in context, and echoes it in the
// X-Request-ID response header. Every other middleware and every handler
// relies on the ID being present.
package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/yourname/backend/internal/shared/requestctx"
)

func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader("X-Request-ID")
		if id == "" {
			id = uuid.NewString()
		}
		ctx := requestctx.WithRequestID(c.Request.Context(), id)
		c.Request = c.Request.WithContext(ctx)
		c.Writer.Header().Set("X-Request-ID", id)
		c.Next()
	}
}
```

Why accept `X-Request-ID` from the client? Because when our API is called by another service, that service may already have a request ID. Honoring it lets you trace a request across service boundaries (even though we're a monolith today, this costs nothing to support).

### Logger middleware

Create `internal/shared/middleware/logger.go`:

```go
// Logger middleware emits one structured log line per request with method,
// path, status, duration, and client IP. The request ID is automatically
// attached via logger.FromContext.
//
// This is the single most useful line of observability in the entire app.
// When something goes wrong, the first thing you do is find the request in
// these logs.
package middleware

import (
	"time"

	"github.com/gin-gonic/gin"

	"github.com/yourname/backend/internal/observability/logger"
)

func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		logger.FromContext(c.Request.Context()).Info("http_request",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"duration_ms", time.Since(start).Milliseconds(),
			"ip", c.ClientIP(),
		)
	}
}
```

### Recover middleware

Create `internal/shared/middleware/recover.go`:

```go
// Recover middleware catches panics that escape handlers and turns them
// into 500 responses. Without this, a panic would crash the whole process.
//
// We log the panic (with stack trace and request ID) so you can debug it,
// and return a generic error to the client so we don't leak internals.
package middleware

import (
	"runtime/debug"

	"github.com/gin-gonic/gin"

	"github.com/yourname/backend/internal/observability/logger"
	"github.com/yourname/backend/internal/shared/response"
)

func Recover() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				logger.FromContext(c.Request.Context()).Error("panic_recovered",
					"panic", r,
					"stack", string(debug.Stack()),
				)
				response.Error(c, 500, "INTERNAL", "an internal error occurred")
				c.Abort()
			}
		}()
		c.Next()
	}
}
```

### Error handler middleware

Create `internal/shared/middleware/errorhandler.go`:

```go
// ErrorHandler middleware runs after all handlers and converts any errors
// they added to c.Errors into properly formatted HTTP responses.
//
// The contract is:
//   - Handlers call c.Error(err) and return, rather than calling
//     response.Error directly.
//   - If the error is an *apierror.Error, we use its HTTPCode, Code, and
//     user-safe Message.
//   - Otherwise, we log the real error (never expose it) and return a
//     generic 500 with the request ID.
//
// This is how we enforce "never leak internals in errors" across the
// whole codebase. A handler can't accidentally return a Postgres error
// string to the user — the middleware intercepts it.
package middleware

import (
	"github.com/gin-gonic/gin"

	"github.com/yourname/backend/internal/observability/logger"
	"github.com/yourname/backend/internal/shared/apierror"
	"github.com/yourname/backend/internal/shared/response"
)

func ErrorHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		if len(c.Errors) == 0 {
			return
		}

		err := c.Errors.Last().Err
		logger.FromContext(c.Request.Context()).Error("handler_error", "err", err.Error())

		if apiErr, ok := apierror.As(err); ok {
			response.Error(c, apiErr.HTTPCode, apiErr.Code, apiErr.Message)
			return
		}
		response.Error(c, 500, "INTERNAL", "an internal error occurred")
	}
}
```

**Pause and appreciate this design.** A handler that fails does this:

```go
if err := h.svc.Create(ctx, req.Title); err != nil {
    c.Error(err)
    return
}
```

It doesn't need to know the HTTP status. It doesn't need to format a response. It doesn't need to log. The middleware does all of it, consistently, for every handler. This is why we built the typed errors and response envelope first.

---

## Step 11: Rate Limiting

Rate limiting protects your API from being hammered — whether maliciously or by a buggy client. We use Redis to track requests per time window.

Create `internal/shared/ratelimit/ratelimit.go`:

```go
// Package ratelimit provides Redis-backed rate limiting for our HTTP API.
//
// Design choices worth knowing:
//
//   - We use an hourly fixed-window counter. Simpler than a sliding window,
//     fine for most APIs. The key is "ratelimit:{bucket}:{YYYY-MM-DD-HH}",
//     so each hour gets its own counter and old counters expire naturally.
//
//   - When Redis is unreachable, we FAIL OPEN — log a warning and let the
//     request through. The rate limiter is a safety net, not a security
//     boundary. (Contrast with idempotency, which fails closed.)
//
//   - We set X-RateLimit-* headers on every response so clients can back
//     off gracefully before they hit the limit.
//
// The Middleware function buckets requests by client IP. If you need
// per-user or per-API-key limits later, add a bucketKey parameter and
// compose another middleware.
package ratelimit

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"github.com/yourname/backend/internal/observability/logger"
	"github.com/yourname/backend/internal/shared/response"
)

type Limiter struct {
	rdb *redis.Client
}

func New(rdb *redis.Client) *Limiter {
	return &Limiter{rdb: rdb}
}

type Result struct {
	Allowed   bool
	Remaining int
	Limit     int
	ResetAt   time.Time
}

func (l *Limiter) Check(ctx context.Context, bucketKey string, limit int) (*Result, error) {
	hour := time.Now().UTC().Format("2006-01-02-15")
	key := fmt.Sprintf("ratelimit:%s:%s", bucketKey, hour)

	count, err := l.rdb.Incr(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	if count == 1 {
		l.rdb.Expire(ctx, key, time.Hour)
	}

	remaining := limit - int(count)
	if remaining < 0 {
		remaining = 0
	}
	resetAt := time.Now().UTC().Truncate(time.Hour).Add(time.Hour)

	return &Result{
		Allowed:   int(count) <= limit,
		Remaining: remaining,
		Limit:     limit,
		ResetAt:   resetAt,
	}, nil
}

func Middleware(l *Limiter, perIPLimit int) gin.HandlerFunc {
	return func(c *gin.Context) {
		res, err := l.Check(c.Request.Context(), "ip:"+c.ClientIP(), perIPLimit)
		if err != nil {
			logger.FromContext(c.Request.Context()).Warn("ratelimit_check_failed", "err", err)
			c.Next()
			return
		}

		c.Writer.Header().Set("X-RateLimit-Limit", strconv.Itoa(res.Limit))
		c.Writer.Header().Set("X-RateLimit-Remaining", strconv.Itoa(res.Remaining))
		c.Writer.Header().Set("X-RateLimit-Reset", strconv.FormatInt(res.ResetAt.Unix(), 10))

		if !res.Allowed {
			c.Writer.Header().Set("Retry-After", strconv.Itoa(int(time.Until(res.ResetAt).Seconds())))
			response.Error(c, 429, "RATE_LIMIT_EXCEEDED",
				fmt.Sprintf("rate limit exceeded, resets at %s", res.ResetAt.Format(time.RFC3339)))
			c.Abort()
			return
		}
		c.Next()
	}
}
```

---

## Step 12: Idempotency

Idempotency prevents double-charges and duplicate creations. The client sends an `Idempotency-Key` header with mutation requests. If we've seen that key before, we return the cached response instead of processing again.

Create `internal/shared/idempotency/idempotency.go`:

```go
// Package idempotency provides middleware that prevents duplicate execution
// of mutation requests.
//
// How it works:
//   - Client sends POST/PUT/DELETE with "Idempotency-Key: <some-uuid>" header.
//   - We hash the request body (SHA-256) and store the response against the
//     key for 48 hours.
//   - If the same key arrives again with the SAME body, we return the
//     cached response — the request is NOT processed again.
//   - If the same key arrives with a DIFFERENT body, that's a bug in the
//     client, and we return 422 IDEMPOTENCY_KEY_REUSED.
//
// This is critical for anything that transfers money, sends notifications,
// or creates unique resources. Apply the middleware ONLY to mutation
// routes — GETs are already idempotent.
//
// Unlike rate limiting, this middleware FAILS CLOSED. If the database is
// unreachable, we return an error rather than risk a double-charge.
//
// The Idempotency-Key header is optional. If a client doesn't send it,
// the middleware does nothing and the request proceeds normally.
package idempotency

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"

	"github.com/yourname/backend/internal/observability/logger"
	"github.com/yourname/backend/internal/shared/requestctx"
	"github.com/yourname/backend/internal/shared/response"
)

type Store struct {
	db *sqlx.DB
}

func NewStore(db *sqlx.DB) *Store {
	return &Store{db: db}
}

type Record struct {
	Key          string    `db:"idempotency_key"`
	ConsumerID   string    `db:"consumer_id"`
	RequestHash  string    `db:"request_hash"`
	ResponseBody []byte    `db:"response_body"`
	StatusCode   int       `db:"status_code"`
	ExpiresAt    time.Time `db:"expires_at"`
}

func (s *Store) Get(ctx context.Context, key, consumerID string) (*Record, error) {
	var r Record
	err := s.db.GetContext(ctx, &r, `
		SELECT idempotency_key, consumer_id, request_hash, response_body, status_code, expires_at
		FROM idempotent_requests
		WHERE idempotency_key = $1 AND consumer_id = $2 AND expires_at > NOW()
	`, key, consumerID)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *Store) Save(ctx context.Context, r *Record) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO idempotent_requests
			(idempotency_key, consumer_id, request_hash, response_body, status_code, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (idempotency_key, consumer_id) DO NOTHING
	`, r.Key, r.ConsumerID, r.RequestHash, r.ResponseBody, r.StatusCode, r.ExpiresAt)
	return err
}

func Middleware(store *Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := c.GetHeader("Idempotency-Key")
		if key == "" {
			c.Next()
			return
		}

		// Consumer ID placeholder. When we add auth, replace this with the
		// authenticated user/API key ID so two clients can reuse the same
		// header value without colliding.
		// TODO: replace with authenticated consumer once auth module exists.
		consumerID := "anon"

		bodyBytes, _ := io.ReadAll(c.Request.Body)
		c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		hash := sha256Hex(bodyBytes)

		existing, err := store.Get(c.Request.Context(), key, consumerID)
		if err == nil && existing != nil {
			if existing.RequestHash != hash {
				response.Error(c, 422, "IDEMPOTENCY_KEY_REUSED",
					"idempotency key was used with different parameters")
				c.Abort()
				return
			}
			c.Data(existing.StatusCode, "application/json", existing.ResponseBody)
			c.Abort()
			return
		}

		rec := &responseCapture{ResponseWriter: c.Writer, body: &bytes.Buffer{}}
		c.Writer = rec

		c.Next()

		saveErr := store.Save(c.Request.Context(), &Record{
			Key:          key,
			ConsumerID:   consumerID,
			RequestHash:  hash,
			ResponseBody: rec.body.Bytes(),
			StatusCode:   c.Writer.Status(),
			ExpiresAt:    time.Now().Add(48 * time.Hour),
		})
		if saveErr != nil {
			logger.FromContext(c.Request.Context()).Error("idempotency_save_failed",
				"err", saveErr,
				"request_id", requestctx.RequestID(c.Request.Context()),
			)
		}
	}
}

func sha256Hex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

type responseCapture struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (r *responseCapture) Write(b []byte) (int, error) {
	r.body.Write(b)
	return r.ResponseWriter.Write(b)
}
```

---

## Step 13: Migrations

We need tables in Postgres. Use [golang-migrate](https://github.com/golang-migrate/migrate) for this; install it separately (not a Go dependency in our project):

```bash
# On macOS:
brew install golang-migrate

# On Linux:
curl -L https://github.com/golang-migrate/migrate/releases/latest/download/migrate.linux-amd64.tar.gz | tar xvz
sudo mv migrate /usr/local/bin/
```

Now create the migration files.

`internal/platform/postgres/migrations/0001_create_todos.up.sql`:
```sql
CREATE TABLE todos (
    id UUID PRIMARY KEY,
    title VARCHAR(255) NOT NULL,
    description TEXT,
    done BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

`internal/platform/postgres/migrations/0001_create_todos.down.sql`:
```sql
DROP TABLE IF EXISTS todos;
```

`internal/platform/postgres/migrations/0002_create_idempotent_requests.up.sql`:
```sql
CREATE TABLE idempotent_requests (
    idempotency_key VARCHAR(255) NOT NULL,
    consumer_id     VARCHAR(100) NOT NULL,
    request_hash    VARCHAR(64)  NOT NULL,
    response_body   BYTEA        NOT NULL,
    status_code     INTEGER      NOT NULL,
    expires_at      TIMESTAMPTZ  NOT NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    PRIMARY KEY (idempotency_key, consumer_id)
);
CREATE INDEX idx_idempotent_expires ON idempotent_requests(expires_at);
```

`internal/platform/postgres/migrations/0002_create_idempotent_requests.down.sql`:
```sql
DROP TABLE IF EXISTS idempotent_requests;
```

And a helper script `scripts/migrate.sh`:
```bash
#!/usr/bin/env bash
set -euo pipefail

source .env

DB_URL="postgres://${DB_USER}:${DB_PASSWORD}@${DB_HOST}:${DB_PORT}/${DB_NAME}?sslmode=${DB_SSLMODE}"

migrate -path internal/platform/postgres/migrations -database "$DB_URL" "$@"
```

Make it executable: `chmod +x scripts/migrate.sh`.

---

## Step 14: The Todo Module

Now the fun part. We build our first feature. Do it bottom-up: domain first, then repository, service, DTOs, mapper, handler, routes. Each layer only knows the one below it.

### Domain model

Create `internal/modules/todo/domain/todo.go`:

```go
// Package domain holds the internal business model for the todo module.
// This struct represents a Todo as our code thinks about it, independent
// of how it's stored in the database or sent over the API.
//
// Domain models never leave this module. They are not sent as API
// responses (that's what DTOs are for). They are not imported by other
// modules (those modules use the todo.TodoService contract instead).
package domain

import "time"

type Todo struct {
	ID          string
	Title       string
	Description string
	Done        bool
	CreatedAt   time.Time
}
```

### Repository

Create `internal/modules/todo/repository/repository.go`:

```go
// Package repository is the data-access layer for the todo module. It
// translates domain.Todo values to/from rows in the todos table.
//
// The Repository interface is what the service depends on. Tests can
// provide a mock implementation; production uses the real sqlx-backed one.
//
// Error translation happens here: sql.ErrNoRows becomes apierror.ErrNotFound,
// and unique violations become apierror.ErrConflict. The service and
// handler layers don't need to know about SQL details.
package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"

	"github.com/yourname/backend/internal/modules/todo/domain"
	"github.com/yourname/backend/internal/shared/apierror"
)

type Repository interface {
	Create(ctx context.Context, t *domain.Todo) error
	GetByID(ctx context.Context, id string) (*domain.Todo, error)
}

type repo struct {
	db *sqlx.DB
}

func New(db *sqlx.DB) Repository {
	return &repo{db: db}
}

type todoRow struct {
	ID          string    `db:"id"`
	Title       string    `db:"title"`
	Description string    `db:"description"`
	Done        bool      `db:"done"`
	CreatedAt   time.Time `db:"created_at"`
}

func (r *repo) Create(ctx context.Context, t *domain.Todo) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO todos (id, title, description, done, created_at)
		VALUES ($1, $2, $3, $4, $5)
	`, t.ID, t.Title, t.Description, t.Done, t.CreatedAt)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			return apierror.ErrConflict
		}
		return err
	}
	return nil
}

func (r *repo) GetByID(ctx context.Context, id string) (*domain.Todo, error) {
	var row todoRow
	err := r.db.GetContext(ctx, &row, `
		SELECT id, title, description, done, created_at
		FROM todos WHERE id = $1
	`, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, apierror.ErrNotFound
		}
		return nil, err
	}
	return &domain.Todo{
		ID:          row.ID,
		Title:       row.Title,
		Description: row.Description,
		Done:        row.Done,
		CreatedAt:   row.CreatedAt,
	}, nil
}
```

You'll need to add `import "time"` at the top — the formatter will do it automatically when you save.

### Service

Create `internal/modules/todo/service/service.go`:

```go
// Package service holds the business logic for the todo module. Handlers
// call into the Service interface; the service calls into the Repository.
//
// The service is where domain rules live: default values, cross-field
// validation, coordination between the database and other subsystems. For
// now, "create a todo" is straightforward — we set an ID and timestamp,
// then save. As the business grows, this is where those rules will land.
package service

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/yourname/backend/internal/modules/todo/domain"
	"github.com/yourname/backend/internal/modules/todo/repository"
	"github.com/yourname/backend/internal/observability/logger"
)

type Service interface {
	Create(ctx context.Context, title, description string) (*domain.Todo, error)
	GetByID(ctx context.Context, id string) (*domain.Todo, error)
}

type service struct {
	repo repository.Repository
}

func New(r repository.Repository) Service {
	return &service{repo: r}
}

func (s *service) Create(ctx context.Context, title, description string) (*domain.Todo, error) {
	t := &domain.Todo{
		ID:          uuid.NewString(),
		Title:       title,
		Description: description,
		Done:        false,
		CreatedAt:   time.Now().UTC(),
	}
	if err := s.repo.Create(ctx, t); err != nil {
		logger.FromContext(ctx).Error("todo_create_failed", "err", err.Error())
		return nil, err
	}
	logger.FromContext(ctx).Info("todo_created", "todo_id", t.ID)
	return t, nil
}

func (s *service) GetByID(ctx context.Context, id string) (*domain.Todo, error) {
	return s.repo.GetByID(ctx, id)
}
```

### DTOs

Request and response DTOs live in separate files — they have different concerns.

Create `internal/modules/todo/dto/request.go`:

```go
// Package dto holds the data transfer objects for the todo module. These
// are the structs clients send us (requests) and the structs we send back
// (responses). They exist so domain models can evolve independently of
// the API contract.
//
// Request DTOs carry validation tags (binding:"required,min=3"). Gin
// validates them automatically when we call c.ShouldBindJSON. If
// validation fails, the handler responds with a 422 via
// response.ValidationError.
package dto

type CreateTodoRequest struct {
	Title       string `json:"title" binding:"required,min=3,max=255"`
	Description string `json:"description" binding:"max=2000"`
}
```

Create `internal/modules/todo/dto/response.go`:

```go
package dto

import "time"

// TodoResponse is what we send back to clients. Notice this has JSON tags
// but no validation tags. It also has no pointer to anything internal —
// handlers build it via the mapper package.
type TodoResponse struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Done        bool      `json:"done"`
	CreatedAt   time.Time `json:"created_at"`
}
```

### Mapper

Create `internal/modules/todo/mapper/mapper.go`:

```go
// Package mapper converts between domain models and DTOs.
//
// Two separate files would be overkill for now, but as the module grows
// you might split this into todo_to_dto.go and todo_from_dto.go. For now,
// one function is enough.
//
// IMPORTANT: the mapper is the ONLY place where domain <-> DTO conversion
// happens. Handlers don't construct DTOs directly from domain fields; they
// call the mapper. This way, if the domain model changes, only the mapper
// needs updating.
package mapper

import (
	"github.com/yourname/backend/internal/modules/todo/domain"
	"github.com/yourname/backend/internal/modules/todo/dto"
)

func ToTodoResponse(t *domain.Todo) dto.TodoResponse {
	return dto.TodoResponse{
		ID:          t.ID,
		Title:       t.Title,
		Description: t.Description,
		Done:        t.Done,
		CreatedAt:   t.CreatedAt,
	}
}
```

### Handler

Create `internal/modules/todo/handler/handler.go`:

```go
// Package handler is the HTTP-facing layer of the todo module. It does
// four things per endpoint:
//
//   1. Bind and validate the incoming DTO
//   2. Call the service with primitive values
//   3. Map the returned domain model to a response DTO
//   4. Send the response through the response package
//
// Handlers are deliberately thin. Business logic belongs in the service.
// Data access belongs in the repository. If you find yourself writing a
// SQL query or a business rule in a handler, move it down a layer.
//
// Swaggo annotations (the `// @Summary`, `// @Success` lines) document
// each endpoint. Running `make docs` regenerates api/docs from these
// annotations. CI fails the build if they drift.
package handler

import (
	"github.com/gin-gonic/gin"
	govalidator "github.com/go-playground/validator/v10"

	"github.com/yourname/backend/internal/modules/todo/dto"
	"github.com/yourname/backend/internal/modules/todo/mapper"
	"github.com/yourname/backend/internal/modules/todo/service"
	platformvalidator "github.com/yourname/backend/internal/platform/validator"
	"github.com/yourname/backend/internal/shared/response"
)

// SuccessResponse is a swaggo helper — it documents the envelope shape
// returned by successful endpoints. Use it in @Success annotations:
//     @Success 201 {object} handler.SuccessResponse{data=dto.TodoResponse}
type SuccessResponse struct {
	Success bool `json:"success" example:"true"`
	Data    any  `json:"data"`
}

// ErrorResponse documents the envelope shape returned by error endpoints.
type ErrorResponse struct {
	Success bool               `json:"success" example:"false"`
	Error   response.ErrorBody `json:"error"`
}

type Handler struct {
	svc       service.Service
	validator *govalidator.Validate
}

func New(svc service.Service, v *govalidator.Validate) *Handler {
	return &Handler{svc: svc, validator: v}
}

// Create godoc
// @Summary      Create a todo
// @Tags         todos
// @Accept       json
// @Produce      json
// @Param        body body dto.CreateTodoRequest true "todo payload"
// @Success      201 {object} handler.SuccessResponse{data=dto.TodoResponse}
// @Failure      422 {object} handler.ErrorResponse
// @Router       /todos [post]
func (h *Handler) Create(c *gin.Context) {
	var req dto.CreateTodoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fields := platformvalidator.TranslateErrors(err)
		if len(fields) > 0 {
			response.ValidationError(c, fields)
			return
		}
		response.Error(c, 400, "BAD_REQUEST", "invalid request body")
		return
	}

	t, err := h.svc.Create(c.Request.Context(), req.Title, req.Description)
	if err != nil {
		c.Error(err)
		return
	}

	response.Success(c, 201, mapper.ToTodoResponse(t))
}

// GetByID godoc
// @Summary      Get a todo by ID
// @Tags         todos
// @Produce      json
// @Param        id path string true "todo ID"
// @Success      200 {object} handler.SuccessResponse{data=dto.TodoResponse}
// @Failure      404 {object} handler.ErrorResponse
// @Router       /todos/{id} [get]
func (h *Handler) GetByID(c *gin.Context) {
	id := c.Param("id")
	t, err := h.svc.GetByID(c.Request.Context(), id)
	if err != nil {
		c.Error(err)
		return
	}
	response.Success(c, 200, mapper.ToTodoResponse(t))
}
```

### Routes

Create `internal/modules/todo/handler/routes.go`:

```go
package handler

import "github.com/gin-gonic/gin"

// RegisterQueryRoutes wires the read-only endpoints. These don't need
// idempotency middleware; GETs are already idempotent.
func (h *Handler) RegisterQueryRoutes(r *gin.RouterGroup) {
	g := r.Group("/todos")
	g.GET("/:id", h.GetByID)
}

// RegisterMutationRoutes wires the write endpoints. wire.go wraps this
// group in idempotency middleware.
func (h *Handler) RegisterMutationRoutes(r *gin.RouterGroup) {
	g := r.Group("/todos")
	g.POST("", h.Create)
}
```

Splitting queries and mutations into two registration functions lets us apply idempotency middleware only where it matters.

### Contract and Module

Create `internal/modules/todo/contract.go`:

```go
// Package todo exposes the public contract for the todo module — the
// interface that other modules import when they need to talk to todos.
//
// If another module (say, a notification module) wants to look up a todo,
// it takes a todo.TodoService in its constructor. It never imports
// internal/modules/todo/service directly. This keeps module boundaries
// clear and makes dependencies explicit.
//
// For now, no other module depends on todos, but defining the contract
// from day one sets the habit.
package todo

import "github.com/yourname/backend/internal/modules/todo/service"

type TodoService = service.Service
```

Create `internal/modules/todo/module.go`:

```go
package todo

import (
	govalidator "github.com/go-playground/validator/v10"
	"github.com/jmoiron/sqlx"

	"github.com/yourname/backend/internal/modules/todo/handler"
	"github.com/yourname/backend/internal/modules/todo/repository"
	"github.com/yourname/backend/internal/modules/todo/service"
)

// New wires the todo module's internals and returns its HTTP handler.
// Called from internal/app/wire.go during application startup.
func New(db *sqlx.DB, v *govalidator.Validate) *handler.Handler {
	repo := repository.New(db)
	svc := service.New(repo)
	return handler.New(svc, v)
}
```

**Verify:** `go build ./...` — everything should compile.

---

## Step 15: Wiring Everything Together

Now we assemble the pieces. This is `internal/app/wire.go` — the one place that knows about everything.

Create `internal/app/app.go`:

```go
// Package app assembles the application from its components and runs it.
//
// BuildApp (in wire.go) is called from main.go with a loaded Config. It
// connects to infrastructure (db, redis), builds shared utilities, wires
// every module, and sets up the HTTP router with middleware.
//
// The App struct holds the assembled pieces so main.go can call Run.
package app

import (
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/redis/go-redis/v9"

	"github.com/yourname/backend/internal/platform/server"
)

type App struct {
	router *gin.Engine
	db     *sqlx.DB
	redis  *redis.Client
	port   string
}

func (a *App) Run() error {
	srv := server.New(":"+a.port, a.router)
	return srv.Run()
}
```

Create `internal/app/wire.go`:

```go
// This file is the wiring diagram for the whole application. Read it top
// to bottom to see how every component connects.
//
// Ordering matters:
//   1. Logger and metrics init first — so everything else can log/measure.
//   2. Infrastructure (db, redis) — so higher layers can use it.
//   3. Shared utilities (validator, rate limiter, idempotency store).
//   4. Modules (todo, and later others).
//   5. Router — middleware chain, then routes.
//
// When you add a new module, you'll touch this file in two places: build
// the module with its New function, then register its routes.
package app

import (
	_ "github.com/yourname/backend/api/docs" // blank import so swaggo docs are registered
	"github.com/gin-gonic/gin"
	swaggerfiles "github.com/swaggo/files"
	ginswagger "github.com/swaggo/gin-swagger"

	"github.com/yourname/backend/internal/config"
	"github.com/yourname/backend/internal/modules/todo"
	"github.com/yourname/backend/internal/observability/logger"
	"github.com/yourname/backend/internal/observability/metrics"
	"github.com/yourname/backend/internal/platform/postgres"
	platformredis "github.com/yourname/backend/internal/platform/redis"
	platformvalidator "github.com/yourname/backend/internal/platform/validator"
	"github.com/yourname/backend/internal/shared/idempotency"
	"github.com/yourname/backend/internal/shared/middleware"
	"github.com/yourname/backend/internal/shared/ratelimit"
	"github.com/yourname/backend/internal/shared/response"
)

func BuildApp(cfg *config.Config) (*App, error) {
	// 1. Observability
	logger.Init(cfg.App.Env)
	metrics.Init()

	// 2. Infrastructure
	db, err := postgres.New(cfg.DB)
	if err != nil {
		return nil, err
	}
	rdb, err := platformredis.New(cfg.Redis)
	if err != nil {
		return nil, err
	}

	// 3. Shared utilities
	v := platformvalidator.New()
	rlim := ratelimit.New(rdb)
	idemStore := idempotency.NewStore(db)

	// 4. Modules
	todoHandler := todo.New(db, v)

	// 5. Router
	if cfg.App.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.New()

	// Middleware order:
	//   RequestID first — everyone else depends on it.
	//   Logger next — so we log even if something below panics.
	//   Metrics next — so we record even failed requests.
	//   Recover next — turns panics into 500s.
	//   ErrorHandler last — translates c.Errors into responses.
	r.Use(
		middleware.RequestID(),
		middleware.Logger(),
		metrics.Middleware(),
		middleware.Recover(),
		middleware.ErrorHandler(),
	)

	// Public endpoints (no rate limit)
	r.GET("/health", func(c *gin.Context) {
		response.Success(c, 200, gin.H{"status": "ok"})
	})
	r.GET("/metrics", metrics.Handler())
	r.GET("/swagger/*any", ginswagger.WrapHandler(swaggerfiles.Handler))

	// API group — rate-limited
	api := r.Group("/api/v1", ratelimit.Middleware(rlim, cfg.RateLimit.PerIPHourly))

	// Read endpoints: rate-limited only
	todoHandler.RegisterQueryRoutes(api)

	// Write endpoints: rate-limited AND idempotency-protected
	mutations := api.Group("", idempotency.Middleware(idemStore))
	todoHandler.RegisterMutationRoutes(mutations)

	return &App{
		router: r,
		db:     db,
		redis:  rdb,
		port:   cfg.App.Port,
	}, nil
}
```

This function is long, but it's flat and grep-able. Every new hire can read it top to bottom and understand how the app fits together in 5 minutes. That's the goal.

---

## Step 16: Main and Graceful Shutdown

Our entry point is tiny because `wire.go` does the heavy lifting.

Create `cmd/api/main.go`:

```go
// Package main is the entry point for the API binary.
//
// This file should stay short. Its only jobs are:
//   1. Load config
//   2. Build the application (see internal/app/wire.go)
//   3. Run it
//
// If you're tempted to put anything else here — a handler, a database
// query, a helper function — put it in the appropriate package instead.
// Keeping main.go minimal means anyone can read it and immediately know
// where to look for real code.
//
// The top-level swaggo annotations (@title, @version, @BasePath) are read
// by `swag init` to populate the generated OpenAPI spec.
package main

import (
	"log"

	_ "github.com/yourname/backend/api/docs"
	"github.com/yourname/backend/internal/app"
	"github.com/yourname/backend/internal/config"
)

// @title           Backend API
// @version         1.0
// @description     Modular monolith backend for learning
// @BasePath        /api/v1
func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	a, err := app.BuildApp(cfg)
	if err != nil {
		log.Fatalf("build app: %v", err)
	}

	if err := a.Run(); err != nil {
		log.Fatalf("run: %v", err)
	}
}
```

---

## Step 17: Swagger Docs and the CI Gate

We're using code-first documentation — annotations in the handlers generate the OpenAPI spec. The CI gate makes sure nobody can merge code without regenerating the spec.

Generate the docs for the first time:

```bash
swag init -g cmd/api/main.go -o api/docs --parseDependency --parseInternal
```

This creates `api/docs/docs.go`, `api/docs/swagger.json`, and `api/docs/swagger.yaml`. Commit them.

Create `.github/workflows/api-docs.yml`:

```yaml
name: API Docs Freshness

on: [pull_request]

jobs:
  check-swagger:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - name: Install swag
        run: go install github.com/swaggo/swag/cmd/swag@latest
      - name: Regenerate docs
        run: swag init -g cmd/api/main.go -o api/docs --parseDependency --parseInternal
      - name: Verify no drift
        run: |
          if [ -n "$(git status --porcelain api/docs)" ]; then
            echo "::error::API docs are stale. Run 'make docs' and commit the result."
            git diff api/docs
            exit 1
          fi
```

Create a `Makefile`:

```makefile
.PHONY: run build test docs docs-check migrate-up migrate-down lint

run:
	go run ./cmd/api

build:
	go build -o bin/api ./cmd/api

test:
	go test ./...

docs:
	swag init -g cmd/api/main.go -o api/docs --parseDependency --parseInternal

docs-check:
	@swag init -g cmd/api/main.go -o api/docs --parseDependency --parseInternal
	@git diff --exit-code api/docs || (echo "docs drift detected, run 'make docs' and commit" && exit 1)

migrate-up:
	./scripts/migrate.sh up

migrate-down:
	./scripts/migrate.sh down 1

lint:
	go vet ./...
```

And a `docker-compose.yml` to run Postgres and Redis locally:

```yaml
services:
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: postgres
      POSTGRES_PASSWORD: postgres
      POSTGRES_DB: backend
    ports:
      - "5432:5432"
    volumes:
      - pg-data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U postgres"]
      interval: 5s
      timeout: 5s
      retries: 5

  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"
    volumes:
      - redis-data:/data
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 5s
      timeout: 5s
      retries: 5

volumes:
  pg-data:
  redis-data:
```

---

## Step 18: Running and Verifying

Time to make sure it all works. Start the infrastructure:

```bash
docker compose up -d
```

Wait a few seconds for Postgres to be ready, then run migrations:

```bash
make migrate-up
```

Start the server:

```bash
make run
```

You should see log output indicating the server is listening. Now let's verify each piece works. **Run all four checks** before declaring victory.

### Check 1: Request ID threading

```bash
curl -i -X POST http://localhost:8080/api/v1/todos \
  -H "Content-Type: application/json" \
  -d '{"title":"buy milk","description":"whole milk"}'
```

Look at the response headers — you should see `X-Request-ID: <some-uuid>`. Now look at your server logs — the same UUID appears on the `http_request` log line and the `todo_created` log line. That's the whole observability thread working.

Now send a request with your own ID:

```bash
curl -i -H "X-Request-ID: my-test-id-123" -X GET http://localhost:8080/api/v1/todos/nonexistent
```

The response header and logs should both show `my-test-id-123`. Your ID was honored.

### Check 2: Error envelope

```bash
curl -X POST http://localhost:8080/api/v1/todos \
  -H "Content-Type: application/json" \
  -d '{"title":"hi"}'
```

Title is too short. Expected response:

```json
{
  "success": false,
  "error": {
    "code": "VALIDATION_FAILED",
    "message": "the request contains invalid data",
    "request_id": "req_xxx",
    "details": [{"field": "Title", "tag": "min", "message": "Title must be at least 3 characters"}]
  }
}
```

Also check the 404 case:

```bash
curl http://localhost:8080/api/v1/todos/does-not-exist
```

Expected: `{"success":false,"error":{"code":"NOT_FOUND","message":"resource not found","request_id":"req_xxx"}}`

### Check 3: Metrics

```bash
curl http://localhost:8080/metrics | grep http_requests_total
```

You should see counter entries with labels `method`, `path`, `status`. Confirm the `path` value is `/api/v1/todos/:id` (route template), not the actual URL.

### Check 4: Idempotency

First create a todo and note the ID in the response:

```bash
curl -X POST http://localhost:8080/api/v1/todos \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: test-key-1" \
  -d '{"title":"buy milk","description":""}'
```

Send the exact same request again with the same key:

```bash
curl -X POST http://localhost:8080/api/v1/todos \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: test-key-1" \
  -d '{"title":"buy milk","description":""}'
```

The second response has the **same ID** as the first. No duplicate was created.

Now reuse the key with a different body:

```bash
curl -X POST http://localhost:8080/api/v1/todos \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: test-key-1" \
  -d '{"title":"buy bread","description":""}'
```

Expected: `422 IDEMPOTENCY_KEY_REUSED`.

### Swagger UI

Visit `http://localhost:8080/swagger/index.html` in a browser. You should see both endpoints documented with request and response schemas.

If all four checks pass, your foundation is solid.

---

## Step 19: Writing Tests

We're going to write one test per layer: service (pure logic), repository (real database), and integration (full HTTP). These three together give us confidence that the whole stack works.

### Service test (pure logic, no DB)

The service depends on the `Repository` interface. For tests, we provide a hand-written fake. No mocking libraries — they add magic we don't need for a project this size.

Create `internal/modules/todo/service/service_test.go`:

```go
package service

import (
	"context"
	"errors"
	"testing"

	"github.com/yourname/backend/internal/modules/todo/domain"
	"github.com/yourname/backend/internal/shared/apierror"
)

// fakeRepo is a hand-written test double for the Repository interface.
// Nothing fancy — just maps for stored state and flags for behavior.
type fakeRepo struct {
	store     map[string]*domain.Todo
	createErr error
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{store: map[string]*domain.Todo{}}
}

func (f *fakeRepo) Create(ctx context.Context, t *domain.Todo) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.store[t.ID] = t
	return nil
}

func (f *fakeRepo) GetByID(ctx context.Context, id string) (*domain.Todo, error) {
	t, ok := f.store[id]
	if !ok {
		return nil, apierror.ErrNotFound
	}
	return t, nil
}

func TestService_Create_Success(t *testing.T) {
	repo := newFakeRepo()
	svc := New(repo)

	got, err := svc.Create(context.Background(), "buy milk", "whole milk")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID == "" {
		t.Error("expected ID to be generated, got empty string")
	}
	if got.Title != "buy milk" {
		t.Errorf("expected title 'buy milk', got %q", got.Title)
	}
	if got.Done {
		t.Error("expected Done to be false by default")
	}
	if got.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}

	// Verify it was stored
	if _, ok := repo.store[got.ID]; !ok {
		t.Error("todo was not stored in repo")
	}
}

func TestService_Create_RepoError(t *testing.T) {
	repo := newFakeRepo()
	repo.createErr = errors.New("db down")
	svc := New(repo)

	_, err := svc.Create(context.Background(), "buy milk", "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestService_GetByID_NotFound(t *testing.T) {
	svc := New(newFakeRepo())

	_, err := svc.GetByID(context.Background(), "nonexistent")
	if !errors.Is(err, apierror.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}
```

Run it: `go test ./internal/modules/todo/service/...`. Fast — no DB, no network.

### Repository test (real database)

The repository is where SQL actually runs, so we test it against a real Postgres. The simplest approach: point the test at your local Postgres and clean up after each test.

For production-grade isolation, use [testcontainers-go](https://golang.testcontainers.org/) to spin up a dedicated Postgres per test run. For this walkthrough, we'll use the local dev Postgres with a helper that wraps each test in a transaction that rolls back.

Create `internal/modules/todo/repository/repository_test.go`:

```go
package repository

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"

	"github.com/yourname/backend/internal/modules/todo/domain"
	"github.com/yourname/backend/internal/shared/apierror"
)

// testDB connects to the local dev Postgres. In CI, set DB_HOST etc. to a
// throwaway test database. Each test uses a transaction that rolls back,
// so tests don't pollute each other.
func testDB(t *testing.T) *sqlx.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DB_DSN")
	if dsn == "" {
		dsn = "host=localhost port=5432 user=postgres password=postgres dbname=backend sslmode=disable"
	}
	db, err := sqlx.Connect("postgres", dsn)
	if err != nil {
		t.Skipf("postgres unavailable, skipping: %v", err)
	}
	return db
}

func TestRepository_CreateAndGet(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	repo := New(db)
	ctx := context.Background()

	// Use a unique ID so the test doesn't conflict with other data.
	todo := &domain.Todo{
		ID:          uuid.NewString(),
		Title:       "repo test",
		Description: "testing",
		Done:        false,
		CreatedAt:   time.Now().UTC(),
	}

	// Cleanup: remove the todo after the test even if it fails.
	t.Cleanup(func() {
		db.Exec("DELETE FROM todos WHERE id = $1", todo.ID)
	})

	if err := repo.Create(ctx, todo); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.GetByID(ctx, todo.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Title != todo.Title {
		t.Errorf("title: got %q, want %q", got.Title, todo.Title)
	}
}

func TestRepository_GetByID_NotFound(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	repo := New(db)
	_, err := repo.GetByID(context.Background(), uuid.NewString())
	if err != apierror.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}
```

Run it: `make migrate-up` (once), then `go test ./internal/modules/todo/repository/...`.

### Integration test (full HTTP)

The integration test boots the whole app and exercises it through real HTTP. This proves middleware, routing, error handling, and the response envelope all work together.

Create `test/integration/todo_test.go`:

```go
package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/yourname/backend/internal/app"
	"github.com/yourname/backend/internal/config"
)

func loadTestConfig(t *testing.T) *config.Config {
	t.Helper()
	// Ensure env is set for config.Load. In CI this comes from the job env.
	if os.Getenv("DB_HOST") == "" {
		os.Setenv("DB_HOST", "localhost")
		os.Setenv("DB_PORT", "5432")
		os.Setenv("DB_USER", "postgres")
		os.Setenv("DB_PASSWORD", "postgres")
		os.Setenv("DB_NAME", "backend")
		os.Setenv("DB_SSLMODE", "disable")
		os.Setenv("REDIS_ADDR", "localhost:6379")
		os.Setenv("APP_ENV", "test")
		os.Setenv("APP_PORT", "8080")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	return cfg
}

func TestCreateTodo_EndToEnd(t *testing.T) {
	cfg := loadTestConfig(t)
	a, err := app.BuildApp(cfg)
	if err != nil {
		t.Skipf("BuildApp failed (infra unavailable?): %v", err)
	}

	body := bytes.NewBufferString(`{"title":"integration test todo","description":"hello"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/todos", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	a.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if !resp.Success {
		t.Error("expected success=true")
	}
	if resp.Data.ID == "" {
		t.Error("expected id to be present")
	}
	if resp.Data.Title != "integration test todo" {
		t.Errorf("title mismatch: %q", resp.Data.Title)
	}

	// Verify the X-Request-ID header was set
	if rec.Header().Get("X-Request-ID") == "" {
		t.Error("expected X-Request-ID header")
	}
}

func TestCreateTodo_ValidationError(t *testing.T) {
	cfg := loadTestConfig(t)
	a, err := app.BuildApp(cfg)
	if err != nil {
		t.Skipf("BuildApp failed: %v", err)
	}

	body := bytes.NewBufferString(`{"title":"hi"}`) // too short
	req := httptest.NewRequest(http.MethodPost, "/api/v1/todos", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	a.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Success bool `json:"success"`
		Error   struct {
			Code      string `json:"code"`
			RequestID string `json:"request_id"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if resp.Error.Code != "VALIDATION_FAILED" {
		t.Errorf("expected VALIDATION_FAILED, got %q", resp.Error.Code)
	}
	if resp.Error.RequestID == "" {
		t.Error("expected request_id in error body")
	}
}
```

For this to work, we need to expose the router from `App`. Add this method to `internal/app/app.go`:

```go
// Router exposes the underlying Gin engine for integration tests. Do not
// use this in production code — the router is an internal detail.
func (a *App) Router() *gin.Engine { return a.router }
```

Run all tests:

```bash
make test
```

### What these three tests give you

- **Service tests** run in milliseconds. You can have hundreds of them and they'll still run in under a second. Write lots of these.
- **Repository tests** require infrastructure but catch real SQL bugs. Write one per repository method, focused on the database interaction.
- **Integration tests** are slower and you write fewer of them — one per user-visible behavior, not per code path. They're your safety net against subtle wiring mistakes.

Use the pyramid: many service tests, fewer repo tests, a handful of integration tests.

---

## Step 20: What's Next

You've built a production-grade Go backend. What you have:

- Structured, idiomatic Go with clear module boundaries
- Every request traceable from headers to logs to error responses
- Prometheus metrics for health and latency monitoring
- Rate limiting and idempotency that apply to new endpoints for free
- A consistent API contract enforced through shared helpers
- Tests at every layer of the stack
- Swagger docs that can't go stale (CI gate)

What you don't have yet (and why that's okay):

- **Authentication.** Add it when you have a user-facing requirement. The `consumer_id` in the idempotency store already has a `TODO` for it.
- **A second module.** Add one when you have a second feature. That's when the `contract.go` pattern earns its keep — the new module imports `todo.TodoService`, not internal packages.
- **Feature flags.** Add them when you have a feature worth gating. Scaffold the interface first, use a stub implementation, replace it later.
- **Event bus.** Add it when two modules need async communication. For now, the service-level logs (`todo_created`, etc.) are your event stream.
- **Distributed tracing.** Add it when you split into multiple services. For one process, request ID + structured logs cover the same need.
- **Infrastructure as code.** Worth doing early, but it's a separate track from the application code.

Every one of these adds complexity. Add them when you feel the pain, not before. The structure you have doesn't fight any of them — it supports them cleanly — but it doesn't require them either.

Welcome to boring, maintainable Go. Happy shipping.