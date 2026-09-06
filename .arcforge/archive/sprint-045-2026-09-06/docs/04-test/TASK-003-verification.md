# TASK-003 验证报告 · M2a 契约结构 `BuildContract` + `Store.Current` / `PriorPublishedAt`

- **验证者**：test-m2a-b
- **判定**：**VERIFIED**
- **判定对象**：`verify_baseline.head = 61403387b4e7a8ed33dd0b136fe0f7a87af4c346`（merge commit；dev 提交 `789989bbfeb0d61d29a269727efb5c7a8a78c606`），discovery sha256 `999c2cf8356a9576f5e32a496a270d3b4fd3962e08367a8edc56fcb413b1fc44`；承接时 `assignment_epoch=1`
- **验证树**：`../wt-verify-TASK-003-b`（detached 于 `61403387…`），全部数字自采于此树，不取 discovery 的数
- **上游**：TASK-001 / TASK-002（均 verified）；开工锚 `d27791c695e8ebd0fd5d54c9161782f52d9b12cb`

## 1. 完成标准覆盖矩阵

| # | 完成标准（摘要） | 对应测试 / 证据（自采） | 判定 |
|---|---|---|---|
| functional[0] | `contract.go` 按原文（17 顶层字段、`ContractValidation`/`ContractCheck`（`Reason` omitempty）/`ContractThresholds`、`orderedValues`+`MarshalJSON`、`ContractInput`、三常量、`pbocArticleURL` 改名、`BuildContract`、`JSON()`、`FileName()`）；五条测试 + 四 helper PASS；不 import `context`；无字段名字面量 | 实现与需求原文代码块（L838-993，`articleURL→pbocArticleURL`）去注释/空行 diff **仅 1 处**：单 import 改裸形式（discovery `decisions[2]` 申报为 code-simplifier 保留项）；`ingest_test.go:82` 确有既有 `articleURL(articleID string)`，重名属实；五条测试 `--- PASS`；`contract_test.go` import 块为 `encoding/json`/`strings`/`testing`/testify，无 `context`；`contract.go` 引号字面量全部是 JSON tag 名与 units 值，无业务字段名；`TestFieldNamesAppearOnlyInFieldsGo` PASS | PASS |
| functional[1] | `store.go` 在 `PrecedingAll` 后只新增 `Current`/`PriorPublishedAt`；删除行 0；两测试按原文 PASS | 追加段与需求原文（L1000-1032）去注释 diff **0 差异**；`git diff d27791c 61403387 -- store.go \| grep -c '^-[^-]'` = **0**；`TestStoreCurrent`/`TestStorePriorPublishedAt` PASS。夹具改 `passing()` 的申报属实：`store.go:937` 有「report claims Passed with zero checks」守卫，`passing()` 在 `store_test.go:725`，需求指向的 `TestSaveRevisionKeepsBothRows` 用的正是它 | PASS |
| functional[2] | reflect 14 / AST 32，字母序插入 | `store_test.go:400` want 计数 **14**（`"Close", "Current"`、`"PrecedingAll", "PriorPublishedAt"`）；`:453` want 计数 **32**（`"BackfillLoad", "BuildContract"`、`"Calibrate", "Contract.FileName", "Contract.JSON"`、`"Store.Close", "Store.Current"`、`"Store.PrecedingAll", "Store.PriorPublishedAt"`），`sort` 比对字母序 OK；两守卫 PASS | PASS |
| boundary[0] | `TestContractJSONTopLevelKeyOrder` 扫两空格 `"<key>":` 行，17 键恰序；`data` 序沿 `orderedSubset` | 测试 PASS；变异 M1（交换 `Period`/`PeriodType` 字段序）KILLED；M21（`data` 按 `fieldOrder` 逆序）由 `MapsFields` 的 `orderedSubset` KILLED | PASS |
| boundary[1] | `PriorPublishedAt`/`Current` 不跨 `period_type` | `TestStorePriorPublishedAt/period_type 不串` PASS（同 `2026-12` 先 annual 后 monthly，prior 为 `""`，`Current` 各取各行）；变异 M10/M12（两个查询各删 `period_type` 条件）均 KILLED | PASS |
| error_handling[0] | 红阶段含 `undefined: BuildContract`；关库后两前缀 | 验证树复现（移走 `contract.go`、`store.go` 退回 `9e9140dd`）：`contract_test.go:50:7: undefined: BuildContract`、`:50:21: undefined: ContractInput`、`:109:7`、`:109:21`。与 discovery 的 `:51`/`:110` 差恰 1 行——成因是 code-simplifier 在红阶段捕获之后把 `contractCfg` 的 `cfg :=` 中转变量去掉（需求原文 4 行 → 现行 3 行），TDD 顺序（红 → 实现 → 提交前简化）下一致，非伪造；`TestStoreCurrentAndPriorErrorsCarryPrefix` PASS，变异 M11/M19（改前缀）均 KILLED | PASS |
| non_functional[0] | 门禁 | 见 §2 | PASS |
| non_functional[1] | 交付流程（AD-9） | 提交 `789989bb` 锚 `feat(TASK-003): M2a …`；merge `61403387`；discovery 记双 sha；code-simplifier 四处改动的申报与 diff 相符（三处保留：裸 import、`contractCfg` 去中转、`TopLevelKeyOrder` 去恒假条件；一处回退：`Current` 的 `rows.Err()` 单行返回）。**回退理由实测**：我在验证树上把那一行展开成 dev 描述的 if 分支，背对背 `-cover` 得 **96.5%**，原样 **96.6%**——理由属实；`git worktree list` 无 `wt-TASK-003-m2a`/`pre-TASK-003` 残留 | PASS（review） |

