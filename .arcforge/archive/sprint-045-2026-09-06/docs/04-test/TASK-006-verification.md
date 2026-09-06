# TASK-006 验证报告 · M2a `atlas hestia contract emit`

- **验证者**：test-m2a-a
- **判定**：**VERIFIED**
- **判定对象**：`verify_baseline.head = 189a9eab5f46d760fd4e4665d2d6e2b0981c5522`（master merge commit，父 `f2079584`（含 TASK-005）+ dev 提交 `2427b6d052280bc433c3f2d8d725c71f5dc9d0cb`）；discovery sha256 `74ebeb9d4a9af29afbb78776037ed50ff64eb0baf9c756106acbf692b89cdf39`；承接时 `assignment_epoch=1`
- **验证树**：`../wt-verify-TASK-006-a`（`git worktree add --detach … 189a9eab5f46d760fd4e4665d2d6e2b0981c5522`），全部数字自采于此树，不取 discovery 的数
- **开工锚**：`d27791c695e8ebd0fd5d54c9161782f52d9b12cb`

## 0. 标准逐字提取

`jq '[.done_criteria // {} | .[]? | .[]?] | length' .arcforge/tasks/TASK-006.json` = **7**（functional 2 / boundary 2 / error_handling 1 / non_functional 2）。原文已存 scratchpad `test-m2a-a-TASK-006-dod.json`，矩阵行数 = 7。

## 1. 完成标准覆盖矩阵

