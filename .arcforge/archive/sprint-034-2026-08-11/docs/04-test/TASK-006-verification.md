# TASK-006 验证报告 —— Parse 组装层与期次元数据推导

- **验证者**: test-agent-22 / 承接时 `assignment_epoch` = 1
- **验证对象**: `parse.go` `parse_test.go` `profiles_test.go` `extract.go` `store_test.go` @ `4948f0f317f2bcc67beb689dc397673b1f2f58b4`
- **验证环境**: 隔离 detached worktree，锚 `4948f0f317f2bcc67beb689dc397673b1f2f58b4`
- **判定**: **PASS → verified**

---

## 一、基线（自证）

| 项 | 我的实测 | Leader 复现 | 一致 |
|---|---|---|---|
| `go test ./internal/hestia/ -v -count=1` | **358 PASS / 0 FAIL / exit 0** | 358 / 0 | ✓ |
| `go test -cover`（go test 自报口径） | **89.5%** | 89.5% | ✓ |
| `go tool cover -func \| tail -1`（门禁口径） | **89.5%** | — | — |
| `go vet ./internal/hestia/` | exit 0 | exit 0 | ✓ |
| `go build ./...`（全仓） | exit 0 | exit 0 | ✓ |
| `gofmt -l` 未格式化文件 | **0** | 0 | ✓ |
| `go test -race` | ok | — | — |

> 与 T5 不同，本次两个覆盖率口径**同为 89.5%**。T5 那次是 89.1 / 89.0——两个口径的差值不固定，
> 各自稳定复现即可，不必视作异常。

变异结束后重跑基线，**358 PASS / 0 FAIL 精确复现**，工作区 `git status` 干净。

## 转前清单（四条，全过）

| # | 检查 | 结果 |
|---|---|---|
| 1 | `jq 'has("discovery")'` | **true** —— dev-45 这次自己写了指针（`transitions.jsonl` 13:39:56 有 `update ['discovery']` 留痕），**前两次都漏**。仍按清单核过，未因它这次做了而跳过 |
| 2 | discovery sha256 vs 基线 | `1b15011e…451edab0` == `verify_baseline.discovery_sha256`，**逐字相同** |
| 3 | commit 范围 vs 声明 `writes` | 实际改 **5 个文件**，与声明 5 项**逐条相同，无越界** |
| 4 | 转后 validator 完整输出 | 见第六节 |

`verify_baseline.head` == 当前 HEAD == `4948f0f`，声明范围内文件 `git diff` 为空 ⇒ **判定对象未漂移**。

---

## 二、完成标准覆盖矩阵（10 条）

| # | 完成标准 | 对应测试 | 变异实证 | 判定 |
|---|---|---|---|---|
| functional[0] | `Parse` 串起四层，`Values` 键全在 `allFields` 内 | `TestParseRealSamples`、`TestParseValuesKeysAreDeclared` | **Q2**（写死 extractor、绕开 detect）→ KILLED | **PASS** |
| functional[1] | `parseTitle` 产出满足 `Meta.validate` 两个正则的 `period`/`periodType` | `TestParseTitle`、`TestParseTitlePadsMonth` | **Q6**（月份不补零）、**Q7**（`titleRE` 去锚）→ 均 KILLED | **PASS** |
| functional[2] | `caliberFor` 取值在白名单内；**多条口径注同时适用时取最新**，且显式表达而非依赖遍历顺序 | `TestCaliberFor`、`TestCaliberForIsOrderIndependent`、`TestCaliberForResultIsAlwaysValid`、`TestCaliberChangesAreDeclaredVersions` | **Q3**（改「取首个命中」）→ KILLED | **PASS** |
| boundary[0] | 两份样本 `Meta` 与 `goldenMeta*` 逐字段相等 | `TestParseRealSamples` | **Q5**（填 `ArticleID`）→ KILLED | **PASS** |
| boundary[1] | `period`×`period_type` 组合过 M1b-1.5 校验 | `TestParseMetaPassesM1b1Validation` | **Q6** 连带打红 → KILLED | **PASS** |
| boundary[2] | `monthly` 零样本必须显式登记（二选一，不得沉默支持） | `TestParseRejectsMonthlyUntilSampled` | **Q4**（去掉守卫）→ KILLED | **PASS**（选项①，见四·③） |
| error_handling[0] | 标题不认识时报错且信息含原标题，不猜期次 | `TestParseTitleRejectsUnknownShape` | **Q7** → KILLED | **PASS** |
| error_handling[1] | `Parse` 不填 `ArticleID` | `TestParseRealSamples` | **Q5** → KILLED | **PASS** |
| **error_handling[2]** | **`Parse` 必须紧接 `detectExtractor` 并把 error 当致命，且此时不产出任何 Values** | `TestParseStopsAtDetectExtractor` | **Q1**（吞掉 error）→ KILLED，**只打红这一条** | **PASS**（详见三） |
| non_functional[0] | `Parse` 不碰数据库、不调 `Save` | `TestParseDoesNotTouchStorage` | **Q8b**（parse.go 出现可编译的 `.Save(` 调用）→ KILLED | **PASS** |

