# Telegram digest 增加「PE 历史百分位」列 设计

> 关联前序：`docs/superpowers/specs/2026-06-17-telegram-digest-table-design.md`（分组表格 digest）

## 目标

在 Telegram 信号汇总表格（digest）中，为**每条信号行**增加一列 `PE_PCT`，显示该标的「当前市盈率在其历史市盈率中的百分位」（0–100%）。

## 范围

- ✅ **PE 历史百分位列**：数据已现成（`Fundamental.PEPercentile`）。
- ❌ **ROE 历史百分位列**：当前无数据源（lixinger `non_financial` 端点不返回 ROE，Yahoo 仅 EPS，本地 qlibpit 仅 eps_ttm，`Fundamental.ROE` 恒为 0）。**本次不做**，待 ROE 数据管道就位后另开 sprint。

## 背景事实（已核验）

- `Fundamental.PEPercentile` 在 `internal/app/app.go` 的 `buildFundamental`（行 825 起）为每个有 PE 的标的算好；初始 `-1` 表示不可用；来源 lixinger cvpos 或 `ReconstructPEPercentile`。
- 当前只有 `pe_percentile` 策略把它抄进 `Metadata["pe_percentile"]`；`price_percentile` 策略的信号不带 PE 百分位。
- 多数 watchlist 标的同时跑 `price_percentile` + `pe_percentile`，故同一标的在 digest 中可能出现两行（确认行为）。
- **router 门控耦合**：`router.percentileOf`（router.go:251）按顺序读 `Metadata["percentile"]`→`["pe_percentile"]` 决定百分位 step 门。**因此本设计不复用这两个键**，改用独立展示键，确保 router 行为零变化。

## 设计

### 数据流（唯一改动点在 app 富化层）

```
analyzeSymbol:
  analysisCtx.Fundamental = buildFundamental(...)   // 已有：含 PEPercentile（-1=不可用）
  signals = AnalyzeWithStrategies(...)
  enrichSignalMetadata(signals, item, fundamental)  // ← 增参：传入该标的 Fundamental
      ├─ 既有：盖 Metadata["name"]
      └─ 新增：若 fundamental != nil && fundamental.PEPercentile >= 0，
               给每条信号盖 Metadata["pe_percentile_display"] = PEPercentile
  router.Route(...) / FlushNotifications → telegram.SendBatch
```

### 组件改动

**1. `internal/app/app.go`**
- `enrichSignalMetadata(signals []core.Signal, item WatchlistItem)` → 增参 `fundamental *core.Fundamental`（或 `pePctl float64`；选 `*core.Fundamental` 以备将来扩展，nil 安全）。
- 函数内：`name` 逻辑不变；新增——`fundamental != nil && fundamental.PEPercentile >= 0` 时，对每条 signal `Metadata["pe_percentile_display"] = fundamental.PEPercentile`（不覆盖既有同键，与 name 一致风格）。
- 调用点（行 503）传入 `analysisCtx.Fundamental`。

**2. `internal/notifier/telegram/telegram.go`**
- `renderTable` 表头由 `["SYMBOL","NAME","CONF","PRICE"]` 改为 `["SYMBOL","NAME","CONF","PRICE","PE_PCT"]`。
- 新列取 `s.Metadata["pe_percentile_display"].(float64)`；存在则 `fmt.Sprintf("%.1f%%", v)`，否则空字符串。
- 列顺序：**末列**（PE_PCT 成为最后一列，沿用「末列不补尾空格」规则）。
- 既有列宽自适应与 CJK 对齐逻辑不变（width.go displayWidth/padRight 复用）。

### 展示键约定
- 键名：`Metadata["pe_percentile_display"]`，类型 `float64`，值域 0–100，语义「当前 PE 的历史百分位」。
- 仅供 telegram 渲染读取；**router 不读、其它 notifier 不依赖**。

### 边界 / 错误处理
- ETF（`buildFundamental` 返回 nil）、金融指数（中证银行/证券公司，配置标注「PE 分位一期不可用」）、未建 Fundamental 的标的 → 不盖键 → 该列留空，不 panic。
- 同一标的两条信号显示相同 PE 百分位（同标的同值），一致。
- `pe_percentile_display` 缺失或类型非 float64 → renderTable 留空（类型断言双返回值）。

## 测试（TDD，done_criteria 驱动）

**app 包**
- `enrichSignalMetadata`：fundamental 有 PE（PEPercentile=62.3）→ 每条 signal 带 `pe_percentile_display=62.3`；PEPercentile=-1 → 不带键；fundamental=nil → 不带键；name 逻辑零回归。

**telegram 包**
- `renderTable`/`formatBatch`：信号带 `pe_percentile_display` → 输出含 `PE_PCT` 表头与对应 `xx.x%`，列对齐（含 CJK 行）；缺该键 → 该格为空、表格结构不破；末列无尾随补空格。
- 既有 `TestFormatBatch_*` / digest 测试零回归（仅表头/列数变化需同步更新断言）。

**router 包**
- 回归：带 `pe_percentile_display` 的信号不改变 `percentileOf` / 门控行为（该键不被 router 读取）。

## 非功能
- 变更函数（enrichSignalMetadata / renderTable）覆盖率 ≥ 80%；app/telegram/router 包测试零回归（沿用前序 sprint 的「变更文件覆盖率」基准）。
- 无新依赖。

## 范围边界（不做）
- 不做 ROE 列（无数据源）。
- 不做 PE 原始数值列（只做百分位）。
- 不做同标的去重 / 跨策略合并。
- 不改 router 门控、不改 email/webhook 格式。
