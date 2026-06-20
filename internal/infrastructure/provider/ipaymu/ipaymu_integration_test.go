//go:build integration

package ipaymu

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
	if !cfg.Provider.IPaymu.Enabled {
		t.Skip("provider.ipaymu.enabled = false in .config.toml — skipping integration test")
	}
	if cfg.Provider.IPaymu.APIKey == "" || cfg.Provider.IPaymu.VA == "" {
		t.Skip("provider.ipaymu.api_key or va not set — skipping integration test")
	}
	gw, err := New(cfg.Provider.IPaymu, zap.NewNop())
	if err != nil {
		t.Fatalf("new gateway: %v", err)
	}
	return gw.(*Gateway)
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
	t.Logf("BCA VA: %s  session: %s", resp.VANumber, resp.ProviderPaymentID)
}

func TestIntegration_CreateQRIS(t *testing.T) {
	g := newIntegrationGateway(t)

	resp, err := g.CreateQRIS(context.Background(), gateway.CreateQRISRequest{
		ExternalID:    refID(),
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
	if resp.QRString == "" && resp.QRImageURL == "" {
		t.Error("QRString and QRImageURL both empty")
	}
	t.Logf("QRIS session: %s  qr: %.30s...", resp.ProviderPaymentID, resp.QRString)
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
		CustomerPhone: "081234567890",
		Description:   "integration test",
		ExpiryAt:      time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("create VA: %v", err)
	}

	status, err := g.GetStatus(context.Background(), created.ProviderPaymentID)
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	t.Logf("session: %s  status: %s", created.ProviderPaymentID, status)
}
