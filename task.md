# Wanpey — Full Development Task List

**Full codebase audit tanggal 2026-06-25.**  
Setiap task mencantumkan file yang terlibat, alasan, dan dependency. Urutan priority **P0 → P3**.

---

## Legend

| Label | Arti |
|---|---|
| **P0** | Kritis — bug aktif atau security hole, harus dikerjakan sebelum apapun |
| **P1** | Blocking — fitur inti yang belum ada, sistem belum bisa beroperasi tanpa ini |
| **P2** | Penting — alur tidak lengkap, tapi sistem bisa jalan dalam mode terbatas |
| **P3** | Improvement — resilience, polish, operasional |
| `[BUG]` | Perilaku yang sudah salah sekarang |
| `[SEC]` | Security/business risk |
| `[FEAT]` | Fitur baru |
| `[RACE]` | Race condition / double-spend risk |
| `[MISS]` | Sesuatu yang hilang dari flow yang sudah ada |

---

## ⚠️ PRODUCTION BLOCKER SUMMARY

Sistem **TIDAK LAYAK PROD** karena 9 hal berikut (semua P0):

| # | Isu | Dampak |
|---|---|---|
| T-01 | `POST /v1/merchants` public | Siapapun bisa daftar merchant dan dapat API key |
| T-02 | Merchant bisa set fee sendiri | Revenue Wanpey bisa di-0-kan oleh merchant |
| T-03 | `ProviderPaymentID` tidak tersimpan | Cancel payment Xendit kirim ID salah → error di provider |
| T-04 | DOKU selalu pakai sandbox URL | Traffic production dikirim ke sandbox DOKU |
| T-05 | DOKU HMAC signature belum bekerja | Semua API call ke DOKU akan gagal autentikasi |
| T-06 | Header case mismatch Xendit & DOKU webhook | Semua webhook Xendit/DOKU ditolak → payment tak pernah update ke paid |
| T-07 | iPaymu webhook tanpa verifikasi signature | Attacker bisa palsukan payment success |
| T-08 | Double-spend race condition disbursement | Dua request concurrent bisa habiskan balance yang sama |
| T-09 | Tidak ada sistem admin | Merchant stuck di status `pending` selamanya, tidak ada cara approve |

---

## P0 — Kritis (Harus dikerjakan duluan)

### T-01 `[SEC]` Hapus akses publik merchant registration — admin-only
> *(Sebelumnya bernomor T-01, sekarang dipertahankan. Temuan baru T-04 s/d T-07 ditambahkan.)*

**File:** `internal/delivery/http/router.go`, `internal/delivery/http/handler/merchant_handler.go`, `internal/usecase/merchant.go`, `internal/usecase/impl/merchant.go`

**Masalah:** `POST /v1/merchants` saat ini public tanpa auth. Siapa saja bisa mendaftarkan merchant. Ini artinya siapa saja bisa mendapatkan API key dan langsung membuat payment.

**Fix:**
- Pindahkan endpoint ke `/admin/merchants` (akan dikerjakan di T-10)
- Hapus `e.POST("/v1/merchants", r.Merchant.Create)` dari router publik
- Endpoint ini hanya boleh dipanggil oleh admin yang sudah login (JWT auth)

**Impact:** Tanpa ini, sistem tidak bisa go-live.

---

### T-02 `[SEC]` Cabut kemampuan merchant mengubah fee config sendiri

**File:** `internal/usecase/merchant.go` (`UpdateMerchantInput`), `internal/usecase/impl/merchant.go` (`Update()`), `internal/delivery/http/handler/merchant_handler.go`

**Masalah:** `UpdateMerchantInput` memiliki field `FeeConfig entity.FeeConfig` dan `Update()` langsung meng-assign `m.FeeConfig = input.FeeConfig`. Merchant bisa SET fee mereka sendiri menjadi 0, yang berarti Wanpey tidak mendapat margin apapun.

**Fix:**
- Hapus `FeeConfig` dari `UpdateMerchantInput`
- Merchant self-service `PATCH /v1/merchants/me` hanya boleh update: `Name`, `Phone`, `WebhookURL`
- `Email` update harus melalui admin (karena menyangkut unique constraint dan identitas)
- Fee config hanya bisa diubah melalui `PATCH /admin/merchants/:id/fee` (admin endpoint)

---

### T-03 `[BUG]` `ProviderPaymentID` tidak disimpan di entity Payment — Xendit cancel/status akan gagal

**File:** `internal/domain/entity/payment.go`, `internal/usecase/impl/payment.go`, `internal/infrastructure/database/postgres/` (migration, query, mapper, payment_repo)

**Masalah:** `gateway.CreateVAResponse` dan `CreateQRISResponse` mengembalikan `ProviderPaymentID` (untuk Xendit: `payment_request_id`, untuk DOKU: internal reference). Field ini **tidak pernah disimpan** ke entity maupun DB. Saat `CancelPayment` dipanggil, kode mengirim `p.ExternalID` ke provider — tapi Xendit memerlukan `payment_request_id` berbeda dari external ID kita.

**CLAUDE.md notes:** `ProviderPaymentID = payment_request_id — used for cancel/status calls (NOT the merchant reference)`

**Fix:**
- Tambah field `ProviderPaymentID string` ke `entity.Payment`
- Migration: `000003_add_provider_payment_id.up.sql` — tambah kolom `provider_payment_id TEXT NOT NULL DEFAULT ''` ke tabel `payments`
- Update sqlc query `query/payments.sql` lalu `make sqlc`
- Update `mapper.go` untuk include field baru
- Simpan `resp.ProviderPaymentID` di `CreateVA` dan `CreateQRIS` sebelum `paymentRepo.Save`
- `CancelPayment` dan `GetStatus` harus pakai `p.ProviderPaymentID` (bukan `p.ExternalID`) saat memanggil provider

---

### T-04 `[BUG]` Audit event `PAYMENT_CREATED` tidak pernah di-insert

**File:** `internal/usecase/impl/payment.go` (`CreateVA`, `CreateQRIS`), `internal/domain/entity/payment_audit.go`

**Masalah:** Tabel `payment_audits` punya constraint `CHECK (event_type IN ('PAYMENT_CREATED', 'STATUS_CHANGED', ...))` dan tabel memiliki field `PAYMENT_CREATED`, tapi `CreateVA` dan `CreateQRIS` tidak pernah insert audit record. Audit trail untuk payment creation hilang.

**Fix:** Di `CreateVA` dan `CreateQRIS`, setelah `paymentRepo.Save`, insert audit record:
```go
auditRepo.Save(ctx, &entity.PaymentAudit{
    PaymentID: p.ID,
    EventType: entity.AuditEventPaymentCreated,
    NewStatus: entity.PaymentStatusPending,
    Actor:     "merchant:" + input.MerchantID,
})
```

---

### T-04 `[BUG]` DOKU selalu pakai sandbox URL — production traffic dikirim ke sandbox

**File:** `internal/infrastructure/provider/doku/doku.go` (fungsi `New()`, baris 78-87)

**Masalah:** Di `doku.New()`, field `baseURL` selalu di-set ke `sandboxBaseURL` terlepas dari config `cfg.IsProduction`:

```go
return &Gateway{
    ...
    baseURL: sandboxBaseURL, // ← HARDCODED! cfg.IsProduction tidak pernah dicek
    ...
}
```

Bandingkan dengan Midtrans dan iPaymu yang benar:
```go
// midtrans.go — BENAR:
baseURL := productionBaseURL
if !cfg.IsProduction {
    baseURL = sandboxBaseURL
}
```

**Dampak:** Semua transaksi DOKU di production environment akan dikirim ke sandbox. Pembayaran akan tampak berhasil di sisi kita tapi tidak ada uang yang bergerak.

**Fix:**
```go
baseURL := sandboxBaseURL
if cfg.Enabled && cfg.IsProduction {
    baseURL = productionBaseURL
}
```

---

### T-05 `[BUG]` DOKU HMAC request signature belum berfungsi — semua API call DOKU akan gagal

**File:** `internal/infrastructure/provider/doku/doku.go` (fungsi `post()`, sekitar baris 349-353)

**Masalah:** Ada komentar TODO yang tidak diselesaikan di kode produksi:

```go
_ = "POST:" + path + ":" + token + ":" + bodyHash + ":" + ts // for reference
// TODO: X-SIGNATURE still failing. Tried: HMAC-SHA256/512 + hex/base64 with api_key and secret_key.
// Contact DOKU support to confirm which dashboard credential maps to clientSecret for HMAC signing.
sig := hmacSHA512hex512(g.hmacKey, "POST:"+path+":"+token+":"+bodyHash+":"+ts)
```

Artinya implementer sudah tahu signature DOKU tidak bekerja tapi belum diselesaikan. Semua request API ke DOKU (CreateVA, CreateQRIS, Disburse, dll) akan ditolak DOKU dengan signature error.

**Dampak:** DOKU provider tidak bisa digunakan sama sekali.

**Fix:** Perlu konfirmasi ke DOKU support mengenai format `stringToSign` yang benar, kemudian:
- Tentukan apakah menggunakan `api_key` atau `secret_key` sebagai HMAC key
- Tentukan format exact: hex vs base64, SHA256 vs SHA512
- Hapus komentar TODO setelah fixed dan diverifikasi di sandbox

---

### T-06 `[BUG]` Header case mismatch — semua webhook Xendit dan DOKU pasti ditolak

