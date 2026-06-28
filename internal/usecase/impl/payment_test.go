package impl

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"

	"wanpey/core/internal/domain/entity"
	"wanpey/core/internal/domain/gateway"
	"wanpey/core/internal/usecase"
	"wanpey/core/pkg/apperror"
)

func newPaymentUC(
	payRepo *stubPaymentRepo,
	merchRepo *stubMerchantRepo,
	auditRepo *stubAuditRepo,
	mutRepo *stubMutationRepo,
	outbox *stubOutboxRepo,
	gw gateway.PaymentGateway,
) *paymentUsecase {
	gws := map[entity.Provider]gateway.PaymentGateway{}
	if gw != nil {
		gws[entity.ProviderMidtrans] = gw
	}
	return &paymentUsecase{
		gateways:     gws,
		paymentRepo:  payRepo,
		merchantRepo: merchRepo,
		auditRepo:    auditRepo,
		mutationRepo: mutRepo,
		outboxRepo:   outbox,
		feeResolver:  newStubFeeResolver(),
		db:           newStubSQLDB(),
		log:          zap.NewNop(),
	}
}

// ── CreateVA ──────────────────────────────────────────────────────────────────

func TestCreateVA_Success(t *testing.T) {
	merch := activeMerchant("m1")
	gw := &stubPaymentGateway{}
	uc := newPaymentUC(&stubPaymentRepo{}, &stubMerchantRepo{merchant: merch}, &stubAuditRepo{}, &stubMutationRepo{}, &stubOutboxRepo{}, gw)

	out, err := uc.CreateVA(context.Background(), usecase.CreateVAInput{
		MerchantID:    "m1",
		Provider:      entity.ProviderMidtrans,
		BankCode:      entity.BankBCA,
		Amount:        100000,
		Currency:      entity.CurrencyIDR,
		CustomerName:  "Test",
		CustomerEmail: "t@t.com",
		ExpiryAt:      time.Now().Add(24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != entity.PaymentStatusPending {
		t.Errorf("status = %q, want pending", out.Status)
	}
	if out.VANumber == "" {
		t.Error("va_number should not be empty")
	}
	if out.Method != entity.PaymentMethodVA {
		t.Errorf("method = %q, want va", out.Method)
	}
}

func TestCreateVA_MerchantNotActive(t *testing.T) {
	merch := pendingMerchant("m1")
	uc := newPaymentUC(&stubPaymentRepo{}, &stubMerchantRepo{merchant: merch}, &stubAuditRepo{}, &stubMutationRepo{}, &stubOutboxRepo{}, &stubPaymentGateway{})

	_, err := uc.CreateVA(context.Background(), usecase.CreateVAInput{
		MerchantID: "m1",
		Provider:   entity.ProviderMidtrans,
		Amount:     10000,
		Currency:   entity.CurrencyIDR,
		ExpiryAt:   time.Now().Add(time.Hour),
	})
	if !isApperror(err, 403) {
		t.Errorf("expected 403 Forbidden, got %v", err)
	}
}

func TestCreateVA_MerchantNoWebhookURL(t *testing.T) {
	merch := &entity.Merchant{ID: "m1", Status: entity.MerchantStatusActive, WebhookURL: ""}
	uc := newPaymentUC(&stubPaymentRepo{}, &stubMerchantRepo{merchant: merch}, &stubAuditRepo{}, &stubMutationRepo{}, &stubOutboxRepo{}, &stubPaymentGateway{})

	_, err := uc.CreateVA(context.Background(), usecase.CreateVAInput{
		MerchantID: "m1",
		Provider:   entity.ProviderMidtrans,
		Amount:     10000,
		Currency:   entity.CurrencyIDR,
		ExpiryAt:   time.Now().Add(time.Hour),
	})
	if !isApperror(err, 403) {
		t.Errorf("expected 403 Forbidden (no webhook URL), got %v", err)
	}
}

func TestCreateVA_ProviderNotEnabled(t *testing.T) {
	merch := activeMerchant("m1")
	uc := newPaymentUC(&stubPaymentRepo{}, &stubMerchantRepo{merchant: merch}, &stubAuditRepo{}, &stubMutationRepo{}, &stubOutboxRepo{}, nil)

	_, err := uc.CreateVA(context.Background(), usecase.CreateVAInput{
		MerchantID: "m1",
		Provider:   entity.ProviderMidtrans, // not in gateway map
		Amount:     10000,
		Currency:   entity.CurrencyIDR,
		ExpiryAt:   time.Now().Add(time.Hour),
	})
	if !isApperror(err, 400) {
		t.Errorf("expected 400 for disabled provider, got %v", err)
	}
}

func TestCreateVA_GatewayError(t *testing.T) {
	merch := activeMerchant("m1")
	gw := &stubPaymentGateway{vaErr: errors.New("provider timeout")}
	uc := newPaymentUC(&stubPaymentRepo{}, &stubMerchantRepo{merchant: merch}, &stubAuditRepo{}, &stubMutationRepo{}, &stubOutboxRepo{}, gw)

	_, err := uc.CreateVA(context.Background(), usecase.CreateVAInput{
		MerchantID: "m1",
		Provider:   entity.ProviderMidtrans,
		Amount:     50000,
		Currency:   entity.CurrencyIDR,
		ExpiryAt:   time.Now().Add(time.Hour),
	})
	if err == nil {
		t.Error("expected error from gateway, got nil")
	}
}

// ── CreateQRIS ────────────────────────────────────────────────────────────────

func TestCreateQRIS_Success(t *testing.T) {
	merch := activeMerchant("m1")
	gw := &stubPaymentGateway{}
	uc := newPaymentUC(&stubPaymentRepo{}, &stubMerchantRepo{merchant: merch}, &stubAuditRepo{}, &stubMutationRepo{}, &stubOutboxRepo{}, gw)

	out, err := uc.CreateQRIS(context.Background(), usecase.CreateQRISInput{
		MerchantID:    "m1",
		Provider:      entity.ProviderMidtrans,
		Amount:        75000,
		Currency:      entity.CurrencyIDR,
		CustomerName:  "Test",
		CustomerEmail: "t@t.com",
		ExpiryAt:      time.Now().Add(24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Method != entity.PaymentMethodQRIS {
		t.Errorf("method = %q, want qris", out.Method)
	}
	if out.QRString == "" {
		t.Error("qr_string should not be empty")
	}
}

func TestCreateQRIS_MerchantNotActive(t *testing.T) {
	merch := pendingMerchant("m1")
	uc := newPaymentUC(&stubPaymentRepo{}, &stubMerchantRepo{merchant: merch}, &stubAuditRepo{}, &stubMutationRepo{}, &stubOutboxRepo{}, &stubPaymentGateway{})

	_, err := uc.CreateQRIS(context.Background(), usecase.CreateQRISInput{
		MerchantID: "m1",
		Provider:   entity.ProviderMidtrans,
		Amount:     10000,
		Currency:   entity.CurrencyIDR,
		ExpiryAt:   time.Now().Add(time.Hour),
	})
	if !isApperror(err, 403) {
		t.Errorf("expected 403, got %v", err)
	}
}

// ── GetPayment ────────────────────────────────────────────────────────────────

func TestGetPayment_Success(t *testing.T) {
	p := pendingPayment("p1", "m1")
	uc := newPaymentUC(&stubPaymentRepo{payment: p}, &stubMerchantRepo{}, &stubAuditRepo{}, &stubMutationRepo{}, &stubOutboxRepo{}, nil)

	out, err := uc.GetPayment(context.Background(), "m1", "p1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.ID != "p1" {
		t.Errorf("id = %q, want p1", out.ID)
	}
}

func TestGetPayment_WrongMerchant(t *testing.T) {
	p := pendingPayment("p1", "m1")
	uc := newPaymentUC(&stubPaymentRepo{payment: p}, &stubMerchantRepo{}, &stubAuditRepo{}, &stubMutationRepo{}, &stubOutboxRepo{}, nil)

	_, err := uc.GetPayment(context.Background(), "m2", "p1") // m2 accessing m1's payment
	if !isApperror(err, 404) {
		t.Errorf("expected 404 for cross-merchant access, got %v", err)
	}
}

func TestGetPayment_NotFound(t *testing.T) {
	uc := newPaymentUC(&stubPaymentRepo{findErr: apperror.NotFound("not found")}, &stubMerchantRepo{}, &stubAuditRepo{}, &stubMutationRepo{}, &stubOutboxRepo{}, nil)

	_, err := uc.GetPayment(context.Background(), "m1", "nope")
	if !isApperror(err, 404) {
		t.Errorf("expected 404, got %v", err)
	}
}

// ── CancelPayment ─────────────────────────────────────────────────────────────

func TestCancelPayment_Success(t *testing.T) {
	p := pendingPayment("p1", "m1")
	uc := newPaymentUC(&stubPaymentRepo{payment: p}, &stubMerchantRepo{}, &stubAuditRepo{}, &stubMutationRepo{}, &stubOutboxRepo{}, &stubPaymentGateway{})

	if err := uc.CancelPayment(context.Background(), "m1", "p1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCancelPayment_WrongMerchant(t *testing.T) {
	p := pendingPayment("p1", "m1")
	uc := newPaymentUC(&stubPaymentRepo{payment: p}, &stubMerchantRepo{}, &stubAuditRepo{}, &stubMutationRepo{}, &stubOutboxRepo{}, &stubPaymentGateway{})

	err := uc.CancelPayment(context.Background(), "m2", "p1")
	if !isApperror(err, 404) {
		t.Errorf("expected 404, got %v", err)
	}
}

func TestCancelPayment_AlreadyPaid(t *testing.T) {
	p := paidPayment("p1", "m1")
	uc := newPaymentUC(&stubPaymentRepo{payment: p}, &stubMerchantRepo{}, &stubAuditRepo{}, &stubMutationRepo{}, &stubOutboxRepo{}, &stubPaymentGateway{})

	err := uc.CancelPayment(context.Background(), "m1", "p1")
	if !isApperror(err, 422) {
		t.Errorf("expected 422 for paid payment, got %v", err)
	}
}

func TestCancelPayment_ProviderFails_RevertsToProding(t *testing.T) {
	// Provider cancel fails → payment should revert to pending, not stay as cancelling.
	p := pendingPayment("p1", "m1")
	payRepo := &stubPaymentRepo{payment: p}
	gw := &stubPaymentGateway{cancelErr: errors.New("provider down")}
	uc := newPaymentUC(payRepo, &stubMerchantRepo{}, &stubAuditRepo{}, &stubMutationRepo{}, &stubOutboxRepo{}, gw)

	err := uc.CancelPayment(context.Background(), "m1", "p1")
	// Error is nil because the revert itself succeeds; the provider error is logged.
	// Status should be back to pending.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Status != entity.PaymentStatusPending {
		t.Errorf("status = %q after provider failure, want pending", p.Status)
	}
}

// ── HandleWebhook ─────────────────────────────────────────────────────────────

func TestHandleWebhook_MarksPaymentPaid(t *testing.T) {
	p := pendingPayment("p1", "m1")
	now := time.Now()
	event := &gateway.WebhookEvent{
		ExternalID: p.ExternalID,
		Status:     entity.PaymentStatusPaid,
		PaidAt:     &now,
		Amount:     p.Amount,
	}
	gw := &stubPaymentGateway{webhookResp: event}
	merch := activeMerchant("m1")
	uc := newPaymentUC(
		&stubPaymentRepo{payment: p},
		&stubMerchantRepo{merchant: merch},
		&stubAuditRepo{},
		&stubMutationRepo{},
		&stubOutboxRepo{},
		gw,
	)

	if err := uc.HandleWebhook(context.Background(), entity.ProviderMidtrans, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Status != entity.PaymentStatusPaid {
		t.Errorf("status = %q, want paid", p.Status)
	}
	if p.PaidAt == nil {
		t.Error("paid_at should be set")
	}
}

func TestHandleWebhook_Idempotent_AlreadyPaid(t *testing.T) {
	p := paidPayment("p1", "m1")
	now := time.Now()
	event := &gateway.WebhookEvent{
		ExternalID: p.ExternalID,
		Status:     entity.PaymentStatusPaid,
		PaidAt:     &now,
		Amount:     p.Amount,
	}
	gw := &stubPaymentGateway{webhookResp: event}
	uc := newPaymentUC(
		&stubPaymentRepo{payment: p},
		&stubMerchantRepo{merchant: activeMerchant("m1")},
		&stubAuditRepo{},
		&stubMutationRepo{},
		&stubOutboxRepo{},
		gw,
	)

	// Second webhook for already-paid payment should return nil (idempotent).
	if err := uc.HandleWebhook(context.Background(), entity.ProviderMidtrans, nil, nil); err != nil {
		t.Fatalf("expected nil for idempotent webhook, got %v", err)
	}
}

func TestHandleWebhook_ParseError(t *testing.T) {
	gw := &stubPaymentGateway{webhookErr: errors.New("invalid signature")}
	uc := newPaymentUC(&stubPaymentRepo{}, &stubMerchantRepo{}, &stubAuditRepo{}, &stubMutationRepo{}, &stubOutboxRepo{}, gw)

	err := uc.HandleWebhook(context.Background(), entity.ProviderMidtrans, nil, nil)
	if err == nil {
		t.Error("expected error from ParseWebhook, got nil")
	}
}

func TestHandleWebhook_MarksPaymentExpired(t *testing.T) {
	p := pendingPayment("p1", "m1")
	event := &gateway.WebhookEvent{
		ExternalID: p.ExternalID,
		Status:     entity.PaymentStatusExpired,
		Amount:     p.Amount,
	}
	gw := &stubPaymentGateway{webhookResp: event}
	merch := activeMerchant("m1")
	uc := newPaymentUC(
		&stubPaymentRepo{payment: p},
		&stubMerchantRepo{merchant: merch},
		&stubAuditRepo{},
		&stubMutationRepo{},
		&stubOutboxRepo{},
		gw,
	)

	if err := uc.HandleWebhook(context.Background(), entity.ProviderMidtrans, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Status != entity.PaymentStatusExpired {
		t.Errorf("status = %q, want expired", p.Status)
	}
}

func TestHandleWebhook_UnknownProvider(t *testing.T) {
	uc := newPaymentUC(&stubPaymentRepo{}, &stubMerchantRepo{}, &stubAuditRepo{}, &stubMutationRepo{}, &stubOutboxRepo{}, nil)

	err := uc.HandleWebhook(context.Background(), entity.ProviderXendit, nil, nil)
	if !isApperror(err, 400) {
		t.Errorf("expected 400 for unknown provider, got %v", err)
	}
}

// ── ListPayments ──────────────────────────────────────────────────────────────

func TestListPayments_PaginationDefaults(t *testing.T) {
	items := []*entity.Payment{pendingPayment("p1", "m1"), pendingPayment("p2", "m1")}
	uc := newPaymentUC(&stubPaymentRepo{listResult: items, listTotal: 2}, &stubMerchantRepo{}, &stubAuditRepo{}, &stubMutationRepo{}, &stubOutboxRepo{}, nil)

	out, err := uc.ListPayments(context.Background(), usecase.ListPaymentsInput{MerchantID: "m1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Page != 1 {
		t.Errorf("page = %d, want 1 (default)", out.Page)
	}
	if out.Limit != 20 {
		t.Errorf("limit = %d, want 20 (default)", out.Limit)
	}
	if len(out.Items) != 2 {
		t.Errorf("items = %d, want 2", len(out.Items))
	}
}

func TestListPayments_LimitCappedAt100(t *testing.T) {
	uc := newPaymentUC(&stubPaymentRepo{}, &stubMerchantRepo{}, &stubAuditRepo{}, &stubMutationRepo{}, &stubOutboxRepo{}, nil)

	out, err := uc.ListPayments(context.Background(), usecase.ListPaymentsInput{
		MerchantID: "m1",
		Page:       1,
		Limit:      999,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Limit != 100 {
		t.Errorf("limit = %d, want 100 (capped)", out.Limit)
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func isApperror(err error, code int) bool {
	var ae *apperror.AppError
	return errors.As(err, &ae) && ae.HTTPCode() == code
}
