# 进度 · Sprint M1d（Sprint 043 · hestia 运行时切换、通知与增量验收 · M1 收口）

**状态**：**8/8 `verified`**（93 条迁移零越权，两包 96.5% / 76.3%）；QA 复审进行中（**qa-m1d-b**，原 qa-m1d 额度耗尽改派，Opus）；master `290370f`
**master**：`290370feaa7e69b1eb533ca3c0a0683000853de4` ｜ 覆盖率 `internal/hestia` 96.3% / `cmd/atlas` 75.7% ｜ 待 merge 分支 0 ｜ worktree 残留 0（`.worktrees/*` 四个是历史 feature 分支，不属本 sprint）
**需求源**：`hestia/docs/superpowers/plans/2026-09-03-hestia-m1d-cutover.md`（只读）
**射程**：需求 TASK-001～008；009/010/011 结转为归档后人类清单（AD-1）

## 任务状态

| TASK | 标题 | wave | deps | 状态 | dev / verifier |
|---|---|---|---|---|---|
| 001 | 配置 `storage.snapshot_dir` | 1 | — | ✅ `verified`（返工 1） | dev-m1d-a / test-m1d-a |
| 002 | `saveSnapshot` 幂等规则 | 1 | — | `review_fix`（QA 第 1 轮） | dev-m1d-b / test-m1d-a |
| 004 | `Sender` + P0/P1/P2 渲染 | 1 | — | `review_fix`（QA 第 1 轮） | dev-m1d-c / test-m1d-a |
| 003 | ingest 接快照 | 2 | 001, 002 | ✅ `verified` | dev-m1d-a / — |
| 005 | ingest 接通知 | 3 | 003, 004 | `review_fix`（QA 第 1 轮）（返工 1） | dev-m1d-a / — |
| 006 | `OnlyPeriod` 包级过滤 | 4 | 005 | ✅ `verified` | dev-m1d-a / test-m1d-a |
| 007 | cmd 层 flag / sender / plist（`coverage_floor: 75`） | 5 | 004, 006 | `review_fix`（QA 第 1 轮）（返工 1） | dev-m1d-c / test-m1d-a |
| 008 | 收口 + CONTRACTS §A/§B（docs-only） | 6 | 001–007 | ✅ `verified` | dev-m1d-b / — |

## merge 记录（Leader 串行）

| TASK | dev commit | merge commit | 预演结果 |
|---|---|---|---|
| 004 | `f8e3c8e` | `69cb71d` | 无冲突，96.4% |
| 001 | `a45ead0` | `256a2de` | 无冲突（与 002 串行预演），96.4% / 75.7% |
| 002 | `84c056b` | `6a126d9` | 同上 |
| 001 返工 | `40e81a7` | `abebb76` | 无冲突，96.4%；Leader 复核变异：删 yaml 行 ⇒ FAIL |
| 003 | `4b829e1` | `656fe05` | 无冲突，96.4% / 75.7%，不动文件 0 行 |
| 005 | `8fec932` | `1efbcfe` | 无冲突，96.4% / 75.7%，不动文件 0 行 |
| 005 返工 | `f7fba51` | `1b6ef24` | Leader 复核变异：send 移到打印前 ⇒ FAIL |
| 006 | `a9cf331` | `3c56760` | 预演 15:27 通过（96.4% / 75.7%，不动文件 0），merge 18:33（失联后补做） |
| 007 | `cdcc64d` | `a1f5bd4` | 预演+合入同一 Bash；cmd/atlas 76.2%（floor 75，基线 75.7），plutil OK |
| 007 返工 | `0cecbae` | `540e84a` | Leader 复核 M6/M7：各自只让对应守卫红 |
| 008 | `ba8e69b` | `f3d6eb2` | docs-only，只含 CONTRACTS.md +68；回归五数逐字相同；新增测试实数 42（预估 29） |
| 005 QA 返工 | `ec93e13` | `208a77c` | A4 变异复核红 |
| 004 QA 返工 | `bef5572` | `687e43a` | 与 007 串行预演 |
| 007 QA 返工 | `aba2fb3` | `b47e440` | cmd/atlas 76.3% |

