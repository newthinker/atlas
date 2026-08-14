# TASK-004 验证报告 · 搜索侧 wzdig 查询构造、结果解析与栏目前缀筛（两轮）

| | 首验 | **复验（返工轮）** |
|---|---|---|
| 判定锚 | `62b924300415fb109e7355aea0785b4c1ab4903b` | **`c661e453920182c04e4f5e7f3d56932c8675f042`** |
| 判定 | VERIFIED | **VERIFIED** |
| 消融 | 21 个，KILLED 21 / SURVIVED 0 | **34 个，KILLED 33 / SURVIVED 1** |
| 覆盖率（背对背） | 93.4% → 93.7% | **93.7% → 94.0%** |
| 报告位置 | 本文件「附录」节 | 本节（§1–§7） |

- **验证者**：test-agent-28（Reality Checker），两轮同一人
- **漂移**：`verify_baseline.head` 与当前 HEAD 逐字一致、`discovery_sha256` 逐字一致 ⇒ **零漂移，未用 `--ack-drift`**
- **验证工作区**：`/Users/zuowei/workspace/go/src/github.com/newthinker/wt-verify-T004r`（detached @ `c661e45`，收尾已 remove）
- **一条 SURVIVED**：日期守卫的**上界**半边（§4.1）。**不据此 rejected** —— 实现正确、该分支本 sprint 不可达，是测试缺口不是缺陷；但**必须在有人加 `--to` 之前补上**。

---

# 第一部分 · 复验（返工轮，锚 `c661e453`）

## 1. 完成标准覆盖矩阵

| # | 完成标准 | 对应测试 | 我自己的消融 | 判定 |
|---|---|---|---|---|
| functional[0] | URL 逐参数 + 中文编码 | URLCarriesMeasuredParams / URLEncodesChineseKeyword | R01 `qAll→q` / R02 去 searchArea / R03 去 advtime / R04 pNo 写死 / R05 中文不编码 —— **5 全 KILLED** | **PASS** |
| functional[1] | 绝对 URL/标题/日期，标题剥 `<font>` | ExtractsFields + StripsHighlightIsNotVacuous | R06 不剥标签 / R07 日期漏月份 —— **2 全 KILLED** | **PASS** |
| functional[2] | 只留 goutongjiaoliu 前缀 | KeepsOnlyGoutongjiaoliu + DropsByPrefixNotByIDShape | R08 改按 32 位 hex 筛 / R09 全放行 —— **2 全 KILLED** | **PASS** |
| functional[3] | 总页数解析 | ReadsTotalPages + RejectsMissingTotals（2 子测试，**钉具体文案**） | R10 正则读成总条数 / R11 计数缺失静默返 0 —— **2 全 KILLED** | **PASS** |
| boundary[0] | 0 条 ⇒ 报错 | RejectsZeroResults + AcceptsPageWithNoKeptColumn（反向） | R12 去掉守卫 —— **KILLED** | **PASS** |
| boundary[1] | **字面表述已按裁决删除**，由 FIX-1 判性质取代 | RejectsOutOfRangePublished + AcceptsBoundaryDates + ChecksAllColumnsDates + DateGuardIsNotVacuousOnRealSample | N1/N2/N2b/N3b/N4/N10/N11/N13/N14/N15 —— **10 KILLED**；**N3（上界）SURVIVED** | **PASS**（裁决已核实，见 §2；缺口见 §4.1） |
| error_handling[0] | 错误**原样**上抛 | PropagatesFetchError + PropagatesParseError | R15 吞成 nil / R16 parse 错被吞 / R21 被**替换**非原样 —— **3 全 KILLED** | **PASS** |
| non_functional[0] | gofmt/vet/test + 不降覆盖率 | 见 §3 | — | **PASS**（93.7% → **94.0%**） |

**FIX-2（本页条数自洽，DoD 之外的返工项）**：ChecksCountAgainstSiteReported（3 子测试）+ RejectsMisalignedItems。
消融 N5 去结构标记守卫 / **N6 页容量换成总页数（我预告的三重撞车）** / N7 去 min 钳位 / N8 末页算术 `(page-1)→page` / N9 页容量 12→15 / N12 整个去掉 —— **6 全 KILLED**。

**dev 自加的两条守卫**：R17 空 id 守卫 / R18 Atoi 溢出 —— **2 全 KILLED**。

**FIX-3（订正 q/qAll 倍数）**：注释已改为单变量对照 2.0×/2.5×，我实网复核逐字相符（§3.4）。

## 2. 「boundary[1] 字面表述无实现」这条裁决 —— 我核实的结果

**依据在三处，我逐处确认存在**：

| 位置 | 内容 |
|---|---|
| `backfill_search.go` 常量段 | 🔴 墓碑注释「这里曾经有一条 `backfillSearchMaxRecords = 5000` … 已删」+ 三条理由 |
| `backfill_search_test.go` 中段 | 🔴「**对验证者**」段落，指明「不再有对应实现，这是显式裁决不是遗漏」 |
| `discoveries/TASK-004.json` | `rework.leader_rulings_2026_08_14.Q2`（含「🔴 对验证者」键）+ `verification.dod_coverage` 对应条目 |

