package http

import (
	"net/http"

	"github.com/labstack/echo/v4"
	echoswagger "github.com/swaggo/echo-swagger"
	"go.uber.org/zap"

	_ "wanpey/core/docs" // swagger generated docs
	"wanpey/core/internal/delivery/http/handler"
	httpmw "wanpey/core/internal/delivery/http/middleware"
	"wanpey/core/internal/domain/entity"
	"wanpey/core/internal/domain/repository"
	"wanpey/core/internal/infrastructure/cache"
)

// Routes holds all route handlers and shared middleware dependencies.
type Routes struct {
	MerchantRepo         repository.MerchantRepository
	Cache                cache.Cache
	Payment              *handler.PaymentHandler
	Disbursement         *handler.DisbursementHandler
	Mutation             *handler.MutationHandler
	Merchant             *handler.MerchantHandler
	Webhook              *handler.WebhookHandler
	Admin                *handler.AdminHandler
	AdminJWTSecret    string
	WebhookAllowedIPs map[string][]string // provider → CIDRs; empty = accept all
	Log               *zap.Logger
}

// Register mounts all API routes on the Echo instance.
func Register(e *echo.Echo, r Routes) {
	// Swagger UI — available at /swagger/index.html
	// Serve static swagger assets from echo-swagger, then override index to load a single spec URL.
	e.GET("/swagger/*", echoswagger.WrapHandler)
	e.GET("/swagger/index.html", swaggerIndexHandler)
	auth := httpmw.APIKeyAuth(r.MerchantRepo)
	idempotency := httpmw.Idempotency(r.Cache)

	// Webhook routes — no auth, provider signs payload
	// IP allowlist is applied when WebhookAllowedIPs is configured (defense-in-depth).
	webhooks := e.Group("/webhooks")
	if len(r.WebhookAllowedIPs) > 0 && r.Log != nil {
		webhooks.Use(httpmw.WebhookIPAllowlist(r.WebhookAllowedIPs, r.Log))
	}
	webhooks.POST("/:provider/payment", r.Webhook.HandlePaymentWebhook)
	webhooks.POST("/:provider/disbursement", r.Webhook.HandleDisbursementWebhook)

	// v1 — all routes require API key auth
	v1 := e.Group("/v1", auth)

	// Payments
	payments := v1.Group("/payments")
	payments.GET("", r.Payment.ListPayments)
	payments.POST("/va", r.Payment.CreateVA, idempotency)
	payments.POST("/qris", r.Payment.CreateQRIS, idempotency)
	payments.GET("/:id", r.Payment.GetPayment)
	payments.DELETE("/:id", r.Payment.CancelPayment)

	// Disbursements
	disbursements := v1.Group("/disbursements")
	disbursements.GET("", r.Disbursement.ListDisbursements)
	disbursements.POST("", r.Disbursement.Disburse, idempotency)
	disbursements.GET("/:id", r.Disbursement.GetDisbursement)

	// Mutations (ledger)
	mutations := v1.Group("/mutations")
	mutations.GET("", r.Mutation.ListMutations)
	mutations.GET("/balance", r.Mutation.GetBalance)
	mutations.GET("/:id", r.Mutation.GetMutation)

	// Merchants (self-service — authenticated)
	me := v1.Group("/merchants/me")
	me.GET("", r.Merchant.GetMerchant)
	me.PATCH("", r.Merchant.Update)
	me.POST("/api-key/regenerate", r.Merchant.RegenerateAPIKey)

	bankAccounts := me.Group("/bank-accounts")
	bankAccounts.GET("", r.Merchant.ListBankAccounts)
	bankAccounts.POST("", r.Merchant.AddBankAccount)
	bankAccounts.DELETE("/:id", r.Merchant.RemoveBankAccount)
	bankAccounts.PATCH("/:id/primary", r.Merchant.SetPrimaryBankAccount)

	me.GET("/webhook-events", r.Merchant.ListWebhookEvents)

	registerAdminRoutes(e, r)
}

