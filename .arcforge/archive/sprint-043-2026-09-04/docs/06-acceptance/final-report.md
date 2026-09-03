# Sprint M1d（Sprint 043）交付报告 · Hestia 运行时切换、通知与增量验收（代码部分）

**日期**：2026-09-03 ｜ **Leader**：leader ｜ **团队**：dev-m1d-a/b/c、test-m1d-a、qa-m1d、dod-reviewer-m1d（只读）
**需求源**：`hestia/docs/superpowers/plans/2026-09-03-hestia-m1d-cutover.md`（只读；本 Sprint 射程 = 需求 TASK-001～008）
**起点锚**：`ae088eb253b64b36e10558a02587e3fa657f5f3e` ｜ **交付锚（master）**：`e500cdfdc4acdfc8b2d528bc811c7c01cf51478f`
**交付锚（TASK-009 的 `$ANCHOR`）**：`e500cdfdc4acdfc8b2d528bc811c7c01cf51478f`
**QA verdict**：第一轮 **CONTESTED**（qa-m1d，2026-09-03）→ 复审 **PASS**（qa-m1d-b，2026-09-04；A 组六条变异 + 一次行为复现全部独立核实、门禁八项逐项复现）

## 1. 交付

| TASK | 内容 | 状态 | 返工 | dev / verifier |
|---|---|---|---|---|
| 001 | 配置 `storage.snapshot_dir`：预填默认、空串拒绝、仓库 yaml 显式写出 | accepted | 1 | dev-m1d-a / test-m1d-a |
| 002 | `saveSnapshot` 三态幂等 + 改稿后副本查重（QA A3） | accepted | 1 | dev-m1d-b→**dev-m1d-e** / test-m1d-a→**test-m1d-b** |
| 003 | ingest 在 Parse 之前落盘快照，写盘失败即该期失败 | accepted | 0 | dev-m1d-a / test-m1d-a |
| 004 | `Sender` 窄接口 + P0/P1/P2 渲染；Duplicate 措辞「已在库（本次抽取值未写入）」（QA A7） | accepted | 1 | dev-m1d-c→**dev-m1d-d** / test-m1d-b |
| 005 | ingest 接通知：pending⇒P0、权威表⇒P2、失败⇒P1，发送失败响亮不级联；汇总按期计数（QA A4） | accepted | 2 | dev-m1d-a / test-m1d-a→**test-m1d-b** |
| 006 | `IngestDeps.OnlyPeriod`：只与 Force 同用、Discover 之后过滤、0 匹配响亮失败 | accepted | 0 | dev-m1d-a / test-m1d-a |
| 007 | cmd 层：`--only-period`、`buildHestiaSender`（装不上即响亮失败，QA A5）、plist 传 `--config` | accepted | 2 | dev-m1d-c→**dev-m1d-d** / test-m1d-a→**test-m1d-b** |
| 008 | 收口：采锚、全量核对、真语料回归、CONTRACTS `## Sprint M1d` §A/§B（docs-only） | accepted | 0 | dev-m1d-b / test-m1d-a |

- 代码改动（`ae088eb..f3d6eb2`，`internal/hestia cmd/atlas configs deploy`）：14 文件，+1427 / −41；`Parse`/`Validate`/`Store` 五个文件 diff **0 行**；导出面 22 项**未改**；`go.mod`/`go.sum` 未动。
- 提交：12 个 dev commit（9 feat/docs + 3 fix）+ 11 个 merge commit + 2 个 chore（见 §3）。
- 每个任务：worktree 隔离、Leader 串行 merge 且 merge 先于 `dev_done`、验证者 detached 树复验 + 变异实测。

## 2. 质量（锚 `540e84a0eee6e37c5a85cb4743a189a1744aae64`，TASK-008 采样；之后只有 docs 提交）