**但我不靠「有人写了理由」放行 —— 三条承重的实测断言我全部实网复现了**（§3.3）：

| 承重断言 | 我的实测（2026-08-14） | 结论 |
|---|---|---|
| 「空查询 ⇒ HTTP 200 + 空壳、计数字段整个不存在」 | HTTP 200、**16452 字节**、`total-records` 与 `total-pages` **两个都缺失**、0 条结果 | ✅ 由「计数字段缺失 ⇒ 报错」拦下，上界轮不到 |
| 「advtime 失效 ⇒ 240 条，远低于 5000」 | 240 条 / 20 页 | ✅ 上界对其命名场景**永不触发** |
| 「区间外条目会立刻出现」 | pNo=1 越界 0 / pNo=11 越界 0 / **pNo=12 越界 7** / pNo=13 越界 12 | ✅ 与 leader 给我的表**逐字相符** |

⇒ **删除是对的，且是我首验时给出的证据的正确落地。** 保留一条「永不触发、且撞上那天会抢先报出方向错误」的守卫，比没有守卫糟。

**顺带一条自我打脸的证据**：首验时我实测 `金融统计数据报告` 全区间 **692** 条，复验时同一条 URL 返回 **693** 条 —— 相隔约一小时，语料真的长了 1 条。**旧的「下界锚 692」在一小时内就已过期。** 这是「判性质不判数量」最直接的经验证明。

## 3. 我自己跑出来的证据

### 3.1 套件
```
go vet ./internal/hestia/           → 空（exit 0）
gofmt -l internal/hestia/           → 空
go test ./internal/hestia/ -count=1 → ok
```
全包 **顶层 PASS 380 / 子测试 PASS 380 / FAIL 0**（本轮 `grep -c '^--- FAIL\|^FAIL'` = 0，无首验那种测试名假阳）。
本任务 **28 条 RUN = 21 顶层 + 7 子测试**，全 PASS。

### 3.2 覆盖率（背对背 A/B/A′，同一 worktree 同一时刻）
| 采样 | 内容 | 覆盖率 |
|---|---|---|
| A | 两个 go 文件移出 package | 93.7% |
| B | 移回 | **94.0%** |
| A′ | 再移出（复采） | 93.7% |

A′==A ⇒ 可复现，**升 +0.3pp**。逐函数（`go tool cover -func`）：**8 个函数全部 100.0%**
（新增的 `backfillSearchCheckCount` 与 `checkBackfillSearchDateRange` 也是 100%）。
⚠️ 见 §4.1：**这个 100% 对 N3 那个缺口没有任何信号**——整个 `if` 是一条语句。

### 3.3 实网复核（13 次请求，全部用实现构造的参数）
除 §2 那三条外，另核实**页容量 12 与末页算术**（`backfillSearchPageSize` 与 `want` 公式的全部依据）：

| pNo（正常态，137 条 / 12 页） | 本页条数 |
|---|---|
| 2 | 12 |
| 11 | 12 |
| **12（末页）** | **5** = 137 − 11×12 ✅ |

### 3.4 FIX-3 的订正数字，我实网复核
| 窗口（均 `searchArea=title`） | `qAll` | `q` | 比值 |
|---|---|---|---|
| 2020-01-01..2026-08-14（`advtime=5`） | 137 | 276 | 2.0× |
| 无日期过滤 | 240 | 610 | 2.5× |

与新注释逐字相符。

### 3.5 样本未变
`testdata/pboc-search-p1.html` 在两轮之间**一个字节未改**（`git diff 62b9243..c661e453` 不含它）；
item 正则与 block 正则在样本上**各匹配 12 次**，页内 `<h3>` 恰好 12 个 ⇒ **块计数是有效的独立锚**，
不会被结果区之外的 `<h3>` 污染。

### 3.6 消融（34 个，我自己的 harness）
纪律：独占 detached worktree；每个变异体先过 `go build` 有效性闸；打印 diff 逐字核对；
anchored `-run` 避免兄弟用例污染；跑完还原并校验 sha256（收尾 = `baac46e0…`，与基线逐字相符）。

**KILLED 33 / SURVIVED 1 / INVALID 0。**

其中 **10 个是「让守卫仍然报错、只改内容」型**（R21/N10/N11/N13/N14 等）—— 这是首验时我漏掉的那一类：
`require.Error` 失败即 `t.FailNow()`，凡「让错误消失」的消融都够不到它后面的 `assert.Contains`/`ErrorIs`。

**两条基线消融已作废**（不是回归失败）：R13/R14 针对已删的 `backfillSearchMaxRecords`。
首验的 M19/M20（错误文案）**未作废，已重新瞄准日期守卫**（N10/N11），均 KILLED。

## 4. 发现

### 4.1 🟡 唯一的 SURVIVED：日期守卫的**上界**半边没有任何测试钉住

```
- if h.Published < lo || h.Published > hi {
+ if h.Published < lo {
```

