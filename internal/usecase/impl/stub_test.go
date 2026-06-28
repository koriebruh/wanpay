package impl

// Shared stubs, no-op DB driver, and helpers used across all usecase unit tests.
// Tests live in the same package (impl) so they can construct structs directly.

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"io"
	"sync"
	"time"

	"wanpey/core/internal/domain/entity"
	"wanpey/core/internal/domain/gateway"
	"wanpey/core/internal/domain/repository"
	"wanpey/core/internal/infrastructure/database/postgres/gen"
	"wanpey/core/pkg/apperror"
)

// ── noop SQL driver ────────────────────────────────────────────────────────────
// Provides a *sql.DB that satisfies database.SQLDB.
// BeginTx returns a no-op *sql.Tx so database.RunInTx can call Commit without error.
// Stub repositories ignore the Tx in context and return preset data instead.

var registerOnce sync.Once

func newStubSQLDB() *sql.DB {
	registerOnce.Do(func() { sql.Register("noop", noopDriver{}) })
	db, _ := sql.Open("noop", "")
	return db
}

type noopDriver struct{}
type noopConn struct{}
type noopTx struct{}
type noopStmt struct{}

// noopRows returns one row with a stub string value, then EOF.
// This satisfies QueryRowContext calls like the merchant lock in disbursement.go.
type noopRows struct{ done bool }

func (noopDriver) Open(_ string) (driver.Conn, error)  { return noopConn{}, nil }
func (noopConn) Prepare(_ string) (driver.Stmt, error) { return noopStmt{}, nil }
func (noopConn) Close() error                           { return nil }
func (noopConn) Begin() (driver.Tx, error)              { return noopTx{}, nil }
func (noopTx) Commit() error                            { return nil }
func (noopTx) Rollback() error                          { return nil }
func (noopStmt) Close() error                           { return nil }
func (noopStmt) NumInput() int                          { return -1 }
func (noopStmt) Exec(_ []driver.Value) (driver.Result, error) {
	return driver.RowsAffected(1), nil
}
func (noopStmt) Query(_ []driver.Value) (driver.Rows, error) { return &noopRows{}, nil }
func (*noopRows) Columns() []string                           { return []string{"id"} }
func (*noopRows) Close() error                                { return nil }
func (r *noopRows) Next(dest []driver.Value) error {
	if r.done {
		return io.EOF
	}
	r.done = true
	if len(dest) > 0 {
		dest[0] = "stub-id"
	}
	return nil
}

// ── stubOutboxRepo ─────────────────────────────────────────────────────────────

type stubOutboxRepo struct {
	insertErr error
}

func (s *stubOutboxRepo) Insert(_ context.Context, _, _, _ string, _ any) error {
	return s.insertErr
}
func (s *stubOutboxRepo) ListByMerchant(_ context.Context, _ string, _, _ int) ([]gen.Outbox, int64, error) {
	return nil, 0, nil
}

// ── stubMerchantRepo ──────────────────────────────────────────────────────────

type stubMerchantRepo struct {
	merchant      *entity.Merchant
	findByIDErr   error
	findByEmailM  *entity.Merchant // nil = not found
	findByEmailE  error
	saveErr       error
	updateErr     error

	bankAccount       *entity.MerchantBankAccount
	bankAccounts      []*entity.MerchantBankAccount
	bankAccountErr    error
	bankCount         int
	bankCountErr      error
	saveBankErr       error
	updateBankErr     error
	deleteBankErr     error
	unsetPrimaryErr   error
}

