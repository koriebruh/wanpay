//go:build !integration

package impl

import (
	"context"
	"errors"
	"testing"
	"time"

	"wanpey/core/internal/domain/entity"
)

// stubFeeRepo implements repository.FeeRepository for unit tests.
// Only GetDefault, GetMargin, and GetHolidayByDate are exercised by FeeResolver.
type stubFeeRepo struct {
	def          *entity.FeeDefault
	margin       *entity.PlatformMargin
	holiday      *entity.FeeHoliday // nil = no holiday
	defaultErr   error
	marginErr    error
}

func (s *stubFeeRepo) GetDefault(ctx context.Context) (*entity.FeeDefault, error) {
	return s.def, s.defaultErr
}
func (s *stubFeeRepo) UpdateDefault(ctx context.Context, f *entity.FeeDefault) error { return nil }
func (s *stubFeeRepo) GetMargin(ctx context.Context) (*entity.PlatformMargin, error) {
	return s.margin, s.marginErr
}
func (s *stubFeeRepo) UpdateMargin(ctx context.Context, m *entity.PlatformMargin) error { return nil }
func (s *stubFeeRepo) CreateHoliday(ctx context.Context, h *entity.FeeHoliday) error   { return nil }
func (s *stubFeeRepo) GetHolidayByDate(ctx context.Context, date time.Time) (*entity.FeeHoliday, error) {
	if s.holiday == nil {
		return nil, errors.New("no holiday")
	}
	return s.holiday, nil
}
func (s *stubFeeRepo) GetHolidayByID(ctx context.Context, id string) (*entity.FeeHoliday, error) {
	return nil, nil
}
func (s *stubFeeRepo) UpdateHoliday(ctx context.Context, h *entity.FeeHoliday) error { return nil }
func (s *stubFeeRepo) ListHolidays(ctx context.Context, page, limit int) ([]*entity.FeeHoliday, int64, error) {
	return nil, 0, nil
}
func (s *stubFeeRepo) WriteAuditLog(ctx context.Context, l *entity.FeeAuditLog) error { return nil }

// helpers for building test data

func flatFee(amount int64) entity.MethodFee {
	return entity.MethodFee{Type: entity.FeeTypeFlat, Amount: amount}
}

func pctFee(pct float64) entity.MethodFee {
	return entity.MethodFee{Type: entity.FeeTypePercentage, Percentage: pct}
}

func defaultFeeRepo() *stubFeeRepo {
	return &stubFeeRepo{
		def: &entity.FeeDefault{
			VA:           flatFee(2000),
			QRIS:         pctFee(0.7),
			Disbursement: flatFee(5000),
		},
		margin: &entity.PlatformMargin{Enabled: false},
	}
}

func merchantWithVAFee(amount int64) *entity.Merchant {
	return &entity.Merchant{
		ID: "merchant-1",
		FeeConfig: entity.FeeConfig{
			VA: flatFee(amount),
		},
	}
}

func merchantWithNoFee() *entity.Merchant {
	return &entity.Merchant{ID: "merchant-2"}
}

// -- Tests --

func TestFeeResolver_VA_MerchantContract(t *testing.T) {
	repo := defaultFeeRepo()
	r := NewFeeResolver(repo)

	merchant := merchantWithVAFee(1500) // merchant has 1500 flat
	res, err := r.resolve(context.Background(), merchant, entity.PaymentMethodVA, 100000, time.Now())
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if res.BaseFee != 1500 {
		t.Errorf("BaseFee = %d, want 1500", res.BaseFee)
	}
	if res.Source != "merchant_contract" {
		t.Errorf("Source = %q, want merchant_contract", res.Source)
	}
	if res.TotalFee != 1500 {
		t.Errorf("TotalFee = %d, want 1500 (no margin, no holiday)", res.TotalFee)
	}
}

func TestFeeResolver_VA_FallsBackToGlobalDefault(t *testing.T) {
	repo := defaultFeeRepo()
	r := NewFeeResolver(repo)

	merchant := merchantWithNoFee()
	res, err := r.resolve(context.Background(), merchant, entity.PaymentMethodVA, 100000, time.Now())
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if res.BaseFee != 2000 {
		t.Errorf("BaseFee = %d, want 2000 (global default)", res.BaseFee)
	}
	if res.Source != "global_default" {
		t.Errorf("Source = %q, want global_default", res.Source)
	}
}

func TestFeeResolver_QRIS_MerchantContract(t *testing.T) {
	repo := defaultFeeRepo()
	r := NewFeeResolver(repo)

	merchant := &entity.Merchant{
		ID: "m",
		FeeConfig: entity.FeeConfig{
			QRIS: pctFee(0.5), // merchant: 0.5%
		},
	}
	// 0.5% of 200000 = 1000
	res, err := r.resolve(context.Background(), merchant, entity.PaymentMethodQRIS, 200000, time.Now())
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if res.BaseFee != 1000 {
		t.Errorf("BaseFee = %d, want 1000", res.BaseFee)
	}
	if res.Source != "merchant_contract" {
		t.Errorf("Source = %q, want merchant_contract", res.Source)
	}
}

