# TASK-001 验证报告 —— 季报识别①（链接层 + discover 标题 + 期次类型）

- **验证者**：test-agent-26
- **被验交付**：dev-agent-52，提交 `13aefb6`（feat）+ `710ef62`（test：否定式边界改为钉 0）
- **验证基线**：`verify_baseline.head = 67249ffb7961aa838f57b4cdfbfef3c94a2ffaaf` = 承接时 HEAD ⇒ **无漂移**
- **assignment_epoch**：1
- **结论**：**VERIFIED**（8/8 条 done_criteria 全部 PASS，无未覆盖项）

---

## 0. 承接核实

| 核项 | 结果 |
|---|---|
| 验证对象漂移 | `verify_baseline.head` == 承接时 HEAD ✅ 无漂移 |
| 方案 C 时序 | `git merge-base --is-ancestor 710ef62 HEAD` = YES ✅ |
| 实际改动 vs `writes` | **逐字一致，无越界**（见下） |
| **DoD 被改写过** | 指纹 `ef0d8b3e8fd77da6` → `d49b7b9be8a82e2e`，**diff 原文见 §1** |
| discovery | 文件存在（21292 B）；**任务文件的 `discovery` 字段原本缺失，由我补上**（见 §6） |

**改动范围**（`git diff --numstat 13aefb6^ 710ef62`，base 取 `13aefb6` 的父 `2e93115`）：

```
47/4    internal/hestia/discover.go          178/8   internal/hestia/discover_test.go
47/6    internal/hestia/types.go             96/1    internal/hestia/types_test.go
7/1     internal/hestia/thresholds.go        50/0    internal/hestia/testdata/README.md
515/0   testdata/pboc-2025-09-q3.html        512/0   testdata/pboc-2026-03-q1.html
640/0   testdata/pboc-index-p18.html         640/0   testdata/pboc-index-p7.html
```

**`writes` 声明的演化是诚实的**（`transitions.jsonl` 实录）：`14:56:24` 加入 `types_test.go` + `thresholds_test.go`
（预留），`15:12:44` 又**移除** `thresholds_test.go` —— 而实际改动确实**没有**碰 `thresholds_test.go`。
⇒ 声明始终与实际一致，且撤回发生在 `dev_done` 之前。**这是越界申报机制被正确使用的样子。**

---

## 1. 🔴 DoD 改写的 diff 原文（leader 要求贴出，不只写「核过了」）

leader 授权 dev-agent-52 自行把「钉 0 不钉总数」的澄清写进本任务 DoD（leader 已交棒、写不了）。
我在 wave1 开工前抓存了改动前的原文快照，逐字 diff 如下 —— **改动仅限 `functional[1]` 一条**，
`functional[0]` / `functional[2]` / `boundary` / `error_handling` / `non_functional` **一字未动**：

```diff
--- 基线 (ef0d8b3e8fd77da6, 采于 63ac5b6 时点)
+++ 当前 (d49b7b9be8a82e2e)
@@ functional[1] @@
-⚠️ **放宽必须配否定式边界**：reviewer 在仓库 p1 快照实测 `\d{14,}`→15 条、`\d{6,}`→57 条，
-多出的 42 条全是栏目导航页（`/rmyh/105145/index.html` 这类），链接文本过不了 `parsePeriod`
-⇒ 放宽本身安全。**但这 42 条产出 0 候选要有一条断言钉住**，否则下次有人再放宽就没有网了。
+⚠️ **放宽必须配否定式边界**：多出来的那些链接全是栏目导航页（`/rmyh/105145/index.html` 这类），
+链接文本过不了 `parsePeriod` ⇒ 放宽本身安全（reviewer 与 test-agent-26 各自独立复现过：
+能过 `reportTitleRE` 的是 **0 条**）。**但这件事要有一条断言钉住**，否则下次有人再放宽就没有网了。
+
+🔴 **断言钉「0 条候选」，刻意不钉「多出多少条」**（leader 2026-08-12 澄清 + dev 实测对账）：
+原 DoD 写的「42 条」只对 `\d{6,}` 成立，本任务选了 `\d+`。更要紧的是 —— 同一个问题「多出几条」
+有**四个各自都对的答案**，p1 实测：全部命中数之差 **44**（含重复）、按 href 去重 **43**、
+非新闻栏目路径含重复 **43**（与前一个同值纯属巧合）、非新闻栏目路径去重 **42**；差别只在
+**去没去重**（「网站地图」在页头页尾各一次）与**算不算栏目自链接**（`/113469/index.html`，
+id 是 6 位的 `113469`）。任一个数写进断言，都会在另一个口径的人重跑时红，而**红的理由与实现无关**，
+最容易被误读成「实现错了」进而去改实现。⇒ 前置锚点用**内容式**（钉住具体两条导航链接 +
+`require.NotEmpty`），断言钉 **0**。
```

