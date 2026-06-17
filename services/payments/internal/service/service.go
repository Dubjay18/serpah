package service

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/Dubjay18/seraph/shared/money"
	"github.com/Dubjay18/seraph/shared/rabbitmq"
)

// EventPublisher is satisfied by *rabbitmq.Publisher and any test double.
type EventPublisher interface {
	Publish(ctx context.Context, routingKey string, payload any) error
}

// PaymentsService holds business logic for the payments domain.
type PaymentsService struct {
	events EventPublisher
	log    *zap.Logger
}

func New(events EventPublisher, log *zap.Logger) *PaymentsService {
	return &PaymentsService{events: events, log: log}
}

// Transition advances a payment's state and publishes the corresponding event.
// paymentID, senderID, receiverID should be UUID strings.
func (s *PaymentsService) Transition(
	ctx context.Context,
	current, next PaymentStatus,
	paymentID, senderID, receiverID string,
	amountKobo int64, currency money.Currency, idempotencyKey string,
) error {
	if err := validateTransition(current, next); err != nil {
		return err
	}

	if err := currency.Validate(); err != nil {
		return fmt.Errorf("payments: invalid currency: %w", err)
	}

	var routingKey string
	var payload any

	switch next {
	case StatusInitiated:
		routingKey = rabbitmq.EventPaymentInitiated
		payload = rabbitmq.PaymentInitiatedPayload{
			PaymentID:      paymentID,
			SenderID:       senderID,
			ReceiverID:     receiverID,
			AmountKobo:     amountKobo,
			Currency:       string(currency),
			IdempotencyKey: idempotencyKey,
		}
	case StatusCompleted:
		routingKey = rabbitmq.EventPaymentCompleted
		payload = rabbitmq.PaymentCompletedPayload{
			PaymentID:  paymentID,
			SenderID:   senderID,
			ReceiverID: receiverID,
			AmountKobo: amountKobo,
			Currency:   string(currency),
		}
	case StatusFailed:
		routingKey = rabbitmq.EventPaymentFailed
		payload = rabbitmq.PaymentFailedPayload{
			PaymentID: paymentID,
			SenderID:  senderID,
			Reason:    fmt.Sprintf("transition to %s", next),
		}
	case StatusReversed:
		routingKey = rabbitmq.EventPaymentReversed
		payload = rabbitmq.PaymentReversedPayload{
			PaymentID:  paymentID,
			SenderID:   senderID,
			ReceiverID: receiverID,
			AmountKobo: amountKobo,
			Currency:   string(currency),
		}
	default:
		// StatusProcessing has no external event — it is an internal guard state.
		return nil
	}

	if err := s.events.Publish(ctx, routingKey, payload); err != nil {
		s.log.Warn("failed to publish payment event",
			zap.String("routing_key", routingKey),
			zap.String("payment_id", paymentID),
			zap.Error(err),
		)
	}
	return nil
}
