# Telegram 信号汇总表格 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把一轮分析的多条信号从「逐条 telegram 消息」改为「一条按动作分组的等宽表格 digest」。

**Architecture:** router 在 `batchNotify` 开关下把已放行信号入缓冲（路由决策/冷却/执行仍逐信号），`runAnalysisCycle` 在 `g.Wait()` 后 `FlushNotifications()` → 一次 `NotifyAllBatch` → telegram `SendBatch` 渲染按动作分组的等宽表格（CJK 按显示宽度对齐）。

**Tech Stack:** Go 1.24（标准库；无新依赖）。

> 关联 spec：`docs/superpowers/specs/2026-06-17-telegram-digest-table-design.md`

---

## 文件结构

- Create: `internal/notifier/telegram/width.go` — 显示宽度工具（CJK 记 2）
- Create: `internal/notifier/telegram/width_test.go`
- Modify: `internal/notifier/telegram/telegram.go` — 新增 `formatBatch`，重写 `SendBatch`
- Modify: `internal/notifier/telegram/telegram_test.go` — 表格测试
- Modify: `internal/router/router.go` — `Config.BatchNotify` + `pending` + Route 缓冲 + `FlushNotifications`
- Modify: `internal/router/router_test.go` — 缓冲/flush 测试
- Modify: `internal/config/config.go` — `RouterConfig.BatchNotify` + `SetDefault`
- Modify: `internal/app/app.go` — 映射 `BatchNotify` + cycle 末 `FlushNotifications`
- Modify: `configs/config.example.yaml` — `router.batch_notify`

---

## Task 1: 显示宽度工具（CJK 对齐）

**Files:**
- Create: `internal/notifier/telegram/width.go`
- Test: `internal/notifier/telegram/width_test.go`

- [ ] **Step 1: 写失败测试**

`internal/notifier/telegram/width_test.go`:
```go
package telegram

import "testing"

func TestDisplayWidth(t *testing.T) {
	cases := map[string]int{"": 0, "AAPL": 4, "贵州茅台": 8, "茅台A": 5, "0700.HK": 7}
	for s, want := range cases {
		if got := displayWidth(s); got != want {
			t.Errorf("displayWidth(%q) = %d, want %d", s, got, want)
		}
	}
}

func TestPadRight(t *testing.T) {
	// ASCII: pad with spaces to target width
	if got := padRight("AAPL", 6); got != "AAPL  " {
		t.Errorf("padRight ascii = %q", got)
	}
	// CJK: 茅台 is width 4, pad to 6 -> 2 trailing spaces
	if got := padRight("茅台", 6); got != "茅台  " {
		t.Errorf("padRight cjk = %q", got)
	}
	// already >= width: unchanged
	if got := padRight("AAPL", 3); got != "AAPL" {
		t.Errorf("padRight overflow = %q", got)
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/notifier/telegram/ -run 'TestDisplayWidth|TestPadRight' -v`
Expected: FAIL（`undefined: displayWidth` / `padRight`）

- [ ] **Step 3: 实现 width.go**

`internal/notifier/telegram/width.go`:
```go
package telegram

import "strings"

// displayWidth returns the number of fixed-width cells s occupies in a Telegram
// monospace code block, counting East-Asian wide runes (CJK) as 2 and others as
// 1. Padding by rune count would misalign rows containing Chinese names.
func displayWidth(s string) int {
	w := 0
	for _, r := range s {
		if isWide(r) {
			w += 2
		} else {
			w++
		}
	}
	return w
}

// padRight right-pads s with spaces to the given display width (no-op if s is
// already at least that wide).
func padRight(s string, width int) string {
	if n := width - displayWidth(s); n > 0 {
		return s + strings.Repeat(" ", n)
	}
	return s
}

// isWide reports whether r renders as a double-width (East-Asian) glyph. Covers
// the ranges that appear in atlas watchlist names/symbols; not a full Unicode
// East-Asian-Width table (avoids a third-party dependency).
func isWide(r rune) bool {
	switch {
	case r >= 0x1100 && r <= 0x115F, // Hangul Jamo
		r >= 0x2E80 && r <= 0x303E, // CJK radicals, Kangxi, CJK symbols
		r >= 0x3041 && r <= 0x33FF, // Hiragana/Katakana/CJK compat
		r >= 0x3400 && r <= 0x4DBF, // CJK Ext A
		r >= 0x4E00 && r <= 0x9FFF, // CJK Unified
		r >= 0xA000 && r <= 0xA4CF, // Yi
		r >= 0xAC00 && r <= 0xD7A3, // Hangul syllables
		r >= 0xF900 && r <= 0xFAFF, // CJK compat ideographs
		r >= 0xFF00 && r <= 0xFF60, // Fullwidth forms
		r >= 0xFFE0 && r <= 0xFFE6, // Fullwidth signs
		r >= 0x20000 && r <= 0x3FFFD: // CJK Ext B+
		return true
	}
	return false
}
```

