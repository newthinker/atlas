package logger

import (
	"testing"
)

func TestNew_Development(t *testing.T) {
	log, err := New(true)
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	if log == nil {
		t.Fatal("expected non-nil logger")
	}

	// Should not panic
	log.Info("test message")
}

func TestNew_Production(t *testing.T) {
	log, err := New(false)
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	if log == nil {
		t.Fatal("expected non-nil logger")
	}
}

func TestMust(t *testing.T) {
	// Should not panic
	log := Must(true)
	if log == nil {
		t.Fatal("expected non-nil logger")
	}
}
