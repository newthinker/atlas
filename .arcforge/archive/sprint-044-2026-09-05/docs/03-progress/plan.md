# 进度 · Sprint M1.5（Sprint 044 · Hestia 健康度可观测）

**状态**：**9/9 `accepted`**（16:00Z）；QA PASS（0/4/13）；W1 已修（`5dafbf0`）并复验；CONTRACTS §C 已由 Leader 提交；cron 已撤；worktree 残留 0；分支已清；进入 final-report / changelog / 归档。Step 3 完成——9 个 `pending`，reviewer 反审 NEEDS WORK（3 阻断 + 8 建议）已全部核实采纳并落盘（7 条 update 审计行），validator 通过；**dod-gate 已过**（人类选「确认，开工」）；token 已登记、团队已 spawn（dev-m15-a/b/c、test-m15-a）；wave 1 已派发
**master**：`5dafbf0cf947204255470d659b563829d30f2c3d`（9 任务 + 003 W1 返工已合入） ｜ 覆盖率基线 `internal/hestia` 96.5% / `internal/metrics` 98.9% / `internal/alert` 92.3% / `internal/config` 83.3% / `cmd/atlas` 76.3% ｜ 待 merge 分支 0 ｜ worktree 残留 0（`.worktrees/*` 四个是历史 feature 分支，不属本 sprint）
**需求源**：`hestia/docs/superpowers/plans/2026-09-04-hestia-m1.5-health.md`（只读）
**射程**：需求 TASK-001～008 → Arcforge TASK-001～008 + TASK-010（AD-2）；009 结转为归档后人类清单（AD-1）

## 任务状态

| TASK | 标题 | wave | deps | 状态 | dev / verifier |
|---|---|---|---|---|---|
| 001 | `hestia_runs` 表、`Run`、`RecordRun`/`RecentRuns`、守卫登记 | 1 | — | ✅ `accepted`（8/8，变异 5/6 KILLED，M6 见账本） | dev-m15-a / test-m15-a |
| 005 | `alert.Rule.Cooldown` | 1 | — | ✅ `accepted`（7/7，变异 M1/M2 验证者重跑 KILLED） | dev-m15-b / test-m15-a |
| 002 | `Ingest` 写运行表 + `no_new` 心跳 | 2 | 001 | `pending` | — |
| 007 | `hestia status` `runs` 段 + cmd 接线测试（`coverage_floor: 75`） | 2 | 001 | ✅ `accepted`（7/7，变异 5/6 KILLED，M1 去 UTC() 存活属预期） | dev-m15-c / test-m15-a |
| 010 | 主配置 `AlertRule.Cooldown` + `HestiaConfig`；`mapRules` 透传（`coverage_floor: 75`） | 2 | 005 | ✅ `accepted`（7/7，变异 4/4 KILLED） | dev-m15-b / test-m15-a |
| 003 | `HealthSummary` | 3 | 001, 002 | ✅ `accepted`（W1 复验 VERIFIED by test-m15-b 15:59Z，变异 5/5；rework 1） | dev-m15-b / test-m15-b（首轮 test-m15-a） |
| 004 | `metrics.HestiaCollector` | 4 | 003 | ✅ `accepted`（7/7，变异 8/8 KILLED，S7 断言独立承重） | dev-m15-c / test-m15-a |
| 006 | `serve` 接线 + 样例配置 + 部署文档（`coverage_floor: 75`） | 5 | 004, 010 | ✅ `accepted`（7/7，变异 9/9 KILLED） | dev-m15-b / test-m15-a |
| 008 | 收口 + CONTRACTS §A/§B（docs-only） | 6 | 001–007, 010 | ✅ `accepted`（7/7，验证者自跑回归六数相等） | dev-m15-a / test-m15-a |

## 派发计划（dag 模式，依赖全部 `verified` 即派）

- wave 1：001 → dev-m15-a；005 → dev-m15-b
- wave 2：002 → dev-m15-a（001 verified 后）；010 → dev-m15-b（005 后）；007 → dev-m15-c（001 后）
- 003 → dev-m15-a（002 后）；004 → dev-m15-b（003 后）；006 → dev-m15-c（004+010 后）；008 → dev-m15-b（全部后）
- 验证：test-m15-a 串行；积压 ≥ 2 时加 test-m15-b

