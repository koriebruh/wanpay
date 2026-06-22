package handler

import (
	"net/http"

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
