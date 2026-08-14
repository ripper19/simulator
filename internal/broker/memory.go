package broker

import (
	"context"
	"sync"
)

// Memory is an in-memory Broker for local development and tests. It provides
// the same Publish/Consume/Ack/Nack semantics as the RabbitMQ implementation,
// including requeue on Nack(requeue=true), so consumers behave identically.
type Memory struct {
	mu     sync.Mutex
	queues map[string]*memQueue
	closed bool
}

type memQueue struct {
	ch   chan []byte
	done chan struct{}
}

// NewMemory returns an empty in-memory broker.
func NewMemory() *Memory {
	return &Memory{queues: make(map[string]*memQueue)}
}

func (m *Memory) getQueue(name string) *memQueue {
	m.mu.Lock()
	defer m.mu.Unlock()
	q, ok := m.queues[name]
	if !ok {
		q = &memQueue{ch: make(chan []byte, 1024), done: make(chan struct{})}
		m.queues[name] = q
	}
	return q
}

// Publish enqueues body onto the named queue.
func (m *Memory) Publish(ctx context.Context, queue string, body []byte) error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return ErrClosed
	}
	m.mu.Unlock()

	q := m.getQueue(queue)
	select {
	case q.ch <- body:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-q.done:
		return ErrClosed
	}
}

// Consume delivers messages from the queue to handler until ctx is cancelled.
func (m *Memory) Consume(ctx context.Context, queue string, handler Handler) error {
	q := m.getQueue(queue)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-q.done:
			return ErrClosed
		case body := <-q.ch:
			acked := make(chan struct{})
			var once sync.Once
			d := Delivery{
				Body: body,
				Ack:  func() error { once.Do(func() { close(acked) }); return nil },
				Nack: func(requeue bool) error {
					var retErr error
					once.Do(func() {
						if requeue {
							retErr = m.Publish(context.Background(), queue, body)
						} else {
							close(acked)
						}
					})
					return retErr
				},
			}
			if err := handler(ctx, d); err != nil {
				// handler returned an error without ack/nack: treat as non-requeue reject
				once.Do(func() { close(acked) })
			}
		}
	}
}

// Close stops the broker.
func (m *Memory) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil
	}
	m.closed = true
	for _, q := range m.queues {
		close(q.done)
	}
	return nil
}

// ErrClosed is returned by Publish/Consume after Close.
var ErrClosed = &closedError{}

type closedError struct{}

func (e *closedError) Error() string { return "broker: closed" }
