# M4 Code Review（Sprint「Hestia 队列可观测与自动触发」）

- 审查者：qa-m4-a｜2026-09-18
- 审查锚点（全 sha）：`24c2702988f851b72d5e218b2a7811f3029185a1..4cd6cccf2b060ccc718d841a6b17d5eb41f3b045`（15 文件，+1238/-19）
- 审查时工作树：`master @ 4cd6cccf2b060ccc718d841a6b17d5eb41f3b045`，`git status --porcelain` 除 `.arcforge/` 外 0 行
- 依据：计划 `hestia/docs/superpowers/plans/2026-09-17-hestia-queue-observability-and-trigger.md`（C1–C8 + 六条验收判据）、`.arcforge/docs/03-progress/plan.md` 的人类裁决、TASK-001…007 的 discovery 与验证报告

## 0. 本轮自证数字（全部采自最后一次探针之后，同一棵树）

| 项 | 结果 | 命令 |
|---|---|---|
| 单测 | `internal/hestia` `internal/metrics` `internal/alert` `cmd/atlas` 全 ok | `GOTOOLCHAIN=local go test ./internal/hestia/ ./internal/metrics/ ./internal/alert/ ./cmd/atlas/` |
| 脚本自测 | `9 passed, 0 failed` | `bash scripts/ops/hestia-warp-trigger-test.sh` |
| vet | 0 条 | `go vet ./internal/hestia/ ./internal/metrics/ ./cmd/atlas/` |
| gofmt | 仅 `internal/metrics/snapshot_test.go`，且 **base 24c2702 同样不合规**（非本轮引入） | `gofmt -l …`；`git show 24c2702:internal/metrics/snapshot_test.go \| gofmt -l` |
| 变异（我自己做的，隔离 worktree） | 见 W-2，主工作区三文件 sha256 前后一致、worktree 已 `remove --force` + `prune` | 见 W-2 复现命令 |

## 1. 结论概览

| 级别 | 条数 | 是否阻断 |
|---|---|---|
| CRITICAL | 0 | — |
| WARNING | 6 | 建议本轮或紧邻一轮修（均为小改动），不阻断上线 |
| INFO | 14 | 记录 / 后续 |

**verdict：PASS（有条件）** —— 无 high-severity 发现；六条 WARNING 中 W-1、W-5 建议本轮修（各 ~1 行代码 / 一条守卫测试），W-2/W-6 登记为待办，W-3/W-4 交人类在生产实测时校准。

> 本报告在第二轮 Minimalist lens 结论回传后更新过一次（新增 W-5、W-6 与 I-11…I-14，强化 I-3）。Minimalist 的每条断言我都独立复现后才收录，未复现的不收录（见 §7）。

## 2. 端到端性质核查（Leader 指定的重点 1–5）

### 2.1 C2 失败隔离：**成立**（实证）
链路 `QueueHealthOf → collectQueue → Snapshot → Rule.Evaluate` 上，我用真实 collector + 真实 Snapshot + 从 `configs/config.example.yaml` 真实读出的规则验证了两个方向：
- 队列目录缺失 ⇒ `hestia_queue_blind` true 且 `hestia_db_blind` false；
- DB 读失败 ⇒ `hestia_db_blind` true 且 `hestia_queue_blind` false。

证据：`cmd/atlas/hestia_alert_rules_test.go:185 TestHestiaAlertRules_E2EBlindRulesIsolated`（非桩：`queueRegistry` 里注册的是真的 `metrics.NewHestiaCollector`，队列侧现调 `hestia.QueueHealthOf`）。结构上的保证在 `internal/metrics/hestia_collector.go:106-109`：`Collect` 拆成 `collectDB` / `collectQueue` 两个方法，一侧 `return` 带不走另一侧。

### 2.2 C3 目录缺失可告警：**成立**（实证）
`internal/hestia/queue_health.go:30-33` 读不到即 `return QueueHealth{}, err`，不退化成零件数；`hestia_collector.go:152-158` 失败时**只**输出 `queue_errors_total` 与 `queue_up=0`，**不输出件数**（报 0 件就是用假值冒充「队列空」）；`configs/config.example.yaml` 的 `hestia_queue_blind`（`hestia_queue_up == 0`，critical）接住。恢复路径也有测试：`TestHestiaAlertRules_E2EQueueBlindRecovers`（删 `pending/` ⇒ 亮；`EnsureQueueDirs` 建回 ⇒ 熄）。up gauge 取代 `*_errors_total > 0` 是 2026-09-17 人类裁决，实现与规则、注释三处一致。

另：`cmd/atlas/hestia_health.go:24-25,31-38` 明确「队列目录缺失不让启动失败、库打不开才失败」，与 M1d 的 C3「装不上即响亮失败」不矛盾——队列每轮现读，可自愈。

### 2.3 计件口径一致性：**不成立**（见 W-1，验证者已报，我复现并量化）

### 2.4 状态名多处副本：**第二种形态确实静默**（见 W-2，我用变异实证，把 TASK-003 报告第五节的「读代码推断」升级成观察）

### 2.5 Snapshot 键冲突：**今日撞不上**（见 I-1）

### 2.6 触发脚本与 plist 生产行为：见 W-3、W-4、I-4、I-5

## 3. WARNING