**判定：改动是纯粹的澄清与收紧，没有放宽任何条目。** 它把一个会产生「无关理由的红」的具体数字，
换成了不随页面装修而变的内容式锚点 —— 而且 dev-52 自己实测对账出**四个口径**（我当初只报了 42/44 两个），
把「为什么不能钉总数」从判断升级成了实证。

---

## 2. 完成标准覆盖矩阵（8 条）

| # | done_criteria | 对应测试/证据 | 判定 |
|---|---|---|---|
| functional[0] | 先抓真实样本；两份 index + 两份正文落 `testdata/`，记下抓取 URL 与日期；**不得用合成标题** | `testdata/{pboc-index-p7,pboc-index-p18,pboc-2026-03-q1,pboc-2025-09-q3}.html` + README 表格（源 URL、**2026-08-12 22:52 CST**、HTTP 字节数、入库字节数、条目日期范围）。标题与 PubDate 与 reviewer 的表逐格一致 | **PASS** |
| functional[1] | `articleLinkRE` 必须放宽；断言 `scanPage` 从真实 p18 提取到 **`5868082`**（钉字面量）；放宽配否定式边界，**钉 0** | 正则 `\d{14,}`→**`\d+`**（无任何位数下界）；`TestScanPage/第 18 页…`：`assert.Equal("5868082", got[0].ArticleID)`；`TestScanPageIgnoresNavigationLinks`：4 份快照各 `require.NotEmpty` + 钉住 `/rmyh/105145/index.html`「货币政策」「网站地图」，末尾 `assert.Zerof(produced)` | **PASS** |
| functional[2] | `parsePeriod` 对两种真实季报标题正确；既有四种映射仍绿；期次值与 `periodEndMonth` 一致并写进 discovery | `TestParsePeriod` 8 条全绿（既有 5 条 + q1/q1_q3/2020-q1）；`q1→03`、`q1_q3→09` 与 `periodEndMonth` 一致；discovery 的 `interfaces_exposed` 已写明供 TASK-004 消费 | **PASS** |
| boundary[0] | 「2026年二季度金融机构贷款投向统计报告」仍被拒；`13月`/`0月` 仍绿；非法季度值语义层拒 | `TestParsePeriodRejects` **12 条全绿**，含那条贷款投向（实测仍被拒）、`13月`/`0月`、新增二/三/四/五季度 | **PASS** |
| error_handling[0]-a | 两个 map 的一致性守卫必须**单向**（`periodEndMonth` 键 ⊆ 白名单；除 `monthly` 外白名单键都有期末月） | `TestPeriodTypeMapsAreConsistent`：两条分开写，`exempt` 用集合而非 `pt != "monthly"`，并**显式断言 `monthly` 不在表内**；另加期末月形态断言 `^(0[1-9]\|1[0-2])$` 与 `periodTypeList` 定序断言。`types.go` 注释写明 monthly 是唯一豁免及理由 | **PASS** |
| error_handling[0]-b | 两处纯字符串取值列表必须由 `validPeriodTypes` 派生 | 新增 `periodTypeList()`（`slices.Sorted(maps.Keys(...))`）；`types.go:181` 与 `thresholds.go:126` 两处均改为 `strings.Join(periodTypeList(), "\|")`。绊线两条：新增 `TestMetaValidateListsEveryValidPeriodTypeInError` + 既有 `TestExemptionRejectsBadPeriodTypes/含未知取值` | **PASS** |
| non_functional[0] | **消融自证**：删季度分支 → 新增季报用例转红且**红的是新加那条**；对 `articleLinkRE` 也做一次 | M1/M2，**我已独立重跑并逐条复现**，见 §4 | **PASS** |
| non_functional[1]+[2] | `types.go:38` 注释「1/6/12」顺手改；覆盖率 ≥93.2%；`gofmt`/`vet` 空；整包 `-count=1` 与 `-race` 全绿 | 注释已改为「monthly 1 / q1 3 / h1 6 / q1_q3 9 / annual 12」并补「**该折算在本包没有实现**，这行只是声明」；实测见 §3 | **PASS** |

