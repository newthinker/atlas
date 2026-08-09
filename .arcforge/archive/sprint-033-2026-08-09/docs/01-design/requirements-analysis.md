# M1b-1 `internal/hestia` types + store —— 需求分析

**需求源**：`hestia/docs/superpowers/plans/2026-08-08-hestia-store.md`（1487 行，含 Self-Review 与 Spec 覆盖矩阵）
**交付位置**：atlas 仓 `internal/hestia/`（需求文档在 hestia 仓，代码在 atlas——与 M1a 同一形态）

## 目标

54 个字段常量、`Observation`/`ValidationReport` 类型、由字段清单生成的 DDL、唯一写入口 `Store.Save`。

**架构要点**：业务字段用 `map[string]float64`，键不存在即字段缺失；54 个字段名只写一次（`fieldOrder`），DDL/INSERT/白名单全部从它派生；`Save` 是唯一写入口，签名强制要求 `ValidationReport`；当前行由 `published_at` 派生，无 `is_current` 列，故写入是普通 INSERT。

## 任务依赖图

```
T1 fields.go ──┬──→ T3 schema.go ──→ T4 store.go(NewStore) ──→ T5 Save+insert ──→ T6 refresh+pending ──→ T7 收尾验证
T2 types.go ───┴──────────────────────────────────────────────↗
```

| Task | 文件 | wave | 依赖 |
|---|---|---|---|
| T1 字段常量与唯一真相源 | `fields.go` | 1 | — |
| T2 类型与 `Meta` 校验 | `types.go` | 1 | — |
| T3 由字段清单生成 DDL | `schema.go` | 2 | T1 |
| T4 `NewStore` 与建表 | `store.go` | 3 | T3 |
| T5 `Save` 输入校验与 INSERT | `store.go` | 4 | T4, T2, T1 |
| T6 `Duplicate` UPDATE 与 pending 分流 | `store.go` | 5 | T5 |
| T7 收尾验证（无生产代码） | — | 6 | T1–T6 |

**并行度低是结构性的**：T4/T5/T6 都写 `store.go` 同一文件，scope 互斥使它们必须串行。真正能并行的只有 T1‖T2。

⇒ **团队规模定为 2 dev + 1 test**，不用满配 4 dev——多派的 dev 会在 wave 2 之后全程空转，而空转的 teammate 会因 `TeammateIdle` 一次性放行而停机（sprint-032 教训）。

## 与需求文档的两处偏差（已核实）

**① 分支指令已过时。** 文档第 29–35 行要求从 `feat/macro-bitemporal` 拉分支，理由是「M1a 尚未合并 master」。**M1a 已于今日合并**（atlas PR#53 → `a03f0a6`）。本 Sprint 分支 `feat/hestia-store` 从 `origin/master`（`d7c9c69`）拉出。

连带影响：文档中 `git diff feat/macro-bitemporal --stat go.mod go.sum`（T7 Step 2）与 `git log --oneline feat/macro-bitemporal..HEAD`（T7 Step 6）两条命令的基准应改为 `master`。

**② M1a 导出面已用 `go doc` 核实可用**：`Lookup(ctx, q, spec, key) (State, error)`、`Classify(s State, incoming string) Verdict`、`NewSpec`、`CurrentQuery`/`AsOfQuery`。文档 Self-Review 第 3 条声称「已用 go doc 核对」，复核成立。

## 风险点

**R1（最高）——`Meta` 七字段三处同序。** 文档 Self-Review 自己点出：`Meta` 结构体字段顺序（T2）、`metaColumns`（T3）、`insert` 取值顺序（T5）**必须一致，否则数据错位写入且不报错**。这是本 Sprint 唯一「静默产生错误数据」的路径，DoD 必须有针对性判据，不能只测「能写进去」。

**R2 —— `savePending` 桩是有意的中间状态。** T5 Step 3 故意留一个返回 "not implemented yet" 的桩，T6 替换。文档已明确标注「不是占位符」。验证 T5 时**不得据此判 REJECT**；但 T6 必须验证桩确实被替换（而非仍在返回该错误）。

**R3 —— 单位不入库。** `Values` 的数值已归一（余额=万亿元、增量=亿元、比率=百分数），无 `units_version` 列。改单位是 breaking change。DoD 不为此设列，但注释须写明。

**R4 —— 公开面必须无绕过校验的写入口。** 不得出现 `Insert`/`Upsert`/`SaveUnchecked` 或任何不要求 `ValidationReport` 的导出写方法。T7 用 `go doc` 核验。

## 本 Sprint 兼作运行时验证载体

sprint-032 同步的 hooks 有三项能力**从未被真实触发验证**：权限矩阵角色校验、`task-completed.sh` 覆盖率门禁、`teammate-idle.sh` 保活。本 Sprint 是它们的首次真实运行，异常须单独记录，不要混进业务问题里。
