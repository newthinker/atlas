# QA Verdict · Sprint M1.5（Sprint 044）健康度可观测

- 审查者：qa-m15（Reality Checker 模式，两轮）
- 判定对象：master `a03293d8c74a78e013368aee92a5b8ed7cd177c5`；基线 `037d1eb1e4f827c415319519e40f4e2208968920`
- 范围：`git diff 037d1eb..a03293d -- internal cmd/atlas configs docs`（28 文件，+1883/−43）
- 方法：detached worktree `../wt-qa-m15` 内自跑全部命令；discovery / 验证报告只作导航，判定只依据自跑命令与读到的代码
- 跨模型：`codex exec` 已启动但 30 分钟零输出（stderr 只回显 prompt，无工具活动）；**本轮退回纯 Claude 跨视角**（Skeptic / Architect / Minimalist 各两次独立 context）

## 结论：**PASS**（0 critical · 4 warning · 13 info）

无 high-severity 发现；三个 lens 对「无 critical」共识一致。Architect 两次运行结论分别为 PASS-with-warnings 与 NEEDS WORK，分歧只在运维步骤文档的**落点归属**（它认为应在仓库里、Leader 已裁定进 final-report §交付后待办 + 009），不构成 CONTESTED。
4 条 warning 的共同形状是「**健康度自己不健康时不会响**」（fail-silent 方向），都不改变本 Sprint 交付物的正确性，但都应在 009 投递前登记或修掉。建议的处置见每条「落点」。

## 第一轮：常规审查证据（qa-m15 自跑，worktree @a03293d）

| 项 | 结果 |
|---|---|
| 四不动文件 + go.mod/go.sum `git diff --stat` | 0 行 |
| `Save` 函数体（两 ref 各提取 `func (s *Store) Save(`…`^}` 后 diff） | 空（69 行完全一致） |
| `cmd/atlas/hestia.go` 含 `"path/filepath"` | 0 处 |
| `health.go` 含 `fields.go` 任一引号字面量（70 个） | 0 处 |
| 两条守卫 `want` | reflect 12 项（+RecentRuns/RecordRun）；导出面 25 项（+HealthSummary/Store.RecordRun/Store.RecentRuns） |
| `go vet` 五包 | 零输出 |
| `go test -cover` 五包 | 全绿；96.6 / 99.2 / 92.6 / 83.3 / 76.4（与 CONTRACTS §B 逐字一致） |
| `gofmt -l` 五包 | 仅 `snapshot_test.go` / `backtest_test.go` / `crisis_test.go` 三处既有欠账 |
| `e5ada52..a03293d` 代码目录 diff | 仅 `CONTRACTS.md` +83（§B 锚仍有效） |
| alert 循环能否看到 hestia 指标 | `snapshot.go:34` 走 `Gather()` ⇒ collector 的 gauge 按名进评估器；`TestHestiaCollector_VisibleInSnapshot` 钉住 |
| 实测：写事务未提交时 `CREATE TABLE/INDEX IF NOT EXISTS`（已存在对象）与 SELECT | err=nil、0s（scratchpad/ddl）⇒ serve 启动撞上 ingest 写不阻塞；WAL 读者不等写者 |
| 实测：`time.Parse(RFC3339, "…Z")` 与 `"…+00:00"` 再 `Format` | 都打 `Z`（scratchpad/tz）⇒ `status.go` 的 `UTC()` 是等价变异（007 M1） |
| ingest 调度 plist | 15:30 / 17:30 / 21:30，最长自然间隔 18h，`> 30` 余量成立 |
| serve 与 hestia-ingest 的 `WorkingDirectory` | 同为 `/Users/zuowei/workspace/runtime/atlas`；`db_path: data/hestia.db` 两进程解析到同一库 |

ingest.go 六条路径（处理失败+P1 失败 / 入库成功+通知失败 / HasArticle 出错 / Force / 全部跳过 / RecordRun 失败）五列取值逐一追过：自洽；P1 每候选至多一次（唯一发送点 `ingest.go:224`，`ingestOne` 内通知失败被 `:222` 挡住）；`err!=nil && processed=false` 构造不出（`fail` 只返回已 `processed:true` 的 `res`）。

## 发现清单

### warning

