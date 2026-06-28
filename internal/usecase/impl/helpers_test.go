//go:build !integration

package impl

import (
	"testing"

	"wanpey/core/internal/domain/entity"
)

func TestComputeMethodFee_Flat(t *testing.T) {
	mf := entity.MethodFee{Type: entity.FeeTypeFlat, Amount: 2500}
	got := computeMethodFee(mf, 100000)
	if got != 2500 {
		t.Errorf("flat fee = %d, want 2500", got)
	}
}

func TestComputeMethodFee_FlatCapAtAmount(t *testing.T) {
	// Fee must not exceed the transaction amount
	mf := entity.MethodFee{Type: entity.FeeTypeFlat, Amount: 5000}
	got := computeMethodFee(mf, 3000) // amount < fee
	if got != 3000 {
		t.Errorf("capped flat fee = %d, want 3000 (capped at amount)", got)
	}
}

func TestComputeMethodFee_FlatExactAmount(t *testing.T) {
	mf := entity.MethodFee{Type: entity.FeeTypeFlat, Amount: 1000}
	got := computeMethodFee(mf, 1000)
	if got != 1000 {
		t.Errorf("flat == amount = %d, want 1000", got)
	}
}

func TestComputeMethodFee_Percentage(t *testing.T) {
	// 1% of 100000 = 1000
	mf := entity.MethodFee{Type: entity.FeeTypePercentage, Percentage: 1.0}
	got := computeMethodFee(mf, 100000)
	if got != 1000 {
		t.Errorf("1%% of 100000 = %d, want 1000", got)
	}
}

func TestComputeMethodFee_Percentage_Fractional(t *testing.T) {
	// 0.7% of 500000 = 3500
	mf := entity.MethodFee{Type: entity.FeeTypePercentage, Percentage: 0.7}
	got := computeMethodFee(mf, 500000)
	if got != 3500 {
		t.Errorf("0.7%% of 500000 = %d, want 3500", got)
	}
}

func TestComputeMethodFee_ZeroPercentage(t *testing.T) {
	mf := entity.MethodFee{Type: entity.FeeTypePercentage, Percentage: 0.0}
	got := computeMethodFee(mf, 100000)
	if got != 0 {
		t.Errorf("0%% fee = %d, want 0", got)
	}
}

func TestComputeMethodFee_UnknownType(t *testing.T) {
	mf := entity.MethodFee{Type: "unknown", Amount: 9999}
	got := computeMethodFee(mf, 100000)
	if got != 0 {
		t.Errorf("unknown fee type = %d, want 0", got)
	}
}

func TestComputeMethodFee_EmptyType(t *testing.T) {
	mf := entity.MethodFee{}
	got := computeMethodFee(mf, 100000)
	if got != 0 {
		t.Errorf("empty MethodFee = %d, want 0", got)
	}
}

func TestIsZeroFee_EmptyType(t *testing.T) {
	if !isZeroFee(entity.MethodFee{}) {
		t.Error("empty MethodFee should be zero fee")
	}
}

func TestIsZeroFee_ZeroAmount(t *testing.T) {
	if !isZeroFee(entity.MethodFee{Type: entity.FeeTypeFlat, Amount: 0}) {
		t.Error("flat fee with Amount=0 should be zero fee")
	}
}

func TestIsZeroFee_ZeroPercentage(t *testing.T) {
	if !isZeroFee(entity.MethodFee{Type: entity.FeeTypePercentage, Percentage: 0}) {
		t.Error("percentage fee with 0% should be zero fee")
	}
}

func TestIsZeroFee_NonZeroFlat(t *testing.T) {
	if isZeroFee(entity.MethodFee{Type: entity.FeeTypeFlat, Amount: 1}) {
		t.Error("flat fee with Amount=1 should NOT be zero fee")
	}
}

func TestIsZeroFee_NonZeroPercentage(t *testing.T) {
	if isZeroFee(entity.MethodFee{Type: entity.FeeTypePercentage, Percentage: 0.1}) {
		t.Error("percentage fee with 0.1%% should NOT be zero fee")
	}
}
