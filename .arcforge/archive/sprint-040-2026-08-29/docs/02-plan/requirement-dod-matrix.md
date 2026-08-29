# 需求 ↔ DoD 双向追溯矩阵（M1c-3a）

> 本文件由脚本生成并做机器检查，不是手写。检查两件事：
> **孤儿需求**（有需求无 DoD 覆盖）与 **凭空 DoD**（有 DoD 不对应任何需求）。

**规模**：31 条需求 × 61 条 DoD，横跨 8 个任务。

## 正向：需求 → DoD

| 需求 | 来源 | 内容 | 落在 |
|---|---|---|---|
| R-01 | 计划 T1 / spec ① | 月报累计前缀「前N个月」接进 periodAlt | `TASK-001:functional[0]` |
| R-02 | 计划 T1 | cumulativePeriods 增 11 个键（含 1月份 特例） | `TASK-001:functional[1]`<br>`TASK-007:boundary[2]` |
| R-03 | spec §5 / 计划 T1 Step7 | 两处硬卡点，反向消融证明都接上了 | `TASK-001:functional[2]`<br>`TASK-001:error_handling[0]` |
| R-04 | 计划 T1 Step3 | 当月句前缀仍被排除 | `TASK-001:boundary[0]` |
| R-05 | 计划 T1 Step3 | 11月份 不退化成 1月份 | `TASK-001:boundary[1]` |
| R-06 | 计划 T2 / spec ② | 社融存量独立报告整篇抽取 18 字段 | `TASK-002:functional[1]`<br>`TASK-002:functional[2]` |
| R-07 | 计划 T2 / spec ③ | 社融增量独立报告整篇抽取 9 字段 | `TASK-002:functional[1]`<br>`TASK-002:functional[2]`<br>`TASK-002:boundary[1]` |
| R-08 | spec §6 / 计划 T2 Step3 | 占比句不被误抽（钉住已正确的性质） | `TASK-002:boundary[0]` |
| R-09 | Global Constraint | mustMatch 纪律不放宽，缺分项一律报错 | `TASK-002:error_handling[0]` |
| R-10 | spec §4 / 计划 T2 Step8 | parseTitle 增加 kind 返回值 | `TASK-007:functional[0]`<br>`TASK-007:error_handling[0]` |
| R-11 | spec §4 / 计划 T2 Step9 | Parse 按 kind 三路分派 | `TASK-007:functional[1]` |
| R-12 | 计划 T2 设计问题 | 社融新增 extractor 值与各自必填集 | `TASK-003:functional[0]`<br>`TASK-003:functional[1]`<br>`TASK-003:boundary[1]` |
| R-13 | Global Constraint / AD-5 | 删 monthly 分支而非删函数 | `TASK-007:functional[2]` |
| R-14 | 计划 T3 ① / spec ④ | 5 节前三季度报告被正确识别 | `TASK-004:boundary[0]`<br>`TASK-004:error_handling[0]` |
| R-15 | 计划 T3 Step2 | 真截断仍被拒（新判据必配的另一半） | `TASK-004:boundary[1]`<br>`TASK-004:boundary[2]`<br>`TASK-004:error_handling[0]` |
| R-16 | 计划 T3 ② / Step5 | 贷款锚点变体 + 五个逐字段数值断言 | `TASK-005:functional[1]`<br>`TASK-005:boundary[0]` |
| R-17 | spec §9 | 真实快照进 testdata（本次 9 份） | `TASK-002:functional[0]`<br>`TASK-004:functional[0]`<br>`TASK-005:functional[0]`<br>`TASK-007:functional[0]` |
| R-18 | spec §10 / 计划 T4 Step2 | 真跑 calibrate 作为唯一端到端验收 | `TASK-008:functional[0]`<br>`TASK-008:boundary[0]`<br>`TASK-008:boundary[1]` |
| R-19 | 计划 T4 Step4 | CONTRACTS 登记 | `TASK-008:functional[2]` |
| R-20 | 计划 T4 Step3 | 回归测试不依赖 15MB 产物目录 | `TASK-008:functional[1]` |
| R-21 | Global Constraint | 无新增依赖 | `TASK-001:non_functional[0]`<br>`TASK-002:non_functional[0]`<br>`TASK-003:non_functional[0]`<br>`TASK-004:non_functional[0]`<br>`TASK-005:non_functional[0]`<br>`TASK-006:non_functional[0]`<br>`TASK-007:non_functional[0]`<br>`TASK-008:non_functional[0]` |
| R-22 | Global Constraint | 每 task 结束 gofmt / vet / go test 干净 | `TASK-001:non_functional[0]`<br>`TASK-002:non_functional[0]`<br>`TASK-003:non_functional[0]`<br>`TASK-004:non_functional[0]`<br>`TASK-005:non_functional[0]`<br>`TASK-006:non_functional[0]`<br>`TASK-007:non_functional[0]`<br>`TASK-008:non_functional[0]` |
| R-23 | Global Constraint | 注释引用任务编号带 milestone 前缀 | `TASK-001:non_functional[0]`<br>`TASK-002:non_functional[0]`<br>`TASK-003:non_functional[0]`<br>`TASK-004:non_functional[0]`<br>`TASK-005:non_functional[0]`<br>`TASK-006:non_functional[0]`<br>`TASK-007:non_functional[0]`<br>`TASK-008:non_functional[0]` |
| R-24 | 人类裁决 / AD-1 | 月报新增 rule-monthly@v1 / @v2 与必填集（25 / 52） | `TASK-003:functional[0]`<br>`TASK-003:functional[1]`<br>`TASK-003:boundary[0]` |
| R-25 | Leader 缺口 G1 / AD-3 | 外汇板块在月报族下声明式跳过，与 requiredFields 同源 | `TASK-006:functional[0]`<br>`TASK-006:functional[1]`<br>`TASK-006:boundary[0]`<br>`TASK-006:boundary[1]` |
| R-26 | Leader 缺口 G2 / AD-2 | 四种月报布局被 detectExtractor 正确识别 | `TASK-004:functional[1]`<br>`TASK-004:functional[2]`<br>`TASK-004:boundary[0]`<br>`TASK-007:boundary[0]` |
| R-27 | Leader 缺口 G3 / AD-6 | moneyRE 认全角括号（M2/M1/M0 三项） | `TASK-005:functional[2]`<br>`TASK-005:boundary[1]` |
| R-28 | Leader 实测 C4 | 3 月报确实存在（2 篇），一季度前缀保持不动 | `TASK-001:boundary[2]`<br>`TASK-007:boundary[0]`<br>`TASK-008:functional[2]` |
| R-29 | Leader 实测 C5 | 住户侧锚点已覆盖两版，不要动 | `TASK-005:functional[1]` |
| R-30 | AD-2 新引入 | periodType 交叉校验堵住「静默降级」缝 | `TASK-004:functional[1]`<br>`TASK-004:boundary[1]` |
| R-32 | spec ① 的**目的** | `*_ytd` 必须装累计句的值，不是当月句 | `TASK-007:boundary[1]` |
| R-31 | AD-9 | pre/post 同源、数字与采样 HEAD 同行落地 | `TASK-008:functional[0]` |

