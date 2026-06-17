package rabbitmq

import (
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

// ─── Exchange / queue names ───────────────────────────────────────────────────

const (
	// ExchangeEvents is the single topic exchange all services publish to.
	ExchangeEvents = "seraph.events"

	// QueueAuthEvents receives all messages with routing key "auth.*".
	QueueAuthEvents = "auth.events"

	// QueuePaymentEvents receives all messages with routing key "payments.*".
	QueuePaymentEvents = "payments.events"

	// QueueDLQ receives messages that could not be processed after retries.
	QueueDLQ = "seraph.dlq"

	// ExchangeDLQ is the dead-letter exchange that backs QueueDLQ.
	ExchangeDLQ = "seraph.dlq"
)

// DeclareTopology idempotently declares all exchanges, queues, and bindings.
// It is safe to call on every service startup — RabbitMQ will no-op if they
// already exist with identical settings.
func DeclareTopology(ch *amqp.Channel) error {
	// ── Dead-letter exchange + queue ─────────────────────────────────────────
	if err := ch.ExchangeDeclare(
		ExchangeDLQ, "fanout", true, false, false, false, nil,
	); err != nil {
		return fmt.Errorf("rabbitmq: declare DLX: %w", err)
	}

	if _, err := ch.QueueDeclare(
		QueueDLQ, true, false, false, false, nil,
	); err != nil {
		return fmt.Errorf("rabbitmq: declare DLQ: %w", err)
	}

	if err := ch.QueueBind(QueueDLQ, "#", ExchangeDLQ, false, nil); err != nil {
		return fmt.Errorf("rabbitmq: bind DLQ: %w", err)
	}

	// ── Main topic exchange ───────────────────────────────────────────────────
	if err := ch.ExchangeDeclare(
		ExchangeEvents, "topic", true, false, false, false, nil,
	); err != nil {
		return fmt.Errorf("rabbitmq: declare exchange: %w", err)
	}

	// Queue args: dead-letter any rejected message to the DLX.
	dlxArgs := amqp.Table{
		"x-dead-letter-exchange": ExchangeDLQ,
	}

	// ── auth.events ──────────────────────────────────────────────────────────
	if _, err := ch.QueueDeclare(
		QueueAuthEvents, true, false, false, false, dlxArgs,
	); err != nil {
		return fmt.Errorf("rabbitmq: declare %s: %w", QueueAuthEvents, err)
	}
	if err := ch.QueueBind(QueueAuthEvents, "auth.*", ExchangeEvents, false, nil); err != nil {
		return fmt.Errorf("rabbitmq: bind %s: %w", QueueAuthEvents, err)
	}

	// ── payments.events ──────────────────────────────────────────────────────
	if _, err := ch.QueueDeclare(
		QueuePaymentEvents, true, false, false, false, dlxArgs,
	); err != nil {
		return fmt.Errorf("rabbitmq: declare %s: %w", QueuePaymentEvents, err)
	}
	if err := ch.QueueBind(QueuePaymentEvents, "payments.*", ExchangeEvents, false, nil); err != nil {
		return fmt.Errorf("rabbitmq: bind %s: %w", QueuePaymentEvents, err)
	}

	return nil
}