## merge 记录（Leader 串行）

| TASK | dev commit | merge commit | 预演结果 |
|---|---|---|---|
| 001 | `5b9859b` | `511ee42` | 预演（`--detach`）无冲突，hestia 96.5% / alert 92.6%，不动文件 0 行；dev 申报：保留 code-simplifier 的 COALESCE 改法、回退 `schemaDDL()` 抽取 |
| 003 fix | `53f1412`（W1：`groupCount` helper + `TestHealthSummaryReportsRowsErr`） | `5dafbf0` | 预演无冲突，hestia 96.6 / metrics 99.2 / cmd 76.4；`rows.Err()` 2 处 |
| 008 | `729b4fe` | `a03293d` | docs-only，只含 CONTRACTS.md +83；§A A1–A7、§B 锚 `e5ada52`；回归六数逐字相等；新增测试 39 |
| 006 | `6007f72` | `e5ada52` | 预演无冲突，cmd 76.4 / config 83.3 / hestia 96.6，不动文件 0 行；示例 yaml 两规则、deployment.md 均在；申报：额外 `FailsLoudlyWhenStoreUnopenable` 接受 |
| 004 | `3ba1e9f` | `4c14d79` | 预演（12:56Z，先于 merge 请求到达）无冲突，metrics 99.2 / hestia 96.6，go.mod 0 行；申报：nil 安全 helper、code-simplifier 三处改动（`hestiaScrapeTimeout` 常量等）均接受 |
| 003 | `266fe40`（dev-b 接手，实现出自 dev-a worktree，三文件 cmp 逐字节一致） | `321dfb6`（**12:21Z**，请求 11:54Z，延迟 27 分钟 = 事故 5） | 预演无冲突，hestia 96.6 / metrics 98.9，不动文件 0 行 |
| 007 | `f9410c0` | `e2d1f2b` | dev 预演对 `5c1f8e8`，Leader 在含 002 的 `c1defc0` 上重新预演无冲突，hestia 96.5 / cmd 76.3；申报：DoD 外加 `TestHestiaStatusPropagatesRunsError`（不加 cmd 76.2% 跌破基线）接受 |
| 002 | `cbac195` | `c1defc0` | 预演无冲突，hestia 96.5 / cmd 76.3，不动文件 0 行，ingest_test 删除行恰 3；申报：`firstLine`→`firstLineOf`（与 status_test 同名）、保留 `isNotifyError` 抽取、回退 `runRow` 去接收者 |
| 010 | `2658e5d` | `5c1f8e8` | 预演无冲突，config 83.3 / cmd 76.3 / alert 92.6 / hestia 96.5，不动文件 0 行 |
| 005 | `d55b66e` | `2db8519` | ⚠️ 预演命令错（`worktree add` 缺 `--detach`）未跑成，正式 merge 已执行；事后在 master 跑四包测试补核（见事故 1） |

## QA 处置（qa-verdict.md §建议处置汇总）

| # | 发现 | 处置 | 落点 |
|---|---|---|---|
| W1 | `health.go:54,71` GROUP BY 循环缺 `rows.Err()`，中断时返回部分计数且 err=nil | **`review_fix` TASK-003**（task_defect，rework +1）：两处 `rows.Err()` + 一条测试，可抽 `groupCount` | dev-m15-b → test-m15-a 复验 |
| W2 | 样例无规则消费 `hestia_collect_errors_total`；抓取失败 ⇒ 指标缺席 ⇒ 零告警且清零 `for` | CONTRACTS `## Sprint M1.5` 追加 **§C 挂账**（Leader，accepted 后）+ 009 验收加「`collect_errors_total` 必须为 0」 | CONTRACTS §C、final-report §8 |
| W3 | `hestia_no_ingest` 首次真实增量入库前结构性不触发 | 同上挂账 + 009 验收加「首期入库后确认 `hours_since_last_ingest` 出现」 | 同上 |
| W4 | 投递步骤不在仓库文档；`deployment.md:290-291` 与 `deploy.sh:95` 文案相反 | final-report §8 逐条写 009 步骤；`deployment.md` 漂移登记 §C 挂账 | final-report、CONTRACTS §C |
| info×12 | 含 001 M6 是非等价变异（DROP INDEX 即可杀，§B 改口径）、code-simplifier 终检净减约 40 行无一够 review_fix | §B 口径订正随 §C 提交；简化打包进 M2 首批 | CONTRACTS §C |

