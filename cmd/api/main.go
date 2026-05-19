package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/wonjong/hunch-server/internal/account"
	"github.com/wonjong/hunch-server/internal/ai"
	"github.com/wonjong/hunch-server/internal/config"
	"github.com/wonjong/hunch-server/internal/db"
	"github.com/wonjong/hunch-server/internal/httpserver"
	"github.com/wonjong/hunch-server/internal/observability"
	"github.com/wonjong/hunch-server/internal/storage"
	"github.com/wonjong/hunch-server/internal/worker"
)

func main() {
	if err := run(); err != nil {
		slog.Error("server exited", "error", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	shutdownTelemetry, err := observability.Setup(ctx, observability.Config{
		ServiceName:       "hunch-server",
		Environment:       cfg.AppEnv,
		OTLPTraceEndpoint: cfg.OTLPTraceEndpoint,
	})
	if err != nil {
		return err
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := shutdownTelemetry(shutdownCtx); err != nil {
			slog.Warn("shutdown telemetry", "error", err)
		}
	}()

	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	_ = ai.NewClient(cfg.OpenAIAPIKey, cfg.OpenAIBaseURL)
	_, err = storage.NewS3Store(ctx, storage.Config{
		Endpoint:        cfg.S3Endpoint,
		Region:          cfg.S3Region,
		Bucket:          cfg.S3Bucket,
		AccessKeyID:     cfg.S3AccessKeyID,
		SecretAccessKey: cfg.S3SecretAccessKey,
		UsePathStyle:    cfg.S3UsePathStyle,
	})
	if err != nil {
		return err
	}
	_ = worker.NewQueue(pool)

	accountRepository := account.NewPostgresRepository(pool)
	accountVerifier := account.HTTPProviderVerifier{
		Client:            &http.Client{Timeout: 5 * time.Second},
		GoogleUserInfoURL: cfg.GoogleUserInfoURL,
		KakaoUserInfoURL:  cfg.KakaoUserInfoURL,
	}
	accountService := account.NewService(account.Config{
		Repository: accountRepository,
		Verifier:   accountVerifier,
		BcryptCost: cfg.BcryptCost,
		SessionTTL: time.Duration(cfg.SessionTTLHours) * time.Hour,
	})
	accountHandlers := account.NewHandlers(accountService, account.HandlerConfig{
		CookieName: cfg.SessionCookieName,
		CookieTTL:  time.Duration(cfg.SessionTTLHours) * time.Hour,
		Secure:     cfg.AppEnv != "local",
	})

	router := httpserver.NewRouter(httpserver.Dependencies{
		ReadinessCheck: db.PingCheck(pool),
		AccountSignup:  http.HandlerFunc(accountHandlers.Signup),
		AccountSignin:  http.HandlerFunc(accountHandlers.Signin),
		AccountSignout: http.HandlerFunc(accountHandlers.Signout),
		AccountDelete:  http.HandlerFunc(accountHandlers.Delete),
	})

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           otelhttp.NewHandler(router, "http.server"),
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", cfg.HTTPAddr)
		errCh <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return server.Shutdown(shutdownCtx)
}