## 机器检查结果

### 1. DoD 位置有效性：✅ 全部存在


### 2. 孤儿需求（有需求无 DoD）：✅ 0 条

需求文档的 Global Constraints 逐条核对：

| Global Constraint | 覆盖 |
|---|---|
| Go 1.24.4 / 包 `internal/hestia` | 环境既定，`packages` 声明即是（不单列 DoD） |
| 无新增依赖 | R-21 ✅ 8/8 任务 |
| `mustMatch` 纪律不放宽 | R-09 ✅ |
| 删分支不删函数 | R-13 ✅ |
| 本迭代不做入库 / LLM / magnitude_ranges | 边界声明，写在 TASK-003 description 与 TASK-008 CONTRACTS 第 5 点 ✅ |
| 注释带 milestone 前缀 | R-23 ✅ 8/8 任务（**本矩阵首轮检查发现的孤儿，已补**） |
| 每 task gofmt / vet / test 干净 | R-22 ✅ 8/8 任务 |
| 测试 import 按需增补 | 琐碎且编译器强制，不单列 DoD |

### 3. 凭空 DoD（有 DoD 不对应任何需求）：7 条

⚠️ **首轮机器检查报 12 条，Leader 逐条核对后发现其中 4 条不是凭空，是矩阵自己的映射漏了**
——最重要的一条是 `TASK-007:boundary[1]`（`*_ytd` 必须装累计句的值），
那正是整个 R-01/R-02 存在的**目的**，却没有出现在需求表里。已补为 R-32。