## 被证否 / 弱判据账本

| # | 条目 | 来源 | 载体 |
|---|---|---|---|
| 1 | 需求「`RecordRun` 失败不影响已入库行无法在不破坏 `Save` 的前提下构造」为假 | reviewer B2（Leader 核实 `ingest_test.go:521-530`） | TASK-002 error_handling[0]、TASK-008 §B |
| 2 | 需求「既有 `TestDDLIsIdempotent` 仍绿」对 runs 表恒真 | reviewer B1 | TASK-001 functional[0] |
| 3 | 需求主循环 `recorded == 0` 触发心跳会在 `RecordRun` 失败时补假心跳 | reviewer S2 | TASK-002 functional[1] |
| 4 | AD-14 初版理由「既有测试可能断言 URL」不成立（结论保留） | reviewer S1 | AD-14 |
| 6 | dev-m15-c 观察：dev_done 门禁的 `git log --grep` 命中旧 sprint 同名 TASK-007 的 10 条提交——**部分成立**：`MINE_H` 全集含它们，但漂移判定用的是 `--since=WORK_SINCE` 截断后的 `COMMITTED_BOUNDED`（task-completed.sh:164/215），早期提交只 WARN 点名不参与判定；对本 sprint 无害，跨 sprint 复用 ID 的根因留 PENDING-MECHANISMS | dev-m15-c 007 报告 | plan.md |
| 7 | 007 discovery「code-simplifier 无改动」为假（实改 `hestia_test.go` 2 处、净 0 行，`git diff --numstat` 对净零行改动无鉴别力 ⇒ 假阴）；改动已在 `f9410c0`，自证数字不受影响；dev 自报、落点验证报告、discovery 保持原样 | dev-m15-c 自报（10:58Z） | 007 验证报告 |
| 8 | 002 变异存活 2（非 task_defect）：M3 通知失败改走 `fail`（stage 被设值）无测试区分——`NotifyError` 测试未断言 `Stage==""`；M7 跳过候选记成带 Period 的 `no_new` 行——`SkippedCandidate` 测试只断言 outcome 序列。各加一行 `assert.Empty` 即可杀；DoD 字面 `recorded++` 实现中无消费者（S2 后良性） | test-m15-a 002 报告 | 008 §B 测试强度登记；不改 002 |
| 9 | 阶段边界清理（12:29Z）：dev-a 孤儿 worktree `../wt-TASK-003-m15` 三文件与 master `cmp` 一致后 `remove --force` + `branch -d task/TASK-003-m15`（was `e2d1f2b`，无独有提交）；dev-a 若恢复其 003 写入会被 epoch DENY | Leader | plan.md |
| 10 | **运维语义提醒（进 final-report / 009）**：`configs/config.example.yaml` 现在声明 `hestia.config_path: configs/hestia.yaml`，照抄样例而没有 `hestia.yaml` 的环境 serve 会以 `hestia health: loading …` 启动失败——这正是 spec「装不上即响亮失败」；运行时 `config.yaml` 由人改、deploy.sh 不覆盖 | dev-m15-b 006 报告 | final-report §交付后待办 |
| 5 | 001 `RecentRuns` 的 `rowid DESC` 无测试守着（变异 M6 SURVIVED：索引反向扫描碰巧给出逆序）——测试强度边界，不阻断 | test-m15-a 001 报告 | 007 已知会；008 §B 可登记 |

## 事故