**跑本任务用例 → 全绿；跑 `go test ./internal/hestia/` → exit 0；跑 `go test ./...`（全仓库）→ exit 0。**

成因：全部越界用例的日期都在 `from` **之下**（2015-02-10）。`AcceptsBoundaryDates` 用的是
恰好等于 `to` 的日期，它只能证明比较不是 `>=`（N2b 确实 KILLED），**不能证明 `> hi` 这一半存在**。

**为什么仍判 PASS 而不是 rejected**（三条都成立才放行）：

1. **实现是对的** —— 两个半边都在，DoD FIX-1「每条 `Published` 落在 `[from,to]` 内」字面满足。
2. **该分支本 sprint 不可达** —— design-spec §131 是 `endTime=<today>`，TASK-007 的 CLI **只有 `--from`、没有 `--to`**（我逐个查过 TASK-005/006/007/008 的 DoD）。`to` 恒为今天 ⇒ 除非站点出现未来日期的条目，`Published > to` 不会发生。
3. **fix_items 的用例要求是「喂一份含越界日期条目的样本」**，未指定方向，已满足。

**但它必须被补上，理由不是现在会出错**：
- `checkBackfillSearchDateRange(hits, from, to)` 是**导出给 TASK-005/006 的接口**，`to` 是**参数**不是常量。
  M1c-2 一旦加 `--to`（回填某个历史区间），**上界会变成 `desc` 排序下在第 1 页就触发的那一半** ——
  即最早的检测点，而它当时**零测试背书**，任何重构都能悄悄删掉它。
- **`checkBackfillSearchDateRange` 的语句覆盖率是 100%，对这个缺口毫无信号** —— 整个 `if` 是一条语句，
  两个半边共用一格。这是「覆盖率 ≠ 守卫有效」的一个干净实例。

**闭合成本一行**：在 `AcceptsBoundaryDates` 之外补一条 `to` 之后的日期（如 `to=2025-09-12` 而条目
`2025-10-01`），断言报错。我已实测：现有实现**会**正确报错，缺的只是那条断言。

⇒ 建议 Leader 二选一：(a) 直接登记为 TASK-005 的一条 DoD；(b) 若认为「导出接口的每个参数都必须被测」是本任务的义务，走 `verified → review_fix`。**我不代你选**，因为这一条的判据取决于「本任务的边界画在哪」，那是你的决定不是我的。

### 4.2 🟡 LOW — `searchArea` 那行注释犯的是 FIX-3 刚修好的**同一个病**，就在它上面几行

注释：「`searchArea=title` 只搜标题（默认搜正文，同一关键词 **1324 条 vs 610 条**）」。
那组数是 **`q` 语义**下测的；本代码用的是 `qAll`。我做同参数对照：

| 关键词（`qAll` + `advtime=5` + 同日期窗口） | 有 `searchArea=title` | 无 `searchArea` | 比值 |
|---|---|---|---|
| 社会融资规模存量统计数据报告 | 137 | **137** | **1.00×** |
| 金融统计数据报告 | 693 | 733 | 1.06× |

⇒ 在本代码实际使用的参数下，`searchArea` 的作用是 **1.00×–1.06×**，不是注释暗示的 2.17×。
**结论不变**（保留 `searchArea=title` 无害且语义上更精确），但这正是 FIX-3 订正的那个失效模式
——**数字真、却被搬到测不出它的条件下**——而它在**同一次返工中、隔几行**存活了下来。
建议顺手把括号里的数标注为「该对比是 `q` 语义下的，`qAll` 下实测 1.00×–1.06×」。

### 4.3 记录：dev 报「26 个变异 0 SURVIVED」属实，但那是它自己那 26 个的边界

我的 34 个里有 1 个 SURVIVED，而它不在 dev 的清单里。这不是 dev 报错数——
**「0 SURVIVED」的强度上限永远是写清单那个人的想象力**，这正是验证者独立自写消融矩阵的意义。
（同理，我这 34 个也有它自己的边界。）

### 4.4 首验遗留的三条低危观察，本轮状态

| 首验发现 | 现状 |
|---|---|
| 结果正则「缺日期 ⇒ 静默错配丢条」（MEDIUM） | **已闭合**（FIX-2，N5/N12 KILLED） |
| 阈值 5000 只钉住区间不钉住值（LOW） | **已消失**（上界整个删除） |
| 「绝对 URL」是样本自带、本层不做 `resolveURL` 归一（LOW） | 未变，仍不判缺陷；`note_for_TASK_005` 已提示 index 侧需归一 |

## 5. 越界申报核对

`git diff --stat 62b9243..c661e453 -- <声明的三个 writes>`：
```
internal/hestia/backfill_search.go      | 161 ++++++++++----
internal/hestia/backfill_search_test.go | 285 +++++++++++++++++-----
2 files changed, 376 insertions(+), 70 deletions(-)
```
两个文件**全部在声明的 `writes` 内**（testdata 未动）。同区间另有 `backfill_scan.go` / `backfill_scan_test.go`
的改动，属 **TASK-002**（commit `8322023`），不在本任务范围，**非越界**。

