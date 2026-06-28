# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What This Project Is

Wanpey is a Go modular monolith payment gateway **aggregator** — one service that routes payments to multiple providers (Midtrans, Xendit, DOKU, iPaymu), handles webhooks reliably, and exposes a single API to merchants.

Module path: `wanpey/core` (Go 1.25)

## Common Commands

```bash
# First-time setup (once per clone)
make install-tools   # installs air, lefthook, golangci-lint, gosec, goimports, gotests, stringer, gomodifytags, sqlc
make install-hooks   # activates git hooks (pre-commit, pre-push, commit-msg)

# Daily development
make dev             # hot reload via Air (preferred)
make run             # build + run foreground
make test            # unit tests only — no network calls (default for CI)
make test-unit       # same as make test
make test-integration # hit real provider sandboxes — requires .config.toml with credentials
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

# Seed first admin (run once after first migrate-up)
go run ./cmd/api seed-admin --email admin@example.com --password secret --role super_admin

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
| `gateway.PaymentGateway` | `internal/domain/gateway/gateway.go` | Implemented by each provider adapter |
| `gateway.DisbursementGateway` | `internal/domain/gateway/gateway.go` | Xendit and DOKU only |

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
    ├── fee_repo.go     ← fee_defaults, platform_margin, fee_holidays, fee_audit_logs
    └── outbox_repo.go  ← outbox CRUD: Insert, Lease, MarkDelivered, MarkFailedFinal, ScheduleRetry, ListByMerchant
```