**File:** `internal/delivery/http/handler/webhook_handler.go` (`extractHeaders`), `internal/infrastructure/provider/xendit/xendit.go` (`ParseWebhook`), `internal/infrastructure/provider/doku/doku.go` (`ParseWebhook`)

**Masalah:** Go's `net/http` menyimpan header dengan canonical form (Title-Case), bukan lowercase. Contoh:
- `x-callback-token` → disimpan sebagai `X-Callback-Token`
- `x-timestamp` → disimpan sebagai `X-Timestamp`
- `x-signature` → disimpan sebagai `X-Signature`

Namun kode provider membaca header dengan lowercase key:

```go
// xendit.go — SELALU KOSONG karena key tidak cocok
if headers["x-callback-token"] != g.webhookToken { // ← key salah

// doku.go — SELALU KOSONG
ts := headers["x-timestamp"]     // ← key salah
sig := headers["x-signature"]    // ← key salah
```

**Dampak:** 
- Xendit: `headers["x-callback-token"]` selalu `""`, `"" != g.webhookToken` → 100% webhook Xendit ditolak → tidak ada payment Xendit yang pernah berhasil update ke `paid`
- DOKU: `ts` dan `sig` selalu `""` → signature verification selalu gagal → 100% webhook DOKU ditolak

**Fix:** Gunakan `textproto.CanonicalMIMEHeaderKey` atau simpan headers dengan lowercase normalization di `extractHeaders`:

```go
// webhook_handler.go
func extractHeaders(r *http.Request) map[string]string {
    out := make(map[string]string, len(r.Header))
    for k, v := range r.Header {
        if len(v) > 0 {
            out[strings.ToLower(k)] = v[0] // ← lowercase semua key
        }
    }
    return out
}
```

Alternatif: ubah provider code untuk pakai canonical key (`"X-Callback-Token"` bukan `"x-callback-token"`). Pilih satu pendekatan dan konsisten.

---

### T-07 `[SEC]` iPaymu `ParseWebhook` tidak verifikasi signature — attacker bisa palsukan payment paid

**File:** `internal/infrastructure/provider/ipaymu/ipaymu.go` (`ParseWebhook`, baris 146-158)

**Masalah:** iPaymu `ParseWebhook` langsung parse JSON tanpa verifikasi apapun:

```go
func (g *Gateway) ParseWebhook(_ context.Context, _ map[string]string, body []byte) (*gateway.WebhookEvent, error) {
    var n notification
    if err := json.Unmarshal(body, &n); err != nil {
        return nil, fmt.Errorf("ipaymu webhook: unmarshal: %w", err)
    }
    // ← TIDAK ADA SIGNATURE CHECK!
    return &gateway.WebhookEvent{
        ExternalID: n.ReferenceID,
        Status:     mapStatus(n.StatusCode),
        ...
    }
}
```

Bandingkan: Midtrans verifikasi SHA512 signature, Xendit verifikasi callback token, DOKU verifikasi HMAC-SHA512.

**Dampak:** Attacker yang mengetahui `reference_id` dari payment (bisa ditebak dari pola `wpay-{timestamp}-{hex}`) bisa mengirim POST ke `/webhooks/ipaymu/payment` dengan body `{"reference_id": "wpay-xxx", "status_code": "paid"}` dan sistem akan menandai payment sebagai paid, lalu insert Mutation credit ke merchant balance, tanpa uang yang masuk.

**Fix:** iPaymu tidak menyediakan HMAC signature di webhook-nya (berbeda dengan provider lain). Pendekatan defense-in-depth yang tersedia:
1. IP allowlist untuk IP milik iPaymu (dari dokumentasi mereka)
2. Secret token di query param: iPaymu mendukung `notifyUrl` dengan parameter tambahan, bisa tambahkan `?token=<random_secret>` yang diverifikasi saat webhook diterima
3. Validasi minimum: pastikan `ReferenceID` ada di DB dengan status `pending` sebelum diproses (sudah dilakukan di `HandleWebhook` usecase, tapi ini defense layer 2, bukan pengganti signature)

Rekomendasi: implementasi opsi 2 (token di URL) sebagai verifikasi minimum + opsi 3 sebagai defense in depth.

---

### T-08 `[RACE]` Disbursement double-spend — balance check dan provider call tidak atomic

> *(Sebelumnya T-05 di draft pertama)*

**File:** `internal/usecase/impl/disbursement.go` (`Disburse()`)

**Masalah:** Flow saat ini:
1. Tx-1: lock merchant row, cek balance, cek daily limit → **Tx-1 commit, lock dilepas**
2. Call provider (outside tx, bisa 1-3 detik)
3. Save disbursement ke DB

Antara step 1 dan 3, request disbursement kedua yang concurrent bisa lolos cek balance dengan balance yang sama → double-spend.

**Fix — Reservation Pattern:**
1. Tx-1: lock merchant row, cek balance + daily limit, **INSERT disbursement dengan `status=pending`** (reserve slot) → Tx-1 commit
2. Call provider (outside tx). Jika gagal → UPDATE disbursement jadi `failed`, return error ke caller
3. Tx-2: UPDATE disbursement dengan `external_id` dan `status` dari provider response

Dengan ini, concurrent request kedua akan melihat disbursement pending yang sudah ada dan balance check kedua akan menghitung disbursement pending sebagai sudah terpakai.

- Update `repository.DisbursementRepository` — tambah `SumDisbursementsPending(ctx, merchantID)` atau include pending dalam balance calc
- Alternatif: `GetBalance` harus memperhitungkan pending disbursements

---

## P1 — Blocking (Sistem tidak bisa beroperasi tanpa ini)

### T-06 `[FEAT]` Admin entity, migration, dan repository

**File baru/ubah:**
- `internal/domain/entity/admin.go` ← buat baru
- `internal/domain/repository/admin.go` ← buat baru
- `internal/infrastructure/database/postgres/admin_repo.go` ← buat baru
- `internal/infrastructure/database/postgres/query/admin.sql` ← buat baru, lalu `make sqlc`
- `migrations/000004_admin.up.sql`, `000004_admin.down.sql` ← buat baru

**Spesifikasi entity:**
```go
type AdminRole string
const (
    AdminRoleSuperAdmin AdminRole = "super_admin" // full access
    AdminRoleOps        AdminRole = "ops"         // approve/suspend merchant, view semua data
    AdminRoleFinance    AdminRole = "finance"      // lihat semua transaksi, update fee, provider balance
)

type Admin struct {
    ID           string
    Email        string
    PasswordHash string    // bcrypt
    Role         AdminRole
    IsActive     bool
    LastLoginAt  *time.Time
    CreatedAt    time.Time
    UpdatedAt    time.Time
}
```

**Migration schema:**
```sql
CREATE TABLE admins (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    email         TEXT        NOT NULL UNIQUE,
    password_hash TEXT        NOT NULL,
    role          TEXT        NOT NULL DEFAULT 'ops',
    is_active     BOOLEAN     NOT NULL DEFAULT true,
    last_login_at TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_admin_role CHECK (role IN ('super_admin', 'ops', 'finance'))
);
```

**Repository interface:**
```go
type AdminRepository interface {
    Save(ctx context.Context, admin *entity.Admin) error
    FindByID(ctx context.Context, id string) (*entity.Admin, error)
    FindByEmail(ctx context.Context, email string) (*entity.Admin, error)
    UpdateLastLogin(ctx context.Context, id string) error
    Update(ctx context.Context, admin *entity.Admin) error
    List(ctx context.Context, page, limit int) ([]*entity.Admin, int64, error)
}
```

---

### T-07 `[FEAT]` Admin JWT authentication — login, middleware, refresh token

**File baru/ubah:**
- `internal/infrastructure/config/config.go` ← tambah `JWTConfig`
- `internal/delivery/http/middleware/admin_auth.go` ← buat baru
- `internal/domain/entity/admin_token.go` ← buat baru (AdminClaims)
- `.config.example.toml` ← tambah section `[jwt]`

**Config:**
```toml
[jwt]
secret              = "change-me-min-32-chars"
access_ttl_minutes  = 60
refresh_ttl_days    = 7
```

**JWT Claims:**
```go
type AdminClaims struct {
    AdminID string          `json:"admin_id"`
    Role    entity.AdminRole `json:"role"`
    jwt.RegisteredClaims
}
```

**Middleware `AdminJWTAuth`:**
- Baca `Authorization: Bearer <token>`
- Verify signature dan expiry
- Set `admin_id` dan `admin_role` ke Echo context
- Return 401 jika token tidak valid

**Role guard helper:**
```go
func RequireRole(roles ...entity.AdminRole) echo.MiddlewareFunc
```

---

### T-08 `[FEAT]` Admin usecase — interface dan implementasi

**File baru:**
- `internal/usecase/admin.go` ← interface
- `internal/usecase/impl/admin.go` ← implementasi

