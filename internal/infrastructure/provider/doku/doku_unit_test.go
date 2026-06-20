//go:build !integration

package doku

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"wanpey/core/internal/domain/entity"
	"wanpey/core/internal/domain/gateway"
	"wanpey/core/internal/infrastructure/config"
)

const (
	testClientID  = "test-client-id"
	testSecretKey = "test-secret-key"
)

func testPrivateKeyPEM(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate test RSA key: %v", err)
	}
	b, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal test RSA key: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: b}))
}

func newTestGateway(t *testing.T, srv *httptest.Server) *Gateway {
	t.Helper()
	gw, err := New(config.DokuConfig{Enabled: true,
		ClientID:      testClientID,
		SecretKey:     testSecretKey,
		PrivateKeyPEM: testPrivateKeyPEM(t),
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("new gateway: %v", err)
	}
	gw.baseURL = srv.URL
	gw.httpClient = srv.Client()
	return gw
}

func tokenServer(handler http.HandlerFunc) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/authorization/v1/access-token/b2b" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
				"responseCode":    "2007300",
				"responseMessage": "Successful",
				"accessToken":     "test-access-token",
				"expiresIn":       900,
			})
			return
		}
		handler(w, r)
	}))
}

func TestMapStatus(t *testing.T) {
	cases := []struct {
		code string
		want entity.PaymentStatus
	}{
		{"00", entity.PaymentStatusPaid},
		{"2002700", entity.PaymentStatusPaid},
		{"03", entity.PaymentStatusPending},
		{"05", entity.PaymentStatusCancelled},
		{"06", entity.PaymentStatusFailed},
		{"99", entity.PaymentStatusFailed},
	}
	for _, tc := range cases {
		if got := mapStatus(tc.code); got != tc.want {
			t.Errorf("mapStatus(%q) = %q, want %q", tc.code, got, tc.want)
		}
	}
}

func TestMapDisbStatus(t *testing.T) {
	cases := []struct {
		code string
		want entity.DisbursementStatus
	}{
		{"2000000", entity.DisbursementStatusCompleted},
		{"00", entity.DisbursementStatusCompleted},
		{"2020000", entity.DisbursementStatusProcessing},
		{"99", entity.DisbursementStatusFailed},
	}
	for _, tc := range cases {
		if got := mapDisbStatus(tc.code); got != tc.want {
			t.Errorf("mapDisbStatus(%q) = %q, want %q", tc.code, got, tc.want)
		}
	}
}

func TestHmacSHA512(t *testing.T) {
	h := hmac.New(sha512.New, []byte("secret"))
	h.Write([]byte("data"))
	expected := base64.StdEncoding.EncodeToString(h.Sum(nil))
	if got := hmacSHA512("secret", "data"); got != expected {
		t.Errorf("hmacSHA512 = %q, want base64-encoded", got)
	}
}

func TestCreateVA(t *testing.T) {
	srv := tokenServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/virtual-accounts/bi-snap-va/v1.1/transfer-va/create-va" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("X-PARTNER-ID") != testClientID {
			t.Errorf("X-PARTNER-ID missing or wrong")
		}
		if r.Header.Get("X-SIGNATURE") == "" {
			t.Error("X-SIGNATURE header missing")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(vaCreateResponse{ //nolint:errcheck
			baseResponse: baseResponse{ResponseCode: "2002700"},
			VirtualAccountData: struct {
				VirtualAccountNo string `json:"virtualAccountNo"`
			}{VirtualAccountNo: "9999000123456789"},
		})
	})
	defer srv.Close()

	g := newTestGateway(t, srv)
	resp, err := g.CreateVA(context.Background(), gateway.CreateVARequest{
		ExternalID:    "order-1",
		BankCode:      entity.BankBCA,
		Amount:        100000,
		Currency:      entity.CurrencyIDR,
		CustomerName:  "John",
		CustomerEmail: "john@test.com",
		ExpiryAt:      time.Now().Add(24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.VANumber != "9999000123456789" {
		t.Errorf("VANumber = %q, want nested virtualAccountNo value", resp.VANumber)
	}
}

func TestCreateVA_ErrorResponse(t *testing.T) {
	srv := tokenServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(vaCreateResponse{ //nolint:errcheck
			baseResponse: baseResponse{
				ResponseCode:    "4002701",
				ResponseMessage: "Invalid Field Format",
			},
		})
	})
	defer srv.Close()

	g := newTestGateway(t, srv)
	_, err := g.CreateVA(context.Background(), gateway.CreateVARequest{
		ExternalID: "order-err",
		BankCode:   entity.BankBCA,
		Amount:     100000,
		ExpiryAt:   time.Now().Add(time.Hour),
	})
	if err == nil {
		t.Error("expected error for non-success response code")
	}
}

