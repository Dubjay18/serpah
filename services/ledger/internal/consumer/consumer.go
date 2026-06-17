// Package consumer wires the ledger service to its RabbitMQ subscriptions.
package consumer

import (
	"context"
	"encoding/json"
	"fmt"

	"go.uber.org/zap"

	"github.com/Dubjay18/seraph/services/ledger/internal/service"
	"github.com/Dubjay18/seraph/shared/money"
	"github.com/Dubjay18/seraph/shared/rabbitmq"
)

// LedgerConsumer handles inbound payment events and posts double-entry
// transactions via the LedgerService.
type LedgerConsumer struct {
	consumer *rabbitmq.Consumer
	svc      *service.LedgerService
	log      *zap.Logger
}

// New creates a LedgerConsumer.
func New(c *rabbitmq.Consumer, svc *service.LedgerService, log *zap.Logger) *LedgerConsumer {
	return &LedgerConsumer{
		consumer: c,
		svc:      svc,
		log:      log,
	}
}

// Start subscribes to payments.events and blocks until ctx is cancelled.
func (lc *LedgerConsumer) Start(ctx context.Context) {
	go func() {
		if err := lc.consumer.Subscribe(
			ctx,
			rabbitmq.QueuePaymentEvents,
			"ledger-payment-events",
			lc.handlePaymentEvent,
		); err != nil {
			lc.log.Error("payment events consumer stopped", zap.Error(err))
		}
	}()
}

func (lc *LedgerConsumer) handlePaymentEvent(ctx context.Context, msg rabbitmq.Message) error {
	switch msg.Raw.Type {
	case rabbitmq.EventPaymentInitiated:
		return lc.onPaymentInitiated(ctx, msg)
	case rabbitmq.EventPaymentReversed:
		return lc.onPaymentReversed(ctx, msg)
	default:
		lc.log.Debug("ledger: ignoring payment event", zap.String("type", msg.Raw.Type))
		return nil
	}
}

func (lc *LedgerConsumer) onPaymentInitiated(ctx context.Context, msg rabbitmq.Message) error {
	var payload rabbitmq.PaymentInitiatedPayload
	if err := remarshal(msg.Raw.Payload, &payload); err != nil {
		return fmt.Errorf("ledger: decode PaymentInitiatedPayload: %w", err)
	}

	lc.log.Info("ledger: posting double-entry for payment",
		zap.String("payment_id", payload.PaymentID),
		zap.String("sender_id", payload.SenderID),
		zap.String("receiver_id", payload.ReceiverID),
		zap.Int64("amount_kobo", payload.AmountKobo),
		zap.String("currency", payload.Currency),
	)

	amount, err := money.New(payload.AmountKobo, money.Currency(payload.Currency))
	if err != nil {
		return fmt.Errorf("ledger: invalid event money units: %w", err)
	}

	_, err = lc.svc.PostTransaction(ctx, service.PostRequest{
		IdempotencyKey: payload.IdempotencyKey,
		Description:    "payment:" + payload.PaymentID,
		Entries: []service.Entry{
			{AccountID: payload.SenderID, Type: service.EntryDebit, Amount: amount},
			{AccountID: payload.ReceiverID, Type: service.EntryCredit, Amount: amount},
		},
	})
	if err != nil {
		return fmt.Errorf("ledger: post transaction for payment %s: %w", payload.PaymentID, err)
	}

	return nil
}

func (lc *LedgerConsumer) onPaymentReversed(ctx context.Context, msg rabbitmq.Message) error {
	var payload rabbitmq.PaymentReversedPayload
	if err := remarshal(msg.Raw.Payload, &payload); err != nil {
		return fmt.Errorf("ledger: decode PaymentReversedPayload: %w", err)
	}

	lc.log.Info("ledger: posting reversal double-entry",
		zap.String("payment_id", payload.PaymentID),
		zap.String("sender_id", payload.SenderID),
		zap.String("receiver_id", payload.ReceiverID),
		zap.Int64("amount_kobo", payload.AmountKobo),
		zap.String("currency", payload.Currency),
	)

	amount, err := money.New(payload.AmountKobo, money.Currency(payload.Currency))
	if err != nil {
		return fmt.Errorf("ledger: invalid event money units: %w", err)
	}

	// For a reversal, we swap the DEBIT and CREDIT roles.
	// The original receiver is debited, and the original sender is credited.
	_, err = lc.svc.PostTransaction(ctx, service.PostRequest{
		IdempotencyKey: "reversal:" + payload.PaymentID,
		Description:    "reversal:" + payload.PaymentID,
		Entries: []service.Entry{
			{AccountID: payload.ReceiverID, Type: service.EntryDebit, Amount: amount},
			{AccountID: payload.SenderID, Type: service.EntryCredit, Amount: amount},
		},
	})
	if err != nil {
		return fmt.Errorf("ledger: post transaction for reversal %s: %w", payload.PaymentID, err)
	}

	return nil
}

// remarshal round-trips v through JSON to decode the untyped any payload into
// the concrete target struct.
func remarshal(src any, dst any) error {
	b, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dst)
}
