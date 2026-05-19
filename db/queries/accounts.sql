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

-- name: SoftDeleteAccountIdentities :exec
UPDATE account_identities
SET deleted_at = now(),
    updated_at = now()
WHERE account_id = $1
  AND deleted_at IS NULL;

-- name: CreateAccountIdentity :one
WITH locked_account AS (
    SELECT accounts.id
    FROM accounts
    WHERE accounts.id = sqlc.arg(account_id)
      AND accounts.deleted_at IS NULL
      AND accounts.status = 'active'
    FOR UPDATE
)
INSERT INTO account_identities AS ai (
    account_id,
    provider,
    provider_subject,
    email,
    normalized_email,
    email_verified,
    password_hash
)
SELECT
    locked_account.id,
    sqlc.arg(provider),
    sqlc.arg(provider_subject),
    sqlc.arg(email),
    sqlc.arg(normalized_email),
    sqlc.arg(email_verified),
    sqlc.arg(password_hash)
FROM locked_account
RETURNING ai.id, ai.account_id, ai.provider, ai.provider_subject, ai.email, ai.normalized_email, ai.email_verified, ai.password_hash, ai.created_at, ai.updated_at, ai.deleted_at;

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
  AND ai.deleted_at IS NULL
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
  AND ai.deleted_at IS NULL
  AND a.deleted_at IS NULL
  AND a.status = 'active';

-- name: CreateAccountSession :one
WITH locked_account AS (
    SELECT accounts.id
    FROM accounts
    WHERE accounts.id = sqlc.arg(account_id)
      AND accounts.deleted_at IS NULL
      AND accounts.status = 'active'
    FOR UPDATE
)
INSERT INTO account_sessions AS s (account_id, token_hash, user_agent, expires_at)
SELECT
    locked_account.id,
    sqlc.arg(token_hash),
    sqlc.arg(user_agent),
    sqlc.arg(expires_at)
FROM locked_account
RETURNING s.id, s.account_id, s.token_hash, s.user_agent, s.expires_at, s.revoked_at, s.created_at;

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
