//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"wanpey/core/internal/domain/entity"
	"wanpey/core/internal/domain/gateway"
)

// TestPaymentFlow_VA_CreateAndGet tests the full VA payment lifecycle:
// create → get by ID → verify fields.
func TestPaymentFlow_VA_CreateAndGet(t *testing.T) {
	token := getAdminToken(t)
	merchantID, key := createTestMerchant(t, token)

	// Approve so merchant can transact
	req(t, http.MethodPatch, "/admin/merchants/"+merchantID+"/approve", nil, bearer(token)) //nolint

	expiry := time.Now().Add(24 * time.Hour).Format(time.RFC3339)
	code, resp := req(t, http.MethodPost, "/v1/payments/va", map[string]any{
		"provider":       "midtrans",
		"bank_code":      "BCA",
		"amount":         100000,
		"currency":       "IDR",
		"customer_name":  "Test Customer",
		"customer_email": "customer@e2e.local",
		"customer_phone": "081234567890",
		"description":    "E2E test VA payment",
		"expiry_at":      expiry,
	}, apiKey(key))
	if code != http.StatusCreated {
		t.Fatalf("create VA: %d %s", code, apiErr(resp))
	}

	var payment struct {
		ID       string `json:"id"`
		Status   string `json:"status"`
		Amount   int64  `json:"amount"`
		VANumber string `json:"va_number"`
		BankCode string `json:"bank_code"`
		Method   string `json:"method"`
		Provider string `json:"provider"`
	}
	mustUnmarshal(t, resp.Data, &payment)

	if payment.ID == "" {
		t.Fatal("payment.id is empty")
	}
	if payment.Status != "pending" {
		t.Errorf("status = %q, want pending", payment.Status)
	}
	if payment.Amount != 100000 {
		t.Errorf("amount = %d, want 100000", payment.Amount)
	}
	if payment.VANumber == "" {
		t.Error("va_number is empty")
	}
	if payment.Method != "va" {
		t.Errorf("method = %q, want va", payment.Method)
	}
	if payment.Provider != "midtrans" {
		t.Errorf("provider = %q, want midtrans", payment.Provider)
	}

	// Get payment by ID
	code2, resp2 := req(t, http.MethodGet, "/v1/payments/"+payment.ID, nil, apiKey(key))
	if code2 != http.StatusOK {
		t.Fatalf("get payment: %d %s", code2, apiErr(resp2))
	}
	var got struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	mustUnmarshal(t, resp2.Data, &got)
	if got.ID != payment.ID {
		t.Errorf("payment ID mismatch: got %q, want %q", got.ID, payment.ID)
	}
}

// TestPaymentFlow_VA_MerchantNotActive ensures payment fails for non-active merchants.
func TestPaymentFlow_VA_MerchantNotActive(t *testing.T) {
	token := getAdminToken(t)
	_, key := createTestMerchant(t, token)
	// Merchant stays in "pending" — not approved

	expiry := time.Now().Add(24 * time.Hour).Format(time.RFC3339)
	code, _ := req(t, http.MethodPost, "/v1/payments/va", map[string]any{
		"provider":       "midtrans",
		"bank_code":      "BCA",
		"amount":         50000,
		"currency":       "IDR",
		"customer_name":  "Test",
		"customer_email": "t@t.com",
		"customer_phone": "081234567890",
		"expiry_at":      expiry,
	}, apiKey(key))
	if code != http.StatusForbidden {
		t.Errorf("expected 403 for inactive merchant, got %d", code)
	}
}

// TestPaymentFlow_VA_ProviderNotEnabled checks that requesting a disabled provider returns 400.
func TestPaymentFlow_VA_ProviderNotEnabled(t *testing.T) {
	token := getAdminToken(t)
	merchantID, key := createTestMerchant(t, token)
	req(t, http.MethodPatch, "/admin/merchants/"+merchantID+"/approve", nil, bearer(token)) //nolint

	expiry := time.Now().Add(24 * time.Hour).Format(time.RFC3339)
	// xendit is not in fake gateway map
	code, _ := req(t, http.MethodPost, "/v1/payments/va", map[string]any{
		"provider":       "xendit",
		"bank_code":      "BCA",
		"amount":         50000,
		"currency":       "IDR",
		"customer_name":  "Test",
		"customer_email": "t@t.com",
		"customer_phone": "081234567890",
		"expiry_at":      expiry,
	}, apiKey(key))
	if code != http.StatusBadRequest {
		t.Errorf("expected 400 for disabled provider, got %d", code)
	}
}