| # | 标准逐字来源 | 完成标准（摘要） | 对应测试 / 证据（自采） | 判定 |
|---|---|---|---|---|
| 1 | functional[0] | 三变量、`hestiaContractCmd`/`hestiaContractEmitCmd`（Long 按原文）、`runHestiaContractEmit`（`--period` 先过 `hestiaBackfillFromRE` 再 `openHestia`；无行 ⇒ `<p>/<t> not in observations`；`IsRevision: prior != ""`/`Supersedes`；`Passed:true`/`Replay:true`；`--stdout` 只写 `JSON()`；否则 `WriteContract` 并打印）、`init` 三 flag + `MarkFlagRequired` + 两级 `AddCommand`（替换原 `:249`）；`--help` 列三 flag | `git show 2427b6d -- hestia.go` 逐项核对（顺序：period 正则 → period-type 正则 → openHestia → Current → PriorPublishedAt → BuildContract → stdout/WriteContract）；`go run ./cmd/atlas hestia contract emit --help` 列出 `--period`（required）、`--period-type`（default monthly）、`--stdout`；变异 M1/M2/M3/M4/M5/M6/M9/M10/M13 全部 KILLED（§3）。偏离申报：`MarkFlagRequired` 错误改 `panic`（原文 `_ =`），与同文件既有三处写法一致，接受 | PASS |
| 2 | functional[1] | 五条原文用例 + `emitFixture`（显式 `queue.dir`）全 PASS；三条真跑用例用 `newCapturingCmd()`；AD-8 先存后还原、不硬编码 `configs/hestia.yaml` | `-run 'TestHestiaContractEmit' -v`：Flags / RejectsBadPeriodBeforeOpeningDB / WritesPending / StdoutDoesNotWrite / UnknownPeriodFails（+ 新增 RejectsBadPeriodType）6 条 `--- PASS`；五条改变量的用例各调一次 `restoreEmitGlobals(t)`（diff 数 5 处；helper 内 `old… := …; t.Cleanup(还原四变量)`，`"configs/hestia.yaml"` 在新增行中 0 次）；真跑三条 + 两条 reject 用例均 `cmd, _ := newCapturingCmd()`；`TestHestiaContractEmitFlags` 仍查 `hestiaContractEmitCmd.Flags()`。偏离申报：`emitFixture` 报告带一条 `CheckPassed`——核 `store.go:937` 确有「Passed with zero checks」拒绝守卫，原文夹具必被 `Save` 拒，接受 | PASS |
| 3 | boundary[0] | `--period-type` 五值之外开库前报错，文案 `--period-type %q 非法：要 monthly\|q1\|h1\|q1_q3\|annual`；补一条测试；`--stdout` 末尾恰一个 `\n` | `hestiaPeriodTypeRE = ^(monthly\|q1\|h1\|q1_q3\|annual)$` 与 `hestiaBackfillFromRE` 同形，位于 `openHestia` 之前；`TestHestiaContractEmitRejectsBadPeriodTypeBeforeOpeningDB` 断言文案逐字 + `NotContains(/nonexistent/hestia.yaml)`；`StdoutDoesNotWrite` 断言末字节 `\n` 且倒数第二字节非 `\n`；变异 M2（去校验）、M13（校验挪到开库后）、M8（多一个换行）KILLED | PASS |
| 4 | boundary[1] | `hestia.go` 不 import `path/filepath` 守卫仍 PASS；`SilenceUsage` 列表加两命令；`IsRegistered` 加 `contract` | `TestHestiaCmdDoesNotResolveDBPath` PASS，import 块 `filepath` 0 次；`TestHestiaCommandsSilenceUsage` 列表 5 项（+`hestiaContractCmd`、`hestiaContractEmitCmd`）PASS；`TestHestiaCommandIsRegistered` `Contains(subs, "contract")` PASS；变异 M7（emit 去 SilenceUsage）、M9（不挂 contract）KILLED | PASS |
| 5 | error_handling[0] | 红阶段 `undefined: hestiaContractEmitCmd` 记进 discovery；`openHestia` 失败带路径；`Current` 错误原样返回 | 隔离目录（`git archive 057a91c` + `2427b6d` 的 `hestia_test.go`）复现：`:110:82 undefined: hestiaContractCmd`、`:110:101 undefined: hestiaContractEmitCmd`、`:1441:58/76 undefined: hestiaEmitPeriod/…Type`，与 discovery `verification.red_phase` 逐行相同；`openHestia` 既有（`TestHestiaConfigErrorNamesThePath`）；`Current` 错误路径 `return err` | PASS |
| 6 | non_functional[0] | 门禁 | 见 §2 | PASS |
| 7 | non_functional[1] | 交付流程（AD-9） | `verify_by` 性质为流程审查：提交 `2427b6d` 锚 `feat(TASK-006): M2a …`；`task/TASK-006-m2a` 分支存在；merge `189a9ea` 父 = `f2079584` + `2427b6d0`；discovery 同时记 `commit_sha`/`merged_master_sha`，`sampled_on` 带全 sha；审计行：`update discovery` 23:54:05Z 早于 `dev_done` 23:54:09Z；`git worktree list` 无 `wt-TASK-006-m2a`/`pre-TASK-006` 残留；改动恰 2 文件 = `writes` | PASS（review） |

## 2. 门禁实测（验证树 `189a9eab5f46d760fd4e4665d2d6e2b0981c5522`）

| 项 | 命令 | 结果 | 门槛 |
|---|---|---|---|
| 测试 + 覆盖率 | `GOTOOLCHAIN=local go test ./internal/hestia/ ./cmd/atlas/ -cover -count=1` | 两包 ok；`internal/hestia` **96.6%**、`cmd/atlas` **76.6%** | ≥96.6 / ≥76.4 |
| vet | `go vet ./internal/hestia/... ./cmd/...` | 零输出 rc=0 | 零输出 |
| gofmt | `gofmt -l internal/hestia cmd/atlas` | 恰 `cmd/atlas/backtest_test.go`、`cmd/atlas/crisis_test.go` | 只允许这两处 |
| 不动文件 + 依赖 | `git diff --stat d27791c 189a9ea -- parse.go extract.go validate.go fields.go go.mod go.sum` | 0 行 | 空 |
| `Save` 函数体 | DoD 给的 grep 链 | 0 | 0 |
| `filepath` | import 块 grep | 0 | 不 import |
| 杂目录 | `find internal/hestia cmd/atlas -type d \( -name pending -o -name queue \)`；`ls -d queue` | 0 条；仓库根无 `queue/` | 空 |
| 注释前缀 | 新增行中 `TASK-006` 不带 `M2a 的` | 0 条 | 0 |
| 越界申报 | `git diff --stat f207958 189a9ea` / `git show --stat 2427b6d` | 恰 `hestia.go` 97 行、`hestia_test.go` 147 行 = `writes` | 无声明外文件 |
| code-simplifier 申报 | dev 申报「回复无改动、实改一处（`ContractInput` 字面量逐字段一行 + 注释）」 | diff 中 `ContractInput{` 为逐字段一行并带 `Passed 恒 true 而 Checks 留空` 注释，与申报形状一致；行为不变 | — |