func (s *stubMerchantRepo) FindByID(_ context.Context, _ string) (*entity.Merchant, error) {
	return s.merchant, s.findByIDErr
}
func (s *stubMerchantRepo) FindByAPIKey(_ context.Context, _ string) (*entity.Merchant, error) {
	return s.merchant, s.findByIDErr
}
func (s *stubMerchantRepo) FindByEmail(_ context.Context, _ string) (*entity.Merchant, error) {
	if s.findByEmailE != nil {
		return nil, s.findByEmailE
	}
	if s.findByEmailM == nil {
		return nil, apperror.NotFound("not found")
	}
	return s.findByEmailM, nil
}
func (s *stubMerchantRepo) Save(_ context.Context, m *entity.Merchant) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	m.ID = "m-stub-id"
	m.CreatedAt = time.Now()
	m.UpdatedAt = time.Now()
	return nil
}
func (s *stubMerchantRepo) Update(_ context.Context, _ *entity.Merchant) error { return s.updateErr }
func (s *stubMerchantRepo) SaveBankAccount(_ context.Context, a *entity.MerchantBankAccount) error {
	if s.saveBankErr != nil {
		return s.saveBankErr
	}
	a.ID = "ba-stub-id"
	return nil
}
func (s *stubMerchantRepo) FindBankAccountsByMerchantID(_ context.Context, _ string) ([]*entity.MerchantBankAccount, error) {
	return s.bankAccounts, s.bankAccountErr
}
func (s *stubMerchantRepo) FindBankAccountByID(_ context.Context, _ string) (*entity.MerchantBankAccount, error) {
	return s.bankAccount, s.bankAccountErr
}
func (s *stubMerchantRepo) FindPrimaryBankAccount(_ context.Context, _ string) (*entity.MerchantBankAccount, error) {
	return s.bankAccount, s.bankAccountErr
}
func (s *stubMerchantRepo) UpdateBankAccount(_ context.Context, _ *entity.MerchantBankAccount) error {
	return s.updateBankErr
}
func (s *stubMerchantRepo) UnsetPrimaryBankAccounts(_ context.Context, _ string) error {
	return s.unsetPrimaryErr
}
func (s *stubMerchantRepo) DeleteBankAccount(_ context.Context, _ string) error {
	return s.deleteBankErr
}
func (s *stubMerchantRepo) CountBankAccounts(_ context.Context, _ string) (int, error) {
	return s.bankCount, s.bankCountErr
}
func (s *stubMerchantRepo) List(_ context.Context, _ repository.ListMerchantFilter) ([]*entity.Merchant, int64, error) {
	return nil, 0, nil
}
func (s *stubMerchantRepo) SoftDelete(_ context.Context, _ string) error { return nil }

// ── stubPaymentRepo ───────────────────────────────────────────────────────────

type stubPaymentRepo struct {
	payment    *entity.Payment
	findErr    error
	saveErr    error
	updateErr  error
	listResult []*entity.Payment
	listTotal  int64
}

func (s *stubPaymentRepo) Save(_ context.Context, p *entity.Payment) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	p.ID = "p-stub-id"
	p.CreatedAt = time.Now()
	p.UpdatedAt = time.Now()
	return nil
}
func (s *stubPaymentRepo) FindByID(_ context.Context, _ string) (*entity.Payment, error) {
	return s.payment, s.findErr
}
func (s *stubPaymentRepo) FindByIDForUpdate(_ context.Context, _ string) (*entity.Payment, error) {
	return s.payment, s.findErr
}
func (s *stubPaymentRepo) FindByExternalID(_ context.Context, _ entity.Provider, _ string) (*entity.Payment, error) {
	return s.payment, s.findErr
}
func (s *stubPaymentRepo) Update(_ context.Context, _ *entity.Payment) error { return s.updateErr }
func (s *stubPaymentRepo) List(_ context.Context, _ repository.ListPaymentFilter) ([]*entity.Payment, int64, error) {
	return s.listResult, s.listTotal, nil
}

// ── stubAuditRepo ─────────────────────────────────────────────────────────────

type stubAuditRepo struct{ saveErr error }

func (s *stubAuditRepo) Save(_ context.Context, _ *entity.PaymentAudit) error { return s.saveErr }
func (s *stubAuditRepo) FindByPaymentID(_ context.Context, _ string) ([]*entity.PaymentAudit, error) {
	return nil, nil
}

// ── stubMutationRepo ──────────────────────────────────────────────────────────

type stubMutationRepo struct {
	balance    int64
	balanceErr error
	saveErr    error
}

func (s *stubMutationRepo) Save(_ context.Context, _ *entity.Mutation) error { return s.saveErr }
func (s *stubMutationRepo) FindByID(_ context.Context, _ string) (*entity.Mutation, error) {
	return nil, nil
}
func (s *stubMutationRepo) FindByReferenceID(_ context.Context, _ string, _ entity.MutationReferenceType) (*entity.Mutation, error) {
	return nil, nil
}
func (s *stubMutationRepo) List(_ context.Context, _ repository.ListMutationFilter) ([]*entity.Mutation, int64, error) {
	return nil, 0, nil
}
func (s *stubMutationRepo) GetBalance(_ context.Context, _ string) (int64, error) {
	return s.balance, s.balanceErr
}

