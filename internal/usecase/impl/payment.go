package impl

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"wanpey/core/internal/domain/entity"
	"wanpey/core/internal/domain/gateway"
	"wanpey/core/internal/domain/repository"
	"wanpey/core/internal/infrastructure/database"
	"wanpey/core/internal/infrastructure/database/postgres"
	"wanpey/core/internal/usecase"
	"wanpey/core/pkg/apperror"
	"wanpey/core/pkg/webhook"
)

type paymentUsecase struct {
	gateways     map[entity.Provider]gateway.PaymentGateway
	paymentRepo  repository.PaymentRepository
	mutationRepo repository.MutationRepository
	auditRepo    repository.AuditRepository
	outboxRepo   *postgres.OutboxRepo
	merchantRepo repository.MerchantRepository
	feeResolver  *FeeResolver
	db           database.SQLDB
	log          *zap.Logger
}

func NewPaymentUsecase(
	gateways map[entity.Provider]gateway.PaymentGateway,
	paymentRepo repository.PaymentRepository,
	mutationRepo repository.MutationRepository,
	auditRepo repository.AuditRepository,
	outboxRepo *postgres.OutboxRepo,
	merchantRepo repository.MerchantRepository,
	feeResolver *FeeResolver,
	db database.SQLDB,
	log *zap.Logger,
) usecase.PaymentUsecase {
	return &paymentUsecase{
		gateways:     gateways,
		paymentRepo:  paymentRepo,
		mutationRepo: mutationRepo,
		auditRepo:    auditRepo,
		outboxRepo:   outboxRepo,
		merchantRepo: merchantRepo,
		feeResolver:  feeResolver,
		db:           db,
		log:          log,
	}
}

func (u *paymentUsecase) gateway(provider entity.Provider) (gateway.PaymentGateway, error) {
	gw, ok := u.gateways[provider]
	if !ok || gw == nil {
		return nil, apperror.BadRequest("provider %s is not enabled", provider)
	}
	return gw, nil
}

func (u *paymentUsecase) CreateVA(ctx context.Context, input usecase.CreateVAInput) (*usecase.PaymentOutput, error) {
	merchant, err := u.merchantRepo.FindByID(ctx, input.MerchantID)
	if err != nil {
		return nil, err
	}
	if !merchant.CanTransact() {
		return nil, apperror.Forbidden("merchant account is not active or webhook URL not set")
	}

	gw, err := u.gateway(input.Provider)
	if err != nil {
		return nil, err
	}

	extID := externalID()
	resp, err := gw.CreateVA(ctx, gateway.CreateVARequest{
		ExternalID:    extID,
		BankCode:      input.BankCode,
		Amount:        input.Amount,
		Currency:      input.Currency,
		CustomerName:  input.CustomerName,
		CustomerEmail: input.CustomerEmail,
		CustomerPhone: input.CustomerPhone,
		Description:   input.Description,
		ExpiryAt:      input.ExpiryAt,
	})
	if err != nil {
		return nil, fmt.Errorf("create VA via %s: %w", input.Provider, err)
	}

	p := &entity.Payment{
		MerchantID:        input.MerchantID,
		ExternalID:        resp.ExternalID,
		ProviderPaymentID: resp.ProviderPaymentID,
		Method:            entity.PaymentMethodVA,
		Provider:          input.Provider,
		Status:            entity.PaymentStatusPending,
		Amount:            input.Amount,
		Currency:          input.Currency,
		Description:       input.Description,
		CustomerName:      input.CustomerName,
		CustomerEmail:     input.CustomerEmail,
		CustomerPhone:     input.CustomerPhone,
		VANumber:          resp.VANumber,
		BankCode:          input.BankCode,
		ExpiryAt:          resp.ExpiryAt,
	}
	if input.CallbackURL != "" {
		p.Metadata = map[string]any{"callback_url": input.CallbackURL}
	}
	if err := u.savePaymentWithAudit(ctx, p, input.MerchantID); err != nil {
		return nil, err
	}

	u.log.Info("VA payment created",
		zap.String("payment_id", p.ID),
		zap.String("provider", string(input.Provider)),
		zap.String("va_number", resp.VANumber),
	)
	return toPaymentOutput(p), nil
}

