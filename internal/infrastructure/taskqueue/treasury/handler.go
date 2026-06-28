package treasury

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"
	"go.uber.org/zap"

	"wanpey/core/internal/domain/entity"
	"wanpey/core/internal/domain/repository"
	"wanpey/core/internal/infrastructure/config"
)

// Handler handles all treasury tasks.
// It reads provider balances from the DB and compares against configured thresholds.
type Handler struct {
	providerBalanceRepo repository.ProviderBalanceRepository
	client              *asynq.Client
	cfg                 config.TreasuryConfig
	log                 *zap.Logger
}

func NewHandler(
	providerBalanceRepo repository.ProviderBalanceRepository,
	client *asynq.Client,
	cfg config.TreasuryConfig,
	log *zap.Logger,
) *Handler {
	return &Handler{
		providerBalanceRepo: providerBalanceRepo,
		client:              client,
		cfg:                 cfg,
		log:                 log,
	}
}

// HandleCheckTopup runs on the cron schedule, reads all provider balances,
// and enqueues execute_topup tasks for any provider below threshold.
// Skips providers that have never been reconciled (LastReconciledAt == nil)
// to avoid triggering topups based on stale zero-balance defaults.
func (h *Handler) HandleCheckTopup(ctx context.Context, _ *asynq.Task) error {
	balances, err := h.providerBalanceRepo.ListAll(ctx)
	if err != nil {
		return fmt.Errorf("list provider balances: %w", err)
	}

	for _, b := range balances {
		if b.LastReconciledAt == nil {
			h.log.Warn("skipping provider with no reconciliation history",
				zap.String("provider", string(b.Provider)),
			)
			continue
		}
		if b.BalanceIDR < h.cfg.TopupThresholdIDR {
			h.log.Warn("provider balance below threshold — queuing topup",
				zap.String("provider", string(b.Provider)),
				zap.Int64("balance_idr", b.BalanceIDR),
				zap.Int64("threshold_idr", h.cfg.TopupThresholdIDR),
			)

			task, err := NewExecuteTopupTask(b.Provider, h.cfg.TopupAmountIDR, "threshold")
			if err != nil {
				h.log.Error("build execute_topup task failed", zap.Error(err))
				continue
			}
			if _, err := h.client.EnqueueContext(ctx, task); err != nil {
				h.log.Error("enqueue execute_topup failed",
					zap.String("provider", string(b.Provider)),
					zap.Error(err),
				)
			}
		}
	}
	return nil
}

// HandleExecuteTopup executes a top-up transfer from the platform pool to a provider.
// Replace the stub body with your actual transfer API call.
func (h *Handler) HandleExecuteTopup(ctx context.Context, t *asynq.Task) error {
	var p ExecuteTopupPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("unmarshal execute_topup payload: %w", err)
	}

	h.log.Info("executing provider topup",
		zap.String("provider", string(p.Provider)),
		zap.Int64("amount_idr", p.AmountIDR),
		zap.String("reason", p.Reason),
	)

	// Manual top-up required until inter-bank transfer API is integrated.
	// Log warning so ops team is alerted via monitoring; acknowledge the task
	// so it does not fill the dead letter queue.
	h.log.Warn("execute_topup not implemented — manual top-up required",
		zap.String("provider", string(p.Provider)),
		zap.Int64("amount_idr", p.AmountIDR),
		zap.String("reason", p.Reason),
	)
	_ = ctx
	return nil
}

// HandleLargeCashout is called by the disbursement usecase when a single cashout
// exceeds LargeCashoutThresholdIDR. Triggers an immediate check for the provider.
func (h *Handler) HandleLargeCashoutTopupCheck(ctx context.Context, provider entity.Provider, cashoutAmount int64) {
	if cashoutAmount < h.cfg.LargeCashoutThresholdIDR {
		return
	}

	h.log.Info("large cashout detected — triggering topup check",
		zap.String("provider", string(provider)),
		zap.Int64("cashout_amount_idr", cashoutAmount),
	)

	task, err := NewExecuteTopupTask(provider, h.cfg.TopupAmountIDR, "large_cashout")
	if err != nil {
		h.log.Error("build large_cashout topup task failed", zap.Error(err))
		return
	}
	if _, err := h.client.EnqueueContext(ctx, task); err != nil {
		h.log.Error("enqueue large_cashout topup failed", zap.Error(err))
	}
}
