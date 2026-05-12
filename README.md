# Hunch Server

Go server foundation for Hunch.

## Stack

- HTTP: `net/http` + chi
- Database: PostgreSQL + pgvector
- Driver: pgx
- SQL layer: sqlc
- Migrations: goose
- Worker: Postgres-backed queue foundation
- Object storage: S3 / Cloudflare R2 compatible interface
- AI: OpenAI API client boundary
- Observability: OpenTelemetry + Prometheus
- Tests: Go testing + testcontainers

## Quickstart

```bash
cp .env.example .env
docker compose up -d postgres prometheus
export DATABASE_URL='postgres://hunch:hunch@localhost:5432/hunch?sslmode=disable'
make migrate-up
make run
```

Health endpoints:

- `GET /healthz`
- `GET /readyz`
- `GET /metrics`

The API listens on `:8080` by default. Override it with `HTTP_ADDR` in `.env` or the shell environment.

## Local Setup

Install the Go toolchain and Docker Desktop, then start the local services:

```bash
docker compose up -d postgres prometheus
```

Run migrations and start the API:

```bash
export DATABASE_URL='postgres://hunch:hunch@localhost:5432/hunch?sslmode=disable'
make migrate-up
make run
```

Prometheus is available at `http://localhost:9090` and scrapes the API metrics endpoint at `http://localhost:8080/metrics`.

## Development

```bash
make fmt
make test
make sqlc
make migrate-up
```
