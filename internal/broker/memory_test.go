package broker

import (
	"context"
	"testing"
	"time"
)

func TestMemoryPublishConsume(t *testing.T) {
	b := NewMemory()
	defer b.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	got := make(chan []byte, 1)
	go func() {
		_ = b.Consume(ctx, "q", func(ctx context.Context, d Delivery) error {
			got <- d.Body
			return d.Ack()
		})
	}()

	if err := b.Publish(context.Background(), "q", []byte("hello")); err != nil {
		t.Fatal(err)
	}
	select {
	case body := <-got:
		if string(body) != "hello" {
			t.Fatalf("got %q", body)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for message")
	}
}

func TestMemoryNackRequeues(t *testing.T) {
	b := NewMemory()
	defer b.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	delivered := make(chan struct{}, 4)
	mu := make(chan struct{}, 1)
	attempts := 0
	go func() {
		_ = b.Consume(ctx, "q", func(ctx context.Context, d Delivery) error {
			mu <- struct{}{}
			attempts++
			<-mu
			if attempts == 1 {
				delivered <- struct{}{}
				return d.Nack(true) // requeue
			}
			delivered <- struct{}{}
			return d.Ack()
		})
	}()

	_ = b.Publish(context.Background(), "q", []byte("x"))
	<-delivered
	select {
	case <-delivered:
		// second delivery after requeue
	case <-time.After(2 * time.Second):
		t.Fatal("message was not redelivered after Nack(requeue=true)")
	}
}

func TestMemoryClose(t *testing.T) {
	b := NewMemory()
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	if err := b.Publish(context.Background(), "q", []byte("x")); err != ErrClosed {
		t.Fatalf("expected ErrClosed, got %v", err)
	}
}