**Transaction pattern** — always use `database.RunInTx` for multi-step operations:
```go
err = database.RunInTx(ctx, db, nil, func(ctx context.Context) error {
    if err := paymentRepo.Update(ctx, p); err != nil { return err }
    if err := mutationRepo.Save(ctx, m); err != nil { return err }
    return outboxRepo.Insert(ctx, eventType, targetURL, merchantID, payload)
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

**Outbox** (`internal/infrastructure/worker/outbox_worker.go`): poll every 5s, atomic lease via `FOR UPDATE SKIP LOCKED`. Delivers up to 5 webhooks concurrently per batch. DB status writes use a detached 5s context so they succeed even during graceful shutdown. All status updates go through `OutboxRepo` methods — no raw SQL in worker. `OutboxRepo.Insert()` must be called inside the same DB transaction as the payment status update. Outbox rows carry `merchant_id` for webhook signing key lookup and delivery history.

**Webhook signing** (`pkg/signature/`, `internal/infrastructure/worker/outbox_worker.go`): outbound merchant webhooks are signed with the merchant's `webhook_signing_key` (HMAC-SHA256). Merchants verify with `X-Wanpey-Signature` header. Standard payload format via `pkg/webhook/payload.go`.

**Webhook IP allowlist** (`internal/delivery/http/middleware/webhook_allowlist.go`): optional per-provider CIDR allowlist applied to `/webhooks/*` routes. Configure via `[provider.midtrans] webhook_allowed_ips = ["..."]`. Empty list = accept all. Enabled at router construction when any provider has IPs configured.

**Circuit breaker** (`internal/infrastructure/provider/circuit_breaker.go`): wraps every provider call. `CBPaymentGateway` and `CBDisbursementGateway` implement the gateway interfaces and proxy all network calls through `gobreaker`. Settings configurable via `[provider.circuit_breaker]` in config. `ParseWebhook` bypasses the breaker (local operation, no network).

**Two-step cancel** (`internal/usecase/impl/payment.go` `CancelPayment`): sets status → `cancelling` in DB first (committed tx), then calls provider cancel, then sets `cancelled`. If provider call fails, reverts to `pending`. The intermediate `cancelling` state prevents double-cancel races and makes the cancel operation recoverable.

**Disbursement reservation** (`internal/usecase/impl/disbursement.go`): inserts disbursement row with `pending` status inside the balance-check transaction before calling the provider. Prevents double-spend from concurrent requests. Disbursement must reference a verified registered bank account (`DisburseInput.BankAccountID` → `FindBankAccountByID` + `IsVerified` check).

**AddBankAccount race** (`internal/usecase/impl/merchant.go`): `CountBankAccounts` runs inside `RunInTx` with `FOR UPDATE` lock — serializes concurrent adds to enforce the 3-account maximum.

**Signature** (`pkg/signature/`): `Sign`/`Verify` use HMAC-SHA256 with `hmac.Equal` (constant-time). `SignSHA512`/`VerifySHA512` for DOKU.

**PII** (`pkg/mask/`): always wrap sensitive fields with `mask.Card`, `mask.Email`, `mask.Phone`, `mask.Name`, `mask.Secret` before passing to `zap.String`.

**Validation** (`pkg/validator/`): `EchoValidator` wraps go-playground/validator. Registered with `e.Validator`. Handlers call `c.Validate(&input)` — returns `apperror.BadRequest` with per-field details on failure.

**Payment expiry** (`internal/infrastructure/worker/expiry_worker.go`): background worker polls expired pending payments and updates status to `expired` with a `PAYMENT_EXPIRED` audit event.

## Fee Resolution Engine

All fee calculations go through `FeeResolver` (`internal/usecase/impl/fee_resolver.go`) — never compute fees ad-hoc in handlers or usecases.

**Resolution priority (highest → lowest):**
1. Merchant's contracted `FeeConfig` (per-method, stored in `merchants` table)
2. Global default (`fee_defaults` table, admin-managed)
3. Platform margin (`platform_margin` table) — added on top of base fee when `enabled = true`
4. Holiday surcharge (`fee_holidays` table) — added on top when `date` matches today and `is_active = true`

**Total fee** = base fee (merchant or global) + platform margin + holiday surcharge

**Boot seed**: on startup, if `platform_margin.updated_by = ''` (migration default), config values from `[fee.margin]` are seeded into the DB. Once an admin updates it manually, the seed is skipped forever.

**Fee audit log**: every fee change (global default, platform margin, merchant fee, holiday) must call `FeeRepository.WriteAuditLog()` with admin_id, reason, old_value, new_value. The `fee_audit_logs` table is immutable append-only.

**`FeeResolution` struct fields:**
- `BaseFee` — from merchant contract or global default
- `PlatformMargin` — Wanpey's margin
- `HolidaySurcharge` — 0 if not a holiday
- `TotalFee` — sum of all three
- `Source` — `"merchant_contract"` or `"global_default"`
- `HolidayName` — name of the holiday if applicable

## Admin System

**Entity** (`internal/domain/entity/admin.go`): `Admin` with roles `super_admin`, `ops`, `finance`.

**Auth**: JWT-based. Login → `POST /admin/login` → access token (short TTL) + refresh token. All admin routes except login/refresh require `Authorization: Bearer <access_token>`. Middleware: `AdminJWTAuth` + `RequireRole`.

**Permission matrix:**

| Action | super_admin | ops | finance |
|--------|-------------|-----|---------|
| Create/delete merchant | ✅ | ✅ | ❌ |
| Approve/suspend merchant | ✅ | ✅ | ❌ |
| Set merchant fee | ✅ | ❌ | ✅ |
| Set cashout limit | ✅ | ❌ | ✅ |
| Verify bank account | ✅ | ✅ | ❌ |
| Manage admins | ✅ | ❌ | ❌ |
| View payments/disbursements | ✅ | ✅ | ✅ |
| View/update provider balances | ✅ | ❌ | ✅ |
| Manage fee defaults/margin | ✅ | ❌ | ✅ |
| Update platform margin | ✅ | ❌ | ❌ |
| Manage holiday surcharges | ✅ | ❌ | ✅ |

**Merchant registration**: public `POST /v1/merchants` is removed. Merchants are created by admins only via `POST /admin/merchants`.

**Self-deactivation guard**: admin cannot deactivate their own account.

## Rate Limiting

Rate limiting is **not** done at the app level — it must be handled at the infrastructure layer (Nginx, Traefik, or cloud load balancer).

Reason: app-level in-memory rate limiting breaks with autoscaling (each instance has its own counter), blocks provider webhook callbacks (Midtrans/Xendit/DOKU), and interferes with load testing (k6).

Webhook routes (`/webhooks/*`) must never be rate limited.

## Provider Gateway Layer

```
internal/domain/gateway/gateway.go     ← PaymentGateway + DisbursementGateway + ProviderCapability
internal/infrastructure/provider/
├── circuit_breaker.go                  ← CBPaymentGateway + CBDisbursementGateway wrappers
├── midtrans/midtrans.go               ← Midtrans Core API adapter
├── xendit/xendit.go                   ← Xendit Payment Request v3 + Payouts v2
├── doku/doku.go                       ← DOKU SNAP BI-SNAP adapter
└── ipaymu/ipaymu.go                   ← iPaymu direct API adapter
```

**Provider HTTP timeouts**: all providers set `http.Client{Timeout: 15s}`. Echo request timeout is 30s — this gives 15s buffer for handler overhead and prevents provider timeout from racing with Echo context cancellation.

**Provider capabilities** — each provider declares what it supports via `Capabilities()`:

| Provider | CashIn (VA/QRIS) | CashOut (Disbursement) |
|---|---|---|
| Midtrans | ✅ | ❌ |
| Xendit | ✅ | ✅ |
| DOKU | ✅ | ✅ |
| iPaymu | ✅ | ❌ |

**Webhook callback routing** — one handler, dispatches by provider name from URL:
```
POST /webhooks/{provider}/payment       → PaymentUsecase.HandleWebhook
POST /webhooks/{provider}/disbursement  → DisbursementUsecase.HandleDisbursementCallback
```

**Webhook idempotency**: `HandleWebhook` re-checks payment status inside the transaction with `FindByIDForUpdate` after acquiring the row lock. If `IsFinal()` is true, returns nil (idempotent). Prevents double mutation when provider retries before receiving a response.

**Midtrans specifics:**
- Mandiri VA uses `payment_type: "echannel"` — response gives `bill_key` + `biller_code` (always `70012`)
- QRIS: `qr_string` fetched via second GET to `actions[0].url`
- Webhook signature: `SHA512(order_id + status_code + gross_amount + server_key)`
- Does NOT support disbursement

**Xendit specifics:**
- Uses Payment Request API v3 (`/v3/payment_requests`) with `api-version` header
- `ProviderPaymentID` = `payment_request_id` — used for cancel/status calls (NOT the merchant reference)
- Webhook: static token comparison via `x-callback-token` header (not HMAC)
- Disbursement via Payouts v2 API (`/v2/payouts`)

**DOKU specifics:**
- Two-step auth: B2B token (SHA256withRSA) then request (HMAC-SHA512 hex)
- Private key (RSA PKCS8 PEM) required — supply via `DOKU_PRIVATE_KEY_PEM` env var (takes precedence) or `private_key_pem` in config. **Never commit `.pem` files** — `*.pem` is in `.gitignore`.
- `migrate_dsn` must bypass PgBouncer (advisory locks used by golang-migrate)
- BaseURL switches between sandbox/production based on `cfg.IsProduction`

**iPaymu specifics:**
- Auth headers: `va` (merchant VA), `signature`, `timestamp` (no Authorization header)
- Signature: `HMAC-SHA256(apiKey, "POST:{va}:{SHA256(body)}:{apiKey}")` — timestamp NOT in signature
- Body must omit empty string fields — iPaymu strips them before hashing on server side
- Webhook verified via `x-wt` token header
- Does NOT support cancel or disbursement via API v2

## HTTP Response Format

```json
// Success
{ "success": true, "data": {...}, "meta": { "trace_id": "...", "timestamp": "..." } }

// List
{ "success": true, "data": [...], "meta": { "trace_id": "...", "pagination": {...} } }

// Error
{ "success": false, "error": { "message": "...", "details": [...] }, "meta": { "trace_id": "..." } }
```

`trace_id` is the OpenTelemetry span ID — searchable in Jaeger UI. `X-Request-ID` header is set on every response by Echo middleware. `merchant_id` is included in every request log line when the route is authenticated.

## Business Model: PayFac / Aggregator

Wanpey uses the **Payment Facilitator** model:
- Wanpey holds **one account per provider** (Midtrans, Xendit, DOKU, iPaymu) — all merchant payments flow into Wanpey's provider accounts
- Merchant balances are tracked in the **internal `Mutation` ledger**, not at the provider level
- Cash-out (disbursement) is sent from Wanpey's provider balance to the merchant's **verified registered bank account**
- Merchants are never exposed to provider accounts — switching or adding providers is invisible to them
- `provider_balances` table tracks the platform's known balance at each provider for audit and reconciliation

**Fee structure** (FeeBearer is always merchant — never customer):
- `entity.FeeConfig` = per-merchant contracted fee (VA flat, QRIS %, Disbursement flat)
- `fee_defaults` table = global fallback fee when merchant has no contract
- `platform_margin` table = Wanpey's margin added on top, toggled per method
- `fee_holidays` table = date-specific surcharge added on top (e.g. public holidays)
- Effective fee = base (merchant or global) + platform margin + holiday surcharge
- Net settlement = paid amount − effective fee → recorded as `Mutation.Amount`
- All fee changes require admin action + mandatory reason → written to `fee_audit_logs`

## Merchant Entity

Key design decisions:
- `APIKey` stored as SHA256 hash in DB. Format: `wpay_live_<32chars>` (production) or `wpay_test_<32chars>` (sandbox). `IsProduction` field on merchant controls which prefix is used. Raw key shown once at creation/regeneration only.
- `WebhookSecret` stored as SHA256 hash. Used to sign outbound webhook payloads via HMAC-SHA256 (`pkg/signature`)
- Max **3 bank accounts** per merchant (`entity.MaxBankAccounts`). Limit enforced in usecase inside a `RunInTx` with `SELECT COUNT(*) FOR UPDATE` to prevent concurrent bypass.
- `Merchant.Balance` is NOT a stored field — always calculated live via `MutationRepository.GetBalance()`
- `Merchant.DailyCashoutLimit` — max total disbursement per day in IDR; `0` = unlimited
- `Merchant.CanTransact()` must return true before any payment can be created for that merchant
- Merchant cannot set their own `FeeConfig` — only admins can via `PATCH /admin/merchants/:id/fee` with a mandatory `reason` field

## Migrations

Format: `migrations/NNNNNN_name.up.sql` / `.down.sql` (golang-migrate v4). Run `make migrate-up` from project root — the `file://migrations` source path is relative to CWD.

Current migrations:
- `000001_outbox` — outbox table (with `merchant_id`, `last_error`, retry columns)
- `000002_schema` — all business tables: merchants (`is_production`, `daily_cashout_limit`), merchant_bank_accounts, payments, disbursements, mutations, payment_audits, provider_balances
- `000003_add_provider_payment_id` — `provider_payment_id` column on payments (required for Xendit cancel/status)
- `000004_outbox_merchant_webhook` — adds `merchant_id` + `webhook_signing_key` to outbox; per-payment `callback_url`
- `000005_outbox_webhook` — outbox signing infrastructure
- `000006_payment_cancelling_status` — adds `cancelling` to payments and payment_audits CHECK constraints
- `000007_merchant_is_production` — `is_production BOOLEAN NOT NULL DEFAULT FALSE` on merchants
- `000008_fee_tables` — `fee_defaults` and `platform_margin` tables with seeded default rows
- `000009_fee_audit_log` — `fee_audit_logs` table (immutable, append-only)
- `000010_fee_holidays` — `fee_holidays` table with `UNIQUE(date)` constraint

## Infrastructure

**PgBouncer** — app connects to PgBouncer on port **6432** (not directly to Postgres on 5432).
- Pool mode: `transaction` — connection released after each tx, optimal for autoscaling
- `max_client_conn=1000`, `default_pool_size=25` real Postgres connections
- Compatible with `database.RunInTx` — one `BEGIN/COMMIT` = one transaction
- Does NOT support: `SET` outside tx, advisory locks, `LISTEN/NOTIFY` (Wanpey does not use any of these)

**Asynq + Asynqmon** — async task queue for treasury operations.
- Asynq uses Redis as broker (same Redis instance, no extra infra)
- Workers run in-process (not a separate container)
- Asynqmon dashboard: `http://localhost:8081` after `make infra-up`
- Config: `[taskqueue]` section in `.config.toml`

**Treasury scheduler flow** (`internal/infrastructure/taskqueue/treasury/`):
```
Cron (*/15 min): treasury:check_topup
  → read provider_balances (skips providers with LastReconciledAt == nil)
  → if balance < threshold: enqueue treasury:execute_topup
  → TaskID dedup: only 1 pending topup per provider at a time

