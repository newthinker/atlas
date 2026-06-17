# 终验收报告 — 第三期 分位「自上市/起始日」全历史回看

> Sprint: percentile-since-inception-phase3 ｜ 完成日期: 2026-06-16
> 需求: `docs/superpowers/plans/2026-06-16-percentile-since-inception-phase3.md`

## 1. 交付概述
price_percentile / pe_percentile 支持 `lookback_years: 0`（自上市/起始日全历史回看），逐标的各按自己最早可得数据算分位，取代固定 5 年窗口。默认仍 5 年（零回归）。

## 2. 任务完成情况（7/7 accepted）

| Task | 标题 | owner | verifier | rework |
|---|---|---|---|---|
| T1 | SinceInceptionBars 常量 | dev-agent-1 | test-agent-1 | 0 |
| T2 | price_percentile lookback:0 | dev-agent-1 | test-agent-1 | 0 |
| T3 | pe_percentile lookback:0 | dev-agent-2 | test-agent-1 | 0 |
| T4 | ValuationConfig + Load 默认 5 | dev-agent-2 | test-agent-1 | 1 (QA C1) |
| T5 | app valuation lookback 可配 + inception | dev-agent-3 | test-agent-2 | 2 (F1 + QA W2/W3) |
| T6 | serve 装配 | dev-agent-1 | test-agent-1 | 0 |
| T7 | 全史 dump + config + 文档 | dev-agent-1 | leader-manual | 1 (QA W1) |

## 3. 测试与覆盖率
- `go build ./... && go test ./internal/... ./cmd/... -count=1` → **全仓零 FAIL**。
- 覆盖率：price_percentile **93.3%** / pe_percentile **95.7%** / config **94.0%** / app **96.2%**（均 ≥80%）。

## 4. Code Review 结果
两轮审查（常规 + 三视角对抗，纯 Claude 降级）。
- 首轮 verdict **REJECT**：1 CRITICAL（C1）+ 3 WARNING（W1/W2/W3）。
- 人类裁决「四项全修」，review_fix 闭环：
  - **C1**（CRITICAL，零回归破坏）：`config.Load()` 未对 valuation.lookback_years 设默认 → 存量配置无 valuation 块时静默切 inception。修：`v.SetDefault("valuation.lookback_years", 5)` + 回归测试。
  - **W1**：example.yaml 两层 lookback 不自洽（开箱误导）→ valuation 示例改 5、inception opt-in 注明需三处同设 0。
  - **W2**：策略 lookback:0 但 valuation:5 时早于最早 EPS 的收盘被静默丢弃 → epsStart 覆盖整个 ohlcv 价格窗口。
  - **W3**：inception start≈1926 → 负 Unix period1 → epochFloor=1970 钳制（消除负值）。
- 二审 verdict **PASS**：四项全部 RESOLVED（QA 独立 throwaway 用例验证），无新隐患。

## 5. 设计硬约束达成
- ✅ **零回归**：默认/未配置 = 5 年（两条生产路径 Load/Defaults 都成立）；const valuationLookbackYears 删除无残留；既有测试全绿。
- ✅ **inception 语义**：lookback:0 → SinceInceptionBars(100*252) 窗口 → 逐标的全史；minSampleBars=252 兜底防新股（price 直接、pe 经 Fundamental nil）；负数仍拒绝。
- ✅ **N2 不变量**：PE 分位窗口由 ohlcv 决定、EPS floor 多取不改分位（TestReconstructPercentileUnaffectedByEPSOverfetch）。
- ✅ **W2/W3 修复**：EPS 覆盖价格窗口（无静默截断）；inception 起点钳 1970（无负 period1）。
- ✅ **lixinger y10 上限**：lixingerLookback() 0→10，封顶在 lixinger 内部桶；文档诚实标注。

## 6. 端到端实跑验证（实际运行 atlas）
配 `lookback_years: 0` 启动 serve（5 年仓库数据），触发 analysis，AAPL 信号：
```
Reason: "price at 98.6% of full history (1368 bars)"
Metadata: lookback_years=0, sample_size=1368, percentile=98.6
```
证明 inception 模式消费全部 1368 根可得 bar（未被 5×252=1260 截断），Reason 显示「full history」。

## 7. 交付物
- Go: `internal/strategy/interface.go`（SinceInceptionBars）、`price_percentile`/`pe_percentile`（lookback:0）、`internal/config`（ValuationConfig + Load 默认）、`internal/app`（valuationLookback 字段/setter/epsFetchStart/lixingerLookback/epochFloor 钳制/EPS 覆盖 ohlcv）、`cmd/atlas/serve.go`（装配）
- 文档/构建：Makefile（WAREHOUSE_FROM）、configs/config.example.yaml（自洽示例 + lixinger 上限）、ADAPTERS.md（Lookback modes）
- 11 commit（66cd6e1..89fd369）

## 8. 已知约束与待办
- **数据前提**：全史回看需重 dump 全史数据（`export-ohlcv --from 1970`）；当前仓库仅 5 年。
- **既有 bug（非本期）**：`export-ohlcv` 在 `yahoo.FetchHistory`(yahoo.go:229) 有 nil-deref panic（standalone 路径），阻碍全史 dump——phase-3 未碰 yahoo.go，建议后续单独修。
- **lixinger 10Y 上限**：A 股个股 + 所有指数 PE 分位 inception 等价「最多 10 年」（外部 API 限制，诚实标注）。
- **QA 可选改进（I3/I4/I5，非阻断）**：PE inception reason 补 bar 数跨度；SinceInceptionBars 与 epochFloor 常量交叉引用；Makefile WAREHOUSE_FROM 收敛进文档。

## 9. 范围边界（本期不做）
不绕过 lixinger y10；不查精确 IPO 日（用早 floor + 数据源裁剪）；全史数据实际生产 best-effort；不改 minSampleBars。
