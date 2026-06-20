//go:build integration

package doku

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
	if cfg.Provider.Doku.ClientID == "" {
		t.Skip("provider.doku.client_id not set — skipping")
	}
	if cfg.Provider.Doku.PrivateKeyPEM == "" {
		t.Skip("provider.doku.private_key not set in .config.toml — skipping integration test")
	}
	gw, err := New(cfg.Provider.Doku, zap.NewNop())
	if err != nil {
		t.Fatalf("new gateway: %v", err)
	}
	return gw
}

func refID() string {
	return fmt.Sprintf("wanpey-test-%d", time.Now().UnixNano())
}

func TestIntegration_AccessToken(t *testing.T) {
	g := newIntegrationGateway(t)
	ts := timestamp()
	stringToSign := g.clientID + "|" + ts

	t.Logf("clientID     : %s", g.clientID)
	t.Logf("timestamp    : %s", ts)
	t.Logf("stringToSign : %s", stringToSign)
	t.Logf("algorithm    : SHA256withRSA (asymmetric)")

	sig, err := signRSASHA256(g.privateKey, stringToSign)
	if err != nil {
		t.Fatalf("signRSASHA256: %v", err)
	}
	t.Logf("signature (prefix): %.30s...", sig)

	token, err := g.accessToken(context.Background())
	if err != nil {
		t.Fatalf("accessToken: %v", err)
	}
	t.Logf("token obtained (len=%d)", len(token))
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
	t.Logf("BCA VA: %s", resp.VANumber)
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
	t.Logf("QRIS string: %.50s...", resp.QRString)
}

func TestIntegration_GetStatus(t *testing.T) {
	g := newIntegrationGateway(t)

	id := refID()
	_, err := g.CreateVA(context.Background(), gateway.CreateVARequest{
		ExternalID:    id,
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

	status, err := g.GetStatus(context.Background(), id)
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	t.Logf("status: %s", status)
}
