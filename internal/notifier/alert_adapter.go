package notifier

import (
	"github.com/newthinker/atlas/internal/core"
)

// textSender is the optional fast path: a notifier that can deliver a plain,
// pre-formatted alert string directly (e.g. telegram), bypassing the Signal
// envelope.
type textSender interface {
	SendText(string) error
}

// AlertAdapter adapts a notifier.Notifier so it can receive alert messages. It
// structurally satisfies the internal/alert.Notifier interface
// (Name() string; Notify(msg string) error) without importing internal/alert,
// keeping the dependency one-way: internal/alert must not import
// internal/notifier (AD-12).
type AlertAdapter struct {
	inner Notifier
}

// NewAlertAdapter wraps a notifier.Notifier as an alert sink.
func NewAlertAdapter(inner Notifier) *AlertAdapter {
	return &AlertAdapter{inner: inner}
}

// Name returns the wrapped notifier's name.
func (a *AlertAdapter) Name() string {
	return a.inner.Name()
}

// Notify delivers a pre-formatted alert message ("[SEVERITY] name: message").
// If the wrapped notifier supports plain text (SendText) it is sent directly;
// otherwise the text is wrapped as a SYSTEM alert signal and sent via Send.
// Either underlying error is returned as-is.
func (a *AlertAdapter) Notify(msg string) error {
	if ts, ok := a.inner.(textSender); ok {
		return ts.SendText(msg)
	}
	return a.inner.Send(core.Signal{
		Symbol:   "SYSTEM",
		Strategy: "alert",
		Reason:   msg,
	})
}