## 6. discovery 与接口登记

`discoveries/TASK-004.json` 已登记签名变更：`parseBackfillSearchPage(html []byte, page int)`（新增 `page`
参数，供末页算术）、新增 `checkBackfillSearchDateRange` 与 `backfillSearchCheckCount`。
`discovery` 指针字段在任务 JSON 中已存在（dev 在转 `dev_done` **之前**写的），validator 无 `missing-discovery`。

## 7. 复验结论

**VERIFIED。** 8 条 done_criteria 在新树上逐条成立（`boundary[1]` 按显式裁决由判性质的守卫取代，
裁决的三条承重实测我全部实网复现）；34 个消融 KILLED 33；覆盖率背对背可复现地上升；
无越界申报；判定对象与 `verify_baseline` 零漂移。

唯一的 SURVIVED（§4.1）是**测试缺口不是实现缺陷**，且所在分支本 sprint 不可达 ⇒ 不构成 rejected 理由，
但已写明闭合成本（一行）与必须闭合的时点（有人加 `--to` 之前）。

---

# 第二部分 · 附录：首验报告全文（第一轮，判定锚 `62b9243`）

> 以下为返工前那一轮的原始报告，逐字保留。其中 §4.1 的 advtime 发现即本次返工的立项理由。

- **验证者**：test-agent-28（Reality Checker）
- **判定**：**VERIFIED**（8 条 done_criteria 全部覆盖且经 21 个消融证明有鉴别力；每个红都核到**具体哪条断言**，见 §2.3）
- **判定锚点**：`62b924300415fb109e7355aea0785b4c1ab4903b`（= `verify_baseline.head`，**零漂移**：声明范围内三个文件自基线起 diff 为空，`discovery_sha256` 逐字一致 `13b68be6…`）
- **验证工作区**：`/Users/zuowei/workspace/go/src/github.com/newthinker/wt-verify-T004`（detached @ 62b9243，收尾已 remove）
- **⚠️ 附带一条 DoD 之外的 CRITICAL 发现**（boundary[1] 的守卫对它自己命名的失效场景实测不触发），见 §4.1 —— **不构成本任务的 reject 理由**（实现完全符合 DoD 字面），但需要 Leader 决定后续处置。

---

## 1. 完成标准覆盖矩阵

| # | 完成标准 | 对应测试 | 消融（我自己跑的，非采信 dev） | 判定 |
|---|---|---|---|---|
| functional[0] | 查询 URL 逐参数含全部实测参数 + 中文正确编码 | `TestBackfillSearchURLCarriesMeasuredParams`（`url.ParseQuery` 逐参数 9 项 `assert.Equal`，并断言 `q`/`advepq`/`adveq` 三个都不出现）、`TestBackfillSearchURLEncodesChineseKeyword`（断言 percent-encoding 字面量 + 遍历全 URL 断言无 rune ≥ 0x80） | M1 `qAll→q` / M2 去 `searchArea` / M3 去 `advtime` / M4 `pNo` 写死 1 / M5 中文不编码 —— **5 个全 KILLED** | **PASS** |
| functional[1] | 取绝对 URL/标题/发布日期三样；标题剥 `<font>` 且不含 `<` | `TestParseBackfillSearchPageExtractsFields`（第一条整个 struct 逐字 `assert.Equal`；12 条逐条 `NotContains "<"` + `Published` 非空 + `HasPrefix "https://"`）+ `TestParseBackfillSearchPageStripsHighlightIsNotVacuous`（断言样本原文含 60 个 `<font color='#FF0000'>` = 12×5，防空集平凡为真） | M6 不剥标签 / M7 日期漏月份段 —— **2 个全 KILLED** | **PASS** |
| functional[2] | 只保留 `/goutongjiaoliu/113456/113469/` 前缀；某标题出现两次，筛后剩一条且保留 goutongjiaoliu 那条 | `TestFilterBackfillSearchHitsKeepsOnlyGoutongjiaoliu`（筛前 `require` 该标题计数 == 2，筛后 == 1，且逐字比对 URL 与 ArticleID）+ `TestFilterBackfillSearchHitsDropsByPrefixNotByIDShape`（**先 `require` 两条反例确实在样本里，再断言都被丢**） | M8 **照 DoD 字面改成按 32 位 hex 筛** / M9 全放行 —— **2 个全 KILLED**；M8 的红精确指向 19 位数字那条 | **PASS**（DoD 文本有误，实现正确，见 §3.1） |
| functional[3] | 总页数从 `default-result-total-pages` 解析 | `TestParseBackfillSearchPageReadsTotalPages`（== 12）+ `TestParseBackfillSearchPageRejectsMissingTotals`（2 子测试：任一计数字段缺失都报错） | M10 总页数正则读成总条数 / M11 计数缺失静默返 0 —— **2 个全 KILLED** | **PASS** |
| boundary[0] | 0 条结果 ⇒ 报错 | `TestParseBackfillSearchPageRejectsZeroResults`（断言错误含 `"0 results"`）+ **反向** `TestParseBackfillSearchPageAcceptsPageWithNoKeptColumn`（解析到 2 条但全是调统司 ⇒ 不报错、筛后为空） | M12 去掉 0 条守卫 —— **KILLED** | **PASS** |
| boundary[1] | 总条数骤增到全站量级 ⇒ 报错，信息指向 advtime 失效；阈值有依据 | `TestParseBackfillSearchPageRejectsSiteWideTotal`（549141 ⇒ 报错，断言错误文本含 `"advtime"` **且**含 `"549141"`）+ **反向** `TestParseBackfillSearchPageAcceptsLargestMeasuredTotal`（692 ⇒ 放行） | M13 阈值抬到 999999999 / M14 阈值压到 1 —— **两端各 KILLED** | **PASS（DoD 字面）**，但守卫**实效性**存疑，见 §4.1 |
| error_handling[0] | Fetcher 错误 / HTTP 非 200 原样上抛，本层不吞不降级 | `TestFetchBackfillSearchPagePropagatesFetchError`（`assert.ErrorIs` 到哨兵 `errBoom`）+ `TestFetchBackfillSearchPagePropagatesParseError`（另一条返回路径） | M15 fetch 错误吞成 nil / M16 parse 错误吞成 nil —— **2 个全 KILLED** | **PASS** |
| non_functional[0] | `gofmt -l` 空、`go vet` 空、`go test -count=1` 全绿，不降低既有覆盖率 | 见 §2 | — | **PASS** |

