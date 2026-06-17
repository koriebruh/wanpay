package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/XSAM/otelsql"
	_ "github.com/lib/pq" // registers the postgres driver with database/sql
	"github.com/samber/do/v2"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.uber.org/zap"

	"wanpey/core/internal/infrastructure/config"
	"wanpey/core/internal/infrastructure/database"
)

// postgresDB wraps *sql.DB and implements database.SQLDB + do.Shutdownable.
type postgresDB struct {
	*sql.DB
}

func (p *postgresDB) Shutdown() error {
	zap.L().Info("closing postgres connection")
	return p.DB.Close()
}

// ProvideDB registers database.SQLDB in the DI container backed by Postgres.
// To swap the RDBMS, replace only this file — all repos and usecases stay unchanged.
func ProvideDB(i do.Injector) {
	do.Provide(i, func(i do.Injector) (database.SQLDB, error) {
		cfg := do.MustInvoke[*config.Config](i)
		log := do.MustInvoke[*zap.Logger](i)
		return newPostgres(cfg.Database, log)
	})
}

func newPostgres(cfg config.DatabaseConfig, log *zap.Logger) (*postgresDB, error) {
	if cfg.MaxOpenConns == 0 {
		cfg.MaxOpenConns = 25
	}
	if cfg.MaxIdleConns == 0 {
		cfg.MaxIdleConns = 10
	}
	if cfg.ConnMaxLifetimeMinutes == 0 {
		cfg.ConnMaxLifetimeMinutes = 5
	}
	if cfg.MaxIdleConns > cfg.MaxOpenConns {
		cfg.MaxIdleConns = cfg.MaxOpenConns
	}

	db, err := otelsql.Open("postgres", cfg.DSN,
		otelsql.WithAttributes(semconv.DBSystemPostgreSQL),
		otelsql.WithSpanOptions(otelsql.SpanOptions{DisableErrSkip: true}),
	)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}

	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(time.Duration(cfg.ConnMaxLifetimeMinutes) * time.Minute)

	pingCtx, pingCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer pingCancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	if _, err := otelsql.RegisterDBStatsMetrics(db, otelsql.WithAttributes(semconv.DBSystemPostgreSQL)); err != nil {
		log.Warn("failed to register db stats metrics", zap.Error(err))
	}

	log.Info("postgres connected",
		zap.String("host", maskDSN(cfg.DSN)),
		zap.Int("max_open_conns", cfg.MaxOpenConns),
		zap.Int("max_idle_conns", cfg.MaxIdleConns),
	)

	return &postgresDB{db}, nil
}

func maskDSN(dsn string) string {
	if at := strings.LastIndex(dsn, "@"); at >= 0 {
		return dsn[at+1:]
	}
	return "***"
}
