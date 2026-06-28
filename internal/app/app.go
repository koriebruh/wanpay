package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/hibiken/asynq"
	"github.com/labstack/echo/v4"
	"github.com/samber/do/v2"
	"go.uber.org/zap"

	deliveryHTTP "wanpey/core/internal/delivery/http"
	"wanpey/core/internal/delivery/http/handler"
	"wanpey/core/internal/domain/entity"
	"wanpey/core/internal/domain/gateway"
	"wanpey/core/internal/infrastructure/cache"
	"wanpey/core/internal/infrastructure/config"
	"wanpey/core/internal/infrastructure/database"
	"wanpey/core/internal/infrastructure/database/postgres"
	"wanpey/core/internal/infrastructure/logger"
	cbprovider "wanpey/core/internal/infrastructure/provider"
	"wanpey/core/internal/infrastructure/provider/doku"
	"wanpey/core/internal/infrastructure/provider/ipaymu"
	"wanpey/core/internal/infrastructure/provider/midtrans"
	"wanpey/core/internal/infrastructure/provider/xendit"
	"wanpey/core/internal/infrastructure/taskqueue"
	"wanpey/core/internal/infrastructure/taskqueue/treasury"
	"wanpey/core/internal/infrastructure/telemetry"
	"wanpey/core/internal/infrastructure/worker"
	"wanpey/core/internal/usecase/impl"
)

const workerDrainTimeout = 15 * time.Second

type App struct {
	injector     do.Injector
	stopWorkers  context.CancelFunc
	workerWg     sync.WaitGroup
	shutdownOnce sync.Once
	shutdownErr  error
	asynqSrv     *asynq.Server
	asynqSched   *asynq.Scheduler
}

func New() *App {
	i := do.New()

	config.Provide(i)
	logger.Provide(i)
	telemetry.ProvideTracer(i)
	postgres.ProvideDB(i)
	cache.ProvideCache(i)
	deliveryHTTP.ProvideEcho(i)

	return &App{injector: i}
}

func (a *App) Boot() error {
	if _, err := do.Invoke[*config.Config](a.injector); err != nil {
		return fmt.Errorf("boot config: %w", err)
	}

	log, err := do.Invoke[*zap.Logger](a.injector)
	if err != nil {
		return fmt.Errorf("boot logger: %w", err)
	}

	log.Info("booting application...")

	if _, err := do.Invoke[*telemetry.Tracer](a.injector); err != nil {
		return fmt.Errorf("boot tracer: %w", err)
	}

	if _, err := do.Invoke[database.SQLDB](a.injector); err != nil {
		return fmt.Errorf("boot db: %w", err)
	}

	if _, err := do.Invoke[cache.Cache](a.injector); err != nil {
		return fmt.Errorf("boot cache: %w", err)
	}

	if _, err := do.Invoke[*echo.Echo](a.injector); err != nil {
		return fmt.Errorf("boot http: %w", err)
	}

	log.Info("all infrastructure ready")
	return nil
}

