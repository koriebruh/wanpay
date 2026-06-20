//go:build !integration

package ipaymu

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/zap"

	"wanpey/core/internal/domain/entity"
	"wanpey/core/internal/domain/gateway"
	"wanpey/core/internal/infrastructure/config"
)

const (
	testAPIKey = "test-api-key"
	testVA     = "0000000000000"
)

func newTestGateway(t *testing.T, srv *httptest.Server) *Gateway {
	t.Helper()
	gw, err := New(config.IPaymuConfig{
		Enabled:   true,
		APIKey:    testAPIKey,
		VA:        testVA,
		NotifyURL: "https://example.com/notify",
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("new gateway: %v", err)
	}
	g := gw.(*Gateway)
	g.baseURL = srv.URL
	g.httpClient = srv.Client()
	return g
}

func TestMapStatus(t *testing.T) {
	cases := []struct {
		in   string
		want entity.PaymentStatus
	}{
		{"paid", entity.PaymentStatusPaid},
		{"settled", entity.PaymentStatusPaid},
		{"1", entity.PaymentStatusPaid},
		{"pending", entity.PaymentStatusPending},
		{"waiting", entity.PaymentStatusPending},
		{"0", entity.PaymentStatusPending},
		{"expired", entity.PaymentStatusExpired},
		{"2", entity.PaymentStatusExpired},
		{"cancelled", entity.PaymentStatusCancelled},
		{"3", entity.PaymentStatusCancelled},
		{"failed", entity.PaymentStatusFailed},
	}
	for _, tc := range cases {
		if got := mapStatus(tc.in); got != tc.want {
			t.Errorf("mapStatus(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestCreateVA_UnsupportedBank(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	g := newTestGateway(t, srv)
	_, err := g.CreateVA(context.Background(), gateway.CreateVARequest{
		BankCode: "UNKNOWN",
		Amount:   100000,
	})
	if err == nil {
		t.Error("expected error for unsupported bank code")
	}
}

func TestCreateVA(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/payment/direct" || r.Method != http.MethodPost {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("va") != testVA {
			t.Errorf("va header = %q, want %q", r.Header.Get("va"), testVA)
		}
		if r.Header.Get("signature") == "" {
			t.Error("signature header missing")
		}
		if r.Header.Get("timestamp") == "" {
			t.Error("timestamp header missing")
		}

		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body) //nolint:errcheck
		if body["paymentMethod"] != "va" {
			t.Errorf("paymentMethod = %v, want va", body["paymentMethod"])
		}
		if body["paymentChannel"] != "bca" {
			t.Errorf("paymentChannel = %v, want bca", body["paymentChannel"])
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(paymentResponse{ //nolint:errcheck
			Status:  200,
			Message: "SUCCESS",
			Data: struct {
				SessionID   string `json:"SessionId"`
				ReferenceID string `json:"ReferenceId"`
				PaymentNo   string `json:"PaymentNo"`
				PaymentName string `json:"PaymentName"`
				QrString    string `json:"QrString"`
				QrImage     string `json:"QrImage"`
				Expired     string `json:"Expired"`
			}{
				SessionID:   "session-bca-1",
				ReferenceID: "order-bca-1",
				PaymentNo:   "1234567890",
				PaymentName: "Bank BCA",
			},
		})
	}))
	defer srv.Close()

	g := newTestGateway(t, srv)
	resp, err := g.CreateVA(context.Background(), gateway.CreateVARequest{
		ExternalID:    "order-bca-1",
		BankCode:      entity.BankBCA,
		Amount:        100000,
		Currency:      entity.CurrencyIDR,
		CustomerName:  "John",
		CustomerEmail: "john@test.com",
		CustomerPhone: "081234567890",
		Description:   "test",
		ExpiryAt:      time.Now().Add(24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.VANumber != "1234567890" {
		t.Errorf("VANumber = %q, want 1234567890", resp.VANumber)
	}
	if resp.ProviderPaymentID != "session-bca-1" {
		t.Errorf("ProviderPaymentID = %q, want session-bca-1", resp.ProviderPaymentID)
	}
}

func TestCreateVA_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(paymentResponse{ //nolint:errcheck
			Status:  400,
			Message: "Bad Request",
		})
	}))
	defer srv.Close()

	g := newTestGateway(t, srv)
	_, err := g.CreateVA(context.Background(), gateway.CreateVARequest{
		BankCode: entity.BankBCA,
		Amount:   100000,
		ExpiryAt: time.Now().Add(time.Hour),
	})
	if err == nil {
		t.Error("expected error for non-200 status")
	}
}

func TestCreateQRIS(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body) //nolint:errcheck
		if body["paymentMethod"] != "qris" {
			t.Errorf("paymentMethod = %v, want qris", body["paymentMethod"])
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(paymentResponse{ //nolint:errcheck
			Status:  200,
			Message: "SUCCESS",
			Data: struct {
				SessionID   string `json:"SessionId"`
				ReferenceID string `json:"ReferenceId"`
				PaymentNo   string `json:"PaymentNo"`
				PaymentName string `json:"PaymentName"`
				QrString    string `json:"QrString"`
				QrImage     string `json:"QrImage"`
				Expired     string `json:"Expired"`
			}{
				SessionID: "session-qris-1",
				QrString:  "00020101021226...",
				QrImage:   "https://sandbox.ipaymu.com/qr/xxx.png",
			},
		})
	}))
	defer srv.Close()

	g := newTestGateway(t, srv)
	resp, err := g.CreateQRIS(context.Background(), gateway.CreateQRISRequest{
		ExternalID:   "order-qris-1",
		Amount:       50000,
		CustomerName: "Jane",
		ExpiryAt:     time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.QRString != "00020101021226..." {
		t.Errorf("QRString = %q", resp.QRString)
	}
}

func TestGetStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/transaction" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(transactionResponse{ //nolint:errcheck
			Status:  200,
			Message: "SUCCESS",
			Data: struct {
				TransactionID int64  `json:"TransactionId"`
				ReferenceID   string `json:"ReferenceId"`
				SessionID     string `json:"SessionId"`
				Status        int    `json:"Status"`
				PaidStatus    string `json:"PaidStatus"`
				Amount        int64  `json:"Amount"`
			}{
				PaidStatus: "paid",
				Amount:     100000,
			},
		})
	}))
	defer srv.Close()

	g := newTestGateway(t, srv)
	status, err := g.GetStatus(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != entity.PaymentStatusPaid {
		t.Errorf("status = %q, want paid", status)
	}
}

func TestCancelPayment_NotSupported(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	g := newTestGateway(t, srv)
	if err := g.CancelPayment(context.Background(), "session-1"); err == nil {
		t.Error("expected error — iPaymu cancel not supported")
	}
}

func TestParseWebhook(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	g := newTestGateway(t, srv)

	body, _ := json.Marshal(notification{ //nolint:errcheck
		ReferenceID: "order-1",
		StatusCode:  "1",
		Status:      "paid",
		Amount:      100000,
	})

	event, err := g.ParseWebhook(context.Background(), nil, body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if event.Status != entity.PaymentStatusPaid {
		t.Errorf("status = %q, want paid", event.Status)
	}
	if event.Amount != 100000 {
		t.Errorf("amount = %d, want 100000", event.Amount)
	}
}

func TestSignature(t *testing.T) {
	g := &Gateway{apiKey: testAPIKey, va: testVA}
	body := []byte(`{"test":"value"}`)
	sig := g.signature(body)
	if sig == "" {
		t.Error("signature should not be empty")
	}
	if sig != g.signature(body) {
		t.Error("signature not deterministic")
	}
}
