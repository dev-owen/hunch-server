package config

import "testing"

func TestLoadAppliesDefaults(t *testing.T) {
	t.Setenv("APP_ENV", "")
	t.Setenv("HTTP_ADDR", "")
	t.Setenv("DATABASE_URL", "")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("OPENAI_BASE_URL", "")
	t.Setenv("S3_ENDPOINT", "")
	t.Setenv("S3_REGION", "")
	t.Setenv("S3_BUCKET", "")
	t.Setenv("S3_ACCESS_KEY_ID", "")
	t.Setenv("S3_SECRET_ACCESS_KEY", "")
	t.Setenv("S3_USE_PATH_STYLE", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.AppEnv != "local" {
		t.Fatalf("AppEnv = %q, want local", cfg.AppEnv)
	}
	if cfg.HTTPAddr != ":8080" {
		t.Fatalf("HTTPAddr = %q, want :8080", cfg.HTTPAddr)
	}
	if cfg.OpenAIBaseURL != "https://api.openai.com/v1" {
		t.Fatalf("OpenAIBaseURL = %q", cfg.OpenAIBaseURL)
	}
	if !cfg.S3UsePathStyle {
		t.Fatal("S3UsePathStyle = false, want true")
	}
}

func TestLoadRejectsInvalidBool(t *testing.T) {
	t.Setenv("S3_USE_PATH_STYLE", "sometimes")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() error = nil, want invalid bool error")
	}
}

func TestLoadAppliesAuthDefaults(t *testing.T) {
	t.Setenv("SESSION_COOKIE_NAME", "")
	t.Setenv("SESSION_TTL_HOURS", "")
	t.Setenv("BCRYPT_COST", "")
	t.Setenv("GOOGLE_USERINFO_URL", "")
	t.Setenv("KAKAO_USERINFO_URL", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.SessionCookieName != "hunch_session" {
		t.Fatalf("SessionCookieName = %q, want hunch_session", cfg.SessionCookieName)
	}
	if cfg.SessionTTLHours != 24*30 {
		t.Fatalf("SessionTTLHours = %d, want %d", cfg.SessionTTLHours, 24*30)
	}
	if cfg.BcryptCost != 12 {
		t.Fatalf("BcryptCost = %d, want 12", cfg.BcryptCost)
	}
	if cfg.GoogleUserInfoURL != "https://openidconnect.googleapis.com/v1/userinfo" {
		t.Fatalf("GoogleUserInfoURL = %q", cfg.GoogleUserInfoURL)
	}
	if cfg.KakaoUserInfoURL != "https://kapi.kakao.com/v2/user/me" {
		t.Fatalf("KakaoUserInfoURL = %q", cfg.KakaoUserInfoURL)
	}
}

func TestLoadRejectsInvalidAuthNumbers(t *testing.T) {
	t.Setenv("SESSION_TTL_HOURS", "0")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() error = nil, want invalid ttl error")
	}

	t.Setenv("SESSION_TTL_HOURS", "24")
	t.Setenv("BCRYPT_COST", "3")

	_, err = Load()
	if err == nil {
		t.Fatal("Load() error = nil, want invalid bcrypt cost error")
	}
}