**Interface:**
```go
type AdminLoginInput struct {
    Email    string `json:"email"    validate:"required,email"`
    Password string `json:"password" validate:"required,min=8"`
}

type AdminTokenOutput struct {
    AccessToken  string    `json:"access_token"`
    RefreshToken string    `json:"refresh_token"`
    ExpiresAt    time.Time `json:"expires_at"`
}

type AdminUsecase interface {
    Login(ctx context.Context, input AdminLoginInput) (*AdminTokenOutput, error)
    RefreshToken(ctx context.Context, refreshToken string) (*AdminTokenOutput, error)

    // Merchant management
    CreateMerchant(ctx context.Context, input CreateMerchantInput) (*CreateMerchantOutput, error)
    ListMerchants(ctx context.Context, filter AdminListMerchantsFilter) (*MerchantListOutput, error)
    GetMerchant(ctx context.Context, merchantID string) (*MerchantOutput, error)
    ApproveMerchant(ctx context.Context, merchantID string) error
    SuspendMerchant(ctx context.Context, merchantID, reason string) error
    DeactivateMerchant(ctx context.Context, merchantID string) error
    UpdateMerchantFee(ctx context.Context, merchantID string, fee entity.FeeConfig) error
    UpdateDailyCashoutLimit(ctx context.Context, merchantID string, limitIDR int64) error
    RegenerateMerchantAPIKey(ctx context.Context, merchantID string) (rawKey string, err error)

    // Bank account verification
    VerifyBankAccount(ctx context.Context, merchantID, accountID string) error

    // Visibility
    ListAllPayments(ctx context.Context, filter AdminPaymentFilter) (*PaymentListOutput, error)
    ListAllDisbursements(ctx context.Context, filter AdminDisbursementFilter) (*DisbursementListOutput, error)
    ListAllMutations(ctx context.Context, filter AdminMutationFilter) (*MutationListOutput, error)
    GetProviderBalances(ctx context.Context) ([]*entity.ProviderBalance, error)
    UpdateProviderBalance(ctx context.Context, provider entity.Provider, balanceIDR int64, note string) error

    // Admin management (super_admin only)
    CreateAdmin(ctx context.Context, email, password string, role entity.AdminRole) error
    ListAdmins(ctx context.Context, page, limit int) ([]*entity.Admin, int64, error)
}
```

**Catatan implementasi:**
- `Login`: bcrypt compare, generate access+refresh JWT, update `last_login_at`
- `ApproveMerchant`: set `status = active`, tidak bisa approve jika `status = inactive`
- `UpdateMerchantFee`: langsung update `fee_config` di merchants tabel
- `CreateMerchant`: pindahkan logic dari `MerchantUsecase.Create` — admin yang generate API key dan status awal bisa langsung `active` atau `pending` tergantung parameter

---

### T-09 `[FEAT]` Admin HTTP handler dan routing

**File baru:**
- `internal/delivery/http/handler/admin_handler.go`
- `internal/delivery/http/handler/admin_merchant_handler.go`
- `internal/delivery/http/handler/admin_report_handler.go`

**File ubah:** `internal/delivery/http/router.go`, `internal/app/app.go`

**Endpoint lengkap:**
```
# Auth
POST   /admin/login
POST   /admin/token/refresh

# Admin self
GET    /admin/me
PATCH  /admin/me/password

# Merchant management [ops, super_admin]
POST   /admin/merchants
GET    /admin/merchants                       ?status=&page=&limit=&search=
GET    /admin/merchants/:id
PATCH  /admin/merchants/:id/approve
PATCH  /admin/merchants/:id/suspend
PATCH  /admin/merchants/:id/deactivate
PATCH  /admin/merchants/:id/fee              body: {va, qris, disbursement}
PATCH  /admin/merchants/:id/cashout-limit    body: {limit_idr}
POST   /admin/merchants/:id/api-key/regenerate
DELETE /admin/merchants/:id                  soft delete

# Bank accounts [ops]
GET    /admin/merchants/:id/bank-accounts
PATCH  /admin/merchants/:id/bank-accounts/:accountId/verify

# Reporting [finance, ops, super_admin]
GET    /admin/payments                        ?merchant_id=&status=&provider=&start=&end=&page=&limit=
GET    /admin/payments/:id
GET    /admin/disbursements                   ?merchant_id=&status=&provider=&start=&end=&page=&limit=
GET    /admin/disbursements/:id
GET    /admin/mutations                       ?merchant_id=&type=&start=&end=&page=&limit=
GET    /admin/provider-balances
PATCH  /admin/provider-balances/:provider     body: {balance_idr, note}

# Admin user management [super_admin only]
POST   /admin/admins
GET    /admin/admins
PATCH  /admin/admins/:id/deactivate
```

**Middleware stack untuk admin routes:**
```go
adminGroup := e.Group("/admin")
adminGroup.POST("/login", adminHandler.Login)
adminGroup.POST("/token/refresh", adminHandler.RefreshToken)

authedAdmin := adminGroup.Group("", AdminJWTAuth(jwtCfg))
// ... route lainnya
```

---

### T-10 `[FEAT]` Admin seed / bootstrap — cara membuat admin pertama

**File baru/ubah:**
- `cmd/api/main.go` atau `cmd/api/seed.go` ← tambah subcommand `seed-admin`
- Atau: environment variable `ADMIN_BOOTSTRAP_EMAIL` + `ADMIN_BOOTSTRAP_PASSWORD` saat startup jika tabel admin kosong

**Alasan:** Tanpa ini, setelah T-06 (blokir public merchant registration) dan T-09 (admin routes), tidak ada cara untuk masuk ke sistem sama sekali.

**Rekomendasi:** Subcommand CLI:
```bash
./wanpey seed-admin --email admin@wanpey.id --password "changeme" --role super_admin
```
Hanya berhasil jika tidak ada admin aktif di DB (one-time bootstrap, gagal jika sudah ada).

---

### T-11 `[FEAT]` Tambah endpoint listing payments dan disbursements untuk merchant

**File ubah:** `internal/usecase/payment.go`, `internal/usecase/impl/payment.go`, `internal/delivery/http/handler/payment_handler.go`, `internal/delivery/http/router.go`

**Masalah:** Repository `PaymentRepository` sudah punya method `List()` dengan `ListPaymentFilter`, tapi usecase dan handler tidak mengeksposnya. Merchant tidak bisa melihat daftar payment mereka — hanya bisa get by ID.

**Fix:**
- Tambah `ListPayments(ctx context.Context, input ListPaymentsInput) (*PaymentListOutput, error)` ke `PaymentUsecase`
- Tambah `ListDisbursements` ke `DisbursementUsecase`
- `GET /v1/payments?status=&provider=&method=&start_date=&end_date=&page=&limit=`
- `GET /v1/disbursements?status=&start_date=&end_date=&page=&limit=`
- MerchantID selalu diambil dari JWT context (tidak dari query param)

---

## P2 — Penting (Flow tidak lengkap)

### T-12 `[MISS]` Webhook ke merchant tidak di-sign — outbox perlu merchant_id dan webhook_secret

**File ubah:**
- `migrations/000005_outbox_merchant.up.sql` ← tambah kolom `merchant_id UUID REFERENCES merchants(id)`
- `internal/infrastructure/database/postgres/outbox_repo.go` ← update `Insert()` signature
- `internal/infrastructure/database/postgres/query/outbox.sql` ← update query sqlc
- `internal/usecase/impl/payment.go` ← pass `merchantID` saat `outboxRepo.Insert`
- `internal/usecase/impl/disbursement.go` ← sama
- `internal/infrastructure/worker/outbox_worker.go` ← saat delivery, query merchant webhook_secret, sign payload

**Masalah:** Outbox worker mengirim webhook ke merchant tanpa signature. Merchant tidak punya cara untuk memverifikasi bahwa payload berasal dari Wanpey dan bukan dari pihak ketiga.

**Fix — update outbox table:**
```sql
ALTER TABLE outbox ADD COLUMN merchant_id UUID REFERENCES merchants(id);
```

**Fix — update outbox_worker `post()`:**
```go
// Lookup merchant webhook_secret (hashed) dari DB
// Gunakan pkg/signature.Sign(rawSecret, payloadBytes) — TAPI secret disimpan hashed
// Perlu store raw secret di outbox atau kolom terpisah di merchants untuk signing
```

**Penting — chicken-and-egg problem:**
`webhook_secret` disimpan sebagai SHA256 hash di DB. Untuk sign payload butuh nilai raw-nya. Solusi:
1. Tambah kolom `webhook_secret_raw` terenkripsi AES di tabel merchants (recommended untuk production)
2. Atau: simpan `webhook_secret` di outbox row saat insert (denormalized, tapi sederhana)
3. Atau: store raw secret di Redis dengan TTL panjang

**Rekomendasi untuk sekarang:** Opsi 2 — tambah kolom `webhook_secret TEXT` ke tabel `outbox`, diisi saat insert dari usecase (ambil raw secret yang tidak tersimpan saat ini — butuh perubahan flow di merchant creation untuk return dan store raw secret).

**Alternatif lebih bersih:** Saat merchant dibuat, simpan `webhook_secret` di dua kolom: `webhook_secret` (hash, untuk auth) dan tambah `webhook_signing_key` (encrypted AES) khusus untuk signing outbound webhook.

---

### T-13 `[FEAT]` Standardisasi format payload callback ke merchant

**File baru:** `pkg/webhook/payload.go`

**File ubah:** `internal/usecase/impl/payment.go`, `internal/usecase/impl/disbursement.go`

**Masalah:** Payload yang dikirim ke merchant saat ini adalah bare map:
```go
map[string]any{"event": "payment.paid", "payment_id": p.ID, ...}
```
Tidak ada format standar, tidak ada versi, tidak ada semua detail payment.