⚠️ 002 的 merge commit 尾部 `Claude-Session` 链接有一个字符笔误（`…bbsn2…` 应为 `…bbsm2…`），发现时 dev 已可能在该 HEAD 上重采，**不 amend**（改写 sha 会作废他人的核实）。

## 🔴 事故 1：dev_done 门禁被非任务的未提交改动阻断（已处置）

- **现象**：TASK-004 / 002 转 `dev_done` 被 DENY，越界文件是主仓库工作树的 21 个非任务改动；001 也会撞。
- **成因两批**：① Leader 应 hook 提示跑 `npx gitnexus analyze`（10:57）再生 CLAUDE/AGENTS/skills（**我的责任**）；② **开工前就在工作树**的上游 PR #6 运行时同步（`arcforge-write.sh` + CLAUDE.md Arcforge 段，人类会话外所做）。门禁口径「未提交 + 未跟踪 − 他人 scope − 自己 writes，唯一豁免 `.arcforge/`」⇒ **②即使没有①也会挡住全部 dev_done**。
- **处置**：拆成两个 chore 提交（CLAUDE.md 经索引操作分离两类改动）：`688c24c chore(arcforge)`、`2f5ad51 chore(gitnexus)`，均不含 Go；工作树除 `.arcforge/` 已空。沿上一 sprint `fe086ba` 先例。两个任务的 `questions[]` 已答复。
- **未采纳**：dev 把这些路径补进 writes（假申报会让互斥/漂移判据失真）；`git checkout` 回滚（会一并抹掉人类的上游同步）。
- **教训**：Leader 开工前的环境检查要多一条「`git status --short` 除 `.arcforge/` 外必须为空，否则先处置」——本 sprint 开工时我看见了那两处改动并决定「不动它」，没有推到「它会挡门禁」这一步。

## 🔴 事故 2：Leader 失联约 3 小时（11:57 → 14:55），dev 四次催办 + 一次 blocked_clarification 无人应答

- **现象**：我 11:57 完成 005 的 merge 预演后，下一个动作（merge + 通知 dev）直到 14:55 才执行；期间 Monitor 的 10 分钟心跳共 17 条**全部排队**、dev-m1d-a 的四条催办与 12:39 的 `blocked_clarification` 提问**没有一条**到达我；每 20 分钟的 cron 扫描（只在空闲时触发）**一次都没触发**。
- **意外的好结果**：延迟执行的 merge 把分支尖 `75961fc`（dev 在等待期间按我建议补的 wrap 前缀断言）一起合入了；ahead 0，内容判据一致。
- **成因未定**（只报观察）：dev 在 04:41Z 收到过我 11:57 发的 merge 回执，说明我的会话是在**同一轮的两次工具调用之间**（预演 Bash 之后、merge Bash 之前）被挂起了约 3 小时；期间排队的心跳、催办、cron 全部在 14:55 一次性到达。不是通知丢失，是**会话本身没有被调度**。dev 侧的行为完全正确（催办、落盘提问、不转 dev_done）。
- **处置**：14:55 补通知 + 答复 questions[]；本事故不计 dev 的 rework。
- **教训**：CLAUDE.md「stale-dispatch」那节写的是 teammate 失联，本次是 **Leader 失联**——现有机制里没有任何东西会为此报警（validator 不查 leader 活性；dev 的催办无回执时只能干等）。候选机制（上游议题）：dev 侧 `blocked_clarification` 超阈值无答复时自动 `blocked_human`。

## 🔴 事故 3：Leader 第二次同形失联（15:27 → 18:33），成因仍未定，**频次证据成立**