> ⚠️ **矩阵最后一行合并了 DoD 的两条**：`non_functional[1]` 与 `non_functional[2]` 的**前半段逐字重复**
> （都是 `types.go:38` 注释那条「顺手改」），`[2]` 只是把覆盖率/gofmt/vet/-race 粘在了重复文本之后。
> leader 已确认这是 DoD 压缩条数时的合并遗留、**不构成 reject**。合并计为一条，**不是漏验**。

---

## 3. 实跑证据

⚠️ **在隔离 worktree（`67249ffb…` 干净树）上采**，理由见 §5。

```
gofmt -l internal/hestia/  → 空        go vet ./internal/hestia/ → 空       go build ./... → OK
go test -count=1 -cover    → ok  coverage: 93.3%   (门槛 93.2% ✅)
go test -count=1 -race     → ok
顶层 PASS 282 / 全部 PASS 614 / FAIL 0
go tool cover -func → parsePeriod 100.0%、scanPage 100.0%、periodTypeList 100.0%
```

`TestParsePeriod` 8 条、`TestParsePeriodRejects` 12 条、`TestScanPage` 5 条子测试全绿。

---

## 4. 消融独立复现（四条全部重跑，不是抽查）

隔离 worktree 内变异，主工作区一字节未碰；每条先 `go vet` 过闸、打印 diff 逐字核对，跑完 `git checkout --` 还原。

| # | 变异 | dev 声称 | **我实测** | 外溢 |
|---|---|---|---|---|
| M1 | 删季度分支（正则段 + 两条 `case`） | 恰好 5 条转红，全是本任务新增；既有 0 条受影响 | **5 条子测试**：`TestParsePeriod/{2026一季度, 2025前三季度, 2020一季度}` + `TestScanPage/{第7页, 第18页}`，红在 `discover_test.go:217/330/356` ✅ | 280+2=282 ✅ |
| M2 | `articleLinkRE` `\d+` → `\d{14,}` | 9 条子测试红，killed_by `discover_test.go:578` | **9 条子测试**：`TestScanPage/第18页` + `FiltersRatherThanMisses ×4` + `IgnoresNavigationLinks ×4`；行号含 **578** ✅ | 279+3=282 ✅ |
| M3 | 删掉 `if seg != "一季度" { return "", "", false }` | 二/三/四/五季度 4 条 `Rejects` 转红 | **4 条子测试全红**，红在 `discover_test.go:270` ✅ | 281+1=282 ✅ |
| M4 | 删否定式边界的内容锚点 **+** 正则改回 `\d{14,}`【阴性对照】 | **PASS（假绿）** | **`TestScanPageIgnoresNavigationLinks` 确实不在 FAIL 列表 ⇒ PASS** ✅ | 280+2=282 ✅ |

**三处值得单独说：**

1. **M3 的价值我实证了。** M1（删整个季度分支）时那 4 条 `Rejects` 用例**没有出现在 FAIL 列表里** ——
   正则不匹配 ⇒ 仍返回 false ⇒ 它们照样绿。⇒ **M1 对它们零鉴别力**，dev-52 的论断成立。
   不补 M3 就分不出「守卫在做功」与「守卫在场而无效」。

2. **M4 的阴性对照成立，而且是被 M2 与 M4 的差集精确证明的**：
   - M2（只改正则，锚点还在）→ `IgnoresNavigationLinks` **4 个子测试全红**
   - M4（改正则 **且** 删三条内容锚点）→ 该用例 **PASS**
   ⇒ **那三条锚点是承重的**，删了它这条「钉 0」的用例就退化成永远绿。
   这是给一条**刚刚才改对**的断言做阴性对照，DoD 没要求，没人会想到要求。