**这就是双向矩阵的用处：它查出的不只是 DoD 的缺口，也包括需求表自己的缺口。**

修正后剩余 7 条，逐条核对均为**守卫的另一半**或既有行为的回归保护，非夹带私货：

| 位置 | 性质 | 摘要 |
|---|---|---|
| `TASK-003:functional[2]` | 既有约定（不交出底层数组） | `requiredFields` 返回的切片必须是 `Clone` 或新建，**不得交出底层数组**（`fieldOrder` 是 DDL、… |
| `TASK-003:boundary[2]` | 回归保护（llm-fallback 行为不变） | 未知 extractor 仍返回 `nil`：`llm-fallback@v1` 的既有行为**一字不变**，`TestRequiredFi… |
| `TASK-003:error_handling[0]` | 回归保护（错误信息由白名单拼出） | `checkEnum` 的错误信息由白名单**本身**拼出（既有机制，不要另抄一份）。新增 4 个值后错误信息自动含它们——加一条断言防回归… |
| `TASK-005:error_handling[0]` | 守卫另一半（阴性对照） | 锚点找不到时错误信息仍可诊断（含锚点原文），既有 `loan scope anchor %s not found` 的形态不变。  加一条阴… |
| `TASK-006:functional[2]` | R-25 的实现细节 | `extractFields` 的 `unknown extractor` 保护相应更新：接受 4 种走板块路径的值（`rule@v1`/`… |
| `TASK-006:boundary[2]` | 守卫另一半（两维度独立） | `rule-monthly@v2` 下社融两节**适用**（7 节月报含它们），外汇节仍跳过。这一格证明两个维度是独立的、不是耦合成一个开关… |
| `TASK-006:error_handling[0]` | 守卫另一半（防「跳过」写成「全放行」） | 适用的板块缺失时仍报错（既有行为不变）：`rule-monthly@v1` 缺「人民币贷款」节 ⇒ 报错。  把它与「外汇节缺失但被跳过」放… |

## 4. 需求文档 Spec 覆盖表的逐条对照

| Spec 要求 | 需求文档说落在 | 本 Sprint 实际落在 | 差异 |
|---|---|---|---|
| ① monthly 累计前缀 | T1 | TASK-001 | — |
| ② 社融存量报告 | T2 | TASK-002 + TASK-003 + TASK-007 | **拆三层**（抽取 / 取值域 / 分派） |
| ③ 社融增量报告 | T2 | 同上 | — |
| ④ 3 条失败 | T3 | TASK-004（2 条布局）+ TASK-005（1 条锚点） | **按成因拆二**，需求文档自己也说是两个成因 |
| §3.1 整篇当一节 | T2 Step5 | TASK-002:functional[1] | — |
| §4 parseTitle 返回 kind | T2 Step8-9 | TASK-007:functional[0][1] | — |
| §5 两处硬卡点 | T1 Step3/5/7 | TASK-001:functional[2] + error_handling[0] | — |
| §6 占比句风险 | T2 Step3 | TASK-002:boundary[0] | 理由改为「钉住已正确的性质」 |
| §9 五份快照 | T1+T2+T3 = 6 份 | **9 份** | **+3**：4 节月报、7 节月报、3 月报（需求文档不知道这三种形态存在） |
| §10 验收 | T4 Step2 | TASK-008 | — |

## 5. 本 Sprint 相对需求文档**多出来**的需求（R-24 ~ R-31）

八条，全部来自 Leader 实测或人类裁决，需求文档一条都没有：

| 需求 | 为什么需求文档没有 |
|---|---|
| R-24 月报专属 extractor | 它不知道 53/55 篇月报缺外汇板块 |
| R-25 外汇板块适用性 | 同上 |
| R-26 四种月报布局 | 它以为月报与季报同布局 |
| R-27 全角括号 | 它没跑过 7 节月报 |
| R-28 3 月报存在 | 它明确断言「3/6/9/12 月不存在月报」，**错的** |
| R-29 住户锚点不要动 | 它引用的是过期的代码状态 |
| R-30 periodType 交叉校验 | 这条缝是**新判据引入的**，需求文档的判据不产生它 |
| R-31 pre/post 同源 | 它只说「重跑 calibrate」，没说 pre 必须在动手前采 |

