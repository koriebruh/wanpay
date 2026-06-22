package http

import (
	"github.com/labstack/echo/v4"

	"wanpey/core/internal/delivery/http/handler"
	httpmw "wanpey/core/internal/delivery/http/middleware"
	"wanpey/core/internal/domain/repository"
	"wanpey/core/internal/infrastructure/cache"
)

// Routes holds all route handlers and shared middleware dependencies.
type Routes struct {
	MerchantRepo repository.MerchantRepository
	Cache        cache.Cache
	Payment      *handler.PaymentHandler
	Disbursement *handler.DisbursementHandler
	Mutation     *handler.MutationHandler
	Merchant     *handler.MerchantHandler
	Webhook      *handler.WebhookHandler
}

// Register mounts all API routes on the Echo instance.
func Register(e *echo.Echo, r Routes) {
	auth := httpmw.APIKeyAuth(r.MerchantRepo)
	idempotency := httpmw.Idempotency(r.Cache)

	// Webhook routes — no auth, provider signs payload
	webhooks := e.Group("/webhooks")
	webhooks.POST("/:provider/payment", r.Webhook.HandlePaymentWebhook)
	webhooks.POST("/:provider/disbursement", r.Webhook.HandleDisbursementWebhook)

	// v1 — all routes require API key auth
	v1 := e.Group("/v1", auth)

	// Payments
	payments := v1.Group("/payments")
	payments.POST("/va", r.Payment.CreateVA, idempotency)
	payments.POST("/qris", r.Payment.CreateQRIS, idempotency)
	payments.GET("/:id", r.Payment.GetPayment)
	payments.DELETE("/:id", r.Payment.CancelPayment)

	// Disbursements
	disbursements := v1.Group("/disbursements")
	disbursements.POST("", r.Disbursement.Disburse, idempotency)
	disbursements.GET("/:id", r.Disbursement.GetDisbursement)

	// Mutations (ledger)
	mutations := v1.Group("/mutations")
	mutations.GET("", r.Mutation.ListMutations)
	mutations.GET("/balance", r.Mutation.GetBalance)
	mutations.GET("/:id", r.Mutation.GetMutation)

	// Merchants
	// POST /v1/merchants — registration, no auth (handled below before the auth group)
	e.POST("/v1/merchants", r.Merchant.Create)

	me := v1.Group("/merchants/me")
	me.GET("", r.Merchant.GetMerchant)
	me.PATCH("", r.Merchant.Update)
	me.POST("/api-key/regenerate", r.Merchant.RegenerateAPIKey)

	bankAccounts := me.Group("/bank-accounts")
	bankAccounts.GET("", r.Merchant.ListBankAccounts)
	bankAccounts.POST("", r.Merchant.AddBankAccount)
	bankAccounts.DELETE("/:id", r.Merchant.RemoveBankAccount)
	bankAccounts.PATCH("/:id/primary", r.Merchant.SetPrimaryBankAccount)
}