**W1 `internal/hestia/health.go:54, 71` — 两段 GROUP BY 循环 `rows.Close()` 后不查 `rows.Err()`**
证据：`for rows.Next()` 结束直接 `Close` 并继续；同批 `store.go:677 RecentRuns` 是 `return out, rows.Err()`，本包惯例被打破。collector 传 5s ctx（`hestia_collector.go:38`），modernc v1.38.2 对带 deadline 的 ctx 用 `sqlite3_interrupt` 中断 ⇒ `Next` 返回 false、错误只在 `rows.Err()` ⇒ 返回**部分计数且 err=nil** ⇒ `Collect` 以缩水的 `count(*)` 输出 CounterValue、`collect_errors` 不加一。`TestHealthSummaryPropagatesQueryErrors` 只覆盖 Query/Scan 错误。
落点：**建议 `review_fix` TASK-003**（`reason_class=task_defect`，fix_items：两处加 `if err := rows.Err(); err != nil { return Health{}, fmt.Errorf("hestia health: <step>: %w", err) }` + 一条测试；可顺手抽 `groupCount` helper 合掉两段循环）。4 行改动、不动接口；若 Leader 选择不返工，则 CONTRACTS 挂账并列入 M2 首批。

**W2 `configs/config.example.yaml:243-255` + `internal/alert/evaluator.go:86` + `hestia_collector.go:84-88` — 「库读不到」零告警，且一次抓取失败清零 `for` 计时**
证据：`HealthSummary` 出错时事实指标全缺席，只 `collect_errors_total` +1；`rules.go:42-44` 对缺席指标返回 false ⇒ `evaluator.go:86 delete(e.pending)`。样例无任何规则消费 `hestia_collect_errors_total`。一行坏 `run_at`（`TestHealthSummaryRejectsCorruptRunAt` 证明一行即够）、权限错、路径换掉 ⇒ 两条 hestia 规则永远沉默；`for:10m` 需连续 10 tick 为真，任一 tick 抓取失败即重计。
落点：**CONTRACTS §A 挂账 + 009 前置提醒**（验收时 `curl /metrics | grep hestia_collect_errors_total` 必须为 0）。样例加 `hestia_collect_errors_total > 0` 规则要先解决「累计 counter 一次瞬时错误后每 24h 重发」的语义（Snapshot 无 delta），不在本 Sprint 改。

**W3 `internal/hestia/health.go:27-30` + `config.example.yaml:249-253` — `hestia_no_ingest` 在首次真实增量入库前结构性不可能触发**
证据：`LastIngest` 只取 `hestia_runs` 里 `ingested/pending` 的 `MAX(run_at)`；投递后 `hestia_runs` 为空（CONTRACTS §B：回填不走 `Ingest`，0 行）；`no_new` 心跳不推进它；collector 对零值不输出 ⇒ 规则恒 false。若 10 月首期前 Discover 坏掉（逐次 `no_new` 或失败），「40 天没有新期次入库」这条恰好在它要检测的形态上沉默。`hestia_stalled` 无此问题（§7.2 验收步骤 1 要求一次日历唤起后心跳指标即存在）。同族盲区：「从未成功跑过」⇒ `LastRun` 零值 ⇒ 两条规则都不响——spec §3.2「表为空时不输出」的已接受代价，但应写明。
落点：**CONTRACTS §A 挂账**（「首个增量入库前 `no_ingest` 不设防；从未成功运行不告警」由人接受）+ **009 验收步骤**加一条「首期入库后确认 `hestia_hours_since_last_ingest` 出现」。M2 候选修法：`LastIngest` 回落 `hestia_observations`/`hestia_pending` 的 `MAX(ingested_at)`。

**W4 `docs/deployment.md:256` + `scripts/ops/deploy.sh:95` — 投递步骤不在仓库任何文档，现网 `configs/config.yaml` 无 `hestia:` 段与两条规则**
证据：`deploy.sh:95` 显式 `--exclude='/configs/config.yaml'`（人改、不同步，账本 #10 已记）；`docs/deployment.md` 关于 hestia 只有一句；`PENDING-ACCEPTANCE.md` 无 M1.5 清单；AD-1 把 009 结转为 final-report「交付后待办」，**该清单目前尚未成文**。照抄样例的环境 serve 会以 `hestia health: loading configs/hestia.yaml:` 启动失败（runtime 目录已有 `hestia.yaml`，现网不会）；不改 runtime `config.yaml` 则走 `config_path not set` 分支——健康度静默不存在，正是本迭代要消灭的形态换了触发路径。
落点：**final-report §交付后待办 / 009**必须逐条写：改 runtime `configs/config.yaml`（加 `hestia.config_path: configs/hestia.yaml`、`alerts.rules` 追加两条）→ `launchctl kickstart -k gui/$(id -u)/com.newthinker.atlas.serve` → `curl -s 127.0.0.1:<port>/metrics | grep ^hestia_` → 等一次日历唤起后确认 `hestia_last_run_timestamp` 出现。附带：`docs/deployment.md:290-291` 既有文案「deploy.sh 会覆盖 runtime config.yaml」与 `deploy.sh:5-8, 95` 相反，运维照它办会错（既有漂移，登记即可）。