**Format baru:**
```go
type WebhookPayload struct {
    EventType   string    `json:"event_type"`   // "payment.paid", "disbursement.completed"
    DeliveryID  string    `json:"delivery_id"`  // outbox row ID, untuk idempotency di sisi merchant
    CreatedAt   time.Time `json:"created_at"`
    Data        any       `json:"data"`
}

type PaymentWebhookData struct {
    PaymentID     string               `json:"payment_id"`
    ExternalID    string               `json:"external_id"`
    MerchantID    string               `json:"merchant_id"`
    Status        entity.PaymentStatus `json:"status"`
    Method        entity.PaymentMethod `json:"method"`
    Provider      entity.Provider      `json:"provider"`
    Amount        int64                `json:"amount"`
    FeeAmount     int64                `json:"fee_amount"`
    NetAmount     int64                `json:"net_amount"`   // amount - fee_amount
    Currency      entity.Currency      `json:"currency"`
    CustomerName  string               `json:"customer_name"`
    CustomerEmail string               `json:"customer_email"`
    PaidAt        *time.Time           `json:"paid_at,omitempty"`
    CreatedAt     time.Time            `json:"created_at"`
}
```

**Headers yang dikirim:**
```
Content-Type: application/json
X-Wanpey-Signature: sha256=<hmac>
X-Wanpey-Event: payment.paid
X-Wanpey-Delivery-ID: <outbox_id>
X-Wanpey-Timestamp: <unix_ts>
```

---

### T-14 `[FEAT]` Per-payment callback URL

**File ubah:**
- `internal/usecase/payment.go` (`CreateVAInput`, `CreateQRISInput`)
- `internal/usecase/impl/payment.go` (`CreateVA`, `CreateQRIS`, `HandleWebhook`)

**Use case:** Merchant ingin menentukan callback URL yang berbeda per transaksi (misal: untuk order tertentu, atau untuk environment testing).

**Fix:**
- Tambah `CallbackURL string \`json:"callback_url" validate:"omitempty,url,max=500"\`` ke `CreateVAInput` dan `CreateQRISInput`
- Simpan ke `Payment.Metadata["callback_url"]` (kolom `metadata JSONB` sudah ada)
- Di `HandleWebhook`, resolusi URL: `callbackURL || merchant.WebhookURL`
- Jika keduanya kosong, jangan insert ke outbox (tidak ada kemana dikirim)

---

### T-15 `[FEAT]` Enforce bank account verification sebelum disbursement

**File ubah:** `internal/usecase/impl/disbursement.go` (`Disburse()`)

**Masalah:** Field `IsVerified` ada di `entity.MerchantBankAccount` dan di schema dengan `CONSTRAINT`, tapi `Disburse()` tidak pernah mengecek nilai ini.

**Fix:** Ketika merchant tidak menyebutkan `account_id` secara eksplisit (menggunakan primary bank account), cek `account.IsVerified`:
```go
// Jika account bukan verified, return error
if !account.IsVerified {
    return nil, apperror.UnprocessableEntity(
        "bank account %s is not verified — contact admin to verify", accountID)
}
```

**Catatan:** Saat ini `DisburseInput` tidak punya `AccountID` — merchant langsung memasukkan `bank_code` + `account_number`. Perlu diputuskan: apakah disbursement harus ke akun terdaftar saja (lebih aman) atau bisa ke akun sembarang. **Rekomendasi: harus ke akun terdaftar yang terverifikasi**.

---

### T-16 `[FEAT]` Platform margin belum diterapkan ke fee calculation

**File ubah:**
- `internal/usecase/impl/helpers.go` (`computeMethodFee`)
- `internal/usecase/impl/payment.go` (`NewPaymentUsecase`, `HandleWebhook`)
- `internal/usecase/impl/disbursement.go` (`NewDisbursementUsecase`, `Disburse`)

**Masalah:** `config.FeeConfig.Margin` ada di config tapi tidak pernah dipakai. `computeMethodFee` hanya terima `entity.MethodFee` (merchant fee) — platform margin tidak pernah dihitung. Artinya Wanpey tidak mengambil keuntungan sama sekali dari setiap transaksi.

**Fix:**
```go
// computeEffectiveFee menghitung total fee: merchant fee + platform margin
func computeEffectiveFee(merchantFee entity.MethodFee, platformMargin config.MethodMargin, amount int64, enabled bool) int64 {
    base := computeMethodFee(merchantFee, amount)
    if !enabled {
        return base
    }
    var margin int64
    switch platformMargin.Type {
    case "flat":
        margin = platformMargin.FlatIDR
    case "percentage":
        margin = int64(float64(amount) * platformMargin.Percentage / 100)
    }
    total := base + margin
    if total > amount {
        return amount // hard cap
    }
    return total
}
```

- Inject `*config.Config` ke `paymentUsecase` dan `disbursementUsecase`
- Panggil `computeEffectiveFee` di `HandleWebhook` (saat insert Mutation) dan `Disburse`

---

### T-17 `[FEAT]` Background job — expire pending payments otomatis

**File baru:** `internal/infrastructure/worker/expiry_worker.go`

**Masalah:** Payment yang sudah melewati `expiry_at` tetap di status `pending` selamanya kecuali provider mengirim webhook. Banyak provider tidak mengirim webhook expired — hanya `paid`. Ini menyebabkan balance salah kalkulasi dan status payment tidak akurat.

**Fix:**
```go
// ExpiryWorker polls setiap menit, update payments yang expiry_at < NOW() AND status = 'pending'
// Juga insert payment_audit dengan event_type = 'PAYMENT_EXPIRED'
```

**Query:**
```sql
UPDATE payments
SET status = 'expired', updated_at = NOW()
WHERE status = 'pending' AND expiry_at < NOW()
RETURNING id, merchant_id
```

- Jalankan di goroutine yang sama lifecycle dengan `OutboxWorker` (register di `app.Run()`)
- Insert audit event `PAYMENT_EXPIRED` per payment yang di-expire
- Insert ke outbox untuk notify merchant

---

### T-18 `[MISS]` Outbox worker menggunakan raw SQL langsung, bukan lewat OutboxRepo — inconsistency

**File ubah:** `internal/infrastructure/worker/outbox_worker.go`

**Masalah:** `OutboxRepo` (sqlc-generated) dipakai untuk `Insert`, tapi `OutboxWorker` menggunakan raw SQL `w.db.QueryContext` dan `w.db.ExecContext` secara langsung. Dua path berbeda untuk tabel yang sama — jika kolom tabel berubah, keduanya harus diupdate secara manual.

**Fix:**
- Refactor `OutboxWorker` untuk inject `OutboxRepo` (atau interface baru `OutboxFetcher`)
- Atau: buat method `FetchAndClaim` dan `UpdateStatus` di `OutboxRepo` yang dipakai worker

---

### T-19 `[FEAT]` Endpoint riwayat webhook delivery untuk merchant (debugging)

**File ubah:** `internal/usecase/mutation.go` (atau usecase baru), router, handler

**Masalah:** Merchant tidak bisa tahu apakah webhook mereka berhasil diterima. Saat ini tidak ada cara untuk debug delivery failure selain tanya ke admin.

**Endpoint baru:** `GET /v1/webhook-events`
```go
type WebhookEventOutput struct {
    ID          string     `json:"id"`
    EventType   string     `json:"event_type"`
    TargetURL   string     `json:"target_url"`
    AttemptCount int       `json:"attempt_count"`
    MaxAttempts  int       `json:"max_attempts"`
    LastError   string     `json:"last_error,omitempty"`
    DeliveredAt *time.Time `json:"delivered_at,omitempty"`
    FailedAt    *time.Time `json:"failed_at,omitempty"`
    CreatedAt   time.Time  `json:"created_at"`
}
```

Query outbox by `merchant_id` (setelah T-12 menambah kolom merchant_id).

---

## EPIC: Fee Management System (P1–P2)

Saat ini fee hanya tersimpan di `merchant.fee_config` (JSONB) dan platform margin di `.config.toml`. Keduanya tidak bisa diubah dari UI/API tanpa restart server atau akses DB langsung. Fee holiday tidak ada sama sekali. Bagian ini merancang sistem fee yang bisa dikontrol admin dari API.

---

### Arsitektur Fee — Hierarki Priority

```
Effective Fee = Base Fee + Platform Margin + Holiday Surcharge

Resolution order (dari highest priority ke lowest):

┌─────────────────────────────────────────────────────────────┐
│  LAYER 3 — Holiday Surcharge (highest, opsional)           │
│  Tambahan biaya khusus hari tertentu                        │
│  Tidak override merchant fee, hanya TAMBAH di atasnya       │
├─────────────────────────────────────────────────────────────┤
│  LAYER 2 — Platform Margin (selalu diterapkan)              │
│  Revenue Wanpey, dikontrol super_admin                       │
│  Dari DB (bukan config file), bisa diubah real-time         │
├─────────────────────────────────────────────────────────────┤
│  LAYER 1 — Base Fee (salah satu dari dua berikut):          │
│    A. Merchant Contracted Fee (jika merchant punya kontrak) │
│    B. Global Default Fee (fallback jika A tidak ada)        │
└─────────────────────────────────────────────────────────────┘
```

**Kenapa holiday tidak override merchant fee?**  
Merchant punya SLA kontrak dengan rate tertentu — Wanpey tidak bisa ubah itu secara sepihak saat hari raya. Yang bisa ditambah adalah `holiday_surcharge` = Wanpey charge ekstra ke merchant karena hari libur (volume tinggi, biaya operasional tinggi). Merchant tahu ini dari kontrak mereka.

