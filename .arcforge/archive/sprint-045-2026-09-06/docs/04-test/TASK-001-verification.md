# TASK-001 验证报告 · M2a 配置 `queue` 与 `signals` 段

- **验证者**：test-m2a-b（备用验证者，2026-09-05T13:44:56Z 经逃生边 `verifying→verifying` 改派；前任 test-m2a-a 无产物）
- **判定**：**VERIFIED**
- **判定对象**：`verify_baseline.head = 70bbc4730b2bae3250562002a19dd1ee6c9a35c8`（merge commit；dev 提交 `913f0c04b02634621174702a068272b4ce653ba1`），discovery sha256 `9747b7b46e40bd109e27eb9bfc98bce43b989f64f40046cd821cabb8c2fe8213`；承接时 `assignment_epoch=1`
- **验证树**：`../wt-verify-TASK-001-b`（`git worktree add --detach … 70bbc4730b2bae3250562002a19dd1ee6c9a35c8`），全部数字自采于此树，不取 discovery 的数
- **开工锚**：`d27791c695e8ebd0fd5d54c9161782f52d9b12cb`

## 1. 完成标准覆盖矩阵

| # | 完成标准（摘要） | 对应测试 / 证据（自采） | 判定 |
|---|---|---|---|
| functional[0] | `QueueCfg{Dir}`、`defaultQueueDir`、`Signals` 8 字段（mapstructure+同名 json tag，`TempScale` `json:"-"`）、`DefaultSignals()` 原值、`Config` 在 `Thresholds` 之后加两字段、`LoadConfig` 预填、`validate()` 四条、三条测试 + `minimalYAML`；AST 守卫 want 插入 `DefaultSignals` ⇒ 26 项 | `git diff d27791c..70bbc47 -- config.go` 逐项核对（字段/tag/预填/switch 四条/错误串 4 处原样）；`go test -v -run 'TestLoadConfig.*(Queue\|Signals)'`：`Defaults`/`Reads`/`RejectsBad`（4 子例）全 PASS；`-run ExposesNoWrite` 两条 PASS；`store_test.go:453` want 计数 **26**，`DefaultSignals` 位于 `Calibrate` 后、`DefaultThresholds` 前 | PASS |
| functional[1] | yaml `storage` 后加 `queue` 段、末尾加 `signals` 段、`config_version` → `"2026-09-05"`、变更记录一行；`config_test.go:387` 改 `"2026-09-05"`；追加 `Queue.Dir`/`Signals` 两条断言 | diff 核对 yaml 三处与 `config_test.go:388`/`:439-445`；`TestShippedConfigLoadsAndIsCalibrated` PASS；变异 M6（yaml `scissors_sink` -2→-3）与 M9（version 回退）均 KILLED，两条断言真在守卫 | PASS |
| functional[2] | `hestia_test.go` `hestiaCfg` 闭包与 `wiringHestiaCfg` 两处 yaml 加 `queue.dir`（tmp）+ 理由注释；`cmd/atlas` 全绿；`find … pending/queue` 为空 | diff 核对 `:1227-1234`、`:1349-1356`（各带 AD-5 理由注释）；`go test ./cmd/atlas/... -count=1` ok；`find internal/hestia cmd/atlas -type d \( -name pending -o -name queue \)` 命中 0 条 | PASS |
| boundary[0] | `TestSignalsJSONKeys` 七键 `ElementsMatch`，无 `temp_scale`、无 Go 字段名 | 测试 PASS；变异 M1（`TempScale` json tag 改回 `temp_scale`）KILLED，断言真在守卫 | PASS |
| error_handling[0] | 红阶段留痕：编译错误含 `cfg.Queue undefined` 记进 discovery | 在验证树上 `git checkout d27791c -- config.go` 复现：`config_test.go:442:38: cfg.Queue undefined`、`:444:18: undefined: DefaultSignals`、`:444:40 cfg.Signals undefined`、`:604:38`，与 discovery `verification.red_phase` 逐行相同；已还原（dirty=0） | PASS |
| non_functional[0] | 门禁（gofmt/vet/test/覆盖率/无新依赖/不动文件/`Save`/`filepath`/注释前缀） | 见 §2 | PASS |
| non_functional[1] | 交付流程（AD-9） | `verify_by` 性质为流程审查：提交 `913f0c0` 锚 `feat(TASK-001): M2a …`（符合 AD-2）；merge 进 master 后 `70bbc47`；discovery 同时记 `commit_sha` 与 `merged_master_sha`，`sampled_on` 带全 sha；`git worktree list` 中无 `wt-TASK-001-m2a`/`pre-TASK-001` 残留 | PASS（review） |

## 2. 门禁实测（验证树 `70bbc4730b2bae3250562002a19dd1ee6c9a35c8`）