### 事故 1（005 merge）：预演没跑、正式 merge 照跑
- 预演用 `git worktree add $PRE master` 对已 checkout 的分支报 `fatal: 'master' is already used`，子 shell 失败，但正式 merge 在 `;` 之后无条件执行。
- 影响：本次无害（3 文件、基于 master 尖、无冲突，事后四包测试补核）。
- 修正：预演一律 `git worktree add --detach $PRE master`，且正式 merge 用 `&&` 挂在预演成功之后。

### 事故 7（003 复验，15:34Z 记，进行中）：validator 首次命中 `stale-dispatch`（verifying 34 分钟无产物）
- **观察**：15:00Z 派验；`running`；验证 worktree `../wt-verify-TASK-003-w1` 在；15:26Z 我误把同机另一会话（`arcforge-88`，在跑 `go test ./pkg/a … TASK-001.out` 的 hook 套件）的 `go test` 进程与 tmp 副本当成了它的活性证据——**同机多会话时进程/临时目录不是某个实例的活性证据**，只有「提及该 task 的产物」才是（validator 的判据正确）。
- **处置**：第 1 步重发已于 15:26Z 做过；等满一个阈值周期（15:56Z）仍无产物 ⇒ 第 2 步 `verifying → verifying --field verifier=test-m15-b`（逃生边，baseline 不刷新）；test-m15-b token 已预登记（15:35Z）；**15:51Z 已 spawn test-m15-b 作备用**（只读准备：worktree `../wt-verify-TASK-003-b`、跑测试、备变异；不写 `.arcforge/`），15:56Z 仍无产物即 `verifying --field verifier=test-m15-b` 生效。
- **结局**：test-m15-b 15:59:04Z 判 VERIFIED（5/5 变异 KILLED），003 转 accepted；`../wt-verify-TASK-003-w1` 已由 Leader 收。
- **第 2 步已执行（15:57:25Z）**：`verifying → verifying --field verifier=test-m15-b`（审计行 by=leader，baseline 未刷新）；test-m15-b 只读准备已全绿（5/5 变异 KILLED，倾向 VERIFIED），改派后落盘判定。test-m15-a 至此 57 分钟零产物、`running`，归入事故 4/5/6 一族（会话挂起，成因未定）。

### 事故 6（004，12:56Z 记，进行中）：dev-m15-c 提交后静默——第二个 dev 出现事故 4 形态
- **观察**（直读）：12:29Z 派发、12:30Z checkpoint、~12:32Z 在 worktree 提交 `3ba1e9f`（工作树干净），此后 24 分钟无 merge 请求、无 checkpoint、无进程、`running`；12:48Z 进度询问 8 分钟未回。
- **与事故 4 的差异**：这次成品**已提交**在 `task/TASK-004-m15`，接手者只差「merge → 重采 → discovery → dev_done」。
- **结局（12:59Z）**：dev-m15-c 的 merge 请求在提交后 ~23 分钟到达（内容完整、含预演与变异），实例活着——是**消息/会话延迟**而非失联；第 2 步未触发。已 merge，无需改派。
- **频次**：本 sprint 内 dev 侧「running 不产出」2 次（dev-a、dev-c）+ Leader 1 次，与 M1d 事故 2/3/4 同族——**成因未定**，不是个体失误。

### 事故 5（Leader 失联，11:54Z → 12:21Z，27 分钟）：与 M1d 事故 2/3 逐字同形，跨 sprint 频次证据成立（3 次 / 2 sprint）
- **观察**：dev-m15-b 的 003 merge 请求 11:54Z（其 commit 时间戳 19:54:10+08）；我的 merge commit 时间戳 **20:21:27+08**；期间 dev 的催办、cron 扫描全部积压。我在 12:21Z 的回信里把 merge 时间写成「11:58Z」——那是**没读时钟的假设**，本条订正（上表 003 行同步订正）。
- **成因未定**（只报观察）：形态与 M1d 完全一致——会话在「收到消息」与「下一个工具调用」之间被挂起；本次挂起前的最后动作是 003 merge 请求到达前的空闲等待。
- **处置**：merge 已执行、dev 已收到 sha 并被告知成因；本事故不计任何人的 rework。
- **推论（给事故 4）**：dev-m15-a 的 45 分钟静默很可能是**同一现象**（teammate 会话被挂起而非崩溃），改派决定按规则仍成立，但归因从「dev 失联」改为「成因未定、疑似同源挂起」。