- [ ] **Step 4: 运行确认通过**

Run: `go test ./internal/notifier/telegram/ -run 'TestDisplayWidth|TestPadRight' -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/notifier/telegram/width.go internal/notifier/telegram/width_test.go
git commit -m "feat(telegram): CJK-aware display-width helpers for table alignment"
```

---

## Task 2: 按动作分组的表格批量格式

**Files:**
- Modify: `internal/notifier/telegram/telegram.go`
- Modify: `internal/notifier/telegram/telegram_test.go`

`core.Action` 取值：`buy`/`sell`/`hold`/`strong_buy`/`strong_sell`（`internal/core/types.go:115-121`）。
分组：买入=`strong_buy`+`buy`，卖出=`strong_sell`+`sell`，持有=`hold`；组内按 `Confidence` 降序。

- [ ] **Step 1: 写失败测试（追加到 telegram_test.go）**

> `strings` 与 `github.com/newthinker/atlas/internal/core` 在 telegram_test.go 已导入，无需新增 import。

```go
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
	if !strings.Contains(out, "⏸ 持有") {
		t.Errorf("hold group missing:\n%s", out)
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/notifier/telegram/ -run TestFormatBatch -v`
Expected: FAIL（`undefined: formatBatch`）

- [ ] **Step 3: 实现 formatBatch + 重写 SendBatch**

在 `internal/notifier/telegram/telegram.go` 增加（`import` 需有 `sort`、`time`；`time` 已在）：
```go
// batchGroup is one action section of the digest table.
type batchGroup struct {
	title   string
	actions []core.Action
}

// digestGroups defines section order: buy, sell, then hold.
var digestGroups = []batchGroup{
	{"📈 买入", []core.Action{core.ActionStrongBuy, core.ActionBuy}},
	{"📉 卖出", []core.Action{core.ActionStrongSell, core.ActionSell}},
	{"⏸ 持有", []core.Action{core.ActionHold}},
}

// formatBatch renders signals as a Telegram message: a title line plus one
// monospace, display-width-aligned table per non-empty action group, rows
// sorted by confidence descending. Returns "" for an empty batch.
func formatBatch(signals []core.Signal) string {
	if len(signals) == 0 {
		return ""
	}
	var latest time.Time
	for _, s := range signals {
		if s.GeneratedAt.After(latest) {
			latest = s.GeneratedAt
		}
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📊 Atlas 信号汇总 · %s · %d 条\n",
		latest.Format("2006-01-02 15:04"), len(signals)))

	for _, g := range digestGroups {
		rows := make([]core.Signal, 0)
		for _, s := range signals {
			for _, a := range g.actions {
				if s.Action == a {
					rows = append(rows, s)
					break
				}
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
	header := []string{"SYMBOL", "NAME", "CONF", "PRICE"}
	cells := [][]string{header}
	for _, s := range rows {
		name, _ := s.Metadata["name"].(string)
		price := ""
		if s.Price > 0 {
			price = fmt.Sprintf("%.2f", s.Price)
		}
		cells = append(cells, []string{
			s.Symbol, name, fmt.Sprintf("%.1f%%", s.Confidence*100), price,
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
```
然后把现有 `SendBatch` 整体替换为：
```go
func (t *Telegram) SendBatch(signals []core.Signal) error {
	msg := formatBatch(signals)
	if msg == "" {
		return nil
	}
	return t.sendMessage(msg)
}
```

- [ ] **Step 4: 运行确认通过**

Run: `go test ./internal/notifier/telegram/ -run TestFormatBatch -v`
Expected: PASS

- [ ] **Step 5: 运行整个 telegram 包（零回归）+ 提交**

Run: `go test ./internal/notifier/telegram/ -count=1`
Expected: ok（含既有 Send/Init/proxy 测试）
```bash
git add internal/notifier/telegram/telegram.go internal/notifier/telegram/telegram_test.go
git commit -m "feat(telegram): group-by-action digest table in SendBatch"
```

