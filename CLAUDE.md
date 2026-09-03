<!-- gitnexus:start -->
# GitNexus — Code Intelligence

This project is indexed by GitNexus as **atlas** (15494 symbols, 40312 relationships, 300 execution flows). Use the GitNexus MCP tools to understand code, assess impact, and navigate safely.

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

- **开工前读 `PENDING-MECHANISMS.md`**（待决机制变更，≤40 行）——上一轮留下的、等人拍板的
  机制问题都在那里。⚠️ 它刻意保持短：`wisdom/decisions-leader.md` 与 `wisdom/_digest.md`
  已分别涨到 33KB / 91KB，**「处方写下了却没人读得到」是本框架实测过的失效模式**。

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
- 🔴 **派发/派验的通知里加一句「请回一句确认收到」。** 一句话，把「静默失败」变成
  「有响应的失败」——通知丢了和「收到了正在做」在 Leader 侧**完全同形**（任务状态、
  产物、实例是否 idle 都一样），这正是 `stale-dispatch` 需要 30 分钟阈值 + 双条件判据
  的原因。有回执时「没收到」在几分钟内就由**沉默本身**报警，不必等阈值。
  ⚠️ 它**不防止**通知丢失，只把发现丢失的时间从阈值级压到一句话。
  **实测（atlas M1c-4）**：前两次派验不加，分别丢了 **48 分钟**与 **138 分钟**（后者
  还因 Leader 用一个只在成功时出声的监视器顶替了周期扫描而放大）；加这一句之后
  **四次派验全部当场回执**。

#### 派发后活性确认（`stale-dispatch` 告警的处置流程）

**为什么需要**：`TeammateIdle` 是「teammate 转 idle 前的**一次性**拦截点」，不是周期性心跳。
放行 idle 后实例即停机，在收到新消息前 hook 永不再触发。于是链路成了「正确放行 idle →
实例停机 → Leader 派活 → 通知丢失 → **没有任何机制让它醒来**」——已停机的实例读不到任何
文档，角色文档里的重扫循环只覆盖「实例还活着」的情况。atlas sprint-028 实测一次停机
89 分钟，最后靠人工发现。`blocked_clarification` 是同源的第二条链路：Leader 答复后若通知
丢失，那个 Dev 同样无法自愈。**不要试图靠改 hook 兜住**——改成「宁可保活」会让无关实例
空转烧 token。出路只在 Leader 侧的周期性校验。

**怎么发现**：每轮进度扫描跑一次 validator，读 `⚠` 段里的 `stale-dispatch`。判据是两个条件
的合取：「在 `assigned` / `verifying` / `blocked_clarification` 停留超阈值」∧「**本轮派发**无产物」。

- **当前生效的阈值以 `arcforge-validate --rules` 的输出为准**——它从代码里的阈值表直接读，
  不是副本。此处不复述具体数值：改了表而没改文档时，文档会安静地变成假的。
- 阈值实测自 atlas 归档 `transitions.jsonl` 的驻留分布（取值论证见 `validator/liveness.go`；
  完整分布见 `discoveries/TASK-010.json`）。`in_progress` 刻意不设阈值——dev 正常干活实测
  p90 就有 65 分钟，任何可用阈值都会把正常长任务淹没在告警里。
- 「本轮产物」= owner 自己的 checkpoint 提及该 task id，或 `discoveries/<ID>.json` /
  `docs/04-test/<ID>-verification.md` 存在，**且**修改时间不早于本轮进入该状态的时刻。
  两个条件缺一不可：只看内容会被**上一轮**留下的旧产物消音（atlas TASK-016 恰是这个形态，
  它在被派验前 60 分钟刚被同一个 verifier 判过 rejected）；只看时间就退化成 agent 的全局
  活动痕迹，owner 为别的任务更新 checkpoint 就会假装本任务活着。
- 规则是**告警级不阻断**：「慢」与「死」在文件层面不可区分，硬失败会在正常长任务上误杀
  整个 wave（`verifying` 实测 max 425 分钟）。命中不影响退出码。
- `blocked_clarification` 只在 `questions[]` **全部已答复**时才可能命中——未答复时 Dev 是在
  合法等待你，此时告警是指错人。

**怎么处置——「重发一次」与「直接改派」的分界**（AD-21 的启用判据是「联系不上且唤不回」，
不是「看起来停了」；跳过第 1 步会把还活着的实例的活抢走，白干加双写）：

| 步骤 | 触发条件 | 动作 | 不要做什么 |
| --- | --- | --- | --- |
| 第 1 步 | validator 首次报该任务 `stale-dispatch` | 向该实例**重发一次**消息，附 task id 与它该做的下一步 | 不要直接收回——「还活着但没写 checkpoint」的实例正是这个样子 |
| 第 2 步 | 重发后再等**一个完整阈值周期**，仍无本轮产物 | 判定「联系不上且唤不回」，走逃生边收回改派（见下） | 不要跳过第 1 步；也不要在第 1 步就改 `assigned_to`/`verifier` |