**Ilustrasi:**
```
Transaksi VA BCA, amount Rp 1.000.000, terjadi di Hari Raya Lebaran

Merchant A (punya kontrak): VA flat Rp 4.500
Merchant B (tidak ada kontrak): Global default VA flat Rp 5.000

Platform margin: VA flat Rp 500
Holiday surcharge: VA flat Rp 1.000 (Lebaran)

Effective fee Merchant A: 4.500 + 500 + 1.000 = Rp 6.000
Effective fee Merchant B: 5.000 + 500 + 1.000 = Rp 6.500
```

---

### T-FEE-01 `[FEAT]` Global default fee — entity, migration, admin CRUD

**Priority: P1** — Wajib ada. Tanpa ini, merchant yang tidak punya kontrak custom tidak dikenakan biaya apapun (fee 0).

**File baru:**
- `internal/domain/entity/fee.go` — `FeeDefault` entity
- `internal/domain/repository/fee.go` — `FeeRepository` interface
- `internal/infrastructure/database/postgres/fee_repo.go`
- `internal/infrastructure/database/postgres/query/fee.sql`
- `migrations/000006_fee_defaults.up.sql`

**Entity:**
```go
// FeeDefault adalah tarif dasar yang digunakan sebagai fallback
// jika merchant tidak punya merchant.fee_config custom.
// Hanya ada 1 row aktif per method — admin update nilainya, tidak insert baru.
type FeeDefault struct {
    ID         string
    VA         MethodFee // flat IDR per transaksi VA
    QRIS       MethodFee // percentage dari amount QRIS
    Disbursement MethodFee // flat IDR per disbursement
    UpdatedBy  string    // admin_id yang terakhir update
    UpdatedAt  time.Time
    CreatedAt  time.Time
}
```

**Migration:**
```sql
CREATE TABLE fee_defaults (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    fee_config   JSONB       NOT NULL DEFAULT '{}',
    updated_by   UUID        NOT NULL REFERENCES admins(id),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
-- Seed 1 row default saat migration
INSERT INTO fee_defaults (fee_config, updated_by)
SELECT '{"va":{"type":"flat","amount":4500},"qris":{"type":"percentage","percentage":0.7},"disbursement":{"type":"flat","amount":5000}}',
       id FROM admins LIMIT 1;
```

**Admin endpoints:**
```
GET    /admin/fee/defaults          ← lihat current global default fee
PATCH  /admin/fee/defaults          ← update (finance, super_admin only)
```

**Who can access:**
- GET: `super_admin`, `finance`
- PATCH: `super_admin`, `finance`

---

### T-FEE-02 `[FEAT]` Platform margin — pindah dari config file ke database

**Priority: P1** — Wajib. Margin di `.config.toml` tidak bisa diubah tanpa restart server.

**File baru/ubah:**
- `internal/domain/entity/fee.go` — tambah `PlatformMargin` entity
- `migrations/000007_platform_margin.up.sql`
- `internal/usecase/impl/payment.go` — baca margin dari DB, bukan config
- `internal/usecase/impl/disbursement.go` — sama

**Entity:**
```go
// PlatformMargin adalah Wanpey's revenue layer, selalu diterapkan
// di atas merchant fee. Hanya super_admin yang bisa mengubah ini.
type PlatformMargin struct {
    ID           string
    Enabled      bool
    VA           MethodFee
    QRIS         MethodFee
    Disbursement MethodFee
    UpdatedBy    string    // admin_id
    UpdatedAt    time.Time
    CreatedAt    time.Time
}
```

**Migration:**
```sql
CREATE TABLE platform_margin (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    enabled      BOOLEAN     NOT NULL DEFAULT true,
    margin_config JSONB      NOT NULL DEFAULT '{}',
    updated_by   UUID        NOT NULL REFERENCES admins(id),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

**Admin endpoints:**
```
GET    /admin/fee/margin           ← lihat current margin (finance, super_admin)
PATCH  /admin/fee/margin           ← update margin (super_admin ONLY)
```

**Who can access:**
- GET: `super_admin`, `finance`
- PATCH: `super_admin` **ONLY** — ini revenue utama Wanpey, harus dibatasi ketat

**Cache:** Platform margin di-cache di Redis dengan TTL 5 menit agar tidak query DB setiap transaksi. Cache invalidated saat admin update.

---

### T-FEE-03 `[FEAT]` Holiday & special day surcharge — entity, migration, admin CRUD

**Priority: P2** — Penting untuk monetisasi hari raya, tapi tidak memblokir operasi dasar.

**File baru:**
- `internal/domain/entity/fee.go` — tambah `FeeHoliday` entity
- `internal/domain/repository/fee.go` — tambah `FeeHolidayRepository`
- `internal/infrastructure/database/postgres/fee_repo.go` — tambah holiday methods
- `migrations/000008_fee_holidays.up.sql`

**Entity:**
```go
type HolidayType string
const (
    HolidayTypeNational HolidayType = "national" // hari libur nasional (Lebaran, Natal, dll)
    HolidayTypeCustom   HolidayType = "custom"   // tanggal khusus yang ditentukan admin
)

// FeeHoliday mendefinisikan surcharge tambahan untuk tanggal tertentu.
// Surcharge DITAMBAHKAN di atas fee normal — tidak menggantikannya.
// Jika ada beberapa holiday yang overlap (tidak seharusnya, karena UNIQUE date),
// hanya 1 yang berlaku (highest priority = national > custom).
type FeeHoliday struct {
    ID          string
    Name        string      // "Idul Fitri 1446H", "Long Weekend Natal"
    Date        time.Time   // DATE (tanpa jam)
    Type        HolidayType
    Surcharge   MethodFee   // tambahan fee untuk VA/QRIS/Disbursement
    AppliesTo   string      // "all" | merchant_id (jika nil = semua merchant)
    IsActive    bool
    CreatedBy   string      // admin_id
    UpdatedBy   string
    CreatedAt   time.Time
    UpdatedAt   time.Time
}
```

**Migration:**
```sql
CREATE TABLE fee_holidays (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name         TEXT        NOT NULL,
    date         DATE        NOT NULL,
    type         TEXT        NOT NULL DEFAULT 'custom',
    surcharge    JSONB       NOT NULL DEFAULT '{}',  -- per method surcharge
    applies_to   TEXT        NOT NULL DEFAULT 'all', -- 'all' or merchant_id UUID
    is_active    BOOLEAN     NOT NULL DEFAULT true,
    created_by   UUID        NOT NULL REFERENCES admins(id),
    updated_by   UUID        NOT NULL REFERENCES admins(id),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_holiday_type   CHECK (type IN ('national', 'custom')),
    CONSTRAINT uq_holiday_date_merchant UNIQUE (date, applies_to)
);

CREATE INDEX idx_fee_holidays_date ON fee_holidays (date) WHERE is_active = true;
```

**Admin endpoints:**
```
GET    /admin/fee/holidays                  ← list semua, ?year=2025&active=true
POST   /admin/fee/holidays                  ← create (finance, super_admin)
GET    /admin/fee/holidays/:id
PATCH  /admin/fee/holidays/:id              ← update (finance, super_admin)
DELETE /admin/fee/holidays/:id              ← soft delete via is_active=false (super_admin ONLY)

# Utility endpoint untuk preview
GET    /admin/fee/preview?date=2025-03-31&merchant_id=xxx&method=va&amount=100000
```

**Who can access:**
- GET, POST, PATCH: `super_admin`, `finance`
- DELETE: `super_admin` **ONLY**

**Endpoint `/admin/fee/preview`** — sangat berguna untuk admin cek sebelum hari raya: "kalau besok ada Lebaran, merchant ini kena berapa?"

---

### T-FEE-04 `[FEAT]` Fee resolution engine — satu tempat untuk resolusi fee dari semua layer

**Priority: P1** — Tanpa ini, semua layer fee tidak bisa dikomposisi dengan benar.

**File baru:** `internal/usecase/impl/fee_resolver.go`

**Masalah saat ini:** `computeMethodFee` di `helpers.go` hanya terima satu `entity.MethodFee`. Tidak ada logika untuk memilih antara merchant fee vs global default, dan tidak ada holiday surcharge.

**Implementasi:**
```go
type FeeResolver struct {
    feeRepo     repository.FeeRepository  // untuk global default + platform margin
    holidayRepo repository.FeeHolidayRepository
    cache       cache.Cache               // cache platform margin
}

type FeeResolution struct {
    BaseFee          int64 // dari merchant contract atau global default
    PlatformMargin   int64 // Wanpey's margin
    HolidaySurcharge int64 // 0 jika bukan hari libur
    TotalFee         int64 // BaseFee + PlatformMargin + HolidaySurcharge
    Source           string // "merchant_contract" | "global_default"
    HolidayName      string // nama hari libur jika berlaku, else ""
}