### W-1 符号链接口径分歧，而两处注释都自称一致
- 位置：`internal/hestia/queue_health.go:25-26,37`（注释「判据与触发脚本一致」）× `scripts/ops/hestia-warp-trigger.sh:15-17,32`
- 事实：Go 侧过滤 `e.IsDir() || 前缀 "." || 后缀 ".tmp"`，`os.ReadDir` 对符号链接返回的 `DirEntry.Type()` 是 `L---------`，`IsDir()` 恒 false ⇒ **符号链接一律计件**（含指向目录的、悬空的）；脚本用 `find -type f` ⇒ **一律不计**。
- 证据（我实跑，scratchpad 内造夹具）：pending/ 放 3 个符号链接（指向文件 / 悬空 / 指向目录），
  - 复刻 Go 过滤判据的探针输出 `count = 3`（三条均 `type=L--------- IsDir=false skip=false`）；
  - `HESTIA_QUEUE_DIR=… TRIGGER_CMD='echo STUB' bash scripts/ops/hestia-warp-trigger.sh` 输出 `hestia-warp: queue empty, nothing to do`，rc=0，桩未调用。
- 后果（值班视角）：队列里出现符号链接时，**指标说有 N 件、触发器说空**；24 小时后 `hestia_queue_stuck` 亮，值班人按告警文案「Warp 触发器没跑，或容器起不来」去查触发器，而触发器的行为是**按设计**的。排障方向被文案引偏。不是静默失效，但是误导性告警。
- 现实概率：低——队列项由 `writeAtomic` 的 rename 产生普通文件（TASK-001 验证者同一判断）。但注释把一个不成立的性质写成了事实，而下一个改这两处的人会依赖它。
- 建议（**须修**，成本 1 行 + 2 处注释）：Go 侧把判据收紧为「只数普通文件」——`if !e.Type().IsRegular() { continue }`（顺带覆盖 socket/FIFO/设备文件），两处注释保留「与触发脚本一致」的说法；或者退一步只改注释为「两边判据不同：脚本只数普通文件，Go 侧另计符号链接」。**不要只改注释**：一致是这套设计想要的性质，改代码更便宜。

### W-2 加第五个队列状态时，collector 的状态名副本会静默丢数据（变异实证）
- 位置：`internal/metrics/hestia_collector.go:162-163`（`map[string]int{"pending":…,"processing":…,"done":…,"failed":…}`）vs `internal/hestia/queue.go:12`（`queueStates`，未导出）vs `internal/hestia/queue_health.go:49-61`（`switch` + `default` 响亮失败）
- 我做的变异（隔离 worktree，主工作区零改动）：
  - **形态一**：只往 `queueStates` 加 `"archived"`，`switch` 不跟上 ⇒ `go test` **红 10 条**（`TestQueueHealthCountsAndAges`、`TestHestiaAlertRules_E2E*` 4 条、`TestBuildHestiaHealth_*` 3 条等）。⇒ 响亮，且本 HEAD 上 `hestia_queue_blind` 规则已入库（`configs/config.example.yaml`），TASK-003 报告第五节里「要等 TASK-005 合入才成立」的前提现在成立了。
  - **形态二**：hestia 侧改齐（`queueStates` + `QueueHealth.ArchivedCount` + `case "archived"`），只不动 collector 的 map ⇒ `GOTOOLCHAIN=local go test ./...` **全绿**，新状态的件数不出现在 `/metrics`，`hestia_queue_items` 基键也不含它。**全仓库没有任何守卫会报这件事。**
- 复现：
  ```bash
  git worktree add --detach <tmp> 4cd6cccf2b060ccc718d841a6b17d5eb41f3b045
  # 在 <tmp> 里：queue.go 的 queueStates 加 "archived"；QueueHealth 加 ArchivedCount；switch 加 case "archived"
  GOTOOLCHAIN=local go test ./...    # 全绿 = 静默
  ```
  收尾：`git worktree remove --force <tmp> && git worktree prune`；主工作区 `internal/hestia/queue.go` / `queue_health.go` / `internal/metrics/hestia_collector.go` 的 sha256 与 `git status --porcelain` 前后一致。
- 与计划的关系：计划 D5 说「件数用带 state label 的一个 gauge 而不是四个指标，因为加第五个状态时只改一处」。**实际是三处**（queueStates、switch+字段、collector map），其中第三处静默。计划宣称的收益只兑现了一部分。
- 建议（成本：hestia 包 +6 行，metrics 包 -2 行）：在 hestia 侧提供唯一口径，例如 `func (h QueueHealth) ByState() map[string]int`（与 `switch` 同一处维护）或导出 `QueueStates()`；collector 直接遍历它。这样第三处副本消失，形态二退化成形态一（响亮）。次选：在 `internal/metrics` 加一条守卫测试，断言 `hestia_queue_items` 的 state 序列集合 == hestia 暴露的状态名集合。
- 不阻断的理由：今天四个状态是稳定的（队列状态机来自 M2a 的方案报告 5.1），且形态二只在有人改 hestia 侧时才可能发生。

### W-3 `hestia_queue_processing_stuck` 的 30 分钟阈值可能在正常运行时假红（须人类在验收判据五校准）
- 位置：`configs/config.example.yaml`（`hestia_queue_processing_age_hours > 0.5`，`for: 10m`，cooldown 6h）
- 机制：告警在契约进入 `processing/` **40 分钟**（30 + for 10）后触发。而 `scripts/ops/hestia-warp-trigger.sh:51-52` 自己写着「客户端 120s 硬超时而 agent 常跑更久」。如果一次正常的 agent 解读常态超过 40 分钟，这条规则每次正常运行都会发一次 critical 之外的 warning，值班人会学会忽略它——**而它正是 D4「用 processing/ 做互斥」那条设计唯一的兜底**。
- 我无法在本仓库内测得 agent 真实耗时（跨仓库、要真跑），故标「须人类实测校准」：计划验收判据五（投一份契约等自动出笔记）跑通时，记下契约在 `processing/` 里停留的实际时长，再决定 0.5h 是否要放宽。
- 建议：验收判据五完成后把实测时长写进 `configs/config.example.yaml` 该规则的注释（现在注释只写了「30 分钟」这个选择，没写依据）。

