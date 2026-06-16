package telemetry

import (
	"context"
	"fmt"
	"time"

	"github.com/samber/do/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.uber.org/zap"

	"wanpey/core/internal/infrastructure/config"
)

type Tracer struct {
	provider *sdktrace.TracerProvider
}

func (t *Tracer) Shutdown() error {
	zap.L().Info("flushing tracer spans to jaeger...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return t.provider.Shutdown(ctx)
}

func ProvideTracer(i do.Injector) {
	do.Provide(i, func(i do.Injector) (*Tracer, error) {
		cfg := do.MustInvoke[*config.Config](i)
		log := do.MustInvoke[*zap.Logger](i)
		return newTracer(cfg, log)
	})
}

func newTracer(cfg *config.Config, log *zap.Logger) (*Tracer, error) {
	if !cfg.OTEL.Enabled {
		tp := sdktrace.NewTracerProvider()
		otel.SetTracerProvider(tp)
		log.Info("tracing disabled")
		return &Tracer{provider: tp}, nil
	}

	ctx := context.Background()

	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(cfg.OTEL.Endpoint),
		otlptracegrpc.WithInsecure(), // replace with TLS in production
	)
	if err != nil {
		return nil, fmt.Errorf("create otlp exporter: %w", err)
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(cfg.OTEL.ServiceName),
			attribute.String("deployment.environment", cfg.App.Env),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("create otel resource: %w", err)
	}

	// sample_ratio=1.0 in dev; lower (e.g. 0.1) in production to reduce Jaeger load.
	sampler := sdktrace.ParentBased(
		sdktrace.TraceIDRatioBased(cfg.OTEL.SampleRatio),
	)

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	log.Info("tracing enabled",
		zap.String("endpoint", cfg.OTEL.Endpoint),
		zap.String("service", cfg.OTEL.ServiceName),
		zap.Float64("sample_ratio", cfg.OTEL.SampleRatio),
	)

	return &Tracer{provider: tp}, nil
}
