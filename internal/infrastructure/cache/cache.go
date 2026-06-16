package cache

import (
	"context"
	"time"
)

// Cache is the abstraction over any key-value store used for application caching.
// Callers depend on this interface, not on Redis directly.
type Cache interface {
	// Get returns the value for key. Returns ErrCacheMiss if the key does not exist.
	Get(ctx context.Context, key string) ([]byte, error)

	// Set stores value with a TTL. Pass 0 for no expiry.
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error

	// SetNX stores value only if the key does not exist (atomic).
	// Returns true if the key was set, false if it already existed.
	SetNX(ctx context.Context, key string, value []byte, ttl time.Duration) (bool, error)

	// Del removes one or more keys.
	Del(ctx context.Context, keys ...string) error

	// Exists reports whether the key exists.
	Exists(ctx context.Context, key string) (bool, error)

	// Ping verifies the cache backend is reachable. Used by the health check endpoint.
	Ping(ctx context.Context) error
}

// ErrCacheMiss is returned by Get when the key is not found.
var ErrCacheMiss = errCacheMiss{}

type errCacheMiss struct{}

func (errCacheMiss) Error() string { return "cache: key not found" }
