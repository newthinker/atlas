# 架构决策 · Sprint M1c-4

> 编号 `AD-M1c4-N`（带 milestone 前缀——归档里多个 sprint 用过裸编号）。
> 上游的技术设计决策在 spec `2026-09-01-hestia-backfill-finalize-design.md` 里，**本文件只记 arcforge 流程层的决策**。

## AD-M1c4-1：跳过 brainstorming，改做「前提核实 + 缺口排查」

**决策**：需求分析阶段不调用 `superpowers:brainstorming`。

**理由**：需求文档已是定稿的实施计划（13 任务、逐步 TDD、验收判据俱全），且引用了独立的设计 spec。CLAUDE.md 的降级路径写的是「ECC 不可用 → 用 brainstorming 精炼设计」，其前提是**设计尚未定稿**。对已定稿的计划做发散，产出的是对已决事项的重新讨论。

**改做什么**：把同等的精力投在**核实**上——文档的每个数字前提逐条实测（见 requirements-analysis §2），并排查文档没覆盖的机制冲突（§3）。**这次核实找出了三条阻断级缺口**，是本阶段的主要产出；brainstorming 找不出它们（它不读测试文件）。

**风险与兜底**：跳过发散意味着不质疑设计本身。兜底是 Step 3 的**独立 reviewer**（只读需求文档、不看 DoD 生成过程）——若设计有根本问题，那里还有一次机会。

---

## AD-M1c4-2：`packages` 与 `writes` 必须分开声明

**决策**：全部 13 个任务，`packages` 写包路径 `./internal/hestia`，`writes` 写**文件级路径**。

**理由**：13 个任务全在同一个包。若 `writes` 缺省回落到 `packages`，validator 的 `scope-mutex` 会判定**两两冲突**，全部任务被迫串行，wave 7 的三路并行窗口消失。

真实的互斥约束是**同文件写入**，不是同包。文件级 `writes` 是唯一能表达它的粒度。

⚠️ 代价：`writes` 会包含 `configs/hestia.yaml`、`CONTRACTS.md` 这类非 Go 路径，validator 会出**告警级** `scope-writes-outside-packages`。按 CLAUDE.md 该规则不阻断；且记忆归档记录它对这种标准形状**零信息量**（条数恒等于 writes 长度）——**不要据「0 条」反推没问题，也不要为消警把非 Go 路径塞进 `packages`**（那会弄坏 `-coverpkg` 覆盖率门禁）。

---

## AD-M1c4-3：TASK-004 做成「原子扩容步」，接受超 Realistic Scope

**决策**：TASK-004 一次性完成——22 常量 + `fieldOrder` + 模板表加 `momField` 列 + `templateFields()` 收两列 + 三处计数断言更新 + `configs/hestia.yaml` 占位 22 项 + `config_version` 递增。触及 **8 个文件**，超出「≤5 文件」。

**理由**：`fieldOrder` 被三条全覆盖断言绑住（config_test / fields_test / profiles_test，明细见 requirements-analysis §3），**拆开必然留下跨 wave 的红测试**，而每个任务的 `dev_done` 门禁都要跑 `go test ./internal/hestia/...`——后续 5 个任务会被逐个 DENY。

**两害相权**：单个任务超范围 vs 红测试跨越 6 个 wave。后者更危险——需求文档 Task 4 自己就写过这句话：「红的测试会被下一个人当成噪声」。

**这不推翻 Realistic Scope**：那条约束的目的是让任务可独立交付。这里的情况恰恰是**机制不允许更细的独立交付单元存在**。约束的目的与它的字面在此冲突，取目的。

**需人类批准**（dod-gate）。

---

## AD-M1c4-4：yaml 的 22 项先填占位值，标注「未标定」

**决策**：TASK-004 填占位区间并在注释写明「**不是标定过的**」；TASK-010 用 TASK-009 的实测残差/字段分布重标。

