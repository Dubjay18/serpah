package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
)

// Publisher sends events to the seraph.events topic exchange.
type Publisher struct {
	ch *amqp.Channel
}

// NewPublisher wraps an already-open channel.
// Call DeclareTopology before creating a publisher if this is the first
// service to start.
func NewPublisher(ch *amqp.Channel) *Publisher {
	return &Publisher{ch: ch}
}

// Publish marshals payload into an Event envelope and routes it to the
// seraph.events exchange using routingKey (e.g. "auth.user.registered").
//
// The message is marked persistent (DeliveryMode 2) so it survives a
// broker restart.
func (p *Publisher) Publish(ctx context.Context, routingKey string, payload any) error {
	evt := Event{
		ID:         uuid.New().String(),
		Type:       routingKey,
		Version:    1,
		OccurredAt: time.Now().UTC(),
		Payload:    payload,
	}

	body, err := json.Marshal(evt)
	if err != nil {
		return fmt.Errorf("rabbitmq: marshal event %s: %w", routingKey, err)
	}

	return p.ch.PublishWithContext(ctx,
		ExchangeEvents, // exchange
		routingKey,     // routing key
		false,          // mandatory
		false,          // immediate
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			MessageId:    evt.ID,
			Timestamp:    evt.OccurredAt,
			Body:         body,
		},
	)
}
