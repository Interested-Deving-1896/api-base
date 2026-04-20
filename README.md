# api-base

A modular-monolith Go backend scaffold. Gin + Postgres + Redis, wired with observability, rate limiting, idempotency, embedded migrations, and auto-generated API docs.

This repo is a **GitHub template**. Click **Use this template** on the repo page to start a new project, or clone it manually and rename the Go module:

```bash
go mod edit -module github.com/<you>/<your-api>
# rewrite imports
find . -type f -name '*.go' -exec sed -i '' 's|github.com/topboyasante/api-base|github.com/<you>/<your-api>|g' {} +
go mod tidy
```

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
| `make gen`          | scaffold a new feature module (see below)                 |
| `make gen-check`    | verify generator templates stay in sync with `todo/`      |

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

Use the generator. `todo/` is the reference module — the generator's `full/` templates render byte-for-byte to it, and `make gen-check` enforces that.

```bash
# full CRUD scaffold (domain, dto, handler, mapper, repository, service)
make gen MODULE=users

# module already plural / irregular plural
make gen MODULE=post PLURAL=posts
make gen MODULE=entity PLURAL=entities

# minimal scaffold (no DB layer) — for endpoints that proxy, aggregate, or have no state
make gen MODULE=health MINIMAL=1
```

After running, the generator prints the 3 snippets to paste into `internal/app/wire.go` (import, construct, register routes). Then:

1. Add a migration if the module needs schema: `internal/platform/postgres/migrations/NNNN_<plural>.up.sql` + `.down.sql`.
2. `make docs` to pick up new swaggo annotations.

### Keeping the generator honest

`todo/` stays the canonical reference module. If you change `todo/`, update `cmd/gen/templates/full/` to match — `make gen-check` diffs the generator's output against `todo/` byte-for-byte and fails CI on drift.

---

## License

MIT — see [`LICENSE`](./LICENSE).
