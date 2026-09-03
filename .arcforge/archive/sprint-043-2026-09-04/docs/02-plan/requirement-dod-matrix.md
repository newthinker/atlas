# 需求 ↔ DoD 双向追溯矩阵 · Sprint M1d（Sprint 043）

**采样锚**：`ae088eb253b64b36e10558a02587e3fa657f5f3e`
**需求源**：`hestia/docs/superpowers/plans/2026-09-03-hestia-m1d-cutover.md`（11 个 Task + Global Constraints + 8 条交付前清单）
**本 Sprint 射程**：需求 TASK-001 ～ 008（AD-1）

## 1. 正向：需求 Task → Arcforge TASK

| 需求 Task | Arcforge | wave | 射程是否逐字一致 |
|---|---|---|---|
| 001 配置 `storage.snapshot_dir` | TASK-001 | 1 | ✓ |
| 002 `saveSnapshot` 与幂等规则 | TASK-002 | 1 | ✓ |
| 003 ingest 接快照 | TASK-003 | 2 | ✓ |
| 004 `Sender` 与三类渲染 | TASK-004 | 1 | ✓ +1 条边界（`Check.Value == nil`） |
| 005 ingest 接通知 | TASK-005 | 3 | ✓（P0 测试的阈值前提标注为未核验，AD-10） |
| 006 `--only-period` | TASK-006（包级 Step 1–3）+ TASK-007（cmd Step 4） | 4 / 5 | ⚠️ **拆分**（AD-2） |
| 007 plist `--config` 与 `buildHestiaSender` | TASK-007 | 5 | ✓（并入 006 的 cmd 部分） |
| 008 收口 | TASK-008 | 6 | ⚠️ **形态改为 docs-only**（AD-5）：code-simplifier 由各任务自跑，008 只终检 |
| 009 运行时切换 | — | — | ❌ 结转：人执行（AD-1） |
| 010 首期增量验收 | — | — | ❌ 结转：时间门控 09-09～09-15（AD-1） |
| 011 文档回写与语料副本 | — | — | ❌ 结转：输入是 009/010 结果（AD-1） |

**孤儿需求检查**：射程内 8/8 有对应任务，无孤儿；射程外 3 条均有 AD-1 裁决与 final-report 交付后待办登记义务。

## 2. 反向：Global Constraints → 落点

| 约束 | 落点 | 载体 |
|---|---|---|
| Go 1.24.4，无新增依赖 | 001–007 | `non_functional` 的 GATE 段（每个任务都写，不靠指针） |
| 五个不动文件 diff 为空 | 001–007 GATE 段；008 functional[1] | 命令给全 |
| 字段名字面量只许在 `fields.go` 与 `_test.go` | 004 non_functional[0]；002 non_functional[0] | 指名 `TestFieldNamesAppearOnlyInFieldsGo` 不改断言 |
| 导出面精确相等、零新增导出函数 | 002 / 004 / 005 / 006 non_functional；008 functional[1] | 指名 `TestPackageExposesNoWriteFunctions` |
| `hestia.go` 不得 import `path/filepath` | 007 error_handling[0] | 指名既有守卫 |
| `Meta` 七字段不动 | GATE 段「不碰 `types.go`」 | — |
| 注释带 milestone 前缀 | GATE 段 | — |
| 每 task gofmt / vet / test 干净；两处既有欠账不修 | GATE 段（判据「两处之外没有新增项」） | — |
| 覆盖率 ≥ 96.3% | GATE 段（零余量、报数带 HEAD sha）；007 单独写 `cmd/atlas` ≥ 75.7 | — |
| 工具只产出依据 | 009/010 结转 | AD-1 |
| 提交前 code-simplifier | DELIVERY 段第 3 步；008 boundary[0] 终检 | AD-5 |
| 测试文件 import 按需增补 | 003 / 004 / 005 non_functional | — |

## 3. 反向：交付前检查清单（8 条）→ 落点

| 清单条目 | 落点 |
|---|---|
| Sprint 043 合并 master，覆盖率 ≥ 96.3%，vet 干净，五个不动文件 diff 空 | 008 functional[1]；合并由 Leader（AD-6） |
| 真语料回归数字与 M1c-4 §B 一个不变 | 008 functional[2]（manual） |
| 运行时切换七步、CONTRACTS §C | 结转（009） |
| 链路实测三件事 | 结转（009 Step 5） |
| 2026-08 月报验收 | 结转（010） |
| 三期判据登记为 M2 前跟踪项 | 结转（010 Step 4） |
| vault 回写、语料副本、§E | 结转（011） |
| 一切自证数字采于最后一次代码改动之后 | 008 functional[0]（前置采锚）+ error_handling[0]；每任务 DELIVERY 第 5 步（merge 后重采） |

## 4. 凭空 DoD 检查（DoD 里有、需求里没有）

| 条目 | 依据 |
|---|---|
| 提交信息锚 `feat(TASK-00N): M1d …`（与需求 `feat(M1d TASK-00N)` 不同） | AD-3：门禁 `task-completed.sh:133` 的 grep 锚 |
| TASK-007 `coverage_floor: 75` | AD-4：`cmd/atlas` 实测 75.7% < 80 |
| TASK-004 `Check.Value == nil` 边界 | 需求 `renderP0` 代码已处理 nil（`val := "n/a"; if c.Value != nil`），原文测试没覆盖；补一条断言不改行为 |
| TASK-005「若 `deposit_sum` 在夹具上 skipped 可换阈值」 | AD-10：Leader 未核验的前提必须带出路 |
| TASK-008「新增测试计数按实际 `git diff` 核对，需求的 29 是预估」 | 需求 §B 模板给的是预填数；M1c-4 教训：写进 CONTRACTS 的数必须实测 |
| 交付流程 7 步（worktree / merge 先于 dev_done / merge 后重采） | AD-6：机制决定 |
| TDD 红阶段输出记进 discovery | config `tdd.require_failing_test_first: true` |

以上每条都有出处，**无真正凭空条目**。

## 5. 机器检查

- 需求 TASK-001～008 每个都至少有一个 Arcforge 任务的 `description` 以「对应需求文档 **TASK-00N**」开头（8/8）。
- 每个 Arcforge 任务 DoD 条数 ≤ 8（7/8/8/8/8/7/7/7），`writes` ≤ 3，`packages` 恰 1。
- validator：8 任务 / 21 条规则通过。
