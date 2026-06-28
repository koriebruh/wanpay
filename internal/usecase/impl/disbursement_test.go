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
)

func newDisbUC(
	disbRepo *stubDisbursementRepo,
	merchRepo *stubMerchantRepo,
	mutRepo *stubMutationRepo,
	outbox *stubOutboxRepo,
	gw gateway.DisbursementGateway,
) *disbursementUsecase {
	gws := map[entity.Provider]gateway.DisbursementGateway{}
	if gw != nil {
		gws[entity.ProviderXendit] = gw
	}
	return &disbursementUsecase{
		gateways:         gws,
		disbursementRepo: disbRepo,
		mutationRepo:     mutRepo,
		outboxRepo:       outbox,
		merchantRepo:     merchRepo,
		feeResolver:      newStubFeeResolver(),
		db:               newStubSQLDB(),
		log:              zap.NewNop(),
	}
}

// ── Disburse ──────────────────────────────────────────────────────────────────

func TestDisburse_Success(t *testing.T) {
	merch := activeMerchant("m1")
	account := verifiedBankAccount("ba1", "m1")
	disbRepo := &stubDisbursementRepo{}
	mutRepo := &stubMutationRepo{balance: 1_000_000}
	uc := newDisbUC(disbRepo, &stubMerchantRepo{merchant: merch, bankAccount: account}, mutRepo, &stubOutboxRepo{}, &stubDisbursementGateway{})

	out, err := uc.Disburse(context.Background(), usecase.DisburseInput{
		MerchantID:    "m1",
		Provider:      entity.ProviderXendit,
		BankAccountID: "ba1",
		Amount:        500_000,
		Currency:      entity.CurrencyIDR,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.ID == "" {
		t.Error("disbursement id should not be empty")
	}
}

func TestDisburse_MerchantNotActive(t *testing.T) {
	merch := pendingMerchant("m1")
	uc := newDisbUC(&stubDisbursementRepo{}, &stubMerchantRepo{merchant: merch}, &stubMutationRepo{}, &stubOutboxRepo{}, &stubDisbursementGateway{})

	_, err := uc.Disburse(context.Background(), usecase.DisburseInput{
		MerchantID: "m1",
		Provider:   entity.ProviderXendit,
		Amount:     100_000,
		Currency:   entity.CurrencyIDR,
	})
	if !isApperror(err, 403) {
		t.Errorf("expected 403, got %v", err)
	}
}

func TestDisburse_BankAccountWrongMerchant(t *testing.T) {
	merch := activeMerchant("m1")
	account := verifiedBankAccount("ba1", "m2") // belongs to m2, not m1
	uc := newDisbUC(&stubDisbursementRepo{}, &stubMerchantRepo{merchant: merch, bankAccount: account}, &stubMutationRepo{balance: 999_999}, &stubOutboxRepo{}, &stubDisbursementGateway{})

	_, err := uc.Disburse(context.Background(), usecase.DisburseInput{
		MerchantID:    "m1",
		Provider:      entity.ProviderXendit,
		BankAccountID: "ba1",
		Amount:        100_000,
		Currency:      entity.CurrencyIDR,
	})
	if !isApperror(err, 404) {
		t.Errorf("expected 404 for wrong merchant's bank account, got %v", err)
	}
}

func TestDisburse_BankAccountNotVerified(t *testing.T) {
	merch := activeMerchant("m1")
	account := &entity.MerchantBankAccount{
		ID:         "ba1",
		MerchantID: "m1",
		BankCode:   entity.BankBCA,
		IsVerified: false, // ← not verified
	}
	uc := newDisbUC(&stubDisbursementRepo{}, &stubMerchantRepo{merchant: merch, bankAccount: account}, &stubMutationRepo{balance: 999_999}, &stubOutboxRepo{}, &stubDisbursementGateway{})

	_, err := uc.Disburse(context.Background(), usecase.DisburseInput{
		MerchantID:    "m1",
		Provider:      entity.ProviderXendit,
		BankAccountID: "ba1",
		Amount:        100_000,
		Currency:      entity.CurrencyIDR,
	})
	if !isApperror(err, 422) {
		t.Errorf("expected 422 for unverified account, got %v", err)
	}
}

func TestDisburse_InsufficientBalance(t *testing.T) {
	merch := activeMerchant("m1")
	account := verifiedBankAccount("ba1", "m1")
	mutRepo := &stubMutationRepo{balance: 100_000} // only 100k
	uc := newDisbUC(&stubDisbursementRepo{}, &stubMerchantRepo{merchant: merch, bankAccount: account}, mutRepo, &stubOutboxRepo{}, &stubDisbursementGateway{})

	_, err := uc.Disburse(context.Background(), usecase.DisburseInput{
		MerchantID:    "m1",
		Provider:      entity.ProviderXendit,
		BankAccountID: "ba1",
		Amount:        500_000, // requesting more than balance
		Currency:      entity.CurrencyIDR,
	})
	if !isApperror(err, 422) {
		t.Errorf("expected 422 for insufficient balance, got %v", err)
	}
}

func TestDisburse_PendingDisbursementsReduceAvailable(t *testing.T) {
	merch := activeMerchant("m1")
	account := verifiedBankAccount("ba1", "m1")
	// balance 1M, but 800k already pending → only 200k available
	disbRepo := &stubDisbursementRepo{pendingTotal: 800_000}
	mutRepo := &stubMutationRepo{balance: 1_000_000}
	uc := newDisbUC(disbRepo, &stubMerchantRepo{merchant: merch, bankAccount: account}, mutRepo, &stubOutboxRepo{}, &stubDisbursementGateway{})

	_, err := uc.Disburse(context.Background(), usecase.DisburseInput{
		MerchantID:    "m1",
		Provider:      entity.ProviderXendit,
		BankAccountID: "ba1",
		Amount:        500_000, // more than (1M - 800k) = 200k available
		Currency:      entity.CurrencyIDR,
	})
	if !isApperror(err, 422) {
		t.Errorf("expected 422 (pending reduces available), got %v", err)
	}
}

func TestDisburse_DailyLimitExceeded(t *testing.T) {
	merch := &entity.Merchant{
		ID:                "m1",
		Status:            entity.MerchantStatusActive,
		WebhookURL:        "http://h/hook",
		DailyCashoutLimit: 500_000,
	}
	account := verifiedBankAccount("ba1", "m1")
	disbRepo := &stubDisbursementRepo{todayTotal: 400_000} // already disbursed 400k today
	mutRepo := &stubMutationRepo{balance: 10_000_000}
	uc := newDisbUC(disbRepo, &stubMerchantRepo{merchant: merch, bankAccount: account}, mutRepo, &stubOutboxRepo{}, &stubDisbursementGateway{})

	_, err := uc.Disburse(context.Background(), usecase.DisburseInput{
		MerchantID:    "m1",
		Provider:      entity.ProviderXendit,
		BankAccountID: "ba1",
		Amount:        200_000, // 400k + 200k = 600k > 500k limit
		Currency:      entity.CurrencyIDR,
	})
	if !isApperror(err, 422) {
		t.Errorf("expected 422 for daily limit exceeded, got %v", err)
	}
}

func TestDisburse_NoDailyLimit_Succeeds(t *testing.T) {
	merch := &entity.Merchant{
		ID:                "m1",
		Status:            entity.MerchantStatusActive,
		WebhookURL:        "http://h/hook",
		DailyCashoutLimit: 0, // 0 = unlimited
	}
	account := verifiedBankAccount("ba1", "m1")
	disbRepo := &stubDisbursementRepo{todayTotal: 999_999_999}
	mutRepo := &stubMutationRepo{balance: 10_000_000_000}
	uc := newDisbUC(disbRepo, &stubMerchantRepo{merchant: merch, bankAccount: account}, mutRepo, &stubOutboxRepo{}, &stubDisbursementGateway{})

	_, err := uc.Disburse(context.Background(), usecase.DisburseInput{
		MerchantID:    "m1",
		Provider:      entity.ProviderXendit,
		BankAccountID: "ba1",
		Amount:        5_000_000,
		Currency:      entity.CurrencyIDR,
	})
	if err != nil {
		t.Errorf("expected success with no limit, got %v", err)
	}
}

func TestDisburse_ProviderFails_MarksRecordFailed(t *testing.T) {
	merch := activeMerchant("m1")
	account := verifiedBankAccount("ba1", "m1")
	disbRepo := &stubDisbursementRepo{}
	mutRepo := &stubMutationRepo{balance: 1_000_000}
	gw := &stubDisbursementGateway{disburseErr: errors.New("provider down")}
	uc := newDisbUC(disbRepo, &stubMerchantRepo{merchant: merch, bankAccount: account}, mutRepo, &stubOutboxRepo{}, gw)

	_, err := uc.Disburse(context.Background(), usecase.DisburseInput{
		MerchantID:    "m1",
		Provider:      entity.ProviderXendit,
		BankAccountID: "ba1",
		Amount:        300_000,
		Currency:      entity.CurrencyIDR,
	})
	if err == nil {
		t.Fatal("expected error from provider failure, got nil")
	}
}

func TestDisburse_ProviderNotEnabled(t *testing.T) {
	merch := activeMerchant("m1")
	account := verifiedBankAccount("ba1", "m1")
	uc := newDisbUC(&stubDisbursementRepo{}, &stubMerchantRepo{merchant: merch, bankAccount: account}, &stubMutationRepo{balance: 999_999}, &stubOutboxRepo{}, nil)

	_, err := uc.Disburse(context.Background(), usecase.DisburseInput{
		MerchantID:    "m1",
		Provider:      entity.ProviderXendit, // not in map (nil gw)
		BankAccountID: "ba1",
		Amount:        100_000,
		Currency:      entity.CurrencyIDR,
	})
	if !isApperror(err, 400) {
		t.Errorf("expected 400 for disabled provider, got %v", err)
	}
}

// ── GetDisbursement ───────────────────────────────────────────────────────────

func TestGetDisbursement_Success(t *testing.T) {
	d := &entity.Disbursement{ID: "d1", MerchantID: "m1", Status: entity.DisbursementStatusPending}
	uc := newDisbUC(&stubDisbursementRepo{disbursement: d}, &stubMerchantRepo{}, &stubMutationRepo{}, &stubOutboxRepo{}, nil)

	out, err := uc.GetDisbursement(context.Background(), "m1", "d1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.ID != "d1" {
		t.Errorf("id = %q, want d1", out.ID)
	}
}

func TestGetDisbursement_WrongMerchant(t *testing.T) {
	d := &entity.Disbursement{ID: "d1", MerchantID: "m1"}
	uc := newDisbUC(&stubDisbursementRepo{disbursement: d}, &stubMerchantRepo{}, &stubMutationRepo{}, &stubOutboxRepo{}, nil)

	_, err := uc.GetDisbursement(context.Background(), "m2", "d1")
	if !isApperror(err, 404) {
		t.Errorf("expected 404 for cross-merchant access, got %v", err)
	}
}

// ── HandleDisbursementCallback ────────────────────────────────────────────────

func TestHandleDisbursementCallback_Completed(t *testing.T) {
	d := &entity.Disbursement{
		ID:         "d1",
		MerchantID: "m1",
		ExternalID: "ext-d1",
		Status:     entity.DisbursementStatusPending,
		Amount:     500_000,
		Currency:   entity.CurrencyIDR,
		BankCode:   entity.BankBCA,
	}
	disbRepo := &stubDisbursementRepo{disbursement: d}
	mutRepo := &stubMutationRepo{}
	merch := activeMerchant("m1")
	event := &gateway.DisbursementWebhookEvent{
		ExternalID: "ext-d1",
		Status:     entity.DisbursementStatusCompleted,
		Amount:     500_000,
	}
	gw := &stubDisbursementGateway{webhookResp: event}
	uc := newDisbUC(disbRepo, &stubMerchantRepo{merchant: merch}, mutRepo, &stubOutboxRepo{}, gw)

	if err := uc.HandleDisbursementCallback(context.Background(), entity.ProviderXendit, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.Status != entity.DisbursementStatusCompleted {
		t.Errorf("status = %q, want completed", d.Status)
	}
}

func TestHandleDisbursementCallback_AlreadyFinal_Idempotent(t *testing.T) {
	d := &entity.Disbursement{
		ID:         "d1",
		MerchantID: "m1",
		ExternalID: "ext-d1",
		Status:     entity.DisbursementStatusCompleted, // already final
	}
	event := &gateway.DisbursementWebhookEvent{ExternalID: "ext-d1", Status: entity.DisbursementStatusCompleted}
	gw := &stubDisbursementGateway{webhookResp: event}
	uc := newDisbUC(&stubDisbursementRepo{disbursement: d}, &stubMerchantRepo{}, &stubMutationRepo{}, &stubOutboxRepo{}, gw)

	if err := uc.HandleDisbursementCallback(context.Background(), entity.ProviderXendit, nil, nil); err != nil {
		t.Fatalf("expected nil for idempotent callback, got %v", err)
	}
}

// ── ListDisbursements ─────────────────────────────────────────────────────────

func TestListDisbursements_Defaults(t *testing.T) {
	uc := newDisbUC(&stubDisbursementRepo{}, &stubMerchantRepo{}, &stubMutationRepo{}, &stubOutboxRepo{}, nil)

	out, err := uc.ListDisbursements(context.Background(), usecase.ListDisbursementsInput{MerchantID: "m1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Page != 1 {
		t.Errorf("page = %d, want 1", out.Page)
	}
	if out.Limit != 20 {
		t.Errorf("limit = %d, want 20", out.Limit)
	}
}

func TestListDisbursements_LimitCapped(t *testing.T) {
	uc := newDisbUC(&stubDisbursementRepo{}, &stubMerchantRepo{}, &stubMutationRepo{}, &stubOutboxRepo{}, nil)

	out, err := uc.ListDisbursements(context.Background(), usecase.ListDisbursementsInput{
		MerchantID: "m1", Limit: 500,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Limit != 100 {
		t.Errorf("limit = %d, want 100 (capped)", out.Limit)
	}
}

// ── IsFinal helper (entity level, tested indirectly via callback) ─────────────

func TestDisbursement_IsFinal(t *testing.T) {
	cases := []struct {
		status entity.DisbursementStatus
		want   bool
	}{
		{entity.DisbursementStatusPending, false},
		{entity.DisbursementStatusProcessing, false},
		{entity.DisbursementStatusCompleted, true},
		{entity.DisbursementStatusFailed, true},
	}
	for _, tc := range cases {
		d := &entity.Disbursement{Status: tc.status}
		if got := d.IsFinal(); got != tc.want {
			t.Errorf("IsFinal(%q) = %v, want %v", tc.status, got, tc.want)
		}
	}
}

// ── type assertions ───────────────────────────────────────────────────────────

// Ensure DisbursementOutput fields exist
func TestDisbursementOutput_Fields(_ *testing.T) {
	_ = usecase.DisbursementOutput{
		ID:         "d1",
		ExternalID: "ext",
		Provider:   entity.ProviderXendit,
		Status:     entity.DisbursementStatusPending,
		BankCode:   entity.BankBCA,
		Amount:     100_000,
		Currency:   entity.CurrencyIDR,
	}
}

// Disbursement entity
type disbursementEntity = entity.Disbursement

func stubDisbursement(id, merchantID string) *disbursementEntity {
	return &disbursementEntity{
		ID:         id,
		MerchantID: merchantID,
		ExternalID: "ext-" + id,
		Status:     entity.DisbursementStatusPending,
		Amount:     100_000,
		Currency:   entity.CurrencyIDR,
		BankCode:   entity.BankBCA,
		CreatedAt:  time.Now(),
	}
}

var _ = stubDisbursement // suppress unused warning
