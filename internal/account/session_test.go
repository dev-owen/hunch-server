package account

import "testing"

func TestNewSessionTokenAndHash(t *testing.T) {
	token, err := NewSessionToken()
	if err != nil {
		t.Fatalf("NewSessionToken() error = %v", err)
	}
	if len(token) < 40 {
		t.Fatalf("token length = %d, want at least 40", len(token))
	}

	hash := HashSessionToken(token)
	if hash == "" {
		t.Fatal("HashSessionToken() returned empty hash")
	}
	if hash == token {
		t.Fatal("HashSessionToken() returned raw token")
	}
	if HashSessionToken(token) != hash {
		t.Fatal("HashSessionToken() is not deterministic")
	}
}