## 3. 变异测试（我独占的验证树内就地变异 + `git checkout` 还原；每个变异体先打 diff、过 gofmt/vet；主仓库两文件 sha256 + `git status` 指纹前后一致 `b16d445a0be06b39`；收尾被测树 0 行改动）

| # | 变异（`cmd/atlas/hestia.go`） | 结果 | 致红测试 |
|---|---|---|---|
| M1 | 去掉 `--period` 格式校验 | KILLED | RejectsBadPeriodBeforeOpeningDB |
| M2 | 去掉 `--period-type` 校验 | KILLED | RejectsBadPeriodTypeBeforeOpeningDB |
| M3 | `Replay: true → false` | KILLED | WritesPending |
| M4 | `IsRevision` 恒 true | KILLED | WritesPending |
| M5 | `--stdout` 分支失效（也落盘） | KILLED | StdoutDoesNotWrite |
| M6 | `not in observations` 文案改 | KILLED | UnknownPeriodFails |
| M7 | emit 命令 `SilenceUsage: false` | KILLED | TestHestiaCommandsSilenceUsage |
| M8 | `--stdout` 末尾多一个 `\n` | KILLED | StdoutDoesNotWrite |
| M9 | `hestiaCmd` 不挂 `hestiaContractCmd` | KILLED | TestHestiaCommandIsRegistered |
| M10 | 路径打印 `→` 改 `->` | KILLED | WritesPending |
| M11 | `Supersedes: prior → ""` | **SURVIVED** | — |
| M12 | `Report{Passed: false}` | **SURVIVED** | — |
| M13 | `openHestia` 挪到两条参数校验之前 | KILLED（2 条） | RejectsBadPeriod… / RejectsBadPeriodType… |

**11/13 KILLED**。M11/M12 存活的原因：`emitFixture` 只有单版期次，cmd 层没有「修订过的期次回放 ⇒ `is_revision: true` + `supersedes_published_at`」的用例（M4 能杀是因为单版下 `is_revision` 应为 false）；`validation.passed` 也无断言。两者的实现按 DoD functional[0] 在 diff 里逐项核过，是**测试覆盖缺口不是实现缺陷**；DoD functional[1] 只要求五条原文用例，故不构成 `task_defect`。

## 4. 结论与残留

- **VERIFIED**：7 条 DoD 逐条有证据；`verify_by: test` 条目的断言经变异证明真在守卫；门禁逐项达标（`cmd/atlas` 76.4 → 76.6）；改动文件集 = `writes`；红阶段可复现；discovery 自证数字与我自采逐项一致（96.6 / 76.6 / gofmt / vet / numstat 96+1、146+1）。
- **非阻断残留（建议落点 TASK-007 终检或 QA）**：给 `cmd/atlas/hestia_test.go` 补一条「修订过的期次回放」用例——夹具再 `Save` 一条同期、`PublishedAt` 更晚、不同 `ArticleID` 的观测，断言输出含 `"is_revision": true` 与 `"supersedes_published_at": "2026-01-15"`，顺带断言 `"passed": true`。期望 M11/M12 转 KILLED。
- dev 申报的两处偏离（`MarkFlagRequired` panic、夹具带一条 check）均有代码依据，已接受。
- GitNexus `detect_changes` 在 dev 侧未能执行（索引陈旧），本任务只加子命令与非导出符号，我以 `git diff` 与全量测试替代，不额外阻断。

