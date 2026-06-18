//go:build integration
package midtrans

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
	if cfg.Provider.Midtrans.ServerKey == "" {
		t.Skip("provider.midtrans.server_key not set in .config.toml — skipping integration test")
	}

	gw := New(cfg.Provider.Midtrans, zap.NewNop())
	return gw.(*Gateway)
}

func orderID() string {
	return fmt.Sprintf("wanpey-test-%d", time.Now().UnixNano())
}

func TestIntegration_CreateVA_BCA(t *testing.T) {
	g := newIntegrationGateway(t)

	resp, err := g.CreateVA(context.Background(), gateway.CreateVARequest{
		ExternalID:    orderID(),
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
	if resp.ExternalID == "" {
		t.Error("ExternalID should not be empty")
	}
	t.Logf("BCA VA: %s (expiry: %s)", resp.VANumber, resp.ExpiryAt.Format(time.RFC3339))
}

func TestIntegration_CreateVA_Mandiri(t *testing.T) {
	g := newIntegrationGateway(t)

	resp, err := g.CreateVA(context.Background(), gateway.CreateVARequest{
		ExternalID:    orderID(),
		BankCode:      entity.BankMandiri,
		Amount:        10000,
		Currency:      entity.CurrencyIDR,
		CustomerName:  "Test User",
		CustomerEmail: "test@example.com",
		CustomerPhone: "081234567890",
		Description:   "integration test",
		ExpiryAt:      time.Now().Add(24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("CreateVA Mandiri: %v", err)
	}
	if resp.VANumber == "" {
		t.Error("bill_key (VANumber) should not be empty")
	}
	if resp.BillerCode != "70012" {
		t.Errorf("BillerCode = %q, want 70012", resp.BillerCode)
	}
	t.Logf("Mandiri bill_key: %s biller_code: %s", resp.VANumber, resp.BillerCode)
}

func TestIntegration_CreateQRIS(t *testing.T) {
	g := newIntegrationGateway(t)

	resp, err := g.CreateQRIS(context.Background(), gateway.CreateQRISRequest{
		ExternalID:    orderID(),
		Amount:        10000,
		Currency:      entity.CurrencyIDR,
		CustomerName:  "Test User",
		CustomerEmail: "test@example.com",
		CustomerPhone: "081234567890",
		Description:   "integration test",
		ExpiryAt:      time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("CreateQRIS: %v", err)
	}
	if resp.QRImageURL == "" {
		t.Error("QRImageURL should not be empty")
	}
	t.Logf("QRIS image URL: %s", resp.QRImageURL)
	t.Logf("QRIS string: %s", resp.QRString)
}

func TestIntegration_GetStatus_Pending(t *testing.T) {
	g := newIntegrationGateway(t)

	// create a VA first, then immediately check status — should be pending
	id := orderID()
	_, err := g.CreateVA(context.Background(), gateway.CreateVARequest{
		ExternalID:    id,
		BankCode:      entity.BankBCA,
		Amount:        10000,
		Currency:      entity.CurrencyIDR,
		CustomerName:  "Test User",
		CustomerEmail: "test@example.com",
		CustomerPhone: "081234567890",
		ExpiryAt:      time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("create VA: %v", err)
	}

	status, err := g.GetStatus(context.Background(), id)
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if status != entity.PaymentStatusPending {
		t.Errorf("status = %q, want pending", status)
	}
	t.Logf("status: %s", status)
}
