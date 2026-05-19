package account

import (
	"context"
	"errors"
	"fmt"
	"strings"
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

func (s *Service) SignupEmail(ctx context.Context, input SignupEmailInput) (AuthResult, error) {
	normalizedEmail, err := NormalizeEmail(input.Email)
	if err != nil {
		return AuthResult{}, err
	}
	email := strings.TrimSpace(input.Email)
	passwordHash, err := HashPassword(input.Password, s.bcryptCost)
	if err != nil {
		return AuthResult{}, err
	}

	var result AuthResult
	err = s.repository.WithTx(ctx, func(repo Repository) error {
		account, err := repo.CreateAccount(ctx, CreateAccountParams{
			Email:           email,
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
			Email:           email,
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
	if profile.Provider != input.Method {
		return AuthResult{}, fmt.Errorf("%w: provider mismatch", ErrInvalidCredentials)
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
			Email:           verifiedEmail(profile),
			NormalizedEmail: verifiedNormalizedEmail(profile),
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

func (s *Service) accountForNewOAuthIdentity(ctx context.Context, repo Repository, profile ExternalProfile) (Account, error) {
	email := verifiedEmail(profile)
	normalizedEmail := verifiedNormalizedEmail(profile)
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
		Email:           email,
		NormalizedEmail: normalizedEmail,
		DisplayName:     profile.DisplayName,
		AvatarURL:       profile.AvatarURL,
	})
}

func verifiedEmail(profile ExternalProfile) string {
	if verifiedNormalizedEmail(profile) == "" {
		return ""
	}
	return strings.TrimSpace(profile.Email)
}

func verifiedNormalizedEmail(profile ExternalProfile) string {
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

func (s *Service) Authenticate(ctx context.Context, rawToken string) (Account, error) {
	if rawToken == "" {
		return Account{}, ErrUnauthenticated
	}
	account, err := s.repository.FindAccountBySessionTokenHash(ctx, HashSessionToken(rawToken))
	if err != nil {
		if errors.Is(err, ErrInvalidCredentials) {
			return Account{}, ErrUnauthenticated
		}
		return Account{}, err
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
		if err := repo.SoftDeleteAccountIdentities(ctx, accountID); err != nil {
			return err
		}
		return repo.RevokeAllAccountSessions(ctx, accountID)
	})
}