### info

| # | 位置 | 发现 | 落点 |
|---|---|---|---|
| I1 | `store.go:647` / `store_test.go` `TestRecentRunsNewestFirst` | 001 M6 存活**不是等价变异**：Skeptic 用 sqlite3 复现，有索引时带/不带 `rowid DESC` 同走 `SCAN USING INDEX` 同序；`DROP INDEX` 后走 TEMP B-TREE，同 `run_at` 内变 `ingested, duplicate`。现行写法正确且必须保留；测试杀不掉是因索引在场。一行修法：测试里经 `rawDB` 先 `DROP INDEX hestia_runs_run_at`。无主键的长期代价：VACUUM 可能重排 rowid、行不可引用；约 1.1k 行/年，十年内无性能问题 | CONTRACTS §B 补「M6 非等价、索引掩盖」；后续加 `id INTEGER PRIMARY KEY` 时改 `ORDER BY run_at DESC, id DESC` |
| I2 | `ingest.go:157-161` | Discover 失败不记行（AD-13 已裁）：index 页结构变更 ⇒ `runs_total{failed}` 不增长，30h 后才以 `hestia_stalled` 文案报出 | M2 候选：记 `failed / stage=discover` |
| I3 | `ingest.go:253` | 仅 RecordRun / 心跳失败时退出摘要为「0/N 期失败 ()」且退出码非零（分子按 `failedPeriods`） | 后续小修 |
| I4 | `ingest.go:219` + A7 | 「入库成功+通知失败」err.log 计 1/1 期失败、runs 表记 ingested——A7 设计，两处口径差异值得登记 | CONTRACTS A7 补一句 |
| I5 | `cmd/atlas/hestia_health.go:36-48` | 错误/日志只带相对路径；手工在错误 cwd 启动 serve 会 `MkdirAll` + 建空库导出「健康的空」（launchd 下 cwd 固定，无事）；serve 侧跑完整 `LoadConfig` + `NewStore`（含 DDL + `verifyObservationsSchema`），未来观测表加列而 runtime 库未迁移会让 Web/API 整个起不来 | 建议 M2 前：`filepath.Abs` 进日志；只读打开（不建库、schema 不符只 Warn + collect_errors）或 CONTRACTS 明记「hestia 迁移先于 serve 重启」 |
| I6 | `hestia_collector.go:62` + `health.go:77` | `hestia_pending_review` = `count(*) FROM hestia_pending`：无 DELETE 路径（非测试代码 0 命中）、PK 含 `ingested_at` ⇒ `--force` 重跑卡 pending 的期次会加行，人处理完也不减；HELP「currently awaiting」略夸 | HELP 改「rows ever pended」或 `count(DISTINCT period, period_type)`；M2 |
| I7 | `status.go:63-77` | runs 段不打印 `Notified`：「没配 Notify」与「发成功」同形，只答得了「没发出去」 | 后续：`Notified` 为真打 `notified` |
| I8 | `status.go:59-61` | 注释理由「读回 Location 可能是 +00:00」与实现不符（自测：`RecordRun` 恒写 `Z`，Parse 得 UTC）；`UTC()` 值得留（调用方可传非 UTC `Run`），理由应改 | 改一行注释 |
| I9 | `health.go:34-39` | `LastRun` / `LastIngest` 解析失败文案同为 `bad run_at`，分不清哪一支 | 后续 |
| I10 | `hestia_collector.go:99-107` | `runs_total` / `validation_blocked_total` / `notify_failures_total` 用 CounterValue 但值来自行数，靠「永不删行」保单调；将来加保留策略须同时改类型 | CONTRACTS 记「不清理；要清理先改类型」 |
| I11 | `internal/metrics` → `internal/hestia` | 方向当下无环（spec §3.2 接受），但 `api` 因此传递依赖 hestia + sqlite；hestia 将来 import metrics 即成环 | 知会 M2 |
| I12 | `serve.go:187-199` | 关闭时序已核无问题：`Shutdown` 先于 defer；晚到的 Snapshot 只得 `database is closed` ⇒ `collect_errors++` | — |
| I13 | `cmd/atlas/hestia_health.go:27-30` | `metrics.enabled=false` 且设了 `config_path` ⇒ 只打一行 Info「hestia health disabled (metrics disabled)」，两条 hestia 规则永不评估为真。**判定：spec §4.2 明文定义的降级**（「`metrics.enabled: false` ⇒ 跳过（没有注册表可挂）」），非缺陷；既有 `high_error_rate`/`api_down` 在同一情形下同样静默，本 Sprint 未引入新形态。样例与现网 `config.yaml` 均 `metrics.enabled: true`（Architect 只读 grep 核实） | **009 前置提醒**：验收步骤加「确认 runtime `config.yaml` 的 `metrics.enabled: true`」；M2 可选把该日志升 Warn |

