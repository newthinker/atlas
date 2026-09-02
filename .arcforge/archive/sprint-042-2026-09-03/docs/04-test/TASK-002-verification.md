# TASK-002 验证报告 —— periodAlt 与 cumulativePeriods 同步加「今年前N个月」族

- **验证者**：test-m1c4-a
- **判定对象**：`master @ 99415b9621d99d920ee0e18025db77f893096b1a`（= `verify_baseline.head`）
- **隔离对**（A/B 用）：base `8d02371ca2bc9513dac1a625bf3a8c73d70a68c2` / post `d54df6d8cb6c899778ed0d7b60d8a9d6a60532ee`
- **assignment_epoch**：1
- **结论：VERIFIED**

---

## 0. 基线核对 + 两类数字钉在哪棵树

| 项 | verify_baseline 记录 | 判定时实测 | 一致 |
|---|---|---|---|
| `head` | `99415b9621d99d920ee0e18025db77f893096b1a` | `git rev-parse HEAD` 同值，且 `99415b9..master` 为空（baseline 就是 master 顶端） | ✅ |
| `discovery_sha256` | `de1ade84020d806de…` | `shasum -a 256` 同值 | ✅ |

**两类数字分树核实（本任务第一次从头执行这条新纪律）**：

| 数字类型 | 我钉在哪 | 核实 |
|---|---|---|
| 门禁类（覆盖率 / test / vet / gofmt） | **merge 后 master `99415b9`**（主仓库，代码目录 0 行改动） | ✅ |
| A/B 比对类（背对背 diff） | **隔离对 `8d02371` / `d54df6d`** | ✅ `d54df6d` 的 parent 实测确为 `8d02371`；两树间 diff 恰为本任务 2 个文件（`38/1` + `128/4`），无第三方改动混入 |

⇒ **未把 A/B 钉在 `verify_baseline.head` 上**，避免了 TASK-004 那次「把别人的行为变化算到本任务头上」。

---

## 1. done_criteria 覆盖矩阵（8 条，逐条对照）

| # | 维度 | 完成标准（摘要） | 对应测试 / 证据 | 判定 |
|---|---|---|---|---|
| F0 | functional | `cumulativeThisYearAlt` 11 项穷举、两位数排前、并入 `cumulativeMonthAlt`；`cumulativePeriods` 补对应 11 键 | 探针实测（§2）：11 项 ↔ 11 键、正则有而 map 无的项 = **空**、前三项为两位数、总键数 37、`cumulativeMonthAlt` 总项数 32；且**未**使用 `今年前[0-9]{1,2}个月` 字符类 | **PASS** |
| F1 | functional | 三条测试存在且通过；n=2..12 每格**正则命中 且 口径表为真，两条断言缺一不可** | 全跑 PASS（11 + 6 + 1 子测试）；**源码逐条核实**断言存在（§3），非只看测试名 | **PASS** |
| B0 | boundary | 结转项 8 未触发，`extract_test.go` 那一格**原样保留** | `git show --numstat d54df6d` 中 `extract_test.go` **命中 0 行**；`go test -run TestExtract` **绿**；探针实测 `cumulativePeriods["今年以来"] = false`、`periodPat` 整段不认它 | **PASS** |
| B1 | boundary | 交替顺序是零成本防御，**别把「必须如此否则会错」写进注释** | 注释原文：「两位数排在一位数前是零成本防御，**不是这里的安全性来源**（同上两族：Go 在交替上会回溯，leftmost-first 是对整体匹配而言的）」 | **PASS** |
| E0 | error_handling | 🔴 A/B **全量 diff**（不是 grep）；`2022-05` 离开失败清单；**其它期次的明细行不得被波及** | 我自己复现（§4）：交替各 2 轮、两端确定性自检通过；18 变更行；期次去重 = `{2022-05}`；**剔除 2022-05 后明细行 55/55 逐字节一致** | **PASS** |
| E1 | error_handling | 语料须主仓库绝对路径 + `--allow-incomplete` | 照此执行，四次运行退出码均 0、各 208 行 | **PASS** |
| N0 | non_functional | 门禁全过 + 覆盖率 ≥96.1%，且**测自 merge 后 master** | 自跑于 `99415b9`（§5）：gofmt 仅 2 个既有欠账、vet 零输出、`go test` 退出码 0、cover **96.1%** 未跌破 | **PASS** |
| N1 | non_functional | 交付流程，**merge 先于 `dev_done`** | merge `99415b9` 于 `00:38:11Z`，`dev_done` 于 `00:40:02Z` ⇒ **merge 早 1 分 51 秒** ✅；commit 锚定 `feat(TASK-002):` | **PASS** |

