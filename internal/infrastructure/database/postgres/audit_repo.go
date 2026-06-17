package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"wanpey/core/internal/domain/entity"
	"wanpey/core/internal/domain/repository"
	"wanpey/core/internal/infrastructure/database"
	"wanpey/core/internal/infrastructure/database/postgres/gen"
)

type auditRepo struct {
	db database.SQLDB
	q  *gen.Queries
}

func NewAuditRepo(db database.SQLDB) repository.AuditRepository {
	return &auditRepo{db: db, q: gen.New(db)}
}

func (r *auditRepo) queries(ctx context.Context) *gen.Queries {
	if tx := database.TxFromContext(ctx); tx != nil {
		return r.q.WithTx(tx)
	}
	return r.q
}

func (r *auditRepo) Save(ctx context.Context, a *entity.PaymentAudit) error {
	meta, err := json.Marshal(a.Metadata)
	if err != nil {
		return fmt.Errorf("marshal audit metadata: %w", err)
	}

	var oldStatus *string
	if a.OldStatus != nil {
		s := string(*a.OldStatus)
		oldStatus = &s
	}

	row, err := r.queries(ctx).InsertPaymentAudit(ctx, gen.InsertPaymentAuditParams{
		PaymentID: a.PaymentID,
		EventType: string(a.EventType),
		OldStatus: toNullString(oldStatus),
		NewStatus: string(a.NewStatus),
		Actor:     a.Actor,
		Metadata:  meta,
	})
	if err != nil {
		return fmt.Errorf("insert payment audit: %w", err)
	}
	a.ID = row.ID
	a.CreatedAt = row.CreatedAt
	return nil
}

func (r *auditRepo) FindByPaymentID(ctx context.Context, paymentID string) ([]*entity.PaymentAudit, error) {
	rows, err := r.queries(ctx).ListPaymentAuditsByPaymentID(ctx, paymentID)
	if err != nil {
		return nil, fmt.Errorf("list payment audits: %w", err)
	}
	result := make([]*entity.PaymentAudit, len(rows))
	for i, a := range rows {
		result[i] = toEntityPaymentAudit(a)
	}
	return result, nil
}
