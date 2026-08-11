# Sprint 035 交付报告 — Hestia M1b-3 validate

**基线** `125ad896fb096f7766cbb3c958ba2635a311c6ba` → **交付** `722aa2728723b573537160b602bb06a03b3169b4`
**包** `internal/hestia` · **需求** `hestia/docs/superpowers/plans/2026-08-11-hestia-validate.md`（2531 行）

---

## 1. 交付结果

| 项 | 结果 |
|---|---|
| 任务 | **8/8 全部 `verified`** |
| 规模 | 10 文件、+2400/-7 |
| 覆盖率 | 89.4% → **92.1%**（门槛 80） |
| `-race` | ✅ 绿 |
| `go build ./...` | ✅ 通过 |
| `gofmt` / `go vet` | ✅ 无输出 |
| QA 两轮 Review | **PASS**（附强制条件，已由 TASK-008 履行） |
| code-simplifier | 只读审查，**结论：不建议开任务** |
| validator | `exit=0`（8 任务、19 规则） |

**每一个数字都由 Leader 在主工作区亲自复跑核实，未采信任何 agent 的自报。**

## 2. 任务与交付

| 任务 | 内容 | dev | 验证 |
|---|---|---|---|
| TASK-001 | `Thresholds` 与配置自校验 | dev-agent-46 | test-agent-24 |
| TASK-002 | `completeness` 必填集派生 | dev-agent-47 | test-agent-24 |
| TASK-003 | `History` 窄接口与 `Store.Preceding` | dev-agent-46 | test-agent-24 |
| TASK-004 | `Validate` 骨架与五道无历史闸门 | dev-agent-46 | test-agent-24 |
| TASK-005 | `deposit_sum` 两判据合成 | dev-agent-47 | test-agent-24 |
| TASK-006 | `stock_continuity` 与跳过理由优先级 | dev-agent-47 | test-agent-24 |
| TASK-007 | 豁免应用、Save 接线、ULP 契约 | dev-agent-46 | test-agent-24 |
| TASK-008 | QA 收尾：豁免边界修补与契约登记 | dev-agent-46 | test-agent-24 |

交付内容见 `changelog.md`。已知限制见 `internal/hestia/CONTRACTS.md` 的 Sprint 035 一节。

## 3. 验证强度

- **累计 100+ 个变异实验**，八份验证报告每一条 KILLED 都**核对到致红断言的行号**
- **八次判定零漂移**（`verify_baseline` 的 `head` 与 `discovery_sha256` 逐字比对），
  唯一一次漂移（TASK-008）**被机制拦下并经验证者显式 `--ack-drift` 确认**
- **开工前**有一个独立 reviewer 在隔离 worktree 里**按计划正文逐字落盘并实跑**，
  查出 4 条必定阻塞 dev、4 条必定产生假绿的问题，全部在派单前写进 DoD
- QA 两轮（三个并行 lens + `codex` 跨模型对抗）

## 4. 本 Sprint 最重要的一条经验

> **「守卫在场 ≠ 守卫有效」出现 10 次，跨 5 个不同 agent（含 Leader 自己），
> 十次全部靠「构造一个应当被排除的实现，看它是否照样绿」发现，无一靠读代码。**

十次的形态（详见 `02-plan/findings-carryover.md` F1–F43）：
断言太弱 ×7、断言被**另一条规则冒名满足** ×1（新形态）、
遍历型断言**遍历不到目标** ×1、审计记录**不记应答** ×1。

由此制度化的四条派单硬要求，每一条都有实测依据、不是凭感觉加的：

1. **消融 harness 必带编译失败闸 + 计数自证**（`变异条数 == 结论行数`）——
   「KILLED 但因果是错的」出现 6 次（后 3 次靠闸拦下）；
   **变异「根本没跑」时只是少一行输出，四道闸结构上拦不住**（F26）
2. **断言「包住了」用 `require.NotNil(t, errors.Unwrap(err))`** —— `ErrorIs` 证不了「包住」，
   不包裹的实现也让它为真（F8）
3. **写断言时就问「有没有一个我想排除的实现，能让这条断言照样绿？」** —— 比事后消融早一轮
4. **别把理由写强**：「因为 X 所以 Y」先问「X 我实测过吗」；
   「全部都要 Y」先问「**我的实测覆盖了这个『全部』里的几个？**」

**最强形态**（TASK-006 的 M7）：**为自己写下的理由预先声明证伪条件，再去跑**。
界线是**变异对象必须是生产代码，不能是判据** —— 变异判据对任何自足套件必然「检测不到」，
会退化成无效实验。

## 5. Leader 自己犯的错（各有实测，全部留档）

| # | 错误 | 被谁发现 | 记录 |
|---|---|---|---|
| 1 | 按「闸门」而非「数值比较」枚举边界 | dev-agent-47 | F17 |
| 2 | 封闭性论证的**前提域选错**（过滤器缩小了候选集） | test-agent-24 | F18 |
| 3 | 「规则」与「表」不是同一个集合（正文对、表按旧单位建） | test-agent-24 | F19 |
| 4 | 从「`0.02` 不精确」**过度泛化**成「这个措辞都是错的」 | dev-agent-46 | F25 |
| 5 | 把「同一口径不同树」误读成「三种口径」—— **真信号读成噪声** | test-agent-24 | F39 |
| 6 | 把实质要求**只放进 inbox、没写进 DoD** | Leader 自查 | F34 |
| 7 | ULP 守卫**不观察生产算术** —— DoD 逐字要求了那种写法 | QA | F30 |

第 6 项尤其要记：**实际损失为零是运气，不是机制** —— 若 dev 当时按**文件**重扫
（本项目明确要求的自愈方式），它会做对而我会误判它漏做，返工一个没有错的交付。

