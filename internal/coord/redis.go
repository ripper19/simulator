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

// RateLimit increments a fixed-window counter and reports whether the request
// is within the limit. The counter expires after the window.
func (r *Redis) RateLimit(ctx context.Context, key string, limit int64, window time.Duration) (bool, error) {
	const script = `
local n = redis.call('INCR', KEYS[1])
if n == 1 then redis.call('EXPIRE', KEYS[1], ARGV[1]) end
return n`
	n, err := r.client.Eval(ctx, script, []string{key}, int64(window.Seconds())).Int64()
	if err != nil {
		return false, err
	}
	return n <= limit, nil
}

// Close releases the client.
func (r *Redis) Close() error { return r.client.Close() }
