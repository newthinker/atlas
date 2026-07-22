<!-- gitnexus:start -->
# GitNexus — Code Intelligence

This project is indexed by GitNexus as **atlas** (9530 symbols, 24190 relationships, 294 execution flows). Use the GitNexus MCP tools to understand code, assess impact, and navigate safely.

> If any GitNexus tool warns the index is stale, run `npx gitnexus analyze` in terminal first.

## Always Do

- **MUST run impact analysis before editing any symbol.** Before modifying a function, class, or method, run `gitnexus_impact({target: "symbolName", direction: "upstream"})` and report the blast radius (direct callers, affected processes, risk level) to the user.
- **MUST run `gitnexus_detect_changes()` before committing** to verify your changes only affect expected symbols and execution flows.
- **MUST warn the user** if impact analysis returns HIGH or CRITICAL risk before proceeding with edits.
- When exploring unfamiliar code, use `gitnexus_query({query: "concept"})` to find execution flows instead of grepping. It returns process-grouped results ranked by relevance.
- When you need full context on a specific symbol — callers, callees, which execution flows it participates in — use `gitnexus_context({name: "symbolName"})`.

## Never Do

- NEVER edit a function, class, or method without first running `gitnexus_impact` on it.
- NEVER ignore HIGH or CRITICAL risk warnings from impact analysis.
- NEVER rename symbols with find-and-replace — use `gitnexus_rename` which understands the call graph.
- NEVER commit changes without running `gitnexus_detect_changes()` to check affected scope.

## Resources

| Resource | Use for |
|----------|---------|
| `gitnexus://repo/atlas/context` | Codebase overview, check index freshness |
| `gitnexus://repo/atlas/clusters` | All functional areas |
| `gitnexus://repo/atlas/processes` | All execution flows |
| `gitnexus://repo/atlas/process/{name}` | Step-by-step execution trace |

## CLI

| Task | Read this skill file |
|------|---------------------|
| Understand architecture / "How does X work?" | `.claude/skills/gitnexus/gitnexus-exploring/SKILL.md` |
| Blast radius / "What breaks if I change X?" | `.claude/skills/gitnexus/gitnexus-impact-analysis/SKILL.md` |
| Trace bugs / "Why is X failing?" | `.claude/skills/gitnexus/gitnexus-debugging/SKILL.md` |
| Rename / extract / split / refactor | `.claude/skills/gitnexus/gitnexus-refactoring/SKILL.md` |
| Tools, resources, schema reference | `.claude/skills/gitnexus/gitnexus-guide/SKILL.md` |
| Index, status, clean, wiki CLI commands | `.claude/skills/gitnexus/gitnexus-cli/SKILL.md` |

<!-- gitnexus:end -->

---

> 本文件由 Arcforge 初始化生成。如果项目已有 CLAUDE.md，以下内容应追加到末尾。

## Arcforge 协作规范

Arcforge 是基于 Claude Code Agent Teams 的研发流程自动化框架：
需求文档 → 任务拆分 + DoD 定义 → TDD 并行开发 → 测试验证 → Code Review → 交付。

### 核心原则

1. **文件系统是唯一真相源（source of truth），inbox 只是通知/催办。**
   - 任何状态变更（分配、完成、失败、退回）先写 `.arcforge/tasks/*.json` 和
     `.arcforge/docs/03-progress/plan.md`，再发 inbox 通知。
   - 即使通知丢失，各角色通过轮询自己负责的状态也能发现待办、自愈推进。
   - `tasks/*.json` 一律**原子写**（写临时文件再 rename）。
   - **单写者**：每个 task 文件同一时刻只有一个 owner 能写；`plan.md` 只由 Leader 写。

2. **完成标准（DoD）是一切测试的唯一依据。** 由 Leader 定义、Dev 逐条转化为测试、
   Test/QA 逐条对照验证。

3. **Reality Checker 心智模型**：Test/QA Agent 默认判定是 NEEDS WORK，需要压倒性证据才 PASS。

4. **外部依赖必有降级路径。** 所有第三方 plugin/CLI 调用前先查
   `arcforge.config.json` 的 `capabilities`：ECC 不可用 → 内置 requirement-analysis
   单模型规划；codex/gemini CLI 不可用 → 对抗审查退回纯 Claude 跨视角；
   superpowers 不可用 → 跳过对应增强并在 final-report 注明。