第 2 步的逃生边随状态而异（三者都在状态机出边表内，不要临时发明）：

- `assigned`：`transition assigned --field assigned_to=<新实例>`（自环重派，`assignment_epoch += 1`）。
- `verifying`：`transition verifying --field verifier=<新实例>`（自环改派验证者，`verify_baseline` 不刷新）。
- `blocked_clarification`：**没有重派逃生边**。合法出边只有 `dev-*` 的 `in_progress` 与 `leader` 的
  `blocked_human`，所以你只能重发答复通知；确认唤不回就转 `blocked_human` 交人类，
  **不要试图改回 `assigned`**——那条边不存在，写通道会 DENY。

### 6. 质量门禁

- 全体任务 `verified` 后，spawn QA Agent 做 Code Review（两轮：常规 + 跨视角对抗）。
- 根据 Review 结果决定是否需要修复迭代（最多 `code_review.max_iterations` 轮）。

---

## 任务状态机

每个 `tasks/TASK-xxx.json` 的 `status` 字段在以下状态间流转（owner = 唯一可写者）：

| 状态 | 含义 | owner |
| --- | --- | --- |
| `pending` | 已拆分，未分配 | Leader |
| `assigned` | 已派给某 Dev（`assigned_to`） | Leader |
| `in_progress` | Dev 正在 TDD | Dev |
| `dev_done` | TDD 完成且通过 hook 门禁 | Dev |
| `verifying` | Test Agent 验证中 | Test |
| `verified` | done_criteria 逐条通过 | Test |
| `rejected` | 验证不通过（带 `reject_reason` + `reason_class`） | Test |
| `review_fix` | QA 发现问题需返工（带 `fix_items` + `reason_class`） | Leader |
| `blocked_clarification` | Dev 对 done_criteria 有疑问，已写入 `questions[]` 等 Leader 答复 | Dev 提问；Leader 答复后**由 Dev 自己**转回 `in_progress`（epoch 不变） |
| `blocked_human` | 返工超限或 CONTESTED，需人类介入（`/arcforge-status` 高亮） | Leader |
| `accepted` | 最终验收通过（终态） | Leader |
| `skipped` | 依赖被永久放弃，跳过 | Leader |

**正常流转：** `pending → assigned → in_progress → dev_done → verifying → verified → accepted`

**返工与澄清环：**

- 验证不过：`verifying → rejected → assigned → ...`
- QA 退回：`verified → review_fix → in_progress → ...`
- 澄清环：`in_progress → blocked_clarification → in_progress → ...`（Leader 周期扫描答复；
  **答复后由 Dev 自己转回 `in_progress`，不是 Leader 改回 `assigned`** —— `blocked_clarification`
  的合法出边只有 `→ in_progress`（dev-*）与 `→ blocked_human`（leader），**没有 `→ assigned`**）
- **Leader 调度边（均 leader 专属，置于 `rejected → assigned` 之后）**：
  `assigned → assigned`（`assigned` 超时**重派**，`assignment_epoch += 1`）、
  `in_progress → assigned`（**收回**卡住任务重新分配，`assignment_epoch += 1`）、
  `rejected → blocked_human`（**熔断**，不改 epoch）。
- **Leader 逃生边（`verifying → verifying`，leader 专属，AD-21）**：verifier 失联且**不可恢复**时，
  Leader 经 `task <ID> transition verifying --field verifier=<新实例>` **一步**收回并改派验证者。
  `assignment_epoch` **不变**——epoch 是 dev 侧的认领凭证，verifier 不持有它；`assigned_to` 同样不变。
  与 `assigned → assigned`（Leader 超时重派 Dev）同构。不带 `--field` 的裸自环是无害 no-op（只留审计行）。
  **不要**改用 `verifying → dev_done` 退回：写通道对 `dev_done` **无条件**执行 dev_done 门禁，
  声明包测试红时逃生动作本身会被 DENY 回原地，死锁原样复现。
- **返工上限**：从 `rejected`/`review_fix` 重派回 Dev 时 `rework_count` 由写通道**自动 +1**
  （Leader 不再手工维护）；`reason_class=env_infra`（限流/超时/环境故障）的重派**不计数**——
  那是环境问题不是任务问题。超过 `max_rework`（config，默认 3）不再重派，转 `blocked_human`；
  `dod_defect`（done_criteria 自身矛盾）累计第 2 次直接转 `blocked_human`——继续机器循环
  只会烧 token；`no_progress`（doom-loop）重派时**必须**更换提示词或换 Dev 实例。

