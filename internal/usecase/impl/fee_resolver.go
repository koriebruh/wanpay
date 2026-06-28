package impl

import (
	"context"
	"fmt"

	"wanpey/core/internal/domain/entity"
	"wanpey/core/internal/domain/repository"
)

// FeeResolution is the result of resolving fees from all layers for one transaction.
type FeeResolution struct {
	BaseFee        int64  // from merchant contract or global default
	PlatformMargin int64  // Wanpey's margin on top
	TotalFee       int64  // BaseFee + PlatformMargin
	Source         string // "merchant_contract" | "global_default"
}

// FeeResolver composes merchant fee, global default, and platform margin into one total.
type FeeResolver struct {
	feeRepo repository.FeeRepository
}

func NewFeeResolver(feeRepo repository.FeeRepository) *FeeResolver {
	return &FeeResolver{feeRepo: feeRepo}
}

// Resolve returns the effective fee for a payment transaction.
// Priority: merchant custom FeeConfig > global FeeDefault.
// Platform margin is always added on top when enabled.
func (r *FeeResolver) Resolve(
	ctx context.Context,
	merchant *entity.Merchant,
	method entity.PaymentMethod,
	amount int64,
) (FeeResolution, error) {
	merchantFee, source, err := r.merchantFee(ctx, merchant, method)
	if err != nil {
		return FeeResolution{}, err
	}

	baseFee := computeMethodFee(merchantFee, amount)

	margin, err := r.feeRepo.GetMargin(ctx)
	if err != nil {
		return FeeResolution{}, fmt.Errorf("get platform margin: %w", err)
	}

	var platformFee int64
	if margin.Enabled {
		platformFee = computeMethodFee(r.marginFee(margin, method), amount)
	}

	return FeeResolution{
		BaseFee:        baseFee,
		PlatformMargin: platformFee,
		TotalFee:       baseFee + platformFee,
		Source:         source,
	}, nil
}

// ResolveDisbursement returns the effective fee for a disbursement transaction.
func (r *FeeResolver) ResolveDisbursement(
	ctx context.Context,
	merchant *entity.Merchant,
	amount int64,
) (FeeResolution, error) {
	var disbFee entity.MethodFee
	source := "merchant_contract"

	if isZeroFee(merchant.FeeConfig.Disbursement) {
		def, err := r.feeRepo.GetDefault(ctx)
		if err != nil {
			return FeeResolution{}, fmt.Errorf("get fee default: %w", err)
		}
		disbFee = def.Disbursement
		source = "global_default"
	} else {
		disbFee = merchant.FeeConfig.Disbursement
	}

	baseFee := computeMethodFee(disbFee, amount)

	margin, err := r.feeRepo.GetMargin(ctx)
	if err != nil {
		return FeeResolution{}, fmt.Errorf("get platform margin: %w", err)
	}

	var platformFee int64
	if margin.Enabled {
		platformFee = computeMethodFee(margin.Disbursement, amount)
	}

	return FeeResolution{
		BaseFee:        baseFee,
		PlatformMargin: platformFee,
		TotalFee:       baseFee + platformFee,
		Source:         source,
	}, nil
}

func (r *FeeResolver) merchantFee(
	ctx context.Context,
	merchant *entity.Merchant,
	method entity.PaymentMethod,
) (entity.MethodFee, string, error) {
	switch method {
	case entity.PaymentMethodVA:
		if !isZeroFee(merchant.FeeConfig.VA) {
			return merchant.FeeConfig.VA, "merchant_contract", nil
		}
	case entity.PaymentMethodQRIS:
		if !isZeroFee(merchant.FeeConfig.QRIS) {
			return merchant.FeeConfig.QRIS, "merchant_contract", nil
		}
	}
	// Fall through to global default
	def, err := r.feeRepo.GetDefault(ctx)
	if err != nil {
		return entity.MethodFee{}, "", fmt.Errorf("get fee default: %w", err)
	}
	switch method {
	case entity.PaymentMethodVA:
		return def.VA, "global_default", nil
	case entity.PaymentMethodQRIS:
		return def.QRIS, "global_default", nil
	}
	return entity.MethodFee{}, "global_default", nil
}

func (r *FeeResolver) marginFee(m *entity.PlatformMargin, method entity.PaymentMethod) entity.MethodFee {
	switch method {
	case entity.PaymentMethodVA:
		return m.VA
	case entity.PaymentMethodQRIS:
		return m.QRIS
	}
	return entity.MethodFee{}
}

// isZeroFee returns true if MethodFee has no meaningful configuration.
// A merchant with zero-initialized FeeConfig should use the global default.
func isZeroFee(f entity.MethodFee) bool {
	return f.Type == "" || (f.Amount == 0 && f.Percentage == 0)
}
