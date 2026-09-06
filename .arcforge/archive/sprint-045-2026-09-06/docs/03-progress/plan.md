# 进度 · Sprint M2a（契约队列与信号快照）

**状态**：**7/7 `accepted`**（04:44Z）；QA 第一轮 REJECT（C1）→ 005 review_fix 2 → 复验 VERIFIED → QA 复审 PASS（04:43Z）；final-report / changelog 已落 `docs/06-acceptance/`；PENDING-MECHANISMS 已收录本 sprint；14 条 task 分支已删、worktree 残留 0；**下一动作 = /arcforge-archive**
**master**：`589835aa2f7373c5c650072411228ddb02d012b6`（001–007 + 三轮返工 + QA C1 修复全部合入） ｜ 覆盖率基线 `internal/hestia` 96.6 / `cmd/atlas` 76.4 ｜ 待 merge 分支 0 ｜ worktree 残留 0（`.worktrees/*` 四个是历史 feature 分支，不属本 sprint）
**需求源**：`hestia/docs/superpowers/plans/2026-09-05-hestia-m2a-contract-queue.md`（只读）
**射程**：需求 TASK-001～007 → Arcforge TASK-001～007（AD-1）；不 deploy

## 任务状态

| TASK | 标题 | wave | deps | 状态 | dev / verifier |
|---|---|---|---|---|---|
| 001 | 配置 `queue`/`signals` + `hestia_test.go` 两处 yaml + 守卫登记（`coverage_floor: 75`） | 1 | — | ✅ `accepted`（13:49Z，7/7，变异 7/9；M3/M8 落 002 DoD） | dev-m2a-a / test-m2a-b（首派 test-m2a-a 卡死，13:44Z 改派） |
| 002 | `Evaluate` + 守卫登记 + 001 变异残留两子例 | 2 | 001 | ✅ `accepted`（14:23Z，7/7，变异 9/15；M1–M5/M13 落 004 DoD） | dev-m2a-a / test-m2a-b |
| 003 | 契约结构 + `Current`/`PriorPublishedAt` + 守卫登记 | 3 | 002 | ✅ `accepted`（16:08Z，8/8，变异 20/22；M3/M6 落 004） | dev-m2a-a / test-m2a-b |
| 004 | 队列写盘 + 守卫登记 + 002/003 变异残留 8 子例 | 4 | 003 | ✅ `accepted`（23:39Z，8/8，变异 17/19，Q4/Q8 结构性不可测） | dev-m2a-b / test-m2a-b |
| 005 | ingest 接线 + P2 信号行 | 5 | 002, 003, 004 | ✅ `accepted`（复验 2 04:41Z，C1 守卫三变异独家转红；rework 2；AD-17；首验 8/8；一次返工 M8/M4） | dev-m2a-a / test-m2a-b |
| 006 | `hestia contract emit`（`coverage_floor: 75`） | 5 | 003, 004 | ✅ `accepted`（复验 00:15Z，13/13；首验 7/7 变异 11/13；rework 1，dod_defect） | dev-m2a-b / test-m2a-a |
| 007 | 收口 + CONTRACTS §A/§B/§C（docs-only，AD-16 两份样本） | 6 | 001–006 | ✅ `accepted`（复验 03:19Z；首验 7/8→自纠 8/8；rework 1；**rejected 03:14Z task_defect：§B 样本字节数不等未写成因（Leader 起草疏漏）⇒ 03:16Z 重派 dev-m2a-d 补两句成因，rework 1，epoch 5**；首验 7/8；记录员 dev-m2a-d；**02:38Z 三次收回改派，epoch 4**：Leader 备齐 §A/§B/§C 草稿与全部实测数字，记录员只落盘/提交/迁移；dev-c 44 分钟零产物=事故 8；dev-a=事故 7；dev-b=事故 6） | dev-m2a-d（记录员；前三任均挂起） / — |

## QA 前置处置