### code-simplifier 终检（qa-m15 独立 + Minimalist lens 两次，逐文件；只报告不改）

| 文件 | 结论 |
|---|---|
| `health.go` | 两段 GROUP BY 循环逐字相同、四处手写 `rows.Close()` ⇒ 抽 `groupCount(ctx, q, prefix, query, args...)`（内 `defer Close` + `rows.Err()`）；≈ −10 行，顺带闭合 W1 |
| `schema.go` / `store.go:72` / `schema_test.go:62, 260` | DDL 列表 `append([]string{…}, runsDDL()...)` 三份副本 ⇒ `allDDL(spec)`；去一个漂移点（reviewer B1 抓到的正是这种形态） |
| `types.go:316` / `hestia_collector.go:41` | outcome 五元枚举两包各一份（`validRunOutcomes` / `allOutcomes`），加第六个 outcome collector 会静默漏序列 ⇒ 导出 `RunOutcomes` 单一出处 |
| `store.go:687 boolToInt` | modernc 驱动直接接受 `bool`（驱动源 `case bool` 已核），可删 −7 行；「可删但不必」 |
| `ingest.go` | `failed` 在 `fail` 与 `runRow` 两处判定、靠注释维系「必须同判」⇒ 可让 `fail` 直接定 outcome 使 `runRow` 变纯映射（−5 行）；`processed` int 只与 0 比、`res.notified = d.Notify != nil` 两处——可读性项，0 行 |
| `status.go` | 只有 I8 注释；渲染循环无可简化项 |
| `hestia_collector.go` | Desc 三处手写（struct / Describe / Collect），9 个可接受；M2/M3 再加后改表驱动 |
| `rules.go` / `evaluator.go` / `config.go` / `serve.go` / `alert_runner.go` / `hestia.go` / `hestia_health.go` | 无可简化项（`runsLimit` 与 `statusLimit` 分开并写明理由合理；`noop` 在错误路径也返回是刻意不变量） |
| 测试 | `cmd/atlas/hestia_test.go` / `hestia_health_test.go` 同一段最小 `hestia.yaml` 本 diff 新增 3 份（包内共 6 份）⇒ `minimalHestiaYAML(dbPath)`（≈ −12 行，既有模式延续）；`runAt` vs `sampleRun` 用途不同不合并；其余无可简化项 |

合计净减约 40 行、全部局部、无接口变更。**没有一条单独够得上 `review_fix`**；建议登记为后续一次打包处理，W1 若返工则顺手做 `groupCount`。

### 三个 dev 报过的 gitnexus HIGH（按 diff 独立判）

- 001 `Save`：函数体两 ref 提取后 diff 为空 ⇒ **误标**（同一文件新增方法被算成 `Save` 的变更）。
- 010 `config.Config` 扇出：只新增 `Hestia HestiaConfig` 字段与 `AlertRule.Cooldown`，零值语义 = 旧行为，既有调用方无一受影响 ⇒ 扇出真但风险不真。
- 006 `runServe` 扇出：只新增一次 `buildHestiaHealth` 调用；行为变化仅「`config_path` 设了但装不上 ⇒ 启动失败」，这是 spec §4.2 的要求且 `TestBuildHestiaHealth_*` 四条覆盖 ⇒ 风险已被测试兑现，非未评估风险。

