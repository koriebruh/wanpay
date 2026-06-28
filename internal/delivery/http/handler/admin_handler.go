package handler

import (
	"strconv"

	"github.com/labstack/echo/v4"

	"wanpey/core/internal/delivery/http/middleware"
	"wanpey/core/internal/domain/entity"
	"wanpey/core/internal/usecase"
	"wanpey/core/pkg/response"
)

type AdminHandler struct {
	uc usecase.AdminUsecase
}

func NewAdminHandler(uc usecase.AdminUsecase) *AdminHandler {
	return &AdminHandler{uc: uc}
}

// ── Auth ──────────────────────────────────────────────────────────────────────

// Login godoc
//
//	@Summary      Admin login
//	@Description  Authenticates an admin and returns access + refresh tokens. Access token expires based on config (default 8h). Never distinguish wrong email vs wrong password in errors.
//	@Tags         Admin / Auth
//	@Accept       json
//	@Produce      json
//	@Param        body  body      usecase.AdminLoginInput  true  "Credentials"
//	@Success      200   {object}  response.SuccessResponse{data=usecase.AdminTokenOutput}
//	@Failure      400   {object}  response.ErrorResponse
//	@Failure      401   {object}  response.ErrorResponse
//	@Router       /admin/login [post]
func (h *AdminHandler) Login(c echo.Context) error {
	var input usecase.AdminLoginInput
	if err := c.Bind(&input); err != nil {
		return err
	}
	if err := c.Validate(&input); err != nil {
		return err
	}
	out, err := h.uc.Login(c.Request().Context(), input)
	if err != nil {
		return err
	}
	return response.OK(c, out)
}

// RefreshToken godoc
//
//	@Summary      Refresh access token
//	@Description  Exchanges a valid refresh token for a new access + refresh token pair.
//	@Tags         Admin / Auth
//	@Accept       json
//	@Produce      json
//	@Param        body  body      object{refresh_token=string}  true  "Refresh token"
//	@Success      200   {object}  response.SuccessResponse{data=usecase.AdminTokenOutput}
//	@Failure      401   {object}  response.ErrorResponse
//	@Router       /admin/token/refresh [post]
func (h *AdminHandler) RefreshToken(c echo.Context) error {
	var input struct {
		RefreshToken string `json:"refresh_token" validate:"required"`
	}
	if err := c.Bind(&input); err != nil {
		return err
	}
	if err := c.Validate(&input); err != nil {
		return err
	}
	out, err := h.uc.RefreshToken(c.Request().Context(), input.RefreshToken)
	if err != nil {
		return err
	}
	return response.OK(c, out)
}

// GetMe godoc
//
//	@Summary      Get current admin profile
//	@Description  Returns the authenticated admin's profile and role.
//	@Tags         Admin / Auth
//	@Produce      json
//	@Security     AdminAuth
//	@Success      200  {object}  response.SuccessResponse{data=usecase.AdminOutput}
//	@Failure      401  {object}  response.ErrorResponse
//	@Router       /admin/me [get]
func (h *AdminHandler) GetMe(c echo.Context) error {
	adminID := c.Get(middleware.ContextKeyAdminID).(string)
	out, err := h.uc.GetMe(c.Request().Context(), adminID)
	if err != nil {
		return err
	}
	return response.OK(c, out)
}

// ChangePassword godoc
//
//	@Summary      Change admin password
//	@Description  Changes the authenticated admin's password. Requires current password for verification.
//	@Tags         Admin / Auth
//	@Accept       json
//	@Produce      json
//	@Security     AdminAuth
//	@Param        body  body      object{old_password=string,new_password=string}  true  "Password change"
//	@Success      200   {object}  response.SuccessResponse{data=object{message=string}}
//	@Failure      400   {object}  response.ErrorResponse
//	@Failure      401   {object}  response.ErrorResponse
//	@Router       /admin/me/password [patch]
func (h *AdminHandler) ChangePassword(c echo.Context) error {
	var input struct {
		OldPassword string `json:"old_password" validate:"required"`
		NewPassword string `json:"new_password" validate:"required,min=8"`
	}
	if err := c.Bind(&input); err != nil {
		return err
	}
	if err := c.Validate(&input); err != nil {
		return err
	}
	adminID := c.Get(middleware.ContextKeyAdminID).(string)
	if err := h.uc.ChangePassword(c.Request().Context(), adminID, input.OldPassword, input.NewPassword); err != nil {
		return err
	}
	return response.OK(c, map[string]string{"message": "password changed"})
}

// ── Merchants ─────────────────────────────────────────────────────────────────

