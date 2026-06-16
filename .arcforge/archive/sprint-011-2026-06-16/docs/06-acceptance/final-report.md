# 终验收报告 — Qlib 数据仓库 第一期

> Sprint: qlib-data-warehouse-phase1 ｜ 完成日期: 2026-06-16
> 需求: `docs/superpowers/plans/2026-06-15-qlib-data-warehouse-phase1.md`

## 1. 交付概述
建立本地 SQLite 历史行情仓库，atlas `FetchHistory` 以仓库为权威主源、外部 API 仅补新鲜尾巴，完全可降级（缺库/关库零回归）。Python dump 管线（仅 stdlib）+ Go qlib collector + selector 路由 + serve 装配，全链路落地。

## 2. 任务完成情况（13/13 accepted）

| Task | 标题 | owner | rework | verifier |
|---|---|---|---|---|
| T1 | SQLite schema (Py) | dev-agent-1 | 0 | test-agent-1 |
| T2 | CSV 摄取归一化 (Py) | dev-agent-1 | 1 | test-agent-1 |
| T3 | 原子 SQLite 写入器 (Py) | dev-agent-1 | 0 | test-agent-1 |
| T4 | build CLI + Makefile (Py) | dev-agent-1 | 1 | test-agent-1 |
| T5 | 驱动+骨架+Covers (Go) | dev-agent-2 | 0 | test-agent-2 |
| T6 | FetchHistory 仓库读 (Go) | dev-agent-2 | 2 | test-agent-2 |
| T7 | 补尾+降级+陈旧度 (Go) | dev-agent-2 | 0 | test-agent-2 |
| T8 | Quote/非日频委托 (Go) | dev-agent-2 | 1 | test-agent-2 |
| T9 | selector 优先 qlib (Go) | dev-agent-3 | 0 | test-agent-2 |
| T10 | QlibConfig (Go) | dev-agent-3 | 0 | test-agent-2 |
| T11 | App.CollectorRegistry (Go) | dev-agent-4 | 0 | test-agent-2 |
| T12 | serve.go 装配+装配单测 (Go) | dev-agent-4 | 0 | test-agent-2 |
| T13 | symbol_detail API 降级 (QA#2) | dev-agent-3 | 0 | test-agent-2 |

T13 为 QA review 派生任务。计划原 Task10（3 package）按 Realistic Scope 拆为 T10/T11/T12。

## 3. 测试与覆盖率

**Python**: `pytest scripts/qlib_warehouse/` → **12 passed**。
**Go**: `go test ./... -count=1` → **全仓零 FAIL**；`go build ./...` 通过。

| 包 | 覆盖率 | 门禁(80%) |
|---|---|---|
| internal/collector/qlib | 95.6% | ✅ |
| internal/collector | 98.2% | ✅ |
| internal/config | 93.9% | ✅ |
| internal/app | 96.4% | ✅ |
| cmd/atlas | 62.8% | ⚠ 见披露 |
| internal/api/handler/api | 38.5% | ⚠ 见披露 |

**覆盖率诚实披露**：核心特性包全部 ≥93%。`cmd/atlas`(62.8%) 与 `api/handler/api`(38.5%) 在**包级**低于 80%，原因是这两个包含大量**既有未测代码**（serve.go 主流程、indicator 计算 handler 等，本 sprint 之前即无单测）。本期**新增/改动代码**均充分覆盖：`wireQlibWarehouse` 四降级分支各有专测（T12），`fetchHistoryWithFallback` 有 5 条测试（T13，含降级/边界/非 qlib 不触发）。将这两个包整体提到 80% 需为既有未测代码补大量测试，超出本期范围，登记为后续技术债。

## 4. Code Review 结果

两轮审查（常规 + 跨视角对抗，codex/gemini 不可用 → 纯 Claude 攻击者/维护者/SRE 三视角降级）。
- 初始 verdict: **CONTESTED**（无 CRITICAL，3 条叠加 WARNING）。
- 人类裁决「三条全修」，review_fix 一轮闭环：
  - **#1** readRange 对 NULL 数值列崩溃 → 改 `sql.NullFloat64/NullInt64` 扫描（NULL→0）+ 专测。
  - **#3** Covers 与 lastDate 判定口径不一致 → Covers 复用 lastDate（校验 last_date 可解析）、解析失败记 Warn + 专测。
  - **#2** symbol_detail API 对 qlib 错误无降级（HTTP 500）→ 新增 `fetchHistoryWithFallback`，qlib 报错时回落 `SelectExternalForSymbol` + 5 测试。
- 修复后全仓零回归，**CONTESTED 解除**。

被对抗审查加强确认的硬约束：防补尾死循环（selector 永不返 qlib）、缺库/关库零回归（Ping 失败不注册+关库无泄漏）、tail-fill 失败可降级、os.replace 原子写。

## 5. 设计硬约束达成

- ✅ 仓库主源 + 外部补尾（区间 `[start,min(end,last_date)]` + `external.FetchHistory(last+1d,end)`）
- ✅ 完全可降级：缺库跳过注册、补尾失败返回仓库段、陈旧仅告警、NULL 列降级为 0
- ✅ 零回归：所有既有测试全绿；`qlib.enabled=false` 不装配，行为与现状一致
- ✅ selector 永不递归回 qlib（GetAll 兜底跳过 + 仅注册 qlib 返回 nil 专测）
- ✅ 原子写：临时库 + `os.replace`，无半成品库

## 6. 交付物
- Python: `scripts/qlib_warehouse/{schema,ingest,writer,build_warehouse}.py` + tests
- Go: `internal/collector/qlib/`、`internal/collector/selector.go`、`internal/config/config.go`、`internal/app/app.go`、`cmd/atlas/{serve.go,qlib_wiring.go}`、`internal/api/handler/api/symbol_detail.go`
- Makefile `warehouse-dump` target；产物 `data/qlib_warehouse.db`（1.9M，20505 行 / 15 symbol）
- 依赖 `modernc.org/sqlite v1.38.2`（纯 Go，go1.24 兼容）

## 7. 范围边界（本期不做，留第二期）
Part B PIT 基本面源（`fundamentals_pit` 建空不填）；A 股/港股 dump（Makefile 仅接 US）；实时/分钟频入库（始终委托外部）。

## 8. 后续技术债（建议登记 phase-2）
1. `cmd/atlas` / `internal/api/handler/api` 既有未测代码补测以达包级 80%。
2. writer.py tmp 名加 pid/随机后缀以支持未来并发 dump。
3. ingest 单文件解析异常加文件名/行号上下文。
4. GitNexus 索引 stale，建议 `npx gitnexus analyze` 刷新。
