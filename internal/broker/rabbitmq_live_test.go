package broker

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestRabbitMQLive exercises the RabbitMQ broker against a live server. Set
// AMQP_URL to run it (e.g. amqp://guest:guest@127.0.0.1:5672/).
func TestRabbitMQLive(t *testing.T) {
	url := os.Getenv("AMQP_URL")
	if url == "" {
		t.Skip("AMQP_URL not set; skipping RabbitMQ live test")
	}
	b, err := NewRabbitMQ(url)
	if err != nil {
		t.Skipf("cannot connect to RabbitMQ: %v", err)
	}
	defer b.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	const queue = "simulator.test.live"
	got := make(chan []byte, 1)
	go func() {
		_ = b.Consume(ctx, queue, func(ctx context.Context, d Delivery) error {
			got <- d.Body
			return d.Ack()
		})
	}()
	time.Sleep(200 * time.Millisecond) // allow consumer to start

	if err := b.Publish(ctx, queue, []byte("hello-rabbit")); err != nil {
		t.Fatal(err)
	}
	select {
	case body := <-got:
		if string(body) != "hello-rabbit" {
			t.Fatalf("got %q", body)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for RabbitMQ message")
	}
}
