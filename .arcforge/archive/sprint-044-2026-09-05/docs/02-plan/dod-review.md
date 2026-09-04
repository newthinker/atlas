# 独立 DoD 反审 · Sprint M1.5（Sprint 044）

**reviewer**：Agent tool 只读子代理（未命名，非 teammate）；先只读需求与 spec 写出自己的验收要点并在 `037d1eb` 上核实前提，再读 9 个任务文件比对
**判定**：NEEDS WORK —— 3 条阻断 + 8 条建议 + 备注
**Leader 处置**：每条打开它给的文件:行号核实后**全部采纳**（S3 采「保留 AD-5 + 记明替代方案」）；DoD 已经写通道 `update` 落盘（7 条审计行），validator 重跑通过。处置表见 `01-design/architecture-decisions.md` AD-15。

## 阻断（3，全部核实成立）

| # | 任务 · 维度 | 问题 | 修法（已落盘） | 证据 |
|---|---|---|---|---|
| B1 | 001 functional[0] | `openWithSchema`（`schema_test.go:47-62`）与 `TestDDLIsIdempotent`（`:248-257`）只执行三段 DDL，`hestia_runs` 不建 ⇒ `TestRunsStructureFromLiveDB` 必红在「表不存在」；「`TestDDLIsIdempotent` 仍绿」恒真 | 两处 DDL 列表 `append(…, runsDDL()...)`；`TestDDLIsIdempotent` 加 `tableInfo(TableRuns)` 前后相等；`TestNewStoreCreatesSchemaIdempotently` 名单加 `TableRuns` | Leader 打开三处核实 |
| B2 | 002 error_handling[0]；008 §B；矩阵 §3 | 需求「`RecordRun` 失败不影响已入库行无法构造」为**假**：`ingest_test.go:521-530` 的 `Save 失败` 用例正是用 `rawDB` 建表级 `BEFORE INSERT` 触发器只挡一张表 | 必写 `TestIngestRunRecordFailureKeepsIngestedRow`（触发器打在 `hestia_runs`；断言 `record run` 错误、`HasPeriod` 仍 true、Verdict 行仍打印、runs 0 行、无补心跳）+ 零候选变体；008 §B 该行改「已测」 | Leader 核实触发器用例存在 |
| B3 | 007 `writes` + functional[2] | 需求 Files 明写 `cmd/atlas/hestia_test.go`，`writes` 漏了；既有 `TestHestiaStatusOnEmptyStore`（`hestia_test.go:224-242`）只断言两行计数，dev 传 `nil` 也绿 | `writes`/`estimated_files` 加该文件；补 `runs: 0` 断言；新增写 6 行后输出 `runs: 5` 的测试 | Leader 核实 |

## 建议（8，全部采纳）

| # | 任务 | 缺口 | 处置 |
|---|---|---|---|
| S1 | AD-14 / 008 A7 | 「既有测试可能断言 URL」未成立（grep 0 命中）；A7 漏 `has article`；AD-14 末句与 008 DoD 矛盾 | AD-14 理由订正；A7 条目列全 |
| S2 | 002 functional[1] | 需求原文 `recorded == 0` 触发心跳：处理过但 `RecordRun` 失败会再补一行 `no_new` | 改计 `processed == 0` |
| S3 | AD-5 | 有零成本替代（`Save(obs, failing())` + 手工 `RecordRun` 造 pending，003 可进 wave 2） | **保留 AD-5**（要「经真实 Ingest」的集成证据；wave 2 已有三个并行任务，再加一个超出 dev × 3 的吞吐），AD-5 记明替代方案已知、刻意不取 |
| S4 | 006 | `config.Load` 装载整份示例 yaml 未核验；`DisabledWhenUnset` 用 `NewNop` 断不到「日志一行」 | 加出路（其他段失败 ⇒ 澄清环）；换 `zaptest/observer`（本包已在用） |
| S5 | 001 | `FinishedAt` 零值回落无测试；结构测试不钉 `NOT NULL` | 加子例；五列 `notNull` 断言 |
| S6 | 003 | 需求测试代码一行两语句不过 gofmt | DoD 提醒先 gofmt |
| S7 | 004 | 出错分支应同时断言 `hestia_runs_total` 缺席 | 加一行 |
| S8 | 008 | 真语料回归标 `manual` 但可机器复跑 | 改 `review` |

## 备注

- 003/004/005/006 的变异自证与各任务 TDD 红阶段留痕只存在于 discovery 文本，验证者需在隔离副本重跑。
- AD-10(a)「最小 hestia.yaml 能过 `LoadConfig`」reviewer 已核验为**真**（`cmd/atlas/hestia_test.go:224-232` 同形配置通过），撤销「未跑过」标记。
- 前提核验表：reviewer 逐条对照 `037d1eb`，与 Leader `requirements-analysis.md` §5 一致，另证伪一条（「无法构造」，见 B2）。

**未采纳**：无。