// CreateMerchant godoc
//
//	@Summary      Create merchant
//	@Description  Registers a new merchant. Returns API key once — store it securely. Roles: super_admin, ops.
//	@Tags         Admin / Merchants
//	@Accept       json
//	@Produce      json
//	@Security     AdminAuth
//	@Param        body  body      usecase.CreateMerchantInput   true  "Merchant details"
//	@Success      201   {object}  response.SuccessResponse{data=usecase.CreateMerchantOutput}
//	@Failure      400   {object}  response.ErrorResponse
//	@Failure      401   {object}  response.ErrorResponse
//	@Failure      403   {object}  response.ErrorResponse
//	@Router       /admin/merchants [post]
func (h *AdminHandler) CreateMerchant(c echo.Context) error {
	var input usecase.CreateMerchantInput
	if err := c.Bind(&input); err != nil {
		return err
	}
	if err := c.Validate(&input); err != nil {
		return err
	}
	out, err := h.uc.CreateMerchant(c.Request().Context(), input)
	if err != nil {
		return err
	}
	return response.Created(c, out)
}

// ListMerchants godoc
//
//	@Summary      List all merchants
//	@Description  Returns paginated list of all merchants. Roles: all admin roles.
//	@Tags         Admin / Merchants
//	@Produce      json
//	@Security     AdminAuth
//	@Param        page    query     int     false  "Page number (default: 1)"
//	@Param        limit   query     int     false  "Items per page (default: 20)"
//	@Param        status  query     string  false  "Filter: pending, active, suspended, inactive"
//	@Param        search  query     string  false  "Search by name or email"
//	@Success      200     {object}  response.ListResponse{data=[]usecase.MerchantOutput}
//	@Failure      401     {object}  response.ErrorResponse
//	@Router       /admin/merchants [get]
func (h *AdminHandler) ListMerchants(c echo.Context) error {
	page, _ := strconv.Atoi(c.QueryParam("page"))
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	filter := usecase.AdminListMerchantsFilter{
		Status: c.QueryParam("status"),
		Search: c.QueryParam("search"),
		Page:   page,
		Limit:  limit,
	}
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.Limit < 1 {
		filter.Limit = 20
	}
	out, err := h.uc.ListMerchants(c.Request().Context(), filter)
	if err != nil {
		return err
	}
	totalPages := int(out.Total) / out.Limit
	if int(out.Total)%out.Limit != 0 {
		totalPages++
	}
	return response.List(c, out.Items, response.Pagination{
		Page:       out.Page,
		PerPage:    out.Limit,
		Total:      int(out.Total),
		TotalPages: totalPages,
	})
}

// GetMerchant godoc
//
//	@Summary      Get merchant detail
//	@Description  Returns merchant detail by ID. Roles: all admin roles.
//	@Tags         Admin / Merchants
//	@Produce      json
//	@Security     AdminAuth
//	@Param        id   path      string  true  "Merchant ID"
//	@Success      200  {object}  response.SuccessResponse{data=usecase.MerchantOutput}
//	@Failure      401  {object}  response.ErrorResponse
//	@Failure      404  {object}  response.ErrorResponse
//	@Router       /admin/merchants/{id} [get]
func (h *AdminHandler) GetMerchant(c echo.Context) error {
	out, err := h.uc.GetMerchant(c.Request().Context(), c.Param("id"))
	if err != nil {
		return err
	}
	return response.OK(c, out)
}

// ApproveMerchant godoc
//
//	@Summary      Approve merchant
//	@Description  Activates a pending merchant so they can start transacting. Roles: super_admin, ops.
//	@Tags         Admin / Merchants
//	@Produce      json
//	@Security     AdminAuth
//	@Param        id   path      string  true  "Merchant ID"
//	@Success      200  {object}  response.SuccessResponse{data=object{message=string}}
//	@Failure      401  {object}  response.ErrorResponse
//	@Failure      403  {object}  response.ErrorResponse
//	@Failure      404  {object}  response.ErrorResponse
//	@Router       /admin/merchants/{id}/approve [patch]
func (h *AdminHandler) ApproveMerchant(c echo.Context) error {
	if err := h.uc.ApproveMerchant(c.Request().Context(), c.Param("id")); err != nil {
		return err
	}
	return response.OK(c, map[string]string{"message": "merchant approved"})
}

// SuspendMerchant godoc
//
//	@Summary      Suspend merchant
//	@Description  Suspends an active merchant — blocks new transactions. Roles: super_admin, ops.
//	@Tags         Admin / Merchants
//	@Produce      json
//	@Security     AdminAuth
//	@Param        id   path      string  true  "Merchant ID"
//	@Success      200  {object}  response.SuccessResponse{data=object{message=string}}
//	@Failure      401  {object}  response.ErrorResponse
//	@Failure      403  {object}  response.ErrorResponse
//	@Failure      404  {object}  response.ErrorResponse
//	@Router       /admin/merchants/{id}/suspend [patch]
func (h *AdminHandler) SuspendMerchant(c echo.Context) error {
	if err := h.uc.SuspendMerchant(c.Request().Context(), c.Param("id")); err != nil {
		return err
	}
	return response.OK(c, map[string]string{"message": "merchant suspended"})
}