---

## 三、`error_handling[2]`：我在 T3 提的悬空承诺，现在真的成了

这条是我验 T5 时接受 T3 现有签名的**条件**。当时的事实是：`splitSections` 与 `detectExtractor`
的**生产调用点数量都是 0**，「两者紧邻」是对 T6 的预期而非既有事实，
⇒ T3 `error_handling[0]` 承诺的保证**不存在于代码库任何位置**。

**现在它存在了，且经变异确认是被守住的**：

- **实现**（`parse.go:126-130`）：`splitSections` 之后**紧接**  `detectExtractor`，`err != nil` 直接
  `return Observation{}, err`，中间没有任何其它调用。注释明写「那个前提由这两行兑现，
  **不是别处已有的事实**；删掉或挪后这一段，T3 的保证就随之消失」。
- **Q1**（把 `if err != nil` 改成 `if false`，即吞掉错误继续）→ **KILLED，且只打红
  `TestParseStopsAtDetectExtractor`**（3 个 FAIL = 1 顶层 + 2 子测试）。因果精确，无连带。
- **Q2**（绕开 `detectExtractor`、写死 extractor）→ KILLED，打红 4 条含 golden 相关。

判据的**两半都在**且都有效：
1. 错误信息含**实际**板块数（`0` / `3`），不是期望值；
2. `assert.Empty(obs.Values)` ——「切分不认识就停」，不是「先抽取再报错」。

⇒ **该承诺已从「对下游的预期」转为「有测试守护的事实」。**

---

## 四、Leader 要求独立判的三处

### ① 导出面契约变更 —— **同意，且我把它声称的分工实测了**

`store_test.go` 的 `TestPackageExposesNoWriteFunctions` 用**全导出面精确相等**锁定名单，
新增 `Parse` 使其转红。处置是把 `Parse` 加进名单、**不放松断言形状**。

dev-45 的理由：该断言意图是防新增写口绕过 `Save`，而 `Parse` 是纯函数，
**意图未被违反 ⇒ 是断言过宽而非被违反**；`Parse` 不碰库由 `TestParseDoesNotTouchStorage` 独立守着，
两条测试「本条管形状、那条管行为」。

**我没有采信这个分工声明，而是实测了两半**（它正是 F1 那类「未实测的断言」形态）：

| 变异 | `TestPackageExposesNoWriteFunctions`（形状） | `TestParseDoesNotTouchStorage`（行为） |
|---|---|---|
| **Q9** `parse.go` 新增一个导出函数（非写口） | **FAIL**（接住） | PASS（察觉不到） |
| **Q8b** `parse.go` 出现可编译的 `.Save(` 调用 | PASS（察觉不到） | **FAIL**（接住） |

