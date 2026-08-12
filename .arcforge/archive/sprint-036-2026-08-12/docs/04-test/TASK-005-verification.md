# TASK-005 验证报告 — Discover 主循环，翻页直到找到未入库的期次

- **验证者**：test-agent-25（Reality Checker，默认判定 NEEDS WORK）
- **判定对象**：`verify_baseline.head = 7b49b13c11f2649017a7f2393da481354dc530e2`（== 当前 HEAD）
- **验证 worktree**：`git worktree add --detach /tmp/verify-036-5 7b49b13c11f2649017a7f2393da481354dc530e2`
- **结论：VERIFIED（8/8 DoD 全部 PASS）**

## 0. 漂移与范围

**双零漂移**：HEAD 与 `discovery_sha256`（`af70f285…`）均与基线逐字相同 ⇒ **未使用任何 `--ack-*`**。
`git show --numstat 7b49b13` → `discover.go 75/0`、`discover_test.go 448/0`、`store_test.go 19/2`，
与 `writes` **三项逐项一致，无越界**。任务文件字段齐全（`discovery` 指针在位，转 `verified` 前已核）。

## 1. DoD 逐条覆盖矩阵

| # | DoD 条目 | 对应测试 | 承重证据 | 判定 |
|---|---|---|---|---|
| F1 | 空库翻到第 2 页找到报告；**同时**断言候选与 `f.calls`；**不硬编码期次** | `TestDiscoverFindsReportOnSecondPage`（期次经 `targetOnPage2` 现问） | M10 KILLED | **PASS** |
| F2 | 碰到已入库立刻停、**不再请求后续页**；必须看 `f.calls` | `TestDiscoverStopsAtKnownPeriod` | M2 KILLED | **PASS** |
| B1 | `MaxPages` 生效 + 总页数夹逼 + **空库翻满**（与 F2 断言对象相反） | `RespectsMaxPages` / `DoesNotExceedTotalPages` / `EmptyStoreExhaustsWhileKnownStopsEarly` | M3、M4 KILLED（§3.2） | **PASS** |
| B2 | 跨页去重；**须有 `require.NotEmpty` 前置** | `DeduplicatesAcrossPages`（锚点在场） | M5 KILLED | **PASS** |
| E1 | 查库失败中断 + `errors.Is`；**另补抓页失败路径带页 URL** | `FailsOnCheckerError` + `ReturnsNoPartialResultOnCheckerError` + `FailsOnFetchError`（两子测试） | M1、M8 KILLED（§3.1） | **PASS** |
| N1 | 全程不碰文章页 | `NeverFetchesArticlePages`（含 `require.NotEmpty` 前置） | — | **PASS** |
| N2 | 守卫登记 `Discover`，**追加不放宽** | AST 版 | **M11 KILLED** + 逐字核对（§3.4） | **PASS** |
| N3 | RED / gofmt / vet / 整包绿 / 覆盖率 ≥92.1%；**承接 T3 两条遗留** | §2、§3.3、§4 | — | **PASS** |

## 2. N3 的命令与输出

```
$ GOTOOLCHAIN=local go vet ./internal/hestia/   → 无输出，exit 0
$ gofmt -l internal/hestia/                     → 无输出，exit 0
$ GOTOOLCHAIN=local go build ./...              → 无输出，exit 0
$ GOTOOLCHAIN=local go test ./internal/hestia/ -count=1 -cover
ok  github.com/newthinker/atlas/internal/hestia  0.857s  coverage: 93.1% of statements
$ go tool cover -func | grep -E 'Discover|total'
discover.go:226:  Discover  100.0%      total: (statements) 93.0%
$ go test -run 'TestStoreExposesNoWriteMethods|TestPackageExposesNoWriteFunctions' -v
--- PASS（两条）
```
**覆盖率口径**：`-cover` 93.1% / `-func` 93.0%，**同树同口径**，0.1 为舍入（本 Sprint 第三次同形态）。
两者均 ≥ DoD 下限 92.1%，也 ≥ 上一水位 92.8%。

