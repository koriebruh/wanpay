package handler

import (
	"net/http"
	"strconv"

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

func (h *PaymentHandler) ListPayments(c echo.Context) error {
	merchantID := c.Get(middleware.ContextKeyMerchantID).(string)
	page, _ := strconv.Atoi(c.QueryParam("page"))
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	out, err := h.uc.ListPayments(c.Request().Context(), usecase.ListPaymentsInput{
		MerchantID: merchantID,
		Status:     c.QueryParam("status"),
		Provider:   c.QueryParam("provider"),
		Method:     c.QueryParam("method"),
		StartDate:  c.QueryParam("start_date"),
		EndDate:    c.QueryParam("end_date"),
		Page:       page,
		Limit:      limit,
	})
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