- **形态与事故 2 逐字相同**：merge 预演的 Bash（含 `go test` 两包，约 1–2 分钟）跑完后，会话在**同一轮的下一个工具调用之前**被挂起约 3 小时；期间 dev-m1d-a 四次催办 + 16:00 `blocked_clarification`、17 条心跳、cron 全部积压到 18:30 一次性送达。
- **两次共同点**：都发生在「SendMessage 回执 → 预演 Bash → （挂起）→ merge Bash」这个序列上；wave 1 的三次 merge 与 003/001 返工/005 返工的 merge 都没有发生（那几次预演与 merge 也是分两个工具调用）。**样本 2/8，看不出与预演时长的关系**（TASK-005 预演 ~2 分钟、TASK-006 ~2 分钟、未发生的几次也在 1–2 分钟）。
- **处置**：18:33 补 merge + 答复 questions[] + 通知；不计 dev 返工。
- **Leader 侧缓解（本 sprint 起）**：预演与正式 merge 合并进**同一个** Bash 调用（预演过即合，不再分两轮）；merge 后的通知与 plan 更新也尽量在同一轮发出。这不解决挂起本身，只缩小「dev 等 Leader 下一步」的窗口。
- **上游候选机制**：dev 侧 `blocked_clarification` 超阈值无答复 ⇒ 自动 `blocked_human` 并推送人类；或允许 dev 在 Leader 静默超阈值后自行 `--ff-only` 合入（AD-6 的例外，需人类拍板）。

## 🔴 事故 4：全体 teammate 20:37 同时撞账号会话上限（`You've hit your session limit · resets 10:30pm`）

- dev-m1d-a：停机前已把 005 转 `dev_done`（discovery 20:37:27）；dev-m1d-c：004/007 已提交并由我合入，但停机前未做 merge 后重采/dev_done；dev-m1d-b：002 改动（snapshot.go/snapshot_test.go 84/1）在 worktree 未提交；test-m1d-a / qa-m1d 状态未知（同账号，推定同样受限）。
- Leader 自身在同一窗口也无输出（20:40→22:31 的 11 条心跳与 5 次 cron 扫描全部积压），与事故 2/3 同形——**事故 2/3 的成因大概率也是账号级限流/排队**，不是会话调度；只报观察。
- **续（22:38–23:00）**：三个实例被唤醒后连续撞 `API Error: 529 Overloaded`（服务端过载，各自 3–4 次），Leader 自身调用正常 ⇒ 过载在 teammate 所用模型层；剩余动作（dev_done 迁移、提交）是 dev/test 专属边，Leader 无权代做，换新实例会撞同一层 ⇒ 只能等待 + 定时重发（150s/300s/600s 三档）。`reason_class` 若需记则为 `env_infra`（不计 rework）。
- **改派续（05:16）**：qa-m1d 亦因 Fable 额度耗尽 ⇒ QA 复审改派 **qa-m1d-b**（Opus，只做 A 组闭合确认，不重做两轮全审）。**全 sprint 共 7 次改派、10 个实例**。
- **改派续（04:33 / 05:02）**：Fable 额度确认耗尽（dev-m1d-e 明确报 `out of usage credits`）⇒ TASK-002 再改派 **dev-m1d-f**（epoch 3，Opus，已完成重采+dev_done）；验证者 test-m1d-b 亦无回执无产物 ⇒ 走 `verifying` 自环改派 **test-m1d-c**（Opus，基线不刷新）。**三人接力的代码一行未重写**，provenance 逐层记录：dev-m1d-b 编写 → dev-m1d-e 核实提交 → dev-m1d-f 重采推进。
- **改派（23:21）**：validator 报 005 `stale-dispatch`（47 分钟）；三实例重发 3–4 次全 529 ⇒ 满足 AD-21「联系不上且唤不回」。`in_progress → assigned`（004/007→dev-m1d-d、002→dev-m1d-e，epoch +1）、`verifying → verifying`（005 verifier→test-m1d-b，基线不刷新）。新实例首次登记 token，spawn 时指定 `model: fable`（Leader 同模型，调用一直正常）。**dev-m1d-a/b/c 的代码不重写**：004/007 代码已在 master，002 改动在 dev-b 的 worktree 里由 dev-e 接手提交并写 provenance。
- 处置（22:32 起）：按 stale-dispatch 第 1 步先向各实例重发一次消息（带 task id 与下一步）；一个阈值周期仍无产物再走 `in_progress → assigned` 收回改派新实例（首次登记 token，discovery 写 provenance）。