// DeactivateMerchant godoc
//
//	@Summary      Deactivate merchant
//	@Description  Permanently deactivates a merchant. Roles: super_admin, ops.
//	@Tags         Admin / Merchants
//	@Produce      json
//	@Security     AdminAuth
//	@Param        id   path      string  true  "Merchant ID"
//	@Success      200  {object}  response.SuccessResponse{data=object{message=string}}
//	@Failure      401  {object}  response.ErrorResponse
//	@Failure      403  {object}  response.ErrorResponse
//	@Router       /admin/merchants/{id}/deactivate [patch]
func (h *AdminHandler) DeactivateMerchant(c echo.Context) error {
	if err := h.uc.DeactivateMerchant(c.Request().Context(), c.Param("id")); err != nil {
		return err
	}
	return response.OK(c, map[string]string{"message": "merchant deactivated"})
}

// DeleteMerchant godoc
//
//	@Summary      Delete merchant (soft)
//	@Description  Soft-deletes a merchant. Data is retained for audit. Roles: super_admin.
//	@Tags         Admin / Merchants
//	@Produce      json
//	@Security     AdminAuth
//	@Param        id   path  string  true  "Merchant ID"
//	@Success      204  "Deleted"
//	@Failure      401  {object}  response.ErrorResponse
//	@Failure      403  {object}  response.ErrorResponse
//	@Router       /admin/merchants/{id} [delete]
func (h *AdminHandler) DeleteMerchant(c echo.Context) error {
	if err := h.uc.DeleteMerchant(c.Request().Context(), c.Param("id")); err != nil {
		return err
	}
	return response.NoContent(c)
}

// SetMerchantFee godoc
//
//	@Summary      Set merchant fee config
//	@Description  Sets custom VA, QRIS, and disbursement fees for a merchant. Reason is mandatory for audit trail. Roles: super_admin, finance.
//	@Tags         Admin / Merchants
//	@Accept       json
//	@Produce      json
//	@Security     AdminAuth
//	@Param        id    path      string                    true  "Merchant ID"
//	@Param        body  body      usecase.SetMerchantFeeInput  true  "Fee config + reason"
//	@Success      200   {object}  response.SuccessResponse{data=object{message=string}}
//	@Failure      400   {object}  response.ErrorResponse
//	@Failure      401   {object}  response.ErrorResponse
//	@Failure      403   {object}  response.ErrorResponse
//	@Router       /admin/merchants/{id}/fee [patch]
func (h *AdminHandler) SetMerchantFee(c echo.Context) error {
	var input usecase.SetMerchantFeeInput
	if err := c.Bind(&input); err != nil {
		return err
	}
	if err := c.Validate(&input); err != nil {
		return err
	}
	input.MerchantID = c.Param("id")
	input.AdminID = c.Get(middleware.ContextKeyAdminID).(string)
	if err := h.uc.UpdateMerchantFee(c.Request().Context(), input); err != nil {
		return err
	}
	return response.OK(c, map[string]string{"message": "fee updated"})
}

// UpdateCashoutLimit godoc
//
//	@Summary      Set daily cashout limit
//	@Description  Sets the maximum total disbursement amount per day in IDR. 0 = unlimited. Roles: super_admin, finance.
//	@Tags         Admin / Merchants
//	@Accept       json
//	@Produce      json
//	@Security     AdminAuth
//	@Param        id    path      string                        true  "Merchant ID"
//	@Param        body  body      object{limit_idr=integer}     true  "Daily limit in IDR"
//	@Success      200   {object}  response.SuccessResponse{data=object{message=string}}
//	@Failure      400   {object}  response.ErrorResponse
//	@Failure      401   {object}  response.ErrorResponse
//	@Failure      403   {object}  response.ErrorResponse
//	@Router       /admin/merchants/{id}/cashout-limit [patch]
func (h *AdminHandler) UpdateCashoutLimit(c echo.Context) error {
	var input struct {
		LimitIDR int64 `json:"limit_idr" validate:"min=0"`
	}
	if err := c.Bind(&input); err != nil {
		return err
	}
	if err := c.Validate(&input); err != nil {
		return err
	}
	if err := h.uc.UpdateDailyCashoutLimit(c.Request().Context(), c.Param("id"), input.LimitIDR); err != nil {
		return err
	}
	return response.OK(c, map[string]string{"message": "cashout limit updated"})
}