**DoD 之外、dev 自加的两条守卫也确认有测试钉住**（超出 DoD 不是缺陷）：

- 保留栏目取不出 article_id ⇒ 报错 → `TestParseBackfillSearchPageRejectsIDlessKeptURL`（2 子测试）；消融 M17 去掉守卫 → **KILLED**。
- 计数字段 Atoi 溢出 ⇒ 报错 → `TestParseBackfillSearchPageRejectsOverflowingCount`（断言错误含 `"bad total-records"`）；消融 M18 → **KILLED**。

---

## 2. 我自己跑出来的证据

全部采自 `62b9243`，在我的独立 worktree（`GOTOOLCHAIN=local`）。

### 2.1 套件

```
go vet ./internal/hestia/           → 空（exit 0）
gofmt -l internal/hestia/           → 空
go test ./internal/hestia/ -count=1 → ok  0.865s
```

`-v` 全包统计：**顶层 PASS 365 条 / 子测试 PASS 370 条 / 真实 FAIL 0**
（输出里 2 处 `FAIL` 字样出自测试名 `TestSaveRejectsUnknownCheckStatus/status=FAILED`，均为 PASS 行，非失败。）

本任务用例：**21 条 RUN = 17 顶层 + 4 子测试，全部 PASS**。

### 2.2 覆盖率（背对背，同一 worktree、同一时刻、A/B/A′ 三采）

| 采样 | 内容 | 覆盖率 |
|---|---|---|
| A | 两个新 go 文件移出 package | 93.4% |
| B | 移回 | **93.7%** |
| A′ | 再移出（复采，验可复现） | 93.4% |

A′ == A ⇒ 采样可复现，**覆盖率是升的（+0.3pp）**，「不降低既有覆盖率」成立。
（dev 报的 93.6%→94.0% 是它自己那棵树上的数字，与本次不同源，未横向比。）

新文件逐函数覆盖率（`go tool cover -func`）：**6 个函数全部 100.0%**
（`backfillSearchURL` / `parseBackfillSearchPage` / `filterBackfillSearchHits` / `fetchBackfillSearchPage` / `backfillSearchCount` / `backfillSearchArticleID`）。

### 2.3 消融实验（21 个，我自己的 harness，未采信 dev 的 16 个）

harness 纪律：作用在我独占的 detached worktree 上；每个变异体落盘后先过 `go build` **有效性闸**（编译不过 ⇒ 记 INVALID、结论作废，不计 KILLED）；打印与原文的 diff 逐字核对；只跑 anchored `-run '^TestXxx$'` 避免兄弟用例污染退出码；跑完还原并校验 sha256。

**结果：KILLED 21 / SURVIVED 0 / INVALID 0**