---

## 2. F0：一一对应的机器核实

用临时探针（跑完即删，工作区复校 0 行改动）直读包内私有符号：

```
正则族项数=11  内容=[今年前10个月 今年前11个月 今年前12个月 今年前2个月 … 今年前9个月]
cumulativePeriods 里「今年前」键数=11
正则有而 map 无的项=[]                     ← 一一对应无缺
前三项(应为两位数)=[今年前10个月 今年前11个月 今年前12个月]
cumulativeThisYearAlt 在 cumulativeMonthAlt 内=true
cumulativeThisYearAlt 在 periodAlt 内=true
cumulativePeriods 总键数=37                ← dev 报 26→37 ✅
cumulativeMonthAlt 总项数=32               ← dev 报 21→32 ✅
「今年以来」是否被认得(应 false)=false      ← 结转项 8 未触发
```

**并入 `cumulativeMonthAlt` 而非直接进 `periodAlt`** 这个决策我复核了理由：`sections.go` 的
`quarterOrLongerPeriods()` 是从 `cumulativePeriods` **减去** `cumulativeMonthAlt` 派生的，
放错地方会让这 11 个月度累计前缀被误判成「季度及以上」。实测该等值断言本改动后**无需修改且绿**，
是该选择正确的旁证。

---

## 3. F1：断言逐条核实（读源码，不看测试名）

`TestThisYearPrefixFamilyIsRecognised` 的每个子测试确实有 **两条独立断言**：

```go
m := anchored.FindStringSubmatch(p)
require.NotNilf(t, m, "periodPat 必须整段认得 %q…", p)     // ① 正则命中
assert.Equalf(t, p, m[1], "…必须逐字整段捕获", p)           // ①' 整段捕获
assert.Truef(t, cumulativePeriods[p], "…必须在 cumulativePeriods 里…", p)  // ② 口径表
```

`TestThisYearPrefixDoesNotShadowExistingFamilies`：DoD 要求 5 个代表（`前五个月`/`1-7月`/
`全年`/`上半年`/`5月份`），实际 **6 个**（多一格「今年以来」断言 `assert.Nil`，即必须**不**被认得）。

`TestCumulativeThisYearAltAndCumulativePeriodsAgree`：DoD 只要求项数相等，dev 另加了**正向逐项**
（`项数相等` 挡不住「两边各有一项对不上」）。

断言非空洞，无 mock。

---

## 4. E0/E1：A/B 背对背（隔离对，交替各 2 轮）

```
base = 8d02371ca2bc9513dac1a625bf3a8c73d70a68c2   （d54df6d 的父，实测确认）
post = d54df6d8cb6c899778ed0d7b60d8a9d6a60532ee
交替顺序：base(r1) → post(r1) → base(r2) → post(r2)
```

- **确定性自检**：base 两轮逐字节一致 ✅；post 两轮逐字节一致 ✅ ⇒ 差异是真差异。
- 四次运行退出码均 0，各 208 行。
- **全量 diff（不是 grep）**：34 行 / **18 个变更行**（与 dev 报的 18 行一致）。

### 机器判据

| 判据 | 结果 |
|---|---|
| 全部变更行里出现的期次（去重，含汇总行末尾的期次列表） | **只有 `2022-05`**，种类数 = 1 ✅ |
| 🔴 **其它期次的明细行是否被波及**：取全部「期次开头」明细行，剔除 `2022-05` 后两端比对 | base 55 行 / post 55 行，**逐字节一致** ✅ ⇒ **0 条被波及** |
| `2022-05` 是否离开「本迭代不解析」 | base 第 204 行在该清单 → post **不在**；改列于「解析失败」✅ |
| 汇总计数 | 待解析 199→200、**本迭代不解析 19→18**、解析失败 38→39 ✅ |
| `2023-05`（预期不变） | base=4 次 / post=4 次，**未变** ✅ |