### W-4 触发器与消费者之间存在「已唤起但尚未进 processing/」的重复唤起窗口
- 位置：`scripts/ops/hestia-warp-trigger.sh:35-45` + `deploy/launchd/…hestia-warp.plist`（`StartInterval 1800`）
- 机制：互斥判据是 `processing/` 非空，而把契约从 `pending/` 移到 `processing/` 的是**消费者（nanoclaw 侧的 warp skill，跨仓库）**。若 agent 从被唤起到真正移动文件超过 30 分钟（容器冷启动、排队、pnpm 拉起失败后重试），下一轮 launchd 会看到「pending 非空、processing 空」再唤起一次 ⇒ 同一份契约被处理两次（烧 token，可能产出两份笔记）。计划 D4 只讨论了「agent 卡死导致 processing/ 永久非空」这一相反方向。
- 本次 diff 无法单独修（互斥的另一半在消费者侧）。建议转人类/hestia 侧：要求 skill **第一件事**就是把文件 rename 进 `processing/`（原子 rename，天然互斥），并把这条写进消费者契约文档；atlas 侧不需要改。

### W-5 人类裁决「plist 不设代理键」只有散文守着，而它的对偶决策有 Go 守卫测试
- 位置：`deploy/launchd/com.newthinker.atlas.hestia-warp.plist:8-24`（散文：「刻意不设任何代理键」「别照抄 crisis / refresh-cnhk / prism-daily」）
- 不对称（我实测）：`grep -rn 'hestia-warp' --include='*.go' .` **零输出**；而同一决策的反面在 `cmd/atlas/hestia_test.go:477 TestHestiaPlistSetsProxyKeysForSheets` 有守卫，断言 `hestia-ingest.plist` **确实设了**代理键（同文件另有三处以 `hestia-ingest.plist` 为常量的断言）。
- 为什么这条值得修：TASK-007 在本 sprint 里因为「照搬计划前提未核实」连吃两次 `dod_defect`，最终靠人类裁决定案；`plan.md` 范围外一节还记着 `hestia-ingest.plist` 头注释里引用的守卫名 `TestHestiaPlistSetsNoProxyKeys` **已不存在**——也就是说，这类散文声明在本仓库已经有过一次过期而无人察觉的实例。裁决结果目前只由注释保护，下一个「照抄再删漏删」的人不会变红。
- 建议（**须修**，成本：一条测试约 10 行）：在 `cmd/atlas` 加对称守卫，断言 `com.newthinker.atlas.hestia-warp.plist` 的 `EnvironmentVariables` 不含 `http_proxy` / `https_proxy` / `no_proxy`（可直接复用 `plistProgramArgs` 那套读法）。这正是本项目自己记过的「改结构优于加意志」。顺带可把 plist 8-24 行那 17 行取证压到 3-4 行（结论 + 取证锚 `e7c6278…` + 指向 `.arcforge/discoveries/TASK-007.json`，那里更完整），减少跨仓库细节过期面。

### W-6 `e.Info()` 与消费者搬件存在竞态，一次抓取会整组熄灭（低危，但可 2 行消掉）
- 位置：`internal/hestia/queue_health.go:41-44`
- 机制（我实测复现）：`os.ReadDir` 先返回条目，`e.Info()` 才真正 `lstat`。若消费者在两步之间把契约从 `pending/` rename 到 `processing/`（**这正是这套队列的正常流转**），`Info()` 返回 `lstat …: no such file or directory`（`os.IsNotExist == true`），而当前实现把它升级成致命错误 ⇒ `QueueHealthOf` 返回 err ⇒ 该轮 `queue_up=0`、件数与年龄全不输出。
- 复现（scratchpad 内，复刻 36-47 行的循环）：`ReadDir` 后 `os.Rename` 走该件，再对旧 entry 调 `Info()` ⇒ `err=lstat …/pending/c.json: no such file or directory  isNotExist=true`。
- 为什么是低危而不是高危：`hestia_queue_blind` 带 `for: 10m`，而 evaluator 在任一轮求值为 false 时会 `delete(e.pending, rule.Name)`（`internal/alert/evaluator.go:84-87`）⇒ **单轮抖动不可能发页**。代价是那一轮队列指标整组缺失，顺带把 `hestia_queue_failed` / `stuck` 的 for 计时清零，真告警最多晚 10 分钟。窗口是微秒级、契约一个月只搬 1–2 次，实际撞上的概率极低。
- 建议（成本 2 行）：对 `Info()` 的 `os.IsNotExist(err)` 走 `continue`（件已被搬走 = 它已不在这个目录，件数本就不该算它）；顺带只对 `pending` / `processing` 调 `Info()`——`done` / `failed` 的 oldest 算了就丢（`queue_health.go:49-61` 的 switch 只用前两个），现在每轮多做两次 `lstat`。
- 注意：**不要**把 `Info()` 的其它错误也吞掉（权限等仍应响亮失败），这条守卫是 TASK-001 的核心。

## 4. INFO

