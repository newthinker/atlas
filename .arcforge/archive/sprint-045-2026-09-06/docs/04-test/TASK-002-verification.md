# TASK-002 验证报告 · M2a 四信号与综合温度 `Evaluate`

- **验证者**：test-m2a-b
- **判定**：**VERIFIED**
- **判定对象**：`verify_baseline.head = 9e9140dd989adde515b73910af264aa6981c323e`（merge commit；dev 提交 `78ee806d532caa8817fe80324a74d903da92bd00`），discovery sha256 `73a4c2ca32dad0af696cd1b2fb3908ef69c55a3af8465c6bdd3aec8a4b43eb99`；承接时 `assignment_epoch=1`
- **验证树**：`../wt-verify-TASK-002-b`（detached 于 `9e9140dd…`），全部数字自采于此树，不取 discovery 的数
- **上游**：TASK-001（`70bbc4730b2bae3250562002a19dd1ee6c9a35c8`，已 verified）；开工锚 `d27791c695e8ebd0fd5d54c9161782f52d9b12cb`

## 1. 完成标准覆盖矩阵

| # | 完成标准（摘要） | 对应测试 / 证据（自采） | 判定 |
|---|---|---|---|
| functional[0] | `signals.go` 按原文：`Signal` 四常量、`Temperature`、`Evaluate`、六个非导出函数；只用 `Field*` 常量；六条测试全 PASS | 实现与需求原文代码块（L437-582）去注释 diff **仅 1 处**：`sameCaliberPair` 命名返回值改无名（discovery `decisions[2]` 已申报为 code-simplifier 改动）；`signals.go` 引号字面量只有 `q1/h1/q1_q3/annual/monthly` 与四个信号值，无业务字段名；`TestFieldNamesAppearOnlyInFieldsGo` PASS；六条测试（golden 3 子例）`--- PASS` | PASS |
| functional[1] | AST 守卫 want 插 `"Evaluate"`（`Discover` 后、`HealthSummary` 前）⇒ 27；三条守卫 PASS | `store_test.go:453` want 计数 **27**，序列片段 `"Discover", "Evaluate", "HealthSummary"`；`-run 'ExposesNoWrite\|TestFieldNamesAppearOnlyInFieldsGo' -v` 三条 `--- PASS` | PASS |
| boundary[0] | `monthsInPeriod` 四类非法输入 ⇒ 0；`monthlyAverage` `_ytd` 在场月数 0 ⇒ `ok=false`；`evalCredit` `total==0` ⇒ unknown（`_mom`/`_ytd` 各一）；空 `Values` ⇒ 四 unknown、0/0；**001 残留**：`RejectsBad` 加「剪刀差两线相等」「票据两线相等」，want 不变，`config_test.go` 只改这一处 | `TestMonthsInPeriod/boundary`（2022-13 / 2022-00 / 2022-7 / weekly）、`TestMonthlyAverageSources` 末段、`TestEvaluateCreditZeroTotalIsUnknown`（2 子例）、`TestEvaluateEmptyValuesAllUnknown`、`RejectsBad/剪刀差两线相等`、`RejectsBad/票据两线相等` 全 PASS；`config_test.go` diff 恰 +4 行（2 注释 + 2 子例）；变异 M6/M7/M8/M9 KILLED 证明四类边界断言真在守卫；001 的 M3/M8 现由两新子例钉住（001 实现未改，直接绿） | PASS |
| boundary[1]（review） | 三期 golden 与 `v_hestia_current` 逐值相等 | 我用 `sqlite3 -readonly data/hestia.db` 查三行（列名取 `fields.go` 常量值）：`2020-06/h1` 6.5/11.1/28000/7552/9697/87700；`2025-12/annual` 3.8/8.5/12800/-8351/16600/154700；`2026-06/h1` 4.0/8.0/2212/-5881/8143/111300——与 `signals_test.go:42-54` 夹具逐值相等；四个 `_mom` 列三行全空；每组 `(period, period_type)` 在视图里恰 1 行。与 discovery `golden_check` 一致，`fixture_changed=false` 属实 | PASS |
| error_handling[0] | 红阶段留痕含 `undefined: Evaluate` | 在验证树复现两段：删 `signals.go` ⇒ `signals_test.go:39:8: undefined: Temperature`…（编译器截断，未到 Evaluate）；只留 types ⇒ `:59:28: undefined: Evaluate`、`:67:11`/`:72:10`/`:77:11: undefined: monthlyAverage`。两段与 discovery `red_phase.capture_1/2` 逐行相同；dev 分两段捕获而非伪造一份含 Evaluate 的输出，处置正确 | PASS |
| non_functional[0] | 门禁 | 见 §2 | PASS |
| non_functional[1] | 交付流程（AD-9） | 提交 `78ee806d` 锚 `feat(TASK-002): M2a …`；merge `9e9140dd`；discovery 记双 sha 与 `sampled_on`；helper 改名 `obsAt` 在 `key_findings[1]`/`decisions[0]`/`interfaces_exposed[4]` 三处申报（`store_test.go:731` 确有 `obsWith(values)` 单参既有 helper，重名属实）；`git worktree list` 无 `wt-TASK-002-m2a`/`pre-TASK-002` 残留 | PASS（review） |

