package webhook

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/core"
	"github.com/newthinker/atlas/internal/notifier"
)

func TestWebhook_ImplementsNotifier(t *testing.T) {
	var _ notifier.Notifier = (*Webhook)(nil)
}

func TestWebhook_Name(t *testing.T) {
	w := New("http://example.com/hook", nil)
	if w.Name() != "webhook" {
		t.Errorf("expected 'webhook', got %s", w.Name())
	}
}

func TestWebhook_Init_RequiresURL(t *testing.T) {
	w := &Webhook{}
	err := w.Init(notifier.Config{Params: map[string]any{}})
	if err == nil {
		t.Error("expected error for missing URL")
	}
}

func TestWebhook_Init_WithURL(t *testing.T) {
	w := &Webhook{}
	err := w.Init(notifier.Config{
		Params: map[string]any{
			"url": "http://example.com/hook",
		},
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if w.url != "http://example.com/hook" {
		t.Errorf("expected url, got %s", w.url)
	}
}

func TestWebhook_Send(t *testing.T) {
	var receivedPayload map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedPayload)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	w := New(server.URL, nil)

	signal := core.Signal{
		Symbol:      "AAPL",
		Action:      core.ActionBuy,
		Confidence:  0.85,
		Strategy:    "pe_band",
		GeneratedAt: time.Now(),
	}

	err := w.Send(signal)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedPayload["symbol"] != "AAPL" {
		t.Errorf("expected symbol AAPL, got %v", receivedPayload["symbol"])
	}
	if receivedPayload["action"] != "buy" {
		t.Errorf("expected action buy, got %v", receivedPayload["action"])
	}
}

func TestWebhook_SendBatch(t *testing.T) {
	var receivedPayload map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedPayload)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	w := New(server.URL, nil)

	signals := []core.Signal{
		{Symbol: "AAPL", Action: core.ActionBuy, GeneratedAt: time.Now()},
		{Symbol: "GOOG", Action: core.ActionSell, GeneratedAt: time.Now()},
	}

	err := w.SendBatch(signals)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedPayload["type"] != "batch" {
		t.Errorf("expected type batch, got %v", receivedPayload["type"])
	}
	if receivedPayload["count"].(float64) != 2 {
		t.Errorf("expected count 2, got %v", receivedPayload["count"])
	}
}

func TestWebhook_SendBatch_Empty(t *testing.T) {
	w := New("http://example.com/hook", nil)
	err := w.SendBatch([]core.Signal{})
	if err != nil {
		t.Errorf("empty batch should not error: %v", err)
	}
}

func TestWebhook_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	w := New(server.URL, nil)

	err := w.Send(core.Signal{Symbol: "TEST", GeneratedAt: time.Now()})
	if err == nil {
		t.Error("expected error for server error response")
	}
}

func TestWebhook_CustomHeaders(t *testing.T) {
	var receivedHeaders http.Header

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	headers := map[string]string{
		"Authorization": "Bearer test-token",
		"X-Custom":      "value",
	}
	w := New(server.URL, headers)

	w.Send(core.Signal{Symbol: "TEST", GeneratedAt: time.Now()})

	if receivedHeaders.Get("Authorization") != "Bearer test-token" {
		t.Error("expected Authorization header")
	}
	if receivedHeaders.Get("X-Custom") != "value" {
		t.Error("expected X-Custom header")
	}
}
