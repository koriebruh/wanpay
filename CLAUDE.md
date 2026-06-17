# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What This Project Is

Wanpey is a Go modular monolith payment gateway **aggregator** — one service that routes payments to multiple providers (Midtrans, Xendit, DOKU), handles webhooks reliably, and exposes a single API to merchants.

Module path: `wanpey/core` (Go 1.25)

## Common Commands

```bash
# First-time setup (once per clone)
make install-tools   # installs lefthook, golangci-lint, gosec, goimports, gotests, stringer, gomodifytags, sqlc
make install-hooks   # activates git hooks (pre-commit, pre-push, commit-msg)

# Daily development
make dev             # hot reload via Air (preferred)
make run             # build + run foreground
make test            # go test -race ./...
make lint            # golangci-lint run ./...
make vet             # go vet ./...

# Single package test
go test -race -run TestFunctionName ./internal/path/to/package/...

# Infrastructure (postgres + pgbouncer + redis + jaeger)
make infra-up
make infra-down

# Migrations (always run from project root — file:// path is CWD-relative)
make migrate-up
make migrate-down
make migrate-status

# sqlc — regenerate after editing query/*.sql files
make sqlc

# Daemon
make daemon-start
make daemon-stop
make daemon-status
```

## Architecture

Clean Architecture. Dependency flow:

```
delivery (HTTP) → usecase → domain ← repository
                                 ↑
                          infrastructure
                    pkg/ (importable by all layers)
```

`domain` imports nothing from this project. `pkg/` has no internal imports. Never import `internal/` from `pkg/`.

## Dependency Injection

samber/do v2 — lazy singleton, lifecycle via `do.Shutdownable`.

- All providers registered in `internal/app/app.go` `New()`
- Invoke with `do.MustInvoke[T](i)` or `do.Invoke[T](i)` (returns error)
- Services implementing `Shutdown() error` are automatically closed in reverse-registration order during `injector.Shutdown()`
- **Never pass the injector as a function argument** — inject the specific dependency instead

## Key Interfaces (not concrete types)

| Interface | Location | Reason |
|---|---|---|
| `database.SQLDB` | `internal/infrastructure/database/db.go` | Swappable SQL backend, mockable in tests |
| `database.Querier` | `internal/infrastructure/database/db.go` | Subset of SQLDB — both `*sql.DB` and `*sql.Tx` implement this |
| `cache.Cache` | `internal/infrastructure/cache/cache.go` | Redis or in-memory fallback transparently |

`cache.ProvideCache` returns `redisCache` when Redis is enabled, `memoryCache` when disabled — idempotency works in both modes.

## Database Layer

```
internal/infrastructure/database/
├── db.go               ← interfaces only: SQLDB, Querier, RunInTx, WithTx, TxFromContext
└── postgres/           ← all concrete postgres code
    ├── gen/            ← sqlc generated (DO NOT EDIT — run make sqlc to regenerate)
    ├── query/          ← SQL source files (edit here, then run make sqlc)
    ├── provider.go     ← postgres driver + ProvideDB for DI
    ├── mapper.go       ← converts gen.* ↔ entity.*
    ├── *_repo.go       ← implements domain repository interfaces
    └── outbox_repo.go  ← outbox operations for worker
```

**Transaction pattern** — always use `database.RunInTx` for multi-step operations:
```go
err = database.RunInTx(ctx, db, nil, func(ctx context.Context) error {
    if err := paymentRepo.Update(ctx, p); err != nil { return err }
    if err := mutationRepo.Save(ctx, m); err != nil { return err }
    return outboxRepo.Insert(ctx, eventType, targetURL, payload)
})
```
Tx is propagated via context — all repos call `database.TxFromContext(ctx)` internally. No need to pass tx explicitly.

**sqlc workflow** — when adding or changing a query:
1. Edit the relevant `.sql` file in `internal/infrastructure/database/postgres/query/`
2. Run `make sqlc` to regenerate `gen/`
3. Update the corresponding `*_repo.go` to use the new generated method

## Graceful Shutdown Order

`app.Shutdown()` is idempotent (`sync.Once`). The 4-stage sequence:
1. HTTP drain — `e.Shutdown(ctx)` with configured timeout
2. Worker drain — cancel worker context, `workerWg.Wait()` with 15s hard deadline
3. Logger flush — `log.Sync()` before infra closes (audit logs must not be lost)
4. Infra close — `injector.Shutdown()` (reverse-registration: Echo → Redis → Postgres → Tracer)

Workers must never query the DB after stage 4 starts — always check `ctx.Done()` first.

## Finance-Grade Patterns

**Idempotency** (`internal/delivery/http/middleware/idempotency.go`): atomic `SetNX` claim with 30s processing guard, 24h TTL for cached responses. Key format: `idempotency:{merchant_id}:{key}`. Delete key on 5xx so client can retry. Requires `merchant_id` in Echo context — skip if not set.