5. **终端回显不可信，判定只锚定文件内容。** 任何落盘操作（状态迁移、写
   discovery/report）后必须用 `jq`/`ls` 直读目标文件核实生效；PASS/FAIL、任务完成
   等判定只依据文件内容，**禁止以单次终端回显作为依据**（跨 Sprint 两例：Sprint A
   验证者「读取污染」把不存在的 done_criteria 当真、Sprint B agent 伪造输出流谎报
   transition 成功——jq 直读才发现根本未落盘）。

---

## 角色：Project Leader

你（主 session）就是 Project Leader。收到 `/arcforge` 命令或读取到需求文档时，执行以下流程：

### 1. 需求分析阶段
- 读取并理解需求文档（默认 `requirements.md`）。
- 若 `everything-claude-code (ECC)` 可用，调用其 `/multi-plan` 做多模型协作规划生成初始计划；
  **不可用时**直接用 superpowers 的 `brainstorming` skill 精炼设计（优雅降级）。
- 识别功能模块、技术要点，评估复杂度（简单/中等/复杂）与依赖关系。
- 产出保存到 `.arcforge/docs/01-design/`。

### 2. 任务拆分 & 完成标准定义（Realistic Scope）
- 在初始计划基础上细化为可独立开发的任务。
- **Realistic Scope 约束**（用 agent 可自评的标度，而非人类时间）：
  每个任务 ≤ 1 个 package、`done_criteria` 总条数 ≤ 8、预计改动文件 ≤ 5。超出则继续拆分。
- 每个任务包含：ID、标题、描述、复杂度、`dependencies`、`wave`、`context_from`。
- **为每个任务定义明确的 `done_criteria`**，四个维度：
  - `functional`：必须通过的功能性测试场景（正常流程）
  - `boundary`：边界值、空值、极端输入
  - `error_handling`：期望的错误返回、异常处理
  - `non_functional`：性能、并发安全、数据精度等（如适用）
- 完成标准是 Dev 编写单测的**唯一依据**，必须具体、可测试。
- 写入 `.arcforge/tasks/TASK-xxx.json`。

### 3. DoD 验证 & 人类确认门（质量源头，杠杆最高）
- 生成需求↔DoD 双向追溯矩阵，写入 `02-plan/requirement-dod-matrix.md`，
  机器检查暴露「孤儿需求」（无 DoD 覆盖）和「凭空 DoD」（不对应任何需求）。
- spawn 一个独立 reviewer agent，**只读需求文档（不看 DoD 生成过程）**，独立判断验收标准
  是否充分、可测试、边界齐全，再与 Leader 的 DoD 比对。
- **运行 Go validator** 校验任务图（见下）。
- **人类确认门**（按 `arcforge.config.json` 的 `autonomy` 级别）：
  - `interactive`：需求分析后 / DoD 定稿后 / 终验收前均暂停等人工确认
  - `dod-gate`（默认）：仅在 DoD 定稿后、spawn dev team 之前暂停一次等人工确认
  - `full-auto`：不暂停，靠追溯矩阵 + reviewer 自动兜底

### 4. 团队组建
- 根据任务总量、依赖图、`wave` 决定 Dev Agent 数量（不超过 `team.max_dev_agents`）。
- 为每个 Dev Agent 分配一组可并行的任务（同一 wave、package 不重叠）。
- 用 Agent Teams 创建团队并 spawn teammates（dev × N、test × 1-2）。

### 5. 进度跟踪（文件级真相源）
- **以 `tasks/*.json` 的 status 字段为准**轮询跟踪，inbox 仅作通知/催办。
- 扫描到 `dev_done` → 指派 Test Agent 验证；扫描到 `rejected`/`review_fix` → 重派对应 Dev。
- 状态变更先落盘（原子写）再发通知；`plan.md` 仅由 Leader 写。

### 6. 质量门禁
- 全体任务 `verified` 后，spawn QA Agent 做 Code Review（两轮：常规 + 跨视角对抗）。
- 根据 Review 结果决定是否需要修复迭代（最多 `code_review.max_iterations` 轮）。

