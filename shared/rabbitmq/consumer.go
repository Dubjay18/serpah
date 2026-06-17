package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Message is the decoded form of an incoming AMQP delivery.
type Message struct {
	// Raw is the full Event envelope decoded from JSON.
	Raw Event
	// delivery is kept private; consumers use Ack/Nack methods.
	delivery amqp.Delivery
}

// Ack acknowledges successful processing of the message.
func (m *Message) Ack() error { return m.delivery.Ack(false) }

// Nack rejects the message. When requeue is false the broker routes it to the
// dead-letter exchange (seraph.dlq); when true it is re-enqueued immediately.
func (m *Message) Nack(requeue bool) error { return m.delivery.Nack(false, requeue) }

// Handler is a callback invoked for each received message.
// Return nil to Ack; return an error to Nack (to DLQ, no requeue).
type Handler func(ctx context.Context, msg Message) error

// Consumer subscribes to a queue and dispatches deliveries to a Handler.
type Consumer struct {
	ch *amqp.Channel
}

// NewConsumer wraps an open channel. The channel should already have its
// prefetch set (handled by Connection.Channel).
func NewConsumer(ch *amqp.Channel) *Consumer {
	return &Consumer{ch: ch}
}

// Subscribe begins consuming from queue. Each delivery is decoded and passed
// to handler in its own goroutine. The method blocks until ctx is cancelled
// or the channel is closed by the broker.
//
// consumerTag must be unique per channel (e.g. "<service>-<queue>").
func (c *Consumer) Subscribe(ctx context.Context, queue, consumerTag string, handler Handler) error {
	deliveries, err := c.ch.Consume(
		queue,       // queue
		consumerTag, // consumer tag
		false,       // auto-ack (we ack manually)
		false,       // exclusive
		false,       // no-local
		false,       // no-wait
		nil,         // args
	)
	if err != nil {
		return fmt.Errorf("rabbitmq: consume %s: %w", queue, err)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case d, ok := <-deliveries:
			if !ok {
				return fmt.Errorf("rabbitmq: delivery channel closed for queue %s", queue)
			}
			go func(d amqp.Delivery) {
				var evt Event
				if err := json.Unmarshal(d.Body, &evt); err != nil {
					// Malformed JSON — send straight to DLQ, don't requeue.
					_ = d.Nack(false, false)
					return
				}
				msg := Message{Raw: evt, delivery: d}
				if err := handler(ctx, msg); err != nil {
					// Handler returned an error — route to DLQ.
					_ = msg.Nack(false)
					return
				}
				_ = msg.Ack()
			}(d)
		}
	}
}
