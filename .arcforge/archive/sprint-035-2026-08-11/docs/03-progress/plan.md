# Sprint 035 进度 — Hestia M1b-3 validate

**需求**：`hestia/docs/superpowers/plans/2026-08-11-hestia-validate.md`
**目标包**：`internal/hestia`（本仓库 atlas）· **调度**：`dag`
**起始基线**：`125ad896…`（覆盖率 89.4-89.5%，口径见 F4）· **当前 HEAD**：`cfc24feb9afc926bd422895085c0c4b3417f77be`

## 阶段

- [x] Step 1 环境检查
- [x] Step 2 需求分析
- [x] Step 3 任务拆分 + DoD + 双向追溯 + 独立 reviewer 反审 + validator
- [x] Step 4 人类确认门（worktree 方案 C、corp_loan Value 裁定，均已批准）
- [ ] Step 5 组队与开发 <- **当前位置（wave 1 已完成，wave 2 在途）**
- [ ] Step 6 QA 两轮审查
- [ ] Step 7 交付验收与归档

## 任务状态

| ID | wave | 标题 | 依赖 | 状态 | owner |
|---|---|---|---|---|---|
| TASK-001 | 1 | Thresholds 与配置自校验 | — | ✅ **verified** | dev-agent-46 / test-agent-24 |
| TASK-002 | 1 | completeness 必填集派生 | — | ✅ **verified** | dev-agent-47 / test-agent-24 |
| TASK-003 | 2 | History 窄接口与 Store.Preceding | 001 | **assigned** | dev-agent-46 |
| TASK-004 | 3 | Validate 骨架与五道无历史闸门 | 002,003 | pending | — |
| TASK-005 | 4 | deposit_sum 两判据合成 | 004 | pending | — |
| TASK-006 | 5 | stock_continuity 与跳过理由优先级 | 005 | pending | — |
| TASK-007 | 6 | 豁免应用、Save 接线、ULP 契约 | 006 | pending | — |

**已合入 master**：`234baea`（TASK-001，4 文件 +239/-5）、`e24c062`+`cfc24fe`（TASK-002 三方合并，2 文件 +161/0）
**Leader 亲自复跑**：`gofmt`/`vet` 无输出、整包绿、覆盖率 **90.2%**（`-coverpkg`）/ **90.1%**（`cover -func` total）

## wave 1 的验证强度（test-agent-24，16 条全过）

- **21 个变异全部 KILLED**（TASK-001 十四个 / TASK-002 七个），**每个都核对到致红断言的行号**
- **漂移为零**：HEAD 与两个 `verify_baseline.head` 逐字相同，两份 discovery 的 sha256 亦逐字相同 ⇒ 未用 `--ack-drift`
- 「dev 交付后未再改动交付物」的声称已**核实为真**，不是采信
- scope 逐项一致：TASK-001 实改 4 == 声明 4；TASK-002 实改 2 == 声明 2
- 主工作区零污染，四个一次性 worktree 已全部拆除

## 三条独立路径撞上同一族问题：**守卫在场 ≠ 守卫有效**

| 发现者 | 发现 | 判据 |
|---|---|---|
| 独立 reviewer | `deposit_sum`/`corp_loan`/`yoy_sanity` 三道阈值翻转比较符**无一转红**（对照组 `stock_continuity` 被杀） | 消融 |
| dev-agent-47 | 计划四条断言对 `HasPrefix` 错误实现**全部 PASS** —— DoD 写了禁令但无机制执行 | 消融 |
| test-agent-24 | `assert.Contains(err,"caliber_version")` 守不住「输出逐字不变」（改成 `…versionX` 仍绿） | 阴性对照 |

统一判据是「**改坏它有东西会红吗**」，答「不会」时守卫是不存在的，哪怕它看起来在场。
前两条已写进 TASK-004/005 的 DoD；第三条不属任何在途任务，记入 `02-plan/findings-carryover.md`（F1）。

## 遗留发现

见 `02-plan/findings-carryover.md`（F1-F7）。**QA 阶段与 TASK-007 都要读**。

## 已识别风险与处置

| 编号 | 处置 | 状态 |
|---|---|---|
| AD-035-1 | worktree 方案 C（wave1 隔离、wave2-6 直连） | wave1 已验证有效 |
| AD-035-2 | wave1 先 commit 加锚点行、wave2-6 先 transition 后 commit | wave2 首次应用中 |
| AD-035-3 | RED 须因预期原因失败，实际输出原文入 discovery | 两任务均已核实为真 `undefined:` |
| AD-035-4 | `scope-writes-outside-packages` 假阳，原样放着 | 每个在途任务均触发，未受影响 |
| F6 | 通知丢失两次，均靠 dev 自查自愈 | 处置写入 findings-carryover |
