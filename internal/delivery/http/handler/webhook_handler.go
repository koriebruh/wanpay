package handler

import (
	"io"
	"net/http"

	"github.com/labstack/echo/v4"

	"wanpey/core/internal/domain/entity"
	"wanpey/core/internal/usecase"
	"wanpey/core/pkg/apperror"
)

type WebhookHandler struct {
	paymentUC      usecase.PaymentUsecase
	disbursementUC usecase.DisbursementUsecase
}

func NewWebhookHandler(paymentUC usecase.PaymentUsecase, disbursementUC usecase.DisbursementUsecase) *WebhookHandler {
	return &WebhookHandler{
		paymentUC:      paymentUC,
		disbursementUC: disbursementUC,
	}
}

// HandlePaymentWebhook handles POST /webhooks/:provider/payment.
// No auth — each provider signs the payload; verification is inside ParseWebhook.
func (h *WebhookHandler) HandlePaymentWebhook(c echo.Context) error {
	provider := entity.Provider(c.Param("provider"))
	if !isKnownProvider(provider) {
		return apperror.NotFound("unknown provider: %s", provider)
	}

	body, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return apperror.BadRequest("failed to read body")
	}

	headers := extractHeaders(c.Request())
	if err := h.paymentUC.HandleWebhook(c.Request().Context(), provider, headers, body); err != nil {
		return err
	}
	return c.NoContent(http.StatusOK)
}

// HandleDisbursementWebhook handles POST /webhooks/:provider/disbursement.
func (h *WebhookHandler) HandleDisbursementWebhook(c echo.Context) error {
	provider := entity.Provider(c.Param("provider"))
	if !isKnownProvider(provider) {
		return apperror.NotFound("unknown provider: %s", provider)
	}

	body, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return apperror.BadRequest("failed to read body")
	}

	headers := extractHeaders(c.Request())
	if err := h.disbursementUC.HandleDisbursementCallback(c.Request().Context(), provider, headers, body); err != nil {
		return err
	}
	return c.NoContent(http.StatusOK)
}

// extractHeaders flattens the first value of each header into a plain map.
// Provider signature verification looks up specific header names (case-insensitive
// normalisation happens inside each adapter).
func extractHeaders(r *http.Request) map[string]string {
	out := make(map[string]string, len(r.Header))
	for k, v := range r.Header {
		if len(v) > 0 {
			out[k] = v[0]
		}
	}
	return out
}

func isKnownProvider(p entity.Provider) bool {
	switch p {
	case entity.ProviderMidtrans, entity.ProviderXendit, entity.ProviderDoku, entity.ProviderIPaymu:
		return true
	}
	return false
}
