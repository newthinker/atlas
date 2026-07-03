package telegram

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/newthinker/atlas/internal/core"
	"github.com/newthinker/atlas/internal/notifier"
)

// httpTimeout bounds each Telegram Bot API call (direct or proxied).
const httpTimeout = 30 * time.Second

// Telegram implements the Notifier interface for Telegram Bot API
type Telegram struct {
	botToken string
	chatID   string
	client   *http.Client
}

// Option configures a Telegram notifier.
type Option func(*Telegram)

// WithProxy routes Telegram API calls through an HTTP/HTTPS/SOCKS5 proxy
// (e.g. "http://127.0.0.1:7890" or "socks5://127.0.0.1:1080"). Needed where
// api.telegram.org is not directly reachable. Empty or unparseable → direct.
// Scoped to this notifier only, so market collectors keep their direct path.
func WithProxy(proxyURL string) Option {
	return func(t *Telegram) { t.applyProxy(proxyURL) }
}

// applyProxy sets a proxied transport on the client when proxyURL is valid.
// The nil-client guard covers Init being called on a bare &Telegram{} (no New),
// as the notifier registry and tests do.
func (t *Telegram) applyProxy(proxyURL string) {
	if proxyURL == "" {
		return
	}
	u, err := url.Parse(proxyURL)
	if err != nil {
		return
	}
	if t.client == nil {
		t.client = &http.Client{Timeout: httpTimeout}
	}
	t.client.Transport = &http.Transport{Proxy: http.ProxyURL(u)}
}

// New creates a new Telegram notifier
func New(botToken, chatID string, opts ...Option) *Telegram {
	t := &Telegram{
		botToken: botToken,
		chatID:   chatID,
		client:   &http.Client{Timeout: httpTimeout},
	}
	for _, o := range opts {
		o(t)
	}
	return t
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
	if proxy, ok := cfg.Params["proxy"].(string); ok {
		t.applyProxy(proxy)
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

// SendText sends a pre-formatted message as plain text with no parse_mode, so
// Telegram does not attempt Markdown parsing. Alert text ("[SEVERITY] name:
// message", rules.go) carries arbitrary operator-supplied characters; unpaired
// Markdown metacharacters (_ * [ `) would otherwise make the API reject the
// message with HTTP 400 and the alert would be silently lost. Plain text
// delivers it verbatim. Used by the alert adapter's direct path.
func (t *Telegram) SendText(text string) error {
	return t.sendPayload(text, "")
}

func (t *Telegram) SendBatch(signals []core.Signal) error {
	msg := formatBatch(signals)
	if msg == "" {
		return nil
	}
	// W1: digest content lives inside ``` code blocks; underscores must render
	// literally. Skip markdown escaping for this path.
	return t.sendRaw(msg)
}

// batchGroup is one action section of the digest table.
type batchGroup struct {
	title   string
	actions []core.Action
}

// digestGroups defines section order: buy, sell, then hold.
// I3: hold icon uses ⏸️ (with variation selector) to match formatSignal.
var digestGroups = []batchGroup{
	{"📈 买入", []core.Action{core.ActionStrongBuy, core.ActionBuy}},
	{"📉 卖出", []core.Action{core.ActionStrongSell, core.ActionSell}},
	{"⏸️ 持有", []core.Action{core.ActionHold}},
}

// formatBatch renders signals as a Telegram message: a title line plus one
// monospace, display-width-aligned table per non-empty action group, rows
// sorted by confidence descending. Returns "" for an empty batch.
func formatBatch(signals []core.Signal) string {
	if len(signals) == 0 {
		return ""
	}
	// W2: omit timestamp when all GeneratedAt are zero to avoid "0001-01-01".
	var latest time.Time
	for _, s := range signals {
		if s.GeneratedAt.After(latest) {
			latest = s.GeneratedAt
		}
	}
	var sb strings.Builder
	if latest.IsZero() {
		sb.WriteString(fmt.Sprintf("📊 Atlas 信号汇总 · %d 条\n", len(signals)))
	} else {
		sb.WriteString(fmt.Sprintf("📊 Atlas 信号汇总 · %s · %d 条\n",
			latest.Format("2006-01-02 15:04"), len(signals)))
	}

	for _, g := range digestGroups {
		rows := make([]core.Signal, 0)
		for _, s := range signals {
			// I1: use slices.Contains instead of inner loop.
			if slices.Contains(g.actions, s.Action) {
				rows = append(rows, s)
			}
		}
		if len(rows) == 0 {
			continue
		}
		sort.SliceStable(rows, func(i, j int) bool { return rows[i].Confidence > rows[j].Confidence })
		sb.WriteString("\n")
		sb.WriteString(g.title)
		sb.WriteString("\n")
		sb.WriteString(renderTable(rows))
	}
	return sb.String()
}

// renderTable builds a fenced, column-aligned table for one group's rows.
func renderTable(rows []core.Signal) string {
	header := []string{"SYMBOL", "NAME", "CONF", "PRICE", "PE%"}
	cells := [][]string{header}
	for _, s := range rows {
		name, _ := s.Metadata["name"].(string)
		price := ""
		if s.Price > 0 {
			price = fmt.Sprintf("%.2f", s.Price)
		}
		// pe_percentile_display is a display-only key (0-100) stamped by the app
		// layer from Fundamental.PEPercentile; absent for symbols without PE.
		pePct := ""
		if v, ok := s.Metadata["pe_percentile_display"].(float64); ok {
			pePct = fmt.Sprintf("%.1f%%", v)
		}
		cells = append(cells, []string{
			s.Symbol, name, fmt.Sprintf("%.1f%%", s.Confidence*100), price, pePct,
		})
	}
	widths := make([]int, len(header))
	for _, row := range cells {
		for i, c := range row {
			if w := displayWidth(c); w > widths[i] {
				widths[i] = w
			}
		}
	}
	var sb strings.Builder
	sb.WriteString("```\n")
	for _, row := range cells {
		for i, c := range row {
			if i == len(row)-1 {
				sb.WriteString(c) // last column: no trailing pad
			} else {
				sb.WriteString(padRight(c, widths[i]))
				sb.WriteString("  ")
			}
		}
		sb.WriteString("\n")
	}
	sb.WriteString("```\n")
	return sb.String()
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

// sendMessage escapes Markdown special characters before sending. Used by the
// per-signal Send path where formatSignal output contains *bold* markers that
// require _ to be escaped.
func (t *Telegram) sendMessage(text string) error {
	return t.sendRaw(escapeMarkdown(text))
}

// sendRaw sends text as-is (no escaping) with Markdown parse_mode. Used by
// SendBatch whose digest content lives inside ``` code blocks where _ must
// render literally.
func (t *Telegram) sendRaw(text string) error {
	return t.sendPayload(text, "Markdown")
}

// sendPayload posts text to the Telegram sendMessage API. A non-empty parseMode
// is forwarded as-is (e.g. "Markdown"); an empty parseMode omits the field so
// the message is delivered as plain text (no metacharacter interpretation).
func (t *Telegram) sendPayload(text, parseMode string) error {
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.botToken)

	payload := map[string]any{
		"chat_id": t.chatID,
		"text":    text,
	}
	if parseMode != "" {
		payload["parse_mode"] = parseMode
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("telegram: failed to marshal payload: %w", err)
	}

	resp, err := t.client.Post(apiURL, "application/json", bytes.NewReader(body))
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
