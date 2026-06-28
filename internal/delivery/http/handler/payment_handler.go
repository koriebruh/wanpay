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

// CreateVA godoc
//
//	@Summary      Create Virtual Account payment
//	@Description  Creates a VA payment via the specified provider. Returns VA number and bank details for the customer to pay.
//	@Tags         Payments
//	@Accept       json
//	@Produce      json
//	@Security     ApiKeyAuth
//	@Param        Idempotency-Key  header    string                  false  "Unique key to prevent duplicate payments on retry"
//	@Param        body             body      usecase.CreateVAInput   true   "VA payment request"
//	@Success      201              {object}  response.SuccessResponse{data=usecase.PaymentOutput}
//	@Failure      400              {object}  response.ErrorResponse
//	@Failure      401              {object}  response.ErrorResponse
//	@Failure      422              {object}  response.ErrorResponse
//	@Router       /v1/payments/va [post]
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

// CreateQRIS godoc
//
//	@Summary      Create QRIS payment
//	@Description  Creates a QRIS payment and returns a QR code string for the customer to scan.
//	@Tags         Payments
//	@Accept       json
//	@Produce      json
//	@Security     ApiKeyAuth
//	@Param        Idempotency-Key  header    string                    false  "Unique key to prevent duplicate payments on retry"
//	@Param        body             body      usecase.CreateQRISInput   true   "QRIS payment request"
//	@Success      201              {object}  response.SuccessResponse{data=usecase.PaymentOutput}
//	@Failure      400              {object}  response.ErrorResponse
//	@Failure      401              {object}  response.ErrorResponse
//	@Failure      422              {object}  response.ErrorResponse
//	@Router       /v1/payments/qris [post]
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

// GetPayment godoc
//
//	@Summary      Get payment detail
//	@Description  Returns full payment detail including status, VA/QR data, and fee breakdown.
//	@Tags         Payments
//	@Produce      json
//	@Security     ApiKeyAuth
//	@Param        id   path      string  true  "Payment ID (UUID)"
//	@Success      200  {object}  response.SuccessResponse{data=usecase.PaymentOutput}
//	@Failure      401  {object}  response.ErrorResponse
//	@Failure      404  {object}  response.ErrorResponse
//	@Router       /v1/payments/{id} [get]
func (h *PaymentHandler) GetPayment(c echo.Context) error {
	merchantID := c.Get(middleware.ContextKeyMerchantID).(string)
	paymentID := c.Param("id")

	out, err := h.uc.GetPayment(c.Request().Context(), merchantID, paymentID)
	if err != nil {
		return err
	}
	return response.OK(c, out)
}

// ListPayments godoc
//
//	@Summary      List payments
//	@Description  Returns paginated list of payments for the authenticated merchant.
//	@Tags         Payments
//	@Produce      json
//	@Security     ApiKeyAuth
//	@Param        page        query     int     false  "Page number (default: 1)"
//	@Param        limit       query     int     false  "Items per page (default: 20, max: 100)"
//	@Param        status      query     string  false  "Filter by status: pending, paid, expired, cancelled, failed"
//	@Param        provider    query     string  false  "Filter by provider: midtrans, xendit, doku, ipaymu"
//	@Param        method      query     string  false  "Filter by method: va, qris"
//	@Param        start_date  query     string  false  "Start date (RFC3339)"
//	@Param        end_date    query     string  false  "End date (RFC3339)"
//	@Success      200         {object}  response.ListResponse{data=[]usecase.PaymentOutput}
//	@Failure      401         {object}  response.ErrorResponse
//	@Router       /v1/payments [get]
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

// CancelPayment godoc
//
//	@Summary      Cancel a payment
//	@Description  Cancels a pending payment. Uses a two-step process (pending → cancelling → cancelled) for financial safety. Only pending payments can be cancelled.
//	@Tags         Payments
//	@Produce      json
//	@Security     ApiKeyAuth
//	@Param        id   path  string  true  "Payment ID (UUID)"
//	@Success      204  "Payment cancelled successfully"
//	@Failure      401  {object}  response.ErrorResponse
//	@Failure      404  {object}  response.ErrorResponse
//	@Failure      422  {object}  response.ErrorResponse  "Payment is already in a final state"
//	@Router       /v1/payments/{id} [delete]
func (h *PaymentHandler) CancelPayment(c echo.Context) error {
	merchantID := c.Get(middleware.ContextKeyMerchantID).(string)
	paymentID := c.Param("id")

	if err := h.uc.CancelPayment(c.Request().Context(), merchantID, paymentID); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}