### 事故 4（003，11:40Z 记）——结局 13:01Z：dev-m15-a 恢复，自述被收回前正在隔离副本跑变异；其 M1/M2 KILLED、消融 M2 SURVIVED 与 dev-b/验证者结果互证；worktree 由 Leader 收（它误以为「消失」，已澄清）。归因：同事故 5/6 一族（会话/消息延迟），不计 rework；它未对 003 做任何过期写入：dev-m15-a 在 `in_progress` 上静默 37 分钟
- **观察**（全部直读）：11:02Z 认领；worktree 三文件最后修改 11:01–11:03Z（`health.go` 88 行、`health_test.go` 175 行、AST 守卫已加 `HealthSummary`——实现已成形）；此后无提交、无 checkpoint（最后一份 10:55Z）、无进程（`go test`/`gofmt` 均无）、无隔离副本活动；`ListAgents` 恒 `running`；11:32Z 的进度询问 8 分钟未回。
- **判定**：与 M1d 事故 2/3「会话在两次工具调用之间被挂起」同形（那两次是 Leader，这次是 dev）；不是子代理卡死（列表无子代理）。`in_progress` 刻意无 stale-dispatch 阈值，本事故靠 Leader 手工扫描发现。
- **处置**：第 1 步重发（11:32Z）；11:48Z 复核仍无任何产物（文件 mtime 11:03Z、无进程、无 checkpoint、无回复、`running`）⇒ 第 2 步已执行（11:49Z）：`in_progress → assigned` 收回改派 dev-m15-b（epoch +1），派发消息写明**继承** `../wt-TASK-003-m15` 里的成品（验证而非重写，`files_modified` 空也可能是正确答案，须写 `provenance`）。

### 事故 3（007 派验，10:57Z）：派验通知无回执
- test-m15-a 002 报告末尾写「等 007 派验」，说明 10:57 的 007 派验通知它未读到（或读到未回执）；文件层 007 已 `verifying`。11:01 重发一次（按「沉默即报警」，不等 30 分钟阈值）。
- **结果（11:03Z）**：验证者出完 002 裁决后**重扫磁盘**（`verifier==我 && verifying`）自行承接了 007 并判 VERIFIED——「文件系统是唯一真相源、inbox 只是通知」这条原则实测自愈成功；通知到底丢没丢无从判定（它说「若你发过，晚于我的扫描」），不影响结论。

### 事故 2（010，10:38Z）：dev 的 code-simplifier 子代理被 TeammateIdle hook 卡死循环——PENDING-MECHANISMS #3 的实例
- **现象**：dev-m15-b 前台委派 code-simplifier 子代理审查 TASK-010；子代理完成（结论无改动）后每次返回都被 TeammateIdle hook 拦下，要求「推进 TASK-010 到 dev_done」，连续 5 次；子代理无权写 `.arcforge/`（纪律 + 无 token），条件对它恒不可满足。子代理主动给 Leader 发消息求助（这是它做对的地方——「卡 running 不转 idle」在 Leader 侧本来无任何通知）。
- **事实核对**（jq/git 直读）：TASK-010 `in_progress`、discovery null；worktree 四个文件 `M`，恰为 writes 声明，无越界；dev-m15-b 本体 `running`（等子代理）。
- **处置**：回子代理「直接返回、不做任何写动作」；把结论直接转给 dev-m15-b 本体让它不等子代理继续。
- **机制归因**：hook 文案不区分调用者是否有权做解锁动作（上游待决 #3 已记，本次是第二个实例，频次证据成立：atlas M1.5 + 上游 sprint-010）。识别判据仍是「零文件产物 + worktree 未被触碰」——本次由子代理自报，若它不报则只能靠 stale-dispatch（in_progress 刻意无阈值 ⇒ 实际上**无人会报警**）。
