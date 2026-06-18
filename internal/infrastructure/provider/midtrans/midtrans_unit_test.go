//go:build !integration

package midtrans

import (
	"context"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/zap"

	"wanpey/core/internal/domain/entity"
	"wanpey/core/internal/domain/gateway"
)

const testServerKey = "test-server-key"

func newTestGateway(baseURL string) *Gateway {
	return &Gateway{
		serverKey:  testServerKey,
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 5 * time.Second},
		log:        zap.NewNop(),
	}
}


func TestMapStatus(t *testing.T) {
	tests := []struct {
		txStatus    string
		fraudStatus string
		want        entity.PaymentStatus
	}{
		{"settlement", "accept", entity.PaymentStatusPaid},
		{"capture", "accept", entity.PaymentStatusPaid},
		{"settlement", "", entity.PaymentStatusPaid},
		{"settlement", "deny", entity.PaymentStatusFailed},
		{"pending", "", entity.PaymentStatusPending},
		{"expire", "", entity.PaymentStatusExpired},
		{"cancel", "", entity.PaymentStatusCancelled},
		{"deny", "", entity.PaymentStatusFailed},
		{"failure", "", entity.PaymentStatusFailed},
	}
	for _, tc := range tests {
		got := mapStatus(tc.txStatus, tc.fraudStatus)
		if got != tc.want {
			t.Errorf("mapStatus(%q, %q) = %q, want %q", tc.txStatus, tc.fraudStatus, got, tc.want)
		}
	}
}


func TestVerifySignature(t *testing.T) {
	g := newTestGateway("")

	orderID := "order-123"
	statusCode := "200"
	grossAmount := "100000.00"

	h := sha512.New()
	h.Write([]byte(orderID + statusCode + grossAmount + testServerKey))
	validSig := hex.EncodeToString(h.Sum(nil))

	if !g.verifySignature(orderID, statusCode, grossAmount, validSig) {
		t.Error("expected valid signature to pass")
	}
	if g.verifySignature(orderID, statusCode, grossAmount, "invalidsig") {
		t.Error("expected invalid signature to fail")
	}
}


func TestExtractVANumber(t *testing.T) {
	g := newTestGateway("")

	bcaResp := chargeResponse{VANumbers: []vaNum{{Bank: "bca", VANumber: "8001234567"}}}
	va, biller := g.extractVANumber(bcaResp, entity.BankBCA)
	if va != "8001234567" || biller != "" {
		t.Errorf("BCA: got va=%q biller=%q", va, biller)
	}

	permataResp := chargeResponse{PermataVANumber: "8531234567"}
	va, biller = g.extractVANumber(permataResp, entity.BankPermata)
	if va != "8531234567" || biller != "" {
		t.Errorf("Permata: got va=%q biller=%q", va, biller)
	}

	mandiriResp := chargeResponse{BillKey: "99912345678", BillerCode: "70012"}
	va, biller = g.extractVANumber(mandiriResp, entity.BankMandiri)
	if va != "99912345678" || biller != "70012" {
		t.Errorf("Mandiri: got va=%q biller=%q", va, biller)
	}
}


func TestCreateVA_BCA(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertBasicAuth(t, r, testServerKey)
		if r.URL.Path != "/v2/charge" || r.Method != http.MethodPost {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(chargeResponse{ //nolint:errcheck
			StatusCode:  "201",
			OrderID:     "order-bca-1",
			ExpiryTime:  "2026-06-19 10:00:00",
			VANumbers:   []vaNum{{Bank: "bca", VANumber: "8001234567890"}},
		})
	}))
	defer srv.Close()

	g := newTestGateway(srv.URL)
	resp, err := g.CreateVA(context.Background(), gateway.CreateVARequest{
		ExternalID:    "order-bca-1",
		BankCode:      entity.BankBCA,
		Amount:        100000,
		Currency:      entity.CurrencyIDR,
		CustomerName:  "John",
		CustomerEmail: "john@test.com",
		CustomerPhone: "081234567890",
		ExpiryAt:      time.Now().Add(24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.VANumber != "8001234567890" {
		t.Errorf("VANumber = %q, want %q", resp.VANumber, "8001234567890")
	}
	if resp.BillerCode != "" {
		t.Errorf("BillerCode should be empty for BCA, got %q", resp.BillerCode)
	}
}

func TestCreateVA_Mandiri(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body) //nolint:errcheck
		if body["payment_type"] != "echannel" {
			t.Errorf("expected payment_type=echannel, got %v", body["payment_type"])
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(chargeResponse{ //nolint:errcheck
			StatusCode: "201",
			OrderID:    "order-mandiri-1",
			BillKey:    "99912345678",
			BillerCode: "70012",
		})
	}))
	defer srv.Close()

	g := newTestGateway(srv.URL)
	resp, err := g.CreateVA(context.Background(), gateway.CreateVARequest{
		ExternalID: "order-mandiri-1",
		BankCode:   entity.BankMandiri,
		Amount:     200000,
		ExpiryAt:   time.Now().Add(24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.VANumber != "99912345678" {
		t.Errorf("VANumber = %q, want bill_key", resp.VANumber)
	}
	if resp.BillerCode != "70012" {
		t.Errorf("BillerCode = %q, want 70012", resp.BillerCode)
	}
}

func TestCreateVA_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(chargeResponse{ //nolint:errcheck
			StatusCode:    "400",
			StatusMessage: "transaction_details.order_id already used",
		})
	}))
	defer srv.Close()

	g := newTestGateway(srv.URL)
	_, err := g.CreateVA(context.Background(), gateway.CreateVARequest{
		ExternalID: "dup-order",
		BankCode:   entity.BankBCA,
		Amount:     100000,
		ExpiryAt:   time.Now().Add(time.Hour),
	})
	if err == nil {
		t.Error("expected error for 400 response")
	}
}


