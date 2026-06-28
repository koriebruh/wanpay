package impl

import (
	"context"
	"errors"
	"fmt"

	"wanpey/core/internal/domain/entity"
	"wanpey/core/internal/domain/repository"
	"wanpey/core/internal/infrastructure/database"
	"wanpey/core/internal/usecase"
	"wanpey/core/pkg/apperror"
)

type merchantUsecase struct {
	merchantRepo repository.MerchantRepository
	mutationRepo repository.MutationRepository
	outboxRepo   outboxPort
	db           database.SQLDB
}

func NewMerchantUsecase(
	merchantRepo repository.MerchantRepository,
	mutationRepo repository.MutationRepository,
	outboxRepo outboxPort,
	db database.SQLDB,
) usecase.MerchantUsecase {
	return &merchantUsecase{
		merchantRepo: merchantRepo,
		mutationRepo: mutationRepo,
		outboxRepo:   outboxRepo,
		db:           db,
	}
}

func (u *merchantUsecase) Create(ctx context.Context, input usecase.CreateMerchantInput) (*usecase.CreateMerchantOutput, error) {
	existing, findErr := u.merchantRepo.FindByEmail(ctx, input.Email)
	if findErr == nil && existing != nil {
		return nil, apperror.Conflict("email %s is already registered", input.Email)
	}
	// Only skip the conflict check on NotFound — propagate real DB errors
	var ae *apperror.AppError
	if findErr != nil && !errors.As(findErr, &ae) {
		return nil, findErr
	}

	rawKey, hashedKey := generateAPIKey(input.IsProduction)
	rawSecret, hashedSecret := generateSecret()

	m := &entity.Merchant{
		Name:              input.Name,
		Email:             input.Email,
		Phone:             input.Phone,
		Status:            entity.MerchantStatusPending,
		APIKey:            hashedKey,
		WebhookURL:        input.WebhookURL,
		WebhookSecret:     hashedSecret,
		WebhookSigningKey: rawSecret,
		IsProduction:      input.IsProduction,
		FeeConfig:         input.FeeConfig,
	}
	if err := u.merchantRepo.Save(ctx, m); err != nil {
		return nil, fmt.Errorf("create merchant: %w", err)
	}

	return &usecase.CreateMerchantOutput{
		ID:            m.ID,
		Name:          m.Name,
		Status:        m.Status,
		APIKey:        rawKey,
		WebhookSecret: rawSecret,
		CreatedAt:     m.CreatedAt,
	}, nil
}

func (u *merchantUsecase) GetMerchant(ctx context.Context, id string) (*usecase.MerchantOutput, error) {
	m, err := u.merchantRepo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	balance, _ := u.mutationRepo.GetBalance(ctx, id)
	return toMerchantOutput(m, balance), nil
}

func (u *merchantUsecase) Update(ctx context.Context, input usecase.UpdateMerchantInput) (*usecase.MerchantOutput, error) {
	m, err := u.merchantRepo.FindByID(ctx, input.MerchantID)
	if err != nil {
		return nil, err
	}
	if input.Name != "" {
		m.Name = input.Name
	}
	if input.Email != "" {
		m.Email = input.Email
	}
	if input.Phone != "" {
		m.Phone = input.Phone
	}
	if input.WebhookURL != "" {
		m.WebhookURL = input.WebhookURL
	}
	if err := u.merchantRepo.Update(ctx, m); err != nil {
		return nil, fmt.Errorf("update merchant: %w", err)
	}
	balance, _ := u.mutationRepo.GetBalance(ctx, m.ID)
	return toMerchantOutput(m, balance), nil
}

func (u *merchantUsecase) Suspend(ctx context.Context, merchantID string) error {
	m, err := u.merchantRepo.FindByID(ctx, merchantID)
	if err != nil {
		return err
	}
	m.Status = entity.MerchantStatusSuspended
	return u.merchantRepo.Update(ctx, m)
}

func (u *merchantUsecase) Activate(ctx context.Context, merchantID string) error {
	m, err := u.merchantRepo.FindByID(ctx, merchantID)
	if err != nil {
		return err
	}
	m.Status = entity.MerchantStatusActive
	return u.merchantRepo.Update(ctx, m)
}

