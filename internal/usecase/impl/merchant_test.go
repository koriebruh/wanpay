package impl

import (
	"context"
	"testing"

	"wanpey/core/internal/domain/entity"
	"wanpey/core/internal/usecase"
	"wanpey/core/pkg/apperror"
)

func newMerchantUC(
	repo *stubMerchantRepo,
	mutRepo *stubMutationRepo,
	outbox *stubOutboxRepo,
) *merchantUsecase {
	return &merchantUsecase{
		merchantRepo: repo,
		mutationRepo: mutRepo,
		outboxRepo:   outbox,
		db:           newStubSQLDB(),
	}
}

// ── Create ────────────────────────────────────────────────────────────────────

func TestCreate_Success(t *testing.T) {
	uc := newMerchantUC(&stubMerchantRepo{}, &stubMutationRepo{}, &stubOutboxRepo{})

	out, err := uc.Create(context.Background(), usecase.CreateMerchantInput{
		Name:       "Toko Makmur",
		Email:      "toko@example.com",
		Phone:      "081234567890",
		WebhookURL: "http://toko.local/hook",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.ID == "" {
		t.Error("id should not be empty")
	}
	if out.APIKey == "" {
		t.Error("api_key should not be empty")
	}
	if out.WebhookSecret == "" {
		t.Error("webhook_secret should not be empty")
	}
	if out.Status != entity.MerchantStatusPending {
		t.Errorf("status = %q, want pending", out.Status)
	}
}

func TestCreate_APIKeyPrefix_Sandbox(t *testing.T) {
	uc := newMerchantUC(&stubMerchantRepo{}, &stubMutationRepo{}, &stubOutboxRepo{})

	out, err := uc.Create(context.Background(), usecase.CreateMerchantInput{
		Name:       "Sandbox Merchant",
		Email:      "sandbox@example.com",
		IsProduction: false,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.APIKey) < 10 || out.APIKey[:10] != "wpay_test_" {
		t.Errorf("sandbox key should start with wpay_test_, got %q", out.APIKey[:min10(out.APIKey)])
	}
}

func TestCreate_APIKeyPrefix_Production(t *testing.T) {
	uc := newMerchantUC(&stubMerchantRepo{}, &stubMutationRepo{}, &stubOutboxRepo{})

	out, err := uc.Create(context.Background(), usecase.CreateMerchantInput{
		Name:         "Live Merchant",
		Email:        "live@example.com",
		IsProduction: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.APIKey) < 10 || out.APIKey[:10] != "wpay_live_" {
		t.Errorf("live key should start with wpay_live_, got %q", out.APIKey[:min10(out.APIKey)])
	}
}

func TestCreate_DuplicateEmail(t *testing.T) {
	existing := activeMerchant("m-existing")
	existing.Email = "dup@example.com"
	// findByEmailM is non-nil → triggers conflict
	repo := &stubMerchantRepo{findByEmailM: existing}
	uc := newMerchantUC(repo, &stubMutationRepo{}, &stubOutboxRepo{})

	_, err := uc.Create(context.Background(), usecase.CreateMerchantInput{
		Name:  "Dup",
		Email: "dup@example.com",
	})
	if !isApperror(err, 409) {
		t.Errorf("expected 409 Conflict for duplicate email, got %v", err)
	}
}

// ── GetMerchant ───────────────────────────────────────────────────────────────

func TestGetMerchant_ReturnsBalance(t *testing.T) {
	merch := activeMerchant("m1")
	uc := newMerchantUC(
		&stubMerchantRepo{merchant: merch},
		&stubMutationRepo{balance: 500_000},
		&stubOutboxRepo{},
	)

	out, err := uc.GetMerchant(context.Background(), "m1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Balance != 500_000 {
		t.Errorf("balance = %d, want 500000", out.Balance)
	}
}

func TestGetMerchant_NotFound(t *testing.T) {
	uc := newMerchantUC(
		&stubMerchantRepo{findByIDErr: apperror.NotFound("not found")},
		&stubMutationRepo{},
		&stubOutboxRepo{},
	)

	_, err := uc.GetMerchant(context.Background(), "nope")
	if !isApperror(err, 404) {
		t.Errorf("expected 404, got %v", err)
	}
}

// ── Update ────────────────────────────────────────────────────────────────────

func TestUpdate_ChangesName(t *testing.T) {
	merch := activeMerchant("m1")
	merch.Name = "Old Name"
	uc := newMerchantUC(&stubMerchantRepo{merchant: merch}, &stubMutationRepo{}, &stubOutboxRepo{})

	out, err := uc.Update(context.Background(), usecase.UpdateMerchantInput{
		MerchantID: "m1",
		Name:       "New Name",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Name != "New Name" {
		t.Errorf("name = %q, want New Name", out.Name)
	}
}

func TestUpdate_EmptyFieldsKeepExisting(t *testing.T) {
	merch := activeMerchant("m1")
	merch.Name = "Keep Me"
	merch.Email = "keep@me.com"
	uc := newMerchantUC(&stubMerchantRepo{merchant: merch}, &stubMutationRepo{}, &stubOutboxRepo{})

	out, err := uc.Update(context.Background(), usecase.UpdateMerchantInput{
		MerchantID: "m1",
		// No Name or Email → both should remain unchanged
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Name != "Keep Me" {
		t.Errorf("name changed unexpectedly to %q", out.Name)
	}
}

// ── Suspend / Activate ────────────────────────────────────────────────────────

func TestSuspend_ChangesStatus(t *testing.T) {
	merch := activeMerchant("m1")
	uc := newMerchantUC(&stubMerchantRepo{merchant: merch}, &stubMutationRepo{}, &stubOutboxRepo{})

	if err := uc.Suspend(context.Background(), "m1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if merch.Status != entity.MerchantStatusSuspended {
		t.Errorf("status = %q, want suspended", merch.Status)
	}
}

func TestActivate_ChangesStatus(t *testing.T) {
	merch := &entity.Merchant{ID: "m1", Status: entity.MerchantStatusSuspended, WebhookURL: "http://h"}
	uc := newMerchantUC(&stubMerchantRepo{merchant: merch}, &stubMutationRepo{}, &stubOutboxRepo{})

	if err := uc.Activate(context.Background(), "m1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if merch.Status != entity.MerchantStatusActive {
		t.Errorf("status = %q, want active", merch.Status)
	}
}

// ── RegenerateAPIKey ──────────────────────────────────────────────────────────

func TestRegenerateAPIKey_ReturnsDifferentKey(t *testing.T) {
	merch := activeMerchant("m1")
	merch.APIKey = "old-hashed-key"
	merch.IsProduction = false
	uc := newMerchantUC(&stubMerchantRepo{merchant: merch}, &stubMutationRepo{}, &stubOutboxRepo{})

	newKey, err := uc.RegenerateAPIKey(context.Background(), "m1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if newKey == "" {
		t.Error("new api key is empty")
	}
	if newKey == "old-hashed-key" {
		t.Error("new key should differ from old key")
	}
	if len(newKey) < 10 || newKey[:10] != "wpay_test_" {
		t.Errorf("sandbox key should start with wpay_test_, got %q...", newKey[:min10(newKey)])
	}
}

func TestRegenerateAPIKey_StoresHash_NotRaw(t *testing.T) {
	merch := activeMerchant("m1")
	merch.IsProduction = false
	repo := &stubMerchantRepo{merchant: merch}
	uc := newMerchantUC(repo, &stubMutationRepo{}, &stubOutboxRepo{})

	rawKey, err := uc.RegenerateAPIKey(context.Background(), "m1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Merchant.APIKey should now hold the hash (SHA256 hex = 64 chars), not the raw key
	if merch.APIKey == rawKey {
		t.Error("merchant.APIKey should store hash, not raw key")
	}
	if len(merch.APIKey) != 64 {
		t.Errorf("hashed key length = %d, want 64 (SHA256 hex)", len(merch.APIKey))
	}
}

// ── AddBankAccount ────────────────────────────────────────────────────────────

func TestAddBankAccount_Success(t *testing.T) {
	repo := &stubMerchantRepo{bankCount: 0}
	uc := newMerchantUC(repo, &stubMutationRepo{}, &stubOutboxRepo{})

	out, err := uc.AddBankAccount(context.Background(), usecase.AddBankAccountInput{
		MerchantID:    "m1",
		BankCode:      entity.BankBCA,
		AccountNumber: "1234567890",
		AccountName:   "Budi Santoso",
		SetAsPrimary:  true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.ID == "" {
		t.Error("bank account id should not be empty")
	}
	if out.IsPrimary != true {
		t.Error("should be primary")
	}
}

func TestAddBankAccount_MaxLimitReached(t *testing.T) {
	repo := &stubMerchantRepo{bankCount: entity.MaxBankAccounts} // already at limit
	uc := newMerchantUC(repo, &stubMutationRepo{}, &stubOutboxRepo{})

	_, err := uc.AddBankAccount(context.Background(), usecase.AddBankAccountInput{
		MerchantID:    "m1",
		BankCode:      entity.BankBNI,
		AccountNumber: "9999999999",
		AccountName:   "Extra",
	})
	if !isApperror(err, 422) {
		t.Errorf("expected 422 when max accounts reached, got %v", err)
	}
}

func TestAddBankAccount_AtLimit_ExactlyAtMax(t *testing.T) {
	// Count = MaxBankAccounts - 1 → should succeed
	repo := &stubMerchantRepo{bankCount: entity.MaxBankAccounts - 1}
	uc := newMerchantUC(repo, &stubMutationRepo{}, &stubOutboxRepo{})

	_, err := uc.AddBankAccount(context.Background(), usecase.AddBankAccountInput{
		MerchantID:    "m1",
		BankCode:      entity.BankBRI,
		AccountNumber: "7777777777",
		AccountName:   "Last Slot",
	})
	if err != nil {
		t.Errorf("should succeed when count < max, got %v", err)
	}
}

// ── RemoveBankAccount ─────────────────────────────────────────────────────────

func TestRemoveBankAccount_Success(t *testing.T) {
	account := &entity.MerchantBankAccount{
		ID:         "ba1",
		MerchantID: "m1",
		IsPrimary:  false,
	}
	repo := &stubMerchantRepo{bankAccount: account}
	uc := newMerchantUC(repo, &stubMutationRepo{}, &stubOutboxRepo{})

	if err := uc.RemoveBankAccount(context.Background(), "m1", "ba1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRemoveBankAccount_CannotRemovePrimary(t *testing.T) {
	account := &entity.MerchantBankAccount{
		ID:         "ba1",
		MerchantID: "m1",
		IsPrimary:  true, // ← primary → cannot remove
	}
	repo := &stubMerchantRepo{bankAccount: account}
	uc := newMerchantUC(repo, &stubMutationRepo{}, &stubOutboxRepo{})

	err := uc.RemoveBankAccount(context.Background(), "m1", "ba1")
	if !isApperror(err, 422) {
		t.Errorf("expected 422 when removing primary account, got %v", err)
	}
}

func TestRemoveBankAccount_WrongMerchant(t *testing.T) {
	account := &entity.MerchantBankAccount{
		ID:         "ba1",
		MerchantID: "m1", // owned by m1
		IsPrimary:  false,
	}
	repo := &stubMerchantRepo{bankAccount: account}
	uc := newMerchantUC(repo, &stubMutationRepo{}, &stubOutboxRepo{})

	err := uc.RemoveBankAccount(context.Background(), "m2", "ba1") // m2 tries to delete m1's account
	if !isApperror(err, 404) {
		t.Errorf("expected 404 for cross-merchant remove, got %v", err)
	}
}

// ── SetPrimaryBankAccount ─────────────────────────────────────────────────────

func TestSetPrimaryBankAccount_Success(t *testing.T) {
	account := &entity.MerchantBankAccount{
		ID:         "ba1",
		MerchantID: "m1",
		IsPrimary:  false,
	}
	repo := &stubMerchantRepo{bankAccount: account}
	uc := newMerchantUC(repo, &stubMutationRepo{}, &stubOutboxRepo{})

	if err := uc.SetPrimaryBankAccount(context.Background(), "m1", "ba1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !account.IsPrimary {
		t.Error("account.IsPrimary should be true after SetPrimaryBankAccount")
	}
}

func TestSetPrimaryBankAccount_WrongMerchant(t *testing.T) {
	account := &entity.MerchantBankAccount{
		ID:         "ba1",
		MerchantID: "m1",
		IsPrimary:  false,
	}
	repo := &stubMerchantRepo{bankAccount: account}
	uc := newMerchantUC(repo, &stubMutationRepo{}, &stubOutboxRepo{})

	err := uc.SetPrimaryBankAccount(context.Background(), "m2", "ba1")
	if !isApperror(err, 404) {
		t.Errorf("expected 404 for cross-merchant set-primary, got %v", err)
	}
}

// ── ListBankAccounts ──────────────────────────────────────────────────────────

func TestListBankAccounts_ReturnsAll(t *testing.T) {
	accounts := []*entity.MerchantBankAccount{
		{ID: "ba1", MerchantID: "m1", BankCode: entity.BankBCA},
		{ID: "ba2", MerchantID: "m1", BankCode: entity.BankBNI},
	}
	repo := &stubMerchantRepo{bankAccounts: accounts}
	uc := newMerchantUC(repo, &stubMutationRepo{}, &stubOutboxRepo{})

	out, err := uc.ListBankAccounts(context.Background(), "m1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 2 {
		t.Errorf("got %d accounts, want 2", len(out))
	}
}

// ── CanTransact logic (entity level) ─────────────────────────────────────────

func TestMerchantCanTransact(t *testing.T) {
	cases := []struct {
		name       string
		status     entity.MerchantStatus
		webhookURL string
		want       bool
	}{
		{"active with webhook", entity.MerchantStatusActive, "http://h", true},
		{"active no webhook", entity.MerchantStatusActive, "", false},
		{"pending with webhook", entity.MerchantStatusPending, "http://h", false},
		{"suspended with webhook", entity.MerchantStatusSuspended, "http://h", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := &entity.Merchant{Status: tc.status, WebhookURL: tc.webhookURL}
			if got := m.CanTransact(); got != tc.want {
				t.Errorf("CanTransact() = %v, want %v", got, tc.want)
			}
		})
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func min10(s string) int {
	if len(s) < 10 {
		return len(s)
	}
	return 10
}
