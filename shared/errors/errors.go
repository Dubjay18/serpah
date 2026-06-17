package errors

import "errors"

type Code string

const (
	CodeNotFound          Code = "NOT_FOUND"
	CodeConflict          Code = "CONFLICT"
	CodeUnauthorized      Code = "UNAUTHORIZED"
	CodeInsufficientFunds Code = "INSUFFICIENT_FUNDS"
	CodeInvalidInput      Code = "INVALID_INPUT"
	CodeInternal          Code = "INTERNAL"
)

type AppError struct {
	Code    Code
	Message string
	Cause   error
}

func (e *AppError) Error() string { return string(e.Code) + ": " + e.Message }
func (e *AppError) Unwrap() error { return e.Cause }

func New(code Code, msg string) *AppError {
	return &AppError{Code: code, Message: msg}
}

func Wrap(code Code, msg string, cause error) *AppError {
	return &AppError{Code: code, Message: msg, Cause: cause}
}

func IsCode(err error, code Code) bool {
	var e *AppError
	if errors.As(err, &e) {
		return e.Code == code
	}
	return false
}
