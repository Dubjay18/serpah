// Package consumer wires the accounts service to its RabbitMQ subscriptions.
package consumer

import (
	"context"
	"encoding/json"
	"fmt"

	"go.uber.org/zap"

	"github.com/Dubjay18/seraph/shared/rabbitmq"
)

// AccountsConsumer handles inbound events relevant to the accounts domain.
type AccountsConsumer struct {
	consumer *rabbitmq.Consumer
	log      *zap.Logger
}

// New creates an AccountsConsumer backed by the given rabbitmq.Consumer.
func New(c *rabbitmq.Consumer, log *zap.Logger) *AccountsConsumer {
	return &AccountsConsumer{consumer: c, log: log}
}

// Start begins consuming from both auth.events and payments.events queues.
// It blocks until ctx is cancelled. Each queue runs in its own goroutine.
func (ac *AccountsConsumer) Start(ctx context.Context) {
	go func() {
		if err := ac.consumer.Subscribe(
			ctx,
			rabbitmq.QueueAuthEvents,
			"accounts-auth-events",
			ac.handleAuthEvent,
		); err != nil {
			ac.log.Error("auth events consumer stopped", zap.Error(err))
		}
	}()

	go func() {
		if err := ac.consumer.Subscribe(
			ctx,
			rabbitmq.QueuePaymentEvents,
			"accounts-payment-events",
			ac.handlePaymentEvent,
		); err != nil {
			ac.log.Error("payment events consumer stopped", zap.Error(err))
		}
	}()
}

// ─── Auth event handlers ──────────────────────────────────────────────────────

func (ac *AccountsConsumer) handleAuthEvent(_ context.Context, msg rabbitmq.Message) error {
	switch msg.Raw.Type {
	case rabbitmq.EventUserRegistered:
		return ac.onUserRegistered(msg)
	case rabbitmq.EventUserDeactivated:
		return ac.onUserDeactivated(msg)
	default:
		// Unknown event type — ack and move on so we don't block the queue.
		ac.log.Debug("accounts: ignoring unknown auth event", zap.String("type", msg.Raw.Type))
		return nil
	}
}

func (ac *AccountsConsumer) onUserRegistered(msg rabbitmq.Message) error {
	var payload rabbitmq.UserRegisteredPayload
	if err := remarshal(msg.Raw.Payload, &payload); err != nil {
		return fmt.Errorf("accounts: decode UserRegisteredPayload: %w", err)
	}

	ac.log.Info("accounts: user registered — provisioning default wallet",
		zap.String("user_id", payload.UserID),
		zap.String("email", payload.Email),
	)

	// TODO: call AccountsService.ProvisionDefaultWallet(ctx, payload.UserID)
	// This is left as a TODO until the accounts repository is implemented.

	return nil
}

func (ac *AccountsConsumer) onUserDeactivated(msg rabbitmq.Message) error {
	var payload rabbitmq.UserDeactivatedPayload
	if err := remarshal(msg.Raw.Payload, &payload); err != nil {
		return fmt.Errorf("accounts: decode UserDeactivatedPayload: %w", err)
	}

	ac.log.Info("accounts: user deactivated — freezing accounts",
		zap.String("user_id", payload.UserID),
	)

	// TODO: call AccountsService.FreezeUserAccounts(ctx, payload.UserID)

	return nil
}

// ─── Payment event handlers ───────────────────────────────────────────────────

func (ac *AccountsConsumer) handlePaymentEvent(_ context.Context, msg rabbitmq.Message) error {
	switch msg.Raw.Type {
	case rabbitmq.EventPaymentCompleted:
		return ac.onPaymentCompleted(msg)
	case rabbitmq.EventPaymentFailed:
		return ac.onPaymentFailed(msg)
	default:
		ac.log.Debug("accounts: ignoring payment event", zap.String("type", msg.Raw.Type))
		return nil
	}
}

func (ac *AccountsConsumer) onPaymentCompleted(msg rabbitmq.Message) error {
	var payload rabbitmq.PaymentCompletedPayload
	if err := remarshal(msg.Raw.Payload, &payload); err != nil {
		return fmt.Errorf("accounts: decode PaymentCompletedPayload: %w", err)
	}

	ac.log.Info("accounts: payment completed — triggering balance reconciliation",
		zap.String("payment_id", payload.PaymentID),
		zap.Int64("amount_kobo", payload.AmountKobo),
	)

	// TODO: call AccountsService.ReconcileBalance(ctx, payload)

	return nil
}

func (ac *AccountsConsumer) onPaymentFailed(msg rabbitmq.Message) error {
	var payload rabbitmq.PaymentFailedPayload
	if err := remarshal(msg.Raw.Payload, &payload); err != nil {
		return fmt.Errorf("accounts: decode PaymentFailedPayload: %w", err)
	}

	ac.log.Info("accounts: payment failed — releasing reserved funds",
		zap.String("payment_id", payload.PaymentID),
		zap.String("sender_id", payload.SenderID),
	)

	// TODO: call AccountsService.ReleaseReservation(ctx, payload.PaymentID)

	return nil
}



// ─── Helper ───────────────────────────────────────────────────────────────────

// remarshal round-trips v through JSON to decode the untyped any payload into
// the concrete target struct.
func remarshal(src any, dst any) error {
	b, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dst)
}
