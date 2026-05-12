# Hunch Server Bootstrap Design

## Goal

Create a production-minded Go server skeleton for Hunch using a small HTTP surface, PostgreSQL with pgvector, pgx/sqlc, goose migrations, a Postgres-backed worker foundation, S3/R2-compatible object storage, OpenAI API integration, and observability hooks.

## Architecture

The app starts from `cmd/api` and wires configuration, logging, PostgreSQL, tracing, metrics, and HTTP routing. HTTP uses `net/http` plus chi so the project stays close to the standard library while still getting composable routing and middleware.

Core packages live under `internal/`:

- `internal/config`: environment-backed settings with explicit defaults.
- `internal/httpserver`: chi router and health endpoints.
- `internal/db`: pgx pool creation and health checks.
- `internal/observability`: OpenTelemetry resource setup and Prometheus metrics registration.
- `internal/ai`: minimal OpenAI API client boundary.
- `internal/storage`: S3/R2-compatible storage boundary.
- `internal/worker`: Postgres-backed job enqueue/dequeue primitives.

## Data

The first goose migration enables `vector` and `pgcrypto`, then creates `documents`, `document_chunks`, and `jobs`. `document_chunks.embedding` uses `vector(1536)` as a practical default for OpenAI embedding models. Query definitions for sqlc cover health pings and job queue operations.

## Operations

Local development uses Docker Compose for PostgreSQL with pgvector and a Prometheus service. `Makefile` commands cover formatting, tests, SQL generation, migrations, and running the API. The API exposes `/healthz`, `/readyz`, and `/metrics`.

## Testing

Unit tests verify configuration defaults and health endpoint behavior. A testcontainers integration test is included for PostgreSQL migrations and can run when Docker is available.