---

## 5. 复验（review_fix R1，2026-09-06）

- **判定**：**VERIFIED**（复验）
- **判定对象**：`verify_baseline.head = 7022d01d9229314f8a43c126d9b9763bbabefe5c`（merge commit；fix 提交 `bb488d4a9cdc52150f89ba783d93e69dc4080fab`，父 `fe6a8099`）；discovery sha256 `f730e08d14514d751c97ef14afa1e51d56496cb1b1726d78d6e08d1cf7eb7cae`；承接时 `assignment_epoch=1`，`rework_count=1`（dod_defect）
- **验证树**：`../wt-verify-TASK-006-a` 重建于 `7022d01d`，全部数字自采

### 5.1 fix_items 逐条核

| # | fix_item | 证据（自采） | 判定 |
|---|---|---|---|
| R1 | 新增 `TestHestiaContractEmitRevisionPeriod`：`emitFixture` 之外再 `Save` 一条同期、`PublishedAt` 更晚、不同 `ArticleID` 的观测；跑 emit `--stdout`；断言 `"is_revision": true`、`"supersedes_published_at": "2026-01-15"`、`"passed": true`；变异 M11/M12 必须由它转红 | `git show bb488d4a`：仅 `cmd/atlas/hestia_test.go` +33/0；夹具 `PublishedAt: "2026-02-20"`、`ArticleID: "2026022009294440746"`（报告带一条 `CheckPassed`，与 `emitFixture` 同理）；用 `restoreEmitGlobals` + `newCapturingCmd`；断言四条（`"published_at": "2026-02-20"` + fix_item 三条）；`-run TestHestiaContractEmit -v` 7 条 `--- PASS`；变异见 5.3 | PASS |

### 5.2 门禁复核（`7022d01d9229314f8a43c126d9b9763bbabefe5c`）

| 项 | 结果 | 门槛 |
|---|---|---|
| 测试 + 覆盖率 | 两包 ok；`internal/hestia` **96.6%**、`cmd/atlas` **76.6%** | ≥96.6 / ≥76.4 |
| gofmt / vet | 恰 `backtest_test.go`、`crisis_test.go`；vet 零输出 | 不变 |
| 改动范围 | `git diff --stat fe6a809 7022d01d` 恰 `cmd/atlas/hestia_test.go` +33 = `writes` 子集；`hestia.go` sha256 `7ad72a35…` 与首轮相同（实现未动，与 discovery `implementation_changed: false` 一致） | 无声明外文件 |
| 杂目录 / 注释前缀 | `find … pending/queue` 0；新增行无不带 `M2a 的` 的 `TASK-006` 引用 | 0 / 0 |

### 5.3 变异重跑（同一 harness，13 个变异体全部重跑；主工作区指纹前后一致 `3d67fc10f55e2084`；收尾被测树 0 行改动）

| # | 变异 | 首轮 | 复验 | 致红测试 |
|---|---|---|---|---|
| M1–M10, M13 | 同 §3 | KILLED | KILLED | 同 §3（M5 现另由 RevisionPeriod 同时转红） |
| M11 | `Supersedes: prior → ""` | SURVIVED | **KILLED** | `TestHestiaContractEmitRevisionPeriod`（独家） |
| M12 | `Report{Passed: false}` | SURVIVED | **KILLED** | `TestHestiaContractEmitRevisionPeriod`（独家） |

**13/13 KILLED**。首轮 §4 的非阻断残留已闭合。

### 5.4 结论

R1 按 fix_items 逐字落地、两条存活变异转红、实现未动、门禁不变 ⇒ **VERIFIED**。无新残留。