**每条恰好接住对方看不见的那一格 ⇒ 分工是被验证的，不只是被断言的。**
判断：**变更正确**。放松成「不得有写口」需要在 AST 层定义什么算写口，那是不可证伪的；
精确相等虽然过宽，但它把每次扩大导出面都变成一次必须留痕的复核——**过宽是设计意图的一部分**。

> ⚠️ 过程记录：Q8 **首版不作数**。我第一次构造的 `.Save(` 调用类型不匹配，`go vet exit=1`、
> `PASS=0 / FAIL=0` —— 那是**编译失败**不是测试杀死。按纪律重做成可编译的 Q8b 才计入。

### ② 越界申报 —— **合规，时点正确**

`writes` 由 2 项扩到 5 项。`transitions.jsonl` 实证：

```
09:38:51  dev-agent-45  assigned -> in_progress
09:46:42  dev-agent-45  update ['writes']   added=[profiles_test.go, extract.go, store_test.go] removed=[]
13:40:10  dev-agent-45  in_progress -> dev_done
```

**申报发生在 `dev_done` 之前**，符合 CLAUDE.md「必须在 `transition dev_done` 之前把该文件补进声明」。
added 三项与实际改动**逐条一致**，交付报告亦有申报。

### ③ `monthly` 选项① —— **理由成立**

它的理由是「悄悄放行会产出**看起来完全正常**的 Values——键数对、量级对、全在白名单内，
而 `*_ytd` 里可能装着当月数，下游没有闸门拦得住」。

**这个判断我可以从 T5 的实测直接支持**：T5 阶段我实测过 2020 存款/贷款板块各有 **4 个候选句**
（2 期次 × 2 币种），期次维度靠 `cumulativePeriods` 只认「全年/上半年」区分。月报的
「1-5月」这类前缀既不在该表内，也无样本可证哪句在前 ⇒ 风险是实在的，不是假想。

守卫只加在 `Parse` 层、`parseTitle` 保持完整可测，将来拿到样本删掉守卫即可——**边界划得干净**。

---

## 五、F1 复核 —— **dev-45 的判断对，而且它的机制解释比我的完整**

它**没有采信我的读数**，自己重跑了五种形态，并多验了一种（`质押式回购.*利率为` → 1.4）。
五行表与我 T5 报告里的四行**逐项一致**。

它的理由：**「F1 的教训恰恰是未实测的断言被传播，拿另一条未经我验证的断言去替换它，
是重复同一个错误。」** —— **这个判断我完全同意**，而且它是本 Sprint 里唯一一次
有人对「验证者的结论」也执行了同一条纪律。

### 5.1 但它补的机制比我的准，我的 F1 只解释了一半

我在 T5 报告里写「保留 `质押式回购` 前缀会把匹配**起点**锚在回购句上，1.36 在其之前取不到」——
**这只解释了带前缀的形态**。对 `加权.*利率为`（无前缀）我的解释不适用：它的起点确实落在拆借句。

dev-45 补的才是对的：**贪婪的 `.*` 会一路吃到本段最后一个「利率为」，而那正是回购句。**

### 5.2 它新加的那句「两份样本里回购句恰好都排在最后」是机制断言，我实测了

| 形态 | 原文顺序（拆借在前） | **顺序对调**（回购在前） |
|---|---|---|
| `质押式回购加权平?均利率为`（现行） | 1.4 ✓ | 1.4 ✓ |
| `质押式回购加权.*利率为` | 1.4 | **1.36 取错** |
| `加权.*利率为` | 1.4 | **1.36 取错** |
| `加权.*?利率为` | **1.36 取错** | 1.4 |

**⇒ 贪婪与非贪婪随句序互换谁出错，两种放松形态都不安全。**
现行的 `平?` 形态在两种顺序下都正确——**它的正确性不依赖句序，而任何放松形态都依赖。**

