package handler

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"wanpey/core/internal/delivery/http/middleware"
	"wanpey/core/internal/usecase"
	"wanpey/core/pkg/response"
)

type MerchantHandler struct {
	uc usecase.MerchantUsecase
}

func NewMerchantHandler(uc usecase.MerchantUsecase) *MerchantHandler {
	return &MerchantHandler{uc: uc}
}

// Create registers a new merchant — no auth required.
func (h *MerchantHandler) Create(c echo.Context) error {
	var input usecase.CreateMerchantInput
	if err := c.Bind(&input); err != nil {
		return err
	}
	if err := c.Validate(&input); err != nil {
		return err
	}

	out, err := h.uc.Create(c.Request().Context(), input)
	if err != nil {
		return err
	}
	return response.Created(c, out)
}

// GetMerchant godoc
//
//	@Summary      Get merchant profile
//	@Description  Returns the authenticated merchant's profile, status, and configuration.
//	@Tags         Merchant
//	@Produce      json
//	@Security     ApiKeyAuth
//	@Success      200  {object}  response.SuccessResponse{data=usecase.MerchantOutput}
//	@Failure      401  {object}  response.ErrorResponse
//	@Router       /v1/merchants/me [get]
func (h *MerchantHandler) GetMerchant(c echo.Context) error {
	merchantID := c.Get(middleware.ContextKeyMerchantID).(string)

	out, err := h.uc.GetMerchant(c.Request().Context(), merchantID)
	if err != nil {
		return err
	}
	return response.OK(c, out)
}

// Update godoc
//
//	@Summary      Update merchant profile
//	@Description  Updates merchant name, webhook URL, or other profile fields. FeeConfig cannot be changed here — contact admin.
//	@Tags         Merchant
//	@Accept       json
//	@Produce      json
//	@Security     ApiKeyAuth
//	@Param        body  body      usecase.UpdateMerchantInput  true  "Fields to update"
//	@Success      200   {object}  response.SuccessResponse{data=usecase.MerchantOutput}
//	@Failure      400   {object}  response.ErrorResponse
//	@Failure      401   {object}  response.ErrorResponse
//	@Router       /v1/merchants/me [patch]
func (h *MerchantHandler) Update(c echo.Context) error {
	var input usecase.UpdateMerchantInput
	if err := c.Bind(&input); err != nil {
		return err
	}
	if err := c.Validate(&input); err != nil {
		return err
	}
	input.MerchantID = c.Get(middleware.ContextKeyMerchantID).(string)

	out, err := h.uc.Update(c.Request().Context(), input)
	if err != nil {
		return err
	}
	return response.OK(c, out)
}

// RegenerateAPIKey godoc
//
//	@Summary      Regenerate API key
//	@Description  Generates a new API key and immediately invalidates the old one. The raw key is shown only once — store it securely.
//	@Tags         Merchant
//	@Produce      json
//	@Security     ApiKeyAuth
//	@Success      200  {object}  response.SuccessResponse{data=object{api_key=string}}
//	@Failure      401  {object}  response.ErrorResponse
//	@Router       /v1/merchants/me/api-key/regenerate [post]
func (h *MerchantHandler) RegenerateAPIKey(c echo.Context) error {
	merchantID := c.Get(middleware.ContextKeyMerchantID).(string)

	rawKey, err := h.uc.RegenerateAPIKey(c.Request().Context(), merchantID)
	if err != nil {
		return err
	}
	return response.OK(c, map[string]string{"api_key": rawKey})
}

// ListBankAccounts godoc
//
//	@Summary      List bank accounts
//	@Description  Returns all registered bank accounts for the authenticated merchant (max 3).
//	@Tags         Merchant / Bank Accounts
//	@Produce      json
//	@Security     ApiKeyAuth
//	@Success      200  {object}  response.SuccessResponse{data=[]usecase.BankAccountOutput}
//	@Failure      401  {object}  response.ErrorResponse
//	@Router       /v1/merchants/me/bank-accounts [get]
func (h *MerchantHandler) ListBankAccounts(c echo.Context) error {
	merchantID := c.Get(middleware.ContextKeyMerchantID).(string)

	out, err := h.uc.ListBankAccounts(c.Request().Context(), merchantID)
	if err != nil {
		return err
	}
	return response.OK(c, out)
}