| 项 | 实测 | 需求门槛 |
|---|---|---|
| `internal/hestia` 覆盖率 | **96.5%**（QA 返工后；§B 登记的 96.44% 采于 `540e84a`） | ≥ 96.3% |
| `cmd/atlas` 覆盖率 | **76.3%**（QA 返工后） | 任务级 floor 75（基线 75.7，全局 80 不适用，AD-4） |
| `go vet` / `gofmt -l` | 零输出 / 仅两处既有欠账（`backtest_test.go`、`crisis_test.go`） | — |
| 真语料回归（218 篇） | `218 = 217 + 1` · `217 = 213 + 4` · `97 = 76 + 21` · 冲突 0 · 路由违反 0 | 与 M1c-4 §B **逐字相同** ✓ |
| 新增测试 | **42** 条（需求预估 29；差因逐任务见 CONTRACTS §B） | — |
| 变异实测（验证者） | 001:3+2 / 002:7 / 003:7 / 004:8 / 005:9+3 / 006:6 / 007:5+2+3 组；**存活 3 组均已处置**（1 条退回返工、1 条 §A4 订正理由、1 条 §A5 弱断言记录） | — |
| 迁移审计 | **零越权**（执行边全 dev-*、判定边全 test-*、调度边全 leader；含 4 条 AD-21 逃生边） | validator 全绿 |

## 3. 🔴 待同步 hooks 清单（人类执行）

**本 Sprint 未改 `project-template/`（本仓库是消费项目，无该目录）。** 但主仓库有两批**非任务**改动被 Leader 作为 chore 提交，请人类复核：

| 文件 | 变更摘要 | 来源 / 动作 |
| --- | --- | --- |
| `.claude/hooks/arcforge-write.sh` | `update` 加 `--expect-status`（上游 ArcForge PR #6） | **人类会话外已同步**，Leader 提交为 `688c24c`；无需再同步，只需确认 |
| `CLAUDE.md`（Arcforge 段） | 派验回执一句、表格格式（上游 PR #6） | 同上，`688c24c` |
| `CLAUDE.md`/`AGENTS.md` gitnexus 块、`.claude/skills/gitnexus-*/`、`.agents/skills/gitnexus-*/` | `npx gitnexus analyze` 再生（工具升级迁移 skill 目录，删 `.claude/skills/gitnexus/*` 6 份） | Leader 应 hook 提示误跑，提交为 `2f5ad51`；**不认可可 `git revert 2f5ad51`**，不含 Go 代码 |

提交原因：`task-completed.sh` 漂移口径「未提交 + 未跟踪 − 他人 scope − 自己 writes，唯一豁免 `.arcforge/`」会把这些文件判给每一个转 `dev_done` 的 dev（事故 1）。

## 4. 已知开口（CONTRACTS `## Sprint M1d` §A + 本报告）

1. **§A4** 需求原文「`%g` 会把 177600 打成 `1.776e+05`」为假（阈值 1e6）；实现正确；真钉法 `assert.Equal("1776000", fmtNum(1776000))` 挂账 M2 前加固。
2. **§A5**（Leader 归档前追加）TASK-003 `Contains(err.Error(), "snapshot")` 被内层文本满足，阶段名改错不红；005 起改用 wrap 前缀形态。
3. `notifyError` 双前缀：dev 选阶段名 `send P2`/`send P0`（与需求原文 `wrap("notify")` 不同，语义等价，记 discovery）。
4. `--only-period` 0 匹配时 `kept 0 of N` 行仍先打出再报错（设计如此，验证者备注）。
5. 需求 TASK-009/010/011 **结转**（见 §9）。

## 5. 🔴 被实测证否 / 太弱的 DoD 断言（5 条，账本在 plan.md）

| # | 任务 | 断言 | 证否方式 | 处置 |
|---|---|---|---|---|
| 1 | 001 R-001 | 「`assert.Equal(cfg.Storage.SnapshotDir)` 守 yaml 显式写出」 | 预填值 == yaml 值 ⇒ 恒真（变异存活） | 退回返工，改断言 yaml 原文 |
| 2 | 004 | 「`%g` 把 177600 打成科学计数」（源自需求原文） | 实测阈值 1e6 | §A4 订正，不退回 |
| 3 | 003 | 「错误链含 `snapshot`」证阶段名 | 内层文本满足，阶段名改错不红 | §A5 记录，不退回 |
| 4 | 005 | 「通知在 Out 打印之后发」 | 零断言，移位仍绿 | 退回返工，补断言 |
| 5 | 007 | 「`IngestDeps` 字面量加 `Notify: sender, OnlyPeriod: …`」 | 零测试，删行仍绿；删 `Notify` 正是 M1d 立项要堵的静默不发 | 退回返工，两条端到端守卫（验证者编写） |

**共同形态**：DoD 陈述了一个性质，测试只有存在性断言，没有能区分「性质成立/不成立」的输入；三次都由验证者用删行/挪行变异抓到。#1 是 Leader 采纳 reviewer 时写的，#2 抄自需求原文没验，#3/#4/#5 是判据没钉在性质上。