- **I-1 Snapshot 的 `state` 展开今日无键冲突**（`internal/metrics/snapshot.go:92-101`）。我求证过三处：① 全仓库带 `state` label 的注册指标只有 `hestia_queue_items`（`grep -rn '"state"' internal cmd` 除 edgar 的 XML testdata 外仅此一处）；② `metrics.NewRegistry()` 另注册的 `collectors.NewGoCollector()` / `NewProcessCollector()`（client_golang v1.23.2）无 `state` label；③ 派生后缀撞 `_count`/`_sum` 需要同名 histogram 与 gauge 共存，prometheus 注册表本身不允许。⇒ plan.md 那条观察「state 值与派生后缀重名会合并累加」是**真实的机制**但**当前无实例**。风险被推给了将来加 `state` label 的人。可选守卫：`TestSnapshot_StateInvalidValue_NoKey` 旁边加一条「state 值为 `5xx` / `count` 时与既有键合并」的**行为登记**测试（现在是未登记行为）。
- **I-2 队列读没有超时**（`hestia_collector.go:148-158`），而 DB 侧有 `hestiaScrapeTimeout = 5s`。本地磁盘 `ReadDir` 阻塞概率极低；若将来队列落到网络卷，`/metrics` 会被拖住。记一笔即可。
- **I-3 `hestia_queue_errors_total` 当前零消费者**：`grep` 全仓（排除 `.arcforge/`）只有定义处 `hestia_collector.go:69` 与三处测试断言，没有任何告警规则、抓取配置或文档引用它（仓库内无 `prometheus.yml` / grafana，唯一自动消费者是 `alert_runner.go:170` 的 `Snapshot()`）；`configs/config.example.yaml` 的注释本身写着「用 `*_up == 0` 而非 `*_errors_total > 0`」。对照：`hestia_collect_errors_total` 至少有 CONTRACTS 里的上线验收判据。保留的理由是 up + errors_total 是 node_exporter 惯例，将来真接 Prometheus 时有用——**留还是删请 Leader/人类拍板**，我不单方面判它是冗余。另记：`/metrics` 抓取与告警器各触发一次 `Gather` ⇒ 这两个计数器每周期可能加两次，规则已不依赖它们，不影响判定。
- **I-4 `|| true` 让唤起路径的一切失败都 exit 0**（`hestia-warp-trigger.sh:53`）。我实测两种生产故障：`NANOCLAW_DIR` 不存在 ⇒ stderr `cd: … No such file or directory`、**rc=0**；`pnpm` 不在 PATH ⇒ stderr `pnpm: command not found`、**rc=0**。这是有意的（退出码不代表处理结果），代价由 `hestia_queue_stuck` 在 24h 后接住——**延迟 24 小时，且告警文案指向「触发器没跑」，方向正确**。plist 注释第 27-30 行已写明这个代价与「验收要真等一次自动唤起」的要求。可接受。
- **I-5 plist 的 PATH 写死 node v22.22.0**（`…hestia-warp.plist:47`）。本机现有 v16.15.0 / v18.16.1 / v22.22.0 三个版本，nvm 升级后该目录消失即静默失效（表现同 I-4）。已在注释里显式登记。可选强化：PATH 里追加 `/opt/homebrew/bin`、或在脚本里 `command -v pnpm || { echo … >&2; exit 1; }`（把 exit 0 变成非零，launchd 日志里留痕）。
- **I-6 队列目录是相对路径**（`hestia_health.go:47-51`，`configs/hestia.yaml: queue.dir: queue/hestia`），依赖 serve 的 launchd `WorkingDirectory`。我核对过：`serve.plist:16-17` 与 `hestia-ingest.plist:22-23` 都是 `/Users/zuowei/workspace/runtime/atlas`，与触发脚本的绝对默认值 `…/runtime/atlas/queue/hestia` 一致（该目录现存、含四个子目录）。手工在别的 cwd 起 serve 时会盯错目录：若那里没有 `queue/hestia` ⇒ `queue_blind` 亮（响）；若恰好有 ⇒ 静默看错队列。注释已写明。
- **I-7 没有一条测试端到端证明 `hestia_queue_stuck` 会亮**。现有覆盖是「空队列不亮」（`TestHestiaAlertRules_E2EEmptyQueueDoesNotFire`）+ 规则层「25h 亮」（`TestHestiaAlertRules_ExprEvaluable`）+ 年龄计算（`TestQueueHealthCountsAndAges`）。补一条 `os.Chtimes` 把 pending 件回拨 25 小时、走真实 Snapshot 的用例即可闭合（约 8 行）。
- **I-8 `done/` 无限增长**：`hestia_queue_items` 基键随历史件数单调上升，`hestia_queue_items_done` 亦然。`configs/config.example.yaml` 的注释已解释「基键恒 > 0，所以规则必须用展开键」，无规则依赖它。将来若有人写 `hestia_queue_items > 0` 会立刻踩坑；建议在 hestia 侧文档记一句「done/ 需要定期归档」。
- **I-9 脚本自测有机器依赖**：`hestia-warp-trigger-test.sh:11` 写死 `DEFAULT_QUEUE_DIR=/Users/zuowei/workspace/runtime/atlas/queue/hestia`，「默认队列目录」一例会**读真实生产队列**（桩兜底，不唤起真 agent；TASK-006 验证者已核实该例不写生产目录）。换机器后该例语义失真但仍 PASS。与 TASK-006 报告观察 1（子串匹配挡不住默认路径被延长）同源，建议一并改成带边界的匹配。
- **I-10 `internal/metrics/snapshot_test.go:86-90` gofmt 不合规**，但 **base `24c2702` 已如此**（`git show 24c2702:… | gofmt -l` 报告不合规），非本轮引入。顺手修即可，不算 M4 的账。
- **I-11 `QueueFunc` 可为 nil 的分支只服务测试**（`hestia_collector.go:15,57,90,149-151`）：生产唯一调用点 `cmd/atlas/hestia_health.go:52` 恒传非 nil，传 `nil` 的三处全在 `hestia_collector_test.go`（我 `grep` 过全部 `NewHestiaCollector(` 调用点）。且 `LoadConfig` 强制 `queue.dir` 非空，「只要 DB 健康度、不要队列」这个形态今天不可达。删掉可省一个分支与一条测试；保留的代价只有三行。倾向保留（`Describe` 已按全集声明，行为有测试登记），记一笔。
- **I-12 `count_of` 依赖 `pipefail`**（`hestia-warp-trigger.sh:32`）：`find | wc -l` 本身会吞掉 `find` 的非零退出码，是第 18 行的 `set -euo pipefail` 兜住的。我实测「pending 权限 000 ⇒ rc=1、桩未调用」（TASK-006 验证者亦有同一条直测），守卫有效；但 `pipefail` 在 14 行之外，后人删它时不会意识到会一并删掉这条守卫。建议在 `count_of` 上方加一行「本行的失败传播依赖 pipefail」。
- **I-13 `hestia_collector.go:18,27,81` 用裸 `TASK-003`**，而同文件 18 行还有「M1.5 的 TASK-004」——编号空间已经撞上（全仓另有 `executor_test.go:3`、`export_signals_test.go:3` 的裸 `TASK-003` 各指不同 sprint）。`cmd/atlas` 侧两份新测试已用「M4 的」前缀，只有 `internal/metrics/` 两处是裸号。顺手补前缀即可。
- **I-14 两处微优化**（零风险）：`hestia_collector.go:162-166` 的 map 字面量可改切片（每轮省一次 map 分配、输出顺序确定，虽然 `Gather` 本就会排序）；`hestia_collector_test.go:262` 的 `len(GetMetric()) != 4 || len(items) != 4` 两个条件等价，可删前半。

