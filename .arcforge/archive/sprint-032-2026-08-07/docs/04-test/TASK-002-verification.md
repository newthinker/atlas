# TASK-002 验证报告 —— Classify 纯函数（四种 Verdict）

- **验证者**：test-agent-19（Reality Checker，默认判定 NEEDS WORK）
- **被验对象**：commit `96641ec` @ 分支 `feat/macro-bitemporal`
- **承接时 assignment_epoch**：1
- **隔离方式**：`git worktree add --detach ../wt-verify-TASK-002 96641ec`，全部变异在 worktree 内注入并逐条还原；收尾 `git status` 空、`git diff --stat` 空，主工作区零污染，worktree 已从主仓库拆除
- **判定**：**VERIFIED（通过）**
- **DoD 依据**：`boundary[1]` 的字面变异判据不可满足（靶不存在），按 Leader 的 inbox 更正裁定处理，见第 6 节

> **修订记录**：初版出具后收到 Leader 的 inbox 更正（确认坏判据在 `boundary[1]`、`boundary[0]` 是好的），
> 与本报告初版第 6 节的独立结论一致，判定不变。本版新增**第 5 节**：对 dev「只改组织方式」
> 声明的机械核验（Leader 点名要求，初版为肉眼比对，证据强度不足）。

---

## 1. 基线实测

| 项 | 命令 | 结果 |
|---|---|---|
| 本任务测试 | `go test ./internal/macro/bitemporal/ -count=1 -v -run 'TestClassify\|TestVerdict'` | PASS，5 个测试函数 / **21 条 `--- PASS`** |
| SKIP 数 | `go test ... -v \| grep -c -- '--- SKIP'` | **0** |
| 全包回归 | `go test ./internal/macro/bitemporal/ -count=1` | `ok`（含 TASK-001 的 spec.go/spec_test.go） |
| `go vet` | `go vet ./internal/macro/...` | exit 0 |
| 覆盖率 | `go tool cover -func` | `String 100.0%` / `Classify 100.0%` / **package total 100.0%**（门槛 80） |

**真隔离编译取证（non_functional[1]）**：把 `classify.go` + `classify_test.go` + `go.mod`/`go.sum`
单独拷进空目录（**不含 spec.go**）后 `go test` → `ok`。这比 grep 更强：证明本任务在编译期
就不依赖 TASK-001。另 grep `\b(Spec|NewSpec|Key)\b` 对两个文件**零命中**。

## 2. 与需求文档的一致性（实现侧）

需求文档 `2026-08-07-macro-bitemporal.md` L378-432 的 `Classify` 实现与 `classify.go:45-59`
**逐字一致**：`if !s.Exists → New`、`==` → Duplicate、`>` → Revision、`default` → OutOfOrder。
**实现中不存在 `<`**（`grep -o '<' classify.go | wc -l` = **0**）。

## 3. done_criteria 覆盖矩阵（7 条）

| # | 完成标准 | 对应测试 | 变异取证 | 判定 |
|---|---|---|---|---|
| functional[0] | 四种 Verdict 各自触发，**每种失败可单独定位** | `TestClassify`（表驱动 + `t.Run`，4 子测试） | **M-C**、**M-D** | **PASS** |
| functional[1] | `String()` 可读 + **未定义值兜底分支** | `TestVerdictString`（含 `Verdict(9)`） | **M-G**、**M-H**、**M-I** | **PASS** |
| boundary[0] | 1/3 个版本判定随之变化；Duplicate 与 OutOfOrder **各有独立断言**（判据：互换二者期望值须各红一条） | `TestClassifyAcrossVersions`（5 子测试） | **M-A** | **PASS** |
| boundary[1] | 相邻边界（相等 vs 更小）**各自独立取证** | `TestClassifyAdjacentBoundary`（3 子测试） | **M-E**、**M-F** | **PASS（实质）**；字面变异判据靶不存在，见第 6 节 |
| error_handling[0] | `Exists=false` 判定与 `LatestRevision` 取值无关 | `TestClassifyAbsentIgnoresRevision`（4 子测试） | **M-B** | **PASS** |
| non_functional[0] | 纯函数零 I/O、`go test` 绿、0 SKIP、测试头 Context Checkpoint、注释中文 | 全项 | 见第 1 节 | **PASS** |
| non_functional[1] | 不依赖 Task 1（不引用 Spec/Key） | 真隔离编译 + grep | 见第 1 节 | **PASS** |

