# 架构/流程决策 · Sprint M2a（契约队列与信号快照）

设计层决策（契约键序、四信号算法、只快照阈值、文件名带 period_type、原子写、契约先于通知）已由人类在 spec 与需求文档定稿，此处不重开。以下是 Arcforge 拆分与执行层的裁决，每条带理由与证据（锚 `d27791c695e8ebd0fd5d54c9161782f52d9b12cb`）。

## AD-1 范围 = 需求 TASK-001 ～ 007；007 形态改为 docs-only；不 deploy

- 需求 007 = code-simplifier + 采锚 + 全量核对 + 真语料回归 + CONTRACTS + 合并归档。合并/归档是 Leader 动作；code-simplifier 按全局规范每个代码任务提交前各跑一次，007 只做**终检审查**（只审不改；有可采纳建议 ⇒ 报 Leader 另开 review_fix，不在 docs-only 任务里改代码）。
- 007 唯一交付物 = `internal/hestia/CONTRACTS.md` 新开 `## Sprint M2a`；全部 DoD `verify_by: review|manual`（沿 M1.5 AD-11）。
- 🔴 不跑 `deploy.sh`（需求明写：与 M1.5 一起等首期验收登记）。

## AD-2 提交锚 `<type>(TASK-00N): M2a …`；分支 `task/TASK-00N-m2a`（沿 M1.5 AD-3）

- `.claude/hooks/task-completed.sh:133/174` 用 `git log -E --grep="^[a-z]+\(${TASK_ID}\):"`；需求写的 `feat(M2a TASK-001):` **不匹配**，`:175` 的 `NONCONFORMING` 探针会 WARN 且改动对漂移检查不可见。Go 注释里仍写 `M2a 的 TASK-00N`。
- 当前 `task/*` 分支 0 条（`git branch --list 'task/*'` 为空），无同名遗留。

## AD-3 002 与 003 串行（需求说可并行）；链 001→002→003→004→{005,006}→007

- **依据**：002（`Evaluate`）、003（`BuildContract`/`Store.Current`/`Store.PriorPublishedAt`）、004（`EnsureQueueDirs`/`WriteContract`）都新增导出符号，`TestPackageExposesNoWriteFunctions`（`store_test.go:453`）是**精确集合**断言 ⇒ 每个任务都必须改 `store_test.go` ⇒ validator `scope-mutex` 不允许同时在途；且不能由 001 预登记（符号不存在时守卫同样红）。
- **代价**：关键路径多一段（002 ≈ 纯函数，小）。005 ∥ 006 仍保留并行。

## AD-4 TASK-005「契约写失败」用例的两处订正（🔴 gate 上请人拍板 4b）

- **4a 夹具**：需求把 `cfg.Queue.Dir` 指向一个普通文件——但需求同一任务把 `EnsureQueueDirs` 放在 `Ingest` **入口**，`MkdirAll(<file>/pending)` ENOTDIR ⇒ 在 Discover 之前就返回 ⇒ 用例里的 `countRows == 1`、`newestRun` 断言必红。改为：`EnsureQueueDirs(cfg.Queue.Dir)` 之后 `os.MkdirAll(<dir>/pending/2025-12-annual.json)` 把目标**文件名预建为目录**，`writeAtomic` 的 `os.Rename(tmp, path)` 在 POSIX 上对「目标是目录、源是文件」返回 EISDIR（`snapshot.go:97-108`，rename 失败会 `os.Remove(tmp)`）。判据不依赖权限位（root 下 chmod 方案会假绿）。
- **4b 判定**：需求 §A6 明写「`hestia_runs` 记 `ingested`」，而 `ingest.go` 的 `runRow`（`:246-259`）对非 `notifyError` 的错误一律记 `failed`，循环（`:186-198`）对非 `notifyError` 一律发 P1；需求用例却断言 `sender.texts` 为空。三者不可能同时成立。**裁决**：新增 `contractError{err}`（与 `notifyError` 同形态，`isContractError`）；`runRow` 对它保持 `outcome=ingested`、`Error` 列记首行、`Stage="contract"`（数据确实在库，`HealthSummary.LastIngest` 应推进；哪里断的由 stage+error 说）；**P1 照发**——P1 是失败通知，不会造成「Telegram 说入库了、队列里却没有」，反而是运维知道要 `contract emit` 补发的唯一即时信号；P2 不发。用例断言改为：`sender.texts` 恰 1 条、以 `[P1]` 开头、含 `contract`、不含 `信号 活化`；`r.Outcome == RunIngested`、`r.Stage == "contract"`、`r.Error` 含 `contract`。
- **替代方案（未取）**：(i) 记 `failed` + 发 P1：最少改动，但违背 §A6 且会让 `hours_since_last_ingest` 在数据已入库时持续增长；(ii) 记 `ingested` + P1 不发：符合需求用例字面，但契约写失败只剩退出码与 err.log，无即时通知。人类若选 (ii)，DoD 只需把「恰 1 条 `[P1]`」改回「为空」并在循环里对 `contractError` 跳过 P1。

