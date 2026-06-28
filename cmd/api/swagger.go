package main

// Wanpey Payment Gateway API
//
//	@title                      Wanpey Payment Gateway API
//	@version                    1.0
//	@description                Payment gateway aggregator — VA, QRIS, and disbursement via Midtrans, Xendit, DOKU, iPaymu.
//	@description
//	@description                ## Authentication
//	@description                Merchant endpoints (/v1/*): `X-API-Key` header with your API key (`wpay_live_*` or `wpay_test_*`).
//	@description                Admin endpoints (/admin/*): `Authorization: Bearer <access_token>` from POST /admin/login.
//	@description
//	@description                ## Idempotency
//	@description                Payment and disbursement creation support `Idempotency-Key` header. Use unique key per request to safely retry.
//	@description
//	@description                ## Webhook
//	@description                Wanpey signs outbound webhooks with `X-Wanpey-Signature` (HMAC-SHA256). Verify using your webhook secret.
//	@contact.name               Wanpey Support
//	@contact.email              support@wanpey.id
//	@host                       localhost:8080
//	@BasePath                   /
//	@schemes                    http https
//	@securityDefinitions.apikey ApiKeyAuth
//	@in                         header
//	@name                       X-API-Key
//	@description                Merchant API key
//	@securityDefinitions.apikey AdminAuth
//	@in                         header
//	@name                       Authorization
//	@description                Admin JWT — Bearer <token>
