# Final Report · Sprint M1.5（Sprint 044 · Hestia 健康度可观测）

**判定对象**：master `c18bf112d69c1ef974e977efd31fc1c37ab34a19`（= `5dafbf0` 代码树 + Leader 的 CONTRACTS §C 提交；QA 判定对象 `a03293d`，其后只合入 003 W1 返工 `5dafbf0` 与 §C 文档）（基线 `037d1eb1e4f827c415319519e40f4e2208968920`）
**需求源**：`hestia/docs/superpowers/plans/2026-09-04-hestia-m1.5-health.md`；spec `specs/2026-09-04-hestia-m1.5-health-design.md`
**射程**：需求 001–008 → Arcforge TASK-001～008 + TASK-010（AD-2）；需求 009 结转（AD-1）
**时间**：首次派发 10:17Z → 9/9 verified 13:38Z（3h21m）→ QA PASS 14:32Z → W1 返工复验 15:59Z → 9/9 accepted 16:00Z（全程 5h43m）
**结论**：QA **PASS**（0/4/13）；9/9 `accepted`；009 结转

## 1. 完成任务（9/9 accepted，返工 1 = 003 的 QA W1）

| TASK | 内容 | dev / verifier | dev commit → merge | 变异 |
|---|---|---|---|---|
| 001 | `hestia_runs` 表、`Run`、`RecordRun`/`RecentRuns`、两条写口守卫登记（12/24 项） | dev-m15-a / test-m15-a | `5b9859b` → `511ee42` | 5/6 KILLED（M6 `rowid DESC` 存活，边界） |
| 005 | `alert.Rule.Cooldown`，未写退回 5m | dev-m15-b / test-m15-a | `d55b66e` → `2db8519` | 2/2 |
| 010 | `config.AlertRule.Cooldown` + `HestiaConfig{ConfigPath}` + `mapRules` 透传（AD-2 新增） | dev-m15-b / test-m15-a | `2658e5d` → `5c1f8e8` | 4/4 |
| 002 | `Ingest` 逐候选写 runs，零行 `no_new` 心跳；`RecordRun` 失败不影响已入库行**已测**（reviewer B2） | dev-m15-a / test-m15-a | `cbac195` → `c1defc0` | 7/9（M3/M7 存活，断言强度边界） |
| 007 | `hestia status` runs 段（`runsLimit` 5）；销 M1d 挂账 C2 第二半 | dev-m15-c / test-m15-a | `f9410c0` → `e2d1f2b` | 5/6（M1 `UTC()` 存活，预期） |
| 003 | `HealthSummary`；`duplicate` 不推进 `LastIngest`；**QA W1 返工**：`groupCount` + `rows.Err()` | dev-m15-b（实现 dev-m15-a，接手零改动；W1 由 dev-m15-b）/ test-m15-a → 复验 test-m15-b | `266fe40` → `321dfb6`；W1 `53f1412` → `5dafbf0` | 6/6 + W1 5/5；rework 1 |
| 004 | `metrics.HestiaCollector` 九指标；空表不输出时间戳；出错只计 `collect_errors` | dev-m15-c / test-m15-a | `3ba1e9f` → `4c14d79` | 8/8 |
| 006 | `serve` 按 `hestia.config_path` 注册 collector（三种语义）；样例配置两规则；部署文档 | dev-m15-b / test-m15-a | `6007f72` → `e5ada52` | 9/9 |
| 008 | docs-only 收口：采锚、全量核对、真语料回归、CONTRACTS `## Sprint M1.5` §A（A1–A7）/§B | dev-m15-a / test-m15-a | `729b4fe` → `a03293d` | — |

diff（`037d1eb..c18bf11`，代码范围）：28 文件 +1883/−43 再加 W1 返工 +87/−22 与 §C +14；20 个 commit（9 feat/docs + 1 fix + 9 merge + 1 docs）。新增 `Test` 函数 39（需求预估 29，差因见 CONTRACTS §B）。

## 2. 覆盖率与门禁（锚 `a03293d`，验证者与 dev 背对背一致）

| 包 | 基线 `037d1eb` | 交付 | 门禁 |
|---|---|---|---|
| `internal/hestia` | 96.5% | **96.6%** | 硬门槛 96.3 ✓ |
| `internal/metrics` | 98.9% | **99.2%** | ✓ |
| `internal/alert` | 92.3% | **92.6%** | ✓ |
| `internal/config` | 83.3% | 83.3% | ✓ |
| `cmd/atlas` | 76.3% | **76.4%** | floor 75（AD-4）✓ |