func (u *merchantUsecase) RegenerateAPIKey(ctx context.Context, merchantID string) (string, error) {
	m, err := u.merchantRepo.FindByID(ctx, merchantID)
	if err != nil {
		return "", err
	}
	rawKey, hashedKey := generateAPIKey(m.IsProduction)
	m.APIKey = hashedKey
	if err := u.merchantRepo.Update(ctx, m); err != nil {
		return "", fmt.Errorf("update api key: %w", err)
	}
	return rawKey, nil
}

func (u *merchantUsecase) AddBankAccount(ctx context.Context, input usecase.AddBankAccountInput) (*usecase.BankAccountOutput, error) {
	var a *entity.MerchantBankAccount
	if err := database.RunInTx(ctx, u.db, nil, func(ctx context.Context) error {
		// Count inside tx to serialize concurrent requests.
		count, err := u.merchantRepo.CountBankAccounts(ctx, input.MerchantID)
		if err != nil {
			return err
		}
		if count >= entity.MaxBankAccounts {
			return apperror.UnprocessableEntity("maximum %d bank accounts allowed", entity.MaxBankAccounts)
		}
		if input.SetAsPrimary {
			if err := u.merchantRepo.UnsetPrimaryBankAccounts(ctx, input.MerchantID); err != nil {
				return fmt.Errorf("unset primary: %w", err)
			}
		}
		a = &entity.MerchantBankAccount{
			MerchantID:    input.MerchantID,
			BankCode:      input.BankCode,
			AccountNumber: input.AccountNumber,
			AccountName:   input.AccountName,
			IsPrimary:     input.SetAsPrimary,
		}
		return u.merchantRepo.SaveBankAccount(ctx, a)
	}); err != nil {
		return nil, err
	}
	return toBankAccountOutput(a), nil
}

func (u *merchantUsecase) ListBankAccounts(ctx context.Context, merchantID string) ([]*usecase.BankAccountOutput, error) {
	accounts, err := u.merchantRepo.FindBankAccountsByMerchantID(ctx, merchantID)
	if err != nil {
		return nil, err
	}
	out := make([]*usecase.BankAccountOutput, len(accounts))
	for i, a := range accounts {
		out[i] = toBankAccountOutput(a)
	}
	return out, nil
}

func (u *merchantUsecase) RemoveBankAccount(ctx context.Context, merchantID, accountID string) error {
	a, err := u.merchantRepo.FindBankAccountByID(ctx, accountID)
	if err != nil {
		return err
	}
	if a.MerchantID != merchantID {
		return apperror.NotFound("bank account not found")
	}
	if a.IsPrimary {
		return apperror.UnprocessableEntity("cannot remove primary bank account — set another as primary first")
	}
	return u.merchantRepo.DeleteBankAccount(ctx, accountID)
}

func (u *merchantUsecase) SetPrimaryBankAccount(ctx context.Context, merchantID, accountID string) error {
	a, err := u.merchantRepo.FindBankAccountByID(ctx, accountID)
	if err != nil {
		return err
	}
	if a.MerchantID != merchantID {
		return apperror.NotFound("bank account not found")
	}
	if err := u.merchantRepo.UnsetPrimaryBankAccounts(ctx, merchantID); err != nil {
		return fmt.Errorf("unset primary: %w", err)
	}
	a.IsPrimary = true
	return u.merchantRepo.UpdateBankAccount(ctx, a)
}

func (u *merchantUsecase) ListWebhookEvents(ctx context.Context, merchantID string, page, limit int) (*usecase.WebhookEventListOutput, error) {
	rows, total, err := u.outboxRepo.ListByMerchant(ctx, merchantID, page, limit)
	if err != nil {
		return nil, err
	}
	if limit < 1 {
		limit = 20
	}
	out := &usecase.WebhookEventListOutput{
		Items: make([]*usecase.WebhookEventOutput, 0, len(rows)),
		Total: total,
		Page:  page,
		Limit: limit,
	}
	for _, r := range rows {
		item := &usecase.WebhookEventOutput{
			ID:           r.ID,
			EventType:    r.EventType,
			TargetURL:    r.TargetUrl,
			AttemptCount: r.AttemptCount,
			NextRetryAt:  r.NextRetryAt,
			CreatedAt:    r.CreatedAt,
		}
		if r.DeliveredAt.Valid {
			t := r.DeliveredAt.Time
			item.DeliveredAt = &t
		}
		if r.FailedAt.Valid {
			t := r.FailedAt.Time
			item.FailedAt = &t
		}
		out.Items = append(out.Items, item)
	}
	return out, nil
}

