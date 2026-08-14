// Package ratelimit provides Redis-backed fixed-window rate limiting for HTTP
// endpoints.
package ratelimit

import (
	"net"
	"net/http"
	"time"

	"github.com/ripper19/simulator/internal/coord"
)

// Middleware returns rate-limiting middleware. Keyed by the client's IP
// (stripped of the port) unless a key function is provided. When Redis is
// unavailable the middleware fails open (passes the request through).
func Middleware(c *coord.Redis, limit int64, window time.Duration, keyFn func(*http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := "ratelimit:" + clientIP(r)
			if keyFn != nil {
				key = "ratelimit:" + keyFn(r)
			}
			ok, err := c.RateLimit(r.Context(), key, limit, window)
			if err != nil {
				next.ServeHTTP(w, r) // fail open on coordinator error
				return
			}
			if !ok {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Retry-After", time.Now().Add(window).Format(http.TimeFormat))
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"error":{"code":"rate_limited","message":"too many requests"}}`))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func clientIP(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