**状态机的完整出边表**（共 21 条，与 `write-matrix.json` 的 `transitions` **逐条一一对应**；
下游实测过「正文写了某条边而边表没有」导致 Leader 与 Dev 一起做错动作，故此处两两可核对。
`tests/hooks/test-arcforge-write.sh` 的 E3 组按「行首竖线 + 反引号包裹的 `源状态 → 目标状态`
- 竖线」这一格式解析本表，与矩阵做**双向集合相等**比对，「合法写者」列做**等值**比对
——**改矩阵必须同时改本表，且两份 CLAUDE.md 都要改**，否则测试直接红。散文里的箭头链、
bullet 举例、上面那张状态表都不匹配该行首格式，不会被解析成边表条目）：

| 出边 | 合法写者 | 用途 | 漂移约束 |
| --- | --- | --- | --- |
| `pending → assigned` | `leader` | 派发给某 Dev（写 `assigned_to`，`assignment_epoch += 1`） | — |
| `pending → skipped` | `leader` | 依赖被永久放弃，未派发即跳过 | — |
| `assigned → in_progress` | `dev-*` | Dev 认领（须 `assigned_to == 自己`），并记下 `assignment_epoch` | — |
| `assigned → assigned` | `leader` | **重派**：`assigned` 超时无人认领，`assignment_epoch += 1` | — |
| `assigned → skipped` | `leader` | 依赖被永久放弃，已派发但未开工 | — |
| `in_progress → dev_done` | `dev-*` | TDD 完成，经 dev_done 门禁 | — |
| `in_progress → blocked_clarification` | `dev-*` | 对 `done_criteria` 有疑问，写入 `questions[]` | — |
| `in_progress → assigned` | `leader` | **收回**卡住的任务重新分配，`assignment_epoch += 1` | — |
| `blocked_clarification → in_progress` | `dev-*` | Leader 答复 `questions[]` 后，**由 Dev 自己**领回继续（`assignment_epoch` 不变） | — |
| `blocked_clarification → blocked_human` | `leader` | 澄清也解决不了，升级人类 | — |
| `dev_done → verifying` | `leader` | 派验（写 `verifier`） | **写入基线**：自动记 `verify_baseline` = `{head, discovery_sha256, at}` |
| `verifying → verified` | `test-*`，且须是本任务 `verifier` | 验证通过 | **受约束**：须与 `verify_baseline` 一致，否则要显式确认 |
| `verifying → rejected` | `test-*`，且须是本任务 `verifier` | 验证不通过，带 `reject_reason` + `reason_class` | **不受约束**（判不过时交付物变没变都不影响结论） |
| `verifying → verifying` | `leader` | **逃生边**：verifier 失联不可恢复时改派验证者，见上 | 不受约束，且**不刷新**基线 |
| `rejected → assigned` | `leader` | 返工重派，`rework_count` 自动 +1（`env_infra` 不计数） | — |
| `rejected → blocked_human` | `leader` | **熔断**：返工超限或 CONTESTED | — |
| `verified → review_fix` | `leader` | QA Review 发现问题需返工，带 `fix_items` + `reason_class` | — |
| `verified → accepted` | `leader` | 最终验收通过（终态） | — |
| `review_fix → in_progress` | `dev-*` | Dev 领回返工 | — |
| `blocked_human → assigned` | `leader` | 人类介入后恢复，重新派发 | — |
| `blocked_human → skipped` | `leader` | 人类判定放弃该任务 | — |

**验证对象漂移告警（`verify_baseline`，AD-29）**：下游 atlas 实测过一次「判定对象与最终交付物
不是同一个 commit」——TASK-018 的 dev 在 `dev_done` 之后 6 分钟又提交了一版，verifier 87 秒后
判 VERIFIED，**没有任何机制会告警**。那次实质无碍（diff 仅注释），但**若 dev 改的是断言，
VERIFIED 就失效了而无人知晓**。

- `dev_done → verifying` 时写通道自动写入 **`verify_baseline`** = `{head, discovery_sha256, at}`
  ——承接那一刻的 `git rev-parse --verify HEAD` 与 `discoveries/<ID>.json` 的 sha256。
  该字段**由脚本管理，禁经 `--field`/`--json-field` 写**（与 `assignment_epoch` 同待遇：
  能被一行命令抹掉的守卫不是守卫）。
- `verifying → verified` 时比对：一致放行；不一致则打出 `--stat` 差异并**要求显式确认**：

  ```bash
  task <ID> transition verified --ack-drift <当前 HEAD 全 sha> [--ack-discovery-drift <当前 sha256>]
  ```

  确认值取**当前值**而非记录值——必须真去看过现状才填得出。两个参数只在这条边合法，
  用在别的边上一律 DENY（不沉默吞无意义参数）。