// ── stubDisbursementRepo ──────────────────────────────────────────────────────

type stubDisbursementRepo struct {
	disbursement   *entity.Disbursement
	findErr        error
	saveErr        error
	updateErr      error
	pendingTotal   int64
	pendingErr     error
	todayTotal     int64
	todayErr       error
}

func (s *stubDisbursementRepo) Save(_ context.Context, d *entity.Disbursement) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	d.ID = "d-stub-id"
	d.CreatedAt = time.Now()
	return nil
}
func (s *stubDisbursementRepo) FindByID(_ context.Context, _ string) (*entity.Disbursement, error) {
	return s.disbursement, s.findErr
}
func (s *stubDisbursementRepo) FindByExternalID(_ context.Context, _ entity.Provider, _ string) (*entity.Disbursement, error) {
	return s.disbursement, s.findErr
}
func (s *stubDisbursementRepo) Update(_ context.Context, _ *entity.Disbursement) error {
	return s.updateErr
}
func (s *stubDisbursementRepo) List(_ context.Context, _ repository.ListDisbursementFilter) ([]*entity.Disbursement, int64, error) {
	return nil, 0, nil
}
func (s *stubDisbursementRepo) SumPendingDisbursements(_ context.Context, _ string) (int64, error) {
	return s.pendingTotal, s.pendingErr
}
func (s *stubDisbursementRepo) SumDisbursementsToday(_ context.Context, _ string) (int64, error) {
	return s.todayTotal, s.todayErr
}

// ── stubPaymentGateway ────────────────────────────────────────────────────────

type stubPaymentGateway struct {
	vaResp      *gateway.CreateVAResponse
	vaErr       error
	qrisResp    *gateway.CreateQRISResponse
	qrisErr     error
	cancelErr   error
	webhookResp *gateway.WebhookEvent
	webhookErr  error
}

func (s *stubPaymentGateway) CreateVA(_ context.Context, req gateway.CreateVARequest) (*gateway.CreateVAResponse, error) {
	if s.vaErr != nil {
		return nil, s.vaErr
	}
	if s.vaResp != nil {
		return s.vaResp, nil
	}
	return &gateway.CreateVAResponse{
		ExternalID: req.ExternalID,
		VANumber:   "8008" + "0000000001",
		BankCode:   req.BankCode,
		Amount:     req.Amount,
		ExpiryAt:   req.ExpiryAt,
	}, nil
}
func (s *stubPaymentGateway) CreateQRIS(_ context.Context, req gateway.CreateQRISRequest) (*gateway.CreateQRISResponse, error) {
	if s.qrisErr != nil {
		return nil, s.qrisErr
	}
	if s.qrisResp != nil {
		return s.qrisResp, nil
	}
	return &gateway.CreateQRISResponse{
		ExternalID: req.ExternalID,
		QRString:   "00020101021226...",
		Amount:     req.Amount,
		ExpiryAt:   req.ExpiryAt,
	}, nil
}
func (s *stubPaymentGateway) CancelPayment(_ context.Context, _ string) error   { return s.cancelErr }
func (s *stubPaymentGateway) GetStatus(_ context.Context, _ string) (entity.PaymentStatus, error) {
	return entity.PaymentStatusPending, nil
}
func (s *stubPaymentGateway) ParseWebhook(_ context.Context, _ map[string]string, _ []byte) (*gateway.WebhookEvent, error) {
	return s.webhookResp, s.webhookErr
}
func (s *stubPaymentGateway) SupportedMethods() []entity.PaymentMethod { return nil }
func (s *stubPaymentGateway) Capabilities() []gateway.ProviderCapability {
	return []gateway.ProviderCapability{gateway.CapabilityCashIn}
}
func (s *stubPaymentGateway) ProviderName() entity.Provider { return entity.ProviderMidtrans }

// ── stubDisbursementGateway ───────────────────────────────────────────────────

type stubDisbursementGateway struct {
	disburseResp *gateway.DisburseResponse
	disburseErr  error
	webhookResp  *gateway.DisbursementWebhookEvent
	webhookErr   error
}

