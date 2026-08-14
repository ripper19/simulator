// Package coord provides Redis-backed transient coordination: worker
// heartbeats, idempotency claims, and distributed locks. Redis is used only for
// transient coordination, never as the primary persistent state store.
package coord

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Redis wraps a go-redis client with coordination primitives.
type Redis struct {
	client *redis.Client
}

// NewRedis connects to Redis at addr (host:port).
func NewRedis(addr string, db int) (*Redis, error) {
	client := redis.NewClient(&redis.Options{Addr: addr, DB: db})
	if err := client.Ping(context.Background()).Err(); err != nil {
		client.Close()
		return nil, fmt.Errorf("coord: connect redis: %w", err)
	}
	return &Redis{client: client}, nil
}

// Ping checks connectivity.
func (r *Redis) Ping(ctx context.Context) error {
	return r.client.Ping(ctx).Err()
}

// Heartbeat sets a liveness key with a TTL (the worker renews it periodically;
// absence after the TTL means the worker is considered dead).
func (r *Redis) Heartbeat(ctx context.Context, key, value string, ttl time.Duration) error {
	return r.client.Set(ctx, key, value, ttl).Err()
}

// Alive reports whether a heartbeat key is currently present.
func (r *Redis) Alive(ctx context.Context, key string) bool {
	n, err := r.client.Exists(ctx, key).Result()
	return err == nil && n > 0
}

// Claim atomically sets key only if it does not already exist, returning
// whether this caller won the claim. Used for idempotency (a job key claimed
// once is never processed twice).
func (r *Redis) Claim(ctx context.Context, key, value string, ttl time.Duration) (bool, error) {
	return r.client.SetNX(ctx, key, value, ttl).Result()
}

// Release removes a previously claimed key, allowing a future Claim to succeed.
func (r *Redis) Release(ctx context.Context, key string) error {
	return r.client.Del(ctx, key).Err()
}

// Close releases the client.
func (r *Redis) Close() error { return r.client.Close() }
