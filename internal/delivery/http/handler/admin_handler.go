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

func (h *AdminHandler) GetMe(c echo.Context) error {
	adminID := c.Get(middleware.ContextKeyAdminID).(string)
	out, err := h.uc.GetMe(c.Request().Context(), adminID)
	if err != nil {
		return err
	}
	return response.OK(c, out)
}

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

func (h *AdminHandler) GetMerchant(c echo.Context) error {
	out, err := h.uc.GetMerchant(c.Request().Context(), c.Param("id"))
	if err != nil {
		return err
	}
	return response.OK(c, out)
}

func (h *AdminHandler) ApproveMerchant(c echo.Context) error {
	if err := h.uc.ApproveMerchant(c.Request().Context(), c.Param("id")); err != nil {
		return err
	}
	return response.OK(c, map[string]string{"message": "merchant approved"})
}

func (h *AdminHandler) SuspendMerchant(c echo.Context) error {
	if err := h.uc.SuspendMerchant(c.Request().Context(), c.Param("id")); err != nil {
		return err
	}
	return response.OK(c, map[string]string{"message": "merchant suspended"})
}

func (h *AdminHandler) DeactivateMerchant(c echo.Context) error {
	if err := h.uc.DeactivateMerchant(c.Request().Context(), c.Param("id")); err != nil {
		return err
	}
	return response.OK(c, map[string]string{"message": "merchant deactivated"})
}

func (h *AdminHandler) DeleteMerchant(c echo.Context) error {
	if err := h.uc.DeleteMerchant(c.Request().Context(), c.Param("id")); err != nil {
		return err
	}
	return response.NoContent(c)
}

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

func (h *AdminHandler) RegenerateAPIKey(c echo.Context) error {
	rawKey, err := h.uc.RegenerateMerchantAPIKey(c.Request().Context(), c.Param("id"))
	if err != nil {
		return err
	}
	return response.OK(c, map[string]string{"api_key": rawKey})
}

// ── Bank accounts ─────────────────────────────────────────────────────────────

func (h *AdminHandler) ListMerchantBankAccounts(c echo.Context) error {
	out, err := h.uc.ListMerchantBankAccounts(c.Request().Context(), c.Param("id"))
	if err != nil {
		return err
	}
	return response.OK(c, out)
}

func (h *AdminHandler) VerifyBankAccount(c echo.Context) error {
	if err := h.uc.VerifyBankAccount(c.Request().Context(), c.Param("id"), c.Param("aid")); err != nil {
		return err
	}
	return response.OK(c, map[string]string{"message": "bank account verified"})
}

// ── Payments ──────────────────────────────────────────────────────────────────

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

func (h *AdminHandler) GetPayment(c echo.Context) error {
	out, err := h.uc.GetPayment(c.Request().Context(), c.Param("id"))
	if err != nil {
		return err
	}
	return response.OK(c, out)
}

// ── Disbursements ─────────────────────────────────────────────────────────────

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

func (h *AdminHandler) GetDisbursement(c echo.Context) error {
	out, err := h.uc.GetDisbursement(c.Request().Context(), c.Param("id"))
	if err != nil {
		return err
	}
	return response.OK(c, out)
}

// ── Mutations ─────────────────────────────────────────────────────────────────

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

func (h *AdminHandler) GetProviderBalances(c echo.Context) error {
	out, err := h.uc.GetProviderBalances(c.Request().Context())
	if err != nil {
		return err
	}
	return response.OK(c, out)
}

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

func (h *AdminHandler) DeactivateAdmin(c echo.Context) error {
	callerID := c.Get(middleware.ContextKeyAdminID).(string)
	if err := h.uc.DeactivateAdmin(c.Request().Context(), callerID, c.Param("id")); err != nil {
		return err
	}
	return response.OK(c, map[string]string{"message": "admin deactivated"})
}

// ── Fee management ────────────────────────────────────────────────────────────

func (h *AdminHandler) GetFeeDefault(c echo.Context) error {
	out, err := h.uc.GetFeeDefault(c.Request().Context())
	if err != nil {
		return err
	}
	return response.OK(c, out)
}

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

func (h *AdminHandler) GetPlatformMargin(c echo.Context) error {
	out, err := h.uc.GetPlatformMargin(c.Request().Context())
	if err != nil {
		return err
	}
	return response.OK(c, out)
}

func (h *AdminHandler) UpdatePlatformMargin(c echo.Context) error {
	adminID := c.Get(middleware.ContextKeyAdminID).(string)
	var body struct {
		Enabled   bool             `json:"enabled"`
		Margin    entity.FeeConfig `json:"margin" validate:"required"`
		Reason    string           `json:"reason" validate:"required,min=10"`
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
