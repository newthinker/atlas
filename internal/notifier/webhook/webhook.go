// Package webhook implements an HTTP webhook notifier
package webhook

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/newthinker/atlas/internal/core"
	"github.com/newthinker/atlas/internal/notifier"
)

// Webhook implements the Notifier interface for HTTP webhooks
type Webhook struct {
	url     string
	headers map[string]string
	client  *http.Client
}

// New creates a new Webhook notifier
func New(url string, headers map[string]string) *Webhook {
	return &Webhook{
		url:     url,
		headers: headers,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (w *Webhook) Name() string { return "webhook" }

func (w *Webhook) Init(cfg notifier.Config) error {
	if url, ok := cfg.Params["url"].(string); ok {
		w.url = url
	}
	if headers, ok := cfg.Params["headers"].(map[string]string); ok {
		w.headers = headers
	}

	if w.url == "" {
		return fmt.Errorf("webhook: url is required")
	}

	if w.client == nil {
		w.client = &http.Client{Timeout: 30 * time.Second}
	}

	return nil
}

func (w *Webhook) Send(signal core.Signal) error {
	payload := w.signalToPayload(signal)
	return w.post(payload)
}

func (w *Webhook) SendBatch(signals []core.Signal) error {
	if len(signals) == 0 {
		return nil
	}

	payloads := make([]map[string]any, len(signals))
	for i, sig := range signals {
		payloads[i] = w.signalToPayload(sig)
	}

	batchPayload := map[string]any{
		"type":    "batch",
		"count":   len(signals),
		"signals": payloads,
	}

	return w.post(batchPayload)
}

func (w *Webhook) signalToPayload(signal core.Signal) map[string]any {
	return map[string]any{
		"type":         "signal",
		"symbol":       signal.Symbol,
		"action":       signal.Action,
		"confidence":   signal.Confidence,
		"price":        signal.Price,
		"reason":       signal.Reason,
		"strategy":     signal.Strategy,
		"metadata":     signal.Metadata,
		"generated_at": signal.GeneratedAt.Format(time.RFC3339),
	}
}

func (w *Webhook) post(payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("webhook: failed to marshal payload: %w", err)
	}

	req, err := http.NewRequest("POST", w.url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("webhook: failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	for k, v := range w.headers {
		req.Header.Set(k, v)
	}

	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook: server returned %d", resp.StatusCode)
	}

	return nil
}
