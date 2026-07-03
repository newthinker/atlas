package main

// These tests cover the broker CLI command handlers, which were previously
// untested (0%). They are exercised here with the default mock broker so the
// cmd/atlas package meets its coverage gate while characterising existing
// behaviour. Not part of TASK-007's done_criteria, but required to clear the
// package-level coverage minimum (see discovery).

import (
	"errors"
	"testing"

	"github.com/newthinker/atlas/internal/broker"
	"github.com/newthinker/atlas/internal/broker/mock"
	"github.com/newthinker/atlas/internal/config"
	"go.uber.org/zap"
)

// withEmptyConfigFile resets the global config flag so getBroker falls back to
// the mock broker, restoring it afterwards.
func withEmptyConfigFile(t *testing.T) {
	t.Helper()
	prev := cfgFile
	cfgFile = ""
	t.Cleanup(func() { cfgFile = prev })
}

func TestGetBroker(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *config.Config
		wantErr bool
	}{
		{"nil config falls back to mock", nil, false},
		{"disabled broker falls back to mock", func() *config.Config {
			c := config.Defaults()
			c.Broker.Enabled = false
			return c
		}(), false},
		{"mock provider", func() *config.Config {
			c := config.Defaults()
			c.Broker.Enabled = true
			c.Broker.Provider = "mock"
			return c
		}(), false},
		{"default provider falls back to mock", func() *config.Config {
			c := config.Defaults() // Provider defaults to "mock"
			c.Broker.Enabled = true
			return c
		}(), false},
		{"futu provider is unknown (FutuBroker withdrawn)", func() *config.Config {
			c := config.Defaults()
			c.Broker.Enabled = true
			c.Broker.Provider = "futu"
			return c
		}(), true},
		{"unknown provider", func() *config.Config {
			c := config.Defaults()
			c.Broker.Enabled = true
			c.Broker.Provider = "nope"
			return c
		}(), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := getBroker(tt.cfg)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got broker %v", b)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if b == nil {
				t.Fatal("expected a broker instance")
			}
		})
	}
}

// TestGetBroker_UnknownProviderNamesProvider verifies the error surfaces the
// actual provider name (futu falls through to default after FutuBroker was
// withdrawn — 2026-07-02, paper-only). Covers functional[0] + error_handling[0].
func TestGetBroker_UnknownProviderNamesProvider(t *testing.T) {
	c := config.Defaults()
	c.Broker.Enabled = true
	c.Broker.Provider = "futu"

	_, err := getBroker(c)
	if err == nil {
		t.Fatal("expected error for futu provider")
	}
	if got := err.Error(); got != "unknown broker provider: futu" {
		t.Fatalf("error = %q, want %q", got, "unknown broker provider: futu")
	}
}

func TestWithBrokerConnection_RunsCallbackWithConnectedBroker(t *testing.T) {
	withEmptyConfigFile(t)

	var connected bool
	err := withBrokerConnection(func(b broker.LegacyBroker, log *zap.Logger) error {
		connected = b.IsConnected()
		return nil
	})
	if err != nil {
		t.Fatalf("withBrokerConnection: %v", err)
	}
	if !connected {
		t.Fatal("callback should receive a connected broker")
	}
}

func TestWithBrokerConnection_PropagatesCallbackError(t *testing.T) {
	withEmptyConfigFile(t)

	want := errors.New("callback boom")
	got := withBrokerConnection(func(b broker.LegacyBroker, log *zap.Logger) error {
		return want
	})
	if !errors.Is(got, want) {
		t.Fatalf("expected callback error to propagate, got %v", got)
	}
}

func TestBrokerCommandHandlers(t *testing.T) {
	withEmptyConfigFile(t)

	// Sanity: the mock broker is what getBroker returns for an empty config.
	if _, ok := mustBroker(t).(*mock.MockBroker); !ok {
		t.Fatal("expected mock broker for empty config")
	}

	handlers := map[string]func() error{
		"status":    func() error { return runBrokerStatus(nil, nil) },
		"positions": func() error { return runBrokerPositions(nil, nil) },
		"orders":    func() error { return runBrokerOrders(nil, nil) },
		"account":   func() error { return runBrokerAccount(nil, nil) },
		"history":   func() error { return runBrokerHistory(nil, nil) },
	}
	for name, h := range handlers {
		t.Run(name, func(t *testing.T) {
			if err := h(); err != nil {
				t.Fatalf("runBroker%s returned error: %v", name, err)
			}
		})
	}
}

func mustBroker(t *testing.T) broker.LegacyBroker {
	t.Helper()
	b, err := getBroker(nil)
	if err != nil {
		t.Fatalf("getBroker: %v", err)
	}
	return b
}