## 5. 计划 C1–C8 与 plan.md「交 QA」观察逐条结论

| 项 | 结论 | 依据 |
|---|---|---|
| C1 纯文件系统读 | **满足** | `queue_health.go` 只 import `os/filepath/strings/time/fmt`，不接收 `*Store`；`store_test.go:419` 的导出面白名单加了 `QueueHealthOf`（精确集合相等，非放宽） |
| C2 两侧互不牵连 | **满足** | §2.1 |
| C3 读不到即可告警 | **满足** | §2.2 |
| C4 先判后唤起 | **满足** | 脚本自测「空队列 ⇒ calls 0」；判据在唤起之前 |
| C5 processing/ 互斥、无锁文件 | **满足**，但有 W-4 的窗口 | 脚本自测「pending+processing 非空 ⇒ busy、calls 0」 |
| C6 Telegram 复用 | 本次 diff 不涉及投递链路，规则的 severity/cooldown 已按既有 notifier 形态写 | — |
| C7 两条写口守卫白名单 | **满足** | `TestPackageExposesNoWriteFunctions` 的 `want` 加 `QueueHealthOf`；反射那条（`*Store` 方法）零改动，理由写在 `store_test.go:446-452` |
| C8 plist 代理键 | **按人类裁决执行**（删掉代理键，覆盖计划原文） | plist 8-24 行给出 nanoclaw@e7c6278 的取证；`.arcforge/docs/03-progress/plan.md` 人类裁决节 |
| 观察：符号链接口径 | **须修**（W-1） | §W-1 |
| 观察：TASK-001 M15 存活 | **可接受**——实现正确，缺口在夹具名字序与年龄序同向；建议后续加一组反向样本（TASK-001 报告观察 1 已记） | TASK-001 验证报告 §4 |
| 观察：state 值与派生后缀重名 | **可接受**（I-1，今日无实例），建议补一条行为登记测试 | §I-1 |
| 观察：collectQueue 重列状态名 | **须修**（W-2，我把它从推断升级为实证）；不阻断 | §W-2 |
| 观察：Pedantic 未覆盖 `queue==nil` | **可接受**——`TestHestiaCollector_QueueNilSkipsQueue` 覆盖了行为，`Describe` 声明全集对注册表合法 | `hestia_collector.go:90-100`、`hestia_collector_test.go:330` |
| 观察：TASK-006 三个变异存活 | **可接受**——M17 属自测断言区分力（建议按 TASK-006 观察 1 改成带边界匹配，见 I-9）；M18/M19 为 DoD 意义等价变异，且验证者补了直测 | TASK-006 验证报告 §3/§4 |
| 观察：TASK-003 验证报告在 verified 后追加第五节 | **转 Leader/人类**——内容正确（我复核了那张表，且形态一「会响」的前提现已成立），但流程上它发生在判定之后；属流程账不属代码账 | §W-2 |
| 范围外：`hestia-ingest.plist` 头注释过期 | **转人类**——不在本次 diff 内，但它正是计划 C8 假前提的源头，建议下一个碰该文件的任务顺手订正 | plan.md 范围外节 |

## 6. 代码质量与测试质量（第一轮常规审查）

