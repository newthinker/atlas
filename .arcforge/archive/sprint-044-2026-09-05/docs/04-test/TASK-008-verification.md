# TASK-008 验证报告 · 代码收口（docs-only）：采锚、全量核对、真语料回归、CONTRACTS `## Sprint M1.5` §A/§B

- 验证者：test-m15-a · 2026-09-04
- 判定对象：`verify_baseline.head = a03293d8c74a78e013368aee92a5b8ed7cd177c5`（master，merge of dev commit `729b4fe504a0e05fa68ab1bc2b57749f4108dcc5`，dev-m15-a）；当前 HEAD 与之一致，无漂移
- discovery：`.arcforge/discoveries/TASK-008.json` sha256 `2a61f2a36a409fd3c0a486fe055570c82619d7f341332830d6510a5c9ee1957c`（与基线一致）
- 验证环境：`git worktree add --detach ../wt-verify-TASK-008 a03293d8c74a78e013368aee92a5b8ed7cd177c5`；所有数字由我在该树**自己重跑**。`git diff --stat e5ada52 a03293d` 仅 `internal/hestia/CONTRACTS.md` 83/0 ⇒ 与 §B 锚 `e5ada52` **代码同树**，§B 数字可直接与我在 `a03293d` 的实测比对
- DoD 全部 `verify_by: review`（functional 4 + boundary 1 + error_handling 1 + non_functional 1）

## 结论：**VERIFIED**

7 条 done_criteria 全部核对通过；§B 每一格都与我自跑的数字相等；真语料回归六个数一个不变；提交只含 CONTRACTS.md；未投递。

## 范围核对

`git show --numstat 729b4fe`：`internal/hestia/CONTRACTS.md` 83/0——唯一文件，与声明 `writes`/`packages`（都指向 CONTRACTS.md）一致。主仓库 `CONTRACTS.md` sha256 与 `729b4fe` 树内相同（`4981b05f…`）。

## §B 逐格比对（§B 锚 e5ada52 ／ 我的实测锚 a03293d，代码同树）

| §B 行 | §B 写的 | 我自跑（a03293d） | 相等 |
|---|---|---|---|
| hestia 覆盖率 | 96.6（基线 96.5，门槛 96.3） | **96.6** | ✓ |
| metrics / alert / config / cmd/atlas | 99.2 / 92.6 / 83.3 / 76.4 | **99.2 / 92.6 / 83.3 / 76.4**（基线 98.9 / 92.3 / 83.3 / 76.3，我在准备阶段于 037d1eb 自采） | ✓ |
| gofmt 五包 | 三处既有欠账 | `snapshot_test.go` / `backtest_test.go` / `crisis_test.go` 恰三项 | ✓ |
| go vet | 零输出 | rc=0 零输出 | ✓ |
| 导出面 | AST 25 / reflect 12 | 机械计数 **25 / 12** | ✓ |
| 三守卫 + 字段名守卫 | 四条 PASS | `TestStoreExposesNoWriteMethods` / `TestPackageExposesNoWriteFunctions` / `TestRecordRunTouchesOnlyRunsTable` / `TestFieldNamesAppearOnlyInFieldsGo`（另 `TestDDLIsIdempotent`）`--- PASS` | ✓ |
| 四不动文件 | 空 | `git diff --stat 037d1eb a03293d -- parse/extract/validate/fields.go go.mod go.sum` 空 | ✓ |
| `Save` 函数体 | 0 | DoD 原命令输出 **0** | ✓ |
| go.mod / go.sum | 无 diff | 空 | ✓ |
| 真语料回归 | 218=217+1 · 217=213+4 · 97=76+21（28+69）· 冲突 0 · 违反 0 · runs 0 行 | 见下节，逐字相等 | ✓ |
| 新增测试 | 39（schema 1 · store 7 · ingest 10 · health 6 · status 2 · metrics 4 · alert 1 · config 1 · cmd 7=5+2） | `grep -c '^+func Test'` = **39**；按文件：schema_test 1、store_test 7、ingest_test 10、health_test 6、status_test 2、hestia_collector_test 4、evaluator_test 1、config_test 1、hestia_health_test 5、hestia_test 2——逐文件相同 | ✓ |
| 「已测」行 | `TestIngestRunRecordFailureKeepsIngestedRow` | 该测试存在且我在 TASK-002 验证中 M1/M2 变异证明其断言承重 | ✓ |
| 验证者变异存活登记 | 001 M6 / 002 M3、M7 / 007 M1 | 与我四份报告（`TASK-001/002/007-verification.md`）逐条一致；004/003/005/006/010 无存活，§B 未列，正确 | ✓ |

## Done Criteria 覆盖矩阵

