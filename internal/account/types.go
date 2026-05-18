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
