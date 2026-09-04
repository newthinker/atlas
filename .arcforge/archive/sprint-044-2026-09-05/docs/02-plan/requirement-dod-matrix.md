# 需求 ↔ DoD 双向追溯矩阵 · Sprint M1.5（Sprint 044）

**采样锚**：`037d1eb1e4f827c415319519e40f4e2208968920`
**需求源**：`hestia/docs/superpowers/plans/2026-09-04-hestia-m1.5-health.md`（9 个 Task + Global Constraints + 9 条交付前清单）；spec `specs/2026-09-04-hestia-m1.5-health-design.md` §8 测试表、§11 交付判据
**本 Sprint 射程**：需求 TASK-001 ～ 008（AD-1）

## 1. 正向：需求 Task → Arcforge TASK

| 需求 Task | Arcforge | wave | 射程是否逐字一致 |
|---|---|---|---|
| 001 `hestia_runs` / `Run` / `RecordRun` / `RecentRuns` / 守卫 | TASK-001 | 1 | ✓ +1 边界（`store.go` 只新增的 `-` 行判据） |
| 002 `Ingest` 写运行表 | TASK-002 | 2 | ✓ +1 边界（spec §8「`--only-period` 过滤掉的不记行」需求测试未覆盖，补 1 条）；AD-13 心跳射程、AD-14 stage 文案 |
| 003 `HealthSummary` | TASK-003 | 3 | ✓；⚠️ **依赖 002**（AD-5，需求说可并行） |
| 004 `metrics.HestiaCollector` | TASK-004 | 4 | ✓ +边界（nil 族不解引用、`now` 注入变异） |
| 005 告警按规则冷却期 | TASK-005（`internal/alert`）+ **TASK-010**（`internal/config` + `cmd/atlas`） | 1 / 2 | ⚠️ **拆分**（AD-2） |
| 006 `serve` 接线与主配置 | TASK-010（`HestiaConfig`/`Config.Hestia`）+ TASK-006（其余） | 2 / 5 | ⚠️ **`HestiaConfig` 移入 010**（AD-2）；006 +1 边界（样例配置装载测试） |
| 007 `hestia status` 的 `runs` 段 | TASK-007 | 2 | ✓（调用点实数 5+1）；`writes` 补 `cmd/atlas/hestia_test.go`（reviewer B3） |
| 008 收口 | TASK-008 | 6 | ⚠️ **形态改为 docs-only**（AD-11）；Step 6 的合并/归档是 Leader 动作；§A 追加 A7 |
| 009 投递与验收 | — | — | ❌ 结转：人执行、前置 M1d §G 未登记、窗口 09-09～09-15（AD-1） |

**孤儿需求检查**：射程内 8/8 有对应任务，无孤儿；射程外 1 条（009）有 AD-1 裁决与 final-report 登记义务。

## 2. 反向：Global Constraints → 落点

| 约束 | 落点 | 载体 |
|---|---|---|
| Go 1.24.4，无新增依赖 | 001–007、010 | `non_functional` GATE 段（每任务都写，不靠指针） |
| 四个不动文件 diff 为空；`store.go` 只新增、`Save` 不动 | 001 boundary[0]；每任务 GATE；008 functional[1] | 命令给全 |
| 两条写口守卫精确集合 + 登记理由注释 | 001 functional[3]（12 / 24）；003 functional[2]（25） | 精确项数 |
| 业务字段名字面量只在 `fields.go`/`_test.go`；`health.go` 不引 `fieldOrder` | 003 functional[2] | `TestFieldNamesAppearOnlyInFieldsGo` |
| `cmd/atlas/hestia.go` 不 import `path/filepath` | 007 functional[2]；006 boundary[0] | `TestHestiaCmdDoesNotResolveDBPath` |
| 注释任务号带 milestone 前缀 | 每任务 GATE 末句 | — |
| gofmt / vet / 五包测试 | 每任务 GATE（**口径订正 AD-8**：五包、三处欠账） | — |
| `internal/hestia` ≥ 96.3% | 每任务 GATE；008 functional[1] | 基线 96.5% |
| 工具只产出依据 | AD-1（009 结转）；006 只改样例 yaml | — |
| 提交前 code-simplifier | 每任务 DELIVERY 第 3 步；008 boundary[0] 终检 | — |
| 测试 import 按需增补 | 每任务 | — |
| 🔴 采锚前置 | 008 functional[0]（**口径订正 AD-7**） | — |