| # | 完成标准 | 证据 | 判定 |
|---|---|---|---|
| functional[0] | 采锚前置（AD-7）：代码范围 status 空、`ANCHOR` 全 sha、写前 numstat 空、§B 表头填锚 | 主仓库 `git status --short -- internal cmd/atlas configs docs go.mod go.sum` 空；§B 表头锚 `e5ada52396fc088434a5def263fb7c23c5347607` 全 sha；`e5ada52..a03293d` 只差 CONTRACTS.md ⇒ 采锚后代码未动；discovery 记「采前采后 HEAD 同值、写前 numstat 空」 | PASS |
| functional[1] | 全量核对九项 | 上表逐格相等 | PASS |
| functional[2] | 真语料回归六个数一个不变 + `hestia_runs` 0 | 目录 `scratchpad/m15-reg-test-m15-a`（带我实例名）；`go build` 自 a03293d rc=0；`backfill load --allow-incomplete` rc=0；输出：语料总篇数 218 / 待解析 217 / 本迭代不解析 1 / 解析成功 213 / 解析失败 4 / 合并后观测 97（单篇 28 + 合并组 69）/ 入权威表 76 / 落 pending 21 / 字段冲突 0 / 口径路由违反 0——与 M1c-4 §B（`CONTRACTS.md:3259`）及 M1d §B 逐字相等；`sqlite3 reg.db "select count(*) from hestia_runs"` = **0**，`sqlite_master` 三表 `hestia_observations / hestia_pending / hestia_runs` | PASS |
| functional[3] | §A A1–A6 按需求原文 + A7 列全 + AD-13 旁注；§B 各行；新增测试用 grep 核对、差因按任务；「未测」改「已测」 | A1–A6 六条关键句 `grep -F` 在需求原文命中；A7 表：`has article`（枚举外）/ `fetch <URL>`（带 URL）/ `snapshot` `parse` `validate` `save` / `mismatch` / （空 = 通知失败不走 fail）七取值列全 + `HealthSummary` 不消费 stage + AD-13 旁注（Discover 失败 / only-period 零候选不记行）——与 AD-14 及 TASK-002 discovery 一致；`## Sprint M1.5` 恰 1 处；§B 差因表按任务列 001 +2 / 002 +2 / 003 +2 / 006·007·010 8 vs 4，与我逐任务验证时看到的新增测试一致（001 补 CorruptTimestamps/PropagateDBErrors、003 补 RejectsCorruptRunAt/PropagatesQueryErrors、002 OnlyPeriod+B2、006 ExampleConfig、007 B3 两条、010 解码测试） | PASS |
| boundary[0] | code-simplifier 终检；若提出改动 ⇒ 不改代码、写 decisions、澄清环 | discovery `key_findings[5]`/`decisions[3]` 与 §B 末行**如实写明**：子代理只审查禁改、回复无文字结论、追问一次未得、以载体（AD-7 口径 status 空、HEAD 仍 e5ada52、tracked diff 零）记「无改动」，并已向 Leader 申报；子代理未提出任何改动 ⇒ DoD 澄清环条件未触发。处置如实、无掩饰（把「没拿到结论」和「没有改动」分开写了）。QA 将独立终检，本报告不代做 | PASS（review） |
| error_handling[0] | 数字同锚；提交只含 CONTRACTS.md；锚 `docs(TASK-008):`；§B 无占位符 | `git show --numstat 729b4fe` 唯一文件；信息 `docs(TASK-008): M1.5 CONTRACTS 新开 Sprint M1.5——…`；§B 每格都是实测值（无 TBD/占位）；discovery `verification` 每项带 `@e5ada52` 锚 | PASS |
| non_functional[0] | docs-only 流程；merge 后 dev_done；discovery `files_modified` + 挂指针；不跑 deploy.sh | merge `a03293d` 在 master；`transitions.jsonl` 记 dev_done 13:34 前 update discovery；`files_modified: [internal/hestia/CONTRACTS.md]`；**未投递旁证**：运行时 `/Users/zuowei/workspace/runtime/atlas/bin/atlas` mtime 2026-09-04 08:51:54 CST（早于 sprint 首次派发 10:20:43Z = 18:20 CST）、`strings` 里 `hestia_runs_total` 出现 **0** 次（不含本 Sprint 代码）、LaunchAgents 11 个 plist mtime 08:52（同早于开工）、运行时目录 18:20 后只有 `qlib_csv_hk/*.csv` 行情数据变动 | PASS（review） |

## 备注（不影响判定）

- 真语料回归 harness 首跑因 zsh 对 `rm -f $S/reg.db*` 无匹配即中断整条命令而未 build（`load_rc=127`），改显式文件名后重跑正常；报告数字取自重跑。
- 本 Sprint 8 个代码任务 + 本 docs 任务全部 verified；结转项 TASK-009（人执行，AD-1）与 `PENDING-ACCEPTANCE.md` 窗口不在本报告射程内。
- 复现命令（锚全 sha）：`git worktree add --detach <dir> a03293d8c74a78e013368aee92a5b8ed7cd177c5 && cd <dir> && GOTOOLCHAIN=local go build -o <S>/atlas ./cmd/atlas && <S>/atlas hestia backfill load --dir <主仓库>/data/hestia-backfill-2026-08-14 --db <S>/reg.db --allow-incomplete`