gofmt 五包仅三处既有欠账；vet 零输出；四个不动文件与 `go.mod`/`go.sum` diff 为空；`Save` 函数体 0 触碰；真语料回归六数逐字相等（218=217+1 · 217=213+4 · 97=76+21 · 冲突 0 · 路由违反 0）、`hestia_runs` 0 行。迁移审计 64 条、5 个写者全登记、零越权（validator 全规则通过）。

## 3. 🔴 待同步 hooks 清单

### 待同步 hooks 清单(人类执行)
| 文件 | 变更摘要 | 同步命令 |
| --- | --- | --- |
| （无） | 本 Sprint 未改动任何 `project-template/hooks/`、`project-template/scripts/`（本仓库是消费项目，无该目录）；运行时 `.claude/`、`CLAUDE.md`、`AGENTS.md`、`.agents/` 亦无改动（`git diff --stat 037d1eb 5dafbf0 -- .claude CLAUDE.md AGENTS.md .agents` 为空） | — |

复制后对新增/改动脚本补可执行位,再运行 `bash project-template/scripts/check-runtime-sync.sh`
核对运行时副本与模板一致(agent 只呈现清单,不执行这些命令)——本 Sprint 清单为空，无需执行。

唯一非代码、非 `.arcforge/docs` 的改动：`.arcforge/write-matrix.json`（登记 5 个 token 的 sha256），随归档提交。**机制变更需求全部落在上游 ArcForge**（见 §7），本仓库不改 hook。

## 4. Review 结果

**qa-m15 两轮 Code Review：PASS**（0 critical · 4 warning · 13 info；`docs/05-review/qa-verdict.md` 18.2 KB）。第二轮 codex CLI 30 分钟零输出 ⇒ 按 CLAUDE.md 降级为纯 Claude 跨视角（Skeptic / Architect / Minimalist 各两次独立 context，结论一致；Architect 两次仅在 W4 落点归属分歧，不构成 CONTESTED）。

| # | 发现 | 处置 |
|---|---|---|
| W1 | `health.go:54,71` GROUP BY 循环缺 `rows.Err()`，中断时返回部分计数且 err=nil | **`review_fix` TASK-003**（task_defect，rework 1）：`53f1412` → `5dafbf0`，抽 `groupCount`（`defer Close` + `rows.Err()`）+ `TestHealthSummaryReportsRowsErr`（确定性 cancel 夹具），变异「删 `rows.Err()`」KILLED；test-m15-a 复验 VERIFIED（15:59Z，5/5 变异 KILLED；首验者 test-m15-a 57 分钟无产物，经 stale-dispatch 第 2 步改派 test-m15-b，见 §6 事故 7） |
| W2 | 「库读不到」零告警：指标缺席 ⇒ 规则 false 且清零 `for`；样例无规则消费 `collect_errors_total` | CONTRACTS `## Sprint M1.5` **§C1** + 009 验收加「`collect_errors_total` 必须为 0」 |
| W3 | `hestia_no_ingest` 首次真实增量入库前结构性不触发 | CONTRACTS **§C2** + 009 验收加「首期入库后确认 `hours_since_last_ingest` 出现」 |
| W4 | 投递步骤不在仓库文档；`deployment.md:290-291` 与 `deploy.sh:95` 相反 | 本报告 §8 逐条写；CONTRACTS **§C3** |
| info×13 | 含 I13（`metrics.enabled=false` 且设了 `config_path` 只打 Info——spec §4.2 明文定义的降级、非缺陷，009 前置：确认 runtime `config.yaml` 的 `metrics.enabled: true`）、001 M6 实为非等价变异（索引掩盖）、gitnexus 三个 HIGH 按 diff 判无回归、code-simplifier 终检净减约 40 行无一够 review_fix | §B 口径订正 + **§C4**（M2 首批打包） |

第一轮 QA 自跑证据：四不动文件 / `go.mod` diff 0 行；`Save` 函数体 diff 空；两守卫 12 / 25；vet 0；五包覆盖率与 §B 一致；另实测「写事务未提交时 `CREATE … IF NOT EXISTS` 不阻塞」（serve 启动撞 ingest 写安全）与「`time.Parse` 的 Z 与 +00:00 都 Format 成 Z」（007 M1 是等价变异，注释理由错）。

## 5. 已知开口与账本（`plan.md` 账本 10 条，摘要）