| # | 变异 | 目标用例 | 结果 |
|---|---|---|---|
| M1 | `qAll` → `q`（分词 OR） | URLCarriesMeasuredParams | KILLED |
| M2 | 去掉 `searchArea=title` | URLCarriesMeasuredParams | KILLED |
| M3 | 去掉 `advtime=5` | URLCarriesMeasuredParams | KILLED |
| M4 | `pNo` 写死 1 | URLCarriesMeasuredParams | KILLED |
| M5 | 中文不编码（手工拼回原文） | URLEncodesChineseKeyword | KILLED |
| M6 | 标题不剥 `<font>` | ExtractsFields | KILLED |
| M7 | 发布日期漏掉月份段 | ExtractsFields | KILLED |
| M8 | **栏目筛改成按 32 位 hex 判（DoD 字面）** | **DropsByPrefixNotByIDShape** | **KILLED** |
| M9 | 栏目筛全放行 | KeepsOnlyGoutongjiaoliu | KILLED |
| M10 | 总页数正则读成总条数 | ReadsTotalPages | KILLED |
| M11 | 计数字段缺失静默返 0 | RejectsMissingTotals（2 子测试全红） | KILLED |
| M12 | 去掉 0 条守卫 | RejectsZeroResults | KILLED |
| M13 | 上界抬到 999999999 | RejectsSiteWideTotal | KILLED |
| M14 | 上界压到 1（反向） | AcceptsLargestMeasuredTotal | KILLED |
| M15 | Fetcher 错误吞成 nil | PropagatesFetchError | KILLED |
| M16 | parse 错误在 fetch 层被吞 | PropagatesParseError | KILLED |
| M17 | 保留栏目空 id 守卫去掉 | RejectsIDlessKeptURL | KILLED |
| M18 | Atoi 溢出静默当 0 | RejectsOverflowingCount | KILLED |
| M19 | **错误文本不再提 `advtime`**（守卫仍报错） | RejectsSiteWideTotal | KILLED |
| M20 | **错误文本不再带实际条数**（守卫仍报错） | RejectsSiteWideTotal | KILLED |
| M21 | **fetch 错误被替换而非原样上抛**（仍返回 error） | PropagatesFetchError | KILLED |

**M19–M21 是补做的，理由是「判据是哪条红，不是红不红」**：M13（阈值抬高）与 M15（错误吞成 nil）
让测试变红时，红的都是 `require.Error`——而 `require` 会**中止后续断言**，于是
`assert.Contains(err, "advtime")` / `assert.Contains(err, "549141")` / `assert.ErrorIs(err, errBoom)`
这三条**根本没跑到**。也就是说 M13/M15 只证明了「会产生一个错误」，没有证明
「错误内容指向 advtime」「错误带上实际条数」「错误是**原样**上抛而非被替换」——
而后三者恰恰是 DoD `boundary[1]`（「错误信息指向 advtime 过滤可能已失效」）与
`error_handling[0]`（「**原样**返回错误」）的字面要求。补做后三条红分别落在：

```
M19 → "…the date filter likely stopped working…" does not contain "advtime"
      Messages: 错误信息必须指向 advtime 过滤失效
M20 → "…too many results, over the 5000 ceiling…" does not contain "549141"
      Messages: 错误信息必须带上实际条数
M21 → Target error should be in err chain:        ← assert.ErrorIs，证明「原样」而非仅「有错」
```

⇒ `boundary[1]` 与 `error_handling[0]` 的**全部**字面要求现在都有定向消融背书。

**还原核实**：`backfill_search.go` 收尾 sha256 == 基线 `4cf55fca5b83a624b83834a13b7218263d8c44ac5f7ce090c3211ca627d62226`；`git status --porcelain` 空。

### 2.4 样本 HTML 独立核实（用 python3，未用裸 `sort -u` / `uniq`）

`internal/hestia/testdata/pboc-search-p1.html`：32372 字节 / CR = 0（LF）。

| 事实 | dev 声称 | 我实测 |
|---|---|---|
| total-records / total-pages | 137 / 12 | **137 / 12** ✓ |
| 实现正则匹配出的条目数 | 12 | **12** ✓ |
| 栏目分布 | 6 goutongjiaoliu + 6 diaochatongjisi | **6 + 6** ✓ |
| `<font color='#FF0000'>` 个数 | 60（12×5） | **60** ✓ |
| diaochatongjisi 里 32 位 hex 的条数 | 2 | **2** ✓ |
| diaochatongjisi 里 19 位纯数字的条数 | 4 | **4** ✓ |
| goutongjiaoliu 侧 id 形态 | 与之撞形 | **1×7 位 + 5×19 位 ⇒ 19 位形态两侧都有** ✓ |
| 12 条 URL 是否绝对 | 是 | **12/12 以 `https://www.pbc.gov.cn/` 开头** ✓ |

日期序列 `2025-09-12 / 08-13 / 07-14 / 06-13 / 05-14 / 04-13` 逐对成双且严格递减 ⇒ 正则的「条目↔日期」配对在样本上是对的，且 `sr=dateTime desc` 确实生效。

### 2.5 实网校验（DoD 之外的加强证据，共 7 次请求）

把**实现真正构造出的那条 URL** 打印出来直接打过去：

```
https://wzdig.pbc.gov.cn/search/pcRender?advSearch=true&advtime=5&endTime=2026-08-14&pNo=1
&pageId=c177a85bd02b4114bebebd210809f691&qAll=%E7%A4%BE%E4%BC%9A...&searchArea=title
&sr=dateTime+desc&startTime=2020-01-01
```

**HTTP 200，32405 字节**，解析出 total-records=137 / total-pages=12 / 12 条 —— 与入库样本**逐项一致**。
与入库样本（CR 归一后）逐行比对：460 行里**只有 3 行不同**，全是 CSS/JS 的 cache-buster token（`?time=…`），数据内容一字不差 ⇒ **样本是忠实的真实抓取，未经修饰**。

