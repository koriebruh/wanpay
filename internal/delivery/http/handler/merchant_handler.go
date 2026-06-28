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

func (h *MerchantHandler) GetMerchant(c echo.Context) error {
	merchantID := c.Get(middleware.ContextKeyMerchantID).(string)

	out, err := h.uc.GetMerchant(c.Request().Context(), merchantID)
	if err != nil {
		return err
	}
	return response.OK(c, out)
}

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

func (h *MerchantHandler) RegenerateAPIKey(c echo.Context) error {
	merchantID := c.Get(middleware.ContextKeyMerchantID).(string)

	rawKey, err := h.uc.RegenerateAPIKey(c.Request().Context(), merchantID)
	if err != nil {
		return err
	}
	return response.OK(c, map[string]string{"api_key": rawKey})
}

func (h *MerchantHandler) ListBankAccounts(c echo.Context) error {
	merchantID := c.Get(middleware.ContextKeyMerchantID).(string)

	out, err := h.uc.ListBankAccounts(c.Request().Context(), merchantID)
	if err != nil {
		return err
	}
	return response.OK(c, out)
}

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

func (h *MerchantHandler) RemoveBankAccount(c echo.Context) error {
	merchantID := c.Get(middleware.ContextKeyMerchantID).(string)
	accountID := c.Param("id")

	if err := h.uc.RemoveBankAccount(c.Request().Context(), merchantID, accountID); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *MerchantHandler) SetPrimaryBankAccount(c echo.Context) error {
	merchantID := c.Get(middleware.ContextKeyMerchantID).(string)
	accountID := c.Param("id")

	if err := h.uc.SetPrimaryBankAccount(c.Request().Context(), merchantID, accountID); err != nil {
		return err
	}
	return response.OK(c, map[string]string{"message": "primary bank account updated"})
}

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