// RegenerateAPIKey godoc
//
//	@Summary      Regenerate merchant API key
//	@Description  Generates a new API key for a merchant and invalidates the old one. Raw key shown once. Roles: super_admin, ops.
//	@Tags         Admin / Merchants
//	@Produce      json
//	@Security     AdminAuth
//	@Param        id   path      string  true  "Merchant ID"
//	@Success      200  {object}  response.SuccessResponse{data=object{api_key=string}}
//	@Failure      401  {object}  response.ErrorResponse
//	@Failure      403  {object}  response.ErrorResponse
//	@Router       /admin/merchants/{id}/api-key/regenerate [post]
func (h *AdminHandler) RegenerateAPIKey(c echo.Context) error {
	rawKey, err := h.uc.RegenerateMerchantAPIKey(c.Request().Context(), c.Param("id"))
	if err != nil {
		return err
	}
	return response.OK(c, map[string]string{"api_key": rawKey})
}

// ── Bank accounts ─────────────────────────────────────────────────────────────

// ListMerchantBankAccounts godoc
//
//	@Summary      List merchant bank accounts
//	@Description  Returns all bank accounts for a merchant including verification status.
//	@Tags         Admin / Merchants
//	@Produce      json
//	@Security     AdminAuth
//	@Param        id   path      string  true  "Merchant ID"
//	@Success      200  {object}  response.SuccessResponse{data=[]usecase.BankAccountOutput}
//	@Failure      401  {object}  response.ErrorResponse
//	@Router       /admin/merchants/{id}/bank-accounts [get]
func (h *AdminHandler) ListMerchantBankAccounts(c echo.Context) error {
	out, err := h.uc.ListMerchantBankAccounts(c.Request().Context(), c.Param("id"))
	if err != nil {
		return err
	}
	return response.OK(c, out)
}

// VerifyBankAccount godoc
//
//	@Summary      Verify bank account
//	@Description  Marks a merchant's bank account as verified. Only verified accounts can receive disbursements. Roles: super_admin, ops.
//	@Tags         Admin / Merchants
//	@Produce      json
//	@Security     AdminAuth
//	@Param        id   path      string  true  "Merchant ID"
//	@Param        aid  path      string  true  "Bank account ID"
//	@Success      200  {object}  response.SuccessResponse{data=object{message=string}}
//	@Failure      401  {object}  response.ErrorResponse
//	@Failure      403  {object}  response.ErrorResponse
//	@Failure      404  {object}  response.ErrorResponse
//	@Router       /admin/merchants/{id}/bank-accounts/{aid}/verify [patch]
func (h *AdminHandler) VerifyBankAccount(c echo.Context) error {
	if err := h.uc.VerifyBankAccount(c.Request().Context(), c.Param("id"), c.Param("aid")); err != nil {
		return err
	}
	return response.OK(c, map[string]string{"message": "bank account verified"})
}

// ── Payments ──────────────────────────────────────────────────────────────────

// ListAllPayments godoc
//
//	@Summary      List all payments (admin)
//	@Description  Returns paginated payments across all merchants. Roles: all admin roles.
//	@Tags         Admin / Payments
//	@Produce      json
//	@Security     AdminAuth
//	@Param        page         query     int     false  "Page (default: 1)"
//	@Param        limit        query     int     false  "Limit (default: 20)"
//	@Param        merchant_id  query     string  false  "Filter by merchant ID"
//	@Param        status       query     string  false  "Filter by status"
//	@Param        provider     query     string  false  "Filter by provider"
//	@Param        start        query     string  false  "Start date (RFC3339)"
//	@Param        end          query     string  false  "End date (RFC3339)"
//	@Success      200          {object}  response.ListResponse{data=[]usecase.PaymentOutput}
//	@Failure      401          {object}  response.ErrorResponse
//	@Router       /admin/payments [get]
func (h *AdminHandler) ListAllPayments(c echo.Context) error {
	page, _ := strconv.Atoi(c.QueryParam("page"))
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	filter := usecase.AdminPaymentFilter{
		MerchantID: c.QueryParam("merchant_id"),
		Status:     c.QueryParam("status"),
		Provider:   c.QueryParam("provider"),
		Start:      c.QueryParam("start"),
		End:        c.QueryParam("end"),
		Page:       page,
		Limit:      limit,
	}
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.Limit < 1 {
		filter.Limit = 20
	}
	out, err := h.uc.ListAllPayments(c.Request().Context(), filter)
	if err != nil {
		return err
	}
	totalPages := int(out.Total) / out.Limit
	if int(out.Total)%out.Limit != 0 {
		totalPages++
	}
	return response.List(c, out.Items, response.Pagination{
		Page:       out.Page,
		PerPage:    out.Limit,
		Total:      int(out.Total),
		TotalPages: totalPages,
	})
}

