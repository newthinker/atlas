// internal/core/errors_test.go
package core

import (
	"errors"
	"testing"
)

func TestError_Error(t *testing.T) {
	err := &Error{Code: "TEST_ERROR", Message: "test message"}
	if err.Error() != "[TEST_ERROR] test message" {
		t.Errorf("unexpected error string: %s", err.Error())
	}
}

func TestError_Unwrap(t *testing.T) {
	cause := errors.New("root cause")
	err := &Error{Code: "WRAP", Message: "wrapped", Cause: cause}
	if !errors.Is(err, cause) {
		t.Error("Unwrap should return cause")
	}
}

func TestError_Is(t *testing.T) {
	if !errors.Is(ErrSymbolNotFound, ErrSymbolNotFound) {
		t.Error("same error should match")
	}
}

func TestWrapError(t *testing.T) {
	cause := errors.New("original")
	wrapped := WrapError(ErrCollectorFailed, cause)
	if wrapped.Cause != cause {
		t.Error("cause not set")
	}
	if wrapped.Code != ErrCollectorFailed.Code {
		t.Error("code not preserved")
	}
}
