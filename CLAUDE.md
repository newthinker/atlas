<!-- gitnexus:start -->
# GitNexus — Code Intelligence

This project is indexed by GitNexus as **atlas** (7204 symbols, 17956 relationships, 219 execution flows). Use the GitNexus MCP tools to understand code, assess impact, and navigate safely.

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
- **返工上限**：每次从 `rejected`/`review_fix` 重派回 Dev 时 `rework_count += 1`；
  超过 `max_rework`（config，默认 3）不再重派，转 `blocked_human` 并在 plan.md 头部
  高亮，附历史 reject_reason 汇总——反复返工 3 次以上大概率是 done_criteria 本身
  不可实现或自相矛盾，继续机器循环只会烧 token。

同一 task 任意时刻只有一个 owner，配合原子写即可避免竞争。

## 认领协议（超时重派防双写）

「owner 移交顺序发生」在一个场景下不成立：Leader 因 assigned 超时重派，而原 Dev
恰好迟到认领。用 epoch + 锁临界区机制性消除：

- **所有对 task JSON 的「读-校验-写」必须在任务锁临界区内完成**：

  ```bash
  bash .claude/hooks/with-task-lock.sh TASK-001 <读 → 校验 → 写临时文件 → mv 覆盖>
  ```

  （辅助脚本优先 flock；无 flock 的环境如 macOS 自动退化为 mkdir 自旋锁。）

- **Leader 每次（重）派**：临界区内 `assignment_epoch += 1`，同时写 `assigned_to`
  与 `status=assigned`。
- **Dev 认领**：临界区内读取 → 校验 `(status==assigned && assigned_to==自己)` →
  写 `status=in_progress`，原样带回读到的 epoch。
- **Dev 后续每次写该 task 文件**：临界区内重读，epoch 与自己持有的不一致 = 已被
  重派，**立即放弃（不写）**，回到任务扫描循环。
- Leader 收到 `dev_done` 时校验 epoch：不一致则忽略（过期 owner 的迟到产物）。

裸的「写前重读」只是缩小竞态窗口；读-校验-写必须原子，才不依赖 agent 自觉。

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
go run ./validator/cmd/arcforge-validate .arcforge/tasks
```

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
