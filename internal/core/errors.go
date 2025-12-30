// internal/core/errors.go
package core

import "fmt"

// Error represents a structured error with code and optional cause.
type Error struct {
	Code    string
	Message string
	Cause   error
}

// Error implements the error interface.
func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap returns the underlying cause for errors.Is/As support.
func (e *Error) Unwrap() error {
	return e.Cause
}

// Is implements errors.Is matching by code.
func (e *Error) Is(target error) bool {
	if t, ok := target.(*Error); ok {
		return e.Code == t.Code
	}
	return false
}

// WrapError creates a new error with the same code but with a cause.
func WrapError(base *Error, cause error) *Error {
	return &Error{
		Code:    base.Code,
		Message: base.Message,
		Cause:   cause,
	}
}

// Predefined errors
var (
	// Data errors
	ErrSymbolNotFound = &Error{Code: "SYMBOL_NOT_FOUND", Message: "symbol not found"}
	ErrNoData         = &Error{Code: "NO_DATA", Message: "no data available"}

	// Collector errors
	ErrCollectorFailed  = &Error{Code: "COLLECTOR_FAILED", Message: "collector failed"}
	ErrCollectorTimeout = &Error{Code: "COLLECTOR_TIMEOUT", Message: "collector timeout"}

	// Strategy errors
	ErrStrategyFailed   = &Error{Code: "STRATEGY_FAILED", Message: "strategy analysis failed"}
	ErrInsufficientData = &Error{Code: "INSUFFICIENT_DATA", Message: "insufficient data for analysis"}

	// Notifier errors
	ErrNotifierFailed = &Error{Code: "NOTIFIER_FAILED", Message: "notifier failed"}

	// Broker errors
	ErrBrokerDisconnected = &Error{Code: "BROKER_DISCONNECTED", Message: "broker not connected"}
	ErrOrderFailed        = &Error{Code: "ORDER_FAILED", Message: "order failed"}

	// Config errors
	ErrConfigInvalid  = &Error{Code: "CONFIG_INVALID", Message: "configuration invalid"}
	ErrConfigMissing  = &Error{Code: "CONFIG_MISSING", Message: "required configuration missing"}

	// LLM errors
	ErrLLMFailed  = &Error{Code: "LLM_FAILED", Message: "LLM request failed"}
	ErrLLMTimeout = &Error{Code: "LLM_TIMEOUT", Message: "LLM request timeout"}
)