**关于「本迭代不解析」计数**（Leader 特别叮嘱的对照点）：TASK-001 那次它 **19→19 没变**
（2023-05 只换了失败原因、没离开分类）；本任务它 **19→18 确实变了** ⇒ `2022-05` 真的离开了该分类，
不是「换了个原因还留在原地」。**符合预期，不需要去查 2022-05 的下落**——它出现在「解析失败」里
（报 `人民币存款分部门段的期次前缀 "5月份" 不是累计口径…`，属 TASK-005 射程）。

**`2023-05` 仍在失败清单** ⇒ 按 DoD 的预期差异声明，**不判红**（它卡在 `selectRMBCumulativeFlow`，
TASK-005 的射程）。

---

## 5. N0：门禁类（测自 merge 后 master `99415b9`）

| 项 | 判据 | 实测 |
|---|---|---|
| `gofmt -l internal/hestia cmd/atlas` | 除两个既有欠账外无新增项 | 输出恰为 `backtest_test.go`、`crisis_test.go` ✅ |
| `go vet ./internal/hestia/... ./cmd/...` | 零输出 | 零输出，退出码 0 ✅ |
| `go test ./internal/hestia/... -count=1` | 全绿 | 退出码 **0** ✅ |
| 覆盖率 | ≥ 96.1% | `go tool cover -func` total = **96.1%**，未跌破 ✅（仍是零余量） |
| 无新增依赖 | `go.mod`/`go.sum` 不在改动里 | `git show --numstat d54df6d` 命中 **0** ✅ |
| 注释任务编号 | 带 `M1c-4 的 TASK-002` | `profiles.go`、`profiles_test.go` 均命中 ✅ |

**声明范围 vs 实际改动**：`writes` 声明 3 个文件，实改 2 个
（`profiles.go` `38/1`、`profiles_test.go` `128/4`，与 dev 报的 numstat 逐个相同）。
`extract_test.go` **声明了但一个字没改**（`git show --numstat` 命中 0 行）——
**在范围内、无越界**，符合 boundary[0] 的「结转项 8 未触发，那一格原样保留」。

---

## 6. 2×2 消融复核（Leader 点名，我在隔离副本上重跑）

harness 作用在隔离 worktree `../wt-t002-post` 上；被变异文件在**主工作区**的 sha256
（`a0908148e78f401e4529ae5673f398dd96be6af8ffb7ebb8660a2c2d90a30011`）在每格窗口后 + 收尾均校验未变，
主工作区代码目录改动 **0 行**。每格过 `gofmt -e` 语法闸 + 打印变异 diff 逐字核对 + `setup failed|build failed` 有效性闸。

### 三格 × 语料输出的两两关系（逐字节）

| 对 | 关系 |
|---|---|
| base ↔ **格 A**（只加口径表） | **逐字节一致** ⇒ 完全 no-op ✅（dev 结论属实） |
| base ↔ **格 B**（只加正则） | 差 4 变更行（**只是诊断文本变了**，2022-05 仍在原分类） |
| base ↔ **post**（两处都加） | 差 18 变更行（真修复） |

⇒ **两处确是独立硬卡点**：任一单独存在都修不好 `2022-05`（格 A/B 的「本迭代不解析」均仍为 **19 篇**，
`2022-05` 明细行仍在清单里）。

### 🔴 格 B 的「诊断退化」—— Leader 要引用进 CONTRACTS 的那条，**属实**

| 特征串 | base | 格 A | **格 B** | post |
|---|---|---|---|---|
| `失败的成因是` | 2 | 2 | **0** | 0 |
| `期次前缀不被识别` | 2 | 2 | **0** | 0 |
| `该往 periodAlt 与 cumulativePeriods 同步加一项` | 2 | 2 | **0** | 0 |
| `among 3 candidate` | 0 | 0 | **2** | 0 |