- 判据**收窄为「声明范围内的文件变了」**（`writes` 优先，字段缺失才回落 `packages`，
  与 `task-completed.sh` 同口径）：多 dev 并行时他人提交推进 `HEAD` 是常态，全量告警会让
  确认退化成条件反射，真信号被噪声淹没。范围外的 HEAD 前进只打 INFO 放行。
- 非 git 仓库 / 无提交（fresh init）时基线记空，转 `verified` 时跳过校验但打 **WARN**
  ——有声跳过，不静默。本机制上线前就已在 `verifying` 的存量任务同样 WARN 跳过，不死锁。

**与 discovery 时机守卫（TASK-002）的分工**：后者堵的是**判定之后**（`verified`/`accepted`
禁止覆盖既有 discovery）；`verify_baseline` 堵的是**判定之中**（`verifying` 窗口）——本 sprint
实测过一次：Leader 派验后 53 秒 discovery 即被改写，改动恰好落在 `verification.suite` 这个
DoD 自证字段。两者与 `assignment_epoch` 同思路：**不禁止变化，只保证变化不会被静默吞掉**。

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

### `packages` 与 `writes`：两个口径，别混用

同一份范围声明要服务两件**目标相反**的事，故拆成两个字段：

| 字段 | 口径 | 消费点 | 回答的问题 |
| --- | --- | --- | --- |
| `packages` | 覆盖率口径（**宽**） | `task-completed.sh` 的 `go test` / `-coverpkg` | 我的改动**应当被哪些包的测试覆盖** |
| `writes` | 互斥口径（**窄**） | validator 的 `scope-empty`/`scope-mutex`；`task-completed.sh` 的 scope 漂移与无代码任务变更判定 | 我**会写哪些路径** |

- **`writes` 缺省 = `packages`**：字段缺失时两个口径合一。这是绝大多数任务的形态，也是
  归档里 200+ 个历史任务的形态（它们的校验结论因此逐字不变）。
- **什么时候该让它们不同**：本任务**只读消费**某个包——要照着它的接口写代码、它的测试
  应当算进覆盖率，但**一行都不改它**。此时把它写进 `packages`、不写进 `writes`：覆盖率
  照常统计，而该包的其他任务可以并行。不拆时这类任务会平白独占它（下游 atlas 的
  TASK-011 就是被这一条卡住的）。
- **`writes: []`（显式空）不等于字段缺失**：显式空是「本任务不写任何文件」的声明，不回落，
  validator 判 `scope-empty` 阻断。理由是门禁的漂移检查会把他人在途 scope 从实际改动里
  减掉，不占任何范围的任务其真实写入对门禁隐形——纯调研任务请把产出的报告/文档路径
  写进 `writes`，而不是留空。
- **`writes` 不必是 `packages` 的子集**：写 `*.md`、`*.sh` 这类不该进 `-coverpkg` 的产物很
  正常（`packages` 会被原样喂给 `go test`，把非 Go 路径塞进去等于弄坏覆盖率门禁）。越界
  时 validator 只出**告警级** `scope-writes-outside-packages` 提示「是不是笔误」，不阻断。

拆任务时的做法：先按覆盖率口径列 `packages`，再逐条问「这里面哪些我一行都不会改」，
把余下的写进 `writes`；两者相同就删掉 `writes`。

#### 改声明范围只有一条路：`update`

`packages` / `writes` **禁止经 `transition` 写**（写通道 DENY，文案指向 `update`）。原因是
迁移审计行只记 `from`/`to`，`transition <状态> --json-field packages='[…]'` 这条路径会让范围
变更**完全不留痕**——一个 flag 就绕过 `transitions.jsonl` 的全部范围变更审计。而这不是对抗
场景专属：把状态推进与范围声明写进同一条命令是会被**误撞**的正常写法。正确写法是拆两步：

```bash
arcforge-write.sh --as <me> task <ID> update --json-field packages='["a","b"]'   # 留 added/removed 审计
arcforge-write.sh --as <me> task <ID> transition <目标状态> --expect-epoch <N>
```

**堵掉这条旁路之后**，「`transitions.jsonl` 记录所有 `packages`/`writes` 变更」这句话才成立
（在此之前只有 `update` 路径成立，别据它做推断）。

#### 越界申报：dev 必须在 `dev_done` 之前自己落盘

**dev 若改了 `packages` / `writes` 声明之外的文件，必须在 `transition dev_done` 之前**经写通道
（`task <ID> update --json-field writes='[…]'`）把该文件补进声明，并在交付报告里申报；Leader 的
「批准」是**事后确认**，不批准则走 `rejected` 让 dev 撤回。