treasury:execute_topup
  → returns nil with warning log (safe no-op — NOT IMPLEMENTED)
  → implement actual inter-bank transfer + update provider_balances when ready
  → MaxRetry(3) with exponential backoff
```

Large cashout trigger: `HandleLargeCashoutTopupCheck()` — call from disbursement usecase when single cashout > `large_cashout_threshold_idr`.

**Shutdown**: `srv.Shutdown()` + `scheduler.Shutdown()` MUST be called in `app.Shutdown()` to drain in-flight tasks — critical for financial operations (wired during DI task).

**Health endpoint** (`GET /health`): checks DB connectivity, cache reachability, and `outbox_backlog` (count of pending undelivered outbox events). Non-zero backlog is not an error but signals webhook delivery lag.

## Testing

**Unit tests** (default): mock HTTP via `httptest`, no real credentials needed.
```bash
make test
```

**Integration tests**: hit real provider sandboxes. Requires `.config.toml` with valid credentials. Use build tag `integration` — excluded from CI.
```bash
make test-integration
```

Integration tests use `t.Skip()` if credentials are empty — safe to commit.

## Git Hooks & CI

lefthook enforces:
- **pre-commit**: `goimports` (staged files), `go vet`, `go build`
- **pre-push**: `go test -race`, `golangci-lint`, `go mod tidy`, `gosec`
- **commit-msg**: Conventional Commits (`feat|fix|docs|style|refactor|test|chore|perf|ci|build|revert`)

GitHub Actions CI mirrors pre-push checks exactly — if hooks pass locally, CI passes.

## Config

`.config.toml` is gitignored. Copy `.config.example.toml` to `.config.toml` to run locally. Path override: `CONFIG_PATH` env var. The `config.Load()` function is public — used by both the DI container and the `migrate` CLI subcommand.

**Important:** `migrate_dsn` must point directly to Postgres (port 5432), not PgBouncer. golang-migrate uses advisory locks which PgBouncer transaction mode does not support.

**Secret env vars** (take precedence over config file values):
- `DOKU_PRIVATE_KEY_PEM` — DOKU RSA private key PEM content. Use this in production instead of `private_key_pem` in config.

**Fee margin config** (`[fee.margin]`): on first boot, if `platform_margin` table row has `updated_by = ''` (migration default), the config values are seeded into the DB. Once an admin updates the margin via the API, the config values are ignored forever.
