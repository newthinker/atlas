# 需求分析 — Cassandra 危机回放报告（Sprint 022）

- **需求来源**: `docs/plans/2026-07-15-crisis-replay-report-impl.md`（实施计划，含完整 TDD 步骤）
- **设计基线**: `docs/plans/2026-07-15-crisis-replay-report-design.md` v1.1（用户已确认）
- **分析日期**: 2026-07-15
- **规划路径**: ECC 不可用（config `capabilities.ecc=false`）→ 降级路径本应为 superpowers brainstorming；
  但输入文档已是用户确认过的完整实施计划（设计 v1.1 定稿 + 逐任务 TDD 代码），设计探索阶段已在计划编写期完成，
  故 brainstorming 判定为无增量价值而跳过，本文档为计划的结构化蒸馏。

## 目标

将手工回放流程产品化为 `atlas crisis report` 子命令：回放引擎（暖机、零写入）、daily/monthly 文本报告、
标准总结、自包含 HTML 详细报告（内联 SVG）、telegram sendDocument 发送链路。
既有 `crisis replay` 子命令重构为调用同一引擎（v1.1：统一暖机语义，全期窗口黄金对照）。

## 需求清单（追溯 ID ↔ 设计章节）

| REQ | 设计 | 内容 | 复杂度 |
|---|---|---|---|
| R1 | §1 | CLI `crisis report --from --to --form daily\|monthly [--send]`：参数校验（必填/格式/顺序/枚举/早于库内最早日提示）、`--send` 量控 31 条（启动前拦截、消息字面值固定）、回放前缀行 cmd 层拼接、门控忽略、HTML 落盘 `reports/crisis-replay-<from>-<to>.html`、发送降级与单条失败继续 | 复杂 |
| R2 | §2 | 回放引擎 `ReplayRange(cfg, sr, from, to) ([]ReplayDay, error)`：从库内最早 vix 观测日暖机逐日推进、只返回窗口切片、StateDays 转移日=1、零写入（MemHistory）；既有 `executeCrisisReplay` 重构调用引擎，全期窗口输出逐字节不变（黄金对照） | 复杂 |
| R3 | §3 | `ReplayReport(cfg, form, day, prev, sr)`：daily PrevDay 链（首日 nil→无变化）、monthly 21 日 Trends（空窗口省略行）、忽略消息矩阵门控、渲染纯函数、保留消息家族页脚 | 中等 |
| R4 | §4 | `RenderReplaySummary(cfg, days)`：期初态/转移列表/各态停留（严重度降序仅列出现过的态）/指标极值（红灯方向、STALE 日不计入）/AMBER 峰值/STALE 统计（全零省略）；专用 `replayFooter`（不复用 notifyFooter）；千日量级 ≤4096 | 中等 |
| R5 | §5 | `RenderReplayHTML(cfg, days, sr)`：五段结构（阈值摘要表/时间线点阵 SVG/7 指标折线 SVG 含阈值线与 STALE 打点/月度汇总表/转移明细表）、自包含无外链、prefers-color-scheme 亮暗、始终全量日粒度、sofr_effr 无数据专用注记 | 复杂 |
| R6 | §6 | `Telegram.SendDocument(path, caption)`：multipart/form-data 上传、caption >1024 rune 截断、文件名 basename、错误语义同 sendPayload；`Sender` 接口不动 | 中等 |
| R7 | §7 | 全局约束：零新第三方依赖、GOTOOLCHAIN=local、回放全程零写入、internal/crisis 纯函数无 IO、禁词（必然/一定/即将）、telegram 单条 ≤4096、gitnexus 门禁、code-simplifier 提交纪律 | 横切 |

## 范围外

- 操作决策/交易信号（页脚明示）；事后调参（阈值恒为当前配置）；HTML 外链资源；`Sender` 接口变更。

## 风险点

1. **黄金对照**（R2）：全期窗口逐字节不变是硬约束，既有 `TestExecuteCrisisReplay*` 为回归黄金；有真实库时另做手工黄金 diff。
2. **测试 helper 跨任务依赖**：`mkReplayDay` 在 Task 2 测试文件定义，Task 3/4 同包复用 → 任务必须按依赖序执行（DAG 已表达）。
3. **Task 1 跨 2 个 package**（internal/crisis + cmd/atlas）：为保证引擎与重构的黄金对照原子性，沿用计划边界，见 AD-3。
