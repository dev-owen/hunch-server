package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	AppEnv      string
	HTTPAddr    string
	DatabaseURL string

	SessionCookieName string
	SessionTTLHours   int
	BcryptCost        int
	GoogleUserInfoURL string
	KakaoUserInfoURL  string

	OpenAIAPIKey  string
	OpenAIBaseURL string

	S3Endpoint        string
	S3Region          string
	S3Bucket          string
	S3AccessKeyID     string
	S3SecretAccessKey string
	S3UsePathStyle    bool

	OTLPTraceEndpoint string
}

func Load() (Config, error) {
	sessionTTLHours, err := envInt("SESSION_TTL_HOURS", 24*30)
	if err != nil {
		return Config{}, err
	}
	if sessionTTLHours <= 0 {
		return Config{}, fmt.Errorf("SESSION_TTL_HOURS must be positive")
	}

	bcryptCost, err := envInt("BCRYPT_COST", 12)
	if err != nil {
		return Config{}, err
	}
	if bcryptCost < 4 || bcryptCost > 31 {
		return Config{}, fmt.Errorf("BCRYPT_COST must be between 4 and 31")
	}

	usePathStyle, err := envBool("S3_USE_PATH_STYLE", true)
	if err != nil {
		return Config{}, err
	}

	return Config{
		AppEnv:      envString("APP_ENV", "local"),
		HTTPAddr:    envString("HTTP_ADDR", ":8080"),
		DatabaseURL: envString("DATABASE_URL", "postgres://hunch:hunch@localhost:5432/hunch?sslmode=disable"),

		SessionCookieName: envString("SESSION_COOKIE_NAME", "hunch_session"),
		SessionTTLHours:   sessionTTLHours,
		BcryptCost:        bcryptCost,
		GoogleUserInfoURL: envString("GOOGLE_USERINFO_URL", "https://openidconnect.googleapis.com/v1/userinfo"),
		KakaoUserInfoURL:  envString("KAKAO_USERINFO_URL", "https://kapi.kakao.com/v2/user/me"),

		OpenAIAPIKey:  envString("OPENAI_API_KEY", ""),
		OpenAIBaseURL: envString("OPENAI_BASE_URL", "https://api.openai.com/v1"),

		S3Endpoint:        envString("S3_ENDPOINT", ""),
		S3Region:          envString("S3_REGION", "auto"),
		S3Bucket:          envString("S3_BUCKET", "hunch"),
		S3AccessKeyID:     envString("S3_ACCESS_KEY_ID", ""),
		S3SecretAccessKey: envString("S3_SECRET_ACCESS_KEY", ""),
		S3UsePathStyle:    usePathStyle,

		OTLPTraceEndpoint: envString("OTEL_EXPORTER_OTLP_ENDPOINT", ""),
	}, nil
}

func envString(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func envInt(key string, fallback int) (int, error) {
	value := os.Getenv(key)
	if value == "" {
		return fallback, nil
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", key, err)
	}
	return parsed, nil
}

func envBool(key string, fallback bool) (bool, error) {
	value := os.Getenv(key)
	if value == "" {
		return fallback, nil
	}

	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("parse %s: %w", key, err)
	}
	return parsed, nil
}
