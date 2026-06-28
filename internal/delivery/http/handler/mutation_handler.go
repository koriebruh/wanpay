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

// ListMutations godoc
//
//	@Summary      List ledger mutations
//	@Description  Returns paginated ledger mutations (credits and debits) for the authenticated merchant.
//	@Tags         Mutations (Ledger)
//	@Produce      json
//	@Security     ApiKeyAuth
//	@Param        page        query     int     false  "Page number (default: 1)"
//	@Param        limit       query     int     false  "Items per page (default: 20, max: 100)"
//	@Param        type        query     string  false  "Filter by type: credit, debit"
//	@Param        start_date  query     string  false  "Start date (RFC3339)"
//	@Param        end_date    query     string  false  "End date (RFC3339)"
//	@Success      200         {object}  response.ListResponse{data=[]usecase.MutationOutput}
//	@Failure      401         {object}  response.ErrorResponse
//	@Router       /v1/mutations [get]
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

// GetBalance godoc
//
//	@Summary      Get current balance
//	@Description  Returns the merchant's current spendable balance in IDR (calculated live from ledger).
//	@Tags         Mutations (Ledger)
//	@Produce      json
//	@Security     ApiKeyAuth
//	@Success      200  {object}  response.SuccessResponse{data=object{balance=integer}}
//	@Failure      401  {object}  response.ErrorResponse
//	@Router       /v1/mutations/balance [get]
func (h *MutationHandler) GetBalance(c echo.Context) error {
	merchantID := c.Get(middleware.ContextKeyMerchantID).(string)

	balance, err := h.uc.GetBalance(c.Request().Context(), merchantID)
	if err != nil {
		return err
	}
	return response.OK(c, map[string]int64{"balance": balance})
}

// GetMutation godoc
//
//	@Summary      Get mutation detail
//	@Description  Returns a single ledger mutation by ID.
//	@Tags         Mutations (Ledger)
//	@Produce      json
//	@Security     ApiKeyAuth
//	@Param        id   path      string  true  "Mutation ID (UUID)"
//	@Success      200  {object}  response.SuccessResponse{data=usecase.MutationOutput}
//	@Failure      401  {object}  response.ErrorResponse
//	@Failure      404  {object}  response.ErrorResponse
//	@Router       /v1/mutations/{id} [get]
func (h *MutationHandler) GetMutation(c echo.Context) error {
	merchantID := c.Get(middleware.ContextKeyMerchantID).(string)
	mutationID := c.Param("id")

	out, err := h.uc.GetMutation(c.Request().Context(), merchantID, mutationID)
	if err != nil {
		return err
	}
	return response.OK(c, out)
}
