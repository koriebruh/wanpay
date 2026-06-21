package impl

import (
	"context"
	"fmt"

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

	// Balance + daily-limit checks run inside a transaction with a merchant row lock
	// (SELECT ... FOR UPDATE) to prevent concurrent double-spend.
	var balance int64
	var todayTotal int64
	if err := database.RunInTx(ctx, u.db, nil, func(ctx context.Context) error {
		// Acquire exclusive lock on merchant row for the duration of this check
		if err := database.QuerierFromContext(ctx, u.db).QueryRowContext(ctx, lockMerchantSQL, input.MerchantID).Scan(new(string)); err != nil {
			return fmt.Errorf("lock merchant: %w", err)
		}
		var err error
		balance, err = u.mutationRepo.GetBalance(ctx, input.MerchantID)
		if err != nil {
			return fmt.Errorf("get balance: %w", err)
		}
		if balance < input.Amount {
			return apperror.UnprocessableEntity("insufficient balance: have %d IDR, need %d IDR", balance, input.Amount)
		}
		if merchant.DailyCashoutLimit > 0 {
			todayTotal, err = u.disbursementRepo.SumDisbursementsToday(ctx, input.MerchantID)
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
		return nil
	}); err != nil {
		return nil, err
	}

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
		return nil, fmt.Errorf("disburse via %s: %w", input.Provider, err)
	}

	fee := computeMethodFee(merchant.FeeConfig.Disbursement, input.Amount)
	d := &entity.Disbursement{
		MerchantID:    input.MerchantID,
		ExternalID:    resp.ExternalID,
		Provider:      input.Provider,
		Status:        resp.Status,
		BankCode:      input.BankCode,
		AccountNumber: input.AccountNumber,
		AccountName:   input.AccountName,
		Amount:        input.Amount,
		FeeAmount:     fee,
		Currency:      input.Currency,
		Description:   input.Description,
	}
	if err := u.disbursementRepo.Save(ctx, d); err != nil {
		return nil, fmt.Errorf("save disbursement: %w", err)
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
