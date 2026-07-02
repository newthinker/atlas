package notifier

import (
	"errors"
	"testing"

	"github.com/newthinker/atlas/internal/core"
)

// Context Checkpoint: done_criteria → test mapping (TASK-202)
// functional[0] "底层实现 SendText → 直发文本 [SEVERITY] name: message" → TestAlertAdapter_DirectPath_WhenSendTextSupported
// functional[1] "底层不实现 SendText → 回退 SYSTEM/alert 信号走 Send"    → TestAlertAdapter_FallbackWrapsSystemSignal
// boundary[0]   "Send/SendText 返回错误如实上传，不吞错"                → TestAlertAdapter_DirectPath_PropagatesError / _FallbackPropagatesError

// alertNotifier mirrors internal/alert.Notifier so this test asserts the
// adapter satisfies that shape without importing internal/alert (AD-12).
type alertNotifier interface {
	Name() string
	Notify(msg string) error
}

var _ alertNotifier = (*AlertAdapter)(nil)

// fakeNotifier is a notifier.Notifier without SendText (fallback path).
type fakeNotifier struct {
	name    string
	sent    []core.Signal
	sendErr error
}

func (f *fakeNotifier) Name() string                  { return f.name }
func (f *fakeNotifier) Init(cfg Config) error         { return nil }
func (f *fakeNotifier) SendBatch([]core.Signal) error { return nil }
func (f *fakeNotifier) Send(s core.Signal) error {
	f.sent = append(f.sent, s)
	return f.sendErr
}

// fakeTextNotifier additionally implements SendText (direct path).
type fakeTextNotifier struct {
	fakeNotifier
	texts   []string
	textErr error
}

func (f *fakeTextNotifier) SendText(text string) error {
	f.texts = append(f.texts, text)
	return f.textErr
}

func TestAlertAdapter_DirectPath_WhenSendTextSupported(t *testing.T) {
	inner := &fakeTextNotifier{fakeNotifier: fakeNotifier{name: "telegram"}}
	a := NewAlertAdapter(inner)

	const msg = "[CRITICAL] cpu_high: cpu usage > 90%"
	if err := a.Notify(msg); err != nil {
		t.Fatalf("Notify returned error: %v", err)
	}

	if len(inner.texts) != 1 || inner.texts[0] != msg {
		t.Errorf("expected direct SendText(%q), got texts=%v", msg, inner.texts)
	}
	if len(inner.sent) != 0 {
		t.Errorf("Send must not be called on direct path, got %v", inner.sent)
	}
}

func TestAlertAdapter_FallbackWrapsSystemSignal(t *testing.T) {
	inner := &fakeNotifier{name: "email"}
	a := NewAlertAdapter(inner)

	const msg = "[WARNING] error_rate: errors elevated"
	if err := a.Notify(msg); err != nil {
		t.Fatalf("Notify returned error: %v", err)
	}

	if len(inner.sent) != 1 {
		t.Fatalf("expected one Send on fallback path, got %d", len(inner.sent))
	}
	got := inner.sent[0]
	if got.Symbol != "SYSTEM" || got.Strategy != "alert" || got.Reason != msg {
		t.Errorf("fallback signal = %+v, want {Symbol:SYSTEM Strategy:alert Reason:%q}", got, msg)
	}
}

func TestAlertAdapter_DirectPath_PropagatesError(t *testing.T) {
	wantErr := errors.New("telegram down")
	inner := &fakeTextNotifier{
		fakeNotifier: fakeNotifier{name: "telegram"},
		textErr:      wantErr,
	}
	a := NewAlertAdapter(inner)

	if err := a.Notify("[CRITICAL] x: y"); !errors.Is(err, wantErr) {
		t.Errorf("Notify error = %v, want %v (must not swallow)", err, wantErr)
	}
}

func TestAlertAdapter_FallbackPropagatesError(t *testing.T) {
	wantErr := errors.New("smtp refused")
	inner := &fakeNotifier{name: "email", sendErr: wantErr}
	a := NewAlertAdapter(inner)

	if err := a.Notify("[WARNING] x: y"); !errors.Is(err, wantErr) {
		t.Errorf("Notify error = %v, want %v (must not swallow)", err, wantErr)
	}
}

func TestAlertAdapter_Name_Delegates(t *testing.T) {
	a := NewAlertAdapter(&fakeNotifier{name: "webhook"})
	if a.Name() != "webhook" {
		t.Errorf("Name() = %q, want webhook", a.Name())
	}
}
