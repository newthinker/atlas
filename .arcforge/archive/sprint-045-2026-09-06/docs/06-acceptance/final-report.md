# Final Report · Sprint M2a（Hestia 契约队列与信号快照）

## 1. 交付概览

- 需求：`hestia/docs/superpowers/plans/2026-09-05-hestia-m2a-contract-queue.md`（射程 TASK-001～007 全部，AD-1）
- 开工锚 `d27791c695e8ebd0fd5d54c9161782f52d9b12cb` → 合并后 master **`589835aa2f7373c5c650072411228ddb02d012b6`**（含 QA C1 修复）
- 任务 7/7 accepted；返工 4 轮（005 dod_defect：Ingest 级 P2 温度断言 M8/M4；006 dod_defect：修订期次回放用例 R1；007 task_defect：§B 字节数成因——事后验证者自纠为超出 DoD 文本，返工保留；**005 task_defect：QA C1 实时契约 `extracted_at` 空串**——`Save` 按值、`IngestedAt` 不回传，修法 Save 后经 `Store.Current` 回读，Ingest 级断言 + CONTRACTS A10）
- 改动 17 文件 +1804/−24（不含 CONTRACTS.md +76）；无新增依赖；四个不动文件与 `Save` 未动；无新增写方法
- **不 deploy**：投递与 M1.5 一起，等 `PENDING-ACCEPTANCE.md` 首期验收登记

## 2. 门禁与实测（锚 `4d81143`，与 CONTRACTS `## Sprint M2a` §B 同源；§B 数字有 Leader / dev-a / dev-b / dev-c / 验证者五份独立采样互证）

| 项 | 值 |
|---|---|
| `internal/hestia` 覆盖率 | 96.6%（DoD 仪器 `go test -cover`；精确 96.5505%，AD-17：`-func` 与门禁合并口径打 96.5 是仪器差异；新增 1 条不可达分支 `Current` 回读失败如实登记） |
| `cmd/atlas` 覆盖率 | 76.6%（基线 76.4） |
| 导出面 | AST 34 / reflect 14（+9 / +2） |
| 真语料回归 | 218 = 217 + 1 · 217 = 213 + 4 · 97 = 76 + 21 · 冲突 0 · 路由违反 0 · 仓库 `queue/` 不存在 · 杂目录 0 |
| 三期 golden | 2020H1 2/4 · 2025 0/4 · 2026H1 1/4 |
| 回放样本 | 2026-06/h1（n=54,m=22，`_mom` 0）；2023-08/monthly（n=53,m=23，`_mom` 20）；字节数跨运行 ±1–2 非判据（`extracted_at` 纳秒尾零） |
| 新增测试 | 48（预估 31；差因表见 §B） |
| 变异（验证者） | 001 7/9 · 002 9/15 · 003 20/22 · 004 17/19 · 005 11/15→复验 +2→复验 2 5/7 · 006 11/13→复验 13/13；存活项全部登记，三轮 review_fix 闭合主路径项 |

## 3. QA 结果

第一轮（03:35Z）**REJECT**：1 CRITICAL / 2 WARNING / 8 SUGGESTION（`docs/05-review/qa-verdict.md` §1–§4）。
- **C1（CRITICAL，已修）**：实时路径契约 `extracted_at` 恒为空串。QA 用临时探针真跑 `Ingest` 实证，并 diff 实时 vs 回放契约定位到唯一差异行。⇒ TASK-005 review_fix 2（task_defect）：`Save` 后经 `Store.Current` 回读、Ingest 级断言（三个变异独家转红）、CONTRACTS A10；test-m2a-b 复验 VERIFIED（04:41Z）。
- **W1**（`Store.Current` 的 `rows.Err()` 出边无前缀，错误文本非行为）与 **W2**（空 `config_version` 该拒，但改两包约 15 处夹具、零生产影响、非需求项）挂账 §C，W2 作 M2b 前独立小任务。
- **S1–S8** 全部挂账（dev-b 终检子代理 3 条、注释中间值、存活变异 004 Q4/Q8 / 005 M11/M12、`PriorPublishedAt` 不可达分支）。
- 跨模型：codex CLI 可运行但配额用尽至 9 月 10 日（`codex exec` 秒回 usage limit）⇒ 退回本体三 lens 跨视角，三 lens 对 C1 一致 high，非 CONTESTED。

