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
	SoftDeleteAccountIdentities(ctx context.Context, accountID pgtype.UUID) error
	CreateIdentity(ctx context.Context, params CreateIdentityParams) (Identity, error)
	FindIdentityWithAccount(ctx context.Context, provider Provider, subject string) (Identity, error)
	FindEmailIdentityWithAccount(ctx context.Context, normalizedEmail string) (Identity, error)
	CreateSession(ctx context.Context, params CreateSessionParams) (Session, error)
	FindAccountBySessionTokenHash(ctx context.Context, tokenHash string) (Account, error)
	RevokeSession(ctx context.Context, tokenHash string) error
	RevokeAllAccountSessions(ctx context.Context, accountID pgtype.UUID) error
}