## 🔴 事故 5：Leader 的 Bash 语法错误让 TASK-002 的 merge 静默消失 4 小时 47 分

- **成因**：23:37 我发出的「预演 + 合入」复合命令把 `python3 - <<'PY' … PY` 嵌进了 `( … && … )` 链，zsh `parse error near '&&'`、**exit 1、一字节未执行**；而我在同一轮已先发了 merge 回执，此后的记忆与消息都建立在「已合入」之上。
- **代价**：dev-m1d-e 按 AD-6 正确地等 merge、发 13 条催办（每条都附「master 仍 b47e440、d241978 NOT-MERGED」的内容判据），最后因 Fable 额度耗尽停机；04:22 我直读 git 才发现。
- **三道机制同时在场、无一能报**：Monitor 的 `BRANCH-AHEAD` 报了 28 次但它是状态行、与「dev 刚提交未请求 merge」同形；`stale-dispatch` 刻意不含 `in_progress`；idle hook 只会催 dev 转 `dev_done`（方向相反）。
- **与事故 2/3 的区别**：dev 侧完全同形（Leader 沉默 + master 不动），我这边区别在于**有产出、但产出建立在假前提上**——「我以为我做了」不留空白，比「我没做」更难自查。
- **处方（已进 wisdom #6）**：`git merge`/`commit` 后必须同条命令 `echo rc=$?` + `git rev-parse HEAD`，消息里的 sha 只能来自刚打印的那一行；复合命令不嵌 heredoc，长文本走 `-F msgfile`；收到 dev 第二次催办就直读 `git rev-list --count master..<branch>`，不用记忆回答。
- **上游机制建议**：Leader 侧缺「收到 merge 请求 → 已合入」的待办队列或 validator 的 `unmerged-after-request` 规则（需 merge 请求落盘，现只在 inbox）。

## QA 终审（qa-m1d，两轮）：**CONTESTED**

- 报告：`05-review/round1-review.md`（79 行）、`round2-adversarial.md`（93 行）、`verdict.md`（65 行）。codex 跨模型：第一次挂 stdin、第二次命中用量上限（至 09-10）⇒ **退回纯 Claude 三视角**（Skeptic / Operator / Architect）。
- **diff 内**：0 CRITICAL；WARNING 4 条 → review_fix 第 1 轮（A3 改稿后快照幂等失效 / A4 汇总分子 / A5 主配置装不上被折成未配置 / A7 Duplicate P2 措辞，Leader 拍板「已在库（本次抽取值未写入）」）；另 A8 三条小补。
- **diff 外、交人类（须在需求 TASK-009 相应步骤之前）**：
  - 🔴 **A1 CRITICAL**：`scripts/ops/deploy.sh` 的 `rsync --delete` 未排除 `configs/config.yaml`（gitignore、不在 git）——**从 worktree 执行会把运行时主配置删掉**（QA `rsync -n` 干跑实测 `deleting configs/config.yaml`；从主仓库执行是同 sha 覆盖，今天无害）。修法：加 `--exclude='/configs/config.yaml'`，且清单注明第 4 步只能在主仓库执行。**须在 009 第 4 步之前。**
  - **A2**（两 lens 评 CRITICAL、QA 评 WARNING）：Telegram 传输错误文本含 bot token（`telegram.go:304,321`，pre-existing，crisis 同暴露），M1d 经 notifyError 把它打进 out.log/err.log。修法在 `internal/notifier/telegram`（脱敏 + NotContains 测试），不在 M1d writes。**建议在 009 第 6 步（首次真实经代理发送）之前。**
- 挂账 C 组：A6 通知失败无重试（M1.5）；CONTRACTS §B「导出面未改」措辞应为「导出函数面未改；新增导出类型 1（Sender）、字段 3」（Leader §A5 提交一并改）。

## 🔴 被实测证否的 DoD 断言（沿 M1c-4 的账本；全部是 Leader 写的）