**为什么不能沿用「dev 申报 → Leader 批准 → dev 补写」**：那条回路**任何时序下都关不上**。
owner_table 决定了任务一进 `dev_done`，**leader 就再也写不了该任务文件，直到 `accepted`**；
而 dev 一旦交棒进 `verifying` 也失去写权。批准必然发生在 dev 交付之后，**那一刻没有任何角色
能合法补这个字段**（实证：dev 被 DENY「合法写者: `["test-*"]`」，Leader 亲自试也被同一条 DENY
拦下——`guard_field_key` 那层确实放行 leader，但 owner-table 那层先拦，两层是与的关系）。

**为什么这是安全属性而非文档洁癖**：`writes` 同时驱动 scope 互斥与漂移判据，
**声明与实际写入不一致 = 范围外的真漂移不会告警**。

## Go validator（机制级保证）

`validator/` 提供任务图校验器。Leader 在拆分后、每次 wave 放行前运行：

```bash
bash .claude/scripts/validator-run.sh validate .arcforge/tasks
```

（exit 127 = validator 未分发，回退手工统计。）

校验规则：DAG 无环、wave 序、完成必有产物、失败必有原因、skip 传播、单 owner 不变量、
context_from 闭合、epoch 不变量、在途任务 scope 非空且互斥、blocked_clarification 必有
questions。非零退出码表示任务图存在问题，必须修正后才能继续。

**告警级规则**（命中不影响退出码，只打在 `⚠` 段）：`orphan-obligation`（义务写在
description 而不在 DoD）、`scope-writes-outside-packages`（writes 越出覆盖率口径）、
`stale-dispatch`（派发后活性确认，处置流程见上面第 5 节）。告警是给人看的，别当噪声划过去。

## 每 dev 独立 worktree（隔离协议）

**人类决策（2026-07-27）：每个 dev 在自己的 `git worktree` 里干活。** 立项依据是一个 sprint
内四起同根因事故（就地变异污染共享文件、无 pathspec 的 `git commit` 扫走别人已暂存的文件、
变异中的文件正好是别人要改的、`tests/hooks` 固定写 `/tmp/…` 致假 PASS），**三起靠事后撞上、
一起靠偶然，没有一起是机制挡下的**；而全 sprint 唯二没出问题的运行，都主动用了 worktree。

| 做什么 | 在哪 |
| --- | --- |
| 编辑代码、跑测试、跑变异 | 自己的 worktree |
| 一切 `.arcforge/` 读写:**cd 回主仓库** | 主仓库（唯一真相源） |
| 提交 | 自己的 worktree，分支 `task/<TASK-ID>` |
| 合并回 `main` | 由 **Leader 串行执行** |

```bash
# 开工（在主仓库里执行）
git worktree add -b task/<TASK-ID> ../wt-<TASK-ID> main
# …在 worktree 里编辑 / 跑测试 / 跑变异 / 提交（显式 pathspec）…
# 收尾（回主仓库执行，谁建谁拆）
git worktree remove ../wt-<TASK-ID>
```

### ⚠ 隔离引入一个新的静默失败模式（已机制堵上）

worktree 只共享 `.git`，而 **`.arcforge/` 是已跟踪目录，会在每个 worktree 里按 checkout 的
commit 铺开一份独立副本**——它可能陈旧、可能残缺（按更早 commit 建的 worktree 里，之后才落盘
的任务文件根本不存在）。在 worktree 里调用写通道，写入落到那份副本上，**主仓库真相源一个字节
不变、退出码仍是 0**。这比它要解决的并发问题更隐蔽：**「文件系统是唯一真相源」这条根基会被
静默架空。**

故 **`linked worktree 中调用写通道会被 DENY`**（`arcforge-write.sh` 主模式入口，先于矩阵检查，
覆盖全部子命令）。判别式是 `git rev-parse --git-dir` 与 `--git-common-dir` 是否相等：主工作区
相等（都是 `.git`），linked worktree 前者为 `<root>/.git/worktrees/<name>`、后者为 `<root>/.git`。
DENY 文案给出主仓库绝对路径与可直接照抄的 `cd`。

**判别不出来时放行（fail-open）**，与「矩阵缺失 fail-closed」刻意相反：矩阵是授权依据，缺了
无从判权必须拒；而本判别只回答「我在哪」，**一次误判就阻断唯一写入通道上的全部写入**。

