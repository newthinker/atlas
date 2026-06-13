package telegram

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/newthinker/atlas/internal/core"
	"github.com/newthinker/atlas/internal/notifier"
)

// Telegram implements the Notifier interface for Telegram Bot API
type Telegram struct {
	botToken string
	chatID   string
	client   *http.Client
}

// New creates a new Telegram notifier
func New(botToken, chatID string) *Telegram {
	return &Telegram{
		botToken: botToken,
		chatID:   chatID,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (t *Telegram) Name() string {
	return "telegram"
}

func (t *Telegram) Init(cfg notifier.Config) error {
	if token, ok := cfg.Params["bot_token"].(string); ok {
		t.botToken = token
	}
	if chatID, ok := cfg.Params["chat_id"].(string); ok {
		t.chatID = chatID
	}

	if t.botToken == "" {
		return fmt.Errorf("telegram: bot_token is required")
	}
	if t.chatID == "" {
		return fmt.Errorf("telegram: chat_id is required")
	}

	return nil
}

func (t *Telegram) Send(signal core.Signal) error {
	message := t.formatSignal(signal)
	return t.sendMessage(message)
}

func (t *Telegram) SendBatch(signals []core.Signal) error {
	if len(signals) == 0 {
		return nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📊 *%d Trading Signals*\n\n", len(signals)))

	for i, signal := range signals {
		sb.WriteString(t.formatSignal(signal))
		if i < len(signals)-1 {
			sb.WriteString("\n---\n\n")
		}
	}

	return t.sendMessage(sb.String())
}

// escapeMarkdown escapes special characters for Telegram Markdown
// Only escapes characters that are not part of our intentional formatting
func escapeMarkdown(text string) string {
	// Replace underscores that are not part of our *bold* markers
	// We use * for bold, so underscores in content should be escaped
	result := strings.ReplaceAll(text, "_", "\\_")
	return result
}

// displaySymbol renders a symbol for humans. HKEX codes are officially five
// digits, so shorter all-digit .HK prefixes are left-padded with zeros for
// display only — data-layer symbols stay exactly as configured.
func displaySymbol(symbol string) string {
	code, found := strings.CutSuffix(symbol, ".HK")
	if !found || code == "" || len(code) >= 5 {
		return symbol
	}
	for _, r := range code {
		if r < '0' || r > '9' {
			return symbol
		}
	}
	return strings.Repeat("0", 5-len(code)) + code + ".HK"
}

func (t *Telegram) formatSignal(signal core.Signal) string {
	var sb strings.Builder

	// Action emoji
	actionEmoji := "📈"
	switch signal.Action {
	case core.ActionSell:
		actionEmoji = "📉"
	case core.ActionHold:
		actionEmoji = "⏸️"
	}

	title := displaySymbol(signal.Symbol)
	if name, ok := signal.Metadata["name"].(string); ok && name != "" {
		title += " " + name
	}
	sb.WriteString(fmt.Sprintf("%s *%s* - %s\n", actionEmoji, title, signal.Action))
	sb.WriteString(fmt.Sprintf("📊 Confidence: %.1f%%\n", signal.Confidence*100))

	if signal.Strategy != "" {
		sb.WriteString(fmt.Sprintf("🎯 Strategy: %s\n", signal.Strategy))
	}

	if signal.Reason != "" {
		sb.WriteString(fmt.Sprintf("💡 Reason: %s\n", signal.Reason))
	}

	if signal.Price > 0 {
		sb.WriteString(fmt.Sprintf("💰 Price: $%.2f\n", signal.Price))
	}

	sb.WriteString(fmt.Sprintf("⏰ Time: %s", signal.GeneratedAt.Format("2006-01-02 15:04:05")))

	return sb.String()
}

func (t *Telegram) sendMessage(text string) error {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.botToken)

	// Escape special characters for Markdown
	text = escapeMarkdown(text)

	payload := map[string]any{
		"chat_id":    t.chatID,
		"text":       text,
		"parse_mode": "Markdown",
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("telegram: failed to marshal payload: %w", err)
	}

	resp, err := t.client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("telegram: failed to send message: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var result map[string]any
		json.NewDecoder(resp.Body).Decode(&result)
		return fmt.Errorf("telegram: API error (status %d): %v", resp.StatusCode, result)
	}

	return nil
}
