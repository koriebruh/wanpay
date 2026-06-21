package taskqueue

import (
	"context"
	"fmt"

	"github.com/hibiken/asynq"
	"go.uber.org/zap"

	"wanpey/core/internal/infrastructure/config"
	"wanpey/core/internal/infrastructure/taskqueue/treasury"
)

// ProvideClient creates an Asynq client for enqueuing tasks.
// Returns nil if taskqueue is disabled.
func ProvideClient(redisCfg config.RedisConfig) (*asynq.Client, error) {
	if !redisCfg.Enabled {
		return nil, fmt.Errorf("taskqueue requires Redis to be enabled")
	}
	return asynq.NewClient(redisOpt(redisCfg)), nil
}

// ProvideServer creates and starts an Asynq server with registered handlers.
// Returns (nil, nil, nil) if taskqueue is disabled.
// Caller MUST call the returned shutdown functions in app.Shutdown() to drain
// in-flight tasks before process exits — critical for financial operations.
func ProvideServer(
	cfg config.TaskQueueConfig,
	redisCfg config.RedisConfig,
	handler *treasury.Handler,
	log *zap.Logger,
) (*asynq.Server, *asynq.Scheduler, error) {
	if !cfg.Enabled {
		return nil, nil, nil
	}

	// Validate financial config — zero values would silently break topup logic
	if cfg.Treasury.TopupThresholdIDR <= 0 {
		return nil, nil, fmt.Errorf("taskqueue.treasury.topup_threshold_idr must be > 0")
	}
	if cfg.Treasury.TopupAmountIDR <= 0 {
		return nil, nil, fmt.Errorf("taskqueue.treasury.topup_amount_idr must be > 0")
	}

	concurrency := cfg.Concurrency
	if concurrency <= 0 {
		concurrency = 10
	}

	srv := asynq.NewServer(redisOpt(redisCfg), asynq.Config{
		Concurrency: concurrency,
		Queues: map[string]int{
			treasury.QueueTreasury: 1,
		},
		Logger: &asynqLogger{log},
		ErrorHandler: asynq.ErrorHandlerFunc(func(ctx context.Context, task *asynq.Task, err error) {
			log.Error("task failed",
				zap.String("type", task.Type()),
				zap.Error(err),
			)
		}),
	})

	mux := asynq.NewServeMux()
	mux.HandleFunc(treasury.TypeCheckTopup, handler.HandleCheckTopup)
	mux.HandleFunc(treasury.TypeExecuteTopup, handler.HandleExecuteTopup)

	if err := srv.Start(mux); err != nil {
		return nil, nil, fmt.Errorf("start asynq server: %w", err)
	}

	// Scheduler runs cron jobs and enqueues tasks at defined intervals.
	scheduler := asynq.NewScheduler(redisOpt(redisCfg), nil)

	checkCron := cfg.Treasury.CheckCron
	if checkCron == "" {
		checkCron = "*/15 * * * *"
	}
	if _, err := scheduler.Register(checkCron, treasury.NewCheckTopupTask()); err != nil {
		srv.Stop()
		return nil, nil, fmt.Errorf("register check_topup cron: %w", err)
	}

	if err := scheduler.Start(); err != nil {
		srv.Stop()
		return nil, nil, fmt.Errorf("start asynq scheduler: %w", err)
	}

	log.Info("asynq task queue started",
		zap.Int("concurrency", concurrency),
		zap.String("treasury_cron", checkCron),
		zap.Int64("topup_threshold_idr", cfg.Treasury.TopupThresholdIDR),
	)

	return srv, scheduler, nil
}

func redisOpt(cfg config.RedisConfig) asynq.RedisClientOpt {
	opt := asynq.RedisClientOpt{Addr: cfg.Addr}
	if cfg.Password != "" {
		opt.Password = cfg.Password
	}
	opt.DB = cfg.DB
	return opt
}

// asynqLogger adapts zap.Logger to asynq's Logger interface.
type asynqLogger struct{ log *zap.Logger }

func (l *asynqLogger) Debug(args ...interface{}) { l.log.Sugar().Debug(args...) }
func (l *asynqLogger) Info(args ...interface{})  { l.log.Sugar().Info(args...) }
func (l *asynqLogger) Warn(args ...interface{})  { l.log.Sugar().Warn(args...) }
func (l *asynqLogger) Error(args ...interface{}) { l.log.Sugar().Error(args...) }
func (l *asynqLogger) Fatal(args ...interface{}) { l.log.Sugar().Fatal(args...) }