⚠ **这是代价权衡，不是「不漏防」。** 早先此处断言那三种成因「都蕴含 linked worktree 不可能
存在」，**那是错的**——只有「非 git 仓库」成立；`git` 不在 PATH 与 `rev-parse` 报错（孤儿
worktree）**都可以发生在一个已经存在的 worktree 里**，实测均静默写坏副本且 `exit 0`。错因是
把「**我现在探测不到**」当成了「**它不存在**」：worktree 是**过去**由一个能用的 git 建的，与
**此刻** git 能否使用无关。故补一道与 git 无关的兜底探针（`.git` 是 `gitdir:` 形态的**文件**），
仅在主判别式无法决断时生效；该探针对 git **子模块**同样命中，是已知假阳，详见代码注释。

### 孤儿 worktree 的清理责任

- **谁建谁拆**：任务收尾（`dev_done` 报告发出后）由建它的 agent 自己 `git worktree remove`，
  这是交付动作的一部分，不是可选的收尾。
- **agent 中途失联时**：残留 worktree 由 **Leader** 收——Leader 在阶段边界跑 `git worktree list`，
  对已 `verified`/`accepted`/被收回重派的任务，其残留一并 `git worktree remove --force` 后
  `git worktree prune`。**这类残留不会自己消失，也不属于任何在世 agent，无人认领就随 sprint 累积。**

### 基线采样纪律（背对背 × 同等负载，两条都要）

共享工作区里任何「改动前 vs 改动后」的 A/B 比较，必须**同时满足**两个条件才算对照；只满足
其一的是两次独立采样，不能当对照结论。这两条的**失效方式相反**，只记一条会漏掉另一半：

| 纪律 | 防的是 | 产生的错误 | 实撞 |
| --- | --- | --- | --- |
| **背对背**：新旧两份二进制/脚本**同一时刻背对背**并排跑 | 基线漂移——时间轴上环境变了 | **假差异** | 跨 12 分钟采样得到 4 处假差异，全由并发 agent 期间新建文件造成；背对背重跑后逐字节一致 |
| **同等负载**：两组承受逐轮相同的负载 | 负载不等——两批同时可跑但压力不同 | **假无差异** | 某版本 10 次全绿被读成「改动前没这个缺陷」，实为那一批没有并发负载；同轮 2×A+2×B 后立刻红 6/12 |

**「假无差异」比「假差异」更危险**：假差异会被追查，假无差异直接变成「已证明没问题」而无人再看。

**推论**：任何跨时间点保存的**绝对指纹**（sha、行数、发现条数）在共享或演进的语料上都不构成
基线，只有**同一时刻并排产生的一对**才构成。判据是「pre 与 post 两份输出彼此逐字节一致」，
不是「sha 与上一轮相同」——拿历史绝对值去比会得到假差异，只是伪装成了「有据可查的历史值」。

#### 验证/隔离命令里的锚必须钉全 sha，不得写 `HEAD` 或分支名

交接文档、checkpoint、discovery 里凡是给出「怎么复现这次验证」的命令，其中指向仓库状态的
锚**一律写全 sha**。`HEAD`／`main` 这类符号引用是**会过期的锚**：写下时它指向 A，别人照跑时
它已指向 B，而命令原文一个字都没变。

**这条与「checkpoint 产物指针必须带 commit sha」是同一条规则，但危害更大，所以单列**：

| | 过期时会发生什么 |
| --- | --- |
| checkpoint 指针过期 | **失效**——接手者比对 HEAD 就会发现不一致，然后去查 |
| **验证命令过期** | **主动产出假红**，而且那份红会被当成判定依据 |

实撞（TASK-015）：交接文档给的隔离命令是「把依赖钉在 `HEAD`」，写下时 `HEAD=af038eb`，确实
隔离了当时在途的 TASK-018；TASK-018 提交后 HEAD 前移，**同一条命令静默从「隔离」变成「包含」**，
套件由 `577/0` 变成 `571/6`。验证者若照抄，会据此 reject 一个无关的交付。

**推论**：变异 / 回归 harness 里的依赖锚要做成**可覆写**的（如 `ARCFORGE_MUT_DEP_REF`），
让人不改文件就能取到干净对照组。

**写下纪律和用上纪律是两件事。** 立此条的那次交付本身就是证据：**同一个提交里，一边把
「指针必须记全 sha，不是短 sha、不是会漂的引用」写进两份角色定义（连理由都写了——「指针写下
时正确，不等于你读它时仍然正确」），一边把 harness 的隔离锚点钉成了 `HEAD`。** 规则和违反
规则的行为出自同一次交付、同一个人之手。⇒ **写下纪律不产生遵守纪律的能力**，要靠机制：
锚点做成可覆写变量、把实际锚点与工作树 HEAD 打进报告，让「测自哪棵树」不依赖人的自觉。

## 矩阵字段登记规则（模板 ↔ 运行时同步）