## AD-5 `Ingest` 对空 `Queue.Dir` 报错；`writeHestiaYAMLWithDB` 补 `queue.dir`（放 001）

- **依据**：`EnsureQueueDirs("")` = `MkdirAll("pending")` 等四个**相对 cwd** 的目录。`go test` 的 cwd 是包目录 ⇒ `ingest_test.go:199` 直建 `Config{}` 的用例会在 `internal/hestia/` 里建目录；`cmd/atlas` 真走到 `hestia.Ingest` 的用例（`hestiaCfg` 闭包 `hestia_test.go:1225`、`wiringHestiaCfg` `:1342`，经 `withConfig` 内联 yaml，无 `queue` 段）⇒ `LoadConfig` 预填 `queue/hestia` ⇒ `TestHestiaIngestWiresNotify` 真入库会写 `cmd/atlas/queue/hestia/pending/2025-12-annual.json`。⚠️ 本条第一版写「10 处 `runHestiaIngest` 经 `writeHestiaYAMLWithDB`」是**假的**（reviewer B7：该 helper 在 `hestia_test.go` 零处使用），已订正。空目录 git 不跟踪，判据用 `find … -type d \( -name pending -o -name queue \)` 为空（reviewer B8）。
- **处置**：`Ingest` 入口在 `OnlyPeriod` 校验旁加 `d.Cfg.Queue.Dir == ""` ⇒ `errors.New("hestia ingest: queue.dir must not be empty")`（配置错误在任何 I/O 之前）；`ingest_test.go:199` 补 `Queue: QueueCfg{Dir: t.TempDir()}`；`hestia_test.go` 两处内联 yaml 加 `queue:\n  dir: <tmp>/queue`。后者是配置形态的事，归 001（001 因此含 `cmd/atlas`，`coverage_floor: 75`，见 AD-7）。

## AD-6 `Signals` 的 `json` tag 在 001 加（需求放在 003）

- tag 属于结构定义；003 再改 `config.go` 会让 001/003 的 `writes` 重叠且让 003 多一个文件。`TempScale` 加 `json:"-"`（需求既定：`thresholds.temp_scale` 顶层已有一份）。

## AD-7 覆盖率基线与门禁形态

- 背对背实测（锚 `d27791c`，`GOTOOLCHAIN=local go test -cover -count=1`）：`internal/hestia` **96.6%**、`cmd/atlas` **76.4%**；vet 零输出；`gofmt -l internal/hestia cmd/atlas` 恰 `cmd/atlas/backtest_test.go`、`cmd/atlas/crisis_test.go`。
- 需求硬门槛 `internal/hestia ≥ 96.6` = 基线；`cmd/atlas` 不低于 76.4。
- 含 `cmd/atlas` 的任务（001、006）`coverage_floor: 75`（门禁合并 total 会被拉到 80 附近；`task-completed.sh:401`）。约束不放宽。