**理由**：存在循环依赖——填表要实测分布 → 分布要抽取路由跑通 → 跑通要测试全绿 → 全绿要表填好。占位值是唯一的破环点。

**先例**：`thresholds.go` 的 `stock_continuity_max` 三档 n=0 就是这个形状。需求文档 Task 7 自己引用它来处理 `DepositSumToleranceMoM`。⇒ 本决策是**沿用既有做法**，不是新发明。

🔴 **占位值必须显式标注**。「值写下来 + 注释写明它不是标定过的」这两半**都要做**：只填值不标注，下一个人会把占位当标定结果引用——而这正是本迭代要订正的四条被证伪契约里的一条的成因（`deposit_sum_tolerance` 的 0.12 出自 M0 三样本，被 `fields.go` 写成「实测残差**稳定在** 7.6–9.1%」）。

---

## AD-M1c4-5：沿用 sprint-041 的 AD-3 / AD-4（不重新发明）

从归档 `sprint-041-2026-09-01` 提取，本 sprint 逐字沿用：

**AD-3 · 语料路径必须用主仓库绝对路径**
```
--dir /Users/zuowei/workspace/go/src/github.com/newthinker/atlas/data/hestia-backfill-2026-08-14
```
`.gitignore` 含 `data/`，`git ls-files data/` 返回 0 ⇒ **linked worktree 里根本没有 `data/` 目录**。
⚠️ **需求文档全篇写 `$PWD/data/...`**，在 worktree 里照抄必然失败。本 sprint 几乎每个任务都要跑真语料，**这条的命中率是全部纪律里最高的**。
⚠️ 不要建软链——软链会让「我测的是哪份语料」多一层间接。绝对路径是观察，软链是推理。
⚠️ `--allow-incomplete` 是**必需**的：该语料 manifest 无 `completed_at`，不传直接报错退出。

**AD-4 · merge 必须在 `dev_done` 之前**
`task-completed.sh` 的 `git log --grep` 全文件不带 `--all` ⇒ 只走 HEAD 祖先链 ⇒ 未合并分支的 commit 对门禁**结构性不可见**，「未提交改动」与「本任务已提交」两个集合双双为空 ⇒ **门禁报绿而它根本没量到你的代码**。
⚠️ idle hook 在 `in_progress` 的解锁文案恒为「推进 dev_done」，**方向与本纪律恰好相反**。等 merge 期间被唤醒的正确动作是查 merge 状态、催 Leader，**不是转状态**。

**分支命名**：`task/TASK-00N-m1c4`。仓库已有 34 个 `task/TASK-*` 分支（`-m1c3a` / `-m1c3b` 后缀），裸名会撞。

---

## AD-M1c4-6：3 个 dev，不是 4

**决策**：`max_dev_agents` 是 4，本 sprint 用 **3**。

**理由**：真实并行度由同文件写入约束决定，不是由任务数决定。串行链是：
- `profiles.go`：TASK-002 → 003 → 004 → 005
- `validate.go`：TASK-006 → 007 → 008
- `calibrate.go`：TASK-009 → 012

最大并行窗口在 wave 7（TASK-008 / 009 / 011 文件互斥）= **3 路**。第 4 个 dev 全程无活可派，只会增加 `stale-dispatch` 噪声与 token 开销。

---

## AD-M1c4-7：gitnexus 影响分析不可用，降级为直读核实

**决策**：项目 CLAUDE.md 要求「编辑任何符号前必须跑 `gitnexus_impact`」。本会话 **gitnexus MCP 连接超时**（filesystem / claude-team 同样超时）。

**降级路径**：改用 `grep -rn <符号> internal/ cmd/` 直接找调用点，并在任务 DoD 里把已知的**下游撞点显式列出**（如 `TestTemplateTablesCoverAllFields`、`TestPackageExposesNoWriteFunctions`、`golden_test.go` 键集），使 dev 不必依赖不可用的工具就能知道血缘。

⚠️ 必须在 `final-report.md` 如实注明本次未跑 gitnexus 影响分析。