**RED 独立复现**（`discover.go` 回退到 TASK-004 版 `2b93ccd`，测试保持交付版）：
```
internal/hestia/discover_test.go:506:14: undefined: Discover
internal/hestia/discover_test.go:508:3:  undefined: DiscoverCfg
（及 3 处同形）
```
因**预期原因**失败，**未被 `"time" imported and not used` 污染** ⇒ reviewer 指出的计划第三处
import 错误（Step 1 多了 `"time"`）确实被规避了。注意 `discover.go` 里 `time` 是**被用到的**
（`DiscoverCfg.Timeout time.Duration`）；计划的错在于把它放进了**测试文件**的 import 清单。

## 3. 变异/消融独立复验（harness 自写）

`scratchpad/test25-TASK-005-ablation.sh`，锚点 `ARCFORGE_MUT_REF` 可覆写、默认**全 sha**；
隔离 worktree。四道闸：基线闸（`--- PASS` = 562 全绿）、生效闸、**编译失败闸**、
**计数自证 12 == 12 → OK**。结果 **11 KILLED / 1 SURVIVED**。

| 变异 | 结果 | 唯一/主要杀手 |
|---|---|---|
| **M1** checker 错误路径 `return nil,err` → `return out,err` | KILLED | **只被** `ReturnsNoPartialResultOnCheckerError` 杀（§3.1） |
| M2 命中已入库不停（`continue`） | KILLED | `StopsAtKnownPeriod`（`should have 2 item(s), but has 3`）+ `EmptyStoreExhausts…` 的 `Greater` 行 |
| **M3** 发现候选就停（违反 spec §4.3） | KILLED | `EmptyStoreExhausts…`：`应一路翻到 MaxPages（spec §4.3 首跑行为）` |
| M4 不夹逼总页数 | KILLED | `DoesNotExceedTotalPages` |
| M5 去重失效 | KILLED | `DeduplicatesAcrossPages` |
| M6 `scanPage` 失败改 `continue` | KILLED | `FailsOnScanError` |
| M7 `pageURL` 失败改 `break` | KILLED | `FailsOnPageURLError` |
| M8 翻页抓取失败改 `break` | KILLED | `FailsOnFetchError/翻页中途抓取失败` |
| M9 `parsePaging` 失败改成只扫第 1 页 | KILLED | `FailsOnPagingParseError`：`不得退化成只扫第 1 页` |
| M10 第 1 页重复请求 | KILLED | 9 处（所有断言 `calls` 的用例） |
| **M11** 守卫期望列表删掉 `Discover`（M9 同形） | KILLED | `TestPackageExposesNoWriteFunctions` |
| M12 单独移除「同页干扰项」的 `NotEmpty` 锚点 | **SURVIVED**（**预期**） | 见 §3.3 —— 锚点只在 `scanPage` 返空时起作用 |

### 3.1 重点① —— M1 的杀手**正是**新补的那条，旧的那条仍然平凡

Leader 要我查的不是「M1 现在 KILLED」，而是**杀死它的是哪条断言**。实测：

```
KILLED(M1)  失败测试: --- FAIL: TestDiscoverReturnsNoPartialResultOnCheckerError|
  Error:    Expected nil, but got: []hestia.Candidate{…ArticleID:"2026011512340454111"…}
  Messages: 第 1 条已收进 out、第 2 条查库失败——那 1 条也不得返回出去
```

**失败测试集合里只有这一条**；`TestDiscoverFailsOnCheckerError`（那条 `assert.Nil` 名义上也在守
「不返回部分结果」）**在 M1 下保持绿** ⇒ **dev 的自陈完全属实**：它的 checker 恒失败、第一次调用
就返回，那一刻 `out` 本来就是空的，两种写法返回值相同。

⇒ 它补的 `failOnNthChecker`（第 n+1 次才失败）+ 同页两候选的合成页面**产生了真实的鉴别力**，
不是把一条无效断言换个位置。**这不是 T3 那个形态的第三次，是它的正确闭合。**

值得记的是 dev 自己在注释里写明了这一点（「我在 T3 修了那个实例，写本条上游的
`TestDiscoverFailsOnCheckerError` 时**又犯了一遍**」），并把规律抽出来：
**「错误路径断言 Nil」只有在失败点之前已经收集过东西时才有鉴别力。**

### 3.2 重点② —— `assert.Greater` 的两个参数确实来自同一次运行的两个对照组

