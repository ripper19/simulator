package coord

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

func testRedis(t *testing.T) *Redis {
	t.Helper()
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		t.Skip("REDIS_ADDR not set; skipping coord test")
	}
	c, err := NewRedis(addr, 0)
	if err != nil {
		t.Skipf("cannot connect to Redis: %v", err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func TestHeartbeatAndAlive(t *testing.T) {
	c := testRedis(t)
	ctx := context.Background()
	key := fmt.Sprintf("coord:test:%d", time.Now().UnixNano())

	if c.Alive(ctx, key) {
		t.Fatal("key should not exist yet")
	}
	if err := c.Heartbeat(ctx, key, "1", time.Second); err != nil {
		t.Fatal(err)
	}
	if !c.Alive(ctx, key) {
		t.Fatal("key should be alive after heartbeat")
	}
	time.Sleep(1100 * time.Millisecond)
	if c.Alive(ctx, key) {
		t.Fatal("key should expire after TTL")
	}
}

func TestClaimAndRelease(t *testing.T) {
	c := testRedis(t)
	ctx := context.Background()
	key := fmt.Sprintf("coord:claim:%d", time.Now().UnixNano())

	ok, err := c.Claim(ctx, key, "1", time.Minute)
	if err != nil || !ok {
		t.Fatalf("first claim should succeed: %v %v", ok, err)
	}
	ok, err = c.Claim(ctx, key, "2", time.Minute)
	if err != nil || ok {
		t.Fatalf("second claim should fail: %v %v", ok, err)
	}
	if err := c.Release(ctx, key); err != nil {
		t.Fatal(err)
	}
	ok, err = c.Claim(ctx, key, "3", time.Minute)
	if err != nil || !ok {
		t.Fatalf("claim after release should succeed: %v %v", ok, err)
	}
}