## 3. 反向：spec §8 测试表 → 落点

| spec §8 行 | 落点 |
|---|---|
| schema_test：DDL 建表；列集精确相等 | 001 functional[0] |
| store_test：读回逐字段相等；非法 Outcome 拒绝；守卫清单 | 001 functional[1]/[2]/[3] |
| health_test：空表零值；duplicate 不推进；pending 推进；GROUP BY；NotifyFailures 只数非空 | 003 functional[1] |
| ingest_test：空跑 no_new；入库/pending/Duplicate/失败各记；通知失败 notify_error 且 notified=0；**`--only-period` 过滤掉的不记行**；**`RecordRun` 失败不影响已入库行** | 002 functional[2]；002 boundary[0]（补测）；002 error_handling[0]（**必测**：表级触发器，reviewer B2 证伪了需求「无法构造」） |
| metrics：指标名与值；空表不输出四个时间戳；出错只输出 collect_errors | 004 functional[0]/[1] |
| alert：按规则 cooldown；未写退回全局 | 005 functional[1] |
| serve（cmd）：config_path 缺 ⇒ 不注册且日志一行；装不上 ⇒ 返回错误 | 006 functional[1] |
| 覆盖率 ≥ 96.3% | 每任务 GATE |

## 4. 反向：需求「交付前检查清单」9 条 → 落点

| # | 清单 | 落点 |
|---|---|---|
| 1 | 五包测试绿、hestia ≥ 96.3%、vet 干净 | 008 functional[1] |
| 2 | 四个不动文件 diff 空；`Save` 不动 | 008 functional[1] |
| 3 | 两条守卫精确集合；`RecordRun` 登记理由在注释里 | 001 functional[3]；008 functional[1] |
| 4 | 真语料回归数字一个不变 | 008 functional[2] |
| 5 | alert 既有两条规则行为不变 | 005 functional[2] |
| 6 | 投递时刻晚于首期验收登记 | 结转（009） |
| 7 | 三条验收全过，Telegram 有时间戳 | 结转（009） |
| 8 | 挂账 C2 第二半销账；vault 回写 | 结转（009）；007 只交付功能 |
| 9 | 自证数字采于最后一次改动之后 | 008 error_handling[0]；每任务 DELIVERY 第 5 步 |

## 5. 凭空 DoD 检查（DoD 条目不对应任何需求/spec 的）

| 条目 | 依据 |
|---|---|
| 002 boundary[0] only-period 测试 | spec §8 明写 |
| 002 boundary[1] 既有测试 `-` 行 ≤ 3 | 需求「其余行不动」+ 实测 3 处直接调用 |
| 003 boundary[0] 变异自证 | 验证者 Reality Checker 判据前置到 dev（M1d 先例） |
| 004 boundary[0] nil 族 / `now` 注入 | 需求代码本身的性质，防假绿 |
| 005 boundary[0] 两个变异 | 同上 |
| 006 boundary[0] 样例配置装载测试 | 需求 Step 4 改了 yaml 却无测试守着；实测本仓库无加载示例配置的测试 |
| 006 error_handling[0] `hestia health:` 前缀断言 | 需求 TASK-009 Step 2 依赖该前缀 grep err.log |
| 007 boundary[0] `%-9s`/`UTC()` | 需求断言的字面空格数是格式宽度的结果，防「调宽度凑断言」 |
| 010 functional[2] config 解码测试 | 需求 005 Step 3 只说「viper 解析 `24h` 字符串」，无测试；`HestiaConfig` 需求 006 也无解码测试 |
| 全部 DELIVERY 段 | 本仓库机制（AD-3/AD-6），不对应需求 |

| 001 S5 / 002 B2·S2 / 004 S7 / 006 S4 / 007 B3 | reviewer 反审补入（`02-plan/dod-review.md`），各有 spec §8/§9 或需求 Files 出处 |

结论：无凭空 DoD——每条都有需求、spec 或机制出处。
