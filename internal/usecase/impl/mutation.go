package impl

import (
	"context"
	"fmt"

	"wanpey/core/internal/domain/repository"
	"wanpey/core/internal/usecase"
	"wanpey/core/pkg/apperror"
)

type mutationUsecase struct {
	mutationRepo repository.MutationRepository
}

func NewMutationUsecase(mutationRepo repository.MutationRepository) usecase.MutationUsecase {
	return &mutationUsecase{mutationRepo: mutationRepo}
}

func (u *mutationUsecase) GetBalance(ctx context.Context, merchantID string) (int64, error) {
	return u.mutationRepo.GetBalance(ctx, merchantID)
}

func (u *mutationUsecase) GetMutation(ctx context.Context, merchantID, mutationID string) (*usecase.MutationOutput, error) {
	m, err := u.mutationRepo.FindByID(ctx, mutationID)
	if err != nil {
		return nil, err
	}
	if m.MerchantID != merchantID {
		return nil, apperror.NotFound("mutation %s not found", mutationID)
	}
	return toMutationOutput(m), nil
}

func (u *mutationUsecase) ListMutations(ctx context.Context, input usecase.ListMutationsInput) (*usecase.MutationListOutput, error) {
	if input.MerchantID == "" {
		return nil, apperror.BadRequest("merchant_id is required")
	}

	filter := repository.ListMutationFilter{
		MerchantID: input.MerchantID,
		Type:       input.Type,
		StartDate:  input.StartDate,
		EndDate:    input.EndDate,
		Page:       input.Page,
		Limit:      input.Limit,
	}

	mutations, total, err := u.mutationRepo.List(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("list mutations: %w", err)
	}

	items := make([]*usecase.MutationOutput, len(mutations))
	for i, m := range mutations {
		items[i] = toMutationOutput(m)
	}

	page, limit := filter.Page, filter.Limit
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 20
	}

	return &usecase.MutationListOutput{
		Items: items,
		Total: total,
		Page:  page,
		Limit: limit,
	}, nil
}