## 4. 变异取证明细（全部由 test-agent-19 独立注入并实跑，**非采信 dev 自述**）

| ID | 判据 | 变异 | 结果 |
|---|---|---|---|
| **M-A** | boundary[0] | `TestClassifyAcrossVersions` 互换 Duplicate/OutOfOrder 期望值 | **KILLED** —— 恰好 2 红且各一条，报出子测试名：`三个版本——比最大者旧的是乱序`、`三个版本——与最大者相等的是重复` |
| **M-B** | error_handling[0] | 删 `if !s.Exists { return New }`，使 Exists=false 时也去比较 revision | **KILLED** —— `TestClassifyAbsentIgnoresRevision` **4 个子测试全红**（另连带红 `TestClassify/业务键首次出现`、`TestClassifyAcrossVersions/零个版本`）。**红因已核实**：`LatestRevision=9999-12-31` 一条报 `expected: 0`(New) / `actual: 3`(OutOfOrder)，正是「调用方填残值 → 新增被误判」的目标风险，不是别的原因蹭红 |
| **M-C** | functional[0] | `TestClassify` 互换 Revision/OutOfOrder 期望值 | **KILLED** —— 恰好 2 红并报名 |
| **M-D** | functional[0] | `TestClassify` 互换 New/Duplicate 期望值（补齐另两种 Verdict） | **KILLED** —— 恰好 2 红并报名。M-C+M-D 合起来证明**四种 Verdict 全部可单独定位** |
| **M-E** | boundary[1] 实质（乱序侧，= dev 的 M4a） | `default: return OutOfOrder` → `return Duplicate` | **KILLED** —— `TestClassifyAdjacentBoundary` 内**恰好只红 1 条**（`小一天——乱序`） |
| **M-F** | boundary[1] 实质（重复侧，= dev 的 M4b） | `case incoming == …: return Duplicate` → `return OutOfOrder` | **KILLED** —— `TestClassifyAdjacentBoundary` 内**恰好只红 1 条**（`相等——重复`）。与 M-E 红的是**不同子测试** ⇒ 两个相邻判定不共用断言 |
| **M-G** | functional[1] | `fmt.Sprintf("Verdict(%d)", …)` → `"%d"` | **KILLED** —— `TestVerdictString` 红 |
| **M-H** | functional[1] | 兜底分支返回 `"New"` 掩盖（同时删随之失效的 `fmt` import） | **KILLED** —— 报 `expected: "Verdict(9)" / actual: "New"` |
| **M-I** | functional[1] | 互换 `String()` 中 Duplicate/Revision 返回串 | **KILLED** —— `TestVerdictString` 红 |
| **M-lit-1** | boundary[1] **字面判据** | 对 `classify.go` 执行 `s/</<=/g` | **靶不存在** —— 文件零字节变化，`<` 出现次数 = 0 |
| **M-lit-2** | 字面判据的「或反之」分支 | `case incoming > s.LatestRevision` → `>=` | **等价变异（存活）** —— 变异确已写入（diff 留证），但 **0 条转红**：`==` 分支在前，`>=` 永远见不到相等值 |

M-lit-1 / M-lit-2 独立复现了 dev 对两个字面分支的实测结论，**结论一致**。

**过程纠错自述**：M-H 首次注入时我只改了返回值、未删随之失效的 `import "fmt"`，
程序编译失败、测试根本没跑，我最初读到的「0 红」是**无效结果**。已按「拿到红/绿先问它是不是
我以为的那个原因」重做（先 `go vet` 确认可编译再跑），重做后 KILLED。

## 5. 「只改组织方式」声明的机械核验（Leader 点名要求）

dev 把文档版 `TestClassifyAcrossVersions`（同一函数体内 5 条顺序 assert）改写为表驱动 + `t.Run`，
声称「断言的状态与期望值与文档逐条一致，只改了组织方式」。该声明**易说难验**，故用脚本从两侧
分别抽取 `(state, incoming, want)` 三元组做集合比对，而非肉眼：

| # | state | incoming | want | 文档 vs 实现 |
|---|---|---|---|---|
| 1 | `State{}` | `2020-07-10` | `New` | ✓ |
| 2 | `Exists: true, LatestRevision: "2020-07-10"` | `2021-01-12` | `Revision` | ✓ |
| 3 | `Exists: true, LatestRevision: "2026-01-15"` | `2021-01-12` | `OutOfOrder` | ✓ |
| 4 | `Exists: true, LatestRevision: "2026-01-15"` | `2026-01-15` | `Duplicate` | ✓ |
| 5 | `Exists: true, LatestRevision: "2026-01-15"` | `2026-07-15` | `Revision` | ✓ |