func (u *paymentUsecase) CreateQRIS(ctx context.Context, input usecase.CreateQRISInput) (*usecase.PaymentOutput, error) {
	merchant, err := u.merchantRepo.FindByID(ctx, input.MerchantID)
	if err != nil {
		return nil, err
	}
	if !merchant.CanTransact() {
		return nil, apperror.Forbidden("merchant account is not active or webhook URL not set")
	}

	gw, err := u.gateway(input.Provider)
	if err != nil {
		return nil, err
	}

	extID := externalID()
	resp, err := gw.CreateQRIS(ctx, gateway.CreateQRISRequest{
		ExternalID:    extID,
		Amount:        input.Amount,
		Currency:      input.Currency,
		CustomerName:  input.CustomerName,
		CustomerEmail: input.CustomerEmail,
		CustomerPhone: input.CustomerPhone,
		Description:   input.Description,
		ExpiryAt:      input.ExpiryAt,
	})
	if err != nil {
		return nil, fmt.Errorf("create QRIS via %s: %w", input.Provider, err)
	}

	p := &entity.Payment{
		MerchantID:        input.MerchantID,
		ExternalID:        resp.ExternalID,
		ProviderPaymentID: resp.ProviderPaymentID,
		Method:            entity.PaymentMethodQRIS,
		Provider:          input.Provider,
		Status:            entity.PaymentStatusPending,
		Amount:            input.Amount,
		Currency:          input.Currency,
		Description:       input.Description,
		CustomerName:      input.CustomerName,
		CustomerEmail:     input.CustomerEmail,
		CustomerPhone:     input.CustomerPhone,
		QRString:          resp.QRString,
		QRImageURL:        resp.QRImageURL,
		ExpiryAt:          resp.ExpiryAt,
	}
	if input.CallbackURL != "" {
		p.Metadata = map[string]any{"callback_url": input.CallbackURL}
	}
	if err := u.savePaymentWithAudit(ctx, p, input.MerchantID); err != nil {
		return nil, err
	}
	return toPaymentOutput(p), nil
}

// savePaymentWithAudit persists a new pending payment and its PAYMENT_CREATED
// audit record in one transaction, so the audit trail can never go missing.
func (u *paymentUsecase) savePaymentWithAudit(ctx context.Context, p *entity.Payment, merchantID string) error {
	return database.RunInTx(ctx, u.db, nil, func(ctx context.Context) error {
		if err := u.paymentRepo.Save(ctx, p); err != nil {
			return fmt.Errorf("save payment: %w", err)
		}
		return u.auditRepo.Save(ctx, &entity.PaymentAudit{
			PaymentID: p.ID,
			EventType: entity.AuditEventPaymentCreated,
			NewStatus: entity.PaymentStatusPending,
			Actor:     "merchant:" + merchantID,
		})
	})
}

func (u *paymentUsecase) GetPayment(ctx context.Context, merchantID, paymentID string) (*usecase.PaymentOutput, error) {
	p, err := u.paymentRepo.FindByID(ctx, paymentID)
	if err != nil {
		return nil, err
	}
	if p.MerchantID != merchantID {
		return nil, apperror.NotFound("payment %s not found", paymentID)
	}
	return toPaymentOutput(p), nil
}

