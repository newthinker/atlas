# 需求分析 — atlas watchlist 指标命令

> Sprint: 018（2026-07-03 启动）
> 需求来源: `docs/superpowers/plans/2026-07-03-watchlist-metrics-command.md`（实施计划，已 self-review）
> 上游 spec: `docs/superpowers/specs/2026-07-03-watchlist-metrics-command-design.md`
> ECC 不可用 → 降级；计划已按 superpowers writing-plans 产出并自审，不重复 brainstorming。

## 1. 目标

新增 `atlas watchlist [--json] [--symbols A,B]` 命令：离线（无需 serve 在跑）输出 watchlist
全部标的的行情/估值/百分位指标（CJK 对齐表格或 JSON）。装配与分析循环口径同源。

## 2. 模块识别（计划已拆 4 Task，直接映射）

| Task | 内容 | 复杂度 | 涉及包 |
|---|---|---|---|
| T1 internal/text 提取 | telegram 的 CJK 宽度函数迁移为共享包（DisplayWidth/PadRight 导出） | 简单 | internal/text（新）+ internal/notifier/telegram |
| T2 App.SnapshotMetrics | 只读指标组装：复用 orderedCollectors/buildFundamental；新增 FundamentalSource 接口 + SetFundamentalSource；errgroup 并发 + per-symbol panic 隔离 | 中等 | internal/app |
| T3 buildCollectors 提取 | serve.go:99-170 装配段原样迁出为共享函数；新增 lixinger 注入 FundamentalSource + qlib 句柄 cleanup | 中等 | cmd/atlas |
| T4 watchlist 命令 | cobra 命令 + 表格/JSON 渲染 + --symbols 校验 + 退出码语义；stdout 只出数据、日志走 stderr | 中等 | cmd/atlas |

## 3. 代码事实核实（2026-07-03，Sprint 017 合并后复核）

- `internal/notifier/telegram/width.go` + `width_test.go` 存在；telegram.go:198 `displayWidth`、:210 `padRight` ✓
- `internal/app/app.go`: `valuationLookback`（:67，默认 5）、`SetValuationLookback`（:176）✓
- `cmd/atlas/export_ohlcv.go:283 loadConfigOrDefaults` ✓
- serve.go 采集器装配段 ~:99-170（cache 设置→`SetValuationLookback(cfg.Valuation.LookbackYears)`）——
  **Sprint 017 改动（buildSignalStore/alert runner）在其后，行号未漂移** ✓
- 计划声明的关键事实（buildFundamental 只组装 PEPercentile、估值三项唯一来源 lixinger.FetchFundamental）
  由计划作者核实，Dev 实施时以源码为准。

## 4. 依赖与交付组织

- T1 ∥ T2（包不相交，wave1 并行）→ T3（依赖 T2 的 SetFundamentalSource，wave2）→ T4（依赖 T1/T2/T3，wave3）。
- **单 PR 交付**（一条分支 `feature/watchlist-metrics-command`），提交格式 `<type>(watchlist-cmd): <描述>`。
- 全局约束（计划 Global Constraints）：离线测试全绿、零新增第三方依赖、不改现有依赖版本、
  gitnexus_impact/detect_changes、code-simplifier 提交前检查。

## 5. 边界（YAGNI，防越界）

- spec §6 的 YAGNI 项不引入；JSON 歧义已由计划裁定（数组 + gaps 内嵌每标的对象）。
- 不动分析循环本体；T3 是纯迁移重构（行为零变化）+ 两处新增（FundamentalSource 注入、cleanup）。