## 2. 门禁实测（验证树 `61403387b4e7a8ed33dd0b136fe0f7a87af4c346`）

| 项 | 结果 | 门槛 |
|---|---|---|
| `GOTOOLCHAIN=local go test ./internal/hestia/... ./cmd/atlas/... -count=1` | rc=0，两包 ok | 全绿 |
| 覆盖率 | `internal/hestia` **96.6%**；`cmd/atlas` **76.4%** | ≥96.6 / ≥76.4 |
| `go vet ./internal/hestia/... ./cmd/...` | 零输出，rc=0 | 零输出 |
| `gofmt -l internal/hestia cmd/atlas` | 恰 `backtest_test.go`、`crisis_test.go` | 只允许这两处 |
| 不动文件 + go.mod/go.sum（`d27791c..61403387`） | 0 行 | 空 |
| `Save` 函数体 grep | 0 | 0 |
| `store.go` 删除行（`d27791c..61403387`） | 0 | 0 |
| `hestia.go` import 块 `filepath` | 0 | 不 import |
| 越界申报 `git diff --stat 9e9140dd 61403387` | 恰 4 文件 = `writes`（`contract.go` 161/0、`contract_test.go` 177/0、`store.go` 34/0、`store_test.go` 99/2，与 discovery numstat 逐项相同） | 无声明外文件 |
| 注释前缀 | 新增行里不带 `M2a 的` 的 `TASK-003` 引用 0 条 | 0 |
| `find … pending/queue` | 命中 0 | 空 |
| 目标测试 `-v` | 四组共 **10** 条 `--- PASS`（含子例），加三条守卫与 `TopLevelKeyOrder`/`ErrorsCarryPrefix` 共 13 条 PASS，与 discovery 一致 | — |

## 3. 变异测试（验证树内变异 + `git checkout` 还原；每个先打 diff；主仓库四文件 sha256 前后一致）

