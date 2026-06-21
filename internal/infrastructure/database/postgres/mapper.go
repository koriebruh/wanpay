package postgres

import (
	"database/sql"
	"encoding/json"
	"time"

	"wanpey/core/internal/domain/entity"
	"wanpey/core/internal/infrastructure/database/postgres/gen"
)

// nullTime converts sql.NullTime to *time.Time.
func nullTime(t sql.NullTime) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

// nullString converts sql.NullString to *string.
func nullString(s sql.NullString) *string {
	if !s.Valid {
		return nil
	}
	v := s.String
	return &v
}

// toNullTime converts *time.Time to sql.NullTime.
func toNullTime(t *time.Time) sql.NullTime {
	if t == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *t, Valid: true}
}

// toNullString converts *string to sql.NullString.
func toNullString(s *string) sql.NullString {
	if s == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *s, Valid: true}
}

// ── Payment ──────────────────────────────────────────────────────────────────

func toEntityPayment(p gen.Payment) *entity.Payment {
	ep := &entity.Payment{
		ID:            p.ID,
		MerchantID:    p.MerchantID,
		ExternalID:    p.ExternalID,
		Method:        entity.PaymentMethod(p.Method),
		Provider:      entity.Provider(p.Provider),
		Status:        entity.PaymentStatus(p.Status),
		Amount:        p.Amount,
		FeeAmount:     p.FeeAmount,
		Currency:      entity.Currency(p.Currency),
		Description:   p.Description,
		CustomerName:  p.CustomerName,
		CustomerEmail: p.CustomerEmail,
		CustomerPhone: p.CustomerPhone,
		VANumber:      p.VaNumber,
		BankCode:      entity.BankCode(p.BankCode),
		QRString:      p.QrString,
		QRImageURL:    p.QrImageUrl,
		ExpiryAt:      p.ExpiryAt,
		PaidAt:        nullTime(p.PaidAt),
		FailedAt:      nullTime(p.FailedAt),
		CancelledAt:   nullTime(p.CancelledAt),
		CreatedAt:     p.CreatedAt,
		UpdatedAt:     p.UpdatedAt,
	}
	if len(p.Metadata) > 0 {
		if err := json.Unmarshal(p.Metadata, &ep.Metadata); err != nil {
			ep.Metadata = map[string]any{}
		}
	}
	return ep
}

// ── Disbursement ─────────────────────────────────────────────────────────────

func toEntityDisbursement(d gen.Disbursement) *entity.Disbursement {
	return &entity.Disbursement{
		ID:            d.ID,
		MerchantID:    d.MerchantID,
		ExternalID:    d.ExternalID,
		Provider:      entity.Provider(d.Provider),
		Status:        entity.DisbursementStatus(d.Status),
		BankCode:      entity.BankCode(d.BankCode),
		AccountNumber: d.AccountNumber,
		AccountName:   d.AccountName,
		Amount:        d.Amount,
		FeeAmount:     d.FeeAmount,
		Currency:      entity.Currency(d.Currency),
		Description:   d.Description,
		FailureReason: d.FailureReason,
		CompletedAt:   nullTime(d.CompletedAt),
		FailedAt:      nullTime(d.FailedAt),
		CreatedAt:     d.CreatedAt,
		UpdatedAt:     d.UpdatedAt,
	}
}

// ── Merchant ─────────────────────────────────────────────────────────────────

func toEntityMerchant(m gen.Merchant) *entity.Merchant {
	em := &entity.Merchant{
		ID:                m.ID,
		Name:              m.Name,
		Email:             m.Email,
		Phone:             m.Phone,
		Status:            entity.MerchantStatus(m.Status),
		APIKey:            m.ApiKey,
		WebhookURL:        m.WebhookUrl,
		WebhookSecret:     m.WebhookSecret,
		DailyCashoutLimit: m.DailyCashoutLimit,
		DeletedAt:         nullTime(m.DeletedAt),
		CreatedAt:         m.CreatedAt,
		UpdatedAt:         m.UpdatedAt,
	}
	if len(m.FeeConfig) > 0 {
		if err := json.Unmarshal(m.FeeConfig, &em.FeeConfig); err != nil {
			em.FeeConfig = entity.FeeConfig{}
		}
	}
	return em
}

func toEntityBankAccount(b gen.MerchantBankAccount) *entity.MerchantBankAccount {
	return &entity.MerchantBankAccount{
		ID:            b.ID,
		MerchantID:    b.MerchantID,
		BankCode:      entity.BankCode(b.BankCode),
		AccountNumber: b.AccountNumber,
		AccountName:   b.AccountName,
		IsPrimary:     b.IsPrimary,
		IsVerified:    b.IsVerified,
		CreatedAt:     b.CreatedAt,
		UpdatedAt:     b.UpdatedAt,
	}
}

// ── Mutation ─────────────────────────────────────────────────────────────────

func toEntityMutation(m gen.Mutation) *entity.Mutation {
	return &entity.Mutation{
		ID:            m.ID,
		ReferenceID:   m.ReferenceID,
		ReferenceType: entity.MutationReferenceType(m.ReferenceType),
		MerchantID:    m.MerchantID,
		Type:          entity.MutationType(m.Type),
		Amount:        m.Amount,
		FeeAmount:     m.FeeAmount,
		Currency:      entity.Currency(m.Currency),
		Description:   m.Description,
		CreatedAt:     m.CreatedAt,
	}
}

// ── PaymentAudit ─────────────────────────────────────────────────────────────

func toEntityPaymentAudit(a gen.PaymentAudit) *entity.PaymentAudit {
	ea := &entity.PaymentAudit{
		ID:        a.ID,
		PaymentID: a.PaymentID,
		EventType: entity.AuditEventType(a.EventType),
		NewStatus: entity.PaymentStatus(a.NewStatus),
		Actor:     a.Actor,
		CreatedAt: a.CreatedAt,
	}
	if s := nullString(a.OldStatus); s != nil {
		st := entity.PaymentStatus(*s)
		ea.OldStatus = &st
	}
	if len(a.Metadata) > 0 {
		if err := json.Unmarshal(a.Metadata, &ea.Metadata); err != nil {
			ea.Metadata = map[string]any{}
		}
	}
	return ea
}

func toEntityProviderBalance(b gen.ProviderBalance) *entity.ProviderBalance {
	return &entity.ProviderBalance{
		ID:               b.ID,
		Provider:         entity.Provider(b.Provider),
		BalanceIDR:       b.BalanceIdr,
		LastReconciledAt: nullTime(b.LastReconciledAt),
		Note:             b.Note,
		CreatedAt:        b.CreatedAt,
		UpdatedAt:        b.UpdatedAt,
	}
}
