package email

import (
	"strings"
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/core"
	"github.com/newthinker/atlas/internal/notifier"
)

func TestEmail_ImplementsNotifier(t *testing.T) {
	var _ notifier.Notifier = (*Email)(nil)
}

func TestEmail_Name(t *testing.T) {
	e := New("smtp.example.com", 587, "", "", "from@example.com", []string{"to@example.com"})
	if e.Name() != "email" {
		t.Errorf("expected 'email', got %s", e.Name())
	}
}

func TestEmail_Init_RequiredFields(t *testing.T) {
	e := &Email{}
	err := e.Init(notifier.Config{Params: map[string]any{}})
	if err == nil {
		t.Error("expected error for missing required fields")
	}
}

func TestEmail_Init_WithConfig(t *testing.T) {
	e := &Email{}
	err := e.Init(notifier.Config{
		Params: map[string]any{
			"host": "smtp.example.com",
			"port": 587,
			"from": "atlas@example.com",
			"to":   []string{"user@example.com"},
		},
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if e.host != "smtp.example.com" {
		t.Errorf("expected host smtp.example.com, got %s", e.host)
	}
}

func TestEmail_FormatSignal(t *testing.T) {
	e := New("smtp.example.com", 587, "", "", "from@example.com", []string{"to@example.com"})

	signal := core.Signal{
		Symbol:      "AAPL",
		Action:      core.ActionBuy,
		Confidence:  0.85,
		Strategy:    "pe_band",
		Reason:      "PE below threshold",
		GeneratedAt: time.Now(),
	}

	formatted := e.formatSignal(signal)

	if !strings.Contains(formatted, "AAPL") {
		t.Error("formatted message should contain symbol")
	}
	if !strings.Contains(formatted, "buy") {
		t.Error("formatted message should contain action")
	}
	if !strings.Contains(formatted, "85.0%") {
		t.Error("formatted message should contain confidence")
	}
}

func TestEmail_FormatSignalHTML_BuyColor(t *testing.T) {
	e := New("smtp.example.com", 587, "", "", "from@example.com", []string{"to@example.com"})

	signal := core.Signal{
		Symbol:      "AAPL",
		Action:      core.ActionBuy,
		Confidence:  0.85,
		Strategy:    "pe_band",
		GeneratedAt: time.Now(),
	}

	formatted := e.formatSignalHTML(signal)

	if !strings.Contains(formatted, "#28a745") {
		t.Error("buy signal should use green color")
	}
}

func TestEmail_FormatSignalHTML_SellColor(t *testing.T) {
	e := New("smtp.example.com", 587, "", "", "from@example.com", []string{"to@example.com"})

	signal := core.Signal{
		Symbol:      "AAPL",
		Action:      core.ActionSell,
		Confidence:  0.85,
		Strategy:    "pe_band",
		GeneratedAt: time.Now(),
	}

	formatted := e.formatSignalHTML(signal)

	if !strings.Contains(formatted, "#dc3545") {
		t.Error("sell signal should use red color")
	}
}

func TestEmail_SendBatch_Empty(t *testing.T) {
	e := New("smtp.example.com", 587, "", "", "from@example.com", []string{"to@example.com"})

	err := e.SendBatch([]core.Signal{})
	if err != nil {
		t.Errorf("empty batch should not error: %v", err)
	}
}
