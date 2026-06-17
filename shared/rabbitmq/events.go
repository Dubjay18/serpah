// Package rabbitmq provides shared RabbitMQ primitives for the Seraph platform.
package rabbitmq

import "time"

// ─── Routing keys ────────────────────────────────────────────────────────────

const (
	// Auth events
	EventUserRegistered  = "auth.user.registered"
	EventUserDeactivated = "auth.user.deactivated"

	// Payment events
	EventPaymentInitiated = "payments.payment.initiated"
	EventPaymentCompleted = "payments.payment.completed"
	EventPaymentFailed    = "payments.payment.failed"
	EventPaymentReversed  = "payments.payment.reversed"

	// Account events
	EventAccountClosed = "accounts.account.closed"
)

// ─── Envelope ────────────────────────────────────────────────────────────────

// Event is the canonical message envelope published to every exchange.
// The Payload field is typed per routing key (see below).
type Event struct {
	// ID is a UUID v4 that uniquely identifies this event instance.
	ID string `json:"id"`
	// Type mirrors the routing key (e.g. "auth.user.registered").
	Type string `json:"type"`
	// Version allows schema evolution without breaking consumers.
	Version int `json:"version"`
	// OccurredAt is the UTC time the event was raised by the publisher.
	OccurredAt time.Time `json:"occurred_at"`
	// Payload is the event-specific body.
	Payload any `json:"payload"`
}

// ─── Auth payloads ────────────────────────────────────────────────────────────

// UserRegisteredPayload is emitted by the auth service when a new user account
// is created (both password-based and OAuth flows).
type UserRegisteredPayload struct {
	UserID    string `json:"user_id"`
	Email     string `json:"email"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

// UserDeactivatedPayload is emitted when a user account is deactivated.
type UserDeactivatedPayload struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
}

// ─── Payment payloads ────────────────────────────────────────────────────────

// PaymentInitiatedPayload is emitted when a payment moves to INITIATED.
type PaymentInitiatedPayload struct {
	PaymentID  string `json:"payment_id"`
	SenderID   string `json:"sender_id"`
	ReceiverID string `json:"receiver_id"`
	AmountKobo int64  `json:"amount_kobo"`
	// Currency is the ISO-4217 3-letter currency code (e.g., NGN, USD, GBP, EUR).
	Currency       string `json:"currency"`
	IdempotencyKey string `json:"idempotency_key"`
}

// PaymentCompletedPayload is emitted when a payment reaches COMPLETED.
type PaymentCompletedPayload struct {
	PaymentID  string `json:"payment_id"`
	SenderID   string `json:"sender_id"`
	ReceiverID string `json:"receiver_id"`
	AmountKobo int64  `json:"amount_kobo"`
	// Currency is the ISO-4217 3-letter currency code (e.g., NGN, USD, GBP, EUR).
	Currency   string `json:"currency"`
}

// PaymentFailedPayload is emitted when a payment reaches FAILED.
type PaymentFailedPayload struct {
	PaymentID string `json:"payment_id"`
	SenderID  string `json:"sender_id"`
	Reason    string `json:"reason"`
}

// PaymentReversedPayload is emitted when a completed payment is REVERSED.
type PaymentReversedPayload struct {
	PaymentID  string `json:"payment_id"`
	SenderID   string `json:"sender_id"`
	ReceiverID string `json:"receiver_id"`
	AmountKobo int64  `json:"amount_kobo"`
	// Currency is the ISO-4217 3-letter currency code (e.g., NGN, USD, GBP, EUR).
	Currency   string `json:"currency"`
}

// ─── Account payloads ─────────────────────────────────────────────────────────

// AccountClosedPayload is emitted when an account's status is changed to CLOSED.
type AccountClosedPayload struct {
	AccountID string `json:"account_id"`
	OwnerID   string `json:"owner_id"`
}