## 6. code-simplifier 审查结论（只读，未改任何代码）

**不建议开任务。** 3 处 A 类（合计约 8 行），其中 2 处审查者自己倾向不做；
7 条 B 类逐条给出「为什么不该改」的理由。

唯一值得的 A1（`thresholds.go` 循环内 `knownCheckIDs()` 被调 N+1 次 + 循环不变量 + 变量遮蔽）
已实测等价（两条错误文案逐字节不变），但 **Leader 裁定不做**：

> 八个 `verified` 各自锚在一个具体 commit 上，而 **`verify_baseline` 只守 `verifying` 窗口 ——
> 任务一进 `verified`，那条路径上没有任何守卫会响**（test-agent-24 提出的边界）。
> A1 的收益是 6 行可读性，成本是让八份验证报告锚到一棵不出货的树。

⇒ **记入遗留，M1b-4 首次触碰 `thresholds.go` 时顺带做。**
审查者本人的结论相同：「把八份验证报告继续锚在这个 commit 上，价值高于这 6 行。」

B 类中一条值得单独记：`gateYoYSanity` 的 `seen bool` **不能**用 `worstField == ""` 代替 ——
所有 `_yoy` 值恰好为 0 时行为会变（「查了，最大同比是 0」变成 `skipped{absent_field}`）。
**又一个「看起来冗余、实际是唯一守卫」。**

---

### 待同步 hooks 清单（人类执行）

**本 Sprint 未改动 `project-template/hooks/` 或 `project-template/scripts/` 的任何文件**
（改动全部在 `internal/hestia/`），因此**无文件需要同步**。

但本 Sprint 实测暴露了下列框架级机制问题，**建议在 ArcForge 仓走
`project-template/` → TDD → 人类确认的路径**：

| # | 机制问题 | 实测依据 | 建议 |
|---|---|---|---|
| 1 | **`--ack-drift` 的应答不留任何痕迹** | `arcforge-write.sh:514` 只 `echo ... >&2`；`transitions.jsonl` 全文件 `ack` 出现 **0** 次（sprint-032/034 同为 0，sprint-033 那 1 次是 `p-ack-ages` 子串误匹配，已排除）；而任务文件的 `verify_baseline` 仍是**过期值** | **守卫的响应与守卫的应答需要各自留痕。** 归档后「基线 A + 判定 verified + 交付物 B」与「**根本没人看过就放行**」在文件层面**完全不可区分** ⇒ 一次认真做过的确认，痕迹和一次没做的一模一样。方案（按成本）：①把两个 ack 值写进审计行（`ack_drift`/`ack_discovery_drift`）；②`verified` 落盘时更新 `verify_baseline` 为实际判定对象并保留 `acked_from` |
| 2 | **`dev_done` 之后无法让自动门禁重跑** | 状态机**无 `dev_done` 自环边、也无 `dev_done → in_progress`**。TASK-008 分三轮交付，门禁只跑过第一轮的树（`e45164d`），后两轮结构上绕过 | 「交付后被要求补一点东西」是**很常见**的场景，现在只能靠人补（本轮靠 dev + Leader + verifier **三方独立复跑**补上）。建议加 `dev_done` 自环边或允许回退 `in_progress` |
| 3 | **子代理无法把发现正文带回父实例** | QA 的三个 lens **最终回复只剩摘要甚至「已完成」三字**，正文被 `TeammateIdle` hook 循环挤掉；QA 靠**解析 transcript JSON** 才拿到全文，且两份是在**发出 verdict 之后**才送达 ⇒ 需补一份增补件（内含 **2 条真实漏项**） | **这是会静默丢失整轮审查产出的通道缺陷** —— 若 QA 没去解析 transcript，那两条就没了 |
| 4 | **`transition-audit` 子命令仍不存在**（sprint-034 已登记，本 Sprint 复现） | `validator-run.sh transition-audit` → 「未知工具(可选 validate\|progress)」。CLAUDE.md 的 Step 6 要求「先运行 `arcforge-validate`(含 transition-audit)」⇒ 本 Sprint **降级执行**（`validate` + 手工 `jq` 审计写者分布） | 文档与实现漂移，二选一修。**连续两个 Sprint 复现** |
| 5 | **共享 worktree 就地变异**（sprint-034 已登记，本 Sprint 复现） | QA 的 Architect lens 在共享 `wt-qa12` 里就地变异 `validate.go` 约 2 分钟，**主动申报并已还原**，对结论无影响 | **这正是那条纪律立项时预言的场景。** 建议把「变异必须在隔离副本」做成 harness 模板而非文档纪律 |
| 6 | **`codex exec --sandbox read-only` 跑不了 `go test`** | build cache 不可写，codex 如实声明 ⇒ **跨模型对抗轮默认只能给静态推断**。QA 把它的 3 条发现全部自己实跑核实（全部证实，1 条比其描述更严重） | 要么给可写 cache 目录，要么在派单时预期它只出静态发现 |
| 7 | **Leader 的非 `plan` 类中间产物无处可放** | `docs/03-progress/` 白名单**只登记 `plan.md`**，写 `findings-carryover.md` 被 deny-by-default 拒 | 本 Sprint 改放 `docs/02-plan/*`（对 leader 整目录开放）。若这是有意设计则无需改，但建议在矩阵注释里写明 |

**归档注意**：TASK-008 的两个 ack 值、时序、差异内容与判断理由
**仅记录在 `.arcforge/docs/04-test/TASK-008-verification.md` 的 §0** —— 目前是唯一留痕处。
`CONTRACTS.md:466` 记着 sprint-033 的评审报告因**落盘晚于归档提交**而没进归档目录的先例，
**同一个坑**。归档时须确保该文件跟着走。
