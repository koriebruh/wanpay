package handler

import (
	"github.com/labstack/echo/v4"

	"wanpey/core/internal/delivery/http/middleware"
	"wanpey/core/internal/usecase"
	"wanpey/core/pkg/response"
)

type DisbursementHandler struct {
	uc usecase.DisbursementUsecase
}

func NewDisbursementHandler(uc usecase.DisbursementUsecase) *DisbursementHandler {
	return &DisbursementHandler{uc: uc}
}

func (h *DisbursementHandler) Disburse(c echo.Context) error {
	var input usecase.DisburseInput
	if err := c.Bind(&input); err != nil {
		return err
	}
	if err := c.Validate(&input); err != nil {
		return err
	}
	input.MerchantID = c.Get(middleware.ContextKeyMerchantID).(string)

	out, err := h.uc.Disburse(c.Request().Context(), input)
	if err != nil {
		return err
	}
	return response.Created(c, out)
}

func (h *DisbursementHandler) GetDisbursement(c echo.Context) error {
	merchantID := c.Get(middleware.ContextKeyMerchantID).(string)
	disbursementID := c.Param("id")

	out, err := h.uc.GetDisbursement(c.Request().Context(), merchantID, disbursementID)
	if err != nil {
		return err
	}
	return response.OK(c, out)
}
