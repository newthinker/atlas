# Telegram digest「PE%」列 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 Telegram 信号汇总表格末尾增加一列 `PE%`，显示每个标的当前市盈率在其历史 PE 中的百分位。

**Architecture:** PE 历史百分位在 `buildFundamental` 已算好（`Fundamental.PEPercentile`，-1=不可用）。`enrichSignalMetadata` 增参接收该标的 Fundamental，给每条信号盖上**展示专用键** `Metadata["pe_percentile_display"]`（router 不读它，门控零影响）；`renderTable` 末列读此键渲染 `PE%`，无值留空。

**Tech Stack:** Go 1.24（标准库，无新依赖）。

> 关联 spec：`docs/superpowers/specs/2026-06-17-telegram-digest-pe-percentile-column-design.md`

---

## 文件结构

- Modify: `internal/notifier/telegram/telegram.go` — `renderTable` 加 `PE%` 列
- Modify: `internal/notifier/telegram/telegram_test.go` — PE% 列测试
- Modify: `internal/app/app.go` — `enrichSignalMetadata` 增参 + 盖 `pe_percentile_display`；调用点传 `analysisCtx.Fundamental`
- Modify: `internal/app/app_test.go` — 更新 2 处调用签名 + 新增 PE 盖键用例

---

## Task 1: renderTable 增加 PE% 列

**Files:**
- Modify: `internal/notifier/telegram/telegram.go`（renderTable，166-202 行）
- Test: `internal/notifier/telegram/telegram_test.go`

- [ ] **Step 1: 写失败测试（追加到 telegram_test.go）**

> `strings`、`github.com/newthinker/atlas/internal/core` 已在 telegram_test.go 导入。

```go
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
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/notifier/telegram/ -run TestRenderTable_PEPercentileColumn -v`
Expected: FAIL（输出无 `PE%` 表头 / 无 `12.3%`）

- [ ] **Step 3: 实现——renderTable 加 PE% 列**

把 `internal/notifier/telegram/telegram.go` 的 `renderTable` 整体替换为：
```go
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
```

- [ ] **Step 4: 运行确认通过 + 整包零回归**

Run: `go test ./internal/notifier/telegram/ -run TestRenderTable_PEPercentileColumn -v`
Expected: PASS
Run: `go test ./internal/notifier/telegram/ -count=1`
Expected: ok（既有 TestFormatBatch_* 等 Contains 断言不受加列影响）

- [ ] **Step 5: 提交**

```bash
git add internal/notifier/telegram/telegram.go internal/notifier/telegram/telegram_test.go
git commit -m "feat(telegram): PE% historical-percentile column in digest table"
```

---

## Task 2: enrichSignalMetadata 盖 pe_percentile_display

**Files:**
- Modify: `internal/app/app.go`（enrichSignalMetadata 399-414；调用点 503）
- Test: `internal/app/app_test.go`（更新 1280/1293 调用 + 新用例）

- [ ] **Step 1: 写失败测试（更新现有调用 + 追加 PE 用例到 app_test.go）**

把 app_test.go 中现有两处调用补第三参（搜索 `enrichSignalMetadata(`）：
```go
// 1280 行附近
enrichSignalMetadata(sigs, WatchlistItem{Symbol: "0883.HK", Name: "中国海洋石油"}, nil)
// 1293 行附近
enrichSignalMetadata(plain, WatchlistItem{Symbol: "X"}, nil)
```
追加新测试：
```go
func TestEnrichSignalMetadata_StampsPEPercentile(t *testing.T) {
	sigs := []core.Signal{{Symbol: "600519.SH"}, {Symbol: "600519.SH"}}
	f := &core.Fundamental{Symbol: "600519.SH", PEPercentile: 12.3}
	enrichSignalMetadata(sigs, WatchlistItem{Symbol: "600519.SH", Name: "贵州茅台"}, f)
	for _, s := range sigs {
		if got, _ := s.Metadata["pe_percentile_display"].(float64); got != 12.3 {
			t.Errorf("pe_percentile_display = %v, want 12.3", s.Metadata["pe_percentile_display"])
		}
		if s.Metadata["name"] != "贵州茅台" {
			t.Errorf("name not stamped: %v", s.Metadata["name"])
		}
	}
}

func TestEnrichSignalMetadata_NoPEWhenUnavailable(t *testing.T) {
	// PEPercentile = -1（不可用哨兵）→ 不盖键
	sigs := []core.Signal{{Symbol: "X"}}
	enrichSignalMetadata(sigs, WatchlistItem{Symbol: "X", Name: "n"},
		&core.Fundamental{Symbol: "X", PEPercentile: -1})
	if _, ok := sigs[0].Metadata["pe_percentile_display"]; ok {
		t.Error("must not stamp pe_percentile_display when PEPercentile < 0")
	}
	// fundamental = nil（如 ETF）→ 不盖键，且 name 仍盖
	sigs2 := []core.Signal{{Symbol: "Y"}}
	enrichSignalMetadata(sigs2, WatchlistItem{Symbol: "Y", Name: "etf"}, nil)
	if _, ok := sigs2[0].Metadata["pe_percentile_display"]; ok {
		t.Error("nil fundamental must not stamp pe_percentile_display")
	}
	if sigs2[0].Metadata["name"] != "etf" {
		t.Error("name must still be stamped when fundamental is nil")
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/app/ -run 'TestEnrichSignalMetadata' -v`
Expected: FAIL（编译失败：enrichSignalMetadata 参数数量不符 / 新键未盖）

