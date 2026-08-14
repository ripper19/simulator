package broker

import (
	"context"
	"fmt"
	"sync"

	amqp "github.com/rabbitmq/amqp091-go"
)

// RabbitMQ is a Broker backed by a RabbitMQ server. Queues are declared as
// durable and messages are acknowledged manually.
type RabbitMQ struct {
	conn *amqp.Connection
	ch   *amqp.Channel
	url  string

	mu      sync.Mutex
	consume map[string]struct{}
}

// NewRabbitMQ connects to the broker at url (amqp://...).
func NewRabbitMQ(url string) (*RabbitMQ, error) {
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, fmt.Errorf("broker: dial: %w", err)
	}
	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("broker: channel: %w", err)
	}
	return &RabbitMQ{conn: conn, ch: ch, url: url, consume: make(map[string]struct{})}, nil
}

// Publish declares (if needed) and publishes to a durable queue.
func (r *RabbitMQ) Publish(ctx context.Context, queue string, body []byte) error {
	if _, err := r.ch.QueueDeclare(queue, true, false, false, false, nil); err != nil {
		return fmt.Errorf("broker: declare %q: %w", queue, err)
	}
	return r.ch.PublishWithContext(ctx, "", queue, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Body:         body,
	})
}

// Consume delivers messages from the queue, acknowledging on handler success
// and rejecting on error.
func (r *RabbitMQ) Consume(ctx context.Context, queue string, handler Handler) error {
	if _, err := r.ch.QueueDeclare(queue, true, false, false, false, nil); err != nil {
		return fmt.Errorf("broker: declare %q: %w", queue, err)
	}
	if err := r.ch.Qos(1, 0, false); err != nil {
		return fmt.Errorf("broker: qos: %w", err)
	}
	msgs, err := r.ch.Consume(queue, "", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("broker: consume %q: %w", queue, err)
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-msgs:
			if !ok {
				return fmt.Errorf("broker: consume channel closed for %q", queue)
			}
			d := Delivery{
				Body: msg.Body,
				Ack:  func() error { return msg.Ack(false) },
				Nack: func(requeue bool) error { return msg.Nack(false, requeue) },
			}
			if err := handler(ctx, d); err != nil {
				_ = msg.Nack(false, false)
			}
		}
	}
}

// Close shuts down the channel and connection.
func (r *RabbitMQ) Close() error {
	var err error
	if r.ch != nil {
		err = r.ch.Close()
	}
	if r.conn != nil {
		if cerr := r.conn.Close(); err == nil {
			err = cerr
		}
	}
	return err
}
