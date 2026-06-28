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
)

const lockMerchantSQL = `SELECT id FROM merchants WHERE id = $1 FOR UPDATE`

type disbursementUsecase struct {
	gateways         map[entity.Provider]gateway.DisbursementGateway
	disbursementRepo repository.DisbursementRepository
	mutationRepo     repository.MutationRepository
	outboxRepo       *postgres.OutboxRepo
	merchantRepo     repository.MerchantRepository
	db               database.SQLDB
	log              *zap.Logger
}

func NewDisbursementUsecase(
	gateways map[entity.Provider]gateway.DisbursementGateway,
	disbursementRepo repository.DisbursementRepository,
	mutationRepo repository.MutationRepository,
	outboxRepo *postgres.OutboxRepo,
	merchantRepo repository.MerchantRepository,
	db database.SQLDB,
	log *zap.Logger,
) usecase.DisbursementUsecase {
	return &disbursementUsecase{
		gateways:         gateways,
		disbursementRepo: disbursementRepo,
		mutationRepo:     mutationRepo,
		outboxRepo:       outboxRepo,
		merchantRepo:     merchantRepo,
		db:               db,
		log:              log,
	}
}

func (u *disbursementUsecase) disbGateway(provider entity.Provider) (gateway.DisbursementGateway, error) {
	gw, ok := u.gateways[provider]
	if !ok || gw == nil {
		return nil, apperror.BadRequest("provider %s does not support disbursement or is not enabled", provider)
	}
	return gw, nil
}

func (u *disbursementUsecase) Disburse(ctx context.Context, input usecase.DisburseInput) (*usecase.DisbursementOutput, error) {
	merchant, err := u.merchantRepo.FindByID(ctx, input.MerchantID)
	if err != nil {
		return nil, err
	}
	if !merchant.CanTransact() {
		return nil, apperror.Forbidden("merchant account is not active")
	}

	gw, err := u.disbGateway(input.Provider)
	if err != nil {
		return nil, err
	}

	fee := computeMethodFee(merchant.FeeConfig.Disbursement, input.Amount)

	// Reservation pattern: lock merchant, check balance (subtracting pending disbursements),
	// and INSERT a pending disbursement record — all inside one transaction.
	// This atomically "reserves" the funds so concurrent requests see the reservation
	// and cannot both pass the balance check with the same balance.
	var d *entity.Disbursement
	if err := database.RunInTx(ctx, u.db, nil, func(ctx context.Context) error {
		if err := database.QuerierFromContext(ctx, u.db).QueryRowContext(ctx, lockMerchantSQL, input.MerchantID).Scan(new(string)); err != nil {
			return fmt.Errorf("lock merchant: %w", err)
		}
		balance, err := u.mutationRepo.GetBalance(ctx, input.MerchantID)
		if err != nil {
			return fmt.Errorf("get balance: %w", err)
		}
		pendingTotal, err := u.disbursementRepo.SumPendingDisbursements(ctx, input.MerchantID)
		if err != nil {
			return fmt.Errorf("sum pending disbursements: %w", err)
		}
		available := balance - pendingTotal
		if available < input.Amount {
			return apperror.UnprocessableEntity("insufficient balance: available %d IDR (balance %d − pending %d), need %d IDR",
				available, balance, pendingTotal, input.Amount)
		}
		if merchant.DailyCashoutLimit > 0 {
			todayTotal, err := u.disbursementRepo.SumDisbursementsToday(ctx, input.MerchantID)
			if err != nil {
				return fmt.Errorf("check daily limit: %w", err)
			}
			if todayTotal+input.Amount > merchant.DailyCashoutLimit {
				return apperror.UnprocessableEntity(
					"daily cashout limit exceeded: limit %d IDR, used %d IDR, requested %d IDR",
					merchant.DailyCashoutLimit, todayTotal, input.Amount,
				)
			}
		}
		// Reserve funds by inserting a pending record before calling the provider.
		d = &entity.Disbursement{
			MerchantID:    input.MerchantID,
			ExternalID:    "", // set after provider call
			Provider:      input.Provider,
			Status:        entity.DisbursementStatusPending,
			BankCode:      input.BankCode,
			AccountNumber: input.AccountNumber,
			AccountName:   input.AccountName,
			Amount:        input.Amount,
			FeeAmount:     fee,
			Currency:      input.Currency,
			Description:   input.Description,
		}
		return u.disbursementRepo.Save(ctx, d)
	}); err != nil {
		return nil, err
	}

	// Call provider outside the transaction — disbursement record already inserted as pending.
	extID := externalID()
	resp, err := gw.Disburse(ctx, gateway.DisburseRequest{
		ExternalID:    extID,
		BankCode:      input.BankCode,
		AccountNumber: input.AccountNumber,
		AccountName:   input.AccountName,
		Amount:        input.Amount,
		Currency:      input.Currency,
		Description:   input.Description,
	})
	if err != nil {
		// Provider failed — mark reserved disbursement as failed so balance is released.
		d.Status = entity.DisbursementStatusFailed
		d.FailureReason = err.Error()
		if updateErr := u.disbursementRepo.Update(ctx, d); updateErr != nil {
			u.log.Error("failed to mark disbursement as failed after provider error",
				zap.String("disbursement_id", d.ID),
				zap.Error(updateErr),
			)
		}
		return nil, fmt.Errorf("disburse via %s: %w", input.Provider, err)
	}

	// Update the reserved record with the provider's response.
	d.ExternalID = resp.ExternalID
	d.Status = resp.Status
	if err := u.disbursementRepo.Update(ctx, d); err != nil {
		return nil, fmt.Errorf("update disbursement after provider: %w", err)
	}

	u.log.Info("disbursement created",
		zap.String("disbursement_id", d.ID),
		zap.String("provider", string(input.Provider)),
		zap.Int64("amount", input.Amount),
	)
	return toDisbursementOutput(d), nil
}