---

## 6. 第二轮：独立 reviewer 回报后新增的需求（R-33 ~ R-40）

`dod-reviewer` 只读需求文档与代码（禁读 `.arcforge/`），在**隔离 worktree 里实际实施了
计划 T1–T3 并真跑 calibrate**。它的第 1、2 节送达后因 session limit 中断，第 3、4 节丢失。

**Leader 对它的每一条都做了独立复验**，结论分三类：

| reviewer 的发现 | Leader 复验 | 处置 |
|---|---|---|
| calibrate 硬过滤社融，138 篇贡献为 0 | ✅ **证实**（`calibrate.go:245`） | **新增 TASK-010** |
| 分部门是当月值而非累计值 | ✅ **证实且精确化**（见下） | **新增 TASK-009** |
| `scopeTotalRE` 字符串拼接，裸交替会打断功能 | ✅ **证实**（`profiles.go:316`） | 补 TASK-005 |
| 「1-N月」形态存在（4 篇） | ✅ **证实**（2022-07/08/10/11） | 补 TASK-008 CONTRACTS |
| 社融两节在 v2 是首部不是末尾 | ✅ **证实** | 补 TASK-004 + TASK-008 |
| 社融增量总量句两种形态 | ✅ **证实且更普遍**（25 篇只有「增量为」） | 补 TASK-002 |
| 存量「余额为 <空格>」3 篇 | ✅ **证实** | 补 TASK-002 |
| 「同比持平」致零命中 | ❓ **未能复现**（69 篇存量搜得 0） | 作为待复验线索写进 TASK-002 |
| 新判据去掉了节数上界 | ✅ 成立 | 补 TASK-004 |
| 3 月报 monthly 语义会错 3 倍 | ✅ 成立 | 补 TASK-008 CONTRACTS |

### Leader 对「分部门口径」的精确化（比 reviewer 的说法更窄也更严重）

reviewer 报「2020-01…2023-11 月报的分部门只有当月一组数字」。Leader 全量分类后发现
**关键区分是「有没有能被认出的累计句」**——没有累计句的会**响亮失败**（安全），
有累计句而分部门是当月的才**口径混杂**（危险，且加总校验自洽反而放行）。

⇒ 危险区是 4 篇而非 27 篇；但结论更严重：**「分部门口径」是独立于「累计句形态」的
第二个维度**，两个方案都没有这个维度。

**结构判据**（Leader 实测 54/54 一致，0 例外）：「分部门看」之前最近的期次前缀决定口径。

| 需求 | 内容 | 落在 |
|---|---|---|
| R-33 | calibrate 放行社融两种 kind（否则 138 篇贡献为 0） | `TASK-010:functional[0][1]` |
| R-34 | 分部门口径守卫：结构判据，非数值启发式、非黑名单 | `TASK-009:functional[0][1]` |
| R-35 | 既有 22 篇季报/年报抽取结果一字不变（回归底线） | `TASK-009:functional[2]` |
| R-36 | 锚点必须用非捕获组 `(?:A\|B)` | `TASK-005:functional[1]` |
| R-37 | 社融增量两种总量句 + 存量「余额为 <空格>」 | `TASK-002:boundary[2]` |
| R-38 | 新判据无节数上界，「多出板块」也要显式决定 | `TASK-004:boundary[1]` |
| R-39 | 218 篇四格加总恒等式（不让东西静默消失） | `TASK-010:boundary[0]` |
| R-40 | CONTRACTS 三条假声明禁令 + 3 月报语义警告 | `TASK-008:functional[2]` |

### 🔴 两级审查都放过的一条

需求文档断言「『1-8月』形态在 55 篇里一次都没出现」，并把它写成**实测更正**。
**Leader 复核时也漏了**——我统计的是 `X人民币贷款增加` 的前缀，而真实形态是
`1-8月，人民币贷款**累计**增加15.61万亿元`（中间有逗号和「累计」二字，不匹配我的正则）。

reviewer 用不同的搜法找到了 4 篇。⇒ **同一个盲区能同时骗过写的人和复核的人，
当两人用的是同一种搜法时。** 已写进 TASK-008 的 CONTRACTS 禁令。