// TestPaymentFlow_VA_ValidationErrors checks that invalid request fields return 400.
func TestPaymentFlow_VA_ValidationErrors(t *testing.T) {
	token := getAdminToken(t)
	merchantID, key := createTestMerchant(t, token)
	req(t, http.MethodPatch, "/admin/merchants/"+merchantID+"/approve", nil, bearer(token)) //nolint

	cases := []struct {
		name string
		body map[string]any
	}{
		{
			name: "amount zero",
			body: map[string]any{
				"provider": "midtrans", "bank_code": "BCA", "amount": 0,
				"currency": "IDR", "customer_name": "T", "customer_email": "t@t.com",
				"customer_phone": "081234567890", "expiry_at": time.Now().Add(time.Hour).Format(time.RFC3339),
			},
		},
		{
			name: "invalid email",
			body: map[string]any{
				"provider": "midtrans", "bank_code": "BCA", "amount": 10000,
				"currency": "IDR", "customer_name": "T", "customer_email": "not-an-email",
				"customer_phone": "081234567890", "expiry_at": time.Now().Add(time.Hour).Format(time.RFC3339),
			},
		},
		{
			name: "invalid provider",
			body: map[string]any{
				"provider": "unknown_provider", "bank_code": "BCA", "amount": 10000,
				"currency": "IDR", "customer_name": "T", "customer_email": "t@t.com",
				"customer_phone": "081234567890", "expiry_at": time.Now().Add(time.Hour).Format(time.RFC3339),
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, _ := req(t, http.MethodPost, "/v1/payments/va", tc.body, apiKey(key))
			if code != http.StatusBadRequest && code != http.StatusUnprocessableEntity {
				t.Errorf("%s: expected 400/422, got %d", tc.name, code)
			}
		})
	}
}

// TestPaymentFlow_WebhookAndStatusUpdate tests the complete payment lifecycle:
// create VA → simulate provider webhook → verify payment status changes to paid.
func TestPaymentFlow_WebhookAndStatusUpdate(t *testing.T) {
	token := getAdminToken(t)
	merchantID, key := createTestMerchant(t, token)
	req(t, http.MethodPatch, "/admin/merchants/"+merchantID+"/approve", nil, bearer(token)) //nolint

	// Step 1: Create VA payment
	expiry := time.Now().Add(24 * time.Hour).Format(time.RFC3339)
	code, resp := req(t, http.MethodPost, "/v1/payments/va", map[string]any{
		"provider":       "midtrans",
		"bank_code":      "BCA",
		"amount":         200000,
		"currency":       "IDR",
		"customer_name":  "Webhook Test",
		"customer_email": "webhook@e2e.local",
		"customer_phone": "081234567890",
		"expiry_at":      expiry,
	}, apiKey(key))
	if code != http.StatusCreated {
		t.Fatalf("create VA for webhook test: %d %s", code, apiErr(resp))
	}
	var payment struct {
		ID         string `json:"id"`
		ExternalID string `json:"external_id"`
	}
	mustUnmarshal(t, resp.Data, &payment)

	// Step 2: Configure fake gateway to return a "paid" event for this external ID.
	paidAt := time.Now()
	testFakeGW.webhookResp = &gateway.WebhookEvent{
		ExternalID: payment.ExternalID,
		Status:     entity.PaymentStatusPaid,
		PaidAt:     &paidAt,
		Amount:     200000,
		RawPayload: []byte(`{"fake":"webhook"}`),
	}

	// Step 3: Send webhook to the server (fake Midtrans callback).
	webhookBody := fmt.Sprintf(`{"order_id":"%s","transaction_status":"settlement","gross_amount":"200000.00"}`, payment.ExternalID)
	webhookCode, webhookResp := req(t, http.MethodPost, "/webhooks/midtrans/payment",
		json.RawMessage(webhookBody), map[string]string{"Content-Type": "application/json"})
	if webhookCode != http.StatusOK {
		t.Fatalf("webhook: %d %s", webhookCode, apiErr(webhookResp))
	}

	// Step 4: Verify payment status is now "paid".
	code3, resp3 := req(t, http.MethodGet, "/v1/payments/"+payment.ID, nil, apiKey(key))
	if code3 != http.StatusOK {
		t.Fatalf("get payment after webhook: %d %s", code3, apiErr(resp3))
	}
	var updated struct {
		Status string     `json:"status"`
		PaidAt *time.Time `json:"paid_at"`
	}
	mustUnmarshal(t, resp3.Data, &updated)
	if updated.Status != "paid" {
		t.Errorf("status = %q after webhook, want paid", updated.Status)
	}
	if updated.PaidAt == nil {
		t.Error("paid_at should be set after webhook")
	}
}