func (r *FeeResolver) Resolve(
    ctx context.Context,
    merchant *entity.Merchant,
    method entity.PaymentMethod,
    amount int64,
    date time.Time,
) (*FeeResolution, error) {
    // 1. Base fee — merchant contract atau global default?
    baseFeeConfig := merchant.FeeConfig.ForMethod(method)
    source := "merchant_contract"
    if baseFeeConfig.IsZero() {
        globalDefault, err := r.feeRepo.GetDefault(ctx)
        ...
        baseFeeConfig = globalDefault.ForMethod(method)
        source = "global_default"
    }
    base := computeMethodFee(baseFeeConfig, amount)

    // 2. Platform margin (cached)
    margin, err := r.getPlatformMargin(ctx, method, amount)

    // 3. Holiday surcharge
    holiday, err := r.holidayRepo.FindByDate(ctx, date.Format("2006-01-02"), merchant.ID)
    surcharge := int64(0)
    holidayName := ""
    if holiday != nil && holiday.IsActive {
        surcharge = computeMethodFee(holiday.Surcharge.ForMethod(method), amount)
        holidayName = holiday.Name
    }

    total := base + margin + surcharge
    if total > amount { total = amount } // hard cap

    return &FeeResolution{
        BaseFee: base, PlatformMargin: margin,
        HolidaySurcharge: surcharge, TotalFee: total,
        Source: source, HolidayName: holidayName,
    }, nil
}
```

**Inject `FeeResolver` ke `paymentUsecase` dan `disbursementUsecase`** — gantikan `computeMethodFee` langsung.

---

### T-FEE-05 `[FEAT]` Fee change audit log — siapa mengubah fee apa kapan

**Priority: P2**

**File baru:**
- `internal/domain/entity/fee.go` — tambah `FeeAuditLog`
- `migrations/000009_fee_audit_log.up.sql`

**Masalah:** Tanpa audit trail, tidak ada cara untuk tahu siapa yang mengubah fee dan kapan. Ini penting untuk compliance dan dispute resolution.

**Entity:**
```go
type FeeAuditLog struct {
    ID         string
    EntityType string    // "global_default" | "merchant_fee" | "platform_margin" | "holiday"
    EntityID   string    // merchant_id, holiday_id, atau "singleton"
    AdminID    string    // who made the change
    AdminEmail string    // denormalized untuk readability
    OldValue   map[string]any
    NewValue   map[string]any
    Reason     string    // required: admin harus tulis alasan perubahan fee
    CreatedAt  time.Time
}
```

**Migration:**
```sql
CREATE TABLE fee_audit_logs (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    entity_type  TEXT        NOT NULL,
    entity_id    TEXT        NOT NULL,
    admin_id     UUID        NOT NULL REFERENCES admins(id),
    admin_email  TEXT        NOT NULL,
    old_value    JSONB,
    new_value    JSONB       NOT NULL,
    reason       TEXT        NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_entity_type CHECK (entity_type IN (
        'global_default', 'merchant_fee', 'platform_margin', 'holiday_surcharge'
    ))
);

CREATE INDEX idx_fee_audit_entity ON fee_audit_logs (entity_type, entity_id, created_at DESC);
CREATE INDEX idx_fee_audit_admin  ON fee_audit_logs (admin_id, created_at DESC);
```

**Setiap endpoint yang ubah fee HARUS:**
1. Require field `reason` di request body (wajib, min 10 chars)
2. Insert ke `fee_audit_logs` dalam transaksi yang sama

**Admin endpoint:**
```
GET /admin/fee/audit-log?entity_type=merchant_fee&entity_id=xxx&start=&end=
```

---

### T-FEE-06 `[FEAT]` Update `FeeConfig` merchant — harus lewat admin + alasan wajib

**Priority: P1** — Linked ke T-02 (hapus self-service fee). Tanpa ini, tidak ada cara untuk ubah fee merchant.

**File ubah:** `internal/usecase/admin.go`, `internal/usecase/impl/admin.go`

**Endpoint:** `PATCH /admin/merchants/:id/fee`

**Request body yang benar:**
```json
{
    "fee_config": {
        "va": { "type": "flat", "amount": 4500 },
        "qris": { "type": "percentage", "percentage": 0.7 },
        "disbursement": { "type": "flat", "amount": 5000 }
    },
    "reason": "Renegotiated contract Q2 2025 - volume tier upgrade"
}
```

**Validasi:**
- `reason` wajib, minimum 10 karakter
- Fee tidak boleh melebihi global default + 50% (sanity check)
- Fee tidak boleh negatif
- Insert ke `fee_audit_logs` setelah update

**Who can access:** `super_admin`, `finance`

---

## Audit Role Matrix — Siapa Boleh Apa

Ini adalah **role matrix resmi** yang harus diimplementasikan di middleware `RequireRole`. Setiap endpoint admin harus mencantumkan role mana yang diizinkan.

### Admin Role Definitions

```go
type AdminRole string
const (
    AdminRoleSuperAdmin AdminRole = "super_admin" // full access + destroy operations
    AdminRoleFinance    AdminRole = "finance"     // fee management + financial visibility
    AdminRoleOps        AdminRole = "ops"         // merchant lifecycle + operational
)
```

**Prinsip:**
- `super_admin` = semua yang `finance` dan `ops` bisa + operasi destruktif (delete, override)
- `finance` = semua hal yang berkaitan dengan uang dan fee, tapi tidak bisa manage merchant status
- `ops` = semua hal operasional (approve merchant, dll) tapi tidak bisa lihat/ubah fee detail

### Permission Matrix Lengkap

| Endpoint | Method | super_admin | finance | ops |
|---|---|---|---|---|
| **Auth** | | | | |
| `/admin/login` | POST | ✅ | ✅ | ✅ |
| `/admin/token/refresh` | POST | ✅ | ✅ | ✅ |
| `/admin/me` | GET | ✅ | ✅ | ✅ |
| `/admin/me/password` | PATCH | ✅ | ✅ | ✅ |
| **Admin Management** | | | | |
| `/admin/admins` | GET | ✅ | ❌ | ❌ |
| `/admin/admins` | POST | ✅ | ❌ | ❌ |
| `/admin/admins/:id/deactivate` | PATCH | ✅ | ❌ | ❌ |
| **Merchant Management** | | | | |
| `/admin/merchants` | GET | ✅ | ✅ | ✅ |
| `/admin/merchants` | POST | ✅ | ❌ | ✅ |
| `/admin/merchants/:id` | GET | ✅ | ✅ | ✅ |
| `/admin/merchants/:id/approve` | PATCH | ✅ | ❌ | ✅ |
| `/admin/merchants/:id/suspend` | PATCH | ✅ | ❌ | ✅ |
| `/admin/merchants/:id/deactivate` | PATCH | ✅ | ❌ | ✅ |
| `/admin/merchants/:id/api-key/regenerate` | POST | ✅ | ❌ | ✅ |
| `/admin/merchants/:id` | DELETE (soft) | ✅ | ❌ | ❌ |
| **Fee Management** | | | | |
| `/admin/merchants/:id/fee` | GET | ✅ | ✅ | ✅ |
| `/admin/merchants/:id/fee` | PATCH | ✅ | ✅ | ❌ |
| `/admin/merchants/:id/cashout-limit` | PATCH | ✅ | ✅ | ❌ |
| `/admin/fee/defaults` | GET | ✅ | ✅ | ✅ |
| `/admin/fee/defaults` | PATCH | ✅ | ✅ | ❌ |
| `/admin/fee/margin` | GET | ✅ | ✅ | ❌ |
| `/admin/fee/margin` | PATCH | ✅ | ❌ | ❌ |
| `/admin/fee/holidays` | GET | ✅ | ✅ | ✅ |
| `/admin/fee/holidays` | POST | ✅ | ✅ | ❌ |
| `/admin/fee/holidays/:id` | PATCH | ✅ | ✅ | ❌ |
| `/admin/fee/holidays/:id` | DELETE | ✅ | ❌ | ❌ |
| `/admin/fee/preview` | GET | ✅ | ✅ | ❌ |
| `/admin/fee/audit-log` | GET | ✅ | ✅ | ❌ |
| **Bank Accounts** | | | | |
| `/admin/merchants/:id/bank-accounts` | GET | ✅ | ✅ | ✅ |
| `/admin/merchants/:id/bank-accounts/:aid/verify` | PATCH | ✅ | ❌ | ✅ |
| **Financial Reporting** | | | | |
| `/admin/payments` | GET | ✅ | ✅ | ✅ |
| `/admin/payments/:id` | GET | ✅ | ✅ | ✅ |
| `/admin/disbursements` | GET | ✅ | ✅ | ✅ |
| `/admin/disbursements/:id` | GET | ✅ | ✅ | ✅ |
| `/admin/mutations` | GET | ✅ | ✅ | ❌ |
| `/admin/provider-balances` | GET | ✅ | ✅ | ❌ |
| `/admin/provider-balances/:provider` | PATCH | ✅ | ✅ | ❌ |

**Catatan penting:**
- `ops` tidak bisa lihat mutations dan provider balances — informasi finansial sensitif
- `finance` tidak bisa approve/suspend merchant — pemisahan duties (separation of concerns)
- `finance` tidak bisa update platform margin — hanya super_admin
- Semua fee change HARUS include `reason` field yang dicatat di `fee_audit_logs`

### T-FEE-07 `[FEAT]` Implementasi `RequireRole` middleware per endpoint

**Priority: P1** — Tanpa ini, semua admin endpoint bisa diakses oleh semua role.

**File ubah:** `internal/delivery/http/middleware/admin_auth.go`, `internal/delivery/http/router.go`

**Implementasi:**
```go
// RequireRole returns middleware that checks the admin's role against the allowed list.
// Must be used AFTER AdminJWTAuth (which sets admin_role in context).
func RequireRole(allowed ...entity.AdminRole) echo.MiddlewareFunc {
    set := make(map[entity.AdminRole]struct{}, len(allowed))
    for _, r := range allowed {
        set[r] = struct{}{}
    }
    return func(next echo.HandlerFunc) echo.HandlerFunc {
        return func(c echo.Context) error {
            role, ok := c.Get("admin_role").(entity.AdminRole)
            if !ok {
                return apperror.Unauthorized("admin role not found in context")
            }
            if _, permitted := set[role]; !permitted {
                return apperror.Forbidden("role %s is not permitted for this action", role)
            }
            return next(c)
        }
    }
}
```

**Usage di router:**
```go
superAdmin := RequireRole(entity.AdminRoleSuperAdmin)
financeOrSuper := RequireRole(entity.AdminRoleSuperAdmin, entity.AdminRoleFinance)
opsOrSuper := RequireRole(entity.AdminRoleSuperAdmin, entity.AdminRoleOps)
allAdmin := RequireRole(entity.AdminRoleSuperAdmin, entity.AdminRoleFinance, entity.AdminRoleOps)