我核了代码结构（不是采信描述）：`fa`/`fb` 是**同一个测试函数内**的两个 fetcher，
两次 `Discover` 调用都在该函数内，最后
`assert.Greater(t, len(fa.calls), len(fb.calls))` —— **同一次运行、两个对照组**，
不是各自跑各自的。

更强的证据是**该行被两个相反方向的变异分别打红**：
```
M2（恒不停）    → "3" is not greater than "3"    ← fb 变成 3
M3（发现即停）  → "2" is not greater than "2"    ← fa 变成 2
```
⇒ dev 的论证成立：**「两者必须不同」这个性质不属于任何一条单独的用例**，
拆成两条各自只是「calls == 某数」，那个性质就无人守。

### 3.3 重点③ —— 锚点三格齐全（**但我第一次做错了，见 §5**）

用**锚定**的 `-run '^TestScanPage$/^同页的干扰项不被收进来$'`：

| 组 | 条件 | 结果 |
|---|---|---|
| (a) | `scanPage` 恒返回 `nil` + **保留**锚点 | **FAIL**：`Should NOT be empty, but was []` / 「前置锚点：…」 |
| (b) | `scanPage` 恒返回 `nil` + **移除**锚点 | **PASS** ← 假绿，与 dev 所述一致 |
| (c) | 未变异 + 保留锚点（**阴性对照**） | **PASS** |

⇒ **消融自证与阴性对照两个方向都成立**，锚点是唯一排除该假绿的断言。
M12 单独 SURVIVED 不是缺口——它只说明「不喂空集时锚点无事可做」，这正是锚点的定义。

### 3.4 重点④ + N2 —— spec §4.3 与守卫登记

**`Discover` 里没有「发现候选就停」的逻辑**（读码确认：唯一的提前返回是 `if has { return out, nil }`）。
M3 注入该逻辑后被 `EmptyStoreExhausts…` 打红，消息正是
`空库下 HasPeriod 恒 false，应一路翻到 MaxPages（spec §4.3 首跑行为）` ⇒ 该行为**有守卫**。

守卫登记逐字核对（按 DoD 明写：靠人核，不靠 M11 同形）：
```
assert.Equal / 十一项 / "DefaultThresholds", "Discover", "NewPBOCFetcher", …, "Validate"
assert.Subset + assert.Contains(t, got, …) 出现次数: 0
```
`assert.Equal` 全集合精确相等**原样保留**，在 T1/T4 的登记基础上**追加**（十→十一），
字典序正确（`De` < `Di` < `N`）。`DiscoverCfg` 是结构体类型，两条守卫均未打红（实测）。

## 4. 承接 TASK-003 两条遗留的验收

**① 锚点已补并自证** —— 见 §3.3，两个方向都跑了。

**② 11 处 `range` 逐个核实** —— 我独立分类，与 dev 声称**逐项吻合**：

| 类别 | 处数 | 行号 |
|---|---|---|
| 字面量表格/切片（构造上非空） | 5 | 103, 139, 203, 230, 412 |
| `require.NotEmpty` 直接前置 | 4 | 280, 296, 654, 867 |
| 循环后的肯定式 `assert.Contains` | 1 | 421（`titles` 空则 `Contains` 红） |
| **间接保护** | 1 | 657（`range seen`） |

dev 坦承第一次 grep 漏掉了 `for k, n := range seen` 这种形态——我复核的 11 处与 `range` 的实际
出现次数（11）一致，无遗漏。

**关于那处「间接保护」（Leader 特别点名）**：
```go
require.NotEmpty(t, got, "前置锚点：…")   // ← 锚点
…
seen := map[string]int{}
for _, c := range got { seen[…]++ }        // 654：无条件累积
for k, n := range seen { assert.Equal(t, 1, n, …) }   // 657：间接受保护
```
**当前稳固**：`seen` 在紧邻的三行内**无条件**从 `got` 累积，`got` 非空 ⇒ `seen` 非空。
**且没有任何生产代码变异能破坏它**（它依赖的是测试内部的数据流）。
失效条件只有一个：**将来有人给 654 那个循环加上 `continue` 类过滤** ⇒ `seen` 可能空而 657 静默平凡。
建议（非阻断）：若 T7 或后续任务动这一段，顺手加 `require.NotEmpty(t, seen)`。