func (s *stubDisbursementGateway) Disburse(_ context.Context, req gateway.DisburseRequest) (*gateway.DisburseResponse, error) {
	if s.disburseErr != nil {
		return nil, s.disburseErr
	}
	if s.disburseResp != nil {
		return s.disburseResp, nil
	}
	return &gateway.DisburseResponse{
		ExternalID: "disb-ext-" + req.ExternalID,
		Status:     entity.DisbursementStatusPending,
		Amount:     req.Amount,
	}, nil
}
func (s *stubDisbursementGateway) GetDisbursementStatus(_ context.Context, _ string) (*gateway.DisburseResponse, error) {
	return nil, nil
}
func (s *stubDisbursementGateway) ParseDisbursementWebhook(_ context.Context, _ map[string]string, _ []byte) (*gateway.DisbursementWebhookEvent, error) {
	return s.webhookResp, s.webhookErr
}
func (s *stubDisbursementGateway) ProviderName() entity.Provider { return entity.ProviderXendit }

// ── helpers ───────────────────────────────────────────────────────────────────

func activeMerchant(id string) *entity.Merchant {
	return &entity.Merchant{
		ID:         id,
		Status:     entity.MerchantStatusActive,
		WebhookURL: "http://merchant.local/hook",
	}
}

func pendingMerchant(id string) *entity.Merchant {
	return &entity.Merchant{ID: id, Status: entity.MerchantStatusPending}
}

func verifiedBankAccount(id, merchantID string) *entity.MerchantBankAccount {
	return &entity.MerchantBankAccount{
		ID:            id,
		MerchantID:    merchantID,
		BankCode:      entity.BankBCA,
		AccountNumber: "1234567890",
		AccountName:   "Budi Santoso",
		IsVerified:    true,
		IsPrimary:     true,
	}
}

func pendingPayment(id, merchantID string) *entity.Payment {
	return &entity.Payment{
		ID:         id,
		MerchantID: merchantID,
		ExternalID: "ext-" + id,
		Status:     entity.PaymentStatusPending,
		Amount:     100000,
		Currency:   entity.CurrencyIDR,
		Method:     entity.PaymentMethodVA,
		Provider:   entity.ProviderMidtrans,
		ExpiryAt:   time.Now().Add(24 * time.Hour),
	}
}

func paidPayment(id, merchantID string) *entity.Payment {
	p := pendingPayment(id, merchantID)
	p.Status = entity.PaymentStatusPaid
	now := time.Now()
	p.PaidAt = &now
	return p
}

// newStubFeeResolver returns a FeeResolver backed by a stub fee repo that returns zero fees.
func newStubFeeResolver() *FeeResolver {
	return NewFeeResolver(&stubFeeRepoForUC{})
}

type stubFeeRepoForUC struct{}

func (s *stubFeeRepoForUC) GetDefault(_ context.Context) (*entity.FeeDefault, error) {
	return &entity.FeeDefault{}, nil
}
func (s *stubFeeRepoForUC) UpdateDefault(_ context.Context, _ *entity.FeeDefault) error { return nil }
func (s *stubFeeRepoForUC) GetMargin(_ context.Context) (*entity.PlatformMargin, error) {
	return &entity.PlatformMargin{}, nil
}
func (s *stubFeeRepoForUC) UpdateMargin(_ context.Context, _ *entity.PlatformMargin) error {
	return nil
}
func (s *stubFeeRepoForUC) GetHolidayByDate(_ context.Context, _ time.Time) (*entity.FeeHoliday, error) {
	return nil, apperror.NotFound("no holiday")
}
func (s *stubFeeRepoForUC) GetHolidayByID(_ context.Context, _ string) (*entity.FeeHoliday, error) {
	return nil, apperror.NotFound("no holiday")
}
func (s *stubFeeRepoForUC) ListHolidays(_ context.Context, _, _ int) ([]*entity.FeeHoliday, int64, error) {
	return nil, 0, nil
}
func (s *stubFeeRepoForUC) CreateHoliday(_ context.Context, _ *entity.FeeHoliday) error { return nil }
func (s *stubFeeRepoForUC) UpdateHoliday(_ context.Context, _ *entity.FeeHoliday) error { return nil }
func (s *stubFeeRepoForUC) WriteAuditLog(_ context.Context, _ *entity.FeeAuditLog) error {
	return nil
}