| # | 发现 | 处置 | 落点 |
|---|---|---|---|
| 005-M8 | `ingestOne` 丢弃 `Evaluate` 结果套件仍绿（P2 温度接线无 Ingest 级断言） | **review_fix**（dod_defect）：Ingest 级断言「温度 0/4」+ M4 双前缀 NotContains | dev-m2a-a → test-m2a-b 复验 |
| 006-M11/M12 | 回放时 `Supersedes` 置空 / `Report.Passed=false` 套件仍绿（需求明写的修订回放行为无 cmd 层守卫） | **review_fix**（dod_defect）：`TestHestiaContractEmitRevisionPeriod` | dev-m2a-b → test-m2a-a 复验 |
| QA-C1 | 实时路径契约 `extracted_at` 恒空串（`Save` 按值、`IngestedAt` 不回传；ingest 路径零断言） | **review_fix TASK-005**（task_defect）：Save 后经 `Current` 回读填入 + Ingest 级断言 + CONTRACTS A10 | dev-m2a-a → test-m2a-b 复验 → qa-m2a 复审 |
| QA-W1 | `Current` 的 `rows.Err()` 出边无前缀 | 挂账 §C（错误文本，非行为） | final-report |
| QA-W2 | 空 `config_version` 该拒但改动 15 处夹具、零生产影响 | 挂账 §C，M2b 前独立小任务 | final-report |
| 007-e0 | §B 两份回放样本字节数跨运行 ±1–2（`extracted_at` 纳秒尾零），3193 vs 3194 写 §B 前已知却未记成因（DoD error_handling[0]） | **rejected → assigned**（task_defect）：§B 两行加成因、字节数标非判据。**事后订正（03:17Z）**：验证者自纠该 REJECTED 超出 DoD 文本（error_handling[0] 括号枚举不含字节数，按文本 8/8 PASS）；Leader 回「判定成立」时也未贴原文核对——两边同错。返工已合入、内容有价值，保留；rework 1 如实归因 | dev-m2a-d → test-m2a-b 复验 |

## 派发计划（dag 模式，依赖全部 `verified` 即派）

- 001 → dev-m2a-a；002 → dev-m2a-a（001 后）；003 → dev-m2a-a（002 后）；004 → dev-m2a-b（003 后）
- 005 → dev-m2a-a、006 → dev-m2a-b（004 后并行）；007 → dev-m2a-b（全部 verified 后）
- 验证：test-m2a-b 串行验 003/004；005 ∥ 006 时 test-m2a-a（已恢复）接其一

## merge 记录（Leader 串行）