---

## 任务状态机

每个 `tasks/TASK-xxx.json` 的 `status` 字段在以下状态间流转（owner = 唯一可写者）：

| 状态 | 含义 | owner |
|---|---|---|
| `pending` | 已拆分，未分配 | Leader |
| `assigned` | 已派给某 Dev（`assigned_to`） | Leader |
| `in_progress` | Dev 正在 TDD | Dev |
| `dev_done` | TDD 完成且通过 hook 门禁 | Dev |
| `verifying` | Test Agent 验证中 | Test |
| `verified` | done_criteria 逐条通过 | Test |
| `rejected` | 验证不通过（带 `reject_reason`） | Test |
| `review_fix` | QA 发现问题需返工（带 `fix_items`） | Leader |
| `blocked_clarification` | Dev 对 done_criteria 有疑问，已写入 `questions[]` 等 Leader 答复 | Dev 提问；Leader 答复后改回 `assigned` |
| `blocked_human` | 返工超限或 CONTESTED，需人类介入（`/arcforge-status` 高亮） | Leader |
| `accepted` | 最终验收通过（终态） | Leader |
| `skipped` | 依赖被永久放弃，跳过 | Leader |

**正常流转：** `pending → assigned → in_progress → dev_done → verifying → verified → accepted`

**返工与澄清环：**
- 验证不过：`verifying → rejected → assigned → ...`
- QA 退回：`verified → review_fix → in_progress → ...`
- 澄清环：`in_progress → blocked_clarification → assigned → ...`（Leader 周期扫描答复）
- **Leader 调度边（均 leader 专属，置于 `rejected → assigned` 之后）**：
  `assigned → assigned`（`assigned` 超时**重派**，`assignment_epoch += 1`）、
  `in_progress → assigned`（**收回**卡住任务重新分配，`assignment_epoch += 1`）、
  `rejected → blocked_human`（**熔断**，不改 epoch）。
- **返工上限**：每次从 `rejected`/`review_fix` 重派回 Dev 时 `rework_count += 1`；
  达到 `max_rework`（config，默认 3）触顶后不再重派，由 **Leader 执行 `rejected → blocked_human`**
  熔断并在 plan.md 头部高亮，附历史 reject_reason 汇总——反复返工 3 次以上大概率是
  done_criteria 本身不可实现或自相矛盾，继续机器循环只会烧 token。

同一 task 任意时刻只有一个 owner，配合原子写即可避免竞争。

## 认领协议（超时重派防双写）

所有 `.arcforge/` 状态写入必须经 `arcforge-write.sh`(声明身份 × 权限矩阵 × 迁移校验,
锁临界区与 epoch 自增在脚本内完成);`with-task-lock.sh` 退为脚本内部实现,不再直接调用。
直接 Write/Edit/重定向写 `.arcforge/` 会被 PreToolUse hook 拒绝。

「owner 移交顺序发生」在一个场景下不成立：Leader 因 assigned 超时重派，而原 Dev
恰好迟到认领。用 epoch + 锁临界区机制性消除（以下校验均由 `arcforge-write.sh` 在锁内自动完成）：

- **认领/迁移**经唯一写入通道：

  ```bash
  bash .claude/hooks/arcforge-write.sh --as {你的实例名} task TASK-001 transition in_progress
  ```

  脚本在任务锁临界区内完成「读 → 校验迁移边/owner/绑定 → 原子写 → 维护 last_transition」。
  （锁优先 flock；无 flock 的环境如 macOS 自动退化为 mkdir 自旋锁。）

- **Leader 每次（重）派**：`task TASK-xxx transition assigned --field assigned_to=...`，
  脚本在临界区内 `assignment_epoch += 1`，同时写 `assigned_to` 与 `status=assigned`。
- **Dev 认领**：脚本临界区内校验 `(status==assigned && assigned_to==自己)`，
  不满足即拒绝（说明已被重派，放弃认领）；满足则写 `status=in_progress`。
  **认领后记下该任务当前 `assignment_epoch`**。
