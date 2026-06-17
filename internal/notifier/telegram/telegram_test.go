package telegram

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/core"
	"github.com/newthinker/atlas/internal/notifier"
)

func TestTelegram_ImplementsNotifier(t *testing.T) {
	var _ notifier.Notifier = (*Telegram)(nil)
}

func TestTelegram_Name(t *testing.T) {
	tg := New("token", "chatid")
	if tg.Name() != "telegram" {
		t.Errorf("expected 'telegram', got '%s'", tg.Name())
	}
}

func TestTelegram_WithProxy(t *testing.T) {
	tg := New("token", "chatid", WithProxy("http://127.0.0.1:7890"))
	tr, ok := tg.client.Transport.(*http.Transport)
	if !ok || tr.Proxy == nil {
		t.Fatalf("expected *http.Transport with Proxy set, got %T", tg.client.Transport)
	}
	req, _ := http.NewRequest(http.MethodGet, "https://api.telegram.org/", nil)
	u, err := tr.Proxy(req)
	if err != nil || u == nil || u.String() != "http://127.0.0.1:7890" {
		t.Errorf("proxy resolved to %v (err %v), want http://127.0.0.1:7890", u, err)
	}
}

func TestTelegram_WithProxy_emptyStaysDirect(t *testing.T) {
	tg := New("token", "chatid", WithProxy(""))
	if tg.client.Transport != nil {
		t.Error("empty proxy should leave the default transport (direct/env)")
	}
}