| # | 变异 | 结果 | 致红测试 |
|---|---|---|---|
| M1 | 交换 `Period`/`PeriodType` 结构体字段序 | KILLED | `TopLevelKeyOrder` |
| M2 | `FileName` 去 `period_type` | KILLED | `FileName` |
| M3 | `JSON()` 去末尾 `\n` | **SURVIVED** | — |
| M4 | 回放不加 `/replay` | KILLED | `Replay` |
| M5 | `Checks` 初值 nil | KILLED | `Replay`（`"checks": []`） |
| M6 | `AbsentFields` 初值 nil | **SURVIVED** | — |
| M7 | 忽略显式 `SourceURL` | KILLED | `RevisionAndExplicitURL` |
| M8 | `Supersedes` 恒 nil | KILLED | `RevisionAndExplicitURL` |
| M9 | `PriorPublishedAt` `<` → `<=` | KILLED | `PriorPublishedAt`（含子例） |
| M10 | `PriorPublishedAt` 删 `period_type` 条件 | KILLED | `period_type 不串` |
| M11 | `Current` 错误前缀改字 | KILLED | `ErrorsCarryPrefix` |
| M12 | `Current` 删 `period_type` 条件 | KILLED | `period_type 不串` |
| M13 | `pbocArticleBase` 改一位 | KILLED | `MapsFields` |
| M14 | units `百分数` → `百分比` | KILLED | `MapsFields` |
| M15 | `orderedValues.MarshalJSON` 去 `:` | KILLED | `Replay`、`Deterministic`、`TopLevelKeyOrder` |
| M16 | `thresholds.temp_scale` 置空 | KILLED | `MapsFields` |
| M17 | `schema_version` `1.0` → `1.1` | KILLED | `MapsFields` |
| M18 | check `Value` 恒 nil | KILLED | `MapsFields` |
| M19 | `PriorPublishedAt` 错误前缀改字 | KILLED | `ErrorsCarryPrefix` |
| M20 | `Current` 空结果返回 `ok=true` | KILLED | `StoreCurrent` |
| M21 | `data`/`absent_fields` 按 `fieldOrder` 逆序 | KILLED | `MapsFields` |
| M22 | `orderedValues.MarshalJSON` 去 `,` | KILLED | `Replay`、`Deterministic`、`TopLevelKeyOrder` |

**20/22 KILLED**（M15 第一轮因我的 harness 缩进写错未真正变异、被误打成 SURVIVED，已让 harness 在变异失败时早退并重做，重做结果 KILLED；此处记的是重做后的结果）。存活两个都是**测试缺用例、实现与需求原文逐字相同**：

- **M3**：DoD functional[0] 明写 `JSON()`「末尾 `\n`」，但五条原文测试没有断言它（`Deterministic` 只比两次相等）。`done/` 里的文件与回放 cmp 会依赖这个字节。
- **M6**：`absent_fields` 只在 76 个字段**全部在场**时才是空数组，此时 nil 会输出 `null` 而非 `[]`。`Replay` 的 `"absent_fields": [` 断言用的夹具缺 71 个字段，覆盖不到这一格。可达性低（真实月报不会 76 字段全齐），但契约 schema 说的是数组。

两者不构成 `task_defect`、不退回。建议 Leader 裁落点：TASK-004 `WriteContract` 写的就是 `JSON()` 的字节，`queue_test.go` 补「文件末字节是 `\n`」一条最顺；M6 可在同处加一条「Values 覆盖全部 `fieldOrder` 时 `absent_fields` 为 `[]`」。

## 4. 结论

- **VERIFIED**：全部 `verify_by: test` 条目有对应测试且断言真在守卫（20 个变异体 KILLED）；实现与需求原文逐字一致（唯一差异已申报）；门禁逐项达标；改动文件集与 `writes` 逐一相同；红阶段可复现（行号偏移有确定成因）；dev 的三条申报（`passing()` 夹具、`articleURL` 重名、覆盖率回退理由）全部经我独立核实属实。
- 非阻断残留：§3 的 M3 / M6，待 Leader 裁落点。