| TASK | dev commit | merge commit | 预演结果 |
|---|---|---|---|
| 005 fix2 | `b9d351b`（QA C1：ingest.go +12 / ingest_test.go +27 / CONTRACTS +4） | `589835a`（04:34Z；Leader 挂起 ~55 分钟后 = 事故 10） | 预演 rc=0 且预演树两包绿 96.6 / 76.6，merge rc=0；Save/Store 未动 |
| 007 fix | `89888ad`（§B 两行加成因，2/2） | `4d81143`（03:16Z，主动 merge） | 预演 rc=0，merge rc=0 |
| 007 | `de2ce7e`（记录员 dev-m2a-d，CONTRACTS.md +74/0） | `f8e9a14`（03:02Z；Leader 按文件级证据主动 merge，未等请求） | 预演 rc=0，merge rc=0 |
| 006 fix | `bb488d4`（R1 一条用例，+33/0） | `7022d01`（00:12Z） | 预演 rc=0，merge rc=0；dev 私有树变异 3/3 KILLED |
| 005 fix | `f743d2f`（M8/M4 两条断言，+10/-1） | `fe6a809`（00:04Z） | 预演 rc=0，merge rc=0；dev 隔离树变异 2/2 KILLED |
| 006 | `2427b6d` | `189a9ea`（23:52Z，父含 005） | 对 `f207958` 重新预演 rc=0 且预演树两包绿（96.6 / 76.6），merge rc=0；2 文件 +242/-2 恰为 writes；dev 申报：emitFixture 报告带一条 check（Save 拒零 checks）、code-simplifier 一处等价改动 |
| 005 | `dca854d` | `f207958`（23:51Z） | 预演 rc=0，merge rc=0；4 文件 +296/-18 恰为 writes；不动文件/go.mod 0 行；零新增导出；dev 申报：`outPeriods` helper 跳过 ` contract → ` 行（需求打印格式不改）、补 Revision/不可用 queue.dir 两测试回到 96.6、code-simplifier 无改动 |
| 004 | `418f23e` | `057a91c`（23:32Z；请求 16:2xZ 发出，Leader 挂起 7h16m = 事故 5；dev 16:38Z 写 blocked_clarification 文件级信号） | 预演 rc=0，merge rc=0；4 文件 +260/-1 恰为 writes；不动文件/go.mod 0 行；dev 申报：code-simplifier 两处等价重构（局部变量、`passingContract` 夹具）并入；私有树变异 7/7 KILLED |
| 003 | `789989b` | `6140338`（15:59Z；请求到达前 Leader 挂起 88 分钟 = 事故 4） | 预演 rc=0，merge rc=0；4 文件 +471/-2 恰为 writes；store.go 删除行 0；Save 0；dev 申报：`pbocArticleURL` 改名、store 夹具改用 `passing()`（`Passed` 且零 checks 被 Save 拒）、code-simplifier 四处保留三处回退一处（`Current` 的 `rows.Err()` 包前缀致覆盖率 96.5 跌破门槛） |
| 002 | `78ee806` | `9e9140d`（14:16Z） | 预演 rc=0，merge rc=0；4 文件 +326/-1 恰为 writes；不动文件/go.mod 0 行；dev 申报：`obsWith` 与 `store_test.go:731` 重名改 `obsAt`；code-simplifier 只改 `sameCaliberPair` 命名返回值 |
| 001 | `913f0c0` | `70bbc47`（12:4xZ） | 预演（`--detach`）rc=0，正式 merge rc=0；5 文件 +170/-3 恰为 writes；不动文件/go.mod 0 行；dev 申报：code-simplifier 只改 `DefaultSignals()` 排版 |

## 事故 / 机制备忘

