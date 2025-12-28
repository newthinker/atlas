package notifier

import (
	"errors"
	"testing"

	"github.com/newthinker/atlas/internal/core"
)

type mockNotifier struct {
	name       string
	sendCalled int
	batchCalls int
	shouldFail bool
}

func (m *mockNotifier) Name() string { return m.name }

func (m *mockNotifier) Init(cfg Config) error { return nil }

func (m *mockNotifier) Send(signal core.Signal) error {
	m.sendCalled++
	if m.shouldFail {
		return errors.New("send failed")
	}
	return nil
}

func (m *mockNotifier) SendBatch(signals []core.Signal) error {
	m.batchCalls++
	if m.shouldFail {
		return errors.New("batch send failed")
	}
	return nil
}

func TestRegistry_Register(t *testing.T) {
	r := NewRegistry()

	mock := &mockNotifier{name: "test"}
	err := r.Register(mock)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Duplicate registration should fail
	err = r.Register(mock)
	if err == nil {
		t.Error("expected error for duplicate registration")
	}
}

func TestRegistry_Get(t *testing.T) {
	r := NewRegistry()

	mock := &mockNotifier{name: "test"}
	r.Register(mock)

	n, err := r.Get("test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n.Name() != "test" {
		t.Errorf("expected 'test', got '%s'", n.Name())
	}

	// Non-existent notifier
	_, err = r.Get("nonexistent")
	if err == nil {
		t.Error("expected error for non-existent notifier")
	}
}

func TestRegistry_GetAll(t *testing.T) {
	r := NewRegistry()

	r.Register(&mockNotifier{name: "a"})
	r.Register(&mockNotifier{name: "b"})

	all := r.GetAll()
	if len(all) != 2 {
		t.Errorf("expected 2 notifiers, got %d", len(all))
	}
}

func TestRegistry_NotifyAll(t *testing.T) {
	r := NewRegistry()

	mock1 := &mockNotifier{name: "n1"}
	mock2 := &mockNotifier{name: "n2"}
	r.Register(mock1)
	r.Register(mock2)

	signal := core.Signal{Symbol: "TEST", Action: core.ActionBuy}
	errs := r.NotifyAll(signal)

	if len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}

	if mock1.sendCalled != 1 {
		t.Errorf("expected mock1.sendCalled = 1, got %d", mock1.sendCalled)
	}
	if mock2.sendCalled != 1 {
		t.Errorf("expected mock2.sendCalled = 1, got %d", mock2.sendCalled)
	}
}

func TestRegistry_NotifyAll_WithFailure(t *testing.T) {
	r := NewRegistry()

	mock1 := &mockNotifier{name: "n1"}
	mock2 := &mockNotifier{name: "n2", shouldFail: true}
	r.Register(mock1)
	r.Register(mock2)

	signal := core.Signal{Symbol: "TEST", Action: core.ActionBuy}
	errs := r.NotifyAll(signal)

	if len(errs) != 1 {
		t.Errorf("expected 1 error, got %d", len(errs))
	}
	if _, ok := errs["n2"]; !ok {
		t.Error("expected error from n2")
	}
}

func TestRegistry_NotifyAllBatch(t *testing.T) {
	r := NewRegistry()

	mock := &mockNotifier{name: "batch"}
	r.Register(mock)

	signals := []core.Signal{
		{Symbol: "A", Action: core.ActionBuy},
		{Symbol: "B", Action: core.ActionSell},
	}
	errs := r.NotifyAllBatch(signals)

	if len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}
	if mock.batchCalls != 1 {
		t.Errorf("expected batchCalls = 1, got %d", mock.batchCalls)
	}
}
