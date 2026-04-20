# api-base

A modular-monolith Go backend scaffold. Gin + Postgres + Redis, wired with observability, rate limiting, idempotency, embedded migrations, and auto-generated API docs.


---

## Quickstart

```bash
# 1. copy env and adjust if needed
cp .env.example .env

# 2. start Postgres + Redis
docker compose up -d

# 3. run the app (migrations run automatically on boot)
make run
# or with hot-reload via air:
make dev
```

- Health: <http://localhost:8080/health>
- Metrics: <http://localhost:8080/metrics>
- API docs (non-prod): <http://localhost:8080/docs> (Scalar UI, backed by `/docs/openapi.json`)

---

## Make targets

| Target              | What it does                                              |
| ------------------- | --------------------------------------------------------- |
| `make run`          | `go run ./cmd/api`                                        |
| `make dev`          | hot-reload via `air`                                      |
| `make build`        | build `bin/api`                                           |
| `make test`         | run all tests                                             |
| `make docs`         | regenerate Swagger/OpenAPI spec from swaggo annotations   |
| `make docs-check`   | fail if generated docs are out of date (used by CI)       |
| `make migrate-up`   | apply pending migrations via `golang-migrate`             |
| `make migrate-down` | roll back the last migration                              |
| `make lint`         | `go vet ./...`                                            |

---

## Project layout

```
cmd/api/                  — binary entrypoint
internal/
  app/                    — dependency wiring + /docs routes
  config/                 — env-driven config loader (single source of truth)
  modules/<name>/         — feature modules (domain, dto, repository, service, handler, mapper)
  platform/
    postgres/             — *sqlx.DB + embedded migrations (migrations/*.sql)
    redis/                — go-redis client
    server/               — http.Server with graceful shutdown
    validator/            — go-playground/validator instance
  shared/                 — cross-cutting utilities used by every module
    apierror/             — typed API errors
    idempotency/          — idempotency store + middleware
    middleware/           — request-id, logger, recover, error-handler
    ratelimit/            — redis-backed per-IP limiter
    requestctx/           — context keys (request_id, etc.)
    response/             — standardized response envelope
  observability/
    logger/               — slog with request-scoped fields
    metrics/              — prometheus metrics + middleware
api/docs/                 — generated swagger artifacts (committed)
scripts/migrate.sh        — migrate CLI wrapper (sources .env)
.github/workflows/        — docs freshness check + manual migration workflow
```

Every new feature goes into `internal/modules/<name>/` following the same shape as `todo`. Wiring happens in one place: `internal/app/wire.go`.

---

## Configuration

All config comes from environment variables, loaded once via `internal/config`. `.env` is auto-loaded in development by `godotenv`; in production, the orchestrator (Kubernetes, Fly, systemd, etc.) provides them.

See [`.env.example`](./.env.example) for the full list.

---

## Database migrations

Migrations live in `internal/platform/postgres/migrations/` and are **embedded into the binary** via `//go:embed`. `postgres.Migrate(db)` runs on startup, applies anything pending, and is safe under multiple replicas (golang-migrate takes an advisory lock).

For ad-hoc work (force a version, inspect state, roll back further than 1):

```bash
./scripts/migrate.sh version
./scripts/migrate.sh down 3
./scripts/migrate.sh force 2
```

For staging/production, a manually-triggered workflow lives at `.github/workflows/migrate.yaml` — pick environment + subcommand from the Actions UI. Wire the DB secrets into the `staging` / `production` GitHub Environments before using it.

---

## API docs

- Swaggo annotations on handlers → `make docs` regenerates `api/docs/{docs.go,swagger.json,swagger.yaml}`.
- In non-production environments, the app serves:
  - `/docs` → Scalar UI
  - `/docs/openapi.json` → raw OpenAPI spec
- CI (`.github/workflows/api-docs.yaml`) fails PRs that didn't regenerate docs after changing handler annotations.

---

## Adding a new module

1. `mkdir internal/modules/<name>/{domain,dto,handler,mapper,repository,service}`
2. Follow the shape of `internal/modules/todo/`.
3. Register its routes and construct its handler in `internal/app/wire.go`.
4. Add a migration if you need schema changes: `internal/platform/postgres/migrations/0003_<name>.up.sql` + `.down.sql`.
5. `make docs` to pick up new swaggo annotations.