| # | 任务 | 断言 | 证否 | 谁发现 |
|---|---|---|---|---|
| 1 | 001 R-001 | 「加一条 `assert.Equal(cfg.Storage.SnapshotDir == "data/hestia-snapshots")` 让 yaml 显式写出有测试守着」 | **恒真**：预填默认值 == yaml 值，删掉 yaml 该行测试仍绿（验证者变异 M3 SURVIVED）。reviewer 提议、Leader 采纳，两人都没推「守卫要能区分两种来源」 | test-m1d-a（变异实测） |

| 2 | 004（需求原文 → DoD boundary[0]） | 「`%g` 会把 177600 打成 `1.776e+05`」 | **理由为假**：`%g` 在 ≥1e6 才切指数记法，177600 也打 `177600`；结论（`'f', -1`）对；dev 据此写的 `NotContains` 断言恒过（变异 N3 SURVIVED）。**源头是需求文档**，我照抄进 DoD 没验 | test-m1d-a（实测三值） |

| 3 | 003 error_handling[0] | 「`err.Error()` 含 `snapshot`」用来证明**阶段名**是 `snapshot` | **判据太弱**：内层 `saveSnapshot` 错误文本本就含 `snapshot dir …`，把 `wrap` 阶段名改成 `fetch` 测试仍绿（变异 M3 SURVIVED）。不是假断言，是判据没钉在它要证的性质上 | test-m1d-a（变异） |

| 4 | 005 functional[0] | 「通知在 Out 打印之后发（Out 是本地真相）」 | **零断言**：性质写在 DoD 里但没要求任何测试守它；send 移到 Fprintf 之前两包全绿（M1 SURVIVED）。与 #3 同类但更重：后果（通知失败 ⇒ 本地少一行入库记录）是运维真会撞的 | test-m1d-a（变异） |

| 5 | 007 functional[0]/[1] | 「`IngestDeps` 字面量加 `Notify: sender, OnlyPeriod: hestiaOnlyPeriod`」 | **零测试**：两行接线删掉全包仍绿。删 `Notify: sender` 的后果——`notify: telegram` 照打而 Ingest 没拿到 sender——**正是 M1d 立项要堵的「静默不发」**。DoD 把接线写成了「加一行」而不是「加了之后什么会变」；验证者补了两条端到端守卫（httptest 伪站点 + 记录 CONNECT 的伪代理） | test-m1d-a（变异 + 自写守卫） |

#1 判据已订正：改为对原始 yaml 文本的 `assert.Contains`，验收看变异态转红。`rework_count` 计 1（写通道自动）；成因两条并记（DoD 给了恒真形态 + dev 未自行变异核实）。#2 不退回（004 VERIFIED，结论正确），载体是 TASK-008 CONTRACTS §A4（已写进其 DoD）。#5 退回（task_defect，rework 1），判据「删任一接线行必须转红」，这条是全 sprint 最值钱的一次退回。#4 退回（task_defect，rework 1），判据改为「send 移到 Fprintf 之前必须转红」。#3 不退回（字面满足）；教训：**「错误链含 X」这类判据要写成「wrap 前缀恰为 `<期次> (<id>): snapshot:`」**，否则内层文本会替它过关。

## 调度

- `scheduling: dag`；wave 1 三任务并行，之后 003→005→006→007→008 串行（同写 `ingest.go` / `hestia.go`）。
- 计划团队：dev × 3（`dev-m1d-a/b/c`）+ test × 1（`test-m1d-a`）。003→005→006 由同一 dev 承接。
- Leader 串行 merge；**merge 先于 `dev_done`**（AD-6）。

## 门禁与机制备忘

- 提交锚 `<type>(TASK-00N): M1d …`（需求原文格式不过门禁，AD-3）
- `cmd/atlas` 75.7% < 80 ⇒ TASK-007 带 `coverage_floor: 75`（AD-4）
- 语料路径用主仓库绝对路径（`data/` 被 `.gitignore`）
- 派发/派验通知加「请回一句确认收到」

## 结转（归档时进 final-report「交付后待办」）

- 需求 TASK-009 运行时切换（人执行，需要合并后 master 全 sha 作 ANCHOR）
- 需求 TASK-010 首期增量验收（2026-09-09～09-15）
- 需求 TASK-011 vault 回写 + 语料副本 + CONTRACTS §C–§F