**文档 5 条断言 ↔ 实现 5 个 case，三元组逐条一致，无增删、无弱化 ⇒ 声明属实。**

同时核了文档给出的另两个测试函数是否被原样保留：

- `TestClassify`：与文档**逐字一致**（忽略空白）。
- `TestVerdictString`：唯一差异是**新增一行中文注释**（`// 未定义值走兜底分支…`）；
  剥掉注释行后**逐字一致**，断言条数文档 5 / 实现 5。**非实质改动。**
- 实现**新增**（文档无）：`TestClassifyAdjacentBoundary`、`TestClassifyAbsentIgnoresRevision`
  —— 均为**增强**（分别服务 boundary[1] 与 error_handling[0]），无删除。

> 排除的一个假阳性：脚本初次比对全文档函数名时报出 19 个「文档有、实现无」的测试，
> 逐个回查其所属 `### Task` 段后确认**全部属于 Task 1/3/4/5**（Spec / fixture / Query / Lookup），
> 不在 TASK-002 范围内，不是删除。

## 6. DoD 文本缺陷 —— `boundary[1]`（Leader 已裁定）

`boundary[1]` 的字面变异判据（「把 `<` 改成 `<=`（或反之），必须恰好只红其中一条」）
**在自己的条款下不成立**，我独立复现了两点（M-lit-1 / M-lit-2），且实现与需求文档逐字一致
⇒ **错的是判据文本，不是代码**。Leader 已在 inbox 中确认该更正并认可 dev 的偏离。

该条 criteria 由两部分组成：
1. **要求**（「边界值必须各自独立取证：相等 Duplicate 与更小 OutOfOrder 是相邻的两个判定」）
   → 已由 M-E / M-F 这对**非等价**变异取证，各自恰好只红 1 条且红的是不同子测试，
   **取证强度不低于原意图**；
2. **取证配方**（那条变异）→ 靶不存在。

我的独立判断：该偏离**等价于原意图**，判 PASS。
**遗留动作（仅 Leader 可做）**：`boundary[1]` 现仍是坏文本。任务已离开 `verifying` 之后
Leader 可在重派/归档时把它同步改成「互换 Duplicate 与 OutOfOrder 的期望值，须各红一条」，
使 DoD 记录自洽——否则该缺陷文本会随 Sprint 归档留存，下一个复用此 DoD 模板的任务会再踩一次。

## 7. 越界申报核对

`git diff --stat 224c960..96641ec` = `classify.go`(59) + `classify_test.go`(99)，**仅此两文件**。
任务 `writes` 声明恰为这两个文件，`packages` = `./internal/macro/bitemporal`。
**无 scope 漂移，无未申报改动。**

## 8. DoD 之外的观察（不影响本次判定）

1. **`revision` 采用字符串比较**，正确性前提是取值为 ISO 8601 形态（字典序 == 时间序）。
   若下游 Lookup/Upsert 喂进 `2026-7-15`（少补零）或纯数字版本号（`"9" > "10"`），
   `Classify` 会**静默判错**且无任何告警。dev 已在注释与 discovery 中申报该前提，
   Leader 亦确认「论证的风险、未验证，当前无非 ISO 调用方」，**不构成本任务 FAIL**。
   建议在 TASK-003+ 的 DoD 里把「只喂 ISO 形态 revision」写成**可测试的约束**，
   而不是只留在注释里——注释挡不住下一个调用方。
2. `Verdict.String()` 的兜底只测了正值 `Verdict(9)`；负值未测。风险极低，不构成缺陷。
3. 遗留物提示（非本任务）：`../wt-verify-TASK-001` 仍存在（他人 worktree），我未触碰。

## 9. 判定

**VERIFIED** —— 7 条 done_criteria 全部有意义覆盖，**9 条针对性变异全部 KILLED**，
另 2 条字面判据变异证伪；dev 的两处主动偏离（组织方式改写、boundary[1] 替代判据）
均已机械核验属实且不弱化。无空洞断言、无 mock（纯函数无需 mock）、无 SKIP，
覆盖率 100%，声明范围与实际改动一致。
