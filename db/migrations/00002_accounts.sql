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
