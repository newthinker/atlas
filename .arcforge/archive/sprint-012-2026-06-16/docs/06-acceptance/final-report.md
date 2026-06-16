# 终验收报告 — Qlib 数据仓库 第二期（Part B PIT 基本面）

> Sprint: qlib-data-warehouse-phase2 ｜ 完成日期: 2026-06-16
> 需求: `docs/superpowers/plans/2026-06-15-qlib-data-warehouse-phase2.md`

## 1. 交付概述
用本地 SQLite 仓库的 **PIT 双轴基本面**（report_period × observe_date）为 atlas 提供**消除前视偏差**的 EPS(TTM) 历史，作为 PE 分位重建权威主源，Yahoo/lixinger 退兜底。Python dump 管线扩展（fundamentals_pit 同次原子写）+ Go qlibpit EPS 源 + serve 装配。

## 2. 任务完成情况（7/7 accepted，零返工一次过）

| Task | 标题 | owner | verifier | rework |
|---|---|---|---|---|
| T1 | 基本面 CSV 摄取 + 空 eps_ttm 跳过 (Py) | dev-agent-1 | test-agent-1 | 0 |
| T2 | writer 同次原子写 fundamentals_pit (Py) | dev-agent-1 | test-agent-1 | 0 |
| T3 | build CLI --fundamentals-dir (Py) | dev-agent-1 | test-agent-1 | 0 |
| T4 | qlibpit EPS 源 PIT 查询 (Go) | dev-agent-2 | test-agent-2 | 0 |
| T5 | qlibpit 兜底委托测试 (Go) | dev-agent-2 | test-agent-2 | 0 |
| T6 | serve 装配 qlibpit (Go) | dev-agent-3 | test-agent-2 | 0 |
| T7 | 适配器 ADAPTERS.md + Makefile (best-effort) | dev-agent-1 | test-agent-1 | 0 |

## 3. 测试与覆盖率
- **Python**: `pytest scripts/qlib_warehouse/` → **19 passed**（含第一期全部，零回归）。
- **Go**: `go test ./internal/... ./cmd/... -count=1` → **全仓零 FAIL**；`go build ./...` 通过。
- 覆盖率：`internal/collector/qlibpit` **87.5%**（≥80%）；`cmd/atlas` 62.4%（包级，受既有未测 serve/handler 代码拉低，本期新增 wiring 四分支单测充分覆盖——同第一期披露）。

## 4. Code Review 结果
两轮审查（常规 + 三视角对抗，codex/gemini 不可用→纯 Claude 降级）。verdict **PASS**（无 CRITICAL、无 WARNING）。第二轮各 lens 提出的「CRITICAL」（db 泄漏 / start 忽略 / NULL 双防 / KeyError / 兼容断言缺失）经下游消费代码（reconstruct.go、app.go、既有 qlib.go/ingest.go）逐条复核后**全部推翻或降级为 INFO**——属既有约定延续、契约外理论顾虑、或被下游语义吸收。

## 5. 设计硬约束达成
- ✅ **PIT 防前视**：`observe_date <= end` 截断；`==end` 含入有专测钉死（off-by-one 防护）
- ✅ **修订语义**：同 report_period 多 observe_date 按升序保留不去重
- ✅ **降级三态**：仓库无符号→委托 yahoo；仓库有→优先仓库不调 fallback；db==nil→EPS 源纯 yahoo 零回归
- ✅ **装配不变量**：wireQlibWarehouse 单次开库（同一 *sql.DB 既注册 qlib collector 又喂 qlibpit）；SetValuationSources 在 Start 前；qlibpit fallback 不递归；collector 注册在外部源之后
- ✅ **原子写**：fundamentals 与 ohlcv 同一临时库 + os.replace；向后兼容（writer/build/serve 三场景与第一期一致）
- ✅ **数据质量**：必填 eps_ttm 空值行 ingest 跳过（人类确认门决策）

## 6. 交付物
- Python: `scripts/qlib_warehouse/fundamentals.py`（新）、`writer.py`/`build_warehouse.py`（改，向后兼容）+ tests
- Go: `internal/collector/qlibpit/`（新 EPS 源）、`cmd/atlas/qlib_wiring.go`（wireQlibWarehouse 返回 (*sql.DB,bool)）、`serve.go`（EPS 源注入）
- 文档/构建: `scripts/qlib_warehouse/ADAPTERS.md`（三市场适配契约）、Makefile（US fundamentals 透传，$(wildcard) 守卫）
- 6 commit（71a0ff4..2fb563a）

## 7. 范围边界（本期不做 / best-effort）
各市场 `fundamentals_csv/` 实际生产为 best-effort 适配器（T7 文档化）；美股 observe_date 为披露滞后近似；PB/PS/ROE 入库但 Go 侧本期仅消费 eps_ttm。

## 8. QA 可选非阻断改进（建议 follow-up，未阻断）
1. qlibpit `hasFundamentals` / `time.Parse` 失败处各加一次性 `log.Warn`（仓库损坏可观测性）。
2. ADAPTERS.md 注明「当前仅消费 eps_ttm，其余列预留」。
3. qlibpit `FetchEPSHistory` 加注释说明 `start` 故意忽略（窗口由下游 ReconstructPEPercentile 处理）。
4. test_build_warehouse 补一行 build 层 fundamentals_pit COUNT=0 兼容断言。