## AD-8 TASK-006 用例的 `hestiaCfgPath` 还原沿既有模式

- 需求用例 cleanup 硬编码 `"configs/hestia.yaml"`，`RejectsBadPeriod` 用例改了 `hestiaCfgPath` 却**不还原**；既有用例（`hestia_test.go:147-149,170-172`）统一 `old := hestiaCfgPath; t.Cleanup(func(){ hestiaCfgPath = old })`。四个 emit 用例全部沿此模式，三个 emit 变量同样先存后还原。

## AD-9 交付协议沿 M1.5 AD-6：每 dev 独立 worktree；merge 先于 `dev_done`；Leader 串行 merge

- `task-completed.sh` 的 `git log --grep` 不带 `--all` ⇒ 未合并分支对门禁结构性不可见。自证数字在 merge 后的 master 重采；discovery 同时写「我的 commit sha」与「merge 后 master sha」。
- 语料 `data/hestia-backfill-2026-08-14` 用主仓库绝对路径（`data/` 被 `.gitignore`）。
- Leader merge 纪律：预演（`git worktree add --detach`）与正式 merge 放同一个 Bash，同条命令打 `rc=$?` 与 `git rev-parse HEAD`。

## AD-10 守卫精确项数：reflect 12 → 14；AST 25 → **34**（第一版写 31，reviewer B1/B4 订正）

- 需求「导出面 +5」列了 6 个名字；而 AST 守卫（`store_test.go:428-441`）收的是**全部导出包级函数 + 导出类型上的导出方法**，需求还漏了 `DefaultSignals`（导出函数）与 `Contract.FileName`/`Contract.JSON`（导出方法）⇒ 共 9 项：AST **34**、reflect **14**。分任务登记（字母序插入）：001 +`DefaultSignals`（26）；002 +`Evaluate`（27）；003 +`BuildContract`/`Contract.FileName`/`Contract.JSON`/`Store.Current`/`Store.PriorPublishedAt`（32）+ reflect `Current`/`PriorPublishedAt`（14）；004 +`EnsureQueueDirs`/`WriteContract`（34）。
- 教训：我按需求「列了几个名字」数，没按守卫「收什么」数——同一个数字两把尺，第一版错在没打开守卫实现。

## AD-11 Duplicate 的 P2 打 `温度 0/0`（需求既定）；`renderP2` 头注释同步

- `notify.go:62-64` 现有注释「四个冷热信号与综合温度尚未实现（等 M2）」在 005 改为描述信号行；既有 7 处 `renderP2(...)` 调用补 `Temperature{}`。

## AD-12 ECC 不可用；brainstorming 降级为「设计由 spec 定稿，不重开」（沿 M1.5）

- `arcforge.config.json` `capabilities.ecc=false`；需求已是经 superpowers writing-plans 产出的逐步计划并附 spec。

## AD-13 团队：dev × 2（`dev-m2a-a`、`dev-m2a-b`）+ test × 1（`test-m2a-a`）；QA `qa-m2a` 全部 verified 后 spawn

- 链几乎全串行（AD-3），只有 005 ∥ 006；第二个 dev 吃 006 与失联接手。`max_dev_agents=4` 未用满是刻意的：多 spawn 只会空转烧 token。

## AD-14 OutOfOrder 不写契约、不算温度（reviewer S2，需求与 spec 未覆盖）