顺带证实：`sr` 的空格用 `+`（`url.Values.Encode` 的输出）在实网上**照常工作**，dev「别改回手工拼接」的结论成立。

---

## 3. 两处「DoD 文本错、实现对」的核实

### 3.1 functional[2] 的「32 位 hex」是错的判据 —— 实现按前缀筛是对的

我独立数过样本：6 条 `/diaochatongjisi/` 里**只有 2 条**是 32 位 hex，另 **4 条是 19 位纯数字**（`2025080618505078072` 等）；而 goutongjiaoliu 侧 6 条里有 **5 条也是 19 位纯数字** ⇒ **两侧形态完全重叠，id 形态根本不构成判据**。

消融 M8 把筛法照 DoD 字面改成「留 32 位 hex 之外的」，`TestFilterBackfillSearchHitsDropsByPrefixNotByIDShape` **红**，红在：

```
Should be false | Messages: 19 位数字的调统司那条同样该被丢
"…/diaochatongjisi/116219/116225/2025080618505078072/index.html" should not contain "/diaochatongjisi/"
```

**该用例不是平凡通过**：它在断言之前先 `require.True(containsURL(hits, hexURL))` 与 `require.True(containsURL(hits, digitDropURL))`，两条反例任一从样本里消失都会**先**红并打出「样本前提变了」。先证反例存在、再断言行为 —— 符合要求。

### 3.2 翻页循环不在本文件 —— 确认是正确解读，不判缺陷

`fetchBackfillSearchPage` 返回 `totalPages` 供调用方决定翻到第几页，与 DoD `functional[3]` 字面一致。按 Leader 已订正的 §1 措辞，**不作为缺陷**。

---

## 4. DoD 之外的发现（不影响本次判定，但请 Leader 处置）

### 4.1 CRITICAL — boundary[1] 的守卫，对它自己命名的失效场景实测不会触发

DoD 与代码注释都写：「`advtime` 失效时返回的是**全站量级**（实测 549141 条）」，据此设 5000 上界。
**我实测了「advtime 失效」这件事本身**（同关键词、同 `searchArea=title`，只动 advtime）：

| 场景 | 实测 total-records | 5000 上界会报错吗 |
|---|---|---|
| A 现状 `advtime=5` + 日期范围 | 137 | 否（正确） |
| **B `advtime` 参数被丢弃**（后端不再接受它） | **240** | **否 ⇒ 静默放行** |
| **C `advtime=0`（值不再被认）** | **240** | **否 ⇒ 静默放行** |
| D 窄窗口对照 `startTime=2025-01-01` | 17 | 否（正确，证明日期范围确实生效） |
| F 下界锚 `金融统计数据报告` 全区间 | **692** ✓ 与 DoD 给的数字逐字相符 | 否（正确） |
| G 同上但**无日期过滤**（= 该关键词 advtime 失效后的量级） | **1136** | **否 ⇒ 静默放行** |
| E 关键词整个丢失（无 `qAll`） | 页面**根本没有** `total-records` 字段 | 由「计数字段缺失 ⇒ 报错」那条守卫拦下（另一条） |

**结论**：`advtime` 失效时，本站返回的是「该关键词的全部历史结果」（240 / 1136），**不是**全站量级。三个关键词里最大的 1136 距 5000 还差 4.4 倍 ⇒ **5000 这个上界永远不会因 advtime 失效而触发**，而它的错误文本恰恰写着 `the advtime date filter likely stopped working`。

`549141` 是**空查询**（`advepq` / `adveq` 单用、无关键词）的返回值，属于**另一种**失效模式；而那种模式实测连 `total-records` 字段都取不到，会被别的守卫拦下。

**归属**：这是 **DoD 前提的事实错误**（Leader 侧、源自 requirements-analysis），dev 完全按 DoD 字面实现且做了两端用例 ⇒ **不是 `task_defect`，我不据此 reject**。

**影响**：advtime 若失效，搜索侧候选会从 137 静默涨到 240（多出的是 2020 年以前的报告），TASK-005 的交叉校验会多出约 100 条「搜索有、index 没有」的假信号，而**没有任何告警** —— 正是这条守卫立项时要防的那种「看起来完全正常，只是多得多」。

**可行的替代守卫（供 Leader 决策，一行成本）**：不看总条数量级，改为**断言解析出的每条 `Published` 都落在 `[from, to]` 内** —— 它直接检测「日期过滤没生效」，与量级无关，对上表 B / C 两种场景都立即报错。

**路径**：`verified → review_fix`（leader 专属边）仍然开着，本次判 VERIFIED 不封死任何后路。

### 4.2 MEDIUM — 结果正则在「某条目缺日期」时会静默错配并丢条

`backfillSearchItemRE` 是 `<h3>…<a href=X>T</a>.*?<span>日期</span>`，非贪婪。我用同语义的正则做最小复现：条目 A 缺 `<span>日期</span>`、条目 B 正常时，**2 条只匹配出 1 条**，且那 1 条是「A 的 URL / 标题 + **B 的日期**」—— B 整条消失，A 的日期是错的，**全程无错误**。