**base（两处都不加）** 时 `2022-05` 的诊断是明确的：

> `hestia: 人民币存款期内合计 失败的成因是**期次前缀不被识别**，不是本节没有累计句: 正文里有 1 句人民币合计句的句尾完全正确，但它们的前缀 今年前5个月 不在 periodAlt 里，于是整条模板不命中、根本没进候选集。这是**解析器缺口**——该往 periodAlt 与 cumulativePeriods 同步加一项…`

**格 B（只加正则、不加口径表）** 时它退化成泛化错误：

> `hestia: 人民币存款期内合计 not found among 3 candidate sentence(s) [今年前5个月/人民币 5月份/人民币 5月份/外币]: refusing to fall back to a neighbouring one…`

⇒ **「半改比不改更难排查」成立**：不改时报表**直接告诉你怎么修**（并点名了要改哪两处），
只改一半时**那句提示消失了**——候选句由 1 涨到 3（新前缀让句子进了候选集），但口径判定把它们全筛掉，
命中仍为 0，而错误信息已不再指向真正的成因。

⇒ 这条同时**支撑 TASK-005 的 DoD 理由**（改 `selectRMBCumulativeFlow` 时必须保住那段诊断）——
该理由**不需要撤回**。

### 单测消融结果与 dev 的口径差异（**非矛盾，已查清**）

| 格 | 我跑的 5 条（3 新 + 2 既有扩容） | 其中**三条新测试**的红数 | dev 报 |
|---|---|---|---|
| A | `…FamilyIsRecognised` + `…MonthAltEnumerates…`（既有）红 | **1 条**（单红） | 「单红」✅ |
| B | `…FamilyIsRecognised` + `…Agree` + `…PeriodsKeySet`（既有）红 | **2 条**（双红） | 「双红」✅ |

dev 报的是**只看三条新增测试**的口径；我额外跑了两条既有的穷举等值断言，故我这边每格多一条红。
**在同一口径下两者完全一致**，dev 的「A 单红 / B 双红、两次单删失败信息不同、两条断言非冗余」结论成立。

---

## 7. 测试质量评审

- **非空洞**：三条新测试在消融下按预期转红（格 A 杀 1 条、格 B 杀 2 条），非重言式。
- **无 mock**：全部直接断言包内常量与真语料行为。
- **既有断言用「扩 want」而非「放松形态」**：`TestCumulativeMonthAltEnumeratesEveryCumulativePrefix`
  与 `TestCumulativePeriodsKeySet` 都保持**等值**断言（21→32 项、26→37 键）。这是对的——
  让「periodAlt 的组成变了」成为一次显式的、有人看见的改动，而不是静默扩面。
- **穷举而非字符类**：符合 F0 的硬要求，一一对应关系保持可机械核对（`…Agree` 就是那把尺）。

---

## 8. 观察项（不影响判定）

1. **覆盖率 96.1% 仍是零余量**（与 TASK-001 相同）。后续任务新增未覆盖语句会立刻破线。
2. dev 的 discovery **两个锚都写了**且明确注明「门禁类测自 merged_master、A/B 测自隔离对，
   两类数字来自不同的树，不要混读」——这正是 E0 新纪律第 4 条的要求，**本任务是第一个从头满足它的**。
3. 本任务只登记了实测出现的「今年前5个月」之外的另十项，理由与既有两族一致（漏一个是**静默 0 命中**）。
   这是有意的过度登记，不是冗余。

---

## 9. 结论

**8 条 done_criteria 全部 PASS**。证据均由验证者独立产生：门禁类测自 merge 后 master `99415b9`、
A/B 比对测自隔离对 `8d02371`/`d54df6d`（交替各 2 轮 + 两端确定性自检）、2×2 消融在隔离副本上重跑
且主工作区指纹未变。核心判据「其它期次的明细行 0 条被波及」用**剔除后逐字节比对**得到，
不靠肉眼看 diff。Leader 点名复核的「诊断退化」属实，可进 CONTRACTS。

**判定：VERIFIED**
