//go:build integration
package xendit

import (
	"context"
	"fmt"
	"testing"
	"time"

	"go.uber.org/zap"

	"wanpey/core/internal/domain/entity"
	"wanpey/core/internal/domain/gateway"
	"wanpey/core/internal/infrastructure/config"
)

func newIntegrationGateway(t *testing.T) *Gateway {
	t.Helper()
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Provider.Xendit.SecretKey == "" {
		t.Skip("provider.xendit.secret_key not set in .config.toml — skipping integration test")
	}
	gw, err := New(cfg.Provider.Xendit, zap.NewNop())
	if err != nil {
		t.Fatalf("new gateway: %v", err)
	}
	return gw
}

func refID() string {
	return fmt.Sprintf("wanpey-test-%d", time.Now().UnixNano())
}

func TestIntegration_CreateVA_BCA(t *testing.T) {
	g := newIntegrationGateway(t)

	resp, err := g.CreateVA(context.Background(), gateway.CreateVARequest{
		ExternalID:    refID(),
		BankCode:      entity.BankBCA,
		Amount:        10000,
		Currency:      entity.CurrencyIDR,
		CustomerName:  "Test User",
		CustomerEmail: "test@example.com",
		CustomerPhone: "081234567890",
		Description:   "integration test",
		ExpiryAt:      time.Now().Add(24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("CreateVA BCA: %v", err)
	}
	if resp.VANumber == "" {
		t.Error("VANumber should not be empty")
	}
	if resp.ProviderPaymentID == "" {
		t.Error("ProviderPaymentID (payment_request_id) should not be empty")
	}
	t.Logf("BCA VA: %s  payment_request_id: %s", resp.VANumber, resp.ProviderPaymentID)
}

func TestIntegration_CreateQRIS(t *testing.T) {
	g := newIntegrationGateway(t)

	resp, err := g.CreateQRIS(context.Background(), gateway.CreateQRISRequest{
		ExternalID:    refID(),
		Amount:        10000,
		Currency:      entity.CurrencyIDR,
		CustomerName:  "Test User",
		CustomerEmail: "test@example.com",
		Description:   "integration test",
		ExpiryAt:      time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("CreateQRIS: %v", err)
	}
	if resp.QRString == "" {
		t.Error("QRString should not be empty")
	}
	if resp.ProviderPaymentID == "" {
		t.Error("ProviderPaymentID (payment_request_id) should not be empty")
	}
	t.Logf("QRIS string: %.50s...  payment_request_id: %s", resp.QRString, resp.ProviderPaymentID)
}

func TestIntegration_GetStatus(t *testing.T) {
	g := newIntegrationGateway(t)

	created, err := g.CreateVA(context.Background(), gateway.CreateVARequest{
		ExternalID:    refID(),
		BankCode:      entity.BankBCA,
		Amount:        10000,
		Currency:      entity.CurrencyIDR,
		CustomerName:  "Test User",
		CustomerEmail: "test@example.com",
		ExpiryAt:      time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("create VA: %v", err)
	}

	// GetStatus uses payment_request_id, not the merchant reference_id.
	status, err := g.GetStatus(context.Background(), created.ProviderPaymentID)
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if status != entity.PaymentStatusPending {
		t.Errorf("status = %q, want pending", status)
	}
	t.Logf("payment_request_id: %s  status: %s", created.ProviderPaymentID, status)
}

func TestIntegration_CancelPayment(t *testing.T) {
	g := newIntegrationGateway(t)

	created, err := g.CreateQRIS(context.Background(), gateway.CreateQRISRequest{
		ExternalID:   refID(),
		Amount:       10000,
		Currency:     entity.CurrencyIDR,
		CustomerName: "Test User",
		ExpiryAt:     time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("create QRIS: %v", err)
	}

	// CancelPayment uses payment_request_id, not the merchant reference_id.
	if err := g.CancelPayment(context.Background(), created.ProviderPaymentID); err != nil {
		t.Fatalf("CancelPayment: %v", err)
	}
	t.Logf("cancelled payment_request_id: %s", created.ProviderPaymentID)
}

func TestIntegration_Disburse(t *testing.T) {
	g := newIntegrationGateway(t)

	resp, err := g.Disburse(context.Background(), gateway.DisburseRequest{
		ExternalID:    refID(),
		BankCode:      entity.BankBCA,
		AccountNumber: "1234567890",
		AccountName:   "Test User",
		Amount:        10000,
		Currency:      entity.CurrencyIDR,
		Description:   "integration test disbursement",
	})
	if err != nil {
		t.Fatalf("Disburse: %v", err)
	}
	if resp.ExternalID == "" {
		t.Error("ExternalID should not be empty")
	}
	t.Logf("disbursement id: %s  status: %s", resp.ExternalID, resp.Status)
}