样本 12/12 都有日期，当前不触发；`0 条 ⇒ 报错` 守卫也看不见这种**部分**丢失。建议 TASK-005/006 加一条「本页解析条数与 `total-records` / 页容量对得上」的一致性检查（与 index 侧 B2「相邻页日期连续性」同族）。**超出本任务 DoD，不判缺陷。**

### 4.3 LOW — 阈值 5000 实际被钉住的是区间，不是这个值

我实测把阈值改成 100000 / 700 / 693，`RejectsSiteWideTotal` 与 `AcceptsLargestMeasuredTotal` 两条**都不红**（SURVIVED）。两条用例把阈值钉在开区间 `(692, 549141]` 内，不是钉在 5000。这是双锚设计的固有限度、且 DoD 只要求「阈值自己定并写清依据」，**不判缺陷**；如实记录，以免后人误以为 5000 这个值被守着。

### 4.4 LOW — 「q 比 qAll 多 25 倍」这个理由数字不可复现（结论仍成立）

代码注释与 DoD 都写「实测同一关键词、同为 `searchArea=title`，`q`=1324 / `qAll`=24，差 25 倍」。我做**同参数对照**实测：

| 窗口（均 `searchArea=title`） | `qAll` | `q` | 比值 |
|---|---|---|---|
| 2020 全年 | 24 | 50 | 2.1× |
| 2020-01-01..2026-08-14（`advtime=5`） | 137 | 276 | 2.0× |
| 无日期过滤 | 240 | **610** | 2.5× |

`1324` 实为 **`q` + 搜正文**（`searchArea` 取默认）的数字 —— 代码里另一处注释「默认搜正文，同一关键词 1324 条 vs 610 条」是**对的**，我实测 `q` + title + 无日期过滤 = **610**，逐字吻合。问题只在「1324 vs 24」这个对比同时混了 **searchArea、q/qAll、日期窗口三个变量**，把 2× 说成了 25×。

**结论不受影响**：`qAll` 在所有条件下都严格窄于 `q`，用 `qAll` 是对的，M1 消融也 KILLED。仅理由数字失真，建议顺手订正注释与 DoD 文本。

### 4.5 LOW — 「绝对 URL」是样本自带的，本层不做 resolveURL 归一

实现把 href 原样透传，样本 12/12 本就是绝对 URL，`HasPrefix "https://"` 断言在样本上成立。若站点改版给出相对 href，本层不会归一（包内 `resolveURL` 在 `internal/hestia/discover.go:70` 可用）。DoD `functional[1]` 的判据锚在样本上，**不判缺陷**；dev 已在 `note_for_TASK_005` 里提示 index 侧需 `resolveURL`。

### 4.6 LOW — 溢出守卫的注释理由与 Go 的实际行为不符（守卫本身有效）

注释写「不判的话 Atoi 返回 0 + err，静默当成『0 条』往下走」。实际 `strconv.Atoi` 溢出时返回 **`MaxInt64`** + `ErrRange`（我的 M18 消融输出印证：`9223372036854775807 results exceed the 5000 ceiling`）。所以对 `total-records` 而言，去掉该守卫仍会被上界拦下（只是错误文本变了）；**守卫的真正价值在 `total-pages`** —— 那条路径去掉守卫会静默变成 9.2e18 页，而它**没有用例覆盖**。属于超出 DoD 的守卫，**不判缺陷**，建议顺手订正注释。

---

## 5. 越界申报核对

`git diff --stat 62b9243^1..62b9243`：

```
internal/hestia/backfill_search.go           | 229 +
internal/hestia/backfill_search_test.go      | 390 +
internal/hestia/testdata/pboc-search-p1.html | 460 +
3 files changed, 1079 insertions(+)
```

三个文件**全部在声明的 `writes` 内**，纯新增 0 删除，与 dev 报的 `numstat` 逐字相符。**无越界申报。**

包内复用的 `tagRE`（discover.go:198）/ `readTestdata`（discover_test.go:19）/ `fakeFetcher`（discover_test.go:611）/ `errBoom`（ingest_test.go:383）均为既有符号，无重复定义、无符号冲突（`go build` + `go vet` 均空）。

---

## 6. 结论

**VERIFIED。** 8 条 done_criteria 逐条有对应测试、逐条经我自己的消融证明「改坏它会有东西变红」（21/21 KILLED，0 SURVIVED，0 INVALID，还原 sha256 一致）；覆盖率背对背可复现地上升（93.4% → 93.7%，新增 6 个函数全 100%）；无越界申报；判定对象与 `verify_baseline` 零漂移。

**移交给 Leader 的一件事**：§4.1 的 advtime 守卫实效性问题是本次验证最有价值的产出。它**不属于本任务 DoD 范围内的缺陷**（DoD 怎么写的、实现就怎么做的，且做对了），但会让 TASK-005 的交叉校验在一种真实失效模式下静默失准。请决定是走 `verified → review_fix` 在本任务内补一条「`Published` 落在 `[from, to]` 内」的断言，还是登记为 TASK-005 的新 DoD。