// GetPaymentAdmin godoc
//
//	@Summary      Get payment detail (admin)
//	@Description  Returns payment detail by ID across all merchants.
//	@Tags         Admin / Payments
//	@Produce      json
//	@Security     AdminAuth
//	@Param        id   path      string  true  "Payment ID"
//	@Success      200  {object}  response.SuccessResponse{data=usecase.PaymentOutput}
//	@Failure      401  {object}  response.ErrorResponse
//	@Failure      404  {object}  response.ErrorResponse
//	@Router       /admin/payments/{id} [get]
func (h *AdminHandler) GetPayment(c echo.Context) error {
	out, err := h.uc.GetPayment(c.Request().Context(), c.Param("id"))
	if err != nil {
		return err
	}
	return response.OK(c, out)
}

// ── Disbursements ─────────────────────────────────────────────────────────────

// ListAllDisbursements godoc
//
//	@Summary      List all disbursements (admin)
//	@Description  Returns paginated disbursements across all merchants. Roles: all admin roles.
//	@Tags         Admin / Disbursements
//	@Produce      json
//	@Security     AdminAuth
//	@Param        page         query     int     false  "Page (default: 1)"
//	@Param        limit        query     int     false  "Limit (default: 20)"
//	@Param        merchant_id  query     string  false  "Filter by merchant ID"
//	@Param        status       query     string  false  "Filter by status"
//	@Param        provider     query     string  false  "Filter by provider"
//	@Param        start        query     string  false  "Start date (RFC3339)"
//	@Param        end          query     string  false  "End date (RFC3339)"
//	@Success      200          {object}  response.ListResponse{data=[]usecase.DisbursementOutput}
//	@Failure      401          {object}  response.ErrorResponse
//	@Router       /admin/disbursements [get]
func (h *AdminHandler) ListAllDisbursements(c echo.Context) error {
	page, _ := strconv.Atoi(c.QueryParam("page"))
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	filter := usecase.AdminDisbursementFilter{
		MerchantID: c.QueryParam("merchant_id"),
		Status:     c.QueryParam("status"),
		Provider:   c.QueryParam("provider"),
		Start:      c.QueryParam("start"),
		End:        c.QueryParam("end"),
		Page:       page,
		Limit:      limit,
	}
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.Limit < 1 {
		filter.Limit = 20
	}
	out, err := h.uc.ListAllDisbursements(c.Request().Context(), filter)
	if err != nil {
		return err
	}
	totalPages := int(out.Total) / out.Limit
	if int(out.Total)%out.Limit != 0 {
		totalPages++
	}
	return response.List(c, out.Items, response.Pagination{
		Page:       out.Page,
		PerPage:    out.Limit,
		Total:      int(out.Total),
		TotalPages: totalPages,
	})
}

// GetDisbursementAdmin godoc
//
//	@Summary      Get disbursement detail (admin)
//	@Description  Returns disbursement detail by ID across all merchants.
//	@Tags         Admin / Disbursements
//	@Produce      json
//	@Security     AdminAuth
//	@Param        id   path      string  true  "Disbursement ID"
//	@Success      200  {object}  response.SuccessResponse{data=usecase.DisbursementOutput}
//	@Failure      401  {object}  response.ErrorResponse
//	@Failure      404  {object}  response.ErrorResponse
//	@Router       /admin/disbursements/{id} [get]
func (h *AdminHandler) GetDisbursement(c echo.Context) error {
	out, err := h.uc.GetDisbursement(c.Request().Context(), c.Param("id"))
	if err != nil {
		return err
	}
	return response.OK(c, out)
}

// ── Mutations ─────────────────────────────────────────────────────────────────

// ListAllMutations godoc
//
//	@Summary      List all mutations (admin)
//	@Description  Returns paginated ledger mutations across all merchants. Roles: super_admin, finance.
//	@Tags         Admin / Mutations
//	@Produce      json
//	@Security     AdminAuth
//	@Param        page         query     int     false  "Page (default: 1)"
//	@Param        limit        query     int     false  "Limit (default: 20)"
//	@Param        merchant_id  query     string  false  "Filter by merchant ID"
//	@Param        type         query     string  false  "Filter by type: credit, debit"
//	@Param        start        query     string  false  "Start date (RFC3339)"
//	@Param        end          query     string  false  "End date (RFC3339)"
//	@Success      200          {object}  response.ListResponse{data=[]usecase.MutationOutput}
//	@Failure      401          {object}  response.ErrorResponse
//	@Failure      403          {object}  response.ErrorResponse
//	@Router       /admin/mutations [get]
func (h *AdminHandler) ListAllMutations(c echo.Context) error {
	page, _ := strconv.Atoi(c.QueryParam("page"))
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	filter := usecase.AdminMutationFilter{
		MerchantID: c.QueryParam("merchant_id"),
		Type:       c.QueryParam("type"),
		Start:      c.QueryParam("start"),
		End:        c.QueryParam("end"),
		Page:       page,
		Limit:      limit,
	}
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.Limit < 1 {
		filter.Limit = 20
	}
	out, err := h.uc.ListAllMutations(c.Request().Context(), filter)
	if err != nil {
		return err
	}
	totalPages := int(out.Total) / out.Limit
	if int(out.Total)%out.Limit != 0 {
		totalPages++
	}
	return response.List(c, out.Items, response.Pagination{
		Page:       out.Page,
		PerPage:    out.Limit,
		Total:      int(out.Total),
		TotalPages: totalPages,
	})
}