复审（04:43Z）**PASS**：探针重跑 `extracted_at` == `Current.IngestedAt`；变异「不填」独家转红；门禁八项与第一轮 §0 逐项相同。

## 4. 事故与机制观察（归档进 PENDING-MECHANISMS）

| # | 时间（UTC） | 形态 | 归类 |
|---|---|---|---|
| 1 | 12:3xZ | dev-a 的 code-simplifier 子代理被 idle hook 以父实例名循环 5 次后自报 | PENDING #3 |
| 2 | 12:40→15:59Z | test-m2a-a 会话挂起 ~3h（工具调用间零输出）；Leader 侧取证对 #3/#4 同形不可区分；逃生边改派 test-m2a-b | PENDING #4 |
| 3 | 13:56→14:16Z | teammate→leader 消息延迟 20 分钟（cron 正常） | 通道延迟 |
| 4 | 14:30→15:59Z | Leader 会话挂起 88 分钟 | PENDING #4 |
| 5 | 16:16→23:32Z | Leader 会话挂起 7h16m；dev-b 的 `blocked_clarification` 文件级信号生效 | PENDING #4 |
| 6 | 00:19→03:13Z | dev-b 为 007 spawn 的 code-simplifier 子代理挂 2.9h 后**带产物**返回；期间父实例收不到消息、不被唤醒 | PENDING #3 |
| 7 | 00:58→03:13Z | dev-a 接 007 后**工具批次本身被挂起**（6 个并行 Bash 里部分 2h13m 后才执行，观测 HEAD 不同）；agent 侧无间隙 | PENDING #4 新形态 |
| 8 | 01:53→03:02Z | dev-c 接 007 后两批调用之间空 65 分钟 | PENDING #4 |
| 9 | 02:40→03:10Z | dev-d 三封 merge 请求延迟 25 分钟到达；Leader 改为按文件级证据主动 merge | 通道延迟 |
| 10 | 03:36→04:34Z | Leader 会话挂起 ~55 分钟；dev-a 的 `blocked_clarification` 文件级信号再次生效 | PENDING #4 |

**QA 第一轮 REJECT 的教训**：C1 是「需求原文接线片段自带隐含前提、dev/verifier/收口三道都没碰实时路径该字段、零测试守卫」——每道都只对照 DoD 文本，DoD 没写的字段值没人看；QA 的探针（真跑 Ingest 后 diff 实时 vs 回放契约）是唯一会发现它的仪器。

处置沉淀：三任 dev 连续在 007 上挂起后改走**记录员模式**（Leader 采数起草、记录员只落盘/提交/迁移），一次通过；Leader 侧 merge 改为「分支有形状正确的提交 + 预演无冲突 ⇒ 直接 merge，不等请求」。其它：write-guard 逃逸一例（dev-b 用 python 直改 checkpoint，自报）；GitNexus detect-changes 全 sprint 不可用（索引落后 + LadybugDB 版本不匹配）；派发/派验通知丢失两次靠 idle hook 重扫兜住。

## 5. 待办（交付后）

- 投递与 M1.5 一起，等 2026-08 首期验收登记；投递后 `atlas hestia contract emit --period 2026-07 --stdout` 核形状
- 回放样本的 `thresholds.config_version` 为 ""（临时 yaml 未写该字段，`LoadConfig` 不强制）；真实投递值 "2026-09-05"——M3 用样本时要知道；是否让 `LoadConfig` 拒绝空 `config_version` 由 QA 裁定（§3）
- 挂账：`Store.PriorPublishedAt` 错误分支不可达；004 Q4/Q8 结构性不可测；005 M11/M12 低优先

## 待同步 hooks 清单

无：本 sprint 未改 `.claude/` 运行时资产。上游议题（PENDING-MECHANISMS #3/#4 频次、Leader 侧「文件级证据主动 merge」与「记录员模式」两条处方）在归档时追加到 `PENDING-MECHANISMS.md`。
