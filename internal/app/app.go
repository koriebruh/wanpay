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

	"github.com/labstack/echo/v4"
	"github.com/samber/do/v2"
	"go.uber.org/zap"

	deliveryHTTP "wanpey/core/internal/delivery/http"
	"wanpey/core/internal/infrastructure/cache"
	"wanpey/core/internal/infrastructure/config"
	"wanpey/core/internal/infrastructure/database"
	"wanpey/core/internal/infrastructure/database/postgres"
	"wanpey/core/internal/infrastructure/logger"
	"wanpey/core/internal/infrastructure/telemetry"
	"wanpey/core/internal/infrastructure/worker"
)

const workerDrainTimeout = 15 * time.Second

type App struct {
	injector     do.Injector
	stopWorkers  context.CancelFunc
	workerWg     sync.WaitGroup
	shutdownOnce sync.Once
	shutdownErr  error
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

	e.GET("/health", healthHandler(db, c))

	// Start workers under a cancellable context stored on App so Shutdown can reach them.
	workerCtx, cancel := context.WithCancel(context.Background())
	a.stopWorkers = cancel

	a.workerWg.Add(1)
	go func() {
		defer a.workerWg.Done()
		worker.NewOutboxWorker(db, log).Run(workerCtx)
	}()

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
//  1. HTTP drain    — stop accepting new requests, wait for in-flight to complete
//  2. Worker drain  — cancel worker context, wait for goroutines to exit cleanly
//  3. Logger flush  — ensure all audit logs are written before infra closes
//  4. Infra close   — samber/do shuts down in reverse-registration order (Echo → Redis → Postgres → Tracer)
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
			"status":     status,
			"components": components,
		})
	}
}

func (a *App) shutdown() error {
	cfg := do.MustInvoke[*config.Config](a.injector)
	log := do.MustInvoke[*zap.Logger](a.injector)
	e := do.MustInvoke[*echo.Echo](a.injector)

	timeout := time.Duration(cfg.App.Shutdown.TimeoutSeconds) * time.Second
	log.Info("graceful shutdown started", zap.Duration("timeout", timeout))

	// Stage 1: HTTP drain
	httpCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := e.Shutdown(httpCtx); err != nil {
		log.Error("http drain timeout, forcing close", zap.Error(err))
	}
	log.Info("http server drained")

	// Stage 2: Worker drain — cancel context then wait with a hard deadline.
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

	// Stage 3: Flush logger before infra closes so final audit logs are not lost.
	log.Info("shutdown complete")
	_ = log.Sync() //nolint:errcheck // Sync on stdout/stderr returns an error on some OS; nothing actionable here

	// Stage 4: Close infra in reverse-registration order.
	return a.injector.Shutdown()
}