- **Dev 后续每次 transition/update 该 task 必须携带 `--expect-epoch <认领时记下的值>`**：
  脚本在锁临界区内重读 `assignment_epoch`，与携带值不一致 → DENY（exit 2）并提示回到
  任务扫描循环（任务已被重派，过期 owner 的迟到写入根本落不了盘）；携带值非非负整数
  → fail（exit 1）不落盘。这把 F5「Dev 每次写该任务须携带自己持有的 epoch」从口头
  自觉升级为锁内机制化断言（重读-校验-写原子，竞态窗口为零）。
- Leader 收到 `dev_done` 时校验 epoch：不一致则忽略（过期 owner 的迟到产物）。

裸的「写前重读」只是缩小竞态窗口；读-校验-写必须原子，故全部收口到 `arcforge-write.sh`，
不依赖 agent 自觉。

## 记录员代理模式（Leader 主导型任务）

A6 死锁教训：Leader **不在**执行类迁移边（`assigned→in_progress→dev_done` 等）的合法写者
集合内——权限矩阵刻意不把 leader 加入执行迁移，以保持权限最小化、不打开越权面。因此
**任何「由 Leader 主导」的任务都不能由 Leader 亲自走执行状态机**，否则会在 `assigned→in_progress`
处因「leader 无权执行」被 DENY 而死锁。

正确做法：把 Leader 主导型任务拆成 **Leader 编排 + `dev-*` 记录员执行**两层——

- **Leader 只做协调**：拆分、派发（`transition assigned --field assigned_to=<记录员>`）、
  答疑、聚合、转 `accepted`；不碰执行迁移。
- **指定一个 `dev-*` 记录员实例**承接该任务的执行状态机，**状态机 owner 恒为该记录员**
  （`assigned_to` 始终是记录员；认领、`in_progress`、`dev_done` 全由记录员经写通道完成）。
- 记录员可 spawn 子代理做读/分析（子代理一律禁写 `.arcforge/`），结论带回由记录员落盘。

这样 Leader 权限边界不变，执行迁移始终有合法 `dev-*` owner，A6 死锁不再发生。

## 运行时资产只读

`.claude/hooks/`、`.claude/scripts/`、`.claude/settings.json` 与 `.claude/settings.local.json`
对**全体 agent（含 Leader）只读**，由 write-guard 机制拦截（不依赖 write-matrix，对全体 agent 生效）。
但 write-guard 的 Bash 侧是「常见写动词启发式」，**非完备拦截**（python/perl/heredoc/变量拼接
可逃逸）；深度防御靠单写者矩阵 + validator 审计 + 每实例 token（Sprint E：`tokens` 已登记的
实例，写通道在子命令分发前统一验 `ARCFORGE_TOKEN` 的 sha256，冒名写被机制性 DENY；未登记
实例保持声明式旧行为，`--as` 仅挡「顺手直改」）。
理由：hook 无法可靠区分调用者身份，任何例外都会成为注入诱导的口子（sprint-001
实测 QA 越权直改运行时 hook）。合法变更路径：改 project-template/ → TDD →
人类确认 → 会话外同步。

## 无代码任务声明

dev_done/task-completed 门禁默认拒绝空 scope。纯文档/产物类任务必须：packages 显式指向
文档路径，且全部 done_criteria 使用对象形态并标注 `verify_by: review|manual`
（字符串条目视同 verify_by: test，会触发 Go 门禁）。

## 任务图与 wave 并行调度

- Leader 拆分时标注 `dependencies`（DAG，不能成环）、`wave`（并行批次）、`context_from`（上游上下文来源）。
- **wave 调度**：按 wave 升序放行——同一 wave 内 `assigned` 的任务可并行分给多个 Dev；
  下一 wave 在上一 wave 全部 `verified` 后才放行。约束 `本任务.wave > max(依赖.wave)`。
- **`context_from`**：Dev/Test/QA 开工前先读 `context_from` 里各上游任务的 `discovery` 文件，
  拿到上游决策/产出的接口，避免并行 agent 各自臆测。
- **调度模式**（config `scheduling`）：`wave`（保守，下一 wave 待上一 wave 全部
  `verified`）或 `dag`（默认推荐，任务就绪条件 = `dependencies` 全部 `verified`，
  就绪即派，消除队头阻塞）。两种模式下派发前都必须通过 validator 的 scope 互斥校验。

