package broker

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

var errChannelClosed = errors.New("broker: channel closed")

// RabbitMQ is a Broker backed by a RabbitMQ server. Queues are declared as
// durable and messages are acknowledged manually. Dropped connections and
// closed consume channels are recovered with bounded exponential backoff.
type RabbitMQ struct {
	url  string
	mu   sync.Mutex
	conn *amqp.Connection
	ch   *amqp.Channel
}

// NewRabbitMQ connects to the broker at url (amqp://...).
func NewRabbitMQ(url string) (*RabbitMQ, error) {
	r := &RabbitMQ{url: url}
	if err := r.dial(); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *RabbitMQ) dial() error {
	conn, err := amqp.Dial(r.url)
	if err != nil {
		return fmt.Errorf("broker: dial: %w", err)
	}
	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return fmt.Errorf("broker: channel: %w", err)
	}
	r.conn, r.ch = conn, ch
	return nil
}

// reconnect closes the current connection/channel and establishes fresh ones.
func (r *RabbitMQ) reconnect() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.ch != nil {
		_ = r.ch.Close()
	}
	if r.conn != nil {
		_ = r.conn.Close()
	}
	conn, err := amqp.Dial(r.url)
	if err != nil {
		return err
	}
	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return err
	}
	r.conn, r.ch = conn, ch
	return nil
}

// Publish declares (if needed) and publishes to a durable queue, reconnecting
// once on a dropped channel.
func (r *RabbitMQ) Publish(ctx context.Context, queue string, body []byte) error {
	r.mu.Lock()
	ch := r.ch
	r.mu.Unlock()
	if err := r.publishOn(ch, ctx, queue, body); err == nil {
		return nil
	} else if ctx.Err() != nil {
		return ctx.Err()
	} else if rerr := r.reconnect(); rerr != nil {
		return err
	}
	r.mu.Lock()
	ch = r.ch
	r.mu.Unlock()
	return r.publishOn(ch, ctx, queue, body)
}

func (r *RabbitMQ) publishOn(ch *amqp.Channel, ctx context.Context, queue string, body []byte) error {
	if _, err := ch.QueueDeclare(queue, true, false, false, false, nil); err != nil {
		return fmt.Errorf("broker: declare %q: %w", queue, err)
	}
	return ch.PublishWithContext(ctx, "", queue, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Body:         body,
	})
}

// Consume delivers messages from the queue, acknowledging on handler success
// and rejecting on error. It reconnects with backoff if the channel closes.
func (r *RabbitMQ) Consume(ctx context.Context, queue string, handler Handler) error {
	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		err := r.consumeOnce(ctx, queue, handler)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !errors.Is(err, errChannelClosed) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
		if rerr := r.reconnect(); rerr != nil {
			if backoff < 30*time.Second {
				backoff *= 2
			}
		} else {
			backoff = time.Second
		}
	}
}

func (r *RabbitMQ) consumeOnce(ctx context.Context, queue string, handler Handler) error {
	r.mu.Lock()
	ch := r.ch
	r.mu.Unlock()
	if _, err := ch.QueueDeclare(queue, true, false, false, false, nil); err != nil {
		return fmt.Errorf("broker: declare %q: %w", queue, err)
	}
	if err := ch.Qos(1, 0, false); err != nil {
		return fmt.Errorf("broker: qos: %w", err)
	}
	msgs, err := ch.Consume(queue, "", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("broker: consume %q: %w", queue, err)
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-msgs:
			if !ok {
				return errChannelClosed
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
	r.mu.Lock()
	defer r.mu.Unlock()
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
