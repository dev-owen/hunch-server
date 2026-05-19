package account

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestPostgresRepositoryCreateIdentityAndSessionRejectDeletedAccount(t *testing.T) {
	ctx := context.Background()
	repo, _ := newPostgresRepositoryTest(t, ctx)

	account, err := repo.CreateAccount(ctx, CreateAccountParams{
		Email:           "user@example.com",
		NormalizedEmail: "user@example.com",
	})
	if err != nil {
		t.Fatalf("CreateAccount() error = %v", err)
	}
	if err := repo.SoftDeleteAccount(ctx, account.ID); err != nil {
		t.Fatalf("SoftDeleteAccount() error = %v", err)
	}

	_, err = repo.CreateIdentity(ctx, CreateIdentityParams{
		AccountID:       account.ID,
		Provider:        ProviderGoogle,
		ProviderSubject: "google-sub",
		Email:           "user@example.com",
		NormalizedEmail: "user@example.com",
		EmailVerified:   true,
	})
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("CreateIdentity() error = %v, want ErrInvalidCredentials", err)
	}

	_, err = repo.CreateSession(ctx, CreateSessionParams{
		AccountID: account.ID,
		TokenHash: "deleted-account-token",
		UserAgent: "test",
		ExpiresAt: time.Now().Add(time.Hour),
	})
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("CreateSession() error = %v, want ErrInvalidCredentials", err)
	}
}

func TestPostgresRepositoryCreateIdentityWaitsForConcurrentDelete(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	repo, pool := newPostgresRepositoryTest(t, ctx)

	account, err := repo.CreateAccount(ctx, CreateAccountParams{
		Email:           "race@example.com",
		NormalizedEmail: "race@example.com",
	})
	if err != nil {
		t.Fatalf("CreateAccount() error = %v", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin delete transaction: %v", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		UPDATE accounts
		SET status = 'deleted',
		    deleted_at = now(),
		    updated_at = now()
		WHERE id = $1
		  AND deleted_at IS NULL
	`, account.ID); err != nil {
		t.Fatalf("soft delete in open transaction: %v", err)
	}

	done := make(chan error, 1)
	started := make(chan struct{})
	go func() {
		close(started)
		_, err := repo.CreateIdentity(ctx, CreateIdentityParams{
			AccountID:       account.ID,
			Provider:        ProviderGoogle,
			ProviderSubject: "race-google-sub",
			Email:           "race@example.com",
			NormalizedEmail: "race@example.com",
			EmailVerified:   true,
		})
		done <- err
	}()
	<-started

	select {
	case err := <-done:
		t.Fatalf("CreateIdentity() returned before delete transaction committed: %v", err)
	case <-time.After(250 * time.Millisecond):
	}

	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit delete transaction: %v", err)
	}

	select {
	case err := <-done:
		if !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("CreateIdentity() error = %v, want ErrInvalidCredentials", err)
		}
	case <-ctx.Done():
		t.Fatalf("CreateIdentity() did not finish after delete committed: %v", ctx.Err())
	}

	_, err = repo.FindIdentityWithAccount(ctx, ProviderGoogle, "race-google-sub")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("FindIdentityWithAccount() error = %v, want ErrInvalidCredentials", err)
	}
}

func newPostgresRepositoryTest(t *testing.T, ctx context.Context) (*PostgresRepository, *pgxpool.Pool) {
	t.Helper()
	if os.Getenv("RUN_INTEGRATION_TESTS") != "1" {
		t.Skip("set RUN_INTEGRATION_TESTS=1 to run account repository integration tests")
	}

	container, err := postgres.Run(ctx,
		"pgvector/pgvector:pg16",
		postgres.WithDatabase("hunch"),
		postgres.WithUsername("hunch"),
		postgres.WithPassword("hunch"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}
	t.Cleanup(func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			t.Fatalf("terminate postgres container: %v", err)
		}
	})

	connString, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}

	conn, err := sql.Open("pgx", connString)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatalf("set goose dialect: %v", err)
	}
	if err := goose.Up(conn, "../../db/migrations"); err != nil {
		t.Fatalf("goose up: %v", err)
	}

	pool, err := pgxpool.New(ctx, connString)
	if err != nil {
		t.Fatalf("open pgx pool: %v", err)
	}
	t.Cleanup(pool.Close)

	return NewPostgresRepository(pool), pool
}