// TestPaymentFlow_ListPayments verifies list endpoint returns payments for the merchant.
func TestPaymentFlow_ListPayments(t *testing.T) {
	token := getAdminToken(t)
	merchantID, key := createTestMerchant(t, token)
	req(t, http.MethodPatch, "/admin/merchants/"+merchantID+"/approve", nil, bearer(token)) //nolint

	// Create two payments
	for i := range 2 {
		expiry := time.Now().Add(24 * time.Hour).Format(time.RFC3339)
		code, resp := req(t, http.MethodPost, "/v1/payments/va", map[string]any{
			"provider":       "midtrans",
			"bank_code":      "BCA",
			"amount":         int64(50000 * (i + 1)),
			"currency":       "IDR",
			"customer_name":  "List Test",
			"customer_email": "list@e2e.local",
			"customer_phone": "081234567890",
			"expiry_at":      expiry,
		}, apiKey(key))
		if code != http.StatusCreated {
			t.Fatalf("create payment %d: %d %s", i, code, apiErr(resp))
		}
	}

	code, resp := req(t, http.MethodGet, "/v1/payments?page=1&limit=10", nil, apiKey(key))
	if code != http.StatusOK {
		t.Fatalf("list payments: %d %s", code, apiErr(resp))
	}
	// List endpoint wraps items in a paginated envelope: {"items":[...],"total":N,...}
	var page struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	mustUnmarshal(t, resp.Data, &page)
	if len(page.Items) < 2 {
		t.Errorf("expected at least 2 payments, got %d", len(page.Items))
	}
}

// TestPaymentFlow_GetPayment_WrongMerchant verifies merchants cannot see each other's payments.
func TestPaymentFlow_GetPayment_WrongMerchant(t *testing.T) {
	token := getAdminToken(t)

	// Merchant A creates payment
	merchantAID, keyA := createTestMerchant(t, token)
	req(t, http.MethodPatch, "/admin/merchants/"+merchantAID+"/approve", nil, bearer(token)) //nolint

	// Merchant B has no payments
	_, keyB := createTestMerchant(t, token)

	expiry := time.Now().Add(24 * time.Hour).Format(time.RFC3339)
	code, resp := req(t, http.MethodPost, "/v1/payments/va", map[string]any{
		"provider":       "midtrans",
		"bank_code":      "BCA",
		"amount":         75000,
		"currency":       "IDR",
		"customer_name":  "Isolation Test",
		"customer_email": "iso@e2e.local",
		"customer_phone": "081234567890",
		"expiry_at":      expiry,
	}, apiKey(keyA))
	if code != http.StatusCreated {
		t.Fatalf("create payment A: %d %s", code, apiErr(resp))
	}
	var pay struct {
		ID string `json:"id"`
	}
	mustUnmarshal(t, resp.Data, &pay)

	// Merchant B should NOT be able to see Merchant A's payment.
	// Returns 404 (not found for this merchant) or 403 (pending merchant can't access).
	code2, _ := req(t, http.MethodGet, "/v1/payments/"+pay.ID, nil, apiKey(keyB))
	if code2 != http.StatusNotFound && code2 != http.StatusForbidden {
		t.Errorf("merchant B accessing merchant A's payment: expected 404 or 403, got %d", code2)
	}
}

// TestPaymentFlow_Idempotency ensures the same Idempotency-Key returns the same response.
func TestPaymentFlow_Idempotency(t *testing.T) {
	token := getAdminToken(t)
	merchantID, key := createTestMerchant(t, token)
	req(t, http.MethodPatch, "/admin/merchants/"+merchantID+"/approve", nil, bearer(token)) //nolint

	idempKey := "e2e-idem-" + randHex(8)
	headers := map[string]string{
		"X-API-Key":         key,
		"X-Idempotency-Key": idempKey,
	}

	body := map[string]any{
		"provider":       "midtrans",
		"bank_code":      "BCA",
		"amount":         88000,
		"currency":       "IDR",
		"customer_name":  "Idem Test",
		"customer_email": "idem@e2e.local",
		"customer_phone": "081234567890",
		"expiry_at":      time.Now().Add(24 * time.Hour).Format(time.RFC3339),
	}

	code1, resp1 := req(t, http.MethodPost, "/v1/payments/va", body, headers)
	if code1 != http.StatusCreated {
		t.Fatalf("first create: %d %s", code1, apiErr(resp1))
	}

	code2, resp2 := req(t, http.MethodPost, "/v1/payments/va", body, headers)
	if code2 != http.StatusCreated && code2 != http.StatusOK {
		t.Fatalf("idempotent create: %d %s", code2, apiErr(resp2))
	}

	var p1, p2 struct{ ID string `json:"id"` }
	mustUnmarshal(t, resp1.Data, &p1)
	mustUnmarshal(t, resp2.Data, &p2)
	if p1.ID != p2.ID {
		t.Errorf("idempotency failed: first ID=%q, second ID=%q", p1.ID, p2.ID)
	}
}