func (u *disbursementUsecase) GetDisbursement(ctx context.Context, merchantID, disbursementID string) (*usecase.DisbursementOutput, error) {
	d, err := u.disbursementRepo.FindByID(ctx, disbursementID)
	if err != nil {
		return nil, err
	}
	if d.MerchantID != merchantID {
		return nil, apperror.NotFound("disbursement %s not found", disbursementID)
	}
	return toDisbursementOutput(d), nil
}

func (u *disbursementUsecase) ListDisbursements(ctx context.Context, input usecase.ListDisbursementsInput) (*usecase.DisbursementListOutput, error) {
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

	filter := repository.ListDisbursementFilter{
		MerchantID: input.MerchantID,
		Page:       page,
		Limit:      limit,
	}
	if input.Status != "" {
		s := entity.DisbursementStatus(input.Status)
		filter.Status = &s
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

	disbursements, total, err := u.disbursementRepo.List(ctx, filter)
	if err != nil {
		return nil, err
	}

	items := make([]*usecase.DisbursementOutput, len(disbursements))
	for i, d := range disbursements {
		items[i] = toDisbursementOutput(d)
	}
	return &usecase.DisbursementListOutput{
		Items: items,
		Total: total,
		Page:  page,
		Limit: limit,
	}, nil
}

func (u *disbursementUsecase) HandleDisbursementCallback(ctx context.Context, provider entity.Provider, headers map[string]string, body []byte) error {
	gw, err := u.disbGateway(provider)
	if err != nil {
		return err
	}
	event, err := gw.ParseDisbursementWebhook(ctx, headers, body)
	if err != nil {
		return fmt.Errorf("parse disbursement webhook: %w", err)
	}

	d, err := u.disbursementRepo.FindByExternalID(ctx, provider, event.ExternalID)
	if err != nil {
		return fmt.Errorf("find disbursement: %w", err)
	}
	if d.IsFinal() {
		return nil
	}

	d.Status = event.Status
	d.FailureReason = event.FailureReason

	merchant, err := u.merchantRepo.FindByID(ctx, d.MerchantID)
	if err != nil {
		return fmt.Errorf("find merchant: %w", err)
	}

	return database.RunInTx(ctx, u.db, nil, func(ctx context.Context) error {
		if err := u.disbursementRepo.Update(ctx, d); err != nil {
			return err
		}
		if d.Status == entity.DisbursementStatusCompleted {
			// FeeAmount is charged at initiation and stored in d.FeeAmount.
			// Include it in the mutation so GetBalance correctly debits the full cost.
			if err := u.mutationRepo.Save(ctx, &entity.Mutation{
				ReferenceID:   d.ID,
				ReferenceType: entity.MutationRefDisbursement,
				MerchantID:    d.MerchantID,
				Type:          entity.MutationTypeCashOut,
				Amount:        d.Amount + d.FeeAmount,
				FeeAmount:     0, // cash_out: fee already embedded in Amount
				Currency:      d.Currency,
				Description:   fmt.Sprintf("Disbursement to %s %s", d.BankCode, d.AccountNumber),
			}); err != nil {
				return err
			}
		}
		if merchant.WebhookURL == "" {
			return nil
		}
		return u.outboxRepo.Insert(ctx, "disbursement.status_changed", merchant.WebhookURL, map[string]any{
			"event":           "disbursement." + string(d.Status),
			"disbursement_id": d.ID,
			"status":          d.Status,
			"amount":          d.Amount,
			"failure_reason":  d.FailureReason,
		})
	})
}