// ── Provider balances ─────────────────────────────────────────────────────────

// GetProviderBalances godoc
//
//	@Summary      Get provider balances
//	@Description  Returns platform's known balance at each payment provider. Roles: super_admin, finance.
//	@Tags         Admin / Treasury
//	@Produce      json
//	@Security     AdminAuth
//	@Success      200  {object}  response.SuccessResponse{data=[]entity.ProviderBalance}
//	@Failure      401  {object}  response.ErrorResponse
//	@Failure      403  {object}  response.ErrorResponse
//	@Router       /admin/provider-balances [get]
func (h *AdminHandler) GetProviderBalances(c echo.Context) error {
	out, err := h.uc.GetProviderBalances(c.Request().Context())
	if err != nil {
		return err
	}
	return response.OK(c, out)
}

// UpdateProviderBalance godoc
//
//	@Summary      Update provider balance
//	@Description  Manually records the platform's balance at a specific provider (for reconciliation). Roles: super_admin, finance.
//	@Tags         Admin / Treasury
//	@Accept       json
//	@Produce      json
//	@Security     AdminAuth
//	@Param        provider  path      string                       true  "Provider: midtrans, xendit, doku, ipaymu"
//	@Param        body      body      object{balance_idr=integer}  true  "Balance in IDR"
//	@Success      200       {object}  response.SuccessResponse{data=object{message=string}}
//	@Failure      400       {object}  response.ErrorResponse
//	@Failure      401       {object}  response.ErrorResponse
//	@Failure      403       {object}  response.ErrorResponse
//	@Router       /admin/provider-balances/{provider} [patch]
func (h *AdminHandler) UpdateProviderBalance(c echo.Context) error {
	var input struct {
		BalanceIDR int64 `json:"balance_idr" validate:"min=0"`
	}
	if err := c.Bind(&input); err != nil {
		return err
	}
	if err := c.Validate(&input); err != nil {
		return err
	}
	provider := entity.Provider(c.Param("provider"))
	if err := h.uc.UpdateProviderBalance(c.Request().Context(), provider, input.BalanceIDR); err != nil {
		return err
	}
	return response.OK(c, map[string]string{"message": "provider balance updated"})
}

// ── Admin management ──────────────────────────────────────────────────────────

// CreateAdmin godoc
//
//	@Summary      Create admin user
//	@Description  Creates a new admin account. Roles: super_admin only.
//	@Tags         Admin / Management
//	@Accept       json
//	@Produce      json
//	@Security     AdminAuth
//	@Param        body  body      usecase.CreateAdminInput  true  "Admin details"
//	@Success      201   {object}  response.SuccessResponse{data=usecase.AdminOutput}
//	@Failure      400   {object}  response.ErrorResponse
//	@Failure      401   {object}  response.ErrorResponse
//	@Failure      403   {object}  response.ErrorResponse
//	@Router       /admin/admins [post]
func (h *AdminHandler) CreateAdmin(c echo.Context) error {
	var input usecase.CreateAdminInput
	if err := c.Bind(&input); err != nil {
		return err
	}
	if err := c.Validate(&input); err != nil {
		return err
	}
	input.RequesterID = c.Get(middleware.ContextKeyAdminID).(string)
	out, err := h.uc.CreateAdmin(c.Request().Context(), input)
	if err != nil {
		return err
	}
	return response.Created(c, out)
}

// ListAdmins godoc
//
//	@Summary      List admin users
//	@Description  Returns all admin accounts. Roles: super_admin only.
//	@Tags         Admin / Management
//	@Produce      json
//	@Security     AdminAuth
//	@Param        page   query     int  false  "Page (default: 1)"
//	@Param        limit  query     int  false  "Limit (default: 20)"
//	@Success      200    {object}  response.ListResponse{data=[]usecase.AdminOutput}
//	@Failure      401    {object}  response.ErrorResponse
//	@Failure      403    {object}  response.ErrorResponse
//	@Router       /admin/admins [get]
func (h *AdminHandler) ListAdmins(c echo.Context) error {
	page, _ := strconv.Atoi(c.QueryParam("page"))
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 20
	}
	items, total, err := h.uc.ListAdmins(c.Request().Context(), page, limit)
	if err != nil {
		return err
	}
	totalPages := int(total) / limit
	if int(total)%limit != 0 {
		totalPages++
	}
	return response.List(c, items, response.Pagination{
		Page:       page,
		PerPage:    limit,
		Total:      int(total),
		TotalPages: totalPages,
	})
}

