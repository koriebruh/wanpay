package treasury

import (
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"

	"wanpey/core/internal/domain/entity"
)

const (
	TypeCheckTopup   = "treasury:check_topup"
	TypeExecuteTopup = "treasury:execute_topup"

	QueueTreasury = "treasury"
)

// ExecuteTopupPayload is the task payload for treasury:execute_topup.
type ExecuteTopupPayload struct {
	Provider  entity.Provider `json:"provider"`
	AmountIDR int64           `json:"amount_idr"`
	Reason    string          `json:"reason"` // "threshold" or "large_cashout"
}

// NewCheckTopupTask creates the periodic balance check task.
func NewCheckTopupTask() *asynq.Task {
	return asynq.NewTask(TypeCheckTopup, nil, asynq.Queue(QueueTreasury))
}

// NewExecuteTopupTask creates a top-up task for a specific provider.
// Uses TaskID = provider name so Asynq deduplicates — prevents double topup
// if check_topup runs again before the previous execute_topup completes.
func NewExecuteTopupTask(provider entity.Provider, amountIDR int64, reason string) (*asynq.Task, error) {
	payload, err := json.Marshal(ExecuteTopupPayload{
		Provider:  provider,
		AmountIDR: amountIDR,
		Reason:    reason,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal execute_topup payload: %w", err)
	}
	return asynq.NewTask(TypeExecuteTopup, payload,
		asynq.Queue(QueueTreasury),
		asynq.MaxRetry(3),
		asynq.TaskID("topup:"+string(provider)), // dedup: only one pending topup per provider at a time
	), nil
}
