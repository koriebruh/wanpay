package handler

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"wanpey/core/internal/delivery/http/middleware"
	"wanpey/core/internal/usecase"
	"wanpey/core/pkg/response"
)

type PaymentHandler struct {
	uc usecase.PaymentUsecase
}

func NewPaymentHandler(uc usecase.PaymentUsecase) *PaymentHandler {
	return &PaymentHandler{uc: uc}
}

func (h *PaymentHandler) CreateVA(c echo.Context) error {
	var input usecase.CreateVAInput
	if err := c.Bind(&input); err != nil {
		return err
	}
	if err := c.Validate(&input); err != nil {
		return err
	}
	input.MerchantID = c.Get(middleware.ContextKeyMerchantID).(string)

	out, err := h.uc.CreateVA(c.Request().Context(), input)
	if err != nil {
		return err
	}
	return response.Created(c, out)
}

func (h *PaymentHandler) CreateQRIS(c echo.Context) error {
	var input usecase.CreateQRISInput
	if err := c.Bind(&input); err != nil {
		return err
	}
	if err := c.Validate(&input); err != nil {
		return err
	}
	input.MerchantID = c.Get(middleware.ContextKeyMerchantID).(string)

	out, err := h.uc.CreateQRIS(c.Request().Context(), input)
	if err != nil {
		return err
	}
	return response.Created(c, out)
}

func (h *PaymentHandler) GetPayment(c echo.Context) error {
	merchantID := c.Get(middleware.ContextKeyMerchantID).(string)
	paymentID := c.Param("id")

	out, err := h.uc.GetPayment(c.Request().Context(), merchantID, paymentID)
	if err != nil {
		return err
	}
	return response.OK(c, out)
}

func (h *PaymentHandler) CancelPayment(c echo.Context) error {
	merchantID := c.Get(middleware.ContextKeyMerchantID).(string)
	paymentID := c.Param("id")

	if err := h.uc.CancelPayment(c.Request().Context(), merchantID, paymentID); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}
