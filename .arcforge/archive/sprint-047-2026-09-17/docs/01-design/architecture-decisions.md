# M2b Sheets 投影 —— Arcforge 侧架构决策

上游技术决策（子包边界、按表头定位、逐格写、RAW、默认 transport 等）全部由 spec 与需求文档的 C1–C11 承担，**此处不复述**。本文件只记 Arcforge 承载层面的决策，编号 AD-M2b-N。

---

## AD-M2b-1 单仓库 sprint：门禁真实触发，验证者在本仓库直接实跑

**决策**：所有 11 个任务的 `packages` 都指向 atlas 本仓库的 Go 包（TASK-011 除外，见 AD-M2b-2）。

**依据**：与 M3 不同，本轮全部代码在 atlas。`dev_done` 门禁会对声明包跑 `go test` + 覆盖率，`verify_baseline` 锚的 `HEAD` 就是交付物所在的树。M3 记录的 PENDING #7（跨仓库写通道 fail-open）、#9（基线够不到目标仓库）本轮不适用。

**推论**：验证者不需要建别的仓库的 worktree，`go test ./internal/hestia/... ./cmd/atlas/...` 在隔离 worktree 里直接跑。

## AD-M2b-2 TASK-011 的 vault 产物按「交付记录 + 人回填」处理

**问题**：需求文档 TASK-011 要写 Obsidian vault 的 `Projects/Hestia/README.md` 与 `M2b-Sheets-接入手册.md`。那是 `/Users/zuowei/Obsidian/ClawdVault`，**不是 git 仓库、不在任何门禁保护下、且是 Loom Spool 的写入面**。

**决策**：
- TASK-011 的 `writes` 只列仓库内文件（`configs/config.example.yaml`、`internal/hestia/CONTRACTS.md`）
- vault 两份 md **不由 dev 直接写**。dev 把内容写进 `docs/hestia-m2b/TASK-011-vault-content.md`（仓库内），DoD 要求该文件含两段可直接粘贴的正文；实际写入 vault 是**人执行**，与 TASK-012 同批
- 理由与 M3 TASK-004 对 `mount-allowlist.json` 的处理同源：agent 不直接写宿主运行时数据面。vault 还多一层——它是 Spool 的白名单目标，agent 直接写会绕过 Spool 的写口守卫

**代价**：需求文档的 TASK-011 Step 3「vault 回写」在本 sprint 内不闭合，结转到人执行清单。

## AD-M2b-3 TASK-012 不建任务文件

**决策**：需求文档的 TASK-012「真实验收（人执行）」不进 Arcforge 任务图。

**依据**：六步全部依赖真实 Google 凭据、真实表、真实断网。建成任务的唯一去向是 `blocked_human`，而 `blocked_human` 在本框架的语义是「机器循环卡住了需要人拍板」，不是「这件事本来就该人做」。

**处置**：spec §10 七条判据的表格骨架由 TASK-011 写进 CONTRACTS.md `## Sprint M2b-2`，留空实测值列；人执行 TASK-012 后回填。final-report 的「未完成/需人执行」节列全六步。

## AD-M2b-4 过程性落盘是本轮全体 teammate 的硬要求

**决策**：spawn prompt 里对 dev / test 全体要求：**每完成一个大步骤（需求文档的一个 `- [ ] Step N`）往自己的 checkpoint 追加一行**，格式 `- [x] TASK-00N Step M <一句话> @<HH:MMZ>`。

**依据**：M3 实测——四个实例先后出现「`ListAgents` 恒 `running` + 建完 worktree 后零落盘 + 不回问询」的相同形态，前三个已死，第四个静止 155 分钟后自行完成交付。**那三条判据只能证明「不可见」，不能证明「已死」**；唯一实测有效的区分是过程性落盘（test-m3-d 两轮皆全程可见）。PENDING #12 已订正。

**机制化**：需求文档本身就是 checkbox 格式，dev 每勾一个就写一行，不需要额外判断"什么算大步骤"。

## AD-M2b-5 C1 依赖锁定由 TASK-006 的 DoD 显式承载，验证者独立核 `go.mod`

**问题**：C1 要求 `google.golang.org/api` 锁 v0.250.0，理由是 v0.251.0+ 要求 go ≥ 1.25 而 `go.mod` 是 1.24.4，且本机 `GOSUMDB=off` 会让工具链自动下载失败。这是**环境耦合**的约束，`go get` 不带版本就会静默拉到新版。

**决策**：TASK-006 的 DoD 里单列一条 `go.mod` 必须含 `google.golang.org/api v0.250.0` 的字面行，验证者用 `grep` 核而不是信 dev 报的版本号。`go.sum` 的对应行也要在。

## AD-M2b-6 scope 重叠靠依赖串行化，禁止手工并行放行 006 / 008 与 007 / 008

**事实**：`store_test.go` 被 001/002/004 写，`sheets/client.go` 被 006/008 写，`push.go` 被 007/008 写。DAG 调度下靠依赖链天然错开。

**决策**：Leader 不做任何"依赖没满足但看起来能提前开工"的手工放行。validator 的 `scope-mutex` 是最后一道闸，但它只查**在途**任务——一旦 Leader 在 006 未 verified 时把 008 派出去，两个任务的 `writes` 就同时在途，validator 会阻断派发；若 Leader 绕过 validator 派发，就会重现 M1 的就地变异事故形态。

## AD-M2b-7 TASK-001 排第一的理由是机制不是依赖

**决策**：TASK-001 单独成 wave 1，002/003 都依赖它，即使 003（子包骨架）在代码上不消费 001 的任何符号。

**依据**：需求文档原文——"先改守卫意味着加子包那一刻守卫立刻要求登记——登记这个动作被强制，而不是靠实施者记得"。这与本框架反复实测的"写下纪律不产生遵守纪律的能力，要靠机制"是同一条。003 的 DoD 里明写"守卫会红，逐条登记后转绿"，那条 DoD 只有在 001 先完成时才可能成立。

## AD-M2b-8 验收判据四～七的基线数字不进 DoD

**事实**：spec §10 判据四给了 `将写 1282 / 一致 34 / 库缺 819` 的 2026-09-16 基线。

**决策**：这些数字**不写进任何任务的 `done_criteria`**。它们依赖真表状态（判据五执行后就变了）和库里的观测数（每次 ingest 都变）。写进 DoD 会让验证者在数字自然漂移后判假红，M1c-3a 实测过一次同形（验收报告数字过期四轮无人发现）。

**处置**：TASK-009 的 DoD 只要求 dry-run 输出**三类计数分开显示**；具体数值留给 TASK-012 人执行时对着当天的表核。
