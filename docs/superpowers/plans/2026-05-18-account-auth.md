# Account Authentication Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build Account business logic for `/signup`, `/signin`, `/signout`, and `/delete` with email/password, Google, and Kakao login methods.

**Architecture:** Keep account behavior in a new `internal/account` package with small domain helpers, provider verification, PostgreSQL persistence, service orchestration, and HTTP handlers. `internal/httpserver` stays responsible for routing and middleware composition, while `cmd/api` wires config, database pool, OAuth verifier, and account service dependencies.

**Tech Stack:** Go, net/http, chi, pgx, sqlc, PostgreSQL, goose, bcrypt from `golang.org/x/crypto/bcrypt`, opaque cookie sessions, Google OpenID Connect userinfo, Kakao Login REST API.

---

## External Provider References

- Google profile verification uses the OpenID Connect userinfo flow documented by Google: `https://developers.google.com/identity/openid-connect/openid-connect`.
- Kakao profile verification uses Kakao Login's Retrieve user information endpoint: `https://developers.kakao.com/docs/en/kakaologin/rest-api`.
- The first implementation accepts provider access tokens at `POST /signin`. It does not implement browser redirect/callback endpoints because the requested public surface is `/signup`, `/signin`, `/signout`, and `/delete`.

## Route Contract

- `POST /signup`: email/password registration.
- `POST /signin`: email/password signin or social signin with `method` set to `google` or `kakao`.
- `POST /signout`: revoke the current session cookie.
- `DELETE /delete`: soft-delete the authenticated account and revoke all account sessions.

Session behavior:

- Server returns an opaque random session token in an HttpOnly cookie.
- Database stores only `sha256` session token hashes.
- Cookie defaults: name `hunch_session`, `HttpOnly`, `SameSite=Lax`, `Path=/`, `Max-Age` matching session TTL.
- Set `Secure` when `APP_ENV` is not `local`.

## File Structure

- Create `db/migrations/00002_accounts.sql`: account, identity, and session tables.
- Create `db/queries/accounts.sql`: sqlc query definitions for account flows.
- Regenerate `internal/db/dbgen/*.go`: generated models and methods for account queries.
- Modify `internal/config/config.go`: auth config values.
- Modify `internal/config/config_test.go`: auth config defaults and validation.
- Modify `.env.example`: local auth and provider settings.
- Create `internal/account/types.go`: public account request/result/domain types.
- Create `internal/account/errors.go`: sentinel errors mapped by handlers.
- Create `internal/account/password.go`: email normalization, password validation, bcrypt hashing.
- Create `internal/account/password_test.go`: domain helper tests.
- Create `internal/account/session.go`: session token generation and hashing.
- Create `internal/account/session_test.go`: session helper tests.
- Create `internal/account/oauth.go`: Google and Kakao access-token profile verifier.
- Create `internal/account/oauth_test.go`: provider verifier tests with `httptest.Server`.
- Create `internal/account/repository.go`: repository interface used by service tests.
- Create `internal/account/postgres_repository.go`: repository backed by sqlc and pgx transactions.
- Create `internal/account/service.go`: signup, signin, signout, delete, and authenticate business logic.
- Create `internal/account/service_test.go`: service tests with an in-memory fake repository.
- Create `internal/account/handlers.go`: HTTP request parsing, response writing, cookie handling.
- Create `internal/account/handlers_test.go`: endpoint tests.
- Modify `internal/httpserver/router.go`: mount account routes and dependencies.
- Modify `internal/httpserver/router_test.go`: preserve health route tests and add account dependency smoke tests.
- Modify `cmd/api/main.go`: wire account repository, OAuth verifier, service, and handlers.
- Modify `README.md`: document auth endpoints and local provider env.

## Data Model

`accounts` is the canonical user record. `account_identities` links login methods to an account. `account_sessions` stores revocable opaque sessions.

Social signin behavior:

- If `(provider, provider_subject)` exists, sign in that account.
- If no identity exists and the provider returns a verified email matching an active account, link the provider identity to that account.
- If no matching account exists, create an account and identity.
- If Kakao does not return email because consent is missing, create the account with `email` and `normalized_email` as `NULL`.

---

### Task 1: Auth Config

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Modify: `.env.example`

- [ ] **Step 1: Write failing config tests**

Append these tests to `internal/config/config_test.go`:

```go
func TestLoadAppliesAuthDefaults(t *testing.T) {
	t.Setenv("SESSION_COOKIE_NAME", "")
	t.Setenv("SESSION_TTL_HOURS", "")
	t.Setenv("BCRYPT_COST", "")
	t.Setenv("GOOGLE_USERINFO_URL", "")
	t.Setenv("KAKAO_USERINFO_URL", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.SessionCookieName != "hunch_session" {
		t.Fatalf("SessionCookieName = %q, want hunch_session", cfg.SessionCookieName)
	}
	if cfg.SessionTTLHours != 24*30 {
		t.Fatalf("SessionTTLHours = %d, want %d", cfg.SessionTTLHours, 24*30)
	}
	if cfg.BcryptCost != 12 {
		t.Fatalf("BcryptCost = %d, want 12", cfg.BcryptCost)
	}
	if cfg.GoogleUserInfoURL != "https://openidconnect.googleapis.com/v1/userinfo" {
		t.Fatalf("GoogleUserInfoURL = %q", cfg.GoogleUserInfoURL)
	}
	if cfg.KakaoUserInfoURL != "https://kapi.kakao.com/v2/user/me" {
		t.Fatalf("KakaoUserInfoURL = %q", cfg.KakaoUserInfoURL)
	}
}

func TestLoadRejectsInvalidAuthNumbers(t *testing.T) {
	t.Setenv("SESSION_TTL_HOURS", "0")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() error = nil, want invalid ttl error")
	}

	t.Setenv("SESSION_TTL_HOURS", "24")
	t.Setenv("BCRYPT_COST", "3")

	_, err = Load()
	if err == nil {
		t.Fatal("Load() error = nil, want invalid bcrypt cost error")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/config -run 'TestLoadAppliesAuthDefaults|TestLoadRejectsInvalidAuthNumbers' -v`

Expected: FAIL because `Config` does not yet expose `SessionCookieName`, `SessionTTLHours`, `BcryptCost`, `GoogleUserInfoURL`, or `KakaoUserInfoURL`.

- [ ] **Step 3: Add auth config fields**

Modify `internal/config/config.go`:

```go
type Config struct {
	AppEnv      string
	HTTPAddr    string
	DatabaseURL string

	SessionCookieName string
	SessionTTLHours   int
	BcryptCost        int
	GoogleUserInfoURL string
	KakaoUserInfoURL  string

	OpenAIAPIKey  string
	OpenAIBaseURL string

	S3Endpoint        string
	S3Region          string
	S3Bucket          string
	S3AccessKeyID     string
	S3SecretAccessKey string
	S3UsePathStyle    bool

	OTLPTraceEndpoint string
}
```

Add integer env parsing near `envBool`:

```go
func envInt(key string, fallback int) (int, error) {
	value := os.Getenv(key)
	if value == "" {
		return fallback, nil
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", key, err)
	}
	return parsed, nil
}
```

Update `Load` before the returned `Config`:

```go
	sessionTTLHours, err := envInt("SESSION_TTL_HOURS", 24*30)
	if err != nil {
		return Config{}, err
	}
	if sessionTTLHours <= 0 {
		return Config{}, fmt.Errorf("SESSION_TTL_HOURS must be positive")
	}

	bcryptCost, err := envInt("BCRYPT_COST", 12)
	if err != nil {
		return Config{}, err
	}
	if bcryptCost < 4 || bcryptCost > 31 {
		return Config{}, fmt.Errorf("BCRYPT_COST must be between 4 and 31")
	}
```

Add these fields to the returned `Config`:

```go
		SessionCookieName: envString("SESSION_COOKIE_NAME", "hunch_session"),
		SessionTTLHours:   sessionTTLHours,
		BcryptCost:        bcryptCost,
		GoogleUserInfoURL: envString("GOOGLE_USERINFO_URL", "https://openidconnect.googleapis.com/v1/userinfo"),
		KakaoUserInfoURL:  envString("KAKAO_USERINFO_URL", "https://kapi.kakao.com/v2/user/me"),
```

Add to `.env.example`:

```dotenv
SESSION_COOKIE_NAME=hunch_session
SESSION_TTL_HOURS=720
BCRYPT_COST=12
GOOGLE_USERINFO_URL=https://openidconnect.googleapis.com/v1/userinfo
KAKAO_USERINFO_URL=https://kapi.kakao.com/v2/user/me
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/config -v`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go .env.example
git commit -m "feat: add auth configuration"
```

---

### Task 2: Account Schema And SQLC Queries

**Files:**
- Create: `db/migrations/00002_accounts.sql`
- Create: `db/queries/accounts.sql`
- Modify: `internal/db/migration_test.go`
- Regenerate: `internal/db/dbgen/*.go`

- [ ] **Step 1: Write failing migration assertion**

Append this assertion after `goose.Up` in `internal/db/migration_test.go`:

```go
	var tableCount int
	if err := conn.QueryRow(`
		SELECT count(*)
		FROM information_schema.tables
		WHERE table_schema = 'public'
			AND table_name IN ('accounts', 'account_identities', 'account_sessions')
	`).Scan(&tableCount); err != nil {
		t.Fatalf("count account tables: %v", err)
	}
	if tableCount != 3 {
		t.Fatalf("account table count = %d, want 3", tableCount)
	}
```

- [ ] **Step 2: Run migration test to verify it fails**

Run: `RUN_INTEGRATION_TESTS=1 go test ./internal/db -run TestMigrationsApply -v`

Expected: FAIL with `account table count = 0, want 3`.

- [ ] **Step 3: Add account migration**

Create `db/migrations/00002_accounts.sql`:

```sql
-- +goose Up
CREATE TABLE accounts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email text,
    normalized_email text,
    display_name text NOT NULL DEFAULT '',
    avatar_url text,
    status text NOT NULL DEFAULT 'active',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz,
    CONSTRAINT accounts_status_check CHECK (status IN ('active', 'deleted')),
    CONSTRAINT accounts_email_pair_check CHECK (
        (email IS NULL AND normalized_email IS NULL)
        OR (email IS NOT NULL AND normalized_email IS NOT NULL)
    )
);

CREATE UNIQUE INDEX accounts_active_normalized_email_idx
    ON accounts (normalized_email)
    WHERE deleted_at IS NULL AND normalized_email IS NOT NULL;

CREATE TABLE account_identities (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id uuid NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    provider text NOT NULL,
    provider_subject text NOT NULL,
    email text,
    normalized_email text,
    email_verified boolean NOT NULL DEFAULT false,
    password_hash text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT account_identities_provider_check CHECK (provider IN ('email', 'google', 'kakao')),
    CONSTRAINT account_identities_email_password_check CHECK (
        provider <> 'email'
        OR (normalized_email IS NOT NULL AND password_hash IS NOT NULL)
    )
);

CREATE UNIQUE INDEX account_identities_provider_subject_idx
    ON account_identities (provider, provider_subject);

CREATE UNIQUE INDEX account_identities_email_idx
    ON account_identities (normalized_email)
    WHERE provider = 'email' AND normalized_email IS NOT NULL;

CREATE TABLE account_sessions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id uuid NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    token_hash text NOT NULL UNIQUE,
    user_agent text NOT NULL DEFAULT '',
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX account_sessions_active_idx
    ON account_sessions (token_hash, expires_at)
    WHERE revoked_at IS NULL;

-- +goose Down
DROP TABLE IF EXISTS account_sessions;
DROP TABLE IF EXISTS account_identities;
DROP TABLE IF EXISTS accounts;
```

- [ ] **Step 4: Add sqlc account queries**

Create `db/queries/accounts.sql`:

```sql
-- name: CreateAccount :one
INSERT INTO accounts (email, normalized_email, display_name, avatar_url)
VALUES ($1, $2, $3, $4)
RETURNING id, email, normalized_email, display_name, avatar_url, status, created_at, updated_at, deleted_at;

-- name: FindActiveAccountByNormalizedEmail :one
SELECT id, email, normalized_email, display_name, avatar_url, status, created_at, updated_at, deleted_at
FROM accounts
WHERE normalized_email = $1
  AND deleted_at IS NULL
  AND status = 'active';

-- name: SoftDeleteAccount :exec
UPDATE accounts
SET status = 'deleted',
    deleted_at = now(),
    updated_at = now()
WHERE id = $1
  AND deleted_at IS NULL;

-- name: CreateAccountIdentity :one
INSERT INTO account_identities (
    account_id,
    provider,
    provider_subject,
    email,
    normalized_email,
    email_verified,
    password_hash
)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, account_id, provider, provider_subject, email, normalized_email, email_verified, password_hash, created_at, updated_at;

-- name: FindIdentityWithAccount :one
SELECT
    ai.id,
    ai.account_id,
    ai.provider,
    ai.provider_subject,
    ai.email,
    ai.normalized_email,
    ai.email_verified,
    ai.password_hash,
    ai.created_at,
    ai.updated_at,
    a.id AS account_id_2,
    a.email AS account_email,
    a.normalized_email AS account_normalized_email,
    a.display_name AS account_display_name,
    a.avatar_url AS account_avatar_url,
    a.status AS account_status,
    a.created_at AS account_created_at,
    a.updated_at AS account_updated_at,
    a.deleted_at AS account_deleted_at
FROM account_identities ai
JOIN accounts a ON a.id = ai.account_id
WHERE ai.provider = $1
  AND ai.provider_subject = $2
  AND a.deleted_at IS NULL
  AND a.status = 'active';

-- name: FindEmailIdentityWithAccount :one
SELECT
    ai.id,
    ai.account_id,
    ai.provider,
    ai.provider_subject,
    ai.email,
    ai.normalized_email,
    ai.email_verified,
    ai.password_hash,
    ai.created_at,
    ai.updated_at,
    a.id AS account_id_2,
    a.email AS account_email,
    a.normalized_email AS account_normalized_email,
    a.display_name AS account_display_name,
    a.avatar_url AS account_avatar_url,
    a.status AS account_status,
    a.created_at AS account_created_at,
    a.updated_at AS account_updated_at,
    a.deleted_at AS account_deleted_at
FROM account_identities ai
JOIN accounts a ON a.id = ai.account_id
WHERE ai.provider = 'email'
  AND ai.normalized_email = $1
  AND a.deleted_at IS NULL
  AND a.status = 'active';

-- name: CreateAccountSession :one
INSERT INTO account_sessions (account_id, token_hash, user_agent, expires_at)
VALUES ($1, $2, $3, $4)
RETURNING id, account_id, token_hash, user_agent, expires_at, revoked_at, created_at;

-- name: FindAccountBySessionTokenHash :one
SELECT
    a.id,
    a.email,
    a.normalized_email,
    a.display_name,
    a.avatar_url,
    a.status,
    a.created_at,
    a.updated_at,
    a.deleted_at
FROM account_sessions s
JOIN accounts a ON a.id = s.account_id
WHERE s.token_hash = $1
  AND s.revoked_at IS NULL
  AND s.expires_at > now()
  AND a.deleted_at IS NULL
  AND a.status = 'active';

-- name: RevokeAccountSession :exec
UPDATE account_sessions
SET revoked_at = now()
WHERE token_hash = $1
  AND revoked_at IS NULL;

-- name: RevokeAllAccountSessions :exec
UPDATE account_sessions
SET revoked_at = now()
WHERE account_id = $1
  AND revoked_at IS NULL;
```

- [ ] **Step 5: Generate SQLC code**

Run: `make sqlc`

Expected: PASS and account query methods appear in `internal/db/dbgen`.

- [ ] **Step 6: Run migration test to verify it passes**

Run: `RUN_INTEGRATION_TESTS=1 go test ./internal/db -run TestMigrationsApply -v`

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add db/migrations/00002_accounts.sql db/queries/accounts.sql internal/db/migration_test.go internal/db/dbgen
git commit -m "feat: add account persistence schema"
```

---

### Task 3: Account Domain Helpers

**Files:**
- Create: `internal/account/types.go`
- Create: `internal/account/errors.go`
- Create: `internal/account/password.go`
- Create: `internal/account/password_test.go`
- Create: `internal/account/session.go`
- Create: `internal/account/session_test.go`

- [ ] **Step 1: Write failing password helper tests**

Create `internal/account/password_test.go`:

```go
package account

import "testing"

func TestNormalizeEmail(t *testing.T) {
	got, err := NormalizeEmail("  USER@Example.COM ")
	if err != nil {
		t.Fatalf("NormalizeEmail() error = %v", err)
	}
	if got != "user@example.com" {
		t.Fatalf("NormalizeEmail() = %q, want user@example.com", got)
	}
}

func TestNormalizeEmailRejectsInvalidEmail(t *testing.T) {
	_, err := NormalizeEmail("not-an-email")
	if err == nil {
		t.Fatal("NormalizeEmail() error = nil, want invalid email")
	}
}

func TestHashAndComparePassword(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple", 4)
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	if err := ComparePassword(hash, "correct horse battery staple"); err != nil {
		t.Fatalf("ComparePassword() error = %v", err)
	}
	if err := ComparePassword(hash, "wrong password"); err == nil {
		t.Fatal("ComparePassword() error = nil, want mismatch")
	}
}

func TestValidatePasswordRejectsShortPassword(t *testing.T) {
	if err := ValidatePassword("short"); err == nil {
		t.Fatal("ValidatePassword() error = nil, want short password error")
	}
}
```

- [ ] **Step 2: Write failing session helper tests**

Create `internal/account/session_test.go`:

```go
package account

import "testing"

func TestNewSessionTokenAndHash(t *testing.T) {
	token, err := NewSessionToken()
	if err != nil {
		t.Fatalf("NewSessionToken() error = %v", err)
	}
	if len(token) < 40 {
		t.Fatalf("token length = %d, want at least 40", len(token))
	}

	hash := HashSessionToken(token)
	if hash == "" {
		t.Fatal("HashSessionToken() returned empty hash")
	}
	if hash == token {
		t.Fatal("HashSessionToken() returned raw token")
	}
	if HashSessionToken(token) != hash {
		t.Fatal("HashSessionToken() is not deterministic")
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/account -run 'TestNormalizeEmail|TestHashAndComparePassword|TestValidatePassword|TestNewSessionTokenAndHash' -v`

Expected: FAIL because the `internal/account` package does not exist.

- [ ] **Step 4: Add domain types and errors**

Create `internal/account/errors.go`:

```go
package account

import "errors"

var (
	ErrInvalidInput        = errors.New("invalid input")
	ErrEmailAlreadyExists  = errors.New("email already exists")
	ErrInvalidCredentials  = errors.New("invalid credentials")
	ErrUnauthenticated     = errors.New("unauthenticated")
	ErrAccountDeleted      = errors.New("account deleted")
	ErrProviderUnavailable = errors.New("provider unavailable")
)
```

Create `internal/account/types.go`:

```go
package account

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

type Provider string

const (
	ProviderEmail  Provider = "email"
	ProviderGoogle Provider = "google"
	ProviderKakao  Provider = "kakao"
)

type Account struct {
	ID              pgtype.UUID `json:"id"`
	Email           string      `json:"email,omitempty"`
	NormalizedEmail string      `json:"-"`
	DisplayName     string      `json:"display_name"`
	AvatarURL       string      `json:"avatar_url,omitempty"`
	Status          string      `json:"status"`
	CreatedAt       time.Time   `json:"created_at"`
	UpdatedAt       time.Time   `json:"updated_at"`
}

type Session struct {
	RawToken  string
	TokenHash string
	ExpiresAt time.Time
}

type SignupEmailInput struct {
	Email       string
	Password    string
	DisplayName string
	UserAgent   string
}

type SigninInput struct {
	Method      Provider
	Email       string
	Password    string
	AccessToken string
	UserAgent   string
}

type AuthResult struct {
	Account Account
	Session Session
}

type ExternalProfile struct {
	Provider      Provider
	Subject       string
	Email         string
	EmailVerified bool
	DisplayName   string
	AvatarURL     string
}
```

- [ ] **Step 5: Add password helpers**

Create `internal/account/password.go`:

```go
package account

import (
	"fmt"
	"net/mail"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

func NormalizeEmail(email string) (string, error) {
	trimmed := strings.TrimSpace(email)
	if trimmed == "" {
		return "", fmt.Errorf("%w: email is required", ErrInvalidInput)
	}
	parsed, err := mail.ParseAddress(trimmed)
	if err != nil || parsed.Address != trimmed {
		return "", fmt.Errorf("%w: invalid email", ErrInvalidInput)
	}
	return strings.ToLower(trimmed), nil
}

func ValidatePassword(password string) error {
	if len(password) < 8 {
		return fmt.Errorf("%w: password must be at least 8 characters", ErrInvalidInput)
	}
	return nil
}

func HashPassword(password string, cost int) (string, error) {
	if err := ValidatePassword(password); err != nil {
		return "", err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), cost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func ComparePassword(hash string, password string) error {
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return ErrInvalidCredentials
	}
	return nil
}
```

- [ ] **Step 6: Add session helpers**

Create `internal/account/session.go`:

```go
package account

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
)

func NewSessionToken() (string, error) {
	var bytes [32]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes[:]), nil
}

func HashSessionToken(rawToken string) string {
	sum := sha256.Sum256([]byte(rawToken))
	return hex.EncodeToString(sum[:])
}
```

- [ ] **Step 7: Run helper tests to verify they pass**

Run: `go test ./internal/account -v`

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/account
git commit -m "feat: add account domain helpers"
```

---

### Task 4: OAuth Provider Verification

**Files:**
- Create: `internal/account/oauth.go`
- Create: `internal/account/oauth_test.go`

- [ ] **Step 1: Write failing OAuth verifier tests**

Create `internal/account/oauth_test.go`:

```go
package account

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPProviderVerifierVerifiesGoogleProfile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer google-token" {
			t.Fatalf("Authorization = %q", r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{
			"sub":"google-subject",
			"email":"user@example.com",
			"email_verified":true,
			"name":"User Name",
			"picture":"https://example.com/avatar.png"
		}`))
	}))
	defer server.Close()

	verifier := HTTPProviderVerifier{
		Client:            server.Client(),
		GoogleUserInfoURL: server.URL,
		KakaoUserInfoURL:  server.URL,
	}

	profile, err := verifier.Verify(context.Background(), ProviderGoogle, "google-token")
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if profile.Provider != ProviderGoogle || profile.Subject != "google-subject" {
		t.Fatalf("profile = %+v", profile)
	}
	if profile.Email != "user@example.com" || !profile.EmailVerified {
		t.Fatalf("email = %q verified=%v", profile.Email, profile.EmailVerified)
	}
}

func TestHTTPProviderVerifierVerifiesKakaoProfile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer kakao-token" {
			t.Fatalf("Authorization = %q", r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{
			"id":12345,
			"kakao_account":{
				"email":"kakao@example.com",
				"is_email_verified":true,
				"profile":{
					"nickname":"Kakao User",
					"profile_image_url":"https://example.com/kakao.png"
				}
			}
		}`))
	}))
	defer server.Close()

	verifier := HTTPProviderVerifier{
		Client:            server.Client(),
		GoogleUserInfoURL: server.URL,
		KakaoUserInfoURL:  server.URL,
	}

	profile, err := verifier.Verify(context.Background(), ProviderKakao, "kakao-token")
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if profile.Provider != ProviderKakao || profile.Subject != "12345" {
		t.Fatalf("profile = %+v", profile)
	}
	if profile.DisplayName != "Kakao User" {
		t.Fatalf("DisplayName = %q", profile.DisplayName)
	}
}

func TestHTTPProviderVerifierRejectsProviderError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "bad token", http.StatusUnauthorized)
	}))
	defer server.Close()

	verifier := HTTPProviderVerifier{
		Client:            server.Client(),
		GoogleUserInfoURL: server.URL,
		KakaoUserInfoURL:  server.URL,
	}

	_, err := verifier.Verify(context.Background(), ProviderGoogle, "bad-token")
	if err == nil {
		t.Fatal("Verify() error = nil, want provider error")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/account -run 'TestHTTPProviderVerifier' -v`

Expected: FAIL because `HTTPProviderVerifier` does not exist.

- [ ] **Step 3: Implement OAuth verifier**

Create `internal/account/oauth.go`:

```go
package account

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
)

type ProviderVerifier interface {
	Verify(ctx context.Context, provider Provider, accessToken string) (ExternalProfile, error)
}

type HTTPProviderVerifier struct {
	Client            *http.Client
	GoogleUserInfoURL string
	KakaoUserInfoURL  string
}

func (v HTTPProviderVerifier) Verify(ctx context.Context, provider Provider, accessToken string) (ExternalProfile, error) {
	if accessToken == "" {
		return ExternalProfile{}, fmt.Errorf("%w: access token is required", ErrInvalidInput)
	}
	switch provider {
	case ProviderGoogle:
		return v.verifyGoogle(ctx, accessToken)
	case ProviderKakao:
		return v.verifyKakao(ctx, accessToken)
	default:
		return ExternalProfile{}, fmt.Errorf("%w: unsupported provider", ErrInvalidInput)
	}
}

func (v HTTPProviderVerifier) httpClient() *http.Client {
	if v.Client != nil {
		return v.Client
	}
	return http.DefaultClient
}

func (v HTTPProviderVerifier) getJSON(ctx context.Context, url string, accessToken string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := v.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrProviderUnavailable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("%w: status %d", ErrInvalidCredentials, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return fmt.Errorf("%w: decode profile", ErrProviderUnavailable)
	}
	return nil
}

func (v HTTPProviderVerifier) verifyGoogle(ctx context.Context, accessToken string) (ExternalProfile, error) {
	var body struct {
		Sub           string `json:"sub"`
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Name          string `json:"name"`
		Picture       string `json:"picture"`
	}
	if err := v.getJSON(ctx, v.GoogleUserInfoURL, accessToken, &body); err != nil {
		return ExternalProfile{}, err
	}
	if body.Sub == "" {
		return ExternalProfile{}, fmt.Errorf("%w: google subject is required", ErrInvalidCredentials)
	}
	return ExternalProfile{
		Provider:      ProviderGoogle,
		Subject:       body.Sub,
		Email:         body.Email,
		EmailVerified: body.EmailVerified,
		DisplayName:   body.Name,
		AvatarURL:     body.Picture,
	}, nil
}

func (v HTTPProviderVerifier) verifyKakao(ctx context.Context, accessToken string) (ExternalProfile, error) {
	var body struct {
		ID           int64 `json:"id"`
		KakaoAccount struct {
			Email           string `json:"email"`
			IsEmailVerified bool   `json:"is_email_verified"`
			Profile         struct {
				Nickname        string `json:"nickname"`
				ProfileImageURL string `json:"profile_image_url"`
			} `json:"profile"`
		} `json:"kakao_account"`
	}
	if err := v.getJSON(ctx, v.KakaoUserInfoURL, accessToken, &body); err != nil {
		return ExternalProfile{}, err
	}
	if body.ID == 0 {
		return ExternalProfile{}, fmt.Errorf("%w: kakao subject is required", ErrInvalidCredentials)
	}
	return ExternalProfile{
		Provider:      ProviderKakao,
		Subject:       strconv.FormatInt(body.ID, 10),
		Email:         body.KakaoAccount.Email,
		EmailVerified: body.KakaoAccount.IsEmailVerified,
		DisplayName:   body.KakaoAccount.Profile.Nickname,
		AvatarURL:     body.KakaoAccount.Profile.ProfileImageURL,
	}, nil
}
```

- [ ] **Step 4: Run OAuth tests to verify they pass**

Run: `go test ./internal/account -run 'TestHTTPProviderVerifier' -v`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/account/oauth.go internal/account/oauth_test.go
git commit -m "feat: verify social login profiles"
```

---

### Task 5: Repository Boundary

**Files:**
- Create: `internal/account/repository.go`
- Create: `internal/account/postgres_repository.go`

- [ ] **Step 1: Add repository interface**

Create `internal/account/repository.go`:

```go
package account

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

type Identity struct {
	ID              pgtype.UUID
	AccountID       pgtype.UUID
	Provider        Provider
	ProviderSubject string
	Email           string
	NormalizedEmail string
	EmailVerified   bool
	PasswordHash    string
	Account         Account
}

type CreateAccountParams struct {
	Email           string
	NormalizedEmail string
	DisplayName     string
	AvatarURL       string
}

type CreateIdentityParams struct {
	AccountID       pgtype.UUID
	Provider        Provider
	ProviderSubject string
	Email           string
	NormalizedEmail string
	EmailVerified   bool
	PasswordHash    string
}

type CreateSessionParams struct {
	AccountID pgtype.UUID
	TokenHash string
	UserAgent string
	ExpiresAt time.Time
}

type Repository interface {
	WithTx(ctx context.Context, fn func(Repository) error) error
	CreateAccount(ctx context.Context, params CreateAccountParams) (Account, error)
	FindActiveAccountByNormalizedEmail(ctx context.Context, normalizedEmail string) (Account, error)
	SoftDeleteAccount(ctx context.Context, accountID pgtype.UUID) error
	CreateIdentity(ctx context.Context, params CreateIdentityParams) (Identity, error)
	FindIdentityWithAccount(ctx context.Context, provider Provider, subject string) (Identity, error)
	FindEmailIdentityWithAccount(ctx context.Context, normalizedEmail string) (Identity, error)
	CreateSession(ctx context.Context, params CreateSessionParams) (Session, error)
	FindAccountBySessionTokenHash(ctx context.Context, tokenHash string) (Account, error)
	RevokeSession(ctx context.Context, tokenHash string) error
	RevokeAllAccountSessions(ctx context.Context, accountID pgtype.UUID) error
}
```

- [ ] **Step 2: Add PostgreSQL repository implementation**

Create `internal/account/postgres_repository.go` with mapping helpers from sqlc rows to account domain types. The implementation must:

```go
package account

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/wonjong/hunch-server/internal/db/dbgen"
)

type PostgresRepository struct {
	pool    *pgxpool.Pool
	queries *dbgen.Queries
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{
		pool:    pool,
		queries: dbgen.New(pool),
	}
}

func (r *PostgresRepository) WithTx(ctx context.Context, fn func(Repository) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	txRepo := &postgresTxRepository{tx: tx, queries: dbgen.New(tx)}
	if err := fn(txRepo); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	return tx.Commit(ctx)
}

type postgresTxRepository struct {
	tx      pgx.Tx
	queries *dbgen.Queries
}
```

Implement the repository methods on both `PostgresRepository` and `postgresTxRepository` by delegating to a shared unexported helper struct or by giving both structs a `queries` field and repeated method receivers. Convert:

- `pgx.ErrNoRows` to `ErrInvalidCredentials` for identity/session lookup methods used by signin/authentication.
- PostgreSQL unique violation code `23505` to `ErrEmailAlreadyExists` when creating email accounts or identities.
- `pgtype.Text` with `Valid == false` to an empty string in domain `Account`.
- `pgtype.Timestamptz` with `Valid == true` to `time.Time`.

Use this duplicate-key helper:

```go
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
```

- [ ] **Step 3: Run package tests**

Run: `go test ./internal/account -v`

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/account/repository.go internal/account/postgres_repository.go
git commit -m "feat: add account repository boundary"
```

---

### Task 6: Signup And Signin Service

**Files:**
- Create: `internal/account/service.go`
- Create: `internal/account/service_test.go`

- [ ] **Step 1: Write failing service tests**

Create `internal/account/service_test.go` with an in-memory fake repository. Include these behaviors:

```go
func TestServiceSignupEmailCreatesAccountIdentityAndSession(t *testing.T) {
	repo := newFakeRepository()
	service := NewService(Config{
		Repository: repo,
		Verifier:  fakeVerifier{},
		BcryptCost: 4,
		SessionTTL: time.Hour,
		Now: func() time.Time { return time.Unix(1000, 0).UTC() },
	})

	result, err := service.SignupEmail(context.Background(), SignupEmailInput{
		Email:       "USER@example.com",
		Password:    "password123",
		DisplayName: "User",
		UserAgent:   "test-agent",
	})
	if err != nil {
		t.Fatalf("SignupEmail() error = %v", err)
	}
	if result.Account.Email != "USER@example.com" {
		t.Fatalf("Email = %q", result.Account.Email)
	}
	if result.Session.RawToken == "" || result.Session.TokenHash == "" {
		t.Fatalf("session = %+v", result.Session)
	}
	if len(repo.identities) != 1 {
		t.Fatalf("identity count = %d, want 1", len(repo.identities))
	}
}

func TestServiceSigninEmailRejectsWrongPassword(t *testing.T) {
	repo := newFakeRepository()
	service := NewService(Config{
		Repository: repo,
		Verifier:  fakeVerifier{},
		BcryptCost: 4,
		SessionTTL: time.Hour,
		Now: func() time.Time { return time.Unix(1000, 0).UTC() },
	})
	_, err := service.SignupEmail(context.Background(), SignupEmailInput{
		Email: "user@example.com", Password: "password123",
	})
	if err != nil {
		t.Fatalf("SignupEmail() error = %v", err)
	}

	_, err = service.Signin(context.Background(), SigninInput{
		Method: ProviderEmail, Email: "user@example.com", Password: "wrong-password",
	})
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("Signin() error = %v, want ErrInvalidCredentials", err)
	}
}

func TestServiceSigninOAuthCreatesAccountWhenIdentityIsNew(t *testing.T) {
	repo := newFakeRepository()
	service := NewService(Config{
		Repository: repo,
		Verifier: fakeVerifier{profile: ExternalProfile{
			Provider: ProviderGoogle, Subject: "google-sub", Email: "google@example.com", EmailVerified: true,
		}},
		BcryptCost: 4,
		SessionTTL: time.Hour,
		Now: func() time.Time { return time.Unix(1000, 0).UTC() },
	})

	result, err := service.Signin(context.Background(), SigninInput{
		Method: ProviderGoogle, AccessToken: "provider-token",
	})
	if err != nil {
		t.Fatalf("Signin() error = %v", err)
	}
	if result.Account.Email != "google@example.com" {
		t.Fatalf("Email = %q", result.Account.Email)
	}
	if len(repo.identities) != 1 {
		t.Fatalf("identity count = %d, want 1", len(repo.identities))
	}
}
```

The fake repository should implement `Repository` using maps keyed by normalized email, provider/subject, and session hash. It should assign deterministic `pgtype.UUID` values by incrementing the last byte.

- [ ] **Step 2: Run service tests to verify they fail**

Run: `go test ./internal/account -run 'TestService' -v`

Expected: FAIL because `NewService`, `Config`, and service methods do not exist.

- [ ] **Step 3: Implement service config and constructor**

Create `internal/account/service.go`:

```go
package account

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

type Config struct {
	Repository Repository
	Verifier   ProviderVerifier
	BcryptCost int
	SessionTTL time.Duration
	Now        func() time.Time
}

type Service struct {
	repository Repository
	verifier   ProviderVerifier
	bcryptCost int
	sessionTTL time.Duration
	now        func() time.Time
}

func NewService(cfg Config) *Service {
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &Service{
		repository: cfg.Repository,
		verifier:   cfg.Verifier,
		bcryptCost: cfg.BcryptCost,
		sessionTTL: cfg.SessionTTL,
		now:        now,
	}
}
```

- [ ] **Step 4: Implement signup and signin**

Add these methods to `internal/account/service.go`:

```go
func (s *Service) SignupEmail(ctx context.Context, input SignupEmailInput) (AuthResult, error) {
	normalizedEmail, err := NormalizeEmail(input.Email)
	if err != nil {
		return AuthResult{}, err
	}
	passwordHash, err := HashPassword(input.Password, s.bcryptCost)
	if err != nil {
		return AuthResult{}, err
	}

	var result AuthResult
	err = s.repository.WithTx(ctx, func(repo Repository) error {
		account, err := repo.CreateAccount(ctx, CreateAccountParams{
			Email:           input.Email,
			NormalizedEmail: normalizedEmail,
			DisplayName:     input.DisplayName,
		})
		if err != nil {
			return err
		}
		_, err = repo.CreateIdentity(ctx, CreateIdentityParams{
			AccountID:       account.ID,
			Provider:        ProviderEmail,
			ProviderSubject: normalizedEmail,
			Email:           input.Email,
			NormalizedEmail: normalizedEmail,
			EmailVerified:   true,
			PasswordHash:    passwordHash,
		})
		if err != nil {
			return err
		}
		session, err := s.createSession(ctx, repo, account.ID, input.UserAgent)
		if err != nil {
			return err
		}
		result = AuthResult{Account: account, Session: session}
		return nil
	})
	if err != nil {
		return AuthResult{}, err
	}
	return result, nil
}

func (s *Service) Signin(ctx context.Context, input SigninInput) (AuthResult, error) {
	switch input.Method {
	case ProviderEmail:
		return s.signinEmail(ctx, input)
	case ProviderGoogle, ProviderKakao:
		return s.signinOAuth(ctx, input)
	default:
		return AuthResult{}, fmt.Errorf("%w: unsupported signin method", ErrInvalidInput)
	}
}

func (s *Service) signinEmail(ctx context.Context, input SigninInput) (AuthResult, error) {
	normalizedEmail, err := NormalizeEmail(input.Email)
	if err != nil {
		return AuthResult{}, err
	}
	identity, err := s.repository.FindEmailIdentityWithAccount(ctx, normalizedEmail)
	if err != nil {
		return AuthResult{}, err
	}
	if err := ComparePassword(identity.PasswordHash, input.Password); err != nil {
		return AuthResult{}, err
	}

	session, err := s.createSession(ctx, s.repository, identity.Account.ID, input.UserAgent)
	if err != nil {
		return AuthResult{}, err
	}
	return AuthResult{Account: identity.Account, Session: session}, nil
}

func (s *Service) signinOAuth(ctx context.Context, input SigninInput) (AuthResult, error) {
	if s.verifier == nil {
		return AuthResult{}, ErrProviderUnavailable
	}
	profile, err := s.verifier.Verify(ctx, input.Method, input.AccessToken)
	if err != nil {
		return AuthResult{}, err
	}

	var result AuthResult
	err = s.repository.WithTx(ctx, func(repo Repository) error {
		identity, err := repo.FindIdentityWithAccount(ctx, profile.Provider, profile.Subject)
		if err == nil {
			session, err := s.createSession(ctx, repo, identity.Account.ID, input.UserAgent)
			if err != nil {
				return err
			}
			result = AuthResult{Account: identity.Account, Session: session}
			return nil
		}
		if !errors.Is(err, ErrInvalidCredentials) {
			return err
		}

		account, err := s.accountForNewOAuthIdentity(ctx, repo, profile)
		if err != nil {
			return err
		}
		_, err = repo.CreateIdentity(ctx, CreateIdentityParams{
			AccountID:       account.ID,
			Provider:        profile.Provider,
			ProviderSubject: profile.Subject,
			Email:           profile.Email,
			NormalizedEmail: normalizedVerifiedEmail(profile),
			EmailVerified:   profile.EmailVerified,
		})
		if err != nil {
			return err
		}
		session, err := s.createSession(ctx, repo, account.ID, input.UserAgent)
		if err != nil {
			return err
		}
		result = AuthResult{Account: account, Session: session}
		return nil
	})
	if err != nil {
		return AuthResult{}, err
	}
	return result, nil
}
```

Add helper methods:

```go
func (s *Service) accountForNewOAuthIdentity(ctx context.Context, repo Repository, profile ExternalProfile) (Account, error) {
	normalizedEmail := normalizedVerifiedEmail(profile)
	if normalizedEmail != "" {
		account, err := repo.FindActiveAccountByNormalizedEmail(ctx, normalizedEmail)
		if err == nil {
			return account, nil
		}
		if !errors.Is(err, ErrInvalidCredentials) {
			return Account{}, err
		}
	}
	return repo.CreateAccount(ctx, CreateAccountParams{
		Email:           profile.Email,
		NormalizedEmail: normalizedEmail,
		DisplayName:     profile.DisplayName,
		AvatarURL:       profile.AvatarURL,
	})
}

func normalizedVerifiedEmail(profile ExternalProfile) string {
	if !profile.EmailVerified || profile.Email == "" {
		return ""
	}
	normalized, err := NormalizeEmail(profile.Email)
	if err != nil {
		return ""
	}
	return normalized
}

func (s *Service) createSession(ctx context.Context, repo Repository, accountID pgtype.UUID, userAgent string) (Session, error) {
	rawToken, err := NewSessionToken()
	if err != nil {
		return Session{}, err
	}
	tokenHash := HashSessionToken(rawToken)
	expiresAt := s.now().Add(s.sessionTTL)
	session, err := repo.CreateSession(ctx, CreateSessionParams{
		AccountID: accountID,
		TokenHash: tokenHash,
		UserAgent: userAgent,
		ExpiresAt: expiresAt,
	})
	if err != nil {
		return Session{}, err
	}
	session.RawToken = rawToken
	session.TokenHash = tokenHash
	session.ExpiresAt = expiresAt
	return session, nil
}
```

- [ ] **Step 5: Run service tests to verify they pass**

Run: `go test ./internal/account -run 'TestService' -v`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/account/service.go internal/account/service_test.go
git commit -m "feat: add account signup and signin service"
```

---

### Task 7: Authentication, Signout, And Delete Service

**Files:**
- Modify: `internal/account/service.go`
- Modify: `internal/account/service_test.go`

- [ ] **Step 1: Write failing service tests**

Append these tests to `internal/account/service_test.go`:

```go
func TestServiceAuthenticateReturnsAccountForValidSession(t *testing.T) {
	repo := newFakeRepository()
	service := NewService(Config{
		Repository: repo,
		Verifier:  fakeVerifier{},
		BcryptCost: 4,
		SessionTTL: time.Hour,
		Now: func() time.Time { return time.Unix(1000, 0).UTC() },
	})
	result, err := service.SignupEmail(context.Background(), SignupEmailInput{
		Email: "user@example.com", Password: "password123",
	})
	if err != nil {
		t.Fatalf("SignupEmail() error = %v", err)
	}

	account, err := service.Authenticate(context.Background(), result.Session.RawToken)
	if err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}
	if account.Email != "user@example.com" {
		t.Fatalf("Email = %q", account.Email)
	}
}

func TestServiceSignoutRevokesSession(t *testing.T) {
	repo := newFakeRepository()
	service := NewService(Config{
		Repository: repo,
		Verifier:  fakeVerifier{},
		BcryptCost: 4,
		SessionTTL: time.Hour,
		Now: func() time.Time { return time.Unix(1000, 0).UTC() },
	})
	result, err := service.SignupEmail(context.Background(), SignupEmailInput{
		Email: "user@example.com", Password: "password123",
	})
	if err != nil {
		t.Fatalf("SignupEmail() error = %v", err)
	}

	if err := service.Signout(context.Background(), result.Session.RawToken); err != nil {
		t.Fatalf("Signout() error = %v", err)
	}
	_, err = service.Authenticate(context.Background(), result.Session.RawToken)
	if !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("Authenticate() error = %v, want ErrUnauthenticated", err)
	}
}

func TestServiceDeleteAccountSoftDeletesAndRevokesSessions(t *testing.T) {
	repo := newFakeRepository()
	service := NewService(Config{
		Repository: repo,
		Verifier:  fakeVerifier{},
		BcryptCost: 4,
		SessionTTL: time.Hour,
		Now: func() time.Time { return time.Unix(1000, 0).UTC() },
	})
	result, err := service.SignupEmail(context.Background(), SignupEmailInput{
		Email: "user@example.com", Password: "password123",
	})
	if err != nil {
		t.Fatalf("SignupEmail() error = %v", err)
	}

	if err := service.DeleteAccount(context.Background(), result.Account.ID); err != nil {
		t.Fatalf("DeleteAccount() error = %v", err)
	}
	_, err = service.Authenticate(context.Background(), result.Session.RawToken)
	if !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("Authenticate() error = %v, want ErrUnauthenticated", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/account -run 'TestServiceAuthenticate|TestServiceSignout|TestServiceDeleteAccount' -v`

Expected: FAIL because `Authenticate`, `Signout`, and `DeleteAccount` do not exist.

- [ ] **Step 3: Implement authenticated service methods**

Add these methods to `internal/account/service.go`:

```go
func (s *Service) Authenticate(ctx context.Context, rawToken string) (Account, error) {
	if rawToken == "" {
		return Account{}, ErrUnauthenticated
	}
	account, err := s.repository.FindAccountBySessionTokenHash(ctx, HashSessionToken(rawToken))
	if err != nil {
		return Account{}, ErrUnauthenticated
	}
	return account, nil
}

func (s *Service) Signout(ctx context.Context, rawToken string) error {
	if rawToken == "" {
		return ErrUnauthenticated
	}
	return s.repository.RevokeSession(ctx, HashSessionToken(rawToken))
}

func (s *Service) DeleteAccount(ctx context.Context, accountID pgtype.UUID) error {
	return s.repository.WithTx(ctx, func(repo Repository) error {
		if err := repo.SoftDeleteAccount(ctx, accountID); err != nil {
			return err
		}
		return repo.RevokeAllAccountSessions(ctx, accountID)
	})
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/account -v`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/account/service.go internal/account/service_test.go
git commit -m "feat: add account session lifecycle"
```

---

### Task 8: Account HTTP Handlers

**Files:**
- Create: `internal/account/handlers.go`
- Create: `internal/account/handlers_test.go`

- [ ] **Step 1: Write failing handler tests**

Create `internal/account/handlers_test.go` using `httptest`. Cover:

```go
func TestHandlersSignupSetsSessionCookie(t *testing.T) {
	service := newTestService(t)
	handlers := NewHandlers(service, HandlerConfig{
		CookieName: "hunch_session",
		CookieTTL:  time.Hour,
		Secure:     false,
	})

	req := httptest.NewRequest(http.MethodPost, "/signup", strings.NewReader(`{
		"email":"user@example.com",
		"password":"password123",
		"display_name":"User"
	}`))
	rec := httptest.NewRecorder()

	handlers.Signup(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if got := rec.Result().Cookies(); len(got) != 1 || got[0].Name != "hunch_session" {
		t.Fatalf("cookies = %+v", got)
	}
}

func TestHandlersSigninRejectsBadCredentials(t *testing.T) {
	service := newTestService(t)
	handlers := NewHandlers(service, HandlerConfig{CookieName: "hunch_session", CookieTTL: time.Hour})

	req := httptest.NewRequest(http.MethodPost, "/signin", strings.NewReader(`{
		"method":"email",
		"email":"missing@example.com",
		"password":"password123"
	}`))
	rec := httptest.NewRecorder()

	handlers.Signin(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandlersSignoutClearsCookie(t *testing.T) {
	service := newTestService(t)
	handlers := NewHandlers(service, HandlerConfig{CookieName: "hunch_session", CookieTTL: time.Hour})

	signupReq := httptest.NewRequest(http.MethodPost, "/signup", strings.NewReader(`{
		"email":"user@example.com",
		"password":"password123"
	}`))
	signupRec := httptest.NewRecorder()
	handlers.Signup(signupRec, signupReq)
	cookie := signupRec.Result().Cookies()[0]

	req := httptest.NewRequest(http.MethodPost, "/signout", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	handlers.Signout(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if clear := rec.Result().Cookies()[0]; clear.MaxAge != -1 {
		t.Fatalf("clear cookie MaxAge = %d, want -1", clear.MaxAge)
	}
}
```

- [ ] **Step 2: Run handler tests to verify they fail**

Run: `go test ./internal/account -run 'TestHandlers' -v`

Expected: FAIL because handlers do not exist.

- [ ] **Step 3: Implement handlers**

Create `internal/account/handlers.go` with:

```go
package account

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"
)

type HandlerConfig struct {
	CookieName string
	CookieTTL  time.Duration
	Secure     bool
}

type Handlers struct {
	service *Service
	config  HandlerConfig
}

func NewHandlers(service *Service, config HandlerConfig) *Handlers {
	return &Handlers{service: service, config: config}
}
```

Add request structs:

```go
type signupRequest struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
}

type signinRequest struct {
	Method      Provider `json:"method"`
	Email       string   `json:"email"`
	Password    string   `json:"password"`
	AccessToken string   `json:"access_token"`
}

type accountResponse struct {
	Account Account `json:"account"`
}
```

Add route methods:

```go
func (h *Handlers) Signup(w http.ResponseWriter, r *http.Request) {
	var body signupRequest
	if !decodeJSON(w, r, &body) {
		return
	}
	result, err := h.service.SignupEmail(r.Context(), SignupEmailInput{
		Email:       body.Email,
		Password:    body.Password,
		DisplayName: body.DisplayName,
		UserAgent:   r.UserAgent(),
	})
	if err != nil {
		writeError(w, err)
		return
	}
	h.setSessionCookie(w, result.Session.RawToken)
	writeJSON(w, http.StatusCreated, accountResponse{Account: result.Account})
}

func (h *Handlers) Signin(w http.ResponseWriter, r *http.Request) {
	var body signinRequest
	if !decodeJSON(w, r, &body) {
		return
	}
	result, err := h.service.Signin(r.Context(), SigninInput{
		Method:      body.Method,
		Email:       body.Email,
		Password:    body.Password,
		AccessToken: body.AccessToken,
		UserAgent:   r.UserAgent(),
	})
	if err != nil {
		writeError(w, err)
		return
	}
	h.setSessionCookie(w, result.Session.RawToken)
	writeJSON(w, http.StatusOK, accountResponse{Account: result.Account})
}

func (h *Handlers) Signout(w http.ResponseWriter, r *http.Request) {
	rawToken, ok := h.sessionTokenFromRequest(r)
	if !ok {
		writeError(w, ErrUnauthenticated)
		return
	}
	if err := h.service.Signout(r.Context(), rawToken); err != nil {
		writeError(w, err)
		return
	}
	h.clearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) Delete(w http.ResponseWriter, r *http.Request) {
	rawToken, ok := h.sessionTokenFromRequest(r)
	if !ok {
		writeError(w, ErrUnauthenticated)
		return
	}
	account, err := h.service.Authenticate(r.Context(), rawToken)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := h.service.DeleteAccount(r.Context(), account.ID); err != nil {
		writeError(w, err)
		return
	}
	h.clearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}
```

Add helper functions:

```go
func (h *Handlers) sessionTokenFromRequest(r *http.Request) (string, bool) {
	cookie, err := r.Cookie(h.config.CookieName)
	if err != nil || cookie.Value == "" {
		return "", false
	}
	return cookie.Value, true
}

func (h *Handlers) setSessionCookie(w http.ResponseWriter, rawToken string) {
	http.SetCookie(w, &http.Cookie{
		Name:     h.config.CookieName,
		Value:    rawToken,
		Path:     "/",
		MaxAge:   int(h.config.CookieTTL.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   h.config.Secure,
	})
}

func (h *Handlers) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     h.config.CookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   h.config.Secure,
	})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(target); err != nil {
		writeError(w, ErrInvalidInput)
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, ErrInvalidInput):
		status = http.StatusBadRequest
	case errors.Is(err, ErrEmailAlreadyExists):
		status = http.StatusConflict
	case errors.Is(err, ErrInvalidCredentials), errors.Is(err, ErrUnauthenticated):
		status = http.StatusUnauthorized
	case errors.Is(err, ErrProviderUnavailable):
		status = http.StatusBadGateway
	}
	http.Error(w, http.StatusText(status), status)
}
```

- [ ] **Step 4: Run handler tests to verify they pass**

Run: `go test ./internal/account -run 'TestHandlers' -v`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/account/handlers.go internal/account/handlers_test.go
git commit -m "feat: expose account auth handlers"
```

---

### Task 9: Router And Runtime Wiring

**Files:**
- Modify: `internal/httpserver/router.go`
- Modify: `internal/httpserver/router_test.go`
- Modify: `cmd/api/main.go`

- [ ] **Step 1: Write failing router smoke test**

Append to `internal/httpserver/router_test.go`:

```go
func TestRouterMountsAccountRoutesWhenHandlersProvided(t *testing.T) {
	called := false
	router := NewRouter(Dependencies{
		AccountSignup: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			called = true
			w.WriteHeader(http.StatusCreated)
		}),
	})

	req := httptest.NewRequest(http.MethodPost, "/signup", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}
	if !called {
		t.Fatal("account signup handler was not called")
	}
}
```

- [ ] **Step 2: Run router test to verify it fails**

Run: `go test ./internal/httpserver -run TestRouterMountsAccountRoutesWhenHandlersProvided -v`

Expected: FAIL because account route dependencies do not exist.

- [ ] **Step 3: Add route dependencies**

Modify `internal/httpserver/router.go`:

```go
type Dependencies struct {
	ReadinessCheck ReadinessCheck

	AccountSignup  http.Handler
	AccountSignin  http.Handler
	AccountSignout http.Handler
	AccountDelete  http.Handler
}
```

Inside `NewRouter`, before `/metrics`:

```go
	if deps.AccountSignup != nil {
		router.Method(http.MethodPost, "/signup", deps.AccountSignup)
	}
	if deps.AccountSignin != nil {
		router.Method(http.MethodPost, "/signin", deps.AccountSignin)
	}
	if deps.AccountSignout != nil {
		router.Method(http.MethodPost, "/signout", deps.AccountSignout)
	}
	if deps.AccountDelete != nil {
		router.Method(http.MethodDelete, "/delete", deps.AccountDelete)
	}
```

- [ ] **Step 4: Wire runtime dependencies**

Modify `cmd/api/main.go` after worker setup:

```go
	accountRepository := account.NewPostgresRepository(pool)
	accountVerifier := account.HTTPProviderVerifier{
		Client:            http.DefaultClient,
		GoogleUserInfoURL: cfg.GoogleUserInfoURL,
		KakaoUserInfoURL:  cfg.KakaoUserInfoURL,
	}
	accountService := account.NewService(account.Config{
		Repository: accountRepository,
		Verifier:   accountVerifier,
		BcryptCost: cfg.BcryptCost,
		SessionTTL: time.Duration(cfg.SessionTTLHours) * time.Hour,
	})
	accountHandlers := account.NewHandlers(accountService, account.HandlerConfig{
		CookieName: cfg.SessionCookieName,
		CookieTTL:  time.Duration(cfg.SessionTTLHours) * time.Hour,
		Secure:     cfg.AppEnv != "local",
	})
```

Add the import:

```go
	"github.com/wonjong/hunch-server/internal/account"
```

Pass handlers to `httpserver.NewRouter`:

```go
	router := httpserver.NewRouter(httpserver.Dependencies{
		ReadinessCheck: db.PingCheck(pool),
		AccountSignup:  http.HandlerFunc(accountHandlers.Signup),
		AccountSignin:  http.HandlerFunc(accountHandlers.Signin),
		AccountSignout: http.HandlerFunc(accountHandlers.Signout),
		AccountDelete:  http.HandlerFunc(accountHandlers.Delete),
	})
```

- [ ] **Step 5: Run router and package tests**

Run: `go test ./internal/httpserver ./cmd/api ./internal/account -v`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/httpserver/router.go internal/httpserver/router_test.go cmd/api/main.go
git commit -m "feat: wire account auth routes"
```

---

### Task 10: Documentation And Verification

**Files:**
- Modify: `README.md`
- Modify: `go.mod`
- Modify: `go.sum`

- [ ] **Step 1: Update README auth section**

Append to `README.md`:

````markdown
## Account Auth

The account API supports email/password signup, email/password signin, Google signin, Kakao signin, signout, and account deletion.

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
````

- [ ] **Step 2: Run formatting and dependency tidy**

Run:

```bash
make fmt
make tidy
```

Expected: PASS. `go.mod` keeps `golang.org/x/crypto` available for bcrypt.

- [ ] **Step 3: Run unit tests**

Run: `make test`

Expected: PASS.

- [ ] **Step 4: Run integration migration test**

Run: `RUN_INTEGRATION_TESTS=1 go test ./internal/db -run TestMigrationsApply -v`

Expected: PASS when Docker is available. If Docker is unavailable, record the exact failure in the task summary before merging.

- [ ] **Step 5: Commit**

```bash
git add README.md go.mod go.sum
git commit -m "docs: document account auth endpoints"
```

---

## Self-Review Checklist

- Spec coverage: `/signup`, `/signin`, `/signout`, and `/delete` are covered by Tasks 6, 7, 8, and 9.
- Login methods: email/password, Google, and Kakao are covered by Tasks 3, 4, 6, and 8.
- Account entity: accounts, identities, and sessions are covered by Tasks 2 and 5.
- Security baseline: bcrypt password hashing, opaque session token hashing, HttpOnly cookies, session revocation, and soft delete are covered by Tasks 3, 6, 7, and 8.
- Verification: unit tests, router tests, sqlc generation, formatting, dependency tidy, and migration integration tests are covered by Tasks 1 through 10.