// registerAdminRoutes mounts /admin endpoints with the full permission matrix.
// Login and refresh are public; all other routes require a valid admin access token.
func registerAdminRoutes(e *echo.Echo, r Routes) {
	if r.Admin == nil {
		return
	}

	admin := e.Group("/admin")

	// Public — no auth required
	admin.POST("/login", r.Admin.Login)
	admin.POST("/token/refresh", r.Admin.RefreshToken)

	// Authenticated — valid admin JWT required for all routes below
	authed := admin.Group("", httpmw.AdminJWTAuth(r.AdminJWTSecret))

	// Self
	authed.GET("/me", r.Admin.GetMe)
	authed.PATCH("/me/password", r.Admin.ChangePassword)

	// Merchants
	merchants := authed.Group("/merchants")
	merchants.POST("", r.Admin.CreateMerchant,
		httpmw.RequireRole(entity.AdminRoleSuperAdmin, entity.AdminRoleOps))
	merchants.GET("", r.Admin.ListMerchants)
	merchants.GET("/:id", r.Admin.GetMerchant)
	merchants.PATCH("/:id/approve", r.Admin.ApproveMerchant,
		httpmw.RequireRole(entity.AdminRoleSuperAdmin, entity.AdminRoleOps))
	merchants.PATCH("/:id/suspend", r.Admin.SuspendMerchant,
		httpmw.RequireRole(entity.AdminRoleSuperAdmin, entity.AdminRoleOps))
	merchants.PATCH("/:id/deactivate", r.Admin.DeactivateMerchant,
		httpmw.RequireRole(entity.AdminRoleSuperAdmin, entity.AdminRoleOps))
	merchants.PATCH("/:id/fee", r.Admin.SetMerchantFee,
		httpmw.RequireRole(entity.AdminRoleSuperAdmin, entity.AdminRoleFinance))
	merchants.PATCH("/:id/cashout-limit", r.Admin.UpdateCashoutLimit,
		httpmw.RequireRole(entity.AdminRoleSuperAdmin, entity.AdminRoleFinance))
	merchants.POST("/:id/api-key/regenerate", r.Admin.RegenerateAPIKey,
		httpmw.RequireRole(entity.AdminRoleSuperAdmin, entity.AdminRoleOps))
	merchants.DELETE("/:id", r.Admin.DeleteMerchant,
		httpmw.RequireRole(entity.AdminRoleSuperAdmin))
	merchants.GET("/:id/bank-accounts", r.Admin.ListMerchantBankAccounts)
	merchants.PATCH("/:id/bank-accounts/:aid/verify", r.Admin.VerifyBankAccount,
		httpmw.RequireRole(entity.AdminRoleSuperAdmin, entity.AdminRoleOps))

	// Payments
	adminPayments := authed.Group("/payments")
	adminPayments.GET("", r.Admin.ListAllPayments)
	adminPayments.GET("/:id", r.Admin.GetPayment)

	// Disbursements
	adminDisbursements := authed.Group("/disbursements")
	adminDisbursements.GET("", r.Admin.ListAllDisbursements)
	adminDisbursements.GET("/:id", r.Admin.GetDisbursement)

	// Mutations — finance + super_admin only
	adminMutations := authed.Group("/mutations",
		httpmw.RequireRole(entity.AdminRoleSuperAdmin, entity.AdminRoleFinance))
	adminMutations.GET("", r.Admin.ListAllMutations)

	// Provider balances — finance + super_admin only
	providerBalances := authed.Group("/provider-balances",
		httpmw.RequireRole(entity.AdminRoleSuperAdmin, entity.AdminRoleFinance))
	providerBalances.GET("", r.Admin.GetProviderBalances)
	providerBalances.PATCH("/:provider", r.Admin.UpdateProviderBalance)

	// Admin management — super_admin only
	admins := authed.Group("/admins",
		httpmw.RequireRole(entity.AdminRoleSuperAdmin))
	admins.POST("", r.Admin.CreateAdmin)
	admins.GET("", r.Admin.ListAdmins)
	admins.PATCH("/:id/deactivate", r.Admin.DeactivateAdmin)

	// Fee management — super_admin and finance
	feeAdmin := authed.Group("/fees",
		httpmw.RequireRole(entity.AdminRoleSuperAdmin, entity.AdminRoleFinance))
	feeAdmin.GET("/default", r.Admin.GetFeeDefault)
	feeAdmin.PUT("/default", r.Admin.UpdateFeeDefault,
		httpmw.RequireRole(entity.AdminRoleSuperAdmin, entity.AdminRoleFinance))
	feeAdmin.GET("/margin", r.Admin.GetPlatformMargin)
	feeAdmin.PUT("/margin", r.Admin.UpdatePlatformMargin,
		httpmw.RequireRole(entity.AdminRoleSuperAdmin))
	feeAdmin.GET("/holidays", r.Admin.ListHolidays)
	feeAdmin.POST("/holidays", r.Admin.CreateHoliday,
		httpmw.RequireRole(entity.AdminRoleSuperAdmin, entity.AdminRoleFinance))
	feeAdmin.PUT("/holidays/:id", r.Admin.UpdateHoliday,
		httpmw.RequireRole(entity.AdminRoleSuperAdmin, entity.AdminRoleFinance))
}

// swaggerIndexHandler serves a clean Swagger UI that auto-loads /swagger/doc.json.
func swaggerIndexHandler(c echo.Context) error {
	html := `<!DOCTYPE html>
<html>
<head>
  <title>Wanpey API Docs</title>
  <meta charset="utf-8"/>
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <link rel="stylesheet" type="text/css" href="./swagger-ui.css">
</head>
<body>
<div id="swagger-ui"></div>
<script src="./swagger-ui-bundle.js"></script>
<script src="./swagger-ui-standalone-preset.js"></script>
<script>
window.onload = function() {
  const ui = SwaggerUIBundle({
    url: "/swagger/doc.json",
    dom_id: '#swagger-ui',
    deepLinking: true,
    presets: [SwaggerUIBundle.presets.apis, SwaggerUIStandalonePreset],
    plugins: [SwaggerUIBundle.plugins.DownloadUrl],
    layout: "StandaloneLayout",
    persistAuthorization: true,
  })
  window.ui = ui
}
</script>
</body>
</html>`
	return c.HTML(http.StatusOK, html)
}