// Fee routes dengan role enforcement
fee := authedAdmin.Group("/fee")
fee.GET("/defaults", adminHandler.GetFeeDefaults, allAdmin)
fee.PATCH("/defaults", adminHandler.UpdateFeeDefaults, financeOrSuper)
fee.GET("/margin", adminHandler.GetPlatformMargin, financeOrSuper)
fee.PATCH("/margin", adminHandler.UpdatePlatformMargin, superAdmin) // ← super_admin ONLY
fee.GET("/holidays", adminHandler.ListHolidays, allAdmin)
fee.POST("/holidays", adminHandler.CreateHoliday, financeOrSuper)
fee.PATCH("/holidays/:id", adminHandler.UpdateHoliday, financeOrSuper)
fee.DELETE("/holidays/:id", adminHandler.DeleteHoliday, superAdmin)
fee.GET("/preview", adminHandler.PreviewFee, financeOrSuper)
fee.GET("/audit-log", adminHandler.ListFeeAuditLog, financeOrSuper)
```

---

### T-FEE-08 `[FEAT]` Migrasi platform margin dari `.config.toml` ke DB

**Priority: P2** — Setelah T-FEE-02 dan T-FEE-04, platform margin yang sekarang ada di config harus di-seed ke DB. `.config.toml` margin menjadi deprecated.

**Migration seeder:**
```go
// Saat boot, jika tabel platform_margin kosong, seed dari config
func seedPlatformMargin(cfg *config.Config, db database.SQLDB, adminID string) error {
    count, _ := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM platform_margin").Scan(&n)
    if n > 0 { return nil } // sudah ada, skip
    // Insert dari config
    ...
}
```

---

## Fee System — Urutan Implementasi

```
Wajib (tanpa ini fee system tidak bekerja):
  T-FEE-01 → T-FEE-02 → T-FEE-04 → T-FEE-06 → T-FEE-07
  (global default, platform margin DB, resolver engine, merchant fee endpoint, role middleware)

Penting tapi bisa menyusul:
  T-FEE-05 → T-FEE-03 → T-FEE-08
  (audit log, holiday surcharge, migrate margin dari config)
