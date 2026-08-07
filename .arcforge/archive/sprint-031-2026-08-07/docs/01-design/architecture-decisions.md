# 架构决策记录（Leader 复核与补充）

设计层面的决策已由 `docs/superpowers/specs/2026-08-06-collector-policy-gate-design.md` 定稿。
本文件只记录 **Leader 在 Arcforge 流程层面**做出的、方案未覆盖的决策。

## AD-1：不重跑需求分析，降级为方案解析

**决策**：跳过 ECC `/multi-plan`（`capabilities.ecc=false`）**与** superpowers `brainstorming`。

**理由**：输入本身是 `writing-plans` 定稿方案 + 独立设计文档 + 自查覆盖矩阵，设计空间已收敛并含逐任务代码。重跑发散式设计会推翻已定稿结论，是净损失。Leader 改为做**自洽性复核**（依赖链、签名演进、偏离理由、覆盖矩阵抽查），结论见 `requirements-analysis.md` §4。

**代价**：放弃了「独立视角挑战设计」的机会。**缓解**：Step 3 的独立 reviewer 仍然只读需求文档反审 DoD，保留了一道独立视角。

## AD-2：Task 5 拆为 T5a / T5b，Task 12 拆为 T12a / T12b

**决策**：方案的 12 个任务落为 **14 个** Arcforge 任务。

**理由**：Realistic Scope（≤1 package / ≤5 文件 / ≤8 条 DoD）。方案 Task 5 有 9 个文件、跨 `policy`+`config`+`cmd/atlas` 三包；Task 12 跨 `collector`+`prism` 两包。

**T5b 的例外说明（如实）**：T5b 仍跨 `internal/config` + `cmd/atlas` 两个 package，未做到 ≤1。继续拆会产生「config 加了字段但无人读」的半成品任务——它既无法独立验证（新字段没有消费者，DoD 只能写「字段存在」这种空洞标准），又会让 T5b 反过来依赖它而零收益。判断：**两包合并为一个原子接线任务的收益大于形式上满足 ≤1 package**。config 侧改动实为 2 个结构体 + 默认值，体量很小。

## AD-3：提交时序 —— Go 任务先 commit 再 dev_done

**决策**：所有任务（本 Sprint 全为 Go 代码任务）的 Dev 必须在 `transition dev_done` **之前**完成 `git commit`。

**依据**（读 `.claude/hooks/task-completed.sh` 源码确认）：
- 门禁按 `packages` 跑 `go test $PKGS` + 覆盖率，**不依赖工作区脏否** → 先提交不削弱门禁效力。
- scope 漂移校验（2c）用 `git diff HEAD` + untracked 推断实际改动，再扣除**在途**任务的 packages；扣除白名单为 `assigned|in_progress|dev_done|verifying|blocked_clarification`，**不含 `verified`/`accepted`**。未提交的已验证改动会被算作后来者的漂移并 DENY——Sprint-030 TASK-001 实录事故。先提交即消除该污染源。

**注意与 docs-only 任务相反**：docs-only 分支要求「声明范围内确有实际变更」，提交后变更消失会被 BLOCKED，故必须后提交。本 Sprint 无 docs-only 任务，不适用。

## AD-4：cmd/atlas 相关任务预设 coverage_floor = 74

**决策**：T5b、T10 的任务 JSON 设 `coverage_floor: 74`。

**依据**：实测 `cmd/atlas` 覆盖率 **74.3%**，低于 `dev_minimum` 80。这是**既有历史水位**，不是本次改动造成的。不设 floor 会让门禁在与本次改动无关的理由上阻断。

**边界**：floor 设为 74（不是 74.3，门禁用整数比较 `${TOTAL%.*} -lt DEV_MIN`）。T10 要删 `TestMaybeCache_*` 测试，可能小幅拉低；T5b 新增 `policy_test.go` 可能拉高。若实际跌破 74，Dev 应提问而非自行下调 floor——下调 floor 是 Leader 的决定。

## AD-5：调度采用 dag 模式（config 默认），wave 仅作标注

**决策**：`scheduling: "dag"`，任务就绪条件 = `dependencies` 全部 `verified`，就绪即派。

**并行度实况**：
- T1 与 T11 无依赖，wave 1 即可双线并行（不同 package：`policy` 子包 vs `collector` 根包，scope 互斥成立）。
- T6/T7/T8/T9 四个 collector 改造在 T5b 完成后可四线并行（四个独立 package）。
- T1→T2→T3→T4→T5a→T5b 是不可压缩的串行链（同一 package 内逐层依赖，且 T3 会改 T2 的 `New` 签名）。

**关键瓶颈**：串行链有 6 环，是整个 Sprint 的关键路径。T11 应在 wave 1 就并行启动以填满这段时间。

## AD-6：policy 包内四任务的 scope 互斥靠串行保证

`T1/T2/T3/T4/T5a` 声明的 packages 均为 `./internal/collector/policy`。validator 要求**在途**任务 scope 互斥——由于该五任务严格串行（每个的 dependency 是前一个），任意时刻至多一个处于在途状态，互斥不变量成立。Leader 派发时必须遵守：**不得同时派发该链上的两个任务**，即使 Dev 空闲。