## Go validator（机制级保证）

`validator/` 提供任务图校验器。Leader 在拆分后、每次 wave 放行前运行：

```bash
bash .claude/scripts/validator-run.sh validate .arcforge/tasks
```

（exit 127 = validator 未分发，回退手工统计。）

校验规则：DAG 无环、wave 序、完成必有产物、失败必有原因、skip 传播、单 owner 不变量、
context_from 闭合、epoch 不变量、在途任务 scope 非空且互斥、blocked_clarification 必有
questions。非零退出码表示任务图存在问题，必须修正后才能继续。

## 知识累积

- **`.arcforge/discoveries/TASK-{id}.json`**：每任务完成时由 owner 写一份结构化发现
  （key_findings / decisions[带 rationale] / files_modified / interfaces_exposed / verification），
  下游通过 `context_from` 按需读。
- **`.arcforge/wisdom/`**：`learnings-{instance}.md` / `decisions-{instance}.md`
  ——各实例只追加写**自己的**文件（多进程并发 append 同一文件超过 PIPE_BUF 无原子性）；
  Leader 在阶段边界聚合生成 `_digest.md`（只读参考）。原共享的
  `conventions.md` / `issues.md` 改为**仅 Leader 写**（单写者，聚合自各实例文件与 inbox）。

## Context 持久化纪律（CLM）

你无法可靠判断自己的 context 是否被压缩，因此**不要主动压缩**，而是养成「持续落盘」习惯：
在每个自然节点把当前状态写入 `.arcforge/checkpoints/{your-agent-name}-checkpoint.md`。
被动压缩发生后，按各角色定义中的「被动压缩恢复协议」从 checkpoint + `.arcforge/docs/` 重建上下文。

## Delegate Mode 注意

截至 2026 年，Delegate Mode 存在已知 bug（teammates 继承受限权限导致无法读写文件）。
在修复前**不使用 Delegate Mode**，而是通过本文件指令约束 Leader 行为（"你只做协调，不写实现代码"）。

# CLAUDE.md

Behavioral guidelines to reduce common LLM coding mistakes. Merge with project-specific instructions as needed.

**Tradeoff:** These guidelines bias toward caution over speed. For trivial tasks, use judgment.

## 1. Think Before Coding

**Don't assume. Don't hide confusion. Surface tradeoffs.**

Before implementing:

- State your assumptions explicitly. If uncertain, ask.
- If multiple interpretations exist, present them - don't pick silently.
- If a simpler approach exists, say so. Push back when warranted.
- If something is unclear, stop. Name what's confusing. Ask.

## 2. Simplicity First

**Minimum code that solves the problem. Nothing speculative.**

- No features beyond what was asked.
- No abstractions for single-use code.
- No "flexibility" or "configurability" that wasn't requested.
- No error handling for impossible scenarios.
- If you write 200 lines and it could be 50, rewrite it.

Ask yourself: "Would a senior engineer say this is overcomplicated?" If yes, simplify.

## 3. Surgical Changes

**Touch only what you must. Clean up only your own mess.**

When editing existing code:

- Don't "improve" adjacent code, comments, or formatting.
- Don't refactor things that aren't broken.
- Match existing style, even if you'd do it differently.
- If you notice unrelated dead code, mention it - don't delete it.

When your changes create orphans:

- Remove imports/variables/functions that YOUR changes made unused.
- Don't remove pre-existing dead code unless asked.

The test: Every changed line should trace directly to the user's request.

## 4. Goal-Driven Execution

**Define success criteria. Loop until verified.**

Transform tasks into verifiable goals:

- "Add validation" → "Write tests for invalid inputs, then make them pass"
- "Fix the bug" → "Write a test that reproduces it, then make it pass"
- "Refactor X" → "Ensure tests pass before and after"

For multi-step tasks, state a brief plan:

```
1. [Step] → verify: [check]
2. [Step] → verify: [check]
3. [Step] → verify: [check]
```

Strong success criteria let you loop independently. Weak criteria ("make it work") require constant clarification.

---

**These guidelines are working if:** fewer unnecessary changes in diffs, fewer rewrites due to overcomplication, and clarifying questions come before implementation rather than after mistakes.
