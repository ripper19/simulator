package ratelimit

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/ripper19/simulator/internal/coord"
)

func TestMiddlewareRateLimits(t *testing.T) {
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		t.Skip("REDIS_ADDR not set; skipping rate limit test")
	}
	c, err := coord.NewRedis(addr, 0)
	if err != nil {
		t.Skipf("cannot connect to Redis: %v", err)
	}
	defer c.Close()

	const limit = int64(2)
	mw := Middleware(c, limit, time.Minute, nil)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	client := fmt.Sprintf("192.0.2.%d", time.Now().UnixNano()%1000)
	for i := int64(0); i < 3; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = client + ":1234"
		h.ServeHTTP(rec, req)
		if i < limit && rec.Code != http.StatusOK {
			t.Fatalf("request %d should pass, got %d", i, rec.Code)
		}
		if i >= limit && rec.Code != http.StatusTooManyRequests {
			t.Fatalf("request %d should be rate-limited, got %d", i, rec.Code)
		}
	}
}
