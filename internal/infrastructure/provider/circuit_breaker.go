package provider

import (
	"fmt"
	"time"

	"github.com/sony/gobreaker"
	"go.uber.org/zap"
)

// Breaker wraps sony/gobreaker for payment provider calls.
// States: Closed (normal) → Open (provider down, fail fast) → Half-Open (probe recovery).
type Breaker struct {
	cb   *gobreaker.CircuitBreaker
	name string
}

type BreakerConfig struct {
	Name                         string
	MaxRequests                  uint32
	Interval                     time.Duration
	Timeout                      time.Duration
	ConsecutiveFailuresThreshold uint32
}

func DefaultBreakerConfig(providerName string) BreakerConfig {
	return BreakerConfig{
		Name:                         providerName,
		MaxRequests:                  3,
		Interval:                     60 * time.Second,
		Timeout:                      30 * time.Second,
		ConsecutiveFailuresThreshold: 5,
	}
}

func NewBreaker(cfg BreakerConfig, log *zap.Logger) *Breaker {
	settings := gobreaker.Settings{
		Name:        cfg.Name,
		MaxRequests: cfg.MaxRequests,
		Interval:    cfg.Interval,
		Timeout:     cfg.Timeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= cfg.ConsecutiveFailuresThreshold
		},
		OnStateChange: func(name string, from, to gobreaker.State) {
			log.Warn("circuit breaker state changed",
				zap.String("provider", name),
				zap.String("from", from.String()),
				zap.String("to", to.String()),
			)
		},
	}

	return &Breaker{
		cb:   gobreaker.NewCircuitBreaker(settings),
		name: cfg.Name,
	}
}

func (b *Breaker) Execute(fn func() (any, error)) (any, error) {
	result, err := b.cb.Execute(func() (any, error) {
		return fn()
	})
	if err != nil {
		return nil, fmt.Errorf("provider %s: %w", b.name, err)
	}
	return result, nil
}

func (b *Breaker) State() gobreaker.State {
	return b.cb.State()
}

func (b *Breaker) IsOpen() bool {
	return b.cb.State() == gobreaker.StateOpen
}
