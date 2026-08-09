# 交接给后续子迭代的已知缺口（M1b-1 产出）

> **本文件的存在理由**：test-agent-21 在验证 TASK-002 时指出——「`types.go` 注释说『留给闸门层』，而闸门层的 DoD 没写 → 没人做。这是**已知缺口变成已遗忘缺口**的典型路径。」
>
> Leader 在消息里口头确认过归属，但消息不是真相源。此文件是。

## H1 — 日历有效性校验：归 **M1b-3（validate）**，本子迭代明示不做

`Meta.validate` 只校形态、不校日历有效性。实测通过的非法取值：`period = "2026-13" / "2026-00" / "9999-99"`、`published_at = "2026-02-31"`。

**为什么形态校验足够堵住 G1 而日历有效性可以推后**（dev-agent-42 论证，Leader 采纳）：G1 三条静默后果的**共同机制都是「长度不定 ⇒ 字典序失效」**，定长补零是充分修复。`2026-13` 是**定长**的，不破坏字典序，只是语义无效——失效机制不同，堵法也该不同。

**落点要求**：M1b-3 的 DoD 必须**显式含**这一条。若届时判断不做，也要显式记为非目标——**不允许它以「上游已经处理了」的形式消失**。

## H2 — 三处同序契约：本 Sprint 已全部闭合 ✅

`Meta` 字段声明顺序在三处必须一致，任一处不一致会**静默写错位数据**（所有列都是 TEXT，错位写入不触发任何数据库错误）。

| 端 | 保护 | 实证 |
|---|---|---|
| `types.go` 的 `Meta` | `TestMetaFieldOrderIsCrossTaskContract`（reflect） | T2 的 M6/M7 |
| `schema.go` 的 `metaColumns` | reflect 与 Meta 字段名逐一相等 | T3 的 M3/V2 |
| `store.go` 的 `insert` 取值 | `TestSaveMetaValuesLandInMatchingColumns` | T5 的 M1/W1 |

**关键证据**（test-agent-21 @ T5）：W1 只改 `insert` 顺序时，**前两条断言全绿**——正是 DoD 预言的那类缺口。三端缺一不可。

## H3 — 【新】G3 数据静默丢失：**已可见，未修复，需单列任务**

**dev-agent-42 在 TASK-006 明确要求裁定，Leader 裁定：单列后续任务，不在 M1b-1 修。**

现状：`Duplicate`（同键同 `published_at`）只刷新 `article_id`，**携带的 `Values` 被丢弃**。这是 DoD `functional[0]` 明文要求的行为，覆盖式更新会直接违反它。

**触发场景是必然会发生的运维动作**：上线 `rule@v2` 后回填重跑历史 ⇒ 每期 `published_at` 都没变 ⇒ 全判 `Duplicate` ⇒ **v2 新抽的字段一个都不写入，`extractor` 列还写着 v1，而 `Save` 返回 nil** ⇒ 运维看到「N 期处理完毕、零错误」。

**本 Sprint 做到的**：把全部可观测后果钉成契约（行数不变 + `article_id` 已刷新 + 列值未变 + `extractor` 未变），由 `TestSaveDuplicateIsDefinedNotSilent` 守住。**丢弃行为现在是一个看得见的决定，不是意外。**

**修它需要设计决策**（给业务键加 `extractor` 维度？引入第二根修订轴？），超出单个任务范围。**它是登记在案的缺口，不是被覆盖了。**

## H4 — pending 表不做列存在性校验：论证已加强并有实证

dev-agent-42 复核后**推翻了自己原来的论证**（「同一 `NewStore` 建出」不够强——挡不住「某版本只改 `pendingDDL`」），换成**危害结构性不同**：

> 观测表的 INSERT 列由 `Values` 里实际存在的字段决定 ⇒ 列清单随数据变化 ⇒ 缺列**只在恰好含该字段的那一期才炸**，可能数周后才复现。
> pending 的 INSERT **八列固定不变** ⇒ 任何不符在**第一次**写 pending 时就确定性失败。

⇒ G7 的「静默到某一期才炸」在 pending 上**结构性不成立**，剩下的要求只是「失败要可定位」。

**没有停在声明上**：`TestSavePendingFailsLoudlyOnDriftedPendingTable` 把它变成证据（重命名 pending 列后 Save 当场失败、错误含步骤名与期次、观测表零行），M5（吞掉 SQL 错误）被它杀死。

代价评估复核后仍成立：加校验要一份 pending 期望列清单，而 `pendingDDL` 已有一份，那是本包明确反对的「第二份 schema 定义」。

## H5 — 【新】`ingested_at` 的字典序与时间序相反

G8 改用 `RFC3339Nano` 后的副作用（dev-agent-42 实测）：它**去掉小数尾零**，于是 `"…:00Z"` 与 `"…:00.5Z"` 的字典序与时间序相反。

**当前无害**：`ingested_at` 不是 revision 列，`bitemporal` 不碰它，pending 主键只要唯一。

已按本仓库既有做法（`bitemporal` 的 `TestLookupRevisionFormIsCallerGuaranteed`）钉成契约：`TestIngestedAtLexicalOrderIsNotTimeOrder`。**谁将来要对 `ingested_at` 做 `ORDER BY` / `MAX()`，得先改定宽补零格式并迁移数据**，那条会转红提醒他。

## H6 — 需求文档 Self-Review 声明的交接项（原样保留）

1. **M1b-2 (parse)** 产出 `Observation`，`Values` 的键必须取自 `Field*` 常量——写字面量会被白名单拦下
2. **M1b-3 (validate)** 填充 `ValidationReport`，`Check.Value` 的单位需自行定义
3. **M1b-4 (discover)** 负责 `article_id` 一级幂等检查——`SELECT 1 FROM hestia_observations WHERE article_id = ?`，走 `Store.DB()`

## H7 — DoD 撰写的两条改进（验证者提）

**① 必测输入应显式包含尾随换行**（test-agent-21 @ T2）：dev 主动加的 `"2026-07-15\n"` 用例**恰好是杀死 M10（正则锚点由 `\A..\z` 弱化为 `(?m)^..$`）的那两条**，而不带换行的用例集完全发现不了它。

**② 把已知的不可测区域写进 DoD，比留给验证者临场判断更可靠**（test-agent-21 @ T5）：`non_functional[0]` 预先声明桩的存在并写明「不得据此判 REJECT」，避免了一次可预见的误判；`functional[2]` 预写「若不抽出 SQL 构造就须标 review」，促成 dev 主动抽出、该条得以留在 test 级。
