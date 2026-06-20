//go:build !integration

package xendit

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

const testSecretKey = "xnd_development_testkey"
const testWebhookToken = "test-webhook-token"

func newTestGateway(baseURL string) *Gateway {
	gw, _ := New(config.XenditConfig{Enabled: true,
		SecretKey:    testSecretKey,
		WebhookToken: testWebhookToken,
	}, zap.NewNop())
	gw.httpClient = &http.Client{Timeout: 5 * time.Second}
	_ = baseURL
	return gw
}

func newTestGatewayWithURL(t *testing.T, srv *httptest.Server) *Gateway {
	t.Helper()
	gw, err := New(config.XenditConfig{Enabled: true,
		SecretKey:    testSecretKey,
		WebhookToken: testWebhookToken,
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("new gateway: %v", err)
	}
	gw.httpClient = srv.Client()
	gw.httpClient.Transport = &rewriteTransport{base: srv.URL, inner: srv.Client().Transport}
	return gw
}

type rewriteTransport struct {
	base  string
	inner http.RoundTripper
}

func (rt *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.URL.Scheme = "http"
	req.URL.Host = rt.base[len("http://"):]
	return rt.inner.RoundTrip(req)
}

func TestMapPaymentStatus(t *testing.T) {
	cases := []struct {
		in   string
		want entity.PaymentStatus
	}{
		{"SUCCEEDED", entity.PaymentStatusPaid},
		{"REQUIRES_ACTION", entity.PaymentStatusPending},
		{"ACCEPTING_PAYMENTS", entity.PaymentStatusPending},
		{"AUTHORIZED", entity.PaymentStatusPending},
		{"EXPIRED", entity.PaymentStatusExpired},
		{"CANCELED", entity.PaymentStatusCancelled},
		{"FAILED", entity.PaymentStatusFailed},
		{"UNKNOWN", entity.PaymentStatusFailed},
	}
	for _, tc := range cases {
		if got := mapPaymentStatus(tc.in); got != tc.want {
			t.Errorf("mapPaymentStatus(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestMapDisbStatus(t *testing.T) {
	cases := []struct {
		in   string
		want entity.DisbursementStatus
	}{
		{"ACCEPTED", entity.DisbursementStatusProcessing},
		{"REQUESTED", entity.DisbursementStatusProcessing},
		{"SUCCEEDED", entity.DisbursementStatusCompleted},
		{"FAILED", entity.DisbursementStatusFailed},
		{"CANCELLED", entity.DisbursementStatusFailed},
		{"REVERSED", entity.DisbursementStatusFailed},
	}
	for _, tc := range cases {
		if got := mapDisbStatus(tc.in); got != tc.want {
			t.Errorf("mapDisbStatus(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestCreateVA(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/payment_requests" || r.Method != http.MethodPost {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("api-version") != apiVersion {
			t.Errorf("api-version header = %q, want %q", r.Header.Get("api-version"), apiVersion)
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body) //nolint:errcheck
		if body["channel_code"] != "BCA_VIRTUAL_ACCOUNT" {
			t.Errorf("channel_code = %v, want BCA_VIRTUAL_ACCOUNT", body["channel_code"])
		}
		if body["country"] != "ID" {
			t.Errorf("country = %v, want ID", body["country"])
		}
		if body["currency"] != "IDR" {
			t.Errorf("currency = %v, want IDR", body["currency"])
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(paymentRequestResponse{ //nolint:errcheck
			PaymentRequestID: "pr-bca-1",
			ReferenceID:      "order-bca-1",
			Status:           "REQUIRES_ACTION",
			Actions: []paymentRequestAction{
				{Type: "PRESENT_TO_CUSTOMER", Descriptor: "VIRTUAL_ACCOUNT_NUMBER", Value: "8801234567890"},
			},
		})
	}))
	defer srv.Close()

	g := newTestGatewayWithURL(t, srv)
	resp, err := g.CreateVA(context.Background(), gateway.CreateVARequest{
		ExternalID:   "order-bca-1",
		BankCode:     entity.BankBCA,
		Amount:       100000,
		Currency:     entity.CurrencyIDR,
		CustomerName: "John",
		ExpiryAt:     time.Now().Add(24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.VANumber != "8801234567890" {
		t.Errorf("VANumber = %q, want 8801234567890", resp.VANumber)
	}
	if resp.ProviderPaymentID != "pr-bca-1" {
		t.Errorf("ProviderPaymentID = %q, want pr-bca-1", resp.ProviderPaymentID)
	}
}

func TestCreateVA_UnsupportedBank(t *testing.T) {
	g := newTestGateway("")
	_, err := g.CreateVA(context.Background(), gateway.CreateVARequest{
		BankCode: "UNKNOWN",
		Amount:   100000,
	})
	if err == nil {
		t.Error("expected error for unsupported bank code")
	}
}

func TestCreateQRIS(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/payment_requests" || r.Method != http.MethodPost {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("api-version") != apiVersion {
			t.Errorf("api-version header = %q, want %q", r.Header.Get("api-version"), apiVersion)
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body) //nolint:errcheck
		if body["channel_code"] != "QRIS" {
			t.Errorf("channel_code = %v, want QRIS", body["channel_code"])
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(paymentRequestResponse{ //nolint:errcheck
			PaymentRequestID: "pr-qris-1",
			ReferenceID:      "order-qr-1",
			Status:           "REQUIRES_ACTION",
			Actions: []paymentRequestAction{
				{Type: "PRESENT_TO_CUSTOMER", Descriptor: "QR_STRING", Value: "00020101021226..."},
			},
		})
	}))
	defer srv.Close()

	g := newTestGatewayWithURL(t, srv)
	resp, err := g.CreateQRIS(context.Background(), gateway.CreateQRISRequest{
		ExternalID: "order-qr-1",
		Amount:     50000,
		ExpiryAt:   time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.QRString != "00020101021226..." {
		t.Errorf("QRString = %q", resp.QRString)
	}
	if resp.ProviderPaymentID != "pr-qris-1" {
		t.Errorf("ProviderPaymentID = %q, want pr-qris-1", resp.ProviderPaymentID)
	}
}

func TestCancelPayment(t *testing.T) {
	const prID = "pr-test-cancel-1"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		want := "/v3/payment_requests/" + prID + "/cancel"
		if r.URL.Path != want || r.Method != http.MethodPost {
			t.Errorf("unexpected %s %s, want POST %s", r.Method, r.URL.Path, want)
		}
		if r.Header.Get("api-version") != apiVersion {
			t.Errorf("api-version header missing")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(paymentRequestResponse{Status: "CANCELED"}) //nolint:errcheck
	}))
	defer srv.Close()

	g := newTestGatewayWithURL(t, srv)
	if err := g.CancelPayment(context.Background(), prID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetStatus(t *testing.T) {
	const prID = "pr-test-status-1"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		want := "/v3/payment_requests/" + prID
		if r.URL.Path != want || r.Method != http.MethodGet {
			t.Errorf("unexpected %s %s, want GET %s", r.Method, r.URL.Path, want)
		}
		if r.Header.Get("api-version") != apiVersion {
			t.Errorf("api-version header missing")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(paymentRequestResponse{ //nolint:errcheck
			PaymentRequestID: prID,
			Status:           "REQUIRES_ACTION",
		})
	}))
	defer srv.Close()

	g := newTestGatewayWithURL(t, srv)
	status, err := g.GetStatus(context.Background(), prID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != entity.PaymentStatusPending {
		t.Errorf("status = %q, want pending", status)
	}
}

func TestParseWebhook_ValidToken(t *testing.T) {
	g := newTestGateway("")

	body, _ := json.Marshal(paymentNotification{ //nolint:errcheck
		Event: "payment.succeeded",
		Data: struct {
			PaymentRequestID string `json:"payment_request_id"`
			ReferenceID      string `json:"reference_id"`
			Status           string `json:"status"`
			Amount           int64  `json:"amount"`
			ChannelCode      string `json:"channel_code"`
		}{
			PaymentRequestID: "pr-1",
			ReferenceID:      "order-1",
			Status:           "SUCCEEDED",
			Amount:           100000,
			ChannelCode:      "BCA_VIRTUAL_ACCOUNT",
		},
	})

	event, err := g.ParseWebhook(context.Background(), map[string]string{
		"x-callback-token": testWebhookToken,
	}, body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if event.Status != entity.PaymentStatusPaid {
		t.Errorf("status = %q, want paid", event.Status)
	}
	if event.Amount != 100000 {
		t.Errorf("amount = %d, want 100000", event.Amount)
	}
	if event.ExternalID != "order-1" {
		t.Errorf("ExternalID = %q, want order-1", event.ExternalID)
	}
}

func TestParseWebhook_InvalidToken(t *testing.T) {
	g := newTestGateway("")

	body, _ := json.Marshal(paymentNotification{Event: "payment.succeeded"}) //nolint:errcheck
	_, err := g.ParseWebhook(context.Background(), map[string]string{
		"x-callback-token": "wrong-token",
	}, body)
	if err == nil {
		t.Error("expected error for invalid token")
	}
}

func TestDisburse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/payouts" || r.Method != http.MethodPost {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Idempotency-key") == "" {
			t.Error("Idempotency-key header missing")
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body) //nolint:errcheck
		if body["channel_code"] != "ID_BCA" {
			t.Errorf("channel_code = %v, want ID_BCA", body["channel_code"])
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(disbResponse{ //nolint:errcheck
			ID:     "disb-id-1",
			Status: "ACCEPTED",
			Amount: 500000,
		})
	}))
	defer srv.Close()

	g := newTestGatewayWithURL(t, srv)
	resp, err := g.Disburse(context.Background(), gateway.DisburseRequest{
		ExternalID:    "disb-ref-1",
		BankCode:      entity.BankBCA,
		AccountNumber: "1234567890",
		AccountName:   "John Doe",
		Amount:        500000,
		Currency:      entity.CurrencyIDR,
		Description:   "withdrawal",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != entity.DisbursementStatusProcessing {
		t.Errorf("status = %q, want processing", resp.Status)
	}
}

func TestDisburse_UnsupportedBank(t *testing.T) {
	g := newTestGateway("")
	_, err := g.Disburse(context.Background(), gateway.DisburseRequest{
		BankCode: "UNKNOWN",
		Amount:   100000,
	})
	if err == nil {
		t.Error("expected error for unsupported bank code")
	}
}

func TestParseDisbursementWebhook(t *testing.T) {
	g := newTestGateway("")

	body, _ := json.Marshal(disbNotification{ //nolint:errcheck
		Event: "payout.succeeded",
		Data: struct {
			ID          string `json:"id"`
			ReferenceID string `json:"reference_id"`
			Status      string `json:"status"`
			Amount      int64  `json:"amount"`
			FailureCode string `json:"failure_code"`
		}{
			ID:     "disb-id-1",
			Status: "SUCCEEDED",
			Amount: 500000,
		},
	})

	event, err := g.ParseDisbursementWebhook(context.Background(), map[string]string{
		"x-callback-token": testWebhookToken,
	}, body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if event.Status != entity.DisbursementStatusCompleted {
		t.Errorf("status = %q, want completed", event.Status)
	}
}