- [ ] **Step 3: 实现——enrichSignalMetadata 增参 + 盖键 + 改调用点**

把 `internal/app/app.go` 的 `enrichSignalMetadata` 整体替换为（注意：去掉原 `item.Name==""` 提前返回，改为内部条件——因为一条信号可能无 name 但有 PE）：
```go
// enrichSignalMetadata stamps watchlist display info onto outgoing signals so
// notifiers can render human-friendly rows. Existing keys are never overwritten.
//   - Metadata["name"]: watchlist display name (skipped when item.Name is empty).
//   - Metadata["pe_percentile_display"]: the symbol's PE historical percentile
//     (0-100) from Fundamental, for the digest PE% column. Display-only — the
//     router never reads it; stamped only when a valid PE percentile exists.
func enrichSignalMetadata(signals []core.Signal, item WatchlistItem, fundamental *core.Fundamental) {
	hasPE := fundamental != nil && fundamental.PEPercentile >= 0
	if item.Name == "" && !hasPE {
		return
	}
	for i := range signals {
		if signals[i].Metadata == nil {
			signals[i].Metadata = make(map[string]any, 2)
		}
		if item.Name != "" {
			if _, exists := signals[i].Metadata["name"]; !exists {
				signals[i].Metadata["name"] = item.Name
			}
		}
		if hasPE {
			if _, exists := signals[i].Metadata["pe_percentile_display"]; !exists {
				signals[i].Metadata["pe_percentile_display"] = fundamental.PEPercentile
			}
		}
	}
}
```
把调用点（app.go:503）改为：
```go
	enrichSignalMetadata(signals, item, analysisCtx.Fundamental)
```

- [ ] **Step 4: 运行确认通过 + 整包零回归**

Run: `go test ./internal/app/ -run 'TestEnrichSignalMetadata' -v`
Expected: PASS
Run: `go test ./internal/app/ -count=1`
Expected: ok

- [ ] **Step 5: 全量构建 + 关键包测试 + 提交**

Run: `go build ./... && go test ./internal/app/ ./internal/notifier/... ./internal/router/ -count=1`
Expected: 编译通过；全部 ok（router 零回归——`pe_percentile_display` 不被 `percentileOf` 读取）
```bash
git add internal/app/app.go internal/app/app_test.go
git commit -m "feat(app): stamp pe_percentile_display on signals for digest PE% column"
```

---

## 完成标准（DoD）

- digest 表格末列出现 `PE%`，有 PE 数据的行显示 `xx.x%`、无 PE 的行（ETF/金融指数/nil Fundamental）留空；CJK 行仍对齐、末列无尾随补空格。
- 同一标的两条信号（price_percentile + pe_percentile）显示相同 PE 百分位。
- `enrichSignalMetadata`：有 PE 盖 `pe_percentile_display`；PEPercentile<0 或 fundamental=nil 不盖；name 逻辑零回归。
- router 门控行为不变（`pe_percentile_display` 非门控键）。
- `go build ./...` 通过；app/telegram/router 包测试全绿。

## 范围边界（不做）

- 不做 ROE 列（无数据源）、不做 PE 原始数值列、不做同标的去重、不改 router 门控 / email / webhook。