func TestCreateQRIS(t *testing.T) {
	qrFetched := false
	var srvURL string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v2/charge" {
			json.NewEncoder(w).Encode(chargeResponse{ //nolint:errcheck
				StatusCode: "201",
				OrderID:    "order-qris-1",
				Actions: []action{
					{Name: "generate-qr-code", URL: srvURL + "/qr-image"},
				},
			})
			return
		}
		if r.URL.Path == "/qr-image" {
			qrFetched = true
			json.NewEncoder(w).Encode(map[string]string{"qr_string": "00020101021226..."}) //nolint:errcheck
			return
		}
		http.NotFound(w, r)
	}))
	srvURL = srv.URL
	defer srv.Close()

	g := newTestGateway(srv.URL)
	resp, err := g.CreateQRIS(context.Background(), gateway.CreateQRISRequest{
		ExternalID:   "order-qris-1",
		Amount:       50000,
		CustomerName: "Jane",
		ExpiryAt:     time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !qrFetched {
		t.Error("expected QR image URL to be fetched")
	}
	if resp.QRString != "00020101021226..." {
		t.Errorf("QRString = %q, want fetched value", resp.QRString)
	}
	if resp.QRImageURL == "" {
		t.Error("QRImageURL should not be empty")
	}
}


func TestGetStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/order-abc/status" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(statusResponse{ //nolint:errcheck
			StatusCode:        "200",
			TransactionStatus: "settlement",
			FraudStatus:       "accept",
		})
	}))
	defer srv.Close()

	g := newTestGateway(srv.URL)
	status, err := g.GetStatus(context.Background(), "order-abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != entity.PaymentStatusPaid {
		t.Errorf("status = %q, want paid", status)
	}
}


func TestCancelPayment(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/order-xyz/cancel" || r.Method != http.MethodPost {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(chargeResponse{StatusCode: "200"}) //nolint:errcheck
	}))
	defer srv.Close()

	g := newTestGateway(srv.URL)
	if err := g.CancelPayment(context.Background(), "order-xyz"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}


func TestParseWebhook_ValidSignature(t *testing.T) {
	g := newTestGateway("")

	orderID := "order-webhook-1"
	statusCode := "200"
	grossAmount := "75000.00"

	h := sha512.New()
	h.Write([]byte(orderID + statusCode + grossAmount + testServerKey))
	sig := hex.EncodeToString(h.Sum(nil))

	body, _ := json.Marshal(notification{
		OrderID:           orderID,
		TransactionStatus: "settlement",
		FraudStatus:       "accept",
		GrossAmount:       grossAmount,
		StatusCode:        statusCode,
		SignatureKey:      sig,
		SettlementTime:    "2026-06-18 10:00:00",
	})

	event, err := g.ParseWebhook(context.Background(), nil, body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if event.Status != entity.PaymentStatusPaid {
		t.Errorf("status = %q, want paid", event.Status)
	}
	if event.Amount != 75000 {
		t.Errorf("amount = %d, want 75000", event.Amount)
	}
	if event.PaidAt == nil {
		t.Error("PaidAt should not be nil for settlement")
	}
}

func TestParseWebhook_InvalidSignature(t *testing.T) {
	g := newTestGateway("")

	body, _ := json.Marshal(notification{
		OrderID:           "order-1",
		TransactionStatus: "settlement",
		FraudStatus:       "accept",
		GrossAmount:       "100000.00",
		StatusCode:        "200",
		SignatureKey:      "invalidsignature",
	})

	_, err := g.ParseWebhook(context.Background(), nil, body)
	if err == nil {
		t.Error("expected error for invalid signature")
	}
}


func assertBasicAuth(t *testing.T, r *http.Request, serverKey string) {
	t.Helper()
	auth := r.Header.Get("Authorization")
	expected := "Basic " + base64.StdEncoding.EncodeToString([]byte(serverKey+":"))
	if auth != expected {
		t.Errorf("Authorization = %q, want %q", auth, expected)
	}
}