// AddBankAccount godoc
//
//	@Summary      Add bank account
//	@Description  Registers a new bank account for disbursement. Max 3 accounts per merchant. New accounts must be verified by admin before they can receive disbursements.
//	@Tags         Merchant / Bank Accounts
//	@Accept       json
//	@Produce      json
//	@Security     ApiKeyAuth
//	@Param        body  body      usecase.AddBankAccountInput  true  "Bank account details"
//	@Success      201   {object}  response.SuccessResponse{data=usecase.BankAccountOutput}
//	@Failure      400   {object}  response.ErrorResponse
//	@Failure      401   {object}  response.ErrorResponse
//	@Failure      422   {object}  response.ErrorResponse  "Maximum 3 bank accounts reached"
//	@Router       /v1/merchants/me/bank-accounts [post]
func (h *MerchantHandler) AddBankAccount(c echo.Context) error {
	var input usecase.AddBankAccountInput
	if err := c.Bind(&input); err != nil {
		return err
	}
	if err := c.Validate(&input); err != nil {
		return err
	}
	input.MerchantID = c.Get(middleware.ContextKeyMerchantID).(string)

	out, err := h.uc.AddBankAccount(c.Request().Context(), input)
	if err != nil {
		return err
	}
	return response.Created(c, out)
}

// RemoveBankAccount godoc
//
//	@Summary      Remove bank account
//	@Description  Removes a registered bank account. Cannot remove the primary account if other accounts exist.
//	@Tags         Merchant / Bank Accounts
//	@Produce      json
//	@Security     ApiKeyAuth
//	@Param        id   path  string  true  "Bank account ID (UUID)"
//	@Success      204  "Bank account removed"
//	@Failure      401  {object}  response.ErrorResponse
//	@Failure      404  {object}  response.ErrorResponse
//	@Router       /v1/merchants/me/bank-accounts/{id} [delete]
func (h *MerchantHandler) RemoveBankAccount(c echo.Context) error {
	merchantID := c.Get(middleware.ContextKeyMerchantID).(string)
	accountID := c.Param("id")

	if err := h.uc.RemoveBankAccount(c.Request().Context(), merchantID, accountID); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

// SetPrimaryBankAccount godoc
//
//	@Summary      Set primary bank account
//	@Description  Marks a bank account as the primary account for disbursements.
//	@Tags         Merchant / Bank Accounts
//	@Produce      json
//	@Security     ApiKeyAuth
//	@Param        id   path      string  true  "Bank account ID (UUID)"
//	@Success      200  {object}  response.SuccessResponse{data=object{message=string}}
//	@Failure      401  {object}  response.ErrorResponse
//	@Failure      404  {object}  response.ErrorResponse
//	@Router       /v1/merchants/me/bank-accounts/{id}/primary [patch]
func (h *MerchantHandler) SetPrimaryBankAccount(c echo.Context) error {
	merchantID := c.Get(middleware.ContextKeyMerchantID).(string)
	accountID := c.Param("id")

	if err := h.uc.SetPrimaryBankAccount(c.Request().Context(), merchantID, accountID); err != nil {
		return err
	}
	return response.OK(c, map[string]string{"message": "primary bank account updated"})
}

// ListWebhookEvents godoc
//
//	@Summary      List webhook delivery events
//	@Description  Returns outbound webhook delivery history for this merchant — useful for debugging failed deliveries.
//	@Tags         Merchant
//	@Produce      json
//	@Security     ApiKeyAuth
//	@Param        page   query     int  false  "Page number (default: 1)"
//	@Param        limit  query     int  false  "Items per page (default: 20, max: 100)"
//	@Success      200    {object}  response.ListResponse{data=[]usecase.WebhookEventOutput}
//	@Failure      401    {object}  response.ErrorResponse
//	@Router       /v1/merchants/me/webhook-events [get]
func (h *MerchantHandler) ListWebhookEvents(c echo.Context) error {
	merchantID := c.Get(middleware.ContextKeyMerchantID).(string)
	page, _ := strconv.Atoi(c.QueryParam("page"))
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 20
	}
	out, err := h.uc.ListWebhookEvents(c.Request().Context(), merchantID, page, limit)
	if err != nil {
		return err
	}
	totalPages := int(out.Total) / limit
	if int(out.Total)%limit != 0 {
		totalPages++
	}
	return response.List(c, out.Items, response.Pagination{
		Page:       out.Page,
		PerPage:    out.Limit,
		Total:      int(out.Total),
		TotalPages: totalPages,
	})
}
