//go:build e2e

package e2e

import (
	"context"
	"time"

	"wanpey/core/internal/domain/entity"
	"wanpey/core/internal/domain/gateway"
)

// fakePaymentGateway is a controllable PaymentGateway for E2E tests.
// It records calls and returns preset responses without hitting any external API.
type fakePaymentGateway struct {
	// Injectable responses
	vaResp      *gateway.CreateVAResponse
	vaErr       error
	qrisResp    *gateway.CreateQRISResponse
	qrisErr     error
	cancelErr   error
	statusResp  entity.PaymentStatus
	webhookResp *gateway.WebhookEvent
	webhookErr  error
}

func newFakeGateway() *fakePaymentGateway {
	now := time.Now().Add(24 * time.Hour)
	return &fakePaymentGateway{
		vaResp: &gateway.CreateVAResponse{
			ExternalID:        "fake-ext-id",
			ProviderPaymentID: "fake-provider-payment-id",
			VANumber:          "80081234567890",
			BankCode:          entity.BankBCA,
			Amount:            100000,
			ExpiryAt:          now,
		},
		qrisResp: &gateway.CreateQRISResponse{
			ExternalID:        "fake-qris-ext-id",
			ProviderPaymentID: "fake-qris-provider-id",
			QRString:          "00020101021226...",
			QRImageURL:        "https://fake.provider.com/qr.png",
			Amount:            50000,
			ExpiryAt:          now,
		},
		statusResp: entity.PaymentStatusPending,
	}
}

func (f *fakePaymentGateway) CreateVA(_ context.Context, req gateway.CreateVARequest) (*gateway.CreateVAResponse, error) {
	if f.vaErr != nil {
		return nil, f.vaErr
	}
	resp := *f.vaResp
	resp.ExternalID = req.ExternalID // mirror the caller's external ID
	resp.Amount = req.Amount
	resp.BankCode = req.BankCode
	resp.ExpiryAt = req.ExpiryAt
	return &resp, nil
}

func (f *fakePaymentGateway) CreateQRIS(_ context.Context, req gateway.CreateQRISRequest) (*gateway.CreateQRISResponse, error) {
	if f.qrisErr != nil {
		return nil, f.qrisErr
	}
	resp := *f.qrisResp
	resp.ExternalID = req.ExternalID
	resp.Amount = req.Amount
	resp.ExpiryAt = req.ExpiryAt
	return &resp, nil
}

func (f *fakePaymentGateway) CancelPayment(_ context.Context, _ string) error {
	return f.cancelErr
}

func (f *fakePaymentGateway) GetStatus(_ context.Context, _ string) (entity.PaymentStatus, error) {
	return f.statusResp, nil
}

// ParseWebhook returns a preset paid event without any signature verification.
// E2E tests set webhookResp to control what event is returned.
func (f *fakePaymentGateway) ParseWebhook(_ context.Context, _ map[string]string, _ []byte) (*gateway.WebhookEvent, error) {
	if f.webhookErr != nil {
		return nil, f.webhookErr
	}
	return f.webhookResp, nil
}

func (f *fakePaymentGateway) SupportedMethods() []entity.PaymentMethod {
	return []entity.PaymentMethod{entity.PaymentMethodVA, entity.PaymentMethodQRIS}
}

func (f *fakePaymentGateway) Capabilities() []gateway.ProviderCapability {
	return []gateway.ProviderCapability{gateway.CapabilityCashIn}
}

func (f *fakePaymentGateway) ProviderName() entity.Provider {
	return entity.ProviderMidtrans
}
