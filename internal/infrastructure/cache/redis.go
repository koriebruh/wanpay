package cache

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/samber/do/v2"
	"go.uber.org/zap"

	"wanpey/core/internal/infrastructure/config"
)

type redisCache struct {
	client *redis.Client
}

// Shutdown implements do.Shutdownable — called by injector.Shutdown().
func (r *redisCache) Shutdown() error {
	zap.L().Info("closing redis connection")
	return r.client.Close()
}

// Ping verifies Redis is reachable. Used by the health check endpoint.
func (r *redisCache) Ping(ctx context.Context) error {
	return r.client.Ping(ctx).Err()
}

func (r *redisCache) Get(ctx context.Context, key string) ([]byte, error) {
	val, err := r.client.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, ErrCacheMiss
	}
	return val, err
}

func (r *redisCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	return r.client.Set(ctx, key, value, ttl).Err()
}

func (r *redisCache) SetNX(ctx context.Context, key string, value []byte, ttl time.Duration) (bool, error) {
	return r.client.SetNX(ctx, key, value, ttl).Result()
}

func (r *redisCache) Del(ctx context.Context, keys ...string) error {
	return r.client.Del(ctx, keys...).Err()
}

func (r *redisCache) Exists(ctx context.Context, key string) (bool, error) {
	n, err := r.client.Exists(ctx, key).Result()
	return n > 0, err
}

// ProvideCache registers Cache in the DI container.
// Redis enabled  → redisCache (distributed, production-grade).
// Redis disabled → memoryCache (in-process, TTL-aware, single-instance only).
// Swap to a different backend by changing only this file.
func ProvideCache(i do.Injector) {
	do.Provide(i, func(i do.Injector) (Cache, error) {
		cfg := do.MustInvoke[*config.Config](i)
		log := do.MustInvoke[*zap.Logger](i)

		if !cfg.Redis.Enabled {
			log.Warn("redis disabled — falling back to in-memory cache (not suitable for multi-instance)")
			return newMemoryCache(), nil
		}

		client := redis.NewClient(&redis.Options{
			Addr:     cfg.Redis.Addr,
			Password: cfg.Redis.Password,
			DB:       cfg.Redis.DB,
		})

		pingCtx, pingCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer pingCancel()
		if err := client.Ping(pingCtx).Err(); err != nil {
			_ = client.Close()
			return nil, fmt.Errorf("ping redis: %w", err)
		}

		log.Info("redis connected", zap.String("addr", cfg.Redis.Addr))
		return &redisCache{client: client}, nil
	})
}