---

## Task 3: router 延迟通知缓冲 + Flush

**Files:**
- Modify: `internal/router/router.go`
- Modify: `internal/router/router_test.go`

- [ ] **Step 1: 写失败测试（追加到 router_test.go）**

```go
// countingNotifier records Send vs SendBatch calls.
type countingNotifier struct {
	sends   int
	batches int
	lastN   int
}

func (c *countingNotifier) Name() string                 { return "counting" }
func (c *countingNotifier) Init(notifier.Config) error   { return nil }
func (c *countingNotifier) Send(core.Signal) error       { c.sends++; return nil }
func (c *countingNotifier) SendBatch(s []core.Signal) error {
	c.batches++
	c.lastN = len(s)
	return nil
}

func newRouterWithNotifier(cfg Config) (*Router, *countingNotifier) {
	reg := notifier.NewRegistry()
	cn := &countingNotifier{}
	reg.Register(cn)
	return New(cfg, reg, nil), cn
}

func TestRoute_BatchNotify_BuffersUntilFlush(t *testing.T) {
	cfg := Config{MinConfidence: 0.5, BatchNotify: true}
	r, cn := newRouterWithNotifier(cfg)

	r.Route(core.Signal{Symbol: "AAPL", Action: core.ActionBuy, Confidence: 0.9})
	r.Route(core.Signal{Symbol: "MSFT", Action: core.ActionSell, Confidence: 0.9})

	if cn.sends != 0 || cn.batches != 0 {
		t.Fatalf("batch mode must not notify during Route (sends=%d batches=%d)", cn.sends, cn.batches)
	}
	r.FlushNotifications()
	if cn.batches != 1 || cn.lastN != 2 {
		t.Fatalf("flush should send one batch of 2, got batches=%d n=%d", cn.batches, cn.lastN)
	}
}

func TestRoute_NonBatch_NotifiesImmediately(t *testing.T) {
	cfg := Config{MinConfidence: 0.5, BatchNotify: false}
	r, cn := newRouterWithNotifier(cfg)
	r.Route(core.Signal{Symbol: "AAPL", Action: core.ActionBuy, Confidence: 0.9})
	if cn.sends != 1 || cn.batches != 0 {
		t.Fatalf("non-batch mode must Send immediately (sends=%d batches=%d)", cn.sends, cn.batches)
	}
}

func TestFlush_EmptyIsNoop(t *testing.T) {
	cfg := Config{MinConfidence: 0.5, BatchNotify: true}
	r, cn := newRouterWithNotifier(cfg)
	r.FlushNotifications()
	if cn.batches != 0 {
		t.Fatalf("empty flush must not send, got batches=%d", cn.batches)
	}
}
```
> 注：若 router_test.go 已有等价 fake notifier，复用它、删去本处重复定义即可。

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/router/ -run 'BatchNotify|NonBatch|Flush' -v`
Expected: FAIL（`unknown field BatchNotify` / `undefined: FlushNotifications`）

- [ ] **Step 3: 实现**

在 `internal/router/router.go` 的 `Config` 增加字段：
```go
	// BatchNotify defers notification: Route buffers passed signals instead of
	// notifying immediately; FlushNotifications sends them as one batch. Routing
	// decision/cooldown/execution stay per-signal. Default wired true by config.
	BatchNotify bool `mapstructure:"batch_notify"`
```
在 `Router` 结构体增加字段（`pending` 复用现有 `mu`）：
```go
	pending []core.Signal
```
把 `Route` 中「Send to all notifiers」段（现 96-119 行，从 `if r.registry == nil` 到 `return true, nil`）替换为：
```go
	// nil registry: nothing to notify (parity with original).
	if r.registry == nil {
		return true, nil
	}
	// Batch mode: buffer and defer; FlushNotifications sends one batch per cycle.
	if r.cfg.BatchNotify {
		r.mu.Lock()
		r.pending = append(r.pending, signal)
		r.mu.Unlock()
		return true, nil
	}
	errors := r.registry.NotifyAll(signal)
	if len(errors) > 0 {
		for name, err := range errors {
			r.logger.Error("notifier failed", zap.String("notifier", name), zap.Error(err))
		}
	}
	r.logger.Info("signal routed",
		zap.String("symbol", signal.Symbol),
		zap.String("action", string(signal.Action)),
		zap.Float64("confidence", signal.Confidence),
		zap.Int("notifiers", len(r.registry.GetAll())),
		zap.Int("errors", len(errors)),
	)
	return true, nil
