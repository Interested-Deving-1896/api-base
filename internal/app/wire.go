// This file is the wiring diagram for the whole application. Read it top
// to bottom to see how every component connects.
//
// Ordering matters:
//  1. Logger and metrics init first — so everything else can log/measure.
//  2. Infrastructure (db, redis) — so higher layers can use it.
//  3. Shared utilities (validator, rate limiter, idempotency store).
//  4. Modules (todo, and later others).
//  5. Router — middleware chain, then routes.
//
// When you add a new module, you'll touch this file in two places: build
// the module with its New function, then register its routes.
package app

import (
	"fmt"

	"github.com/gin-gonic/gin"
	_ "github.com/topboyasante/api-base/api/docs" // blank import so swaggo docs are registered

	"github.com/topboyasante/api-base/internal/config"
	"github.com/topboyasante/api-base/internal/modules/todo"
	"github.com/topboyasante/api-base/internal/modules/uploads"
	"github.com/topboyasante/api-base/internal/observability/logger"
	"github.com/topboyasante/api-base/internal/observability/metrics"
	"github.com/topboyasante/api-base/internal/platform/postgres"
	platformredis "github.com/topboyasante/api-base/internal/platform/redis"
	platformvalidator "github.com/topboyasante/api-base/internal/platform/validator"
	"github.com/topboyasante/api-base/internal/providers/storage"
	"github.com/topboyasante/api-base/internal/providers/storage/local"
	"github.com/topboyasante/api-base/internal/providers/storage/r2"
	"github.com/topboyasante/api-base/internal/providers/storage/s3"
	"github.com/topboyasante/api-base/internal/shared/idempotency"
	"github.com/topboyasante/api-base/internal/shared/middleware"
	"github.com/topboyasante/api-base/internal/shared/ratelimit"
	"github.com/topboyasante/api-base/internal/shared/response"
)

func Build(cfg *config.Config) (*App, error) {
	// 1. Observability
	logger.Init(cfg.App.Env)
	metrics.Init()

	// 2. Infrastructure
	db, err := postgres.New(cfg.DB)
	if err != nil {
		return nil, err
	}
	if err := postgres.Migrate(db); err != nil {
		return nil, err
	}
	rdb, err := platformredis.New(cfg.Redis)
	if err != nil {
		return nil, err
	}

	storageReg := storage.NewRegistry()
	storageReg.Register("s3", s3.New)
	storageReg.Register("local", local.New)
	storageReg.Register("r2", r2.New)

	activeStorage, err := storageReg.Resolve(cfg.Storage.Provider, cfg.Storage.Options["s3"])
	if err != nil {
		return nil, fmt.Errorf("resolve storage: %w", err)
	}

	// 3. Shared utilities
	v := platformvalidator.New()
	rlim := ratelimit.New(rdb)
	idemStore := idempotency.NewStore(db)

	// 4. Modules
	todoHandler := todo.New(db, v)
	uploadsHandler := uploads.New(activeStorage)


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
	if cfg.App.Env != "production" {
		registerDocs(r)
	}

	// API group — rate-limited
	api := r.Group("/api/v1", ratelimit.Middleware(rlim, cfg.RateLimit.PerIPHourly))

	// Read endpoints: rate-limited only
	todoHandler.RegisterQueryRoutes(api)

	// Write endpoints: rate-limited AND idempotency-protected
	mutations := api.Group("", idempotency.Middleware(idemStore))
	todoHandler.RegisterMutationRoutes(mutations)
	uploadsHandler.RegisterRoutes(mutations)

	return &App{
		router:  r,
		db:      db,
		redis:   rdb,
		port:    cfg.App.Port,
		storage: activeStorage,
	}, nil
}