func TestFeeResolver_QRIS_GlobalDefault(t *testing.T) {
	repo := defaultFeeRepo() // global QRIS = 0.7%
	r := NewFeeResolver(repo)

	merchant := merchantWithNoFee()
	// 0.7% of 100000 = 700
	res, err := r.resolve(context.Background(), merchant, entity.PaymentMethodQRIS, 100000, time.Now())
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if res.BaseFee != 700 {
		t.Errorf("BaseFee = %d, want 700 (0.7%% of 100000)", res.BaseFee)
	}
}

func TestFeeResolver_PlatformMargin_Added(t *testing.T) {
	repo := defaultFeeRepo()
	repo.margin = &entity.PlatformMargin{
		Enabled: true,
		VA:      flatFee(500),
	}
	r := NewFeeResolver(repo)

	merchant := merchantWithVAFee(1500)
	res, err := r.resolve(context.Background(), merchant, entity.PaymentMethodVA, 100000, time.Now())
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if res.PlatformMargin != 500 {
		t.Errorf("PlatformMargin = %d, want 500", res.PlatformMargin)
	}
	if res.TotalFee != 2000 { // 1500 base + 500 margin
		t.Errorf("TotalFee = %d, want 2000", res.TotalFee)
	}
}

func TestFeeResolver_PlatformMargin_Disabled(t *testing.T) {
	repo := defaultFeeRepo()
	repo.margin = &entity.PlatformMargin{
		Enabled: false,
		VA:      flatFee(500), // configured but disabled
	}
	r := NewFeeResolver(repo)

	merchant := merchantWithVAFee(1500)
	res, err := r.resolve(context.Background(), merchant, entity.PaymentMethodVA, 100000, time.Now())
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if res.PlatformMargin != 0 {
		t.Errorf("PlatformMargin = %d, want 0 (disabled)", res.PlatformMargin)
	}
	if res.TotalFee != 1500 {
		t.Errorf("TotalFee = %d, want 1500 (base only)", res.TotalFee)
	}
}

func TestFeeResolver_HolidaySurcharge_Added(t *testing.T) {
	repo := defaultFeeRepo()
	repo.holiday = &entity.FeeHoliday{
		Name:      "Idul Fitri",
		IsActive:  true,
		Surcharge: flatFee(1000),
	}
	r := NewFeeResolver(repo)

	merchant := merchantWithVAFee(2000)
	res, err := r.resolve(context.Background(), merchant, entity.PaymentMethodVA, 100000, time.Now())
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if res.HolidaySurcharge != 1000 {
		t.Errorf("HolidaySurcharge = %d, want 1000", res.HolidaySurcharge)
	}
	if res.HolidayName != "Idul Fitri" {
		t.Errorf("HolidayName = %q, want Idul Fitri", res.HolidayName)
	}
	if res.TotalFee != 3000 { // 2000 base + 1000 holiday
		t.Errorf("TotalFee = %d, want 3000", res.TotalFee)
	}
}

func TestFeeResolver_HolidayInactive_Ignored(t *testing.T) {
	repo := defaultFeeRepo()
	repo.holiday = &entity.FeeHoliday{
		Name:      "Inactive Holiday",
		IsActive:  false, // inactive — must not be applied
		Surcharge: flatFee(5000),
	}
	r := NewFeeResolver(repo)

	merchant := merchantWithVAFee(2000)
	res, err := r.resolve(context.Background(), merchant, entity.PaymentMethodVA, 100000, time.Now())
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if res.HolidaySurcharge != 0 {
		t.Errorf("HolidaySurcharge = %d, want 0 (inactive holiday)", res.HolidaySurcharge)
	}
}

func TestFeeResolver_NoHolidayToday(t *testing.T) {
	repo := defaultFeeRepo()
	repo.holiday = nil // GetHolidayByDate will return error → no surcharge
	r := NewFeeResolver(repo)

	merchant := merchantWithVAFee(2000)
	res, err := r.resolve(context.Background(), merchant, entity.PaymentMethodVA, 100000, time.Now())
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if res.HolidaySurcharge != 0 || res.HolidayName != "" {
		t.Errorf("no holiday: Surcharge=%d HolidayName=%q, want 0 and empty", res.HolidaySurcharge, res.HolidayName)
	}
}