- **命名与职责**：`QueueHealth` / `QueueHealthOf` / `QueueFunc` / `collectDB` / `collectQueue` 职责单一，边界清楚；`hestia`（文件系统事实）→ `metrics`（映射成指标）→ `cmd`（接线）→ `alert`（求值）四层无回环依赖。
- **错误处理**：三条错误路径都「不退化成看起来正常的值」——目录读不到报错、`e.Info()` 出错报错、未知状态报错。这是本次 diff 质量最高的部分。
- **测试真实性**：E2E 用真 collector + 真 Snapshot + 真配置读规则，不是桩对桩；`TestHestiaAlertRules_DeclaredInExampleConfig` 对 name/expr/for/cooldown/severity/message **六项逐字比对**（返工后补的 message 逐字比对确实在，`hestia_alert_rules_test.go:59-89`）；`E2EFailedItem` 特意在 `done/` 放一件，使「误用基键代替展开键」会红——这是有区分力的夹具设计。
- **安全**：无外部输入解析、无凭据、无网络出口；脚本对路径变量全部加引号，`set -euo pipefail`，`eval "$TRIGGER_CMD"` 只在测试注入路径上（生产不设该变量），可接受。
- **性能**：每轮抓取四次 `ReadDir`；件数是个位数，可忽略。
- **注释**：密度高但绝大多数是决策记录（为什么不退化、为什么拆两个方法、为什么不加超时清理），符合本项目文化。会过期的是 plist 里对 nanoclaw 仓库 commit/文件段落的引用（已写了 sha，可追溯）与 I-5 的版本号，均已显式标注代价。

## 7. 第二轮：跨视角对抗

codex/gemini CLI 在本项目不可用（PENDING #5），按纪律退回纯 Claude 跨视角。我 spawn 了三个 lens 子代理（Skeptic / Architect / Minimalist）：

- **Minimalist：结论已回传并收录。** 它的每条断言我都独立复现后才写进本报告：W-5（`grep -rn 'hestia-warp' --include='*.go' .` 零输出，而 `hestia_test.go:477` 守着反面决策）、W-6（ReadDir→Rename→Info() 探针实测 `isNotExist=true`）、I-3（消费者 grep）、I-11（`NewHestiaCollector(` 全部调用点）、I-12/I-13/I-14。
  - **它自己证否并主动撤回了一条假设**（怀疑 `find | wc -l` 跨管道吞退出码），实测是 `pipefail` 兜住的 ⇒ 我按它的实测收成 I-12 的「加一行注释」，没按它的怀疑写成缺陷。
  - **我不采纳的两条**：① 建议把五条 message 的逐字全等改成「两两不等 + 含判别词」——该逐字比对正是 TASK-005 返工时按 Leader 要求补上的，弱化等于回退一次已付费的加固，润色成本（改两处）远低于文案互换的风险；② 建议删 `QueueFunc` 的 nil 分支——见 I-11，我倾向保留。两条都记录在案，供 Leader 复议。
- **Skeptic / Architect：结论未能回传**（最终回复为空摘要，二次索回至今无返回）。这两个视角因此由 qa-m4-a 本体按 Leader 指定的角度补做：
  - **生产值班人视角**：问「哪些故障不会有人知道」。结论：DB 瞎 / 队列瞎 / pending 堆积 / processing 卡死 / failed 有件，五种形态各有规则接住，且恢复后能自动熄（up gauge 的价值）。**没有静默失效**；有两处告警会把人引偏——W-1（符号链接导致指标与触发器不一致，文案指错方向）、W-3（阈值可能常态假红，长期看等于把兜底规则训练成噪声）。W-4 是重复处理而不是漏处理，代价是 token 与重复笔记。
  - **将来加第五个队列状态的维护者视角**：W-2 的两个形态已逐条走完并实测——只改一处会红（好），改齐 hestia 侧而漏 collector 会**全绿且静默**（坏）。修法便宜（在 hestia 侧给唯一口径）。
- **分歧判定**：三个视角对 W-1 / W-2 / W-5 的处置意见一致（都主张改代码或加守卫，而非改注释）；唯二分歧（message 逐字、nil 分支）都属 INFO 级取舍，不涉及 high-severity ⇒ **不构成 CONTESTED**。

## 8. verdict

**PASS（有条件）**：无 CRITICAL；6 条 WARNING 均不阻断上线，但建议按下列顺序处置：

1. **W-1**（1 行代码 + 注释）——建议本轮或紧邻一轮修，否则一条不成立的性质会被后人继续依赖。
2. **W-5**（约 10 行守卫测试）——建议本轮修：人类裁决的结果现在只由散文守着，而本 sprint 已经有过「散文前提过期两次、守卫名指向不存在的测试」的实例。
3. **W-6**（2 行）——顺手修，代价极低；**W-2**（hestia 侧加 `ByState()` 或守卫测试）登记为待办，不必阻断 M4。
4. **W-3 / W-4**——转人类：W-3 在计划验收判据五跑通时校准阈值并把实测写进注释；W-4 属消费者（nanoclaw warp skill）契约，建议要求「skill 第一件事就是 rename 进 processing/」。
5. I-7 / I-9 / I-10 / I-12 / I-13 是低成本收尾，随手可做；I-3（`queue_errors_total` 去留）与 I-11（nil 分支去留）请 Leader/人类拍板。

判定的证据边界（诚实声明）：本报告的全部结论来自源码、单测、脚本自测与我在隔离环境里做的探针/变异；计划的六条验收判据里**二、三、四、五、六都要生产实测**，本报告**不能**替代它们——尤其判据五（真等一次 launchd 自动唤起并看到笔记产出）是 I-4/I-5 那两个「失败也 exit 0」路径的唯一诚实证明。

