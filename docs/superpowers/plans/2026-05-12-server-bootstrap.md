# Server Bootstrap Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the initial Hunch Go server skeleton with HTTP, PostgreSQL/pgvector, pgx/sqlc, goose, Postgres-backed worker, OpenAI, S3/R2, and observability foundations.

**Architecture:** `cmd/api` wires small `internal/` packages. Runtime dependencies are injected into an HTTP router that exposes health, readiness, and Prometheus metrics. Database schema and typed queries live under `db/`.

**Tech Stack:** Go, net/http, chi, pgx, sqlc, goose, PostgreSQL pgvector, OpenTelemetry, Prometheus, testcontainers.

---

### Task 1: Project Metadata

**Files:**
- Create: `.gitignore`
- Create: `.env.example`
- Create: `Makefile`
- Create: `README.md`
- Create: `docker-compose.yml`
- Create: `sqlc.yaml`

- [x] Add local development metadata, commands, and service configuration.

### Task 2: Config And HTTP Health

**Files:**
- Create: `internal/config/config_test.go`
- Create: `internal/config/config.go`
- Create: `internal/httpserver/router_test.go`
- Create: `internal/httpserver/router.go`

- [x] Write tests first for environment defaults and health responses.
- [x] Implement minimal config loader and chi router.

### Task 3: Runtime Wiring

**Files:**
- Create: `cmd/api/main.go`
- Create: `internal/db/db.go`
- Create: `internal/observability/observability.go`
- Create: `internal/ai/client.go`
- Create: `internal/storage/storage.go`
- Create: `internal/worker/worker.go`

- [x] Add startup wiring and boundaries for DB, OpenAI, S3/R2, and worker code.

### Task 4: Database Assets

**Files:**
- Create: `db/migrations/00001_init.sql`
- Create: `db/queries/health.sql`
- Create: `db/queries/jobs.sql`
- Create: `internal/db/migration_test.go`

- [x] Add pgvector schema, sqlc query definitions, and a testcontainers migration test.

### Task 5: Verification

**Files:**
- Modify: `go.mod`
- Create: `go.sum`

- [x] Run `go mod tidy`.
- [x] Run `gofmt`.
- [x] Run `go test ./...`.