| 项 | 命令 | 结果 | 门槛 |
|---|---|---|---|
| 测试 | `GOTOOLCHAIN=local go test ./internal/hestia/... ./cmd/atlas/... -count=1` | rc=0，两包 ok | 全绿 |
| 覆盖率 | 同上 `-cover` | `internal/hestia` **96.6%**；`cmd/atlas` **76.4%** | ≥96.6 / ≥76.4 |
| vet | `go vet ./internal/hestia/... ./cmd/...` | 零输出，rc=0 | 零输出 |
| gofmt | `gofmt -l internal/hestia cmd/atlas` | 恰 `cmd/atlas/backtest_test.go`、`cmd/atlas/crisis_test.go` | 只允许这两处 |
| 不动文件 | `git diff --stat d27791c 70bbc47 -- parse.go extract.go validate.go fields.go go.mod go.sum` | 0 行 | 空 |
| `Save` 函数体 | DoD 给的 grep 链 | 0 | 0 |
| `hestia.go` 不 import `path/filepath` | import 块 grep | 无（全文 2 处命中均为注释 `:150`/`:260`）；守卫 `TestHestiaCmdDoesNotResolveDBPath` PASS | 不 import |
| 越界申报 | `git diff --stat d27791c 70bbc47` | 恰 5 文件 = `writes` 声明（`hestia_test.go` 9/0、`hestia.yaml` 21/1、`config.go` 50/0、`config_test.go` 89/1、`store_test.go` 1/1，与 discovery numstat 逐项相同） | 无声明外文件 |
| 注释前缀 | diff 新增行里 `TASK-001` 不带 `M2a 的` 的引用 | 0 条 | 0 |

## 3. 变异测试（隔离树内就地变异 + `git checkout` 还原；每个变异体先打 diff 核对；主仓库四文件 sha256 前后一致）

| # | 变异 | 结果 | 致红测试 |
|---|---|---|---|
| M1 | `TempScale` `json:"-"` → `json:"temp_scale"` | KILLED | `TestSignalsJSONKeys` |
| M2 | 删 `case c.Queue.Dir == ""` 分支 | KILLED | `RejectsBad/queue.dir 空串` |
| M3 | `ScissorsSink >= ScissorsActive` → `>` | **SURVIVED** | — |
| M4 | `defaultQueueDir` → `"queue/hestia2"` | KILLED | `TestLoadConfigDefaultsQueueAndSignals` |
| M5 | want 里去掉 `"DefaultSignals"` | KILLED | `TestPackageExposesNoWriteFunctions` |
| M6 | yaml `scissors_sink: -2` → `-3` | KILLED | `TestShippedConfigLoadsAndIsCalibrated` |
| M7 | 删 `LoadConfig` 的 `Signals: DefaultSignals()` 预填 | KILLED | `Defaults`/`Reads`/`RejectsBad`（3 条） |
| M8 | `BillRatioHealthy >= BillRatioSevere` → `>` | **SURVIVED** | — |
| M9 | yaml `config_version` 回退 `"2026-09-03"` | KILLED | `TestShippedConfigLoadsAndIsCalibrated` |

**7/9 KILLED**。M3/M8 存活的原因：需求原文给的两个倒置子例（`scissors_active: -3` vs 预填 `-2`；`bill_ratio_severe: 5` vs 预填 `10`）都是**严格**倒置，「两线相等」这一边界没有用例。**实现本身正确**——我用临时测试（已删）直接验证 `scissors_active: -2`（== sink）与 `bill_ratio_severe: 10`（== healthy）都被拒，错误串分别为 `signals.scissors_sink (-2) must be < scissors_active (-2)`、`signals.bill_ratio_healthy (10) must be < bill_ratio_severe (10)`。

## 4. 结论与残留

- **VERIFIED**：全部 `verify_by: test` 条目有对应测试且断言真在守卫（变异 KILLED 证明）；门禁逐项达标；改动文件集与 `writes` 逐一相同；红阶段留痕可复现；discovery 自证数字与我自采一致。
- **非阻断残留（建议 Leader 裁决落点，不退回本任务）**：`validate()` 两条倒置校验的**相等边界**无测试（M3/M8 存活）。DoD 只要求「4 子例按原文追加」，dev 已按原文照做，故不构成 `task_defect`；建议在 TASK-002（同包、会改 `config_test.go` 相邻区域）或 TASK-007 终检时给 `TestLoadConfigRejectsBadQueueAndSignals` 补两条子例 `"剪刀差两线相等"`（`scissors_active: -2`）与 `"票据两线相等"`（`bill_ratio_severe: 10`），期望 `want` 不变。
- discovery `key_findings[3]`：dev 自述 `find pending/queue` 判据在本任务恒真、真正生效点在 TASK-005 之后——属实，与 AD-5 一致，TASK-005 验证时须再采一次。
- GitNexus `detect_changes` 在 dev 侧未能执行（索引陈旧 + DB 版本不匹配）；本任务只新增符号与一个 struct 字段，我以 `git diff` 与全量测试替代，不额外阻断。
