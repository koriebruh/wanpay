.PHONY: run dev build daemon-start daemon-stop daemon-status \
        migrate-up migrate-down migrate-status \
        sqlc \
        test test-unit test-integration \
        lint tidy vet infra-up infra-down infra-logs docker-build \
        install-hooks install-tools

APP_NAME = wanpey
CMD_PATH = ./cmd/api
BIN      = ./tmp/main

# Make go-installed tools (air, sqlc, golangci-lint, etc.) available without full path.
export PATH := $(shell go env GOPATH)/bin:$(PATH)

## dev: hot reload with air
dev:
	air

## run: build and run in foreground
run: build
	$(BIN) serve

## build: compile binary
build:
	@mkdir -p tmp
	go build -o $(BIN) $(CMD_PATH)

## daemon-start: run server as background daemon
daemon-start: build
	$(BIN) daemon start

## daemon-stop: send SIGTERM to daemon
daemon-stop: build
	$(BIN) daemon stop

## daemon-status: check if daemon is running
daemon-status: build
	$(BIN) daemon status

## migrate-up: apply all pending migrations
migrate-up: build
	$(BIN) migrate up

## migrate-down: rollback the last migration
migrate-down: build
	$(BIN) migrate down

## migrate-status: show current migration version
migrate-status: build
	$(BIN) migrate status

## install-hooks: install lefthook git hooks (run once after clone)
install-hooks:
	lefthook install

## install-tools: install all local dev tools (run once after clone)
install-tools:
	go install github.com/air-verse/air@latest
	go install github.com/evilmartians/lefthook@latest
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install github.com/securego/gosec/v2/cmd/gosec@latest
	go install golang.org/x/tools/cmd/goimports@latest
	go install github.com/cweill/gotests/gotests@latest
	go install golang.org/x/tools/cmd/stringer@latest
	go install github.com/fatih/gomodifytags@latest
	go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest

## sqlc: regenerate type-safe Go from SQL queries (run after editing query/*.sql)
sqlc:
	sqlc generate

## tidy: clean up go.mod and go.sum
tidy:
	go mod tidy

## test: run unit tests only (default, no network calls)
test:
	go test -race -count=1 ./...

## test-unit: same as test — explicit alias
test-unit:
	go test -race -count=1 ./...

## test-integration: run integration tests (requires .config.toml with real credentials)
test-integration:
	CONFIG_PATH=$(PWD)/.config.toml go test -race -count=1 -tags integration -v ./...

## vet: run static analysis
vet:
	go vet ./...

## lint: requires golangci-lint
lint:
	golangci-lint run ./...

## infra-up: start postgres + pgbouncer + redis + jaeger via docker compose
infra-up:
	docker compose up -d postgres pgbouncer redis jaeger

## infra-down: stop all infra containers
infra-down:
	docker compose down

## infra-logs: tail logs from all containers
infra-logs:
	docker compose logs -f

## docker-build: build production image
docker-build:
	docker build -t wanpey:latest .
