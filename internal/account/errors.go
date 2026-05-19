package account

import "errors"

var (
	ErrInvalidInput          = errors.New("invalid input")
	ErrEmailAlreadyExists    = errors.New("email already exists")
	ErrInvalidCredentials    = errors.New("invalid credentials")
	ErrIdentityAlreadyExists = errors.New("identity already exists")
	ErrUnauthenticated       = errors.New("unauthenticated")
	ErrAccountDeleted        = errors.New("account deleted")
	ErrProviderUnavailable   = errors.New("provider unavailable")
)