```

**Dependency ke task lain:**
- T-FEE-01 sampai T-FEE-08 semua depend pada T-14 (admin entity + migration) yang sudah selesai dulu
- T-FEE-04 (resolver) menggantikan T-16 (computeEffectiveFee) yang ada di task sebelumnya — T-16 **superseded** oleh T-FEE-04
- T-FEE-06 adalah implementasi nyata dari T-02 (hapus self-service fee) — keduanya harus dikerjakan bersama

---

## P3 — Improvement (Resilience, Polish, Ops)

### T-20 `[BUG]` `AddBankAccount` count check race condition — bisa melebihi 3 akun

**File:** `internal/usecase/impl/merchant.go` (`AddBankAccount`)

**Masalah:**
```go
count, err := u.merchantRepo.CountBankAccounts(ctx, input.MerchantID) // ← baca count
if count >= entity.MaxBankAccounts { return error }                    // ← di luar tx
// dua request concurrent bisa lolos di sini dengan count = 2 yang sama
a := &entity.MerchantBankAccount{...}
u.merchantRepo.SaveBankAccount(ctx, a) // ← keduanya berhasil insert → 4 akun
```

**Fix:** Wrap `CountBankAccounts` + `SaveBankAccount` dalam satu `RunInTx` dengan `SELECT COUNT(*) FOR UPDATE` di tabel `merchant_bank_accounts` untuk serialize concurrent requests.

---

### T-21 `[SEC]` Private key DOKU tersimpan sebagai file di root repo

**File:** `doku_private.pem`, `doku_public.pem` (root repository)

**Masalah:** File private key RSA DOKU ada langsung di root repo. Meskipun mungkin di-gitignore, ini berbahaya:
- Mudah ter-commit ke git secara tidak sengaja  
- Tidak ada enkripsi at-rest
- Semua orang dengan akses server bisa membaca file ini

**Fix:**
1. Hapus `doku_private.pem` dan `doku_public.pem` dari root (pastikan tidak ada di git history)
2. Simpan private key di secrets manager (AWS Secrets Manager, HashiCorp Vault, atau minimal environment variable)
3. Config: ubah `doku.private_key` di `.config.toml` untuk membaca dari env var:
   ```toml
   [provider.doku]
   private_key = "${DOKU_PRIVATE_KEY_PEM}"
   ```
4. Tambahkan `doku_private.pem` dan `*.pem` ke `.gitignore`

---

### T-22 `[BUG]` Provider HTTP timeout sama dengan Echo request timeout — chain timeout tidak aman

**File:** `internal/infrastructure/provider/midtrans/midtrans.go`, `xendit/xendit.go`, `doku/doku.go`, `ipaymu/ipaymu.go`, `internal/delivery/http/server.go`

**Masalah:** Semua provider menggunakan `http.Client{Timeout: 30 * time.Second}`. Echo server dikonfigurasi dengan `RequestTimeoutSeconds` yang default-nya juga 30s. Jika provider tepat 30s, Echo context akan cancel lebih dulu → provider tetap jalan di background → response tidak terkirim ke merchant → merchant retry → potensi duplikat payment di provider.

**Fix:**
- Set provider HTTP timeout ke **15 detik** (memberi buffer 15s untuk handler overhead)
- Atau set Echo request timeout ke **45 detik** untuk payment routes
- Tambahkan config per-provider: `[provider.midtrans] timeout_seconds = 15`

---

### T-24 `[RACE]` Cancel payment — provider sukses tapi DB update gagal

**File ubah:** `internal/usecase/impl/payment.go` (`CancelPayment`)

**Masalah saat ini:**
```go
// Step 1: Call provider — berhasil
gw.CancelPayment(ctx, p.ExternalID)
// Step 2: Update DB — GAGAL (timeout, DB down, dll)
database.RunInTx(...) // ← kalau ini gagal, payment tetap 'pending' di DB tapi sudah cancelled di provider
```

**Fix — Idempotent cancel pattern:**
- Sebelum memanggil provider, update DB dulu ke status `cancelled` dalam satu tx
- Jika provider gagal, rollback DB (sudah otomatis karena dalam tx)
- Tapi: beberapa provider (Midtrans) tidak support cancel — harus handle `ErrNotSupported` dengan graceful

**Alternatif lebih safe:**
- Tambah intermediate status `cancelling` di Payment entity
- UPDATE ke `cancelling` dulu → call provider → UPDATE ke `cancelled` atau rollback ke `pending`
- Perlu migration untuk tambah status baru ke CHECK constraint

---

### T-25-b `[MISS]` API key selalu `wpay_test_` — tidak ada production key

**File ubah:** `internal/usecase/impl/helpers.go` (`generateAPIKey`), `internal/usecase/impl/merchant.go`

**Masalah:** `generateAPIKey(false)` selalu dipanggil dengan `false` — semua merchant mendapat `wpay_test_` prefix terlepas dari environment config.

**Fix:**
- Tambah field `IsProduction bool` ke `entity.Merchant` dan tabel (migration)
- Admin bisa set merchant sebagai production saat approval
- `generateAPIKey(m.IsProduction)` → `wpay_live_xxx` untuk production merchant

---

### T-26 `[FEAT]` Treasury `execute_topup` — implementasi atau no-op aman

**File ubah:** `internal/infrastructure/taskqueue/treasury/handler.go`

**Masalah:** Handler `ExecuteTopup` saat ini mengembalikan `error` (atau unimplemented stub). Ini menyebabkan task masuk ke Asynq dead letter queue dan menumpuk.

**Fix jangka pendek:** Return `nil` dengan log warning:
```go
func (h *Handler) HandleExecuteTopup(ctx context.Context, t *asynq.Task) error {
    h.log.Warn("treasury execute_topup not implemented — manual top-up required",
        zap.String("payload", string(t.Payload())))
    return nil // acknowledge task, tidak di-retry
}
```

**Fix jangka panjang:** Implementasi transfer antar bank melalui provider yang mendukung disbursement (topup dari rekening Wanpey ke provider balance).

---

### T-27 `[FEAT]` Webhook provider IP allowlist (defense-in-depth)

**File baru:** `internal/delivery/http/middleware/webhook_allowlist.go`

**Masalah:** Endpoint `/webhooks/:provider/*` menerima request dari IP manapun. Setiap provider memiliki list IP resmi:
- Midtrans: `103.208.23.x`, dll
- Xendit: documented di dashboard mereka

**Fix:**
- Middleware opsional `WebhookIPAllowlist(provider, allowedCIDRs)` berdasarkan config
- Config di `.config.toml`:
```toml
[provider.midtrans]
webhook_allowed_ips = ["103.208.23.0/24"]
```
- Skip jika list kosong (default — tidak memblokir)

---

### T-28 `[FEAT]` Webhook idempotency yang lebih robust di sisi provider callback

**File ubah:** `internal/usecase/impl/payment.go` (`HandleWebhook`)

**Masalah:** `p.IsFinal()` dicek di luar transaction. Jika dua webhook dari provider tiba bersamaan (provider retry sebelum response diterima):
1. Request A: FindByExternalID → not final → masuk RunInTx → Update payment → Insert mutation
2. Request B: FindByExternalID → not final → masuk RunInTx → BLOCKED (row lock dari A) → selesai → insert mutation → GAGAL karena `uq_mutation_reference` constraint

Constraint menyelamatkan dari double mutation, tapi request B akan return error ke provider → provider retry lagi.

**Fix:** Dalam transaction, cek ulang status setelah acquire lock:
```go
database.RunInTx(ctx, u.db, nil, func(ctx context.Context) error {
    // Re-check di dalam tx dengan SELECT FOR UPDATE
    freshPayment, err := paymentRepo.FindByIDForUpdate(ctx, p.ID)
    if freshPayment.IsFinal() {
        return nil // sudah diproses, idempotent
    }
    // ... lanjut update
})
```

---

### T-29 `[MISS]` Health endpoint tidak mencakup asynq dan outbox worker

**File ubah:** `internal/app/app.go` (`healthHandler`)

**Masalah:** `/health` hanya cek database dan cache. Tidak ada info tentang:
- Asynq server status
- Outbox backlog size (apakah ada webhook yang stuck?)
- Provider circuit breaker state

**Fix:**
```go
// Tambah ke healthHandler:
components["asynq"] = "ok" // jika a.asynqSrv != nil dan IsRunning
components["outbox_backlog"] = outboxCount // SELECT COUNT(*) FROM outbox WHERE delivered_at IS NULL AND failed_at IS NULL
```

---

### T-30 `[FEAT]` Disbursement harus ke bank account terdaftar (bukan account sembarang)

**File ubah:** `internal/usecase/disbursement.go` (`DisburseInput`), `internal/usecase/impl/disbursement.go`

**Masalah saat ini:** `DisburseInput` menerima `BankCode`, `AccountNumber`, `AccountName` secara bebas. Merchant bisa disbursement ke rekening manapun, melewati proses verifikasi.

**Fix — account_id based disbursement:**
```go
type DisburseInput struct {
    MerchantID    string          `json:"-"`
    Provider      entity.Provider `json:"provider"     validate:"required,oneof=xendit doku"`
    BankAccountID string          `json:"bank_account_id" validate:"required,uuid"` // harus akun terdaftar
    Amount        int64           `json:"amount"       validate:"required,gt=0"`
    Currency      entity.Currency `json:"currency"     validate:"required,oneof=IDR"`
    Description   string          `json:"description"  validate:"max=255"`
}
```

Usecase mengambil `BankCode`, `AccountNumber`, `AccountName` dari `MerchantBankAccount` yang sudah terverifikasi — bukan dari input merchant.

---

### T-31 `[FEAT]` Request files untuk semua endpoint (untuk testing/dokumentasi)

**File ubah/baru:** `requests/` directory

**Masalah:** Ada folder `requests/` di root tapi belum tentu lengkap. Tambahkan `.http` atau `.json` request files untuk:
- Admin: login, create merchant, approve, fee update
- Merchant: create VA, create QRIS, cancel, disburse
- Webhook: contoh payload dari setiap provider (Midtrans, Xendit, DOKU, iPaymu)

---

### T-32 `[MISS]` Error handling di `idempotency.go` — response dari provider tidak di-masker

**File ubah:** `internal/delivery/http/middleware/idempotency.go`

**Perlu dicek:** Saat idempotency middleware meng-cache response, apakah response yang di-cache termasuk data sensitif (VA number, QR string)? Jika iya, TTL-nya harus sesuai dan cache harus di-scope per merchant (sudah ada dengan format `idempotency:{merchant_id}:{key}`).

---

### T-33 `[FEAT]` Tambah `MerchantID` ke log context untuk semua handler

**File ubah:** `internal/delivery/http/middleware/auth.go`, `internal/delivery/http/server.go` (requestLogger)

**Masalah:** Request log saat ini mencatat `method`, `path`, `status`, `latency`, `ip` — tapi tidak mencatat `merchant_id`. Debugging issue per merchant jadi sulit.

**Fix:**
```go
// Di requestLogger, setelah handler selesai:
if merchantID, ok := c.Get(middleware.ContextKeyMerchantID).(string); ok && merchantID != "" {
    fields = append(fields, zap.String("merchant_id", merchantID))
}
```

---

## Urutan Pengerjaan yang Direkomendasikan

```
Sprint 1 (Critical Bug Fixes — PROD BLOCKER, bisa dikerjakan paralel):
  T-04  DOKU sandbox URL fix (5 menit, 1 baris)
  T-06  Header case mismatch Xendit/DOKU webhook (15 menit, extractHeaders)
  T-07  iPaymu webhook tanpa signature → tambah token verification
  T-05  DOKU HMAC signature (perlu konfirmasi ke DOKU support dulu)
  T-03  ProviderPaymentID — tambah field + migration + update sqlc

Sprint 2 (Security):
  T-01  Hapus public merchant registration endpoint
  T-02  Hapus FeeConfig dari UpdateMerchantInput
  T-08  Disbursement reservation pattern (anti double-spend)

Sprint 3 (Admin System):
  T-14 → T-15 → T-16 → T-17 → T-18
  (entity+migration, JWT, usecase, handler+routes, seed)

Sprint 4 (Audit & Merchant API):
  T-09  PAYMENT_CREATED audit event
  T-19  List payments/disbursements endpoint untuk merchant
  T-20  AddBankAccount race condition fix

Sprint 5 (Webhook Reliability):
  T-20 → T-21 → T-22 → T-23
  (outbox signing + merchant_id, payload standar, per-payment callback URL)

Sprint 6 (Business Logic):
  T-24  Platform margin fee
  T-25  Payment expiry worker
  T-26  Bank account verification enforcement
  T-30  Account-based disbursement

Sprint 7 (Ops & Polish):
  T-21 (private key vault) → T-26 (treasury) → T-27 → T-28 → T-29 → T-31 → T-32 → T-33
```

---

## Summary Count

| Priority | Jumlah Task |
|---|---|
| P0 — Kritis (prod blocker) | **9** |
| P1 — Blocking | 6 |
| P2 — Penting | 8 |
| Fee System (P1–P2) | 8 |
| P3 — Improvement | 10 |
| **Total** | **41** |

### P0 Prod Blocker — Ringkasan Singkat

| Task | File Utama | Satu Baris Fix |
|---|---|---|
| T-01 | `router.go` | Hapus `e.POST("/v1/merchants", ...)` dari public route |
| T-02 | `usecase/merchant.go` | Hapus `FeeConfig` dari `UpdateMerchantInput` |
| T-03 | `entity/payment.go` + migration | Tambah field + kolom `provider_payment_id` |
| T-04 | `provider/doku/doku.go:78` | Tambah `if cfg.IsProduction { baseURL = productionBaseURL }` |
| T-05 | `provider/doku/doku.go:349` | Selesaikan TODO HMAC signature + konfirmasi ke DOKU support |
| T-06 | `handler/webhook_handler.go:68` | `out[strings.ToLower(k)] = v[0]` di `extractHeaders` |
| T-07 | `provider/ipaymu/ipaymu.go:146` | Tambah token-based verification di `ParseWebhook` |
| T-08 | `usecase/impl/disbursement.go:76` | Reservation pattern — INSERT disbursement sebelum call provider |
| T-09 | Semua | Bangun admin system (T-14 s/d T-18) |

---

## File Baru yang Akan Dibuat

```
migrations/
  000003_add_provider_payment_id.up.sql
  000003_add_provider_payment_id.down.sql
  000004_admin.up.sql
  000004_admin.down.sql
  000005_outbox_merchant_id.up.sql
  000005_outbox_merchant_id.down.sql

internal/domain/entity/
  admin.go

internal/domain/repository/
  admin.go

internal/infrastructure/database/postgres/
  admin_repo.go
  query/admin.sql

internal/usecase/
  admin.go

internal/usecase/impl/
  admin.go

internal/delivery/http/middleware/
  admin_auth.go
  webhook_allowlist.go        (opsional, T-23)

internal/delivery/http/handler/
  admin_handler.go
  admin_merchant_handler.go
  admin_report_handler.go

internal/infrastructure/worker/
  expiry_worker.go

pkg/webhook/
  payload.go

cmd/api/
  seed.go
```

## File yang Diubah (Major)

```
internal/domain/entity/payment.go              ← T-03 ProviderPaymentID
internal/usecase/merchant.go                   ← T-02 hapus FeeConfig dari UpdateInput
internal/usecase/impl/merchant.go              ← T-02
internal/usecase/impl/payment.go               ← T-03 T-04 T-14 T-16 T-24
internal/usecase/impl/disbursement.go          ← T-05 T-15 T-16 T-26
internal/usecase/impl/helpers.go               ← T-16 computeEffectiveFee
internal/infrastructure/database/postgres/     ← sqlc regen setelah tiap migration
internal/infrastructure/worker/outbox_worker.go ← T-12 T-18
internal/infrastructure/database/postgres/outbox_repo.go ← T-12 T-18
internal/delivery/http/router.go               ← T-01 T-09 T-11 T-19
internal/app/app.go                            ← T-09 T-17 T-25
```
