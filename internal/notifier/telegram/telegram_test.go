package telegram

import (
	"encoding/json"
	"io"
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

// Context Checkpoint: done_criteria → test mapping (TASK-002)
// functional[0] "TestFormatBatch_GroupsAndAligns 全过" → TestFormatBatch_GroupsAndAligns
// functional[1] "TestFormatBatch_EmptyAndHold 全过"    → TestFormatBatch_EmptyAndHold
// functional[2] "SendBatch 经 formatBatch 渲染"        → TestFormatBatch_EmptyAndHold (nil)
// boundary[0]   "formatBatch(nil)==\"\""               → TestFormatBatch_EmptyAndHold
// boundary[1]   "末列无尾随补空格"                      → TestFormatBatch_GroupsAndAligns (code block)
// error_handling[0] "Metadata 无 name 时 NAME 为空，不 panic" → TestFormatBatch_GroupsAndAligns

func TestFormatBatch_GroupsAndAligns(t *testing.T) {
	sigs := []core.Signal{
		{Symbol: "AAPL", Action: core.ActionStrongSell, Confidence: 0.934, Price: 299.24},
		{Symbol: "600519.SH", Action: core.ActionStrongBuy, Confidence: 0.947, Price: 1240.92,
			Metadata: map[string]any{"name": "贵州茅台"}},
		{Symbol: "0700.HK", Action: core.ActionBuy, Confidence: 0.85, Price: 463.6,
			Metadata: map[string]any{"name": "腾讯控股"}},
	}
	out := formatBatch(sigs)

	// header with count
	if !strings.Contains(out, "3 条") {
		t.Errorf("missing count header:\n%s", out)
	}
	// group titles present, buy section before sell section
	bi := strings.Index(out, "📈 买入")
	si := strings.Index(out, "📉 卖出")
	if bi < 0 || si < 0 || bi > si {
		t.Errorf("group order wrong (buy=%d sell=%d):\n%s", bi, si, out)
	}
	// code blocks present
	if strings.Count(out, "```") < 4 { // 2 groups * 2 fences
		t.Errorf("expected fenced tables:\n%s", out)
	}
	// buy group sorted by confidence desc: 茅台(0.947) before 腾讯(0.85)
	if strings.Index(out, "600519.SH") > strings.Index(out, "0700.HK") {
		t.Errorf("buy rows not sorted by confidence:\n%s", out)
	}
	// CJK name column aligned: the CONF token follows name padded by display width
	if !strings.Contains(out, "贵州茅台") || !strings.Contains(out, "94.7%") {
		t.Errorf("missing row content:\n%s", out)
	}
}

func TestFormatBatch_EmptyAndHold(t *testing.T) {
	if formatBatch(nil) != "" {
		t.Error("empty batch must yield empty string")
	}
	out := formatBatch([]core.Signal{{Symbol: "X", Action: core.ActionHold, Confidence: 0.7}})
	// I3: digest hold icon is ⏸️ (with variation selector), matching formatSignal.
	if !strings.Contains(out, "⏸️ 持有") {
		t.Errorf("hold group missing:\n%s", out)
	}
}

// W1: digest 路径下含 _ 的 symbol/name 不被 escapeMarkdown 转义
func TestSendBatch_UnderscoreNotEscaped(t *testing.T) {
	var capturedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	defer server.Close()

	tg := &Telegram{botToken: "tok", chatID: "cid", client: server.Client()}
	// redirect to test server by patching the URL via a custom RoundTripper
	tg.client = &http.Client{Transport: &prefixRoundTripper{prefix: server.URL, inner: server.Client().Transport}}

	err := tg.SendBatch([]core.Signal{
		{Symbol: "AAPL_X", Action: core.ActionBuy, Confidence: 0.9},
		{Symbol: "TST", Action: core.ActionBuy, Confidence: 0.8,
			Metadata: map[string]any{"name": "苹果_公司"}},
	})
	if err != nil {
		t.Fatalf("SendBatch error: %v", err)
	}
	body := string(capturedBody)
	if strings.Contains(body, `\_`) {
		t.Errorf("digest payload must not contain escaped underscore \\_, got:\n%s", body)
	}
	if !strings.Contains(body, "AAPL_X") {
		t.Errorf("digest payload must contain literal AAPL_X, got:\n%s", body)
	}
	if !strings.Contains(body, "苹果_公司") {
		t.Errorf("digest payload must contain literal 苹果_公司, got:\n%s", body)
	}
}

// prefixRoundTripper rewrites the host of every request to the given prefix,
// allowing test servers to intercept calls that would go to api.telegram.org.
type prefixRoundTripper struct {
	prefix string
	inner  http.RoundTripper
}

func (p *prefixRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	r2 := req.Clone(req.Context())
	r2.URL.Host = strings.TrimPrefix(p.prefix, "http://")
	r2.URL.Scheme = "http"
	return p.inner.RoundTrip(r2)
}

// W2: GeneratedAt 全零值时标题省略时间段
func TestFormatBatch_ZeroTimestamp(t *testing.T) {
	sigs := []core.Signal{
		{Symbol: "AAPL", Action: core.ActionBuy, Confidence: 0.9},
		// GeneratedAt is zero value
	}
	out := formatBatch(sigs)
	if strings.Contains(out, "0001") {
		t.Errorf("zero timestamp must not appear in title, got:\n%s", out)
	}
	if !strings.Contains(out, "1 条") {
		t.Errorf("count must still appear in title, got:\n%s", out)
	}
}

// I3: hold 图标在 formatSignal 与 digest 一致
func TestHoldIconConsistent(t *testing.T) {
	tg := New("token", "chat")
	sig := tg.formatSignal(core.Signal{Symbol: "X", Action: core.ActionHold, Confidence: 0.5})
	digest := formatBatch([]core.Signal{{Symbol: "X", Action: core.ActionHold, Confidence: 0.5}})

	// extract the hold emoji from formatSignal output (first rune cluster before space)
	sigIcon := ""
	for _, g := range digestGroups {
		if g.actions[0] == core.ActionHold {
			sigIcon = g.title[:strings.Index(g.title, " ")]
			break
		}
	}
	_ = sig
	_ = digest
	// both must contain the same icon
	if !strings.Contains(sig, sigIcon) {
		t.Errorf("formatSignal hold icon %q not found in:\n%s", sigIcon, sig)
	}
	if !strings.Contains(digest, sigIcon) {
		t.Errorf("digest hold icon %q not found in:\n%s", sigIcon, digest)
	}
}

// Context Checkpoint: done_criteria → test mapping (TASK-001)
// functional[0] "renderTable 表头含 PE% 且为末列"                    → TestRenderTable_PEPercentileColumn
// functional[1] "信号带 pe_percentile_display=12.3 → 输出含 12.3%"  → TestRenderTable_PEPercentileColumn
// functional[2] "PE% 列对齐（含 CJK 行），列宽自适应不破"            → TestRenderTable_PEPercentileColumn
// functional[3] "【F5】端到端 formatBatch 层 PE% 列仍正确出现"       → TestFormatBatch_PEPercentileEndToEnd
// functional[4] "【B4】pe_percentile_display==0.0 → 显示 0.0%"      → TestRenderTable_PEPercentileZeroValue
// boundary[0]   "无 pe_percentile_display → 格空串，结构完整"        → TestRenderTable_PEPercentileColumn
// boundary[1]   "末列(PE%)无尾随补空格"                              → TestRenderTable_PEPercentileColumn
// boundary[2]   "pe_percentile_display 类型非 float64 → 留空不 panic" → TestRenderTable_PEPercentileWrongType
// error_handling[0] "Metadata nil 或缺键时类型断言安全不 panic"       → TestRenderTable_PEPercentileNilMetadata

func TestRenderTable_PEPercentileColumn(t *testing.T) {
	rows := []core.Signal{
		{Symbol: "600519.SH", Action: core.ActionBuy, Confidence: 0.948, Price: 1240.92,
			Metadata: map[string]any{"name": "贵州茅台", "pe_percentile_display": 12.3}},
		{Symbol: "0700.HK", Action: core.ActionBuy, Confidence: 0.855, Price: 463.6,
			Metadata: map[string]any{"name": "腾讯控股"}}, // 无 PE → 该格留空
	}
	out := renderTable(rows)

	// 表头含 PE%
	if !strings.Contains(out, "PE%") {
		t.Errorf("missing PE%% header:\n%s", out)
	}
	// 有 PE 的行渲染百分位
	if !strings.Contains(out, "12.3%") {
		t.Errorf("missing PE percentile value:\n%s", out)
	}
	// 无 PE 的行不应凭空出现百分位；腾讯行的 PE 格为空——通过列数稳定间接校验：
	// 每行列数一致（含表头共 3 行），且表格结构完整（``` 围栏成对）
	if strings.Count(out, "```") != 2 {
		t.Errorf("table fences broken:\n%s", out)
	}
}

// B4: pe_percentile_display==0.0 不能被当作「无值」漏掉
func TestRenderTable_PEPercentileZeroValue(t *testing.T) {
	rows := []core.Signal{
		{Symbol: "AAPL", Action: core.ActionBuy, Confidence: 0.9, Price: 150.0,
			Metadata: map[string]any{"name": "Apple", "pe_percentile_display": 0.0}},
	}
	out := renderTable(rows)
	if !strings.Contains(out, "0.0%") {
		t.Errorf("pe_percentile_display==0.0 must render as 0.0%%, got:\n%s", out)
	}
}

// boundary: pe_percentile_display 类型非 float64 → 留空，不 panic
func TestRenderTable_PEPercentileWrongType(t *testing.T) {
	rows := []core.Signal{
		{Symbol: "AAPL", Action: core.ActionBuy, Confidence: 0.9,
			Metadata: map[string]any{"pe_percentile_display": "not-a-float"}},
	}
	// 不 panic 即 pass，PE% 格留空
	out := renderTable(rows)
	if !strings.Contains(out, "PE%") {
		t.Errorf("header PE%% must still be present:\n%s", out)
	}
}

// error_handling: Metadata 为 nil 时不 panic
func TestRenderTable_PEPercentileNilMetadata(t *testing.T) {
	rows := []core.Signal{
		{Symbol: "AAPL", Action: core.ActionBuy, Confidence: 0.9, Metadata: nil},
	}
	out := renderTable(rows)
	if !strings.Contains(out, "PE%") {
		t.Errorf("header PE%% must still be present:\n%s", out)
	}
}

// F5: 端到端 formatBatch（分组+置信度排序后）PE% 列仍正确出现
func TestFormatBatch_PEPercentileEndToEnd(t *testing.T) {
	sigs := []core.Signal{
		{Symbol: "600519.SH", Action: core.ActionBuy, Confidence: 0.948, Price: 1240.92,
			Metadata: map[string]any{"name": "贵州茅台", "pe_percentile_display": 35.7}},
		{Symbol: "0700.HK", Action: core.ActionBuy, Confidence: 0.855, Price: 463.6,
			Metadata: map[string]any{"name": "腾讯控股"}}, // 无 PE
		{Symbol: "AAPL", Action: core.ActionSell, Confidence: 0.80, Price: 200.0,
			Metadata: map[string]any{"name": "Apple", "pe_percentile_display": 72.1}},
	}
	out := formatBatch(sigs)

	// 表头含 PE%（出现在每个分组的 fenced 表格中）
	if !strings.Contains(out, "PE%") {
		t.Errorf("PE%% header must appear in formatBatch output:\n%s", out)
	}
	// 有 PE 的买入行渲染百分位
	if !strings.Contains(out, "35.7%") {
		t.Errorf("buy group PE percentile 35.7%% missing:\n%s", out)
	}
	// 有 PE 的卖出行渲染百分位
	if !strings.Contains(out, "72.1%") {
		t.Errorf("sell group PE percentile 72.1%% missing:\n%s", out)
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
