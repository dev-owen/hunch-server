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