## 6. 🔴 事故（3 起，全文在 plan.md）

| # | 事故 | 成因 | 处置 |
|---|---|---|---|
| 1 | 三个 dev 首次 `dev_done` 全部被门禁 DENY | 主仓库有两批非任务未提交改动（人类会话外上游同步 + Leader 误跑 gitnexus 再生）；门禁唯一豁免 `.arcforge/` | 拆两个 chore 提交（§3）；教训：Leader 开工检查加「`git status` 除 `.arcforge/` 外必须为空」 |
| 2 | Leader 会话失联 ~3h（11:57→14:55） | 会话在同一轮两次工具调用之间被挂起；dev 四次催办 + `blocked_clarification` 积压 | 恢复后补 merge/答复；dev 行为正确，不计返工 |
| 3 | Leader 会话失联 ~3h（15:27→18:33），与 2 逐字同形 | 同上；2/8 次 merge 命中，成因未定 | 同上；Leader 缓解：预演与 merge 合进同一个 Bash |
| 4 | 20:37 全体 teammate 撞账号会话上限，随后 22:38–23:05 连续 `529 Overloaded` 3–4 次 | 账号级限流 + 服务端过载（Leader 自身调用正常 ⇒ 过载在 teammate 所用模型层）。这也说明事故 2/3 的成因大概率同源 | 定时重发（150s/300s/600s 三档）无效后，按 AD-21 走逃生边改派：004/007→dev-m1d-d、002→dev-m1d-e、005 验证者→test-m1d-b（新实例、新模型、首次登记 token，discovery 全部写 provenance，**原实例代码一行未重写**） |
| 4b | Fable 额度耗尽，23:21 改派的 3 个新实例中 2 个在 5h 内再次停机 | 模型级配额 | 04:33 / 05:02 再改派 dev-m1d-f、test-m1d-c（**Opus**）。**全 sprint 共 6 次改派、9 个实例、代码零重写**，每次都走 AD-21 逃生边、provenance 逐层记进 discovery |
| 5 | **Leader 的 Bash 语法错误让 TASK-002 的 merge 静默消失 4h47m** | 复合命令里嵌 `heredoc` ⇒ zsh `parse error`、exit 1、整条未执行；而我在同一轮已先发 merge 回执，此后记忆与消息全建立在假前提上 | dev 13 条催办（每条带内容判据）+ 我直读 git 才发现；已补做。**处方**：merge/commit 后同条命令打 `rc=$?` 与 `rev-parse HEAD`，消息里的 sha 只能来自那一行；复合命令不嵌 heredoc |

## 7. 给人类的机制提案（`wisdom/learnings-leader.md`，5 条）

1. Leader 开工前置检查：`git status --short | grep -v .arcforge` 必须为空，否则先 chore 提交再 spawn。
2. `npx gitnexus analyze` 改写已跟踪文件，sprint 内不跑；PostToolUse 的「索引过期」提示不等于该做。
3. teammate 派出的子代理继承 teammate 实例身份触发 `teammate-idle.sh`（本 sprint 3 次独立发生）——上游议题。
4. **Leader 失联无任何机制报警**（validator 只查 teammate 活性）：候选上游机制——dev 侧 `blocked_clarification` 超阈值无答复自动 `blocked_human`；或允许 dev 在 Leader 静默超阈值后 `--ff-only` 自合（AD-6 的例外，需人类拍板）。
5. 「要进 discovery 的内容写进 DoD，不写进 merge 通知」——本 sprint 三次补充要求到达时任务已过 `dev_done`（与 M1c-4 同形）。
6. 🔴 **Leader 侧缺「merge 请求 → 已合入」的待办队列**（事故 5）：Monitor 的 `BRANCH-AHEAD` 状态行与「dev 刚提交未请求 merge」同形、报了 28 次无人能据它判断；`stale-dispatch` 刻意不含 `in_progress`；idle hook 方向相反。建议 validator 增一条 `unmerged-after-request`（需 dev 的 merge 请求落盘，现只在 inbox）。
7. **teammate 派出的子代理继承实例身份触发 idle hook**：本 sprint **4 次**独立发生（每个 dev 的 code-simplifier 各一次），子代理均正确拒绝并直接向 Leader 报告。上游议题。

## 7.1 🔴 机制射程缺口（本 sprint 新发现，dev-m1d-f 主动申报）

