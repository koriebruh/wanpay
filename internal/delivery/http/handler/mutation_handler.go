package handler

import (
	"strconv"
	"time"

	"github.com/labstack/echo/v4"

	"wanpey/core/internal/delivery/http/middleware"
	"wanpey/core/internal/domain/entity"
	"wanpey/core/internal/usecase"
	"wanpey/core/pkg/response"
)

type MutationHandler struct {
	uc usecase.MutationUsecase
}

func NewMutationHandler(uc usecase.MutationUsecase) *MutationHandler {
	return &MutationHandler{uc: uc}
}

func (h *MutationHandler) ListMutations(c echo.Context) error {
	merchantID := c.Get(middleware.ContextKeyMerchantID).(string)

	input := usecase.ListMutationsInput{
		MerchantID: merchantID,
		Page:       1,
		Limit:      20,
	}

	if p := c.QueryParam("page"); p != "" {
		if v, err := strconv.Atoi(p); err == nil && v > 0 {
			input.Page = v
		}
	}
	if l := c.QueryParam("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 && v <= 100 {
			input.Limit = v
		}
	}
	if t := c.QueryParam("type"); t != "" {
		mt := entity.MutationType(t)
		input.Type = &mt
	}
	if s := c.QueryParam("start_date"); s != "" {
		if ts, err := time.Parse(time.RFC3339, s); err == nil {
			input.StartDate = &ts
		}
	}
	if e := c.QueryParam("end_date"); e != "" {
		if ts, err := time.Parse(time.RFC3339, e); err == nil {
			input.EndDate = &ts
		}
	}

	out, err := h.uc.ListMutations(c.Request().Context(), input)
	if err != nil {
		return err
	}

	totalPages := int(out.Total) / input.Limit
	if int(out.Total)%input.Limit != 0 {
		totalPages++
	}
	return response.List(c, out.Items, response.Pagination{
		Page:       input.Page,
		PerPage:    input.Limit,
		Total:      int(out.Total),
		TotalPages: totalPages,
	})
}

func (h *MutationHandler) GetBalance(c echo.Context) error {
	merchantID := c.Get(middleware.ContextKeyMerchantID).(string)

	balance, err := h.uc.GetBalance(c.Request().Context(), merchantID)
	if err != nil {
		return err
	}
	return response.OK(c, map[string]int64{"balance": balance})
}

func (h *MutationHandler) GetMutation(c echo.Context) error {
	merchantID := c.Get(middleware.ContextKeyMerchantID).(string)
	mutationID := c.Param("id")

	out, err := h.uc.GetMutation(c.Request().Context(), merchantID, mutationID)
	if err != nil {
		return err
	}
	return response.OK(c, out)
}