func TestFeeResolver_AllLayersStacked(t *testing.T) {
	// Merchant contract: flat 3000
	// Platform margin:   flat 500 (enabled)
	// Holiday:           flat 1000 (active)
	// Total:             4500
	repo := &stubFeeRepo{
		def: &entity.FeeDefault{VA: flatFee(9999)}, // not used — merchant has contract
		margin: &entity.PlatformMargin{
			Enabled: true,
			VA:      flatFee(500),
		},
		holiday: &entity.FeeHoliday{
			Name:      "New Year",
			IsActive:  true,
			Surcharge: flatFee(1000),
		},
	}
	r := NewFeeResolver(repo)

	merchant := merchantWithVAFee(3000)
	res, err := r.resolve(context.Background(), merchant, entity.PaymentMethodVA, 100000, time.Now())
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if res.BaseFee != 3000 {
		t.Errorf("BaseFee = %d, want 3000", res.BaseFee)
	}
	if res.PlatformMargin != 500 {
		t.Errorf("PlatformMargin = %d, want 500", res.PlatformMargin)
	}
	if res.HolidaySurcharge != 1000 {
		t.Errorf("HolidaySurcharge = %d, want 1000", res.HolidaySurcharge)
	}
	if res.TotalFee != 4500 {
		t.Errorf("TotalFee = %d, want 4500", res.TotalFee)
	}
	if res.Source != "merchant_contract" {
		t.Errorf("Source = %q, want merchant_contract", res.Source)
	}
	if res.HolidayName != "New Year" {
		t.Errorf("HolidayName = %q, want New Year", res.HolidayName)
	}
}

func TestFeeResolver_DefaultErr_ReturnsError(t *testing.T) {
	repo := &stubFeeRepo{
		defaultErr: errors.New("db down"),
		margin:     &entity.PlatformMargin{},
	}
	r := NewFeeResolver(repo)

	merchant := merchantWithNoFee() // will need global default
	_, err := r.resolve(context.Background(), merchant, entity.PaymentMethodVA, 100000, time.Now())
	if err == nil {
		t.Error("expected error when GetDefault fails")
	}
}

func TestFeeResolver_MarginErr_ReturnsError(t *testing.T) {
	repo := &stubFeeRepo{
		def:       &entity.FeeDefault{VA: flatFee(2000)},
		marginErr: errors.New("margin db error"),
	}
	r := NewFeeResolver(repo)

	merchant := merchantWithNoFee()
	_, err := r.resolve(context.Background(), merchant, entity.PaymentMethodVA, 100000, time.Now())
	if err == nil {
		t.Error("expected error when GetMargin fails")
	}
}

// -- ResolveDisbursement --

func TestResolveDisbursement_MerchantContract(t *testing.T) {
	repo := defaultFeeRepo()
	repo.margin = &entity.PlatformMargin{Enabled: false}
	r := NewFeeResolver(repo)

	merchant := &entity.Merchant{
		ID:        "m",
		FeeConfig: entity.FeeConfig{Disbursement: flatFee(3500)},
	}
	res, err := r.ResolveDisbursement(context.Background(), merchant, 100000)
	if err != nil {
		t.Fatalf("ResolveDisbursement: %v", err)
	}
	if res.BaseFee != 3500 {
		t.Errorf("BaseFee = %d, want 3500", res.BaseFee)
	}
	if res.Source != "merchant_contract" {
		t.Errorf("Source = %q, want merchant_contract", res.Source)
	}
}

func TestResolveDisbursement_GlobalDefault(t *testing.T) {
	repo := defaultFeeRepo() // global disb = 5000 flat
	repo.margin = &entity.PlatformMargin{Enabled: false}
	r := NewFeeResolver(repo)

	merchant := merchantWithNoFee()
	res, err := r.ResolveDisbursement(context.Background(), merchant, 100000)
	if err != nil {
		t.Fatalf("ResolveDisbursement: %v", err)
	}
	if res.BaseFee != 5000 {
		t.Errorf("BaseFee = %d, want 5000 (global default)", res.BaseFee)
	}
	if res.Source != "global_default" {
		t.Errorf("Source = %q, want global_default", res.Source)
	}
}

func TestResolveDisbursement_WithMarginAndHoliday(t *testing.T) {
	repo := &stubFeeRepo{
		def: &entity.FeeDefault{Disbursement: flatFee(5000)},
		margin: &entity.PlatformMargin{
			Enabled:      true,
			Disbursement: flatFee(1000),
		},
		holiday: &entity.FeeHoliday{
			IsActive:  true,
			Surcharge: flatFee(500),
		},
	}
	r := NewFeeResolver(repo)

	merchant := merchantWithNoFee()
	res, err := r.ResolveDisbursement(context.Background(), merchant, 100000)
	if err != nil {
		t.Fatalf("ResolveDisbursement: %v", err)
	}
	if res.TotalFee != 6500 { // 5000 + 1000 + 500
		t.Errorf("TotalFee = %d, want 6500", res.TotalFee)
	}
}