```
在 `RouteBatch` 之前新增方法：
```go
// FlushNotifications sends all buffered (batch-notify) signals as one batch and
// clears the buffer. No-op when the buffer is empty or no registry is set.
// Called at the end of an analysis cycle.
func (r *Router) FlushNotifications() {
	r.mu.Lock()
	batch := r.pending
	r.pending = nil
	r.mu.Unlock()

	if len(batch) == 0 || r.registry == nil {
		return
	}
	errors := r.registry.NotifyAllBatch(batch)
	for name, err := range errors {
		r.logger.Error("notifier failed on digest", zap.String("notifier", name), zap.Error(err))
	}
	r.logger.Info("signal digest sent", zap.Int("count", len(batch)), zap.Int("errors", len(errors)))
}
```

- [ ] **Step 4: 运行确认通过 + 全包零回归**

Run: `go test ./internal/router/ -count=1`
Expected: ok（新测试 + 既有路由/冷却/门测试全过）

- [ ] **Step 5: 提交**

```bash
git add internal/router/router.go internal/router/router_test.go
git commit -m "feat(router): batch-notify buffer + FlushNotifications (defer to cycle end)"
```

---

## Task 4: 配置默认 + app 接线 + cycle 末 flush

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/app/app.go`
- Modify: `configs/config.example.yaml`

- [ ] **Step 1: config 加字段 + 默认**

在 `internal/config/config.go` 的 `RouterConfig` 增加字段（紧接 `PercentileStep` 之后）：
```go
	// BatchNotify aggregates a cycle's signals into one digest notification
	// instead of per-signal messages. Default true (see Load SetDefault).
	BatchNotify bool `mapstructure:"batch_notify"`
```
在 `Load`（`internal/config/config.go:254`）的 SetDefault 区块追加一行：
```go
	v.SetDefault("router.batch_notify", true)
```

- [ ] **Step 2: app 映射 + flush 接线**

在 `internal/app/app.go` 的 `router.Config{...}`（约 92-96 行）增加字段：
```go
		BatchNotify:      cfg.Router.BatchNotify,
```
在 `runAnalysisCycle` 的 `_ = g.Wait()` 之后追加一行（同函数内，紧接其后）：
```go
	r.FlushNotifications()
```
> ⚠ 用 `runAnalysisCycle` 内对 router 的实际字段名（结构体为 `a.router`，函数内若无局部别名则写 `a.router.FlushNotifications()`）。确认：该函数末尾当前是 `_ = g.Wait()`，其后直接加 `a.router.FlushNotifications()`。

- [ ] **Step 3: config.example.yaml 文档化**

在 `configs/config.example.yaml` 的 `router:` 段增加：
```yaml
  batch_notify: true     # true=一轮信号汇总成一条表格消息(digest)；false=每条信号即时单发
```

- [ ] **Step 4: 全量构建 + 测试**

Run: `go build ./... && go test ./internal/router/ ./internal/config/ ./internal/notifier/... ./internal/app/ ./cmd/... -count=1`
Expected: 编译通过；全部 ok（零回归）

- [ ] **Step 5: 提交**

```bash
git add internal/config/config.go internal/app/app.go configs/config.example.yaml
git commit -m "feat(router): default batch_notify on; flush digest at cycle end"
```

---

## 完成标准（DoD）

- 一轮分析的多条放行信号汇成**一条** telegram 消息，按「买入/卖出/持有」分组、组内按置信度降序、含中文名时列对齐。
- `batch_notify:false` 时完全回到逐条即时发（回退路径，有测试）。
- 执行/冷却/信号存储语义不变（仍逐信号）。
- 空轮不发消息。
- `go build ./...` 通过；router/telegram/config/app 包测试全绿。

## 部署验证（实现后，按运维手册）

```bash
bash scripts/ops/deploy.sh && bash scripts/ops/services.sh restart
bash scripts/ops/services.sh analysis-now      # 触发一轮
# 预期：Telegram 收到一条按动作分组的等宽表格，而非数十条单条消息
```

## 范围边界（不做）

- 不引第三方 runewidth 库；不改 email/webhook 批量格式；不做跨轮聚合 / 自定义列。