## 2. 门禁实测（验证树 `9e9140dd989adde515b73910af264aa6981c323e`）

| 项 | 结果 | 门槛 |
|---|---|---|
| `GOTOOLCHAIN=local go test ./internal/hestia/... ./cmd/atlas/... -count=1` | rc=0，两包 ok | 全绿 |
| 覆盖率 | `internal/hestia` **96.6%**；`cmd/atlas` **76.4%** | ≥96.6 / ≥76.4 |
| `go vet ./internal/hestia/... ./cmd/...` | 零输出，rc=0 | 零输出 |
| `gofmt -l internal/hestia cmd/atlas` | 恰 `backtest_test.go`、`crisis_test.go` | 只允许这两处 |
| 不动文件 + go.mod/go.sum（`d27791c..9e9140dd`） | 0 行 | 空 |
| `Save` 函数体 grep | 0 | 0 |
| `hestia.go` import 块 `filepath` | 0 | 不 import |
| 越界申报 `git diff --stat 70bbc47 9e9140dd` | 恰 4 文件 = `writes`（`config_test.go` 4/0、`signals.go` 153/0、`signals_test.go` 168/0、`store_test.go` 1/1，与 discovery numstat 逐项相同） | 无声明外文件 |
| 注释前缀 | 新增行里不带 `M2a 的` 的 `TASK-002` 引用 0 条 | 0 |
| `find … pending/queue` | 命中 0 | 空 |
| 目标测试 `-v` | `TestEvaluate*|TestMonthlyAverage|TestMonthsInPeriod|RejectsBad` 共 **21** 条 `--- PASS`（含子例），与 discovery 一致 | — |

## 3. 变异测试（验证树内变异 + `git checkout` 还原；每个先打 diff；主仓库四文件 sha256 前后一致）

| # | 变异（`signals.go`） | 结果 | 致红测试 |
|---|---|---|---|
| M1 | `d >= ScissorsActive` → `>` | **SURVIVED** | — |
| M2 | `d <= ScissorsSink` → `<` | **SURVIVED** | — |
| M3 | `v >= warm` → `>` | **SURVIVED** | — |
| M4 | `r < BillRatioHealthy` → `<=` | **SURVIVED** | — |
| M5 | `r >= BillRatioSevere` → `>` | **SURVIVED** | — |
| M6 | 删 `total == 0` 闸 | KILLED | `CreditZeroTotalIsUnknown`（2 子例） |
| M7 | 删 `n == 0 ⇒ ok=false` | KILLED | `MonthlyAverageSources` |
| M8 | 月份范围 `1..12` → `0..13` | KILLED | `MonthsInPeriod/boundary` |
| M9 | 删 `len(period) != 7` | KILLED | `MonthsInPeriod/boundary` |
| M10 | `_ytd` 优先于 `_mom` | KILLED | golden ×3、`MonthlyAverageSources`、`UnknownShrinks` |
| M11 | 黄灯也计分 | KILLED | golden ×2、`ScissorsYellowScoresZero` |
| M12 | unknown 计入 `Known` | KILLED | `UnknownShrinks`、`ScissorsYellow`、`EmptyValues` |
| M13 | `sameCaliberPair` 允许 (`_mom`, `_ytd`) 混配 | **SURVIVED** | — |
| M14 | 楼市信号读住户短期字段 | KILLED | golden/2020H1、`UnknownShrinks` |
| M15 | `annual` ⇒ 11 | KILLED | `MonthlyAverageSources` |

**9/15 KILLED**。存活的 6 个分两类，都是**测试缺边界用例**、实现本身正确（实现与需求原文逐字相同，阈值语义按方案报告 4.7）：

- **M1–M5：五个阈值的相等边**（`d == active` / `d == sink` / `v == warm` / `r == healthy` / `r == severe`）无用例。与 001 的 M3/M8 同类。其中 `HHShortMonthlyWarm = 0` 与 `ScissorsSink = -2` 在真实数据上可精确命中（yoy 一位小数，`3.0 − 5.0 = −2.0`），边的方向有业务意义。
- **M13：混口径只测了一个方向**。`TestEvaluateCreditRefusesMixedCaliber` 的混配夹具是 (`bill_ytd`, `corp_total_mom`)；(`bill_mom`, `corp_total_ytd`) 这一方向无用例，`sameCaliberPair` 的 `_mom` 分支若错配到 `bYTD` 不会变红。

DoD boundary[0] 未要求以上两类，dev 按 DoD 与需求原文照做，**不构成 `task_defect`、不退回**。建议 Leader 裁落点：TASK-005 会接 `Evaluate` 且 `writes` 含 `signals_test.go`？（若不含则放 TASK-007 终检或单独 review_fix）——补 6 条子例：五个相等边各一（期望分别 green / red / green / yellow / red）+ 混配反向一条（期望 unknown）。

## 4. 结论

- **VERIFIED**：全部 `verify_by: test` 条目有对应测试且断言真在守卫（M6–M12/M14/M15 KILLED）；golden 由我独立只读复核逐值相等；门禁逐项达标；改动文件集与 `writes` 逐一相同；红阶段两段可复现；discovery 自证数字与我自采一致；`obsAt` 改名与 code-simplifier 改动均已申报且与 diff 相符。
- 非阻断残留：§3 的 M1–M5 与 M13，待 Leader 裁落点。