func (u *paymentUsecase) CancelPayment(ctx context.Context, merchantID, paymentID string) error {
	p, err := u.paymentRepo.FindByID(ctx, paymentID)
	if err != nil {
		return err
	}
	if p.MerchantID != merchantID {
		return apperror.NotFound("payment %s not found", paymentID)
	}
	if !p.CanCancel() {
		return apperror.UnprocessableEntity("payment cannot be cancelled in status %s", p.Status)
	}

	// Step 1: Mark as cancelling before calling provider.
	// If the process dies after this and before step 3, the payment stays
	// in "cancelling" and ops can resolve it manually. It cannot be retried
	// by the merchant because CanCancel() returns false for cancelling.
	oldStatus := p.Status
	if err := database.RunInTx(ctx, u.db, nil, func(ctx context.Context) error {
		p.Status = entity.PaymentStatusCancelling
		if err := u.paymentRepo.Update(ctx, p); err != nil {
			return err
		}
		return u.auditRepo.Save(ctx, &entity.PaymentAudit{
			PaymentID: p.ID,
			EventType: entity.AuditEventStatusChanged,
			OldStatus: &oldStatus,
			NewStatus: entity.PaymentStatusCancelling,
			Actor:     "merchant:" + merchantID,
		})
	}); err != nil {
		return err
	}

	gw, err := u.gateway(p.Provider)
	if err != nil {
		return err
	}
	// Xendit cancel needs the ProviderPaymentID (payment_request_id), not ExternalID.
	// For other providers (Midtrans, DOKU, iPaymu), ExternalID is the correct cancel key.
	cancelID := p.ExternalID
	if p.ProviderPaymentID != "" {
		cancelID = p.ProviderPaymentID
	}
	providerErr := gw.CancelPayment(ctx, cancelID)

	// Step 3: Finalize status based on provider result.
	return database.RunInTx(ctx, u.db, nil, func(ctx context.Context) error {
		cancellingStatus := entity.PaymentStatusCancelling
		if providerErr != nil {
			// Provider failed — roll back to pending so merchant can retry.
			u.log.Warn("cancel provider call failed, reverting to pending",
				zap.String("payment_id", p.ID),
				zap.Error(providerErr),
			)
			p.Status = entity.PaymentStatusPending
			p.CancelledAt = nil
			if err := u.paymentRepo.Update(ctx, p); err != nil {
				return err
			}
			return u.auditRepo.Save(ctx, &entity.PaymentAudit{
				PaymentID: p.ID,
				EventType: entity.AuditEventStatusChanged,
				OldStatus: &cancellingStatus,
				NewStatus: entity.PaymentStatusPending,
				Actor:     "system:cancel_revert",
			})
		}
		now := time.Now()
		p.Status = entity.PaymentStatusCancelled
		p.CancelledAt = &now
		if err := u.paymentRepo.Update(ctx, p); err != nil {
			return err
		}
		return u.auditRepo.Save(ctx, &entity.PaymentAudit{
			PaymentID: p.ID,
			EventType: entity.AuditEventPaymentCancelled,
			OldStatus: &cancellingStatus,
			NewStatus: entity.PaymentStatusCancelled,
			Actor:     "merchant:" + merchantID,
		})
	})
}

