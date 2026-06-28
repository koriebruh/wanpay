package impl

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"wanpey/core/internal/domain/entity"
	"wanpey/core/internal/usecase"
)

// externalID generates a unique payment/disbursement reference.
func externalID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	return fmt.Sprintf("wpay-%d-%s", time.Now().UnixNano(), hex.EncodeToString(b))
}

// generateAPIKey returns (rawKey, hashedKey).
// Format: wpay_live_<32 random hex chars> | wpay_test_<32 random hex chars>
func generateAPIKey(isProduction bool) (raw, hashed string) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	suffix := hex.EncodeToString(b)
	prefix := "wpay_test_"
	if isProduction {
		prefix = "wpay_live_"
	}
	raw = prefix + suffix
	h := sha256.Sum256([]byte(raw))
	hashed = hex.EncodeToString(h[:])
	return
}

// generateSecret returns (rawSecret, hashedSecret).
func generateSecret() (raw, hashed string) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	raw = hex.EncodeToString(b)
	h := sha256.Sum256([]byte(raw))
	hashed = hex.EncodeToString(h[:])
	return
}

// computeMethodFee calculates the fee for a single payment method.
func computeMethodFee(mf entity.MethodFee, amount int64) int64 {
	switch mf.Type {
	case entity.FeeTypeFlat:
		if mf.Amount > amount {
			return amount // cap: fee cannot exceed the transaction amount
		}
		return mf.Amount
	case entity.FeeTypePercentage:
		return int64(float64(amount) * mf.Percentage / 100)
	}
	return 0
}

func toPaymentOutput(p *entity.Payment) *usecase.PaymentOutput {
	return &usecase.PaymentOutput{
		ID:            p.ID,
		ExternalID:    p.ExternalID,
		Method:        p.Method,
		Provider:      p.Provider,
		Status:        p.Status,
		Amount:        p.Amount,
		FeeAmount:     p.FeeAmount,
		Currency:      p.Currency,
		CustomerName:  p.CustomerName,
		CustomerEmail: p.CustomerEmail,
		VANumber:      p.VANumber,
		BankCode:      p.BankCode,
		QRString:      p.QRString,
		QRImageURL:    p.QRImageURL,
		ExpiryAt:      p.ExpiryAt,
		PaidAt:        p.PaidAt,
		CancelledAt:   p.CancelledAt,
		CreatedAt:     p.CreatedAt,
	}
}

func toDisbursementOutput(d *entity.Disbursement) *usecase.DisbursementOutput {
	return &usecase.DisbursementOutput{
		ID:            d.ID,
		ExternalID:    d.ExternalID,
		Provider:      d.Provider,
		Status:        d.Status,
		BankCode:      d.BankCode,
		AccountNumber: d.AccountNumber,
		AccountName:   d.AccountName,
		Amount:        d.Amount,
		FeeAmount:     d.FeeAmount,
		Currency:      d.Currency,
		Description:   d.Description,
		FailureReason: d.FailureReason,
		CompletedAt:   d.CompletedAt,
		CreatedAt:     d.CreatedAt,
	}
}

func toMutationOutput(m *entity.Mutation) *usecase.MutationOutput {
	return &usecase.MutationOutput{
		ID:            m.ID,
		ReferenceID:   m.ReferenceID,
		ReferenceType: m.ReferenceType,
		Type:          m.Type,
		Amount:        m.Amount,
		FeeAmount:     m.FeeAmount,
		NetAmount:     m.NetAmount(),
		Currency:      m.Currency,
		Description:   m.Description,
		CreatedAt:     m.CreatedAt,
	}
}

func toMerchantOutput(m *entity.Merchant, balance int64) *usecase.MerchantOutput {
	return &usecase.MerchantOutput{
		ID:         m.ID,
		Name:       m.Name,
		Email:      m.Email,
		Phone:      m.Phone,
		Status:     m.Status,
		FeeConfig:  m.FeeConfig,
		WebhookURL: m.WebhookURL,
		Balance:    balance,
		CreatedAt:  m.CreatedAt,
		UpdatedAt:  m.UpdatedAt,
	}
}

func toBankAccountOutput(a *entity.MerchantBankAccount) *usecase.BankAccountOutput {
	return &usecase.BankAccountOutput{
		ID:            a.ID,
		BankCode:      a.BankCode,
		AccountNumber: a.AccountNumber,
		AccountName:   a.AccountName,
		IsPrimary:     a.IsPrimary,
		IsVerified:    a.IsVerified,
		CreatedAt:     a.CreatedAt,
	}
}