- 提交锚 `<type>(TASK-00N): M2a …`（AD-2）；分支 `task/TASK-00N-m2a`
- 🔴 事故 1（12:3xZ）：dev-a 的 code-simplifier 子代理被 idle hook 以父实例名循环 5 次（PENDING #3 第三实例）；让其直接返回
- 🔴 事故 2（12:40Z→13:44Z）：test-m2a-a 验证 001 64 分钟零产物、无进程、running 不转 idle、重发无回执；13:31Z spawn 备用 test-m2a-b，13:44Z 逃生边改派。**成因订正（16:0xZ，test-m2a-a 恢复后自述）：会话挂起约 3 小时（工具调用之间零输出，15:59Z 恢复），不是子代理卡死 ⇒ 归 PENDING #4，不是 #3**；Leader 侧取证对两种成因同形、不可区分；`../wt-verify-TASK-001` 待 Leader 收
- 🔴 事故 3（13:56Z→14:16Z）：dev-a 的 002 merge 请求三封（13:56/14:02/14:14Z）在 Leader 侧 14:16Z 才一起到达，延迟 20 分钟；期间 13:57Z 的 cron tick 正常到达 ⇒ 是 teammate→leader 消息通道延迟而非 Leader 挂起（PENDING #4 形态相近但成因不同，归档时登记）
- 🔴 事故 4（14:30Z→15:59Z）：Leader 会话挂起 **88 分钟**（14:30Z cron tick 之后无任何事件，15:59Z 收到 dev-a 的 003 merge 请求才恢复；期间 cron 也未触发 ⇒ 是整个 Leader 会话挂起，PENDING #4 形态）；dev-a 停在 in_progress 等 merge，无损失但关键路径空转
- 🔴 事故 5（16:16Z→23:32Z）：Leader 会话再次挂起 **7 小时 16 分**（16:16Z cron tick 之后无事件，23:32Z 收到 dev-b 的 004 merge 请求才恢复，cron 也停）；dev-b 16:38Z 按约定把请求写进 `blocked_clarification` 的 `questions[0]`（文件级信号生效，Leader 恢复后据此答复）。本 sprint Leader 累计挂起 ≈ 8.7 小时，PENDING #4 频次 +2
- 🔴 事故 6（00:19Z→00:58Z）：dev-b 为 007 spawn 的 code-simplifier 终检子代理运行 38 分钟零产物、Leader 直发消息未送达（「排队待其下一个工具轮次」）⇒ 收回 007 改派 dev-a（`in_progress → assigned`，epoch 2），并令本体审查、不再 spawn 子代理；`wt-TASK-007-m2a` 待 Leader 收。本 sprint 子代理事故 2 起（事故 1、6），均为 code-simplifier
- 🔴 事故 7（00:58Z→01:52Z）：dev-a 认领 007 后 53 分钟零痕迹（worktree/scratchpad/进程/checkpoint 全无）、重发 27 分钟无回执、running 不转 idle ⇒ 二次收回改派新实例 dev-m2a-c（epoch 3）。**至此两个 dev 同时挂起**（dev-b 自 00:19Z、dev-a 自 ~00:59Z），PENDING #4 频次再 +1
- 🔴 事故 8（01:53Z→02:38Z）：dev-c 认领 007 后 44 分钟零痕迹、重发无回执 ⇒ **三任 dev 连续在 007 上挂起**；Leader 自跑回归 3 秒完成（排除命令挂起），改走记录员模式：Leader 在锚 `7022d01` 采齐 §B 数字（scratchpad `m2a-reg-leader/`）并起草 CONTRACTS 段（`contracts-m2a-draft.md`），spawn dev-m2a-d 只做落盘/提交/迁移（epoch 4）
- 事故 8 现场（dev-c 03:07Z 自述）：两批并行工具调用之间空 65 分钟，不是长命令；醒来后独立复采全套数字与 §B 相等；样本① 字节数 3192/3193/3194 成因 = `extracted_at` 纳秒尾零裁剪（运行时差异，非形状差异，验证按 n+m 与键族计数）
- 事故 9（02:40Z→03:10Z）：dev-d 的三封 merge 请求延迟 ~25 分钟到达 Leader（事故 3 同形）；Leader 已于 03:02Z 按文件级证据主动 merge，未受影响
- 03:1xZ 全员恢复：dev-a 挂 54 分钟、dev-b 的终检子代理挂 2.9h 才返回（事故 6 成因订正为子代理本身挂起）；§B 数字 Leader/dev-c/dev-a/dev-b 四份独立采样逐项相等；dev-b 子代理的 3 条可选建议 + 注释中间值提示交 QA 打包
- 现场汇总：事故 7 = 工具批次被挂起（部分调用 2h13m 后才执行）；事故 6 = 子代理挂 2.9h 带产物返回；事故 8 = 两批调用间空 65 分钟。三者同族「工具调用被挂起」，agent 侧无感、Leader 侧只能靠文件产物判活
- 非阻断：回放样本 `thresholds.config_version` 为空串（临时 yaml 未写），进 final-report §5 与 QA 输入
- 🔴 事故 10（03:36Z→04:34Z）：Leader 会话挂起 ~55 分钟（dev-a 的 fix2 merge 请求到达即处理）；本 sprint Leader 累计挂起 ≈ 9.6 小时
- 裁决 AD-17（04:37Z）：`internal/hestia` 覆盖率精确值 96.5505%（2743/2841），DoD 指定仪器 `go test -cover` 打 96.6 ⇒ 门槛成立；`go tool cover -func` 与门禁合并口径打 96.5 是仪器差异；新增 1 条不可达分支（`Current` 回读失败，Store 不可注入）如实登记，不为抬数字补人工测试
- 每次派发后 CronCreate（阈值 assigned 15m / verifying 30m 的一半）跑 validator 读 `stale-dispatch`；派发/派验通知带「请回一句确认收到」
