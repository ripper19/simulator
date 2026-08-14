// Package broker defines the message-broker abstraction used for job dispatch
// and worker communication, with in-memory and RabbitMQ implementations. The
// interface is deliberately minimal so the backing broker can be replaced
// without changing consumers.
package broker

import "context"

// Delivery is a single received message. Ack positively acknowledges the
// message (it will not be redelivered); Nack rejects it, optionally requesting
// redelivery.
type Delivery struct {
	Body []byte
	Ack  func() error
	Nack func(requeue bool) error
}

// Handler processes a single delivery. It must call Ack or Nack exactly once,
// unless it returns an error (in which case the consumer treats it as a
// non-requeued rejection).
type Handler func(ctx context.Context, d Delivery) error

// Broker is a minimal publish/subscribe-queue abstraction.
type Broker interface {
	// Publish sends body to the named queue, creating it if necessary.
	Publish(ctx context.Context, queue string, body []byte) error
	// Consume delivers messages from the queue to handler until ctx is
	// cancelled. Implementations may call handler concurrently.
	Consume(ctx context.Context, queue string, handler Handler) error
	// Close releases broker resources.
	Close() error
}
