APP_NAME := hunch-server
SQLC ?= go run github.com/sqlc-dev/sqlc/cmd/sqlc@latest
GOOSE ?= go run github.com/pressly/goose/v3/cmd/goose@latest

.PHONY: help fmt build test test-ci run tidy sqlc migrate-up migrate-down compose-up compose-down

help:
	@printf "%s\n" "Targets: fmt build test test-ci run tidy sqlc migrate-up migrate-down compose-up compose-down"

fmt:
	gofmt -w ./cmd ./internal

build:
	mkdir -p bin
	go build -trimpath -o bin/$(APP_NAME) ./cmd/api

test:
	go test ./...

test-ci:
	RUN_INTEGRATION_TESTS=1 go test ./...

run:
	go run ./cmd/api

tidy:
	go mod tidy

sqlc:
	$(SQLC) generate

migrate-up:
	$(GOOSE) -dir db/migrations postgres "$$DATABASE_URL" up

migrate-down:
	$(GOOSE) -dir db/migrations postgres "$$DATABASE_URL" down

compose-up:
	docker compose up -d postgres prometheus

compose-down:
	docker compose down
