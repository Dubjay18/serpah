package service

import "fmt"

type PaymentStatus string

const (
	StatusInitiated  PaymentStatus = "INITIATED"
	StatusProcessing PaymentStatus = "PROCESSING"
	StatusCompleted  PaymentStatus = "COMPLETED"
	StatusFailed     PaymentStatus = "FAILED"
	StatusReversed   PaymentStatus = "REVERSED"
)

// validTransitions defines all legal state moves.
var validTransitions = map[PaymentStatus][]PaymentStatus{
	StatusInitiated:  {StatusProcessing},
	StatusProcessing: {StatusCompleted, StatusFailed},
	StatusCompleted:  {StatusReversed},
	StatusFailed:     {},
	StatusReversed:   {},
}

// validateTransition checks whether moving from current to next is permitted.
func validateTransition(current, next PaymentStatus) error {
	for _, allowed := range validTransitions[current] {
		if allowed == next {
			return nil
		}
	}
	return fmt.Errorf("invalid transition: %s → %s", current, next)
}
