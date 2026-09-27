# 需求分析 — 银行股关键指标月度监控（sprint 2026-09-27）

需求源（人类已确认，不重开）：
- 设计 spec：`docs/superpowers/specs/2026-09-26-bank-indicator-monitor-design.md`（2026-09-26 人类确认）
- 实施计划：`docs/superpowers/plans/2026-09-27-bank-indicator-monitor.md`（7 个任务，含完整测试与实现草稿）
- 锚点：分支 `feature/bank-indicator-monitor` @ `b2f85462aa32f7766a07dba5938ea9c8d4440830`

## 规划降级说明
`capabilities.ecc=false` ⇒ 不跑 `/multi-plan`。superpowers brainstorming 的产物（spec）与 writing-plans 的产物（plan）
已在本 sprint 之前完成并经人类确认，重跑会重开已拍板决策 ⇒ 本阶段只做「计划 × spec × 现有代码」三方核对。

## 需求条目（R 编号供追溯矩阵使用）
| R | 内容 | spec 出处 |
|---|---|---|
| R1 | 配置：banks 非空、market∈{CN_A,HK}、symbol 不重复、A 股 `^\d{6}\.(SH\|SZ)$`、港股 `^\d{4,5}\.HK$` 且 a_share_ref 必填合法、阈值全 >0、rank_by∈{npl,coverage,cet1} 缺省 npl、aktools_url 缺省 | §3.3 |
| R2 | 数据源：aktools `stock_financial_analysis_indicator_em?symbol=&indicator=按报告期`；字段 NONPERLOAN/BLDKBBL/HXYJBCZL/REPORT_DATE；禁用 LOAN_PROVISION_RATIO/NEWCAPITALADER/FIRST_ADEQUACY_RATIO；超时 60s | §2 §3.1 |
| R3 | 数值容错：字符串数值、null、缺键 → NaN；坏日期行跳过；整列消失标注字段缺失 | §6 §9 |
| R4 | 口径：最新期=至少一项非 NaN 的最近期；环比=紧邻上期；同比=上年同日；变动 pp，round 1e-4 | §4 |
| R5 | 阈值预警严格不等号；恶化预警（npl_up/coverage_down/cet1_down）；NaN 不预警 | §4 |
| R6 | A+H：按 a_share_ref 去重只拉一次、只计一次，H 股附注「同 600036.SH」 | §4 |
| R7 | 同期统计：统计期=最晚最新期；只统计等于统计期的主体；均值/中位数跳过 NaN；排名；未更新名单 | §4 |
| R8 | 报告格式（列表式纯文本，预警/统计/排名/未更新失败/字段缺失）；N/A 与 — | §5 |
| R9 | 分段：单段 ≤4000 rune，只在行边界切 | §5 |
| R10 | 命令 `atlas bank report --bank-config --dry-run`；依赖注入；复用 notifiers.telegram；未启用 → 打印并 stderr 提示 | §3.2 |
| R11 | 退出码：0 全成功 / 2 部分失败照推 / 1 配置非法·全失败（推错误摘要）·推送失败 | §6 |
| R12 | 测试：cmd 层 `--dry-run` 不调用 Sender；render golden 比对；live 冒烟 600036 三字段非 NaN 且最新期 9 个月内 | §7 |
| R13 | 部署：launchd 每月 1 日 09:00、no_proxy、日志 bank-monthly.{out,err}.log；示例 configs/bank-monitor.yaml；上线前 dry-run | §8 |

## 计划 × spec 核对发现（进 DoD）
1. **孤儿需求 ×2**：R12 的「`--dry-run` 不调用 Sender」——计划的 `runBankReport` 无测试，dry-run 分支不可观测；
   「render golden 比对」——计划只做 `Contains` 断言。⇒ 分别补进 TASK-006 / TASK-005 的 DoD。
2. **测试辅助跨任务耦合**：计划中 `analyze_test.go`（Task 3）调用的 `day()` 定义在 `source_test.go`（Task 2）
   ⇒ Task 3 实为依赖 Task 2 才能编译。改为 TASK-001 提供 `internal/bank/helpers_test.go`（仅含 `day`），
   TASK-002/003 不得再定义 ⇒ 二者可并行。
3. **覆盖率门禁**：`cmd/atlas` 在 b2f8546 实测 `-func` 78.6% / profile 逐块求和 78.44%（1248/1591），
   低于 `dev_minimum 80` ⇒ TASK-006 设 `coverage_floor: 78`（与历史同包取值一致）。PENDING-MECHANISMS #10 的偏差在此再现。
4. **提交信息**：计划写 `feat(bank): …`，项目 `.claude/CLAUDE.md` 约定 `<type>(TASK-XXX): …`，且门禁按 TASK-ID grep
   ⇒ 以项目约定为准：`feat(TASK-00x): …`。
5. **spec 偏差（计划有意为之，接受）**：集成标签用 `integration` 非 `live`；头部分列「未更新 N；失败 M」；
   空预警渲染为 `⚠️ 预警 (0)` 换行 `无`；排名标题「按X由优到劣」（兼容降序指标）。
6. **现有代码事实已核实**：`telegram.New/WithProxy`、`(*Telegram).SendText`（纯文本、token 脱敏）、
   `loadConfigOrDefaults`（export_ohlcv.go:283）、`buildCrisisSender`（crisis.go:428）、根 `--config` 持久 flag、
   `main` 对 `Execute` 错误 `os.Exit(1)`；gate_wiring 只守 collector 构造点，与本特性无关；aktools 在线（200/0.38s）。
7. **Telegram 长度余量**：4000 rune 上限对 4096 UTF-16 限额留 96 单位；报告里 astral emoji（🏦📊🏷）仅每段首部数个 ⇒ 充足。

## 人类裁决（2026-09-27，DoD 定稿前；改动 spec 语义）
| R | 裁决 | 来源 | 落点 |
|---|---|---|---|
| R14 | 非 dry-run 拿不到 sender ⇒ 打印 + stderr 说明 + **退出码 1** | reviewer D1（实测 exit 0 静默丢月报） | TASK-006 |
| R15 | 按指标回退到最近非 NaN 值并标「截至」；阈值/恶化预警对回退值生效；环比/同比对该指标自己的上一个非 NaN 期 | reviewer D2（实测 600919/002142 季报 CET1 为 null） | TASK-003/004/005/007 |
| R16 | 统计期 = 以之为最新期的主体数最多的期（并列取晚）；领先主体单列 Ahead | reviewer D15（实测提前披露致 n=1） | TASK-004/005 |
| R17 | 预警期次 ≠ 统计期时行末标注期次 | reviewer D16 | TASK-005 |

## 独立 reviewer 比对结论
- 报告：scratchpad `dod-reviewer-report.md`（327 行，19 条缺陷）。技术项 D3–D14、D18、D19 已并入 DoD；D17 为 spec 笔误（`*telegram.Notifier` 应为 `*telegram.Telegram`），计划正确，不需动作。
- **一处 reviewer 数值不成立**：它称「实测 `1.02-0.92 = 0.10000000000000009`」，Leader 用 Go 运行时解析与 Python 两把尺复测均为 `0.099999999999999978`。它观察到的「去 round4 被 KILLED」来自 `require.Equal(0.1, QoQ)` 而非预警断言——**结论（round4 被守住）对，理由（测到了浮点假阳）错**。TASK-003 保留 Leader 实测的三组真阳性边界对。