func (a *App) Run() error {
	cfg := do.MustInvoke[*config.Config](a.injector)
	log := do.MustInvoke[*zap.Logger](a.injector)
	e := do.MustInvoke[*echo.Echo](a.injector)
	db := do.MustInvoke[database.SQLDB](a.injector)
	c := do.MustInvoke[cache.Cache](a.injector)

	// Repositories
	merchantRepo := postgres.NewMerchantRepo(db)
	paymentRepo := postgres.NewPaymentRepo(db)
	disbursementRepo := postgres.NewDisbursementRepo(db)
	mutationRepo := postgres.NewMutationRepo(db)
	auditRepo := postgres.NewAuditRepo(db)
	outboxRepo := postgres.NewOutboxRepo(db)
	adminRepo := postgres.NewAdminRepo(db)
	providerBalanceRepo := postgres.NewProviderBalanceRepo(db)
	feeRepo := postgres.NewFeeRepo(db)
	feeResolver := impl.NewFeeResolver(feeRepo)

	// T-FEE-08: Seed platform margin from config on first boot.
	// Applies only when updated_by is empty (migration default = never manually changed by admin).
	seedCtx := context.Background()
	if m, err := feeRepo.GetMargin(seedCtx); err == nil && m.UpdatedBy == "" {
		cfgMargin := cfg.Fee.Margin
		m.Enabled = cfgMargin.Enabled
		m.VA = marginToMethodFee(cfgMargin.VA)
		m.QRIS = marginToMethodFee(cfgMargin.QRIS)
		m.Disbursement = marginToMethodFee(cfgMargin.Disbursement)
		m.UpdatedBy = "system:boot_seed"
		if seedErr := feeRepo.UpdateMargin(seedCtx, m); seedErr != nil {
			log.Warn("fee margin boot seed failed", zap.Error(seedErr))
		} else {
			log.Info("platform margin seeded from config")
		}
	}

	// Payment gateways
	cbCfg := cfg.Provider.CircuitBreaker
	payGWs := make(map[entity.Provider]gateway.PaymentGateway)
	disbGWs := make(map[entity.Provider]gateway.DisbursementGateway)

	if cfg.Provider.Midtrans.Enabled {
		gw, err := midtrans.New(cfg.Provider.Midtrans, log)
		if err != nil {
			return fmt.Errorf("midtrans init: %w", err)
		}
		payGWs[entity.ProviderMidtrans] = cbprovider.NewCBPaymentGateway(gw, cbCfg, log)
		log.Info("midtrans gateway enabled")
	}

	if cfg.Provider.Xendit.Enabled {
		gw, err := xendit.New(cfg.Provider.Xendit, log)
		if err != nil {
			return fmt.Errorf("xendit init: %w", err)
		}
		payGWs[entity.ProviderXendit] = cbprovider.NewCBPaymentGateway(gw, cbCfg, log)
		disbGWs[entity.ProviderXendit] = cbprovider.NewCBDisbursementGateway(gw, cbCfg, log)
		log.Info("xendit gateway enabled")
	}

	if cfg.Provider.Doku.Enabled {
		gw, err := doku.New(cfg.Provider.Doku, log)
		if err != nil {
			return fmt.Errorf("doku init: %w", err)
		}
		payGWs[entity.ProviderDoku] = cbprovider.NewCBPaymentGateway(gw, cbCfg, log)
		disbGWs[entity.ProviderDoku] = cbprovider.NewCBDisbursementGateway(gw, cbCfg, log)
		log.Info("doku gateway enabled")
	}

	if cfg.Provider.IPaymu.Enabled {
		gw, err := ipaymu.New(cfg.Provider.IPaymu, log)
		if err != nil {
			return fmt.Errorf("ipaymu init: %w", err)
		}
		payGWs[entity.ProviderIPaymu] = cbprovider.NewCBPaymentGateway(gw, cbCfg, log)
		log.Info("ipaymu gateway enabled")
	}

	if len(payGWs) == 0 {
		log.Warn("no payment gateways enabled — payment creation will fail at runtime")
	}

	// Usecases
	paymentUC := impl.NewPaymentUsecase(payGWs, paymentRepo, mutationRepo, auditRepo, outboxRepo, merchantRepo, feeResolver, db, log)
	disbursementUC := impl.NewDisbursementUsecase(disbGWs, disbursementRepo, mutationRepo, outboxRepo, merchantRepo, feeResolver, db, log)
	mutationUC := impl.NewMutationUsecase(mutationRepo)
	merchantUC := impl.NewMerchantUsecase(merchantRepo, mutationRepo, outboxRepo, db)
	adminUC := impl.NewAdminUsecase(adminRepo, merchantRepo, merchantUC, paymentRepo, disbursementRepo, mutationRepo, providerBalanceRepo, feeRepo, cfg.Admin)

	// Task queue (optional — requires Redis)
	if cfg.TaskQueue.Enabled && cfg.Redis.Enabled {

		client, err := taskqueue.ProvideClient(cfg.Redis)
		if err != nil {
			return fmt.Errorf("asynq client: %w", err)
		}

		treasuryHandler := treasury.NewHandler(providerBalanceRepo, client, cfg.TaskQueue.Treasury, log)

		srv, sched, err := taskqueue.ProvideServer(cfg.TaskQueue, cfg.Redis, treasuryHandler, log)
		if err != nil {
			return fmt.Errorf("asynq server: %w", err)
		}
		a.asynqSrv = srv
		a.asynqSched = sched
	}

	// HTTP routes
	e.GET("/health", healthHandler(db, c))

	webhookAllowedIPs := map[string][]string{
		"midtrans": cfg.Provider.Midtrans.AllowedIPs,
		"xendit":   cfg.Provider.Xendit.AllowedIPs,
		"doku":     cfg.Provider.Doku.AllowedIPs,
		"ipaymu":   cfg.Provider.IPaymu.AllowedIPs,
	}
	deliveryHTTP.Register(e, deliveryHTTP.Routes{
		MerchantRepo:      merchantRepo,
		Cache:             c,
		Payment:           handler.NewPaymentHandler(paymentUC),
		Disbursement:      handler.NewDisbursementHandler(disbursementUC),
		Mutation:          handler.NewMutationHandler(mutationUC),
		Merchant:          handler.NewMerchantHandler(merchantUC),
		Webhook:           handler.NewWebhookHandler(paymentUC, disbursementUC),
		Admin:             handler.NewAdminHandler(adminUC),
		AdminJWTSecret:    cfg.Admin.JWTSecret,
		WebhookAllowedIPs: webhookAllowedIPs,
		Log:               log,
	})

	// Outbox worker
	workerCtx, cancel := context.WithCancel(context.Background())
	a.stopWorkers = cancel

	a.workerWg.Add(1)
	go func() {
		defer a.workerWg.Done()
		worker.NewOutboxWorker(db, outboxRepo, log).Run(workerCtx)
	}()

	a.workerWg.Add(1)
	go func() {
		defer a.workerWg.Done()
		worker.NewExpiryWorker(db, log).Run(workerCtx)
	}()

	// HTTP server
	serverErr := make(chan error, 1)
	go func() {
		log.Info("http server listening", zap.String("port", cfg.App.Port))
		if err := e.Start(":" + cfg.App.Port); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	log.Info("application running",
		zap.String("app", cfg.App.Name),
		zap.String("env", cfg.App.Env),
	)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(quit)

	select {
	case sig := <-quit:
		log.Info("signal received", zap.String("signal", sig.String()))
		return a.Shutdown()
	case err := <-serverErr:
		log.Error("server crashed", zap.Error(err))
		if shutdownErr := a.Shutdown(); shutdownErr != nil {
			log.Error("shutdown error after server crash", zap.Error(shutdownErr))
		}
		return err
	}
}

// Shutdown stops all components in the correct order for a financial application:
//
//  1. HTTP drain     — stop accepting new requests, wait for in-flight to complete
//  2. Asynq drain    — stop scheduler (no new tasks), drain in-flight tasks
//  3. Worker drain   — cancel outbox worker context, wait for goroutines to exit
//  4. Logger flush   — ensure all audit logs are written before infra closes
//  5. Infra close    — samber/do shuts down in reverse-registration order
//
// Safe to call multiple times — subsequent calls return the first call's error immediately.
func (a *App) Shutdown() error {
	a.shutdownOnce.Do(func() { a.shutdownErr = a.shutdown() })
	return a.shutdownErr
}

func healthHandler(db database.SQLDB, c cache.Cache) echo.HandlerFunc {
	return func(ec echo.Context) error {
		ctx := ec.Request().Context()
		components := map[string]string{
			"database": "ok",
			"cache":    "ok",
		}

		if err := db.PingContext(ctx); err != nil {
			components["database"] = "unhealthy"
		}
		if err := c.Ping(ctx); err != nil {
			components["cache"] = "unhealthy"
		}

		var outboxBacklog int64
		_ = db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM outbox WHERE delivered_at IS NULL AND failed_at IS NULL`,
		).Scan(&outboxBacklog)

		status := "ok"
		httpCode := http.StatusOK
		for _, v := range components {
			if v != "ok" {
				status = "degraded"
				httpCode = http.StatusServiceUnavailable
				break
			}
		}

		return ec.JSON(httpCode, map[string]any{
			"status":         status,
			"components":     components,
			"outbox_backlog": outboxBacklog,
		})
	}
}

func (a *App) shutdown() error {
	cfg := do.MustInvoke[*config.Config](a.injector)
	log := do.MustInvoke[*zap.Logger](a.injector)
	e := do.MustInvoke[*echo.Echo](a.injector)

	timeout := time.Duration(cfg.App.Shutdown.TimeoutSeconds) * time.Second
	log.Info("graceful shutdown started", zap.Duration("timeout", timeout))

	// Stage 1: HTTP drain — stop accepting requests, wait for in-flight handlers.
	httpCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := e.Shutdown(httpCtx); err != nil {
		log.Error("http drain timeout, forcing close", zap.Error(err))
	}
	log.Info("http server drained")

	// Stage 2: Asynq drain — scheduler first (no new tasks), then server (in-flight tasks).
	// Must happen before worker/DB close so in-flight financial tasks can write to DB.
	if a.asynqSched != nil {
		a.asynqSched.Shutdown()
	}
	if a.asynqSrv != nil {
		a.asynqSrv.Stop()
	}
	if a.asynqSrv != nil || a.asynqSched != nil {
		log.Info("asynq drained")
	}

	// Stage 3: Outbox worker drain — cancel context then wait with a hard deadline.
	// Workers must stop BEFORE infra closes or DB queries will hit "sql: database is closed".
	if a.stopWorkers != nil {
		a.stopWorkers()
	}
	workerStopped := make(chan struct{})
	go func() {
		a.workerWg.Wait()
		close(workerStopped)
	}()
	select {
	case <-workerStopped:
		log.Info("all workers stopped")
	case <-time.After(workerDrainTimeout):
		log.Warn("workers did not stop within drain timeout — forcing shutdown",
			zap.Duration("timeout", workerDrainTimeout),
		)
	}

	// Stage 4: Flush logger before infra closes so final audit logs are not lost.
	log.Info("shutdown complete")
	_ = log.Sync() //nolint:errcheck // Sync on stdout/stderr returns an error on some OS; nothing actionable here

	// Stage 5: Close infra in reverse-registration order (Echo → Redis → Postgres → Tracer).
	return a.injector.Shutdown()
}

func marginToMethodFee(m config.MethodMargin) entity.MethodFee {
	return entity.MethodFee{
		Type:       entity.FeeType(m.Type),
		Amount:     m.FlatIDR,
		Percentage: m.Percentage,
	}
}
