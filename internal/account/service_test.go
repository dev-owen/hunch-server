package account

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestServiceSignupEmailCreatesAccountIdentityAndSession(t *testing.T) {
	repo := newFakeRepository()
	service := NewService(Config{
		Repository: repo,
		Verifier:   fakeVerifier{},
		BcryptCost: 4,
		SessionTTL: time.Hour,
		Now:        func() time.Time { return time.Unix(1000, 0).UTC() },
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
		Verifier:   fakeVerifier{},
		BcryptCost: 4,
		SessionTTL: time.Hour,
		Now:        func() time.Time { return time.Unix(1000, 0).UTC() },
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
		Now:        func() time.Time { return time.Unix(1000, 0).UTC() },
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

func TestServiceSigninOAuthIgnoresUnverifiedEmailForStorage(t *testing.T) {
	repo := newFakeRepository()
	service := NewService(Config{
		Repository: repo,
		Verifier: fakeVerifier{profile: ExternalProfile{
			Provider: ProviderKakao, Subject: "kakao-sub", Email: "kakao@example.com", EmailVerified: false,
		}},
		BcryptCost: 4,
		SessionTTL: time.Hour,
		Now:        func() time.Time { return time.Unix(1000, 0).UTC() },
	})

	result, err := service.Signin(context.Background(), SigninInput{
		Method: ProviderKakao, AccessToken: "provider-token",
	})
	if err != nil {
		t.Fatalf("Signin() error = %v", err)
	}
	if result.Account.Email != "" || result.Account.NormalizedEmail != "" {
		t.Fatalf("account email = %q normalized=%q, want empty", result.Account.Email, result.Account.NormalizedEmail)
	}
	identity := repo.identities[identityKey(ProviderKakao, "kakao-sub")]
	if identity.Email != "" || identity.NormalizedEmail != "" {
		t.Fatalf("identity email = %q normalized=%q, want empty", identity.Email, identity.NormalizedEmail)
	}
}

func TestServiceSigninOAuthRejectsProviderMismatch(t *testing.T) {
	repo := newFakeRepository()
	service := NewService(Config{
		Repository: repo,
		Verifier: fakeVerifier{profile: ExternalProfile{
			Provider: ProviderKakao, Subject: "kakao-sub",
		}},
		BcryptCost: 4,
		SessionTTL: time.Hour,
		Now:        func() time.Time { return time.Unix(1000, 0).UTC() },
	})

	_, err := service.Signin(context.Background(), SigninInput{
		Method: ProviderGoogle, AccessToken: "provider-token",
	})
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("Signin() error = %v, want ErrInvalidCredentials", err)
	}
}

func TestServiceSigninOAuthRetriesIdentityConflict(t *testing.T) {
	repo := newFakeRepository()
	account, err := repo.CreateAccount(context.Background(), CreateAccountParams{
		Email: "google@example.com", NormalizedEmail: "google@example.com",
	})
	if err != nil {
		t.Fatalf("CreateAccount() error = %v", err)
	}
	_, err = repo.CreateIdentity(context.Background(), CreateIdentityParams{
		AccountID:       account.ID,
		Provider:        ProviderGoogle,
		ProviderSubject: "google-sub",
		Email:           "google@example.com",
		NormalizedEmail: "google@example.com",
		EmailVerified:   true,
	})
	if err != nil {
		t.Fatalf("CreateIdentity() error = %v", err)
	}
	repo.identityLookupMisses = 1
	service := NewService(Config{
		Repository: repo,
		Verifier: fakeVerifier{profile: ExternalProfile{
			Provider: ProviderGoogle, Subject: "google-sub", Email: "google@example.com", EmailVerified: true,
		}},
		BcryptCost: 4,
		SessionTTL: time.Hour,
		Now:        func() time.Time { return time.Unix(1000, 0).UTC() },
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
}

func TestServiceSigninOAuthRetriesAccountEmailConflict(t *testing.T) {
	repo := newFakeRepository()
	_, err := repo.CreateAccount(context.Background(), CreateAccountParams{
		Email: "google@example.com", NormalizedEmail: "google@example.com",
	})
	if err != nil {
		t.Fatalf("CreateAccount() error = %v", err)
	}
	repo.accountLookupMisses = 1
	service := NewService(Config{
		Repository: repo,
		Verifier: fakeVerifier{profile: ExternalProfile{
			Provider: ProviderGoogle, Subject: "google-sub", Email: "google@example.com", EmailVerified: true,
		}},
		BcryptCost: 4,
		SessionTTL: time.Hour,
		Now:        func() time.Time { return time.Unix(1000, 0).UTC() },
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
}

func TestServiceAuthenticateReturnsAccountForValidSession(t *testing.T) {
	repo := newFakeRepository()
	service := NewService(Config{
		Repository: repo,
		Verifier:   fakeVerifier{},
		BcryptCost: 4,
		SessionTTL: time.Hour,
		Now:        func() time.Time { return time.Unix(1000, 0).UTC() },
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
		Verifier:   fakeVerifier{},
		BcryptCost: 4,
		SessionTTL: time.Hour,
		Now:        func() time.Time { return time.Unix(1000, 0).UTC() },
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
		Verifier:   fakeVerifier{},
		BcryptCost: 4,
		SessionTTL: time.Hour,
		Now:        func() time.Time { return time.Unix(1000, 0).UTC() },
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
	if _, err := repo.FindActiveAccountByNormalizedEmail(context.Background(), "user@example.com"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("FindActiveAccountByNormalizedEmail() error = %v, want ErrInvalidCredentials", err)
	}
	_, err = service.Authenticate(context.Background(), result.Session.RawToken)
	if !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("Authenticate() error = %v, want ErrUnauthenticated", err)
	}
	if len(repo.emailIdentities) != 0 {
		t.Fatalf("email identity count = %d, want 0", len(repo.emailIdentities))
	}
}

type fakeVerifier struct {
	profile ExternalProfile
	err     error
}

func (v fakeVerifier) Verify(context.Context, Provider, string) (ExternalProfile, error) {
	if v.err != nil {
		return ExternalProfile{}, v.err
	}
	return v.profile, nil
}

type fakeRepository struct {
	nextID               byte
	accounts             map[string]Account
	accountsByID         map[string]Account
	identities           map[string]Identity
	emailIdentities      map[string]Identity
	sessions             map[string]Account
	revokedSessions      map[string]bool
	identityLookupMisses int
	accountLookupMisses  int
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{
		nextID:          1,
		accounts:        make(map[string]Account),
		accountsByID:    make(map[string]Account),
		identities:      make(map[string]Identity),
		emailIdentities: make(map[string]Identity),
		sessions:        make(map[string]Account),
		revokedSessions: make(map[string]bool),
	}
}

func (r *fakeRepository) WithTx(ctx context.Context, fn func(Repository) error) error {
	return fn(r)
}

func (r *fakeRepository) CreateAccount(_ context.Context, params CreateAccountParams) (Account, error) {
	if (params.Email == "") != (params.NormalizedEmail == "") {
		return Account{}, fmt.Errorf("email and normalized email must both be present or empty")
	}
	if params.NormalizedEmail != "" {
		if _, ok := r.accounts[params.NormalizedEmail]; ok {
			return Account{}, ErrEmailAlreadyExists
		}
	}
	account := Account{
		ID:              r.newUUID(),
		Email:           params.Email,
		NormalizedEmail: params.NormalizedEmail,
		DisplayName:     params.DisplayName,
		AvatarURL:       params.AvatarURL,
		Status:          "active",
		CreatedAt:       time.Unix(1000, 0).UTC(),
		UpdatedAt:       time.Unix(1000, 0).UTC(),
	}
	if params.NormalizedEmail != "" {
		r.accounts[params.NormalizedEmail] = account
	}
	r.accountsByID[uuidKey(account.ID)] = account
	return account, nil
}

func (r *fakeRepository) FindActiveAccountByNormalizedEmail(_ context.Context, normalizedEmail string) (Account, error) {
	if r.accountLookupMisses > 0 {
		r.accountLookupMisses--
		return Account{}, ErrInvalidCredentials
	}
	account, ok := r.accounts[normalizedEmail]
	if !ok {
		return Account{}, ErrInvalidCredentials
	}
	return account, nil
}

func (r *fakeRepository) SoftDeleteAccount(_ context.Context, accountID pgtype.UUID) error {
	account, ok := r.accountsByID[uuidKey(accountID)]
	if !ok {
		return nil
	}
	delete(r.accountsByID, uuidKey(accountID))
	if account.NormalizedEmail != "" {
		delete(r.accounts, account.NormalizedEmail)
	}
	return nil
}

func (r *fakeRepository) SoftDeleteAccountIdentities(_ context.Context, accountID pgtype.UUID) error {
	for key, identity := range r.identities {
		if identity.AccountID == accountID {
			delete(r.identities, key)
		}
	}
	for key, identity := range r.emailIdentities {
		if identity.AccountID == accountID {
			delete(r.emailIdentities, key)
		}
	}
	return nil
}

func (r *fakeRepository) CreateIdentity(_ context.Context, params CreateIdentityParams) (Identity, error) {
	if (params.Email == "") != (params.NormalizedEmail == "") {
		return Identity{}, fmt.Errorf("email and normalized email must both be present or empty")
	}
	if params.Provider == ProviderEmail && (params.Email == "" || params.NormalizedEmail == "" || params.PasswordHash == "") {
		return Identity{}, fmt.Errorf("email identity requires email and password hash")
	}
	key := identityKey(params.Provider, params.ProviderSubject)
	if _, ok := r.identities[key]; ok {
		return Identity{}, ErrIdentityAlreadyExists
	}
	account := r.accountsByID[uuidKey(params.AccountID)]
	identity := Identity{
		ID:              r.newUUID(),
		AccountID:       params.AccountID,
		Provider:        params.Provider,
		ProviderSubject: params.ProviderSubject,
		Email:           params.Email,
		NormalizedEmail: params.NormalizedEmail,
		EmailVerified:   params.EmailVerified,
		PasswordHash:    params.PasswordHash,
		Account:         account,
	}
	r.identities[key] = identity
	if params.Provider == ProviderEmail {
		r.emailIdentities[params.NormalizedEmail] = identity
	}
	return identity, nil
}

func (r *fakeRepository) FindIdentityWithAccount(_ context.Context, provider Provider, subject string) (Identity, error) {
	if r.identityLookupMisses > 0 {
		r.identityLookupMisses--
		return Identity{}, ErrInvalidCredentials
	}
	identity, ok := r.identities[identityKey(provider, subject)]
	if !ok {
		return Identity{}, ErrInvalidCredentials
	}
	identity.Account = r.accountsByID[uuidKey(identity.AccountID)]
	return identity, nil
}

func (r *fakeRepository) FindEmailIdentityWithAccount(_ context.Context, normalizedEmail string) (Identity, error) {
	identity, ok := r.emailIdentities[normalizedEmail]
	if !ok {
		return Identity{}, ErrInvalidCredentials
	}
	identity.Account = r.accountsByID[uuidKey(identity.AccountID)]
	return identity, nil
}

func (r *fakeRepository) CreateSession(_ context.Context, params CreateSessionParams) (Session, error) {
	account, ok := r.accountsByID[uuidKey(params.AccountID)]
	if !ok {
		return Session{}, fmt.Errorf("account not found")
	}
	r.sessions[params.TokenHash] = account
	return Session{TokenHash: params.TokenHash, ExpiresAt: params.ExpiresAt}, nil
}

func (r *fakeRepository) FindAccountBySessionTokenHash(_ context.Context, tokenHash string) (Account, error) {
	if r.revokedSessions[tokenHash] {
		return Account{}, ErrInvalidCredentials
	}
	account, ok := r.sessions[tokenHash]
	if !ok {
		return Account{}, ErrInvalidCredentials
	}
	return account, nil
}

func (r *fakeRepository) RevokeSession(_ context.Context, tokenHash string) error {
	r.revokedSessions[tokenHash] = true
	return nil
}

func (r *fakeRepository) RevokeAllAccountSessions(_ context.Context, accountID pgtype.UUID) error {
	for tokenHash, account := range r.sessions {
		if account.ID == accountID {
			r.revokedSessions[tokenHash] = true
		}
	}
	return nil
}

func (r *fakeRepository) newUUID() pgtype.UUID {
	var id [16]byte
	id[15] = r.nextID
	r.nextID++
	return pgtype.UUID{Bytes: id, Valid: true}
}

func identityKey(provider Provider, subject string) string {
	return string(provider) + ":" + subject
}

func uuidKey(id pgtype.UUID) string {
	return string(id.Bytes[:])
}