- 需求文档三处断言被证伪：「`RecordRun` 失败无法构造」（B2，改为必测）、「`TestDDLIsIdempotent` 仍绿」对 runs 表恒真（B1）、心跳条件 `recorded == 0` 会补假心跳（S2）。
- 验证者变异存活 4 条（非缺陷，测试强度边界）：001 M6、002 M3/M7、007 M1——已登记 CONTRACTS §B。
- 仪器边界两条：`git diff --numstat` 对净零行改动无鉴别力（007 假阴，dev 自报）；code-simplifier 子代理可能 0 工具调用即报「无可改」（008，以载体为准）。
- 门禁 `git log --grep` 命中旧 sprint 同名 TASK-007 的 10 条提交：只 WARN 不参与漂移判定（`--since` 截断），跨 sprint 复用 ID 的根因留待决。

## 6. 事故（7 起，全部已处置；成因未定的标明）

| # | 事故 | 处置 | 归因 |
|---|---|---|---|
| 1 | 005 merge 预演命令缺 `--detach` 未跑成、正式 merge 未受守卫 | 事后补核；后续预演 `--detach` + `&&` | Leader 命令错 |
| 2 | dev-m15-b 的 code-simplifier 子代理被 TeammateIdle hook 卡死循环 | 叫子代理直接返回；结论转本体 | **待决机制 #3 第二实例** |
| 3 | 007 派验通知疑似未达 | 验证者重扫磁盘自行承接（文件真相源自愈） | 验证者时序表述错，非丢失 |
| 4 | dev-m15-a 在 003 上静默 45 分钟（running 零产出） | 按第 5 节第 2 步收回改派 dev-m15-b，接手零改动 | **会话挂起一族，成因未定** |
| 5 | Leader 自身 11:54Z→12:21Z 被挂起 27 分钟 | dev 催办后补 merge | **同族；M1d 事故 2/3 同形，3 次 / 2 sprint** |
| 6 | dev-m15-c 提交后 merge 请求晚到 23 分钟 | 预演先行，未改派 | **同族（消息延迟）** |
| 7 | test-m15-a 在 003 复验上 57 分钟零产物（validator `stale-dispatch` 首次命中） | 第 1 步重发（15:26Z）→ 第 2 步逃生边改派 test-m15-b（15:57Z，baseline 不刷新） | **同族（会话挂起，成因未定）**；顺带一条教训：同机多会话时进程/临时目录不是某实例的活性证据 |

## 7. 待决机制（进 `PENDING-MECHANISMS.md`，落点上游 ArcForge）

1. **会话/消息挂起无告警**：本 sprint Leader 1 次 + dev 2 次 + verifier 1 次，M1d Leader 2 次；形态一致（工具调用之间被挂起、被挂起方无间隙感知）；`in_progress` 刻意无阈值 ⇒ 只能靠对方催办。候选：dev 侧 merge 请求超阈值无回执 ⇒ 自动 `blocked_clarification`；Leader 侧无解。
2. TeammateIdle hook 未排除子代理（待决 #3 频次 2）。
3. 任务 ID 跨 sprint 复用致门禁 `git log --grep` 全集膨胀（只 WARN，无害）。

## 8. 交付后待办（人类）

- **需求 TASK-009 投递与验收**：前置 M1d §G 首期验收（窗口 2026-09-09～09-15）；`ANCHOR = c18bf112d69c1ef974e977efd31fc1c37ab34a19`（9/9 accepted 后、归档提交前的 master；归档提交只加 `.arcforge/archive/`，代码树不变）；步骤按需求原文 Step 0–6（运行时 `config.yaml` 两条规则 + `hestia.config_path`、`deploy.sh` + kickstart、三条验收、CONTRACTS §C/§D、销 M1d C2 第二半、vault 回写）。
- ⚠️ **运维语义**：`configs/config.example.yaml` 现声明 `hestia.config_path: configs/hestia.yaml`；照抄样例而无 `hestia.yaml` 的环境 serve 会以 `hestia health: loading …` 启动失败（spec「装不上即响亮失败」）。运行时 `config.yaml` 由人改，`deploy.sh` 不覆盖。
- CONTRACTS `## Sprint M1.5` §C（`c18bf11`）登记 C1–C5 挂账与 §B M6 口径订正。
- 009 验收另加三条核查（QA）：`hestia_collect_errors_total` 必须为 0（C1）；首期入库后 `hestia_hours_since_last_ingest` 出现（C2）；runtime `config.yaml` 的 `metrics.enabled: true`（I13）。
- 本 Sprint **未跑 `deploy.sh`**（验证者旁证：运行时 `bin/atlas` mtime 早于首次派发）。