矩阵新增「只存在于运行时」的顶层字段（如 `tokens` 这类 session 状态）时，必须同时登记进
`project-template/scripts/check-runtime-sync.sh` 顶部的 `RUNTIME_ONLY_FIELDS` 常量，否则
模板↔运行时同步检查会对该字段恒报 DRIFT。判据刻意是**显式黑名单**：漏登记 = 可见噪音
（安全方向），而**白名单式漏登记 = 静默不比对**（假绿）。

## 变异测试纪律（变异必须作用在隔离副本上）

变异测试（AD-27/28）证明断言真的在守卫，但**变异手法本身**在共享 worktree 里有个静默的坑：
就地变异真实源文件，会让**并发 agent 读到变异态**，拿到与自己改动无关的假红。实测两例：
dev-agent-44 跑回归时撞见 15 个 FAIL 全部指向 `verifying->verifying`，成因是当时有 3 个变异
harness 在就地改 `templates/write-matrix.json`；`validator/` 的 harness 同样是 10 变异 × 3 次
≈ 30 个窗口。**假红最坏的后果是被误记成真缺陷**，而受害者只看到「重跑就好」，是最难归因的
一类。「就地 + 还原」再严密也只是**缩小窗口，不是消除**。

**约定**：变异一律作用在隔离副本（`mktemp -d` 拷贝，或 `git worktree add` 到临时目录）上，
被测脚本经环境变量指向副本，主工作区文件一个字节都不碰。可照抄的写法：

```bash
TMPD=$(mktemp -d); trap 'rm -rf "$TMPD"' EXIT INT TERM
WORK="$TMPD/work"; mkdir -p "$WORK/hooks"
cp "$ROOT/templates/write-matrix.json"  "$WORK/write-matrix.json"
cp "$ROOT/CLAUDE.md"                    "$WORK/CLAUDE.md"
cp "$ROOT/templates/CLAUDE.md.template" "$WORK/CLAUDE.md.template"
cp "$ROOT"/project-template/hooks/*.sh  "$WORK/hooks/"
BASE_ENV=(ARCFORGE_MATRIX_TPL="$WORK/write-matrix.json"
          ARCFORGE_DOC_ROOT="$WORK/CLAUDE.md"
          ARCFORGE_DOC_TPL="$WORK/CLAUDE.md.template"
          ARCFORGE_WRITE_SCRIPT="$WORK/hooks/arcforge-write.sh")
# 对照组与变异组走同一组 env（A/B 必须同环境同负载），变异只改 $WORK 下的副本
env "${BASE_ENV[@]}" bash "$SUITE"
```

配套三条闸（都是实撞出来的）：

- **有效性闸**：变异体落盘后先 `bash -n`（shell）/ `jq -e .`（JSON）验一遍。崩溃型变异语法
  非法会让套件**全面**变红，被误记成 KILLED——sha256「变异确实生效」只挡得住对偶方向的一半。
- **语义闸**：语法闸挡不住语义走样。实撞过漏写续行反斜杠，`bash -n` 照过但变量恒空，变异以
  **错误理由**致红同样记 KILLED。故 harness 必须打印变异体与原文的 diff，落盘前逐字核对。
- **主工作区指纹**：每个变异窗口内 + 收尾各校验一次「被变异文件在主工作区的 sha256 与
  `git status --porcelain` 输出均未变化」。用**指纹比对**而非「必须干净」——sprint 进行中
  工作区本就有未提交改动，「干净」这个判据在真实场景里恒假。
- **整文件重写的隐患**：共享工作区里用「整文件读入 → 修改 → 整文件写回」的方式改共享文件，
  **若写回那一刻文件正处于变异态，变异会被固化进提交**；而变异 harness 随后的还原只比对
  它自己的快照，**不覆盖这种情况**——它比的是自己的快照，不是仓库历史。⇒ **「harness 报还原
  成功、工作区干净」不足以排除这个风险。** 故：**优先行级编辑**；必须整文件重写时，跑
  `git diff --numstat <base> <head> -- <file>` 自检，删除行数应与预期一致（纯新增则为 0）。
  这比「跑之前用 `ps` 查变异进程」可靠——`ps` 只查得到那一瞬间，numstat 是对结果的事后
  确定性检查。
  **时点必须是「最后一次改动之后」，不是「每次写回之后」**：自检跑过之后再改一行，结论就
  失效了，而它看上去仍然是一份跑过的自检。实撞（TASK-015）：numstat 跑在补 `assert_contains`
  的 `--` 之前，补完没重跑，于是 discovery 里记着「+0 删除（纯追加）」而实际是 `236/1`——
  **交付前以提交本身的 `git show --numstat HEAD` 为准复核一次**，那份数字才和交付物同源。

#### 一切自证数字必须在最后一次改动之后统一重采