---

# 9. 增量复审（返工 diff，2026-09-18）

- 锚点（全 sha）：`4cd6cccf2b060ccc718d841a6b17d5eb41f3b045..bd31159ba70b9d664fed5b66b787b26a0a26cfb8`（6 文件 +262/-11；`3d94a52` W-5、`e6f4e9d` W-1/W-2/W-6、`bd31159` W-2 collector 侧）
- 复审方式：**全部自己复现，不采信转述**。每条都做成「会变红的实验」——变异体落盘后先过有效性闸（Go 侧 `go build`，plist 侧 `plutil -lint`），打印 diff 逐字核对，跑完还原并比对 `git status`。变异一律在隔离 worktree（`git worktree add --detach bd31159…`）里做；收尾 `worktree remove --force` + `prune`，主工作区 `internal/hestia/queue_health.go` / `internal/metrics/hestia_collector.go` / `…hestia-warp.plist` 三者 sha256 与 `git status --porcelain` 前后一致。
- 基线（锚点树，未缓存）：`internal/hestia` / `internal/metrics` / `cmd/atlas` 三包 ok；`hestia-warp-trigger-test.sh` `9 passed, 0 failed`；`go vet` 0 条；`go test -race ./internal/hestia/ ./internal/metrics/` ok；`gofmt -l` 仍只有三份**返工前就已存在**的文件（`snapshot_test.go` / `backtest_test.go` / `crisis_test.go`），返工未引入新的。

## 9.1 W-1 符号链接口径 —— **闭合**

实现改为 `!e.Type().IsRegular()`（`internal/hestia/queue_health.go`），注释同步改写为「一切非常规文件（子目录、符号链接、FIFO、socket、设备）…判据与 `scripts/ops/hestia-warp-trigger.sh` 的 `find -type f` 一致」。

我的独立复现（**同一份夹具同时喂两侧**，这是我要求的验收方式）：夹具含指向文件的 symlink、悬空 symlink、指向目录的 symlink、FIFO、子目录、`.DS_Store`、`a.json.tmp`。

| 夹具 | `QueueHealthOf` 的 PendingCount | 触发脚本判定 |
|---|---|---|
| 上述 7 个非常规/排除项 | **0** | **queue empty**（未唤起） |
| 再加一个真普通文件 `c.json` | **1** | **triggering**（唤起 1 次） |

第二行是正向对照，排除「两侧都瞎所以碰巧一致」。我原报告 W-1 里那组 `Go=3 / 脚本=0` 的读数在本锚点上已不复现。

## 9.2 W-2 状态名单单一口径 —— **闭合**，并接受 Leader 的措辞更正

新增 `QueueHealth.ByState()`（未接线状态**不给键**）+ `TestQueueHealthByStateCoversAllStates`；`collectQueue` 改为遍历它，字面量 map 删除；`store_test.go` 的导出面白名单同步加 `QueueHealth.ByState`（C7 仍是精确集合相等）。

四个变异（隔离 worktree，均过 `go build` 闸）：

| # | 变异 | 变红的测试 |
|---|---|---|
| M-a | **撤销修复**（collector 退回字面量 map）**且**完整加第五状态 `archived` | `TestHestiaCollector_QueueItemsFollowByState`、`TestQueueHealthByStateCoversAllStates` |
| M-a2 | 只撤销修复，不加第五状态 | **全绿**（两份口径此时仍等价） |
| M-b | 只在 hestia 侧完整加第五状态（含 ByState），collector 不动 | `TestHestiaCollector_QueueFullOutput`、`QueueEmptyOmitsAges`、`DBFailureKeepsQueueMetrics`、`TestQueueHealthByStateCoversAllStates` |
| M-c | hestia 侧加第五状态但 **ByState 漏跟**（接线缺口） | `TestQueueHealthByStateCoversAllStates` |

结论三条：

1. **Leader 的措辞更正成立，我的原报告该处应按「返工前的事实」读。** M-b 证实：返工后 collector 自动跟随，新状态**不再静默消失**；此时变红的是钉死「恰四个序列」的夹具（可见信号，需人更新期望），不是静默。
2. **真正由新守卫抓住的是 M-a 与 M-c**：前者是「撤销修复 + 加状态」（`QueueItemsFollowByState` 动态地从 `ByState()` 导出期望，故能抓两份口径的分叉），后者是「ByState 接线缺口」。两条守卫都有区分力，不是摆设。
3. M-a2 全绿是**正确**的：字面量 map 与 `ByState()` 在四状态下行为等价，守卫护的是「两份口径不得分叉」这个性质，不是某种写法。这一点值得知道——它意味着这条守卫只在有人**加状态**时才发声，平时不设防，与设计意图一致。

`ByState()` 对未接线状态不补零的选择我复核过：补零会把「不知道」说成「确认为 0 件」，且会让 `ByStateCoversAllStates` 恒绿（M-c 就抓不到了）。这个取舍写在代码注释里，正确。

## 9.3 W-6 `Info()` 竞态 —— **闭合，双向都有区分力**

| # | 变异 | 变红的测试 |
|---|---|---|
| M-d | 吞得太宽：`os.IsNotExist(err)` → `err != nil` | `TestQueueHealthOtherInfoErrorsAreFatal` |
| M-e | 撤销修复：删掉 `IsNotExist` 分支 | `TestQueueHealthEntryVanishingIsNotAnError` |

