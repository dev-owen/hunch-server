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