func TestCreateQRIS(t *testing.T) {
	srv := tokenServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/snap-adapter/b2b/v1.0/qr/qr-mpm-generate" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(qrCreateResponse{ //nolint:errcheck
			baseResponse: baseResponse{ResponseCode: "2002700"},
			ReferenceNo:  "order-qr-1",
			QRContent:    "00020101021226...",
		})
	})
	defer srv.Close()

	g := newTestGateway(t, srv)
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
}

func TestGetStatus(t *testing.T) {
	srv := tokenServer(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("GetStatus must use POST (not GET like Midtrans/Xendit), got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(vaInquiryResponse{ //nolint:errcheck
			baseResponse: baseResponse{ResponseCode: "03"},
		})
	})
	defer srv.Close()

	g := newTestGateway(t, srv)
	status, err := g.GetStatus(context.Background(), "order-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != entity.PaymentStatusPending {
		t.Errorf("status = %q, want pending", status)
	}
}

func TestParseWebhook_ValidSignature(t *testing.T) {
	srv := tokenServer(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	defer srv.Close()

	g := newTestGateway(t, srv)
	token, err := g.accessToken(context.Background())
	if err != nil {
		t.Fatalf("accessToken: %v", err)
	}

	body, _ := json.Marshal(vaNotification{ //nolint:errcheck
		ResponseCode: "00",
		TrxID:        "order-1",
		Amount:       100000,
	})

	ts := timestamp()
	method := "POST"
	path := "/notification/payment"
	bodyHash := strings.ToLower(hex.EncodeToString(func() []byte { h := sha256.Sum256(body); return h[:] }()))
	sig := hmacSHA512(testSecretKey, method+":"+path+":"+token+":"+bodyHash+":"+ts)

	event, err := g.ParseWebhook(context.Background(), map[string]string{
		"x-timestamp":    ts,
		"x-signature":    sig,
		"x-http-method":  method,
		"x-endpoint-url": path,
	}, body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if event.Status != entity.PaymentStatusPaid {
		t.Errorf("status = %q, want paid", event.Status)
	}
}

func TestParseWebhook_InvalidSignature(t *testing.T) {
	srv := tokenServer(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	defer srv.Close()

	g := newTestGateway(t, srv)
	if _, err := g.accessToken(context.Background()); err != nil {
		t.Fatalf("accessToken: %v", err)
	}

	body, _ := json.Marshal(vaNotification{ResponseCode: "00", TrxID: "order-1"}) //nolint:errcheck
	_, err := g.ParseWebhook(context.Background(), map[string]string{
		"x-timestamp":    timestamp(),
		"x-signature":    "invalidsig",
		"x-http-method":  "POST",
		"x-endpoint-url": "/notification/payment",
	}, body)
	if err == nil {
		t.Error("expected error for invalid signature")
	}
}

func TestDisburse(t *testing.T) {
	srv := tokenServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/snap/v1.1/emoney/transfer-bank" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(disbResponse{ //nolint:errcheck
			baseResponse:       baseResponse{ResponseCode: "2000000"},
			PartnerReferenceNo: "disb-ref-1",
		})
	})
	defer srv.Close()

	g := newTestGateway(t, srv)
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
	if resp.Status != entity.DisbursementStatusCompleted {
		t.Errorf("status = %q, want completed", resp.Status)
	}
}