**③ `TestDiscoverFailsOnPageURLError` 里的语义闸** —— Leader 要我确认那条 `require.NotEqual`
真会在「替换没改中目标」时红。实测：把 `strings.Replace(...)` 换成原样不变后
```
Error:    Should not be: "/goutongjiaoliu/113456/113469/11040-%1.html"
Messages: 解析出的模板必须已被改坏，否则本条测的是好模板
--- FAIL: TestDiscoverFailsOnPageURLError
```
⇒ **它确实会红，且红在语义闸本身而不是别处**。这是把变异 harness 的「语义闸」下沉进测试内部，
我同意 Leader 的评价——**它把一个只有跑 harness 时才有的检查，变成了测试自带的**。

## 5. ⚠️ 我自己的一次无效实验，已作废重做

首轮做 §3.3 时我用的是 `-run 'TestScanPage/同页的干扰项不被收进来'`（**未锚定**）。
结果 (b) 组得到 **FAIL**，与 dev 的声称相反——我差一步就据此反驳它。

复查发现：**Go 的 `-run` 对顶层测试名是【前缀匹配】**，那条命令实际跑出 **7 条 `=== RUN` 行
= 6 个顶层测试 + 1 个子测试**：

```
=== RUN   TestScanPage                              ← 顶层（本来要测的）
=== RUN   TestScanPage/同页的干扰项不被收进来           ← 子测试（本来要测的）
=== RUN   TestScanPageReturnsEveryReportOnPage      ┐
=== RUN   TestScanPageStripsInlineTags              │ 5 个被前缀匹配捎带进来的
=== RUN   TestScanPageFailsOnUnresolvableURL        │ 顶层测试，且因为没有子测试，
=== RUN   TestScanPageReturnsNoPartialResultOnError │ 不受子测试过滤影响、整个跑完
=== RUN   TestScanPageFiltersRatherThanMisses       ┘
```
⇒ 兄弟测试的红污染了退出码。锚定后是 **2 条 RUN 行 = 1 顶层 + 1 子测试**。

⚠️ **本节初版我把这个数写错了**：写成「实际跑了 **7 个顶层测试**」，实际是 **7 条 RUN 行 /
6 个顶层**。我数的是 `=== RUN` 行却标成了顶层测试数 —— 而列在后面的名字恰好只有 6 个，
**证伪它的数据又一次就在同一段里**。Leader 核对时指出口径不一致，我重跑确认后更正。
这正是本 Sprint 反复出现的计数口径问题（F39：dev 报 266 / Leader 数 562，顶层 vs 含子测试）。

锚定成 `^TestScanPage$/^同页的干扰项不被收进来$` 后，三格与 dev 所述**逐格吻合**。

**这条对本项目要紧**：「按 DoD 逐条单跑取证」已经写进 TASK-003/005 的 DoD，
而未锚定写法会让那种取证**系统性地拿到兄弟测试的结论**。
⇒ 建议后续 DoD 里凡要求单跑的，一律写成 `^Top$/^Sub$`；退一步说，
即使用了未锚定式，也只读那一行 `--- PASS: TestFoo`，**不要读整体退出码**。

## 6. 结论

**VERIFIED。** 8 条 done_criteria 逐条有对应测试、逐条有消融证据；12 条变异 11 KILLED，
唯一 SURVIVED 的一条属预期（锚点在不喂空集时无事可做，配对消融已证其承重）；
四条特设重点逐条查实：M1 的杀手是新补的那条而非旧的、`assert.Greater` 来自同一次运行的两个对照组
且被两个相反方向分别打红、锚点三格齐全、`Discover` 无「发现候选就停」且该行为有守卫；
11 处 `range` 分类无遗漏；RED 独立复现；双零漂移；主工作区零污染。

## 7. 主工作区完整性

变异窗口内 + 收尾双重核实，三文件 sha256 与 `git status --porcelain` 前后逐字相同：
`discover.go 6e83db5a…`、`discover_test.go dbd2f1a4…`、`store_test.go 416547a2…`。
变异树收尾 sha256 一致；`/tmp/mut-036-5`、`/tmp/verify-036-5` 均已 remove + prune。
