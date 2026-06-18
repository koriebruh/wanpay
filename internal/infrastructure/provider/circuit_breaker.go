package provider

import (
	"context"
	"fmt"
	"time"

	"github.com/sony/gobreaker"
	"go.uber.org/zap"

	"wanpey/core/internal/domain/entity"
	"wanpey/core/internal/domain/gateway"
	"wanpey/core/internal/infrastructure/config"
)

type Breaker struct {
	cb   *gobreaker.CircuitBreaker
	name string
}

func NewBreaker(providerName string, cfg config.CircuitBreakerConfig, log *zap.Logger) *Breaker {
	intervalSeconds := cfg.IntervalSeconds
	if intervalSeconds == 0 {
		intervalSeconds = 60
	}
	timeoutSeconds := cfg.TimeoutSeconds
	if timeoutSeconds == 0 {
		timeoutSeconds = 30
	}
	maxRequests := cfg.MaxRequests
	if maxRequests == 0 {
		maxRequests = 3
	}
	consecutiveFailures := cfg.ConsecutiveFailures
	if consecutiveFailures == 0 {
		consecutiveFailures = 5
	}

	return &Breaker{
		name: providerName,
		cb: gobreaker.NewCircuitBreaker(gobreaker.Settings{
			Name:        providerName,
			MaxRequests: maxRequests,
			Interval:    time.Duration(intervalSeconds) * time.Second,
			Timeout:     time.Duration(timeoutSeconds) * time.Second,
			ReadyToTrip: func(c gobreaker.Counts) bool {
				return c.ConsecutiveFailures >= consecutiveFailures
			},
			OnStateChange: func(name string, from, to gobreaker.State) {
				log.Warn("circuit breaker state changed",
					zap.String("provider", name),
					zap.String("from", from.String()),
					zap.String("to", to.String()),
				)
			},
		}),
	}
}

func (b *Breaker) IsOpen() bool { return b.cb.State() == gobreaker.StateOpen }

// run is a package-level generic helper — Go does not support generic methods.
func run[T any](b *Breaker, fn func() (T, error)) (T, error) {
	result, err := b.cb.Execute(func() (any, error) { return fn() })
	if err != nil {
		var zero T
		return zero, fmt.Errorf("provider %s: %w", b.name, err)
	}
	return result.(T), nil //nolint:forcetypeassert,errcheck // safe: fn returns T, gobreaker passes it through unchanged
}

type CBPaymentGateway struct {
	inner   gateway.PaymentGateway
	breaker *Breaker
}

func NewCBPaymentGateway(inner gateway.PaymentGateway, cfg config.CircuitBreakerConfig, log *zap.Logger) gateway.PaymentGateway {
	return &CBPaymentGateway{
		inner:   inner,
		breaker: NewBreaker(string(inner.ProviderName()), cfg, log),
	}
}

func (g *CBPaymentGateway) CreateVA(ctx context.Context, req gateway.CreateVARequest) (*gateway.CreateVAResponse, error) {
	return run(g.breaker, func() (*gateway.CreateVAResponse, error) {
		return g.inner.CreateVA(ctx, req)
	})
}

func (g *CBPaymentGateway) CreateQRIS(ctx context.Context, req gateway.CreateQRISRequest) (*gateway.CreateQRISResponse, error) {
	return run(g.breaker, func() (*gateway.CreateQRISResponse, error) {
		return g.inner.CreateQRIS(ctx, req)
	})
}

func (g *CBPaymentGateway) CancelPayment(ctx context.Context, externalID string) error {
	_, err := run(g.breaker, func() (struct{}, error) {
		return struct{}{}, g.inner.CancelPayment(ctx, externalID)
	})
	return err
}

func (g *CBPaymentGateway) GetStatus(ctx context.Context, externalID string) (entity.PaymentStatus, error) {
	return run(g.breaker, func() (entity.PaymentStatus, error) {
		return g.inner.GetStatus(ctx, externalID)
	})
}

func (g *CBPaymentGateway) ParseWebhook(ctx context.Context, headers map[string]string, body []byte) (*gateway.WebhookEvent, error) {
	return g.inner.ParseWebhook(ctx, headers, body)
}

func (g *CBPaymentGateway) SupportedMethods() []entity.PaymentMethod {
	return g.inner.SupportedMethods()
}
func (g *CBPaymentGateway) ProviderName() entity.Provider { return g.inner.ProviderName() }

type CBDisbursementGateway struct {
	inner   gateway.DisbursementGateway
	breaker *Breaker
}

func NewCBDisbursementGateway(inner gateway.DisbursementGateway, cfg config.CircuitBreakerConfig, log *zap.Logger) gateway.DisbursementGateway {
	return &CBDisbursementGateway{
		inner:   inner,
		breaker: NewBreaker(string(inner.ProviderName()), cfg, log),
	}
}

func (g *CBDisbursementGateway) Disburse(ctx context.Context, req gateway.DisburseRequest) (*gateway.DisburseResponse, error) {
	return run(g.breaker, func() (*gateway.DisburseResponse, error) {
		return g.inner.Disburse(ctx, req)
	})
}

func (g *CBDisbursementGateway) GetDisbursementStatus(ctx context.Context, externalID string) (*gateway.DisburseResponse, error) {
	return run(g.breaker, func() (*gateway.DisburseResponse, error) {
		return g.inner.GetDisbursementStatus(ctx, externalID)
	})
}

func (g *CBDisbursementGateway) ParseDisbursementWebhook(ctx context.Context, headers map[string]string, body []byte) (*gateway.DisbursementWebhookEvent, error) {
	return g.inner.ParseDisbursementWebhook(ctx, headers, body)
}

func (g *CBDisbursementGateway) ProviderName() entity.Provider { return g.inner.ProviderName() }