上一条不止适用于 numstat。**discovery / 交付报告里的每一个自证数字（断言数、覆盖率、变异
KILLED 数、外溢度、行数、sha）都必须在最后一次改动之后统一重采**，分批采的数字即使每个在
当时都对，合到一份报告里也不构成对最终产物的自证。

**理由是它的读者**：自证数字是**验证者的判定原料**。跨时间点采的数字之所以危险，不在于它
不准，而在于**它在报告里长得和同时刻采的一模一样**——验证者无法从数字本身看出它测的是哪
棵树。同一份 TASK-015 交付里这个形状犯了两次：numstat `236/1` 与断言数 `576 vs 577`，两次
都不是漏做检查，而是**采样时点早于最后一次改动**。

⇒ 机制化：让 harness 把**实际锚点与工作树 HEAD 打进输出**，使「这批数字测自哪棵树」成为
报告的一部分，而不是靠作者记得声明。

- **外溢闸命中即早退**：变异 harness 的有效性闸（语法 / 可执行位 / 作用域外红过半）一旦命中，
  必须**立刻 return**，不得继续打印 `KILLED`。实撞：某 harness 的「作用域外红过半」闸不早退，
  输出里于是留着 `PASS: M6 KILLED(1/1,确定性)` 这行**假话**，只有 FAIL 行和退出码说真话——
  而这道闸的全部意义正是别让假 KILLED 立住。

## 知识累积

- **`.arcforge/discoveries/TASK-{id}.json`**：每任务完成时由 owner 写一份结构化发现
  （key_findings / decisions[带 rationale] / files_modified / interfaces_exposed / verification），
  下游通过 `context_from` 按需读。
- **`.arcforge/docs/04-test/TASK-{id}-verification.md`**：Test Agent 的验证报告，**文件名
  以此为准**（含 done_criteria 覆盖矩阵 + 证据 + 结论）。角色定义（`global/agents/test-agent.md`）、
  技能卡（`skills/test-verification/`）与本文件三处必须同名——本 sprint 实测过一次不一致的代价：
  验证者按角色定义写了一份 `-test-report.md`、按 Leader 要求又写了一份 `-verification.md`，
  留下一份内容完全相同的重复文件，而**写通道只能创建/覆盖、不能删除**（子命令里根本没有删除），
  write-guard 又禁止直接 `rm`（Leader 同样被拒），最后只能靠人类在会话外清理。
- **`.arcforge/wisdom/`**：`learnings-{instance}.md` / `decisions-{instance}.md`
  ——各实例只追加写**自己的**文件（多进程并发 append 同一文件超过 PIPE_BUF 无原子性）；
  Leader 在阶段边界聚合生成 `_digest.md`（只读参考）。原共享的
  `conventions.md` / `issues.md` 改为**仅 Leader 写**（单写者，聚合自各实例文件与 inbox）。

## Context 持久化纪律（CLM）

你无法可靠判断自己的 context 是否被压缩，因此**不要主动压缩**，而是养成「持续落盘」习惯：
在每个自然节点把当前状态写入 `.arcforge/checkpoints/{your-agent-name}-checkpoint.md`。
被动压缩发生后，按各角色定义中的「被动压缩恢复协议」从 checkpoint + `.arcforge/docs/` 重建上下文。
checkpoint 摘要必须带原始产物指针；恢复时指针优先于摘要（有损摘要 + 无损指针）。

**指针必须带 commit sha（人类 2026-07-27 选定）**：写入时在「原始产物指针」一节记下
`git rev-parse HEAD` 的**全 sha** 与**分支名**；**恢复/接手的第一步是比对当前 HEAD 与记录值**，
不一致就先 `git log --oneline <记录值>..HEAD` 看中间发生了什么，再决定哪些指针仍然有效。

立项依据（本 sprint 真实发生）：某 dev 三次失联后由他人接手，前任 checkpoint 的产物指针
**写于 `f1b3764`、被读取于 `453ec4a`**，中间静默漂移了 **3 个 commit**（`369c312` /
`5317464` / `453ec4a`）而没有任何机制告警；同一份
checkpoint 还把分支记成了 `master`（本仓库是 `main`）。**接手者是靠自己核对才发现的，不是被
提醒的。** 这与验证对象漂移（`verify_baseline`）是同一失效模式——判定/交接的依据在被使用时
已经不是写下时的那一份，只是载体从 verifier 的判定对象换成了 checkpoint 的指针。

**为什么这条只做纪律、不做机制**（人类选定，理由一并记明）：真要机制化（写通道校验
checkpoint 里的 ref）成本远超收益——触发条件是「agent 失联 + 交接」，比其它几条罕见得多；
而 checkpoint 本就是给人/agent 读的自述文档，加一行 sha 与一步比对即可闭合。

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