**Auth** (`internal/delivery/http/middleware/auth.go`): `X-API-Key` header → SHA256 hash → `MerchantRepository.FindByAPIKey`. Returns 401 for both not-found and DB errors (avoids key enumeration). Sets `merchant_id` in Echo context.

**Outbox** (`internal/infrastructure/worker/outbox_worker.go`): poll every 5s, atomic lease via `FOR UPDATE SKIP LOCKED`. Delivers up to 5 webhooks concurrently per batch. DB status writes (`markDelivered`, `scheduleRetry`) use a detached 5s context so they succeed even during graceful shutdown. `OutboxRepo.Insert()` must be called inside the same DB transaction as the payment status update.

**Circuit breaker**: wrap every provider call. Open after 5 consecutive failures, half-open after 30s. (Implementation in `internal/infrastructure/database/postgres/provider/` — not yet built.)

**Signature** (`pkg/signature/`): `Sign`/`Verify` use HMAC-SHA256 with `hmac.Equal` (constant-time). `SignSHA512`/`VerifySHA512` for DOKU.

**PII** (`pkg/mask/`): always wrap sensitive fields with `mask.Card`, `mask.Email`, `mask.Phone`, `mask.Name`, `mask.Secret` before passing to `zap.String`.

## Rate Limiting

Rate limiting is **not** done at the app level — it must be handled at the infrastructure layer (Nginx, Traefik, or cloud load balancer).

Reason: app-level in-memory rate limiting breaks with autoscaling (each instance has its own counter), blocks provider webhook callbacks (Midtrans/Xendit/DOKU), and interferes with load testing (k6).

Webhook routes (`/webhooks/*`) must never be rate limited.

## Business Model: PayFac / Aggregator

Wanpey uses the **Payment Facilitator** model:
- Wanpey holds **one account per provider** (Midtrans, Xendit, DOKU) — all merchant payments flow into Wanpey's provider accounts
- Merchant balances are tracked in the **internal `Mutation` ledger**, not at the provider level
- Cash-out (disbursement) is sent from Wanpey's provider balance to the merchant's registered bank account
- Merchants are never exposed to provider accounts — switching or adding providers is invisible to them

**Fee structure** (FeeBearer is always merchant — never customer):
- `entity.FeeConfig` = per-merchant contracted fee (VA flat, QRIS %, Disbursement flat)
- `config.FeeConfig.Margin` = platform-wide Wanpey margin added on top, toggled via `[fee.margin] enabled`
- Effective fee = merchant FeeConfig + platform margin (if enabled)
- Net settlement = paid amount − effective fee → recorded as `Mutation.Amount`

## Merchant Entity

Key design decisions:
- `APIKey` stored as SHA256 hash in DB. Format: `wpay_live_<32chars>` or `wpay_test_<32chars>`. Raw key shown once at creation/regeneration only
- `WebhookSecret` stored as SHA256 hash. Used to sign outbound webhook payloads via HMAC-SHA256 (`pkg/signature`)
- Max **3 bank accounts** per merchant (`entity.MaxBankAccounts`). Limit enforced in usecase, not entity
- `Merchant.Balance` is NOT a stored field — always calculated live via `MutationRepository.GetBalance()`
- `Merchant.CanTransact()` must return true before any payment can be created for that merchant

## Migrations

Format: `migrations/NNNNNN_name.up.sql` / `.down.sql` (golang-migrate v4). Run `make migrate-up` from project root — the `file://migrations` source path is relative to CWD.

Current migrations:
- `000001_outbox` — outbox table
- `000002_schema` — all business tables: merchants, merchant_bank_accounts, payments, disbursements, mutations, payment_audits

## Infrastructure

**PgBouncer** — app connects to PgBouncer on port **6432** (not directly to Postgres on 5432).
- Pool mode: `transaction` — connection released after each tx, optimal for autoscaling
- `max_client_conn=1000`, `default_pool_size=25` real Postgres connections
- Compatible with `database.RunInTx` — one `BEGIN/COMMIT` = one transaction
- Does NOT support: `SET` outside tx, advisory locks, `LISTEN/NOTIFY` (Wanpey does not use any of these)

## Git Hooks & CI

lefthook enforces:
- **pre-commit**: `goimports` (staged files), `go vet`, `go build`
- **pre-push**: `go test -race`, `golangci-lint`, `go mod tidy`, `gosec`
- **commit-msg**: Conventional Commits (`feat|fix|docs|style|refactor|test|chore|perf|ci|build|revert`)

GitHub Actions CI mirrors pre-push checks exactly — if hooks pass locally, CI passes.

## Config

`.config.toml` is gitignored. Copy `.config.example.toml` to `.config.toml` to run locally. Path override: `CONFIG_PATH` env var. The `config.Load()` function is public — used by both the DI container and the `migrate` CLI subcommand.