- **依据**：`Save`（`store.go:772`）对 `OutOfOrder`（同键、更旧 `published_at` 迟到，`bitemporal/classify.go:57`）同样返回 `Table=observations`；需求 005 的条件 `Verdict != Duplicate` 会为它生成契约并**同名覆盖** `pending/` 里可能尚未消费的更新契约——违背 spec「最新的赢」。`v_hestia_current` 也不会把它当 current 行。
- **裁决**：契约与 `Evaluate` 只在 `Verdict ∈ {New, Revision}` 时做；OutOfOrder 与 Duplicate 同待遇（P2 照发，`温度 0/0`）。005 加 `TestIngestNoContractOnOutOfOrder`。007 §A 记 A9。
- **替代（未取）**：写契约但 `is_revision`/`supersedes` 标注——消费者拿到的仍是旧数据，且要靠它自己比 `published_at`，把「最新的赢」的责任推给 M3。

## AD-15 reviewer 反审处置（8 阻断 + 8 建议，全部核实、全部采纳）

| # | 结论 | 落点 |
|---|---|---|
| B1 `DefaultSignals` 未登记 | 成立（`store_test.go:433-435` 收全部导出函数） | 001 f[0] + `writes` 加 `store_test.go`；AD-10 |
| B2 `articleURL` 与 `ingest_test.go:82` 重名 | 成立（19 处调用） | 003 f[0]：改名 `pbocArticleURL` |
| B3 `contract_test.go` 多 import `context` | 成立（需求 import 块有、用例无用） | 003 f[0] |
| B4 `Contract.JSON`/`FileName` 进导出面 | 成立（`:437-441`） | 003 f[2]：32；AD-10 |
| B5 `config_test.go:387` 钉 `2026-09-03` | 成立 | 001 f[1] |
| B6 直传 `hestiaContractEmitCmd` ⇒ `Context()` nil ⇒ sql panic | 成立（`newCapturingCmd` `:131-139` 正为此 `SetContext`） | 006 f[1] |
| B7 AD-5 指错文件 | 成立（`writeHestiaYAML` 在 `hestia_test.go` 0 次；真入库用例 `:1225`/`:1342`） | 001 f[2]；AD-5 订正 |
| B8 `git status` 对空目录恒空 | 成立 | 001/005/007 判据改 `find` |
| S1 `encoding/json` import | 成立 | 005 f[1] |
| S2 OutOfOrder | 成立 | AD-14；005 f[1]；007 A9 |
| S3 `SilenceUsage`/`IsRegistered` 守卫 | 成立 | 006 b[1] |
| S4 `writeAtomic` 文案 | 接受不改 | 005 f[2]；007 §A |
| S5 回归 `find` | 采纳 | 007 f[3] |
| S6 golden 只读查 `data/hestia.db` | 采纳 | 002 b[1] |
| S7 `--period-type` 错误串 | 采纳 | 006 b[0] |
| S8 计数顺延 | 采纳 | 002/004/007 |
| AD-4b 意见 | 与 Leader 一致选方案一；补一句「`Notified==true` 依赖 P1 发送成功」 | 005 f[2]；gate 上仍由人拍板 |

## AD-16 007 的回放样本改为两份：2026-06/h1（累计口径，`_mom` 为空）+ 2023-08/monthly（`_mom` 20 个）

- **依据**：需求 TASK-007 Step 4 的 Expected 写「`data` 含 `_ytd` 与 `_mom` 两族」，但 h1 报告本就是累计口径（需求 A3 自己也这么说）；dev-m2a-b 在 007 派发前试跑发现 2026-06/h1 的 `data` 无 `_mom` 键，Leader `sqlite3 -readonly data/hestia.db` 核实：2026-06/h1 与 2025-12/annual 的 22 个 `_mom` 列全空，2023-08/monthly 非空 20 个。
- **裁决**：不删判据、不换期次——保留 2026-06/h1 并把判据改成「`_mom` 族 0 个、全进 `absent_fields`」，另加 2023-08/monthly 作第二份样本核「`_mom` 原样进 data」。两份都是 M3 写消费者的对照样本，多一份形态更有用。
- **未取**：只留 2026-06/h1 并删掉「两族」判据——那样「`_mom` 进 data」这条需求行为在回放路径上就没有样本证据。
