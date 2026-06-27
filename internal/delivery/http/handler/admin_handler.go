package handler

import (
	"github.com/labstack/echo/v4"

	"wanpey/core/internal/delivery/http/middleware"
	"wanpey/core/internal/usecase"
	"wanpey/core/pkg/response"
)

type AdminHandler struct {
	uc usecase.AdminUsecase
}

func NewAdminHandler(uc usecase.AdminUsecase) *AdminHandler {
	return &AdminHandler{uc: uc}
}

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

func (h *AdminHandler) SetMerchantFee(c echo.Context) error {
	var input usecase.SetMerchantFeeInput
	if err := c.Bind(&input); err != nil {
		return err
	}
	input.MerchantID = c.Param("id")
	if err := h.uc.SetMerchantFee(c.Request().Context(), input); err != nil {
		return err
	}
	return response.OK(c, map[string]string{"message": "merchant fee updated"})
}