**AD-21 改派会让 `dev_done` 门禁的范围漂移判定看到空集**：门禁的「本轮已提交改动」下界取新 owner 的认领时刻，而改派场景下代码由前一位 dev 在改派**之前**提交（本次 15:37Z vs 20:34Z）⇒ 该提交被判「早于本轮开工」、不参与漂移判定。覆盖率与包测试证据仍有效，但**越界检查实际没跑**。补偿：新 owner 在 master 独立重采 + 写明「fix commit 的 numstat 恰等于 `writes`」（本次已做，拓扑 + 内容两把尺）。建议上游把 `WORK_SINCE` 的下界改为「本任务上一次 `verified` 之后」，或至少把「本轮漂移集为空」变成显式 WARN。

## 8. 未走完整验证流程的部分

- QA 报告中标注「仅转述」的条目未独立复算（引用时只用「已独立核实」一档）。
- TDD 红阶段输出只能靠 dev 自述（验证者标为弱证据，不阻断）。
- 事故 2/3 成因未定，只报观察。

## 9. 🔴 交付后待办（人类执行，不在本 Sprint）

### 9.0 先于运行时切换的两条（QA 终审 CONTESTED 的成因，均在 diff 之外）

| # | 问题 | 证据 | 须在何时之前 |
|---|---|---|---|
| 🔴 **A1**（QA 评 CRITICAL） | `scripts/ops/deploy.sh` 的 `rsync --delete` 会清掉运行时里所有「源树没有的 gitignore 内容」 | QA 复审背对背干跑（`rsync -n`，同时刻两次、只换源，逐字抄排除表，未写一字节）：**worktree 源 30202 条 `deleting`**（`qlib_eval/.venv` **30177** 条、`configs/config.yaml` **1** 条、其余零星），主仓库源 8 条（全是过期日志，无害）。排除表已有 `akshare/.venv`（该目录实际不存在）与 `baostock/.venv`，**唯独漏 `qlib_eval/.venv`**（39525 文件；本机系统 python 损坏，qlib_eval 的 pytest 依赖它） | **TASK-009 第 4 步之前**。完整修法三条：① 补 `--exclude='/configs/config.yaml'` **与** `--exclude='/scripts/qlib_eval/.venv/'`；② **堵成因**——`deploy.sh` 开头加 linked worktree 判别并拒绝执行（照抄 `arcforge-write.sh` 的判别式）；③ 清单第 4 步注明「必须在主仓库、`HEAD` == 合并锚下执行」。⚠️ **只做 ① 仅闭合 1/30202** |
| **A2**（两 lens 评 CRITICAL、QA 评 WARNING） | Telegram 传输错误文本含 bot token（`telegram.go:304,321`，pre-existing，crisis 同暴露），M1d 经 `notifyError` 把它接进 out.log / err.log | QA 探针实测 `Post "https://api.telegram.org/botSECRET-TOKEN-123/sendMessage": …` | **TASK-009 第 6 步之前**（首次真实经代理发送）。修法：`internal/notifier/telegram` 脱敏 + NotContains 测试 |

### 9.1 需求原文的三个结转任务

| 需求 Task | 内容 | 需要的锚 / 前置 |
|---|---|---|
| **TASK-009** 运行时切换（七步清单） | 集合差证明 → 备份三件套 → `deploy.sh` + `install-services.sh` → 替换库 → 链路实测三件事（`notify: telegram` / `2026-07 Duplicate → hestia_observations` / 快照 sha256 = 语料 + Telegram `[P2]`）→ 装载 → CONTRACTS §C | `ANCHOR` = **归档后的 master 全 sha**（本报告顶部「交付锚」） |
| **TASK-010** 首期增量验收 | 2026-08 月报预计 09-09～09-15 发布；两种通过形态之一 + Telegram 消息为证；三期判据挂账 M2 前 | 009 完成后 |
| **TASK-011** 文档回写与语料副本 | vault README / 方案报告回写；`corpus/hestia-backfill-2026-08-14.tar.gz` + sha256；CONTRACTS §E/§F | 009/010 结果 |

⚠️ 需求原文 TASK-009 Step 0 的「`git rev-parse HEAD` 必须 == `$ANCHOR`」以本报告交付锚为准；Step 3 的 `deploy.sh` 会投递本 Sprint 改过的 `configs/hestia.yaml`（`config_version: "2026-09-03"`，新增 `snapshot_dir`）与带 `--config` 的 plist。