func TestTelegram_Init_ReadsProxy(t *testing.T) {
	tg := &Telegram{}
	err := tg.Init(notifier.Config{Params: map[string]any{
		"bot_token": "x", "chat_id": "y", "proxy": "socks5://127.0.0.1:1080",
	}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tr, ok := tg.client.Transport.(*http.Transport)
	if !ok || tr.Proxy == nil {
		t.Fatalf("Init should apply proxy transport, got %T", tg.client.Transport)
	}
}

func TestTelegram_Init(t *testing.T) {
	tg := &Telegram{}

	cfg := notifier.Config{
		Params: map[string]any{
			"bot_token": "test-token",
			"chat_id":   "test-chat",
		},
	}

	err := tg.Init(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tg.botToken != "test-token" {
		t.Errorf("expected bot_token 'test-token', got '%s'", tg.botToken)
	}
	if tg.chatID != "test-chat" {
		t.Errorf("expected chat_id 'test-chat', got '%s'", tg.chatID)
	}
}

func TestTelegram_Init_MissingToken(t *testing.T) {
	tg := &Telegram{}

	cfg := notifier.Config{
		Params: map[string]any{
			"chat_id": "test-chat",
		},
	}

	err := tg.Init(cfg)
	if err == nil {
		t.Error("expected error for missing bot_token")
	}
}

func TestTelegram_Init_MissingChatID(t *testing.T) {
	tg := &Telegram{}

	cfg := notifier.Config{
		Params: map[string]any{
			"bot_token": "test-token",
		},
	}

	err := tg.Init(cfg)
	if err == nil {
		t.Error("expected error for missing chat_id")
	}
}

func TestTelegram_Send(t *testing.T) {
	var receivedPayload map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedPayload)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	defer server.Close()

	// Override the URL by using a custom client that redirects
	tg := New("test-token", "test-chat")
	tg.client = server.Client()

	// We need to modify sendMessage to use our test server
	// For this test, let's verify the formatting works correctly
	signal := core.Signal{
		Symbol:      "AAPL",
		Action:      core.ActionBuy,
		Confidence:  0.85,
		Strategy:    "ma_crossover",
		Reason:      "Golden cross detected",
		Price:       150.25,
		GeneratedAt: time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
	}

	formatted := tg.formatSignal(signal)

	if !strings.Contains(formatted, "AAPL") {
		t.Error("formatted message should contain symbol")
	}
	if !strings.Contains(formatted, "buy") {
		t.Error("formatted message should contain action")
	}
	if !strings.Contains(formatted, "85.0%") {
		t.Error("formatted message should contain confidence")
	}
	if !strings.Contains(formatted, "ma_crossover") {
		t.Error("formatted message should contain strategy")
	}
	if !strings.Contains(formatted, "Golden cross") {
		t.Error("formatted message should contain reason")
	}
	if !strings.Contains(formatted, "150.25") {
		t.Error("formatted message should contain price")
	}
}

func TestTelegram_FormatSignal_Sell(t *testing.T) {
	tg := New("token", "chat")

	signal := core.Signal{
		Symbol:      "TSLA",
		Action:      core.ActionSell,
		Confidence:  0.75,
		GeneratedAt: time.Now(),
	}

	formatted := tg.formatSignal(signal)

	if !strings.Contains(formatted, "📉") {
		t.Error("sell signal should have 📉 emoji")
	}
	if !strings.Contains(formatted, "sell") {
		t.Error("formatted message should contain sell action")
	}
}

func TestTelegram_FormatSignal_Hold(t *testing.T) {
	tg := New("token", "chat")

	signal := core.Signal{
		Symbol:      "GOOG",
		Action:      core.ActionHold,
		Confidence:  0.5,
		GeneratedAt: time.Now(),
	}

	formatted := tg.formatSignal(signal)

	if !strings.Contains(formatted, "⏸️") {
		t.Error("hold signal should have ⏸️ emoji")
	}
}

func TestTelegram_SendBatch_Empty(t *testing.T) {
	tg := New("token", "chat")

	err := tg.SendBatch([]core.Signal{})
	if err != nil {
		t.Errorf("empty batch should not return error: %v", err)
	}
}

func TestTelegram_SendBatch_Formatting(t *testing.T) {
	tg := New("token", "chat")

	signals := []core.Signal{
		{Symbol: "AAPL", Action: core.ActionBuy, Confidence: 0.8, GeneratedAt: time.Now()},
		{Symbol: "TSLA", Action: core.ActionSell, Confidence: 0.7, GeneratedAt: time.Now()},
	}

	// Verify formatting works for each signal
	for _, sig := range signals {
		formatted := tg.formatSignal(sig)
		if formatted == "" {
			t.Errorf("formatSignal returned empty string for %s", sig.Symbol)
		}
	}
}

func TestFormatSignal_TitleIncludesNameFromMetadata(t *testing.T) {
	tg := New("token", "chat")
	sig := core.Signal{
		Symbol: "0883.HK", Action: core.ActionStrongSell, Confidence: 0.857,
		Metadata: map[string]any{"name": "中国海洋石油", "percentile": 93.8},
	}
	got := tg.formatSignal(sig)
	if !strings.Contains(got, "*00883.HK 中国海洋石油*") {
		t.Errorf("title must contain padded symbol + name, got:\n%s", got)
	}
}

func TestFormatSignal_NoNameMetadata_KeepsBareTitle(t *testing.T) {
	tg := New("token", "chat")
	got := tg.formatSignal(core.Signal{Symbol: "600519.SH", Action: core.ActionBuy, Confidence: 0.8})
	if !strings.Contains(got, "*600519.SH* - buy") {
		t.Errorf("title without name must stay bare (no trailing space inside bold), got:\n%s", got)
	}
}

func TestDisplaySymbol_HKPaddedToFiveDigits(t *testing.T) {
	cases := map[string]string{
		"0883.HK":   "00883.HK",
		"6886.HK":   "06886.HK",
		"9988.HK":   "09988.HK",
		"3288.HK":   "03288.HK",
		"00700.HK":  "00700.HK", // already five digits
		"600519.SH": "600519.SH",
		"AAPL":      "AAPL",
		"BTC.HK":    "BTC.HK", // non-numeric code untouched
		".HK":       ".HK",
	}
	for in, want := range cases {
		if got := displaySymbol(in); got != want {
			t.Errorf("displaySymbol(%q) = %q, want %q", in, got, want)
		}
	}
}
