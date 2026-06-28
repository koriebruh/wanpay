package handler

import (
	"strconv"

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

// Disburse godoc
//
//	@Summary      Create disbursement (cash out)
//	@Description  Sends funds from merchant balance to a registered verified bank account. The bank account must be verified by admin before use. Daily cashout limit applies if configured.
//	@Tags         Disbursements
//	@Accept       json
//	@Produce      json
//	@Security     ApiKeyAuth
//	@Param        Idempotency-Key  header    string                   false  "Unique key to prevent duplicate disbursements on retry"
//	@Param        body             body      usecase.DisburseInput    true   "Disbursement request"
//	@Success      201              {object}  response.SuccessResponse{data=usecase.DisbursementOutput}
//	@Failure      400              {object}  response.ErrorResponse
//	@Failure      401              {object}  response.ErrorResponse
//	@Failure      402              {object}  response.ErrorResponse  "Insufficient balance"
//	@Failure      422              {object}  response.ErrorResponse  "Bank account not verified or daily limit exceeded"
//	@Router       /v1/disbursements [post]
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

// ListDisbursements godoc
//
//	@Summary      List disbursements
//	@Description  Returns paginated list of disbursements for the authenticated merchant.
//	@Tags         Disbursements
//	@Produce      json
//	@Security     ApiKeyAuth
//	@Param        page        query     int     false  "Page number (default: 1)"
//	@Param        limit       query     int     false  "Items per page (default: 20, max: 100)"
//	@Param        status      query     string  false  "Filter by status: pending, processing, completed, failed"
//	@Param        start_date  query     string  false  "Start date (RFC3339)"
//	@Param        end_date    query     string  false  "End date (RFC3339)"
//	@Success      200         {object}  response.ListResponse{data=[]usecase.DisbursementOutput}
//	@Failure      401         {object}  response.ErrorResponse
//	@Router       /v1/disbursements [get]
func (h *DisbursementHandler) ListDisbursements(c echo.Context) error {
	merchantID := c.Get(middleware.ContextKeyMerchantID).(string)
	page, _ := strconv.Atoi(c.QueryParam("page"))
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	out, err := h.uc.ListDisbursements(c.Request().Context(), usecase.ListDisbursementsInput{
		MerchantID: merchantID,
		Status:     c.QueryParam("status"),
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

// GetDisbursement godoc
//
//	@Summary      Get disbursement detail
//	@Description  Returns full disbursement detail including status, bank account info, and fee breakdown.
//	@Tags         Disbursements
//	@Produce      json
//	@Security     ApiKeyAuth
//	@Param        id   path      string  true  "Disbursement ID (UUID)"
//	@Success      200  {object}  response.SuccessResponse{data=usecase.DisbursementOutput}
//	@Failure      401  {object}  response.ErrorResponse
//	@Failure      404  {object}  response.ErrorResponse
//	@Router       /v1/disbursements/{id} [get]
func (h *DisbursementHandler) GetDisbursement(c echo.Context) error {
	merchantID := c.Get(middleware.ContextKeyMerchantID).(string)
	disbursementID := c.Param("id")

	out, err := h.uc.GetDisbursement(c.Request().Context(), merchantID, disbursementID)
	if err != nil {
		return err
	}
	return response.OK(c, out)
}