### 账本 4 条变异存活的独立判定

| 条目 | 判定 |
|---|---|
| 001 M6 `rowid DESC` | **非等价变异，测试强度边界**（见 I1）：代码正确，索引让变异体碰巧同序；一行 `DROP INDEX` 即可杀 |
| 002 M3 通知失败走 `fail` | 测试强度边界：后果只是 `stage=send P0` 多出现在 status 行，无消费者读 `stage`；加 `assert.Empty(r.Stage)` 即杀 |
| 002 M7 跳过候选记成带 Period 的 `no_new` | 测试强度边界：后果是 `runs_total{no_new}` 按跳过候选数膨胀、`LastRun` 不受影响；加 `assert.Empty(runs[0].Period)` 即杀 |
| 007 M1 `UTC()` | **等价变异**（自测：`Z` 与 `+00:00` 都 Format 成 `Z`）；注释理由错（I8） |

### 已核无问题（有证据的 PASS 项）
- `MAX(run_at)` TEXT 比较：唯一写者 `store.go:626` 固定 UTC RFC3339 定宽 `…Z`，字典序 = 时间序。
- 五条查询非同一快照：同一次抓取内计数可差一行；两条规则只消费第一条查询的两个时间戳（原子），无消费者做恒等式。
- 5s 抓取超时 vs `busy_timeout(5000)`：相等是巧合非依赖；WAL 读者不等写者（实测）。
- `for 10m` + `cooldown 24h` + 60s：首条约 t0+10~11 min，之后每 24h 一条；全部通知失败时 `lastFired`/`pending` 不动、下一 tick 重试。
- 首次部署空表不假红：`TestHestiaCollector_EmptyOmitsTimestamps` + `TestBuildHestiaHealth_RegistersCollector` 断言时间戳缺席。
- `> 960`：最坏入库滞后 35d + 约 18h ≈ 858h，余量约 100h。
- `TestIngestRunRecordFailureKeepsIngestedRow` 确实证明已入库行不受影响（Save 独立提交、`HasPeriod` 读回、`assert.Less` 钉顺序）；若 RecordRun 与 Save 共事务或先于 Save，两条断言各自转红。
- `RecordRun` 未破坏「Save 唯一写入口」：ADR-0003 射程是 observations/pending，`TestRecordRunTouchesOnlyRunsTable` 钉住，CONTRACTS A4 登记。小瑕疵：A4 引「同 `BackfillLoad` 先例」，仓库里 `BackfillLoad` 不是 `Store` 方法，指涉不准（文案级）。
- `HealthSummary(Querier)`、`HestiaConfig` 只存路径、per-rule `Cooldown` 最小改动：取舍合理。

## 建议处置汇总

| 发现 | 建议 | 决定者 |
|---|---|---|
| W1 | `review_fix` TASK-003（`task_defect`，两处 `rows.Err()` + 一条测试；可并 `groupCount`）——或 CONTRACTS 挂账进 M2 首批 | Leader |
| W2 / W3 | CONTRACTS §A 挂账（「缺席 = 静默」「首个增量入库前 `no_ingest` 不设防」「从未成功运行不告警」）+ 009 验收步骤各加一条核查 | Leader（TASK-008 已 verified，由 Leader 决定是否开 review_fix 008 补 §A，或写进 final-report） |
| W4 | final-report §交付后待办写全 009 步骤（含「改的是 runtime 副本、deploy.sh 不同步」）；`docs/deployment.md:290-291` 既有漂移登记 | Leader / 009 |
| I1 | CONTRACTS §B 把 M6 从「碰巧同序」改为「非等价、索引掩盖」 | Leader |
| 其余 info + 简化项 | 一次打包进 M2 前置清单 | Leader |

## 附：lens 一致性
- Skeptic ×2：均 PASS / 4 warning，条目一致（W1、W2、I1、I2）。
- Architect ×2：PASS-with-warnings / NEEDS WORK，条目一致（W3、W4、I5），分歧仅在 W4 落点归属。
- Minimalist ×2：一致，无 review_fix 级项，唯一正确性项即 W1。
- qa-m15 本体对每条 warning 都自读代码或自跑命令核过（W1 行号、W3 `LastIngest` 来源、W4 `deploy.sh:95`、I6 PK 与 DELETE 0 命中、I8 时区实验）。