### 5.3 这要求我更正自己在 T5 报告里的 F1 定性

我原来写「DoD 的机制描述**是错的**」。更准确的表述是：
**DoD 那句话在本样本上不成立，但它警示的危害是真实的，只是依赖句序才显形。**

我的定性偏重了。已在 `TASK-005-verification.md` 追加更正段（保留原文，标注来源与日期），
按我自己主张的「判定依据只追加不原地改写」处理。

---

## 六、变异汇总：9 条有效，**全部 KILLED，0 存活**

harness 内置「diff 为空即作废」护栏，**本轮实际触发一次**：Q1 首版用多行 sed 未落地，
直接判作废而非打出假 SURVIVED。

| # | 变异 | 包内 PASS（基线 358） | 打红的具名测试 | 判定 |
|---|---|---|---|---|
| Q1 | 吞掉 `detectExtractor` 的 error | 355 | **仅** `TestParseStopsAtDetectExtractor` | KILLED |
| Q2 | 绕开 `detectExtractor`，写死 extractor | 349 | `ParseRealSamples`、`ValuesKeysAreDeclared`、`StopsAtDetectExtractor`、`MetaPassesM1b1Validation` | KILLED |
| Q3 | `caliberFor` 改「取首个命中」 | 351 | `CaliberFor`、`CaliberForIsOrderIndependent`、`ParseRealSamples` | KILLED |
| Q4 | 去掉 `monthly` 守卫 | 357 | **仅** `TestParseRejectsMonthlyUntilSampled` | KILLED |
| Q5 | `Parse` 填 `ArticleID` | 352 | `ParseRealSamples`、`MetaPassesM1b1Validation` | KILLED |
| Q6 | 月份不补零（`2026-6`） | 353 | `ParseTitle`、`ParseTitlePadsMonth`、`RejectsMonthlyUntilSampled` | KILLED |
| Q7 | `titleRE` 去掉两端锚定 | 356 | **仅** `TestParseTitleRejectsUnknownShape` | KILLED |
| Q8b | `parse.go` 出现可编译的 `.Save(` 调用 | 357 | **仅** `TestParseDoesNotTouchStorage` | KILLED |
| Q9 | `parse.go` 新增导出物 | 357 | **仅** `TestPackageExposesNoWriteFunctions` | KILLED |

因果性逐条核对：**五条变异只打红一个具名测试**，其余四条打红的测试均与变异点直接相关，无连带伤害。

---

## 七、validator 观察（约定的第四次）

按与 Leader 的约定，输出一律 `cat` 全量、汇报格式为 `N 条告警 / EXIT=M`。

- **在途（我的设计观察，转 `verified` 之前专门跑）**：**5 条告警 / EXIT=0**，
  5 条全部是 `[scope-writes-outside-packages] TASK-006`，与 `writes` 的 5 项一一对应。
  与 Leader 独立取的那份**逐条相同**。
- **转 `verified` 之后**：见发给 Leader 的消息（本报告落盘时尚未转）。

⇒ 加上 Leader 的在途观察，本次是**双方各有设计观察**的一组，可与 T3（我双向）、
T5（Leader 双向）合并成三次双向实测。

---

## 八、结论

10 条完成标准逐条有证据支撑，**每条至少一条变异实证**；9 条变异全杀、0 存活，因果逐条核对。
自报数字（358 / 89.5% / gofmt 0 / build 0）双方复现一致，范围无越界、申报时点合规、判定对象未漂移。

**我在 T3 提出、经 T5 确认悬空的那条保证，在本任务落地并被守住**——这是本次验证最要紧的一条。

dev-45 三处需要判断的地方处置都正确，其中两处（导出面分工、F1 机制）我做了独立实测而非采信声明，
**结果都支持它的判断，且 F1 那处它比我更准**。

无退回理由；唯一需要跟进的是我自己 T5 报告里 F1 的定性偏重，已追加更正。

**判定：PASS → `verified`**