这正是我最担心的形态——「顺手把其它 `Info` 错误一起吞掉」会悄悄废掉 TASK-001 的核心守卫，而 M-d 证明它会立刻变红。实现另把 `done`/`failed` 的 `Info()` 调用省掉（只有 `pending`/`processing` 需要最旧年龄），每轮少两次 `lstat`。

测试接缝 `var queueReadDir = os.ReadDir` 我单独查过：两处注入均用 `t.Cleanup` 还原，本包无 `t.Parallel()`（`internal/hestia/required_test.go:48` 已登记这条约束），`go test -race` 干净。可接受。

## 9.4 W-5 plist 无代理键守卫 —— **闭合，四个方向都变红**

新测试 `cmd/atlas/hestia_test.go TestHestiaWarpPlistSetsNoProxyKeys`：先立肯定式锚点（键非空 + `PATH` 在场且首段是 nvm node bin），再按**后缀** `_proxy`（`ToLower` 后）否定式断言。我对 plist 副本做四个变异，每个先过 `plutil -lint`：

| # | 变异 | 结果 |
|---|---|---|
| P1 | 加 `http_proxy` | **红** |
| P2 | 加 `ALL_PROXY`（大写、不在任何枚举名单里） | **红** |
| P3 | 删掉整个 `EnvironmentVariables` dict | **红**（肯定式锚点接住——这正是否定式断言在空集上平凡为真的那个坑） |
| P4 | PATH 首段去掉 nvm node bin | **红** |

「测试存在」与「测试有区分力」是两件事，这条过了后者。

## 9.5 Leader 问的两条

**Q1：不改 `scripts/ops/hestia-warp-trigger.sh` 的注释 —— 裁决成立。**
该注释原文（锚点树 14-17 行）是「计件判据与 internal/hestia 的 QueueHealthOf 一致：**只数普通文件**，子目录、以 `.` 开头的文件…与以 `.tmp` 结尾的文件都不算」。「只数普通文件」精确描述了 `find -type f`（符号链接、FIFO、socket、设备一概不是 `-type f`），子目录/点文件/`.tmp` 是举例不是穷举 ⇒ **脚本侧从头到尾没有错误陈述**；W-1 的分叉是 Go 侧偏离了这句话，现已对齐。行为上也已交叉验证（§9.1 的表两行）。
唯一可选的小改进（INFO，不必本轮做）：在该注释末尾加一句指向 `TestQueueHealthIgnoresIrregularFiles` 与那条同夹具交叉比对用例——让「两边一致」这个声明有个可点开的证据，而不是两处各自声明。

**Q2：值得现在留一句提示，我建议放在四个确切位置，并把话说到「不许怎么改」那一层。**
钉死四序列的断言共四处：`internal/metrics/hestia_collector_test.go:265`、`:314`、`:376`，以及 `internal/hestia/queue_health_test.go:192`（`require.Equal` 那个四键 map；它上一行的 `ElementsMatch(queueStates, keys)` 是动态的，不会因加状态而红）。目前四处都没有任何提示。
- **风险的具体形状**：加第五状态的人会同时看到 4 条红，而让它们变绿最省事的动作就是把 `len(items) != 4` 放宽成 `>= 4`、把 `require.Equal` 换成 `Contains`。这一步**不会有任何东西变红**，而它恰好删掉了「加状态必须被看见」这个信号。这与本项目记过的「为变绿而放宽断言」同形。
- **建议（成本 4 行注释）**：在四处各加一句「**加状态时请更新期望值，不要把等值放宽成包含**——等值是让『新增状态』可见的唯一机制；四处位置：hestia_collector_test.go:265/314/376、queue_health_test.go:192」。把四个位置写全，是为了让那个人不必自己去找，从而没有理由改判据。
- **更强的一档（可选）**：在 hestia 侧加一条单点绊线 `require.Len(t, queueStates, 4, "加状态时请同步更新：…四处夹具…，并在 ByState 接线")`，让「该更新哪些地方」只有一处权威说明，其余三处只留指针。代价是又多一份「4」的副本，收益是失败信息自带操作指引。两者选一即可，我倾向前者（更便宜，且不新增副本）。
- 诚实声明：注释是**弱载体**（本项目自己记过「改结构优于加意志」）。我之所以仍建议注释而不是机制，是因为这里的「机制」必须保留钉死语义——任何把期望改成动态推导的做法都会消灭那个可见信号，那比放宽断言更糟。

## 9.6 增量 verdict

**PASS。** 返工 diff 未引入新的 CRITICAL / WARNING；原 **W-1、W-2、W-5、W-6 判定为已闭合**（四条都由我自己的变异/交叉夹具复现确认，不是读代码确认）。

仍然挂着的：
- **W-3**（`processing_stuck` 0.5h 阈值）、**W-4**（消费者 rename 时机造成的重复唤起窗口）——转人类，随计划验收判据五实测校准，本次返工未涉及、也不该在代码里猜。
- §4 的 INFO 项除 I-1/I-3 外未处置，均为可选收尾；新增两条：
  - **I-15**：`var queueReadDir = os.ReadDir` 是生产代码里的包级可变量（测试接缝）。当前用法规范（`t.Cleanup` 还原、无 `t.Parallel`、`-race` 干净），记一笔，别让它成为将来第二个注入点的先例。
  - **I-16**：Q1 里提到的注释交叉指针（脚本注释 → 交叉比对测试），一行，随手可做。

本节结论同样**不能替代**计划的六条生产验收判据；判据五（真等一次 launchd 自动唤起并看到笔记产出）仍是 `|| true` 与写死 PATH 那两条「失败也 exit 0」路径的唯一诚实证明。
