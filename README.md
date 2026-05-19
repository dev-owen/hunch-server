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

## Account Auth

The account API supports email/password signup, email/password signin, Google signin, Kakao signin, signout, and account deletion.

Auth-related environment variables:

- `SESSION_COOKIE_NAME`: session cookie name, default `hunch_session`.
- `SESSION_TTL_HOURS`: session lifetime in hours, default `720`.
- `BCRYPT_COST`: password hash cost, default `12`.
- `GOOGLE_USERINFO_URL`: Google OpenID Connect userinfo endpoint override.
- `KAKAO_USERINFO_URL`: Kakao user information endpoint override.

Session cookies are `HttpOnly`, `SameSite=Lax`, and `Secure` outside `APP_ENV=local`.

### Signup

```bash
curl -i -X POST http://localhost:8080/signup \
  -H 'Content-Type: application/json' \
  -d '{"email":"user@example.com","password":"password123","display_name":"User"}'
```

### Signin With Email

```bash
curl -i -X POST http://localhost:8080/signin \
  -H 'Content-Type: application/json' \
  -d '{"method":"email","email":"user@example.com","password":"password123"}'
```

### Signin With Google Or Kakao

```bash
curl -i -X POST http://localhost:8080/signin \
  -H 'Content-Type: application/json' \
  -d '{"method":"google","access_token":"provider-access-token"}'
```

```bash
curl -i -X POST http://localhost:8080/signin \
  -H 'Content-Type: application/json' \
  -d '{"method":"kakao","access_token":"provider-access-token"}'
```

### Signout

```bash
curl -i -X POST http://localhost:8080/signout \
  -b 'hunch_session=session-token'
```

### Delete Account

```bash
curl -i -X DELETE http://localhost:8080/delete \
  -b 'hunch_session=session-token'
```