func (u *paymentUsecase) HandleWebhook(ctx context.Context, provider entity.Provider, headers map[string]string, body []byte) error {
	gw, err := u.gateway(provider)
	if err != nil {
		return err
	}
	event, err := gw.ParseWebhook(ctx, headers, body)
	if err != nil {
		return fmt.Errorf("parse webhook: %w", err)
	}

	p, err := u.paymentRepo.FindByExternalID(ctx, provider, event.ExternalID)
	if err != nil {
		return fmt.Errorf("find payment: %w", err)
	}
	if p.IsFinal() {
		return nil // idempotent
	}

	merchant, err := u.merchantRepo.FindByID(ctx, p.MerchantID)
	if err != nil {
		return fmt.Errorf("find merchant: %w", err)
	}

	return database.RunInTx(ctx, u.db, nil, func(ctx context.Context) error {
		// Re-fetch with SELECT FOR UPDATE inside tx so concurrent webhook deliveries
		// are serialized. The first one wins; the second sees IsFinal() = true and exits.
		fresh, err := u.paymentRepo.FindByIDForUpdate(ctx, p.ID)
		if err != nil {
			return err
		}
		if fresh.IsFinal() {
			return nil // already processed by concurrent request
		}
		p.Status = event.Status
		if event.Status == entity.PaymentStatusPaid {
			p.PaidAt = event.PaidAt
		}
		oldStatus := fresh.Status
		if err := u.paymentRepo.Update(ctx, p); err != nil {
			return err
		}
		if err := u.auditRepo.Save(ctx, &entity.PaymentAudit{
			PaymentID: p.ID,
			EventType: entity.AuditEventWebhookReceived,
			OldStatus: &oldStatus,
			NewStatus: p.Status,
			Actor:     "webhook:" + string(provider),
		}); err != nil {
			return err
		}
		fee := int64(0)
		if p.Status == entity.PaymentStatusPaid {
			res, err := u.feeResolver.Resolve(ctx, merchant, p.Method, p.Amount)
			if err != nil {
				u.log.Warn("fee resolution failed, using zero fee",
					zap.String("payment_id", p.ID),
					zap.Error(err),
				)
			} else {
				fee = res.TotalFee
			}
			if err := u.mutationRepo.Save(ctx, &entity.Mutation{
				ReferenceID:   p.ID,
				ReferenceType: entity.MutationRefPayment,
				MerchantID:    p.MerchantID,
				Type:          entity.MutationTypeCashIn,
				Amount:        p.Amount,
				FeeAmount:     fee,
				Currency:      p.Currency,
				Description:   fmt.Sprintf("Payment %s via %s", p.Method, p.Provider),
			}); err != nil {
				return err
			}
		}
		callbackURL := merchant.WebhookURL
		if cb, ok := p.Metadata["callback_url"].(string); ok && cb != "" {
			callbackURL = cb
		}
		if callbackURL == "" {
			return nil
		}
		eventType := "payment." + string(p.Status)
		return u.outboxRepo.Insert(ctx, eventType, callbackURL, merchant.ID, webhook.Payload{
			EventType: eventType,
			CreatedAt: time.Now(),
			Data: webhook.PaymentData{
				PaymentID:     p.ID,
				ExternalID:    p.ExternalID,
				MerchantID:    p.MerchantID,
				Status:        string(p.Status),
				Method:        string(p.Method),
				Provider:      string(p.Provider),
				Amount:        p.Amount,
				FeeAmount:     fee,
				NetAmount:     p.Amount - fee,
				Currency:      string(p.Currency),
				CustomerName:  p.CustomerName,
				CustomerEmail: p.CustomerEmail,
				PaidAt:        p.PaidAt,
				CreatedAt:     p.CreatedAt,
			},
		})
	})
}

func (u *paymentUsecase) ListPayments(ctx context.Context, input usecase.ListPaymentsInput) (*usecase.PaymentListOutput, error) {
	page := input.Page
	if page < 1 {
		page = 1
	}
	limit := input.Limit
	if limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	filter := repository.ListPaymentFilter{
		MerchantID: input.MerchantID,
		Page:       page,
		Limit:      limit,
	}
	if input.Status != "" {
		s := entity.PaymentStatus(input.Status)
		filter.Status = &s
	}
	if input.Provider != "" {
		p := entity.Provider(input.Provider)
		filter.Provider = &p
	}
	if input.Method != "" {
		m := entity.PaymentMethod(input.Method)
		filter.Method = &m
	}
	if input.StartDate != "" {
		t, err := time.Parse("2006-01-02", input.StartDate)
		if err == nil {
			filter.StartDate = &t
		}
	}
	if input.EndDate != "" {
		t, err := time.Parse("2006-01-02", input.EndDate)
		if err == nil {
			filter.EndDate = &t
		}
	}

	payments, total, err := u.paymentRepo.List(ctx, filter)
	if err != nil {
		return nil, err
	}

	items := make([]*usecase.PaymentOutput, len(payments))
	for i, p := range payments {
		items[i] = toPaymentOutput(p)
	}
	return &usecase.PaymentListOutput{
		Items: items,
		Total: total,
		Page:  page,
		Limit: limit,
	}, nil
}

