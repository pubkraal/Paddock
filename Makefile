.DEFAULT_GOAL := help

# ── Config ──────────────────────────────────────────────────────────────────
MIGRATE_ROLE_URL ?= postgres://paddock_migrate:paddock_migrate@localhost:5432/paddock?sslmode=disable
APP_ROLE_URL     ?= postgres://paddock_app:paddock_app@localhost:5432/paddock?sslmode=disable
GOLANGCI_CFG     := build/ci/golangci.yml
MIGRATIONS_DIR   := migrations

# ── Help ────────────────────────────────────────────────────────────────────
.PHONY: help
help: ## List available targets
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'

# ── Dev stack ───────────────────────────────────────────────────────────────
.PHONY: up
up: ## Start the local backing services (postgres, redis, minio, mailpit, sftp)
	docker compose -f deploy/docker-compose.yml up -d

.PHONY: down
down: ## Stop the local backing services
	docker compose -f deploy/docker-compose.yml down

.PHONY: run
run: up migrate ## Bring the stack up and run cmd/web (dev env, cookies over http)
	DATABASE_URL="$(APP_ROLE_URL)" \
	PADDOCK_BASE_URL="http://localhost:8080" \
	PADDOCK_MAIL_FROM="no-reply@paddock.local" \
	PADDOCK_COOKIE_SECURE="false" \
	PADDOCK_DEV="true" \
	PADDOCK_SMTP_ADDR="localhost:1025" \
	S3_ENDPOINT="http://localhost:9000" \
	S3_ACCESS_KEY_ID="minioadmin" \
	S3_SECRET_ACCESS_KEY="minioadmin" \
	S3_BUCKET="paddock-dev" \
	go run ./cmd/web

# ── Migrations (run as the BYPASSRLS migration role) ─────────────────────────
.PHONY: migrate
migrate: ## Apply app migrations (golang-migrate) then River's own schema
	migrate -path $(MIGRATIONS_DIR) -database "$(MIGRATE_ROLE_URL)" up
	go tool river migrate-up --database-url "$(MIGRATE_ROLE_URL)" --line main

.PHONY: migrate-down
migrate-down: ## Roll back River's schema then one app migration
	go tool river migrate-down --database-url "$(MIGRATE_ROLE_URL)" --line main --target-version 0
	migrate -path $(MIGRATIONS_DIR) -database "$(MIGRATE_ROLE_URL)" down 1

.PHONY: seed
seed: ## Seed dev orgs + admins (idempotent); prints the sign-in emails
	DATABASE_URL="$(MIGRATE_ROLE_URL)" go run ./cmd/seed

# ── Build / format / lint / test (mirror the CLAUDE.md pre-commit gates) ─────
.PHONY: build
build: ## Compile all three deployables
	go build ./...

.PHONY: fmt
fmt: ## Format with gofumpt + gofmt
	gofumpt -w .
	gofmt -w .

.PHONY: test
test: ## Unit tier — sqlmock/miniredis, no Docker
	go test ./internal/... ./web/... -count=1

.PHONY: cover
cover: ## Unit tier with coverage report
	go test ./internal/... ./web/... -count=1 -coverprofile=coverage.out
	go tool cover -func=coverage.out

.PHONY: test-int
test-int: ## Integration tier — testcontainers (Docker required)
	go test ./... -tags integration -count=1

.PHONY: lint
lint: ## Lint changed code against main
	golangci-lint run -c $(GOLANGCI_CFG) --new-from-rev=main

.PHONY: lint-all
lint-all: ## Lint the whole tree
	golangci-lint run -c $(GOLANGCI_CFG)

.PHONY: tidy
tidy: ## go mod tidy (must produce zero diff)
	go mod tidy
