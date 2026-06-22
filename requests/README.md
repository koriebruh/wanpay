# HTTP Request Files — Wanpey API

Format: **VS Code REST Client** (extension `humao.rest-client`) atau **JetBrains HTTP Client**.

## Setup

1. Install REST Client di VS Code: `Ctrl+Shift+X` → cari `REST Client`
2. Copy `.config.example.toml` ke `.config.toml` dan isi credentials provider
3. `make infra-up` → `make migrate-up` → `make dev`
4. Buka file `.http`, klik **Send Request** di atas tiap request

## Alur Testing End-to-End

```
00_health.http      → verifikasi server dan semua komponen hidup
      ↓
01_merchant.http    → daftar merchant, dapat api_key dari response
      ↓
02_payment.http     → buat VA/QRIS (isi @apiKey dari step sebelumnya)
      ↓
05_webhook.http     → simulasi notifikasi dari provider (payment → paid)
      ↓
04_mutation.http    → cek saldo dan riwayat mutasi (harus ada cash_in)
      ↓
03_disbursement.http → cairkan saldo ke rekening bank
      ↓
05_webhook.http     → simulasi disbursement callback (disbursement → completed)
      ↓
04_mutation.http    → cek saldo berkurang (ada cash_out)
```

## Generate Signature Midtrans

```bash
# Format: SHA512(order_id + status_code + gross_amount + server_key)
ORDER_ID="wpay-xxx"
STATUS_CODE="200"
GROSS_AMOUNT="100000.00"
SERVER_KEY="SB-Mid-server-xxxx"  # dari .config.toml

echo -n "${ORDER_ID}${STATUS_CODE}${GROSS_AMOUNT}${SERVER_KEY}" | sha512sum | awk '{print $1}'
```

## Generate Signature DOKU (HMAC-SHA512)

```bash
# Dibuat otomatis oleh DOKU gateway saat memanggil webhook ke URL kita.
# Untuk simulasi manual, lihat pkg/signature/signature.go → SignSHA512().
```

## Variable Quick Reference

| Variable | Cara dapat | Dipakai di |
|---|---|---|
| `@apiKey` | Response `POST /v1/merchants` → field `api_key` | Semua request berauth |
| `@paymentId` | Response `POST /v1/payments/va` atau `/qris` → field `id` | `02_payment.http` |
| `@disbursementId` | Response `POST /v1/disbursements` → field `id` | `03_disbursement.http` |
| `@mutationId` | Response `GET /v1/mutations` → field `items[0].id` | `04_mutation.http` |
| `@bankAccountId` | Response `POST /v1/merchants/me/bank-accounts` → field `id` | `01_merchant.http` |
| `@externalId` | Dari response payment/disbursement → field `external_id` | `05_webhook.http` |
