package account

import (
	"context"
	"errors"
	"time"

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

func (r *PostgresRepository) CreateAccount(ctx context.Context, params CreateAccountParams) (Account, error) {
	return createAccount(ctx, r.queries, params)
}

func (r *PostgresRepository) FindActiveAccountByNormalizedEmail(ctx context.Context, normalizedEmail string) (Account, error) {
	return findActiveAccountByNormalizedEmail(ctx, r.queries, normalizedEmail)
}

func (r *PostgresRepository) SoftDeleteAccount(ctx context.Context, accountID pgtype.UUID) error {
	return r.queries.SoftDeleteAccount(ctx, accountID)
}

func (r *PostgresRepository) SoftDeleteAccountIdentities(ctx context.Context, accountID pgtype.UUID) error {
	return r.queries.SoftDeleteAccountIdentities(ctx, accountID)
}

func (r *PostgresRepository) CreateIdentity(ctx context.Context, params CreateIdentityParams) (Identity, error) {
	return createIdentity(ctx, r.queries, params)
}

func (r *PostgresRepository) FindIdentityWithAccount(ctx context.Context, provider Provider, subject string) (Identity, error) {
	return findIdentityWithAccount(ctx, r.queries, provider, subject)
}

func (r *PostgresRepository) FindEmailIdentityWithAccount(ctx context.Context, normalizedEmail string) (Identity, error) {
	return findEmailIdentityWithAccount(ctx, r.queries, normalizedEmail)
}

func (r *PostgresRepository) CreateSession(ctx context.Context, params CreateSessionParams) (Session, error) {
	return createSession(ctx, r.queries, params)
}

func (r *PostgresRepository) FindAccountBySessionTokenHash(ctx context.Context, tokenHash string) (Account, error) {
	return findAccountBySessionTokenHash(ctx, r.queries, tokenHash)
}

func (r *PostgresRepository) RevokeSession(ctx context.Context, tokenHash string) error {
	return r.queries.RevokeAccountSession(ctx, tokenHash)
}

func (r *PostgresRepository) RevokeAllAccountSessions(ctx context.Context, accountID pgtype.UUID) error {
	return r.queries.RevokeAllAccountSessions(ctx, accountID)
}

type postgresTxRepository struct {
	tx      pgx.Tx
	queries *dbgen.Queries
}

func (r *postgresTxRepository) WithTx(ctx context.Context, fn func(Repository) error) error {
	return fn(r)
}

func (r *postgresTxRepository) CreateAccount(ctx context.Context, params CreateAccountParams) (Account, error) {
	return createAccount(ctx, r.queries, params)
}

func (r *postgresTxRepository) FindActiveAccountByNormalizedEmail(ctx context.Context, normalizedEmail string) (Account, error) {
	return findActiveAccountByNormalizedEmail(ctx, r.queries, normalizedEmail)
}

func (r *postgresTxRepository) SoftDeleteAccount(ctx context.Context, accountID pgtype.UUID) error {
	return r.queries.SoftDeleteAccount(ctx, accountID)
}

func (r *postgresTxRepository) SoftDeleteAccountIdentities(ctx context.Context, accountID pgtype.UUID) error {
	return r.queries.SoftDeleteAccountIdentities(ctx, accountID)
}

func (r *postgresTxRepository) CreateIdentity(ctx context.Context, params CreateIdentityParams) (Identity, error) {
	return createIdentity(ctx, r.queries, params)
}

func (r *postgresTxRepository) FindIdentityWithAccount(ctx context.Context, provider Provider, subject string) (Identity, error) {
	return findIdentityWithAccount(ctx, r.queries, provider, subject)
}

func (r *postgresTxRepository) FindEmailIdentityWithAccount(ctx context.Context, normalizedEmail string) (Identity, error) {
	return findEmailIdentityWithAccount(ctx, r.queries, normalizedEmail)
}

func (r *postgresTxRepository) CreateSession(ctx context.Context, params CreateSessionParams) (Session, error) {
	return createSession(ctx, r.queries, params)
}

func (r *postgresTxRepository) FindAccountBySessionTokenHash(ctx context.Context, tokenHash string) (Account, error) {
	return findAccountBySessionTokenHash(ctx, r.queries, tokenHash)
}

func (r *postgresTxRepository) RevokeSession(ctx context.Context, tokenHash string) error {
	return r.queries.RevokeAccountSession(ctx, tokenHash)
}

func (r *postgresTxRepository) RevokeAllAccountSessions(ctx context.Context, accountID pgtype.UUID) error {
	return r.queries.RevokeAllAccountSessions(ctx, accountID)
}

func createAccount(ctx context.Context, queries *dbgen.Queries, params CreateAccountParams) (Account, error) {
	account, err := queries.CreateAccount(ctx, dbgen.CreateAccountParams{
		Email:           textParam(params.Email),
		NormalizedEmail: textParam(params.NormalizedEmail),
		DisplayName:     params.DisplayName,
		AvatarUrl:       textParam(params.AvatarURL),
	})
	if isUniqueViolation(err) {
		return Account{}, ErrEmailAlreadyExists
	}
	if err != nil {
		return Account{}, err
	}
	return mapAccount(account), nil
}

func findActiveAccountByNormalizedEmail(ctx context.Context, queries *dbgen.Queries, normalizedEmail string) (Account, error) {
	account, err := queries.FindActiveAccountByNormalizedEmail(ctx, textParam(normalizedEmail))
	if errors.Is(err, pgx.ErrNoRows) {
		return Account{}, ErrInvalidCredentials
	}
	if err != nil {
		return Account{}, err
	}
	return mapAccount(account), nil
}

func createIdentity(ctx context.Context, queries *dbgen.Queries, params CreateIdentityParams) (Identity, error) {
	identity, err := queries.CreateAccountIdentity(ctx, dbgen.CreateAccountIdentityParams{
		AccountID:       params.AccountID,
		Provider:        string(params.Provider),
		ProviderSubject: params.ProviderSubject,
		Email:           textParam(params.Email),
		NormalizedEmail: textParam(params.NormalizedEmail),
		EmailVerified:   params.EmailVerified,
		PasswordHash:    textParam(params.PasswordHash),
	})
	if isUniqueViolation(err) {
		return Identity{}, ErrEmailAlreadyExists
	}
	if err != nil {
		return Identity{}, err
	}
	return mapIdentity(identity), nil
}

func findIdentityWithAccount(ctx context.Context, queries *dbgen.Queries, provider Provider, subject string) (Identity, error) {
	row, err := queries.FindIdentityWithAccount(ctx, dbgen.FindIdentityWithAccountParams{
		Provider:        string(provider),
		ProviderSubject: subject,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Identity{}, ErrInvalidCredentials
	}
	if err != nil {
		return Identity{}, err
	}
	return mapIdentityWithAccount(row), nil
}

func findEmailIdentityWithAccount(ctx context.Context, queries *dbgen.Queries, normalizedEmail string) (Identity, error) {
	row, err := queries.FindEmailIdentityWithAccount(ctx, textParam(normalizedEmail))
	if errors.Is(err, pgx.ErrNoRows) {
		return Identity{}, ErrInvalidCredentials
	}
	if err != nil {
		return Identity{}, err
	}
	return mapEmailIdentityWithAccount(row), nil
}

func createSession(ctx context.Context, queries *dbgen.Queries, params CreateSessionParams) (Session, error) {
	session, err := queries.CreateAccountSession(ctx, dbgen.CreateAccountSessionParams{
		AccountID: params.AccountID,
		TokenHash: params.TokenHash,
		UserAgent: params.UserAgent,
		ExpiresAt: timestamptzParam(params.ExpiresAt),
	})
	if err != nil {
		return Session{}, err
	}
	return Session{
		TokenHash: session.TokenHash,
		ExpiresAt: timeFromTimestamptz(session.ExpiresAt),
	}, nil
}

func findAccountBySessionTokenHash(ctx context.Context, queries *dbgen.Queries, tokenHash string) (Account, error) {
	account, err := queries.FindAccountBySessionTokenHash(ctx, tokenHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return Account{}, ErrInvalidCredentials
	}
	if err != nil {
		return Account{}, err
	}
	return mapAccount(account), nil
}

func mapIdentity(identity dbgen.AccountIdentity) Identity {
	return Identity{
		ID:              identity.ID,
		AccountID:       identity.AccountID,
		Provider:        Provider(identity.Provider),
		ProviderSubject: identity.ProviderSubject,
		Email:           stringFromText(identity.Email),
		NormalizedEmail: stringFromText(identity.NormalizedEmail),
		EmailVerified:   identity.EmailVerified,
		PasswordHash:    stringFromText(identity.PasswordHash),
	}
}

func mapIdentityWithAccount(row dbgen.FindIdentityWithAccountRow) Identity {
	return Identity{
		ID:              row.ID,
		AccountID:       row.AccountID,
		Provider:        Provider(row.Provider),
		ProviderSubject: row.ProviderSubject,
		Email:           stringFromText(row.Email),
		NormalizedEmail: stringFromText(row.NormalizedEmail),
		EmailVerified:   row.EmailVerified,
		PasswordHash:    stringFromText(row.PasswordHash),
		Account: Account{
			ID:              row.AccountID2,
			Email:           stringFromText(row.AccountEmail),
			NormalizedEmail: stringFromText(row.AccountNormalizedEmail),
			DisplayName:     row.AccountDisplayName,
			AvatarURL:       stringFromText(row.AccountAvatarUrl),
			Status:          row.AccountStatus,
			CreatedAt:       timeFromTimestamptz(row.AccountCreatedAt),
			UpdatedAt:       timeFromTimestamptz(row.AccountUpdatedAt),
		},
	}
}

func mapEmailIdentityWithAccount(row dbgen.FindEmailIdentityWithAccountRow) Identity {
	return Identity{
		ID:              row.ID,
		AccountID:       row.AccountID,
		Provider:        Provider(row.Provider),
		ProviderSubject: row.ProviderSubject,
		Email:           stringFromText(row.Email),
		NormalizedEmail: stringFromText(row.NormalizedEmail),
		EmailVerified:   row.EmailVerified,
		PasswordHash:    stringFromText(row.PasswordHash),
		Account: Account{
			ID:              row.AccountID2,
			Email:           stringFromText(row.AccountEmail),
			NormalizedEmail: stringFromText(row.AccountNormalizedEmail),
			DisplayName:     row.AccountDisplayName,
			AvatarURL:       stringFromText(row.AccountAvatarUrl),
			Status:          row.AccountStatus,
			CreatedAt:       timeFromTimestamptz(row.AccountCreatedAt),
			UpdatedAt:       timeFromTimestamptz(row.AccountUpdatedAt),
		},
	}
}

func mapAccount(account dbgen.Account) Account {
	return Account{
		ID:              account.ID,
		Email:           stringFromText(account.Email),
		NormalizedEmail: stringFromText(account.NormalizedEmail),
		DisplayName:     account.DisplayName,
		AvatarURL:       stringFromText(account.AvatarUrl),
		Status:          account.Status,
		CreatedAt:       timeFromTimestamptz(account.CreatedAt),
		UpdatedAt:       timeFromTimestamptz(account.UpdatedAt),
	}
}

func textParam(value string) pgtype.Text {
	if value == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: value, Valid: true}
}

func stringFromText(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func timestamptzParam(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: !value.IsZero()}
}

func timeFromTimestamptz(value pgtype.Timestamptz) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return value.Time
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
