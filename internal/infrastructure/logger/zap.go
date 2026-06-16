package logger

import (
	"fmt"

	"github.com/samber/do/v2"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"wanpey/core/internal/infrastructure/config"
)

func Provide(i do.Injector) {
	do.Provide(i, func(i do.Injector) (*zap.Logger, error) {
		cfg := do.MustInvoke[*config.Config](i)
		return build(cfg.Logger)
	})
}

func build(cfg config.LoggerConfig) (*zap.Logger, error) {
	level, err := zapcore.ParseLevel(cfg.Level)
	if err != nil {
		level = zapcore.InfoLevel
	}

	var zapCfg zap.Config
	if cfg.Encoding == "json" {
		zapCfg = zap.NewProductionConfig()
	} else {
		zapCfg = zap.NewDevelopmentConfig()
		zapCfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}

	zapCfg.Level = zap.NewAtomicLevelAt(level)
	if cfg.Encoding == "json" || cfg.Encoding == "console" {
		zapCfg.Encoding = cfg.Encoding
	} else {
		zapCfg.Encoding = "console"
	}

	// ISO8601 timestamp required for financial audit trails.
	zapCfg.EncoderConfig.TimeKey = "timestamp"
	zapCfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	zapCfg.EncoderConfig.CallerKey = "caller"
	zapCfg.EncoderConfig.EncodeCaller = zapcore.ShortCallerEncoder

	logger, err := zapCfg.Build(
		zap.AddCaller(),
		zap.AddStacktrace(zapcore.ErrorLevel),
	)
	if err != nil {
		return nil, fmt.Errorf("build zap logger: %w", err)
	}

	zap.ReplaceGlobals(logger)
	return logger, nil
}