// DeactivateAdmin godoc
//
//	@Summary      Deactivate admin user
//	@Description  Deactivates an admin account. Cannot deactivate yourself. Roles: super_admin only.
//	@Tags         Admin / Management
//	@Produce      json
//	@Security     AdminAuth
//	@Param        id   path      string  true  "Admin ID"
//	@Success      200  {object}  response.SuccessResponse{data=object{message=string}}
//	@Failure      401  {object}  response.ErrorResponse
//	@Failure      403  {object}  response.ErrorResponse
//	@Failure      422  {object}  response.ErrorResponse  "Cannot deactivate yourself"
//	@Router       /admin/admins/{id}/deactivate [patch]
func (h *AdminHandler) DeactivateAdmin(c echo.Context) error {
	callerID := c.Get(middleware.ContextKeyAdminID).(string)
	if err := h.uc.DeactivateAdmin(c.Request().Context(), callerID, c.Param("id")); err != nil {
		return err
	}
	return response.OK(c, map[string]string{"message": "admin deactivated"})
}

// ── Fee management ────────────────────────────────────────────────────────────

// GetFeeDefault godoc
//
//	@Summary      Get global default fee
//	@Description  Returns the platform's global default fee applied when a merchant has no custom contract. Roles: super_admin, finance.
//	@Tags         Admin / Fee Management
//	@Produce      json
//	@Security     AdminAuth
//	@Success      200  {object}  response.SuccessResponse{data=entity.FeeDefault}
//	@Failure      401  {object}  response.ErrorResponse
//	@Failure      403  {object}  response.ErrorResponse
//	@Router       /admin/fees/default [get]
func (h *AdminHandler) GetFeeDefault(c echo.Context) error {
	out, err := h.uc.GetFeeDefault(c.Request().Context())
	if err != nil {
		return err
	}
	return response.OK(c, out)
}

// UpdateFeeDefault godoc
//
//	@Summary      Update global default fee
//	@Description  Updates the platform's global default fee. Mandatory reason for audit trail. Roles: super_admin, finance.
//	@Tags         Admin / Fee Management
//	@Accept       json
//	@Produce      json
//	@Security     AdminAuth
//	@Param        body  body      object{fee_config=entity.FeeConfig,reason=string}  true  "New fee config + reason (min 10 chars)"
//	@Success      200   {object}  response.SuccessResponse{data=object{message=string}}
//	@Failure      400   {object}  response.ErrorResponse
//	@Failure      401   {object}  response.ErrorResponse
//	@Failure      403   {object}  response.ErrorResponse
//	@Router       /admin/fees/default [put]
func (h *AdminHandler) UpdateFeeDefault(c echo.Context) error {
	adminID := c.Get(middleware.ContextKeyAdminID).(string)
	var body struct {
		FeeConfig entity.FeeConfig `json:"fee_config" validate:"required"`
		Reason    string           `json:"reason"     validate:"required,min=10"`
	}
	if err := c.Bind(&body); err != nil {
		return err
	}
	if err := c.Validate(&body); err != nil {
		return err
	}
	if err := h.uc.UpdateFeeDefault(c.Request().Context(), adminID, body.Reason, body.FeeConfig); err != nil {
		return err
	}
	return response.OK(c, map[string]string{"message": "fee default updated"})
}

// GetPlatformMargin godoc
//
//	@Summary      Get platform margin
//	@Description  Returns Wanpey's platform margin added on top of base fees. Roles: super_admin, finance.
//	@Tags         Admin / Fee Management
//	@Produce      json
//	@Security     AdminAuth
//	@Success      200  {object}  response.SuccessResponse{data=entity.PlatformMargin}
//	@Failure      401  {object}  response.ErrorResponse
//	@Failure      403  {object}  response.ErrorResponse
//	@Router       /admin/fees/margin [get]
func (h *AdminHandler) GetPlatformMargin(c echo.Context) error {
	out, err := h.uc.GetPlatformMargin(c.Request().Context())
	if err != nil {
		return err
	}
	return response.OK(c, out)
}