3. **dev-52 对 M2 的自述是克制的、且准确**：它写「`require.NotEmpty` 的**独有**价值是给出可读的红；
   即使删掉它，紧随其后的 `require.Len(got,1)` 也会红……**不夸大：它在本条上不是「唯一挡住假绿的东西」**」。
   ⚠️ 这实际上**温和地纠正了 DoD 的措辞** —— DoD 说该锚点「在这里第一次真正兑现」。
   按 M4，真正兑现「锚点是唯一挡假绿者」的是 `IgnoresNavigationLinks` 那三条，不是 `第18页` 这条。
   **dev 没有顺着 DoD 的说法邀功，而是把差别说清楚了。**

**外溢度全部为 0**（基线 282 顶层 PASS，四个变异体 `PASS + FAIL` 恒等于 282）。收尾时工作树 `git status` 空。

---

## 5. ⚠️ 验证环境事件：主工作区在我验证期间被 wave2 在途代码污染

验完 TASK-003 后主工作区 `git status` 已不干净：`store.go`(+88) / `store_test.go`(+277) 被改，
新增未跟踪的 `ingest_test.go` / `status.go` / `status_test.go` —— wave2 的 `RecentObservations` /
`StatusRow` / status 命令在**主工作区**开发（纯新增，365 insertions / 0 deletions）。

**处置**：TASK-001/002 的全部判定数据在隔离 worktree（`67249ffb…`）上**重采**，
结果与污染前逐字一致（282/614/0/93.3%）⇒ **本次判定未受污染**。
但这属于运气而非机制：`go test` 会编译未跟踪的 `status_test.go`，它当时若已存在且是红的，
**我会把别人的红记到本任务头上**。已单独报 leader。

---

## 6. 我补的字段（申报，非静默修复）

承接时 `jq 'has("discovery")'` 返回 **false**（discovery **文件**在，21292 B，但任务文件缺 `discovery` 字段）。
**实测确认这会阻断**：任务副本改成 `verified` 后跑 validator 报
`[missing-discovery] … 但任务未声明 discovery 字段`，**退出码 1**。该规则只在 `verified` 时触发
⇒ 转前查不出、转后 leader 与 dev 都写不了。故我在转 `verified` 前经写通道补上并 jq 直读核实。
**TASK-002 同样缺失、一并补；TASK-003 由 dev-54 自己写了。**

---

## 7. DoD 之外的观察（不据此判定）

**O1（设计，值得下游知道）** 季度段正则写成 `[一二三四五六七八九十]季度` 而非只列「一季度」，
让二/三/四/五季度落进**语义层**被显式拒绝，而不是在正则处悄悄失配。两种写法可观测结果相同，
差别是语义层的拒绝**有位置、有注释、有用例钉住**（M3 证明了这些用例真的由它挡下）。
同样的安排见于既有的 `13月`/`0月`。

**O2（命名，对 TASK-004 是硬约束）** `q1_q3` **不是 `q3`**：「前三季度」是 1–9 月累计，
第三季度单季是 7–9 月，**两者期末月同为 `09` 而月均折算除数是 9 与 3**。
`TestMetaValidateRejectsBadPeriodType` 已把 `"q3"` 加进拒绝列表。TASK-004 消费这个 periodType 时
若按字面理解成「第三季度」，就会在 `validPeriodTypes` 注释警告的那个量级上出错。

**O3** 月均折算（除数 1/3/6/9/12）**在本包没有实现**，`types.go` 的注释现在明确说了这一点。
下游若以为它已实现会踩空 —— 这正是 dev-52 补那半句的原因。

**O4（给 TASK-004 的提醒，已写在 README 里）** 两份季报正文快照的「模板 / 板块数」两格是**待定**，
README 明写「谁先跑谁填，别照着上面两行的 `rule@v2` 抄一个看起来合理的值」。**这条提醒本身就是一道防线**，
建议 TASK-004 的 dev 先读 README 再动手。

---

## 8. 复现命令（锚已钉全 sha）

```bash
git worktree add --detach ../wt-verify-t1 67249ffb7961aa838f57b4cdfbfef3c94a2ffaaf
cd ../wt-verify-t1
GOTOOLCHAIN=local go test ./internal/hestia/ -count=1 -cover     # 93.3%
GOTOOLCHAIN=local go test ./internal/hestia/ -count=1 -race
git diff --numstat 2e93115b6e88de55b357026ca086890ea87ba93b 710ef62 -- internal/hestia/
```