// UpdatePlatformMargin godoc
//
//	@Summary      Update platform margin
//	@Description  Updates Wanpey's margin on top of all fees. Can be toggled on/off per method. Roles: super_admin only.
//	@Tags         Admin / Fee Management
//	@Accept       json
//	@Produce      json
//	@Security     AdminAuth
//	@Param        body  body      object{enabled=bool,margin=entity.FeeConfig,reason=string}  true  "Margin config + reason"
//	@Success      200   {object}  response.SuccessResponse{data=object{message=string}}
//	@Failure      400   {object}  response.ErrorResponse
//	@Failure      401   {object}  response.ErrorResponse
//	@Failure      403   {object}  response.ErrorResponse
//	@Router       /admin/fees/margin [put]
func (h *AdminHandler) UpdatePlatformMargin(c echo.Context) error {
	adminID := c.Get(middleware.ContextKeyAdminID).(string)
	var body struct {
		Enabled bool             `json:"enabled"`
		Margin  entity.FeeConfig `json:"margin" validate:"required"`
		Reason  string           `json:"reason" validate:"required,min=10"`
	}
	if err := c.Bind(&body); err != nil {
		return err
	}
	if err := c.Validate(&body); err != nil {
		return err
	}
	if err := h.uc.UpdatePlatformMargin(c.Request().Context(), adminID, body.Reason, body.Enabled, body.Margin); err != nil {
		return err
	}
	return response.OK(c, map[string]string{"message": "platform margin updated"})
}

// ── Holiday surcharge ─────────────────────────────────────────────────────────

// ListHolidays godoc
//
//	@Summary      List holiday surcharges
//	@Description  Returns all configured holiday surcharges. Roles: super_admin, finance.
//	@Tags         Admin / Fee Management
//	@Produce      json
//	@Security     AdminAuth
//	@Param        page   query     int  false  "Page (default: 1)"
//	@Param        limit  query     int  false  "Limit (default: 20)"
//	@Success      200    {object}  response.ListResponse{data=[]entity.FeeHoliday}
//	@Failure      401    {object}  response.ErrorResponse
//	@Failure      403    {object}  response.ErrorResponse
//	@Router       /admin/fees/holidays [get]
func (h *AdminHandler) ListHolidays(c echo.Context) error {
	page, _ := strconv.Atoi(c.QueryParam("page"))
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 20
	}
	items, total, err := h.uc.ListHolidays(c.Request().Context(), page, limit)
	if err != nil {
		return err
	}
	totalPages := int(total) / limit
	if int(total)%limit != 0 {
		totalPages++
	}
	return response.List(c, items, response.Pagination{
		Page:       page,
		PerPage:    limit,
		Total:      int(total),
		TotalPages: totalPages,
	})
}

// CreateHoliday godoc
//
//	@Summary      Create holiday surcharge
//	@Description  Adds a fee surcharge for a specific date (e.g. public holiday). Roles: super_admin, finance.
//	@Tags         Admin / Fee Management
//	@Accept       json
//	@Produce      json
//	@Security     AdminAuth
//	@Param        body  body      entity.FeeHoliday  true  "Holiday config"
//	@Success      200   {object}  response.SuccessResponse{data=entity.FeeHoliday}
//	@Failure      400   {object}  response.ErrorResponse
//	@Failure      401   {object}  response.ErrorResponse
//	@Failure      403   {object}  response.ErrorResponse
//	@Router       /admin/fees/holidays [post]
func (h *AdminHandler) CreateHoliday(c echo.Context) error {
	adminID := c.Get(middleware.ContextKeyAdminID).(string)
	var holiday entity.FeeHoliday
	if err := c.Bind(&holiday); err != nil {
		return err
	}
	if err := h.uc.CreateHoliday(c.Request().Context(), adminID, &holiday); err != nil {
		return err
	}
	return response.OK(c, holiday)
}

// UpdateHoliday godoc
//
//	@Summary      Update holiday surcharge
//	@Description  Updates an existing holiday surcharge (can toggle active state). Roles: super_admin, finance.
//	@Tags         Admin / Fee Management
//	@Accept       json
//	@Produce      json
//	@Security     AdminAuth
//	@Param        id    path      string             true  "Holiday ID"
//	@Param        body  body      entity.FeeHoliday  true  "Updated holiday config"
//	@Success      200   {object}  response.SuccessResponse{data=entity.FeeHoliday}
//	@Failure      400   {object}  response.ErrorResponse
//	@Failure      401   {object}  response.ErrorResponse
//	@Failure      403   {object}  response.ErrorResponse
//	@Router       /admin/fees/holidays/{id} [put]
func (h *AdminHandler) UpdateHoliday(c echo.Context) error {
	adminID := c.Get(middleware.ContextKeyAdminID).(string)
	var holiday entity.FeeHoliday
	if err := c.Bind(&holiday); err != nil {
		return err
	}
	holiday.ID = c.Param("id")
	if err := h.uc.UpdateHoliday(c.Request().Context(), adminID, &holiday); err != nil {
		return err
	}
	return response.OK(c, holiday)
}
