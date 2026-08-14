# Sprint M1c-1 · 第一轮 Code Review（常规审查）

<!-- 审查者: qa-m1c1 | 2026-08-15 | 审查对象树: 768064f5868274687dc3a5d598b49068a6587cd3 -->

## 0. 判定

**CONTESTED** —— 2 条 CRITICAL 必须返工，5 条 WARNING 中 3 条建议返工、2 条登记即可。

不判 REJECT 的理由：两条 CRITICAL 都**不是**「实现算错了」，而是「守卫装在了没人走的那条路上」
与「守卫的判据覆盖不到它自己声称覆盖的形态」，两者的修复各约 1–3 行，且现有 1133 条测试、
真跑 452 文件产物、逐条消融的价值一个都不作废。
不判 PASS 的理由：其中一条 CRITICAL 直接**证伪了 CONTRACTS 里一句写给下游的普遍命题**，
而 M1c-2 被明确要求依赖那份 CONTRACTS。

## 1. 审查基线（全部自采于同一棵树，采样时点晚于最后一次改动）

| 项 | 值 | 怎么来的 |
|---|---|---|
| 审查对象 | `768064f5868274687dc3a5d598b49068a6587cd3` | `git rev-parse HEAD` |
| 测试 | **1133 条 RUN / 1133 PASS / 0 FAIL**。口径：`internal/hestia` + `cmd/atlas` 两包 `go test -count=1 -v` 的 `=== RUN` 行数，**含子测试**（不是 1133 个顶层用例） | 跑出来的 |
| 仅 `internal/hestia` 包 | **851 条 RUN / 0 红** | 跑出来的；与 CONTRACTS C1 追加声称的「851 全绿」逐字相符 |
| `go vet ./internal/hestia/ ./cmd/atlas/` | exit 0，无输出 | 跑出来的 |
| `gofmt -l` | 只报 `cmd/atlas/backtest_test.go`、`cmd/atlas/crisis_test.go` | 跑出来的；`37388df` 上即如此，非本 sprint 引入 |
| CONTRACTS M1c-1 节内条目 | **20**（A1-3 B1-2 C1-3 D1-8 E1 F1 G1-2） | 跑出来的，用节内 awk 判据 |

消融一律在隔离 worktree（`scratchpad/wt-qa-m1c1`，detached @ 768064f）里做。
收尾自证：worktree `git status --porcelain` 为空；主工作区 `backfill_fetch.go` sha256
`4724986a30aa5f70…` 与消融前逐字节相同。

---

## 2. CRITICAL（必须返工）

### C-1｜搜索侧的日期区间守卫**装在没有生产调用方的那个函数上**；删掉生产路径那道校验，1133 条测试红 0 条

**位置**：`internal/hestia/backfill_fetch.go:316`（生产路径调用点）、
`internal/hestia/backfill_search.go:293`（`fetchBackfillSearchPage`，零生产调用方）、
`internal/hestia/backfill_search_test.go:448`（顺序断言，钉的是死的那个）

**事实（全部跑出来的）**：

| 消融 | 红条数 |
|---|---|
| M1：把 `fetchBackfillSearchAll` 里的区间校验**移到栏目筛之后**（注释明写不许） | **0（SURVIVED）** |
| M2：把 `fetchBackfillSearchAll` 里的区间校验**整条删掉** | **0（SURVIVED）** |

🔴 **最干净的表述是把两个调用点并排跑（同一道守卫，覆盖完全相反）**——攻击者视角提出，
我复跑确认（基线 0 红）：

| 变异（各只删一处，同一道 `checkBackfillSearchDateRange` 调用） | 红 | 谁杀的 |
|---|---|---|
| **A：删生产路径** `backfill_fetch.go:316` | **0** | 无 |
| **B：删副本** `backfill_search.go:304` | **2** | `TestFetchBackfillSearchPageRejectsOutOfRangePublished`、`TestFetchBackfillSearchPageChecksAllColumnsDates` |

⇒ 这道守卫**被测得很好**——只是测在了不上线的那一份上。
`backfill_search_test.go:434` 的用例名与注释逐字写着「区间校验发生在**栏目筛之前**」，
而生产路径上这件事红 0。**修法必须是消掉复制，不是再补一条用例**（后者又要靠人维护同步）。

调用方统计（搜的空间：`internal/hestia/*.go` 与 `internal/hestia/*_test.go` 全部文件的字面量引用）：

- `fetchBackfillSearchPage`：**生产调用方 0 个**，测试引用 **6 处**；
  `backfill_fetch.go:294` 的注释自己写明「这里不用 `fetchBackfillSearchPage`」。
- `fetchBackfillSearchAll`（**唯一**生产入口）：**直接测试引用 0 处**，只被 `runBackfill` 的用例间接走到。
- `checkBackfillSearchDateRange` 函数本身有单测（`backfill_crosscheck_test.go:271/298`）——
  **函数是对的，无背书的是它在生产路径上的位置。**

**失效场景**：站点的 `advtime` 参数哪天失效（`backfill_search.go:254` 声称这是本层认该失效的
**唯一**判据），有人在重构中把这三行挪到 `filterBackfillSearchHits` 之后或直接删掉 ——
交叉校验会拿着区间外的条目算差集，两个差集凭空变化，**没有任何测试会红**。
更直接的场景：`backfill_search_test.go:442` 那条用例的全部意义是「越界条目即使在会被筛掉的栏目里
也要报错」，而生产路径上这件事**从来没有被断言过**。

**建议**（二选一，我倾向前者）：
1. 让 `fetchBackfillSearchAll` **复用** `fetchBackfillSearchPage`，把「顺带存快照」做成参数或回调，
   消掉两份顺序；那 6 条测试立刻变成生产路径的背书。
2. 保留两份，但给 `fetchBackfillSearchAll` **补三条对称用例**（越界条目在被筛掉的栏目里 ⇒ 报错 /
   区间校验在筛之前 / Fetcher 错误上抛），并在两处注释里互指。
**验收判据**：重跑 M1 与 M2，两个消融各自至少红 1 条，且红的是新补的那条（不看红了没有，看哪条红）。

---

### C-2｜协议相对外链（`href="//host/..."`）被判为**站内**，一路静默走到 manifest；CONTRACTS C1 追加那句「正交」被证伪

**位置**：`internal/hestia/backfill_scan.go:108`（`backfillInternalHrefRE = \shref="/"`）、
`internal/hestia/CONTRACTS.md:869`（「「站内 / 站外」这个分界与 href 的**具体形态正交**，所以形态变化仍会告警」）

**事实（跑出来的，端到端）**：喂一页含一条正常站内条目 + 一条
`<a href="//evil.example.com/zhengce/113469/9002/index.html" … title="2020年2月金融统计数据报告" istitle="true">`：

```
resolveURL(base, "//www.gov.cn/zhengce/113469/9002/index.html") = "https://www.gov.cn/zhengce/113469/9002/index.html"
scanBackfillPage 放行：Items=2 Reports=2        ← 不报错
manifest article: id=9002 url=https://evil.example.com/…/index.html file=articles/9002.html 内容="<html>攻击者内容</html>"
```

三道守卫**逐道被绕过**，且成因各不相同：

1. `backfillInternalIstitleCount` 判「href 以 `/` 开头」⇒ `//host/...` **以 `/` 开头** ⇒ 计入站内基准；
2. `backfillItemRE` 的 `/(\d+)/index\.html` 在 `//host/zhengce/113469/9002/index.html` 上**匹配得到**
   ⇒ 条数与基准**相等** ⇒ 计数守卫不报（这正是 p54 那条 gov.cn 绝对 URL **会**报的原因的反面：
   绝对 URL 是 `https://…`，不以 `/` 开头，才被排掉）；
3. `resolveURL` 用 `ResolveReference`，协议相对 URL 按标准**换主机保协议** ⇒ 得到外部主机的绝对 URL；
   `pbocFetcher.Get`（`internal/hestia/fetch.go:46`）**不做任何主机校验**（推出来的：读代码，未发真实请求）。

**这条与 C1 追加是同一条守卫的第二个漏洞**：C1 追加解决的是「绝对 URL 外链」，而
「协议相对外链」同样是站外、同样带全套 `istitle`/`title`/日期属性，**却连硬失败都不会有**——
比 C1 追加要修的那个更糟（那个是 14 vs 15 硬失败，这个是静默产出错数据）。

**当前语料上不触发**：dev 实测 p1..p155 的 2325 条 `istitle` 锚里站外恰好 1 条且是绝对 URL
（**这个数我没有第二条路径复核**，标注为 dev 自陈）。所以这不是一条现存数据缺陷，
是一条**守卫正确性**缺陷，以及一句**已被证伪的契约文字**。

**建议**：
- 代码：`backfillInternalHrefRE` 收成 `\shref="/(?:[^/]|$)`（以 `/` 开头但**下一个字符不是 `/`**），
  一个谓词。补一格用例：协议相对外链 ⇒ 与绝对 URL 外链同样被排出基准 ⇒ 计数不符 ⇒ 报错。
- 文档：`CONTRACTS.md:869` 那句必须改。现在的措辞是**普遍命题**（「形态变化仍会告警」），
  而实际成立的是收窄版：「href 以 `/` 开头**且不以 `//` 开头**」这个分界，与站内 href 的
  **路径形态**正交。M1c-2 会照着这句话推断守卫覆盖面。

---

## 3. WARNING

### W-1｜`failed[]` 跨次累积、不去重、抓成功后不清理；同一 id 会同时出现在 `articles[]` 与 `failed[]`

**位置**：`internal/hestia/backfill_manifest.go:131`（`AppendFailed`）、`backfill_fetch.go:225`（错误文案）

**事实（跑出来的）**：同一篇持续失败跑三轮 ⇒ `failed[]` 长度 = **3**，三条 id 全同；
第四轮抓成功后 ⇒ `articles=2 / failed=3`，`id=9002` **同时**出现在两个数组里。
同时 `runBackfill` 的错误文案「回填完成但有 **N** 篇抓取失败（详见 …failed[]）」里的 N 是
**本轮**计数（=1），与 `failed[]` 的长度（=3）不同口径。

**失效场景**：M1c-2 读 manifest 决定「哪些篇要重试 / 哪些期缺篇是因为抓取失败」，
拿 `failed[]` 直接用会把已经补齐的篇目算成缺失，且重试次数被计成不同篇数。
断点续抓是本设计的核心卖点，而**续抓成功这件事在 manifest 上没有任何痕迹**。

**建议**：`AppendFailed` 按 id 覆盖而非追加（保留最后一次错误 + 加 `attempts` 计数），
且 `AppendArticle` 成功时把同 id 从 `Failed` 里摘掉；或者退一步——只在 `Failed` 里按 id 去重，
并把错误文案改成引用 `len(store.Manifest.Failed)`。无论选哪个，**契约要写明 `failed[]` 的口径
是「当前仍失败」还是「历史失败流水」**——现在两种读法都说得通，而它们互斥。

### W-2｜任一关键词在窗口内 0 条 ⇒ **整条**交叉校验被关掉，且 `search_skipped_reason` 把成因写成「版式变了」

**位置**：`internal/hestia/backfill_search.go:174`（0 条即报错）、`backfill_fetch.go:312`（错误上抛终止整轮搜索）

**事实（跑出来的）**：三个关键词里让第三个返回「站点自报 0 条 0 页、页上无结果块」，得到

```
search_skipped_reason = "search side failed, cross-check skipped: 搜索 \"社会融资规模增量统计数据报告\" 第 1 页:
                         hestia backfill search: 0 results parsed while the page claims 0: the result layout likely changed"
only_in_index=0 only_in_search=0 articles=2
```

**这不是构造出来的边角**：社融存量/增量自 **2025-09 起央行不再发**（本 sprint 自己实测并写进
`backfill_reconcile.go:37`）。⇒ **任何 `--from` 晚于 2025-09 的回填，这两个关键词必然 0 条，
交叉校验必然整轮关闭**。而 M1c-2 若要做增量补抓，这正是它会用的窗口。

三点危害，按严重度排：
1. 一个关键词无结果，**另外两个已经取回的结果也被丢弃**（`fetchBackfillSearchAll` 出错即 `return nil`）；
2. 原因文本**误归因**——把「窗口内没有这种报告」说成「the result layout likely changed」，
   而 `backfill_search.go:206` 那段注释自己论证过「防误归因」正是留两条计数检查的理由；
3. 机制本身是 fail-open 且有声（`search_skipped_reason` 落了盘），所以**不判 CRITICAL**。

**建议**：`parseBackfillSearchPage` 区分 `records == 0`（合法空结果，返回空 + nil）与
`records > 0 但解析出 0 条`（版式变了，报错）。前者是站点自报的事实，不是解析失效。
若嫌改动大，退一步：`fetchBackfillSearchAll` 把单个关键词的失败降级成「该关键词跳过 + 记原因」，
不要让它否决另外两个。

### W-3｜`testdata/README.md` 仍留着被 A2 证伪的那句「没有混合页」

**位置**：`internal/hestia/testdata/README.md:104,106` vs `internal/hestia/CONTRACTS.md:807-810`
vs `internal/hestia/discover.go:167-170`

**事实（跑出来的，三处逐字对照）**：

- README:104 `| **p15 / p16 / p17 / p18** | **7 位 ×15** | **0 条** |`
- README:106 「这 10 页每页 15 条、位数**页内完全一致**，**没有混合页**。」
- CONTRACTS A2「实测 **p15 = 1×19 位 + 14×7 位，p19 亦混合**」
- `discover.go:167` 保留原句，但 **169-170 行紧跟一条订正**。

`discover.go` 订正了，README **没有**。而 A2 的结论句是「凡按此写的判断都要改」。
README 是本包的样本登记处、且 dev 本 sprint 正在编辑它（补了 B1/B2 两节），
**改到了同一个文件却没改这一句**。

**建议**：README:106 后补一句与 `discover.go:169` 同构的订正，注明「该表是 2026-08-12 的观测，
M1c-1（2026-08-14）实测 p15 已混合 —— 齐整是巧合不是规律」。

### W-4｜`--out` 的契约写着「必须是绝对路径 + 仓库外」，代码里零校验

**位置**：`cmd/atlas/hestia.go:125-133`、`internal/hestia/backfill_fetch.go:53`

`MarkFlagRequired("out")` 只保证非空。`--out out/` 或 `--out .` 会把 452 个文件写进仓库工作区，
而 `BackfillConfig` 的文档注释把「仓库外的绝对路径」写成了**必须**，理由列了三条。
`hestia.go:131` 那句「让「误落进仓库」需要显式打出来才会发生」是**当前唯一的防线**，
它防的是「忘了写」，防不住「写错了」。

这正是本 sprint 反复登记的形状（「注释里写着意图、删掉却无一变红」）：
把这行注释删掉，**没有任何测试会红**（推出来的：`cmd/atlas/hestia_test.go` 里搜不到对 `--out`
取值的断言；搜的空间是 `cmd/atlas/*_test.go`）。

**建议**：`runHestiaBackfillFetch` 里加一句 `filepath.IsAbs` 校验 + 一句「不得在仓库工作区内」
（与 `--from` 的校验同位置、同风格，都是「触网前的本地判断」）。

### W-5｜`BackfillConfig.Cutover` 是**导出**字段，非法值静默改变每一个期次的规则判定

**位置**：`internal/hestia/backfill_reconcile.go:215`（`p >= cutover` 字典序比较）、`backfill_fetch.go:75`

**事实（跑出来的）**，输入 2025-09/10/11 三期各一篇《金融统计数据报告》：

| `cutover` | 判出的规则 | 缺篇期次 | Violations |
|---|---|---|---|
| `"2025-09"` | 三期全 rule@v2 | 无 | 0 |
| `"2025-9"`（漏补零） | 三期全 **rule@v1** | **2025-09, 2025-10, 2025-11** | 0 |
| `"garbage"` | 三期全 **rule@v1** | **同上三条假缺篇** | 0 |
| `"0000-00"` | 三期全 rule@v2 | 无 | 0 |

**没有任何报错或告警**，退出码 0，报告里凭空多出三条「缺篇」。
CLI 目前不暴露该 flag（`hestiaBackfillConfig` 刻意留空，注释也说明了理由），
所以**当前不可达**；但它在导出 API 面上，M1c-2 直接构造 `BackfillConfig` 就会踩。

**建议**：`reconcileBackfill` 入口校验 cutover 形态（`^\d{4}-(0[1-9]|1[0-2])$`，与
`hestiaBackfillFromRE` 同一把尺），不符则进 `Violations` 或直接 panic（编程错误）。
**不要**只在 CLI 层校验——那会造出第二个定义处，正是本文件反复反对的。

---

## 4. SUGGESTION

| # | 位置 | 内容 |
|---|---|---|
| S-1 | `backfill_fetch.go:302` | 搜索侧翻页**无上限**：`total = totalPages` 直接来自站点自报，`backfillSearchCount` 只校验 `n >= 0`。站点返回 `total-pages=999999` 时按 1 req/s 会跑一天多（ctx 取消是唯一出口）。index 侧有 `backfillMaxPages` 且翻满即报错，两侧不对称。判**性质**的做法：`totalPages` 超过 `records/pageSize + 1` 即自相矛盾，可当版式失效报错——不依赖任何量级常量。 |
| S-2 | `backfill_reconcile.go:79` | `backfillReconcileItemsFromCandidates` **零生产调用方**（搜的空间：全仓 `*.go`），而 `backfill_reconcile.go:70-72` 的注释声称对账「在两个位置都有意义」并据此说明为什么不直接吃 `backfillCandidate`。实际只有 `FromArticles` 那一个位置存在。要么接上抓取前那次对账（注释说它能「在花掉 250 次请求前就发现缺口」，是真价值），要么把注释改成事实。 |
| S-3 | `backfill_manifest.go:196` | `articleFile(id)` 对 id 无任何约束，`path.Join("articles", "../../etc/passwd"+".html")` ⇒ `../etc/passwd.html`，**逃出 `--out` 一层**（跑出来的）。当前**两个**调用点结构性免疫（搜的空间是全部生产调用点，共 2 处：`backfillItemRE` 第 2 组是 `\d+`；`backfillSearchIDRE` 第 1 组是 `[^/]+`，字面量排除 `/`，`..` 拼上 `.html` 得 `...html` 不构成上跳）。⇒ **现在不可达**，但这条免疫依赖两条正则的字符集，而本 sprint 的核心教训之一正是「任何位数/形态假设都是对下一次站点重建的猜测」。建议在 `articleFile` 里加一句 `if strings.ContainsAny(id, "/\\") || id == "." || id == ".."` 的守卫。 |
| S-4 | `backfill_crosscheck.go:18` | 文件头注释写「搜索侧 `fetchBackfillSearchPage`」——那是非生产路径（见 C-1）。 |
| S-5 | `backfill_fetch.go:157` | `cfg.From.Format("2006-01")` 用字面量，而同包已有 `backfillDateLayout` 常量的先例。期次形态字符串在本包出现 4 处（`"2006-01"` ×3、`"2006-01-02"` ×2），建议各收一个常量——W-5 那个坑的根源之一就是「期次形态」没有单一定义处。 |
| S-6 | `backfill_crosscheck.go:127` | fail-open 路径的 `taggedCandidates` 不做 id 去重，而正常路径显式做了并注明「上游已去重，这里只是不让重复穿透到 manifest」。两条路径的防御不对称；当前无害（`HasArticle` 会兜住），但那句注释的理由对 fail-open 路径同样成立。 |

---

## 5. 对「本 sprint 未跑 code-simplifier」的评估（Leader 点名要我判）

**结论：这个决定的两条理由里，一条不成立、一条成立但被高估；而它确实挡住了东西。**

**理由一「改代码会作废 `verify_baseline`」——不成立。**
`verify_baseline` 约束的是 `verifying → verified` 这一条边（CLAUDE.md 出边表）。
9 个任务已全部 `verified`，**没有任何任务还在那条边上**。此刻改代码走的是
`verified → review_fix → in_progress` 这条正常返工路径，与 `verify_baseline` 无关。
⇒ 这条理由**引用了一个不适用于当前状态的机制**。

**理由二「会作废全部消融结论」——成立，但被高估。**
消融结论是**按文件/按守卫**绑定的，不是全局的。code-simplifier 若只动了
`backfill_manifest.go`，`backfill_scan.go` 那三条消融一个字都不用重跑。
而且我刚刚**用第二条路径把 C1 追加那三条消融全部重跑并复现**（见第 6 节），
成本是三次 `go test`，不到两分钟。⇒ 「重跑消融」的实际代价远低于这条理由暗示的量级。

**它挡住了什么（这是实质部分）**：本轮我发现的
**C-1（`fetchBackfillSearchPage` 零生产调用方）** 与 **S-2（`backfillReconcileItemsFromCandidates` 零生产调用方）**
都是 code-simplifier 的典型命中项。其中 C-1 **不是可读性问题** ——
它是「6 条测试钉在一个没人调用的函数上，而生产路径那道守卫删掉红 0 条」。
⇒ **跳过简化这一步，直接掩护了一条 CRITICAL。**

另外，全局 CLAUDE.md 写的是「在 git commit **之前**必须先运行 code-simplifier」。
本 sprint 的代码已经提交（`768064f` 及其前的 13 个提交），⇒ 无论理由是否成立，
**那条规范在这些提交上没有被遵守**，这是事实层面的偏离，需要 Leader 与人类知情。

**建议**：返工 C-1/C-2 时，对 6 个 `backfill_*.go` 跑一次 code-simplifier，
**只接受它对死代码与重复顺序的处置**，其余一律不动（避免大面积改动作废消融）。

---

## 6. 我复核过的、结论**成立**的部分（登记，不返工）

这一节是正向证据，用第二条产生路径复核 dev 自陈的数字。

| 被复核项 | dev 自陈 | 我的独立复现 | 结论 |
|---|---|---|---|
| C1 追加消融表第 1 行「基准回退成全部 istitle 锚」 | 红 2，只有第一格杀 | **红 2**，killer = `…ExcludesExternalLinks/站外条目不计入基准` | ✅ 逐条相符 |
| 第 3 行「基准改松成 n == n」 | 红 6，第二、三格 + 既有两格 | **红 6**，killer = 部分失效 / istitle 被换掉 / 共存场景 / href 形态 | ✅ 逐条相符 |
| 第 4 行「基准改成 href 形如文章页」 | 补第三格后红 2，只有第三格杀 | **红 2**，killer 只有 `站内条目的 href 形态变了` | ✅ 逐条相符 |
| 「基线 851 全绿」 | 851 | **851 RUN / 0 红** | ✅ |
| A2「discover.go 只改注释、代码零改动」 | git diff 已核 | `git show --numstat d662135 -- discover.go` = **8 增 2 删**，且滤掉注释与空行后**非注释改动行为 0** | ✅ 比消融更强的直接证据 |
| C3 挂账项「第二格同时钉住 A16，而测试里无痕迹」 | `grep -c errBackfillDisk` = 0 | **= 0**；消融删掉 `runBackfill:151` 的 `errors.Is` 判断 ⇒ **红 2**，killer 正是 `TestBackfillFetchAbortsOnEveryDiskFailurePath/第二步：检索页快照落盘失败` | ✅ 挂账描述准确，缺口真实存在 |
| CONTRACTS「7 组 20 条」 | 20 | **20**（用节内 awk 判据自跑） | ✅ |

CONTRACTS 的 M1c-1 一节整体质量高于我见过的多数契约文档：几乎每条都标了证据类型、
多条主动记下「这个结论一开始没有任何东西守着，是消融查出来的」。C-2 那条之所以判 CRITICAL，
恰恰因为它是**这份文档里少数几句无限定的普遍命题之一**。

---

## 7. 我**没有**覆盖的空间（诚实边界）

- **未发任何真实网络请求**，故「p1..p155 共 2325 条 istitle 锚、站外恰好 1 条」「155 页语料统计」
  「82 页搜索快照」这三组数**我没有第二条路径**，全部按 dev 自陈引用。
- **未审 `~/hestia-backfill-2026-08-14/` 的 452 个文件内容**（第二轮的下游消费者视角在做）。
- **未做覆盖率复核**：Leader 已指出「覆盖率证明的锚只到解析层、测量层无锚」，我认为
  在 C-1 这类缺陷面前覆盖率不是有效工具（CONTRACTS 的 D2 已经论证过同一件事），故未投入。
- **未审 `.arcforge/` 里 9 份 verification 报告的逐条 DoD 对照**——那是 test-agent 的产出，
  重做一遍是重复劳动；我只在 C-1/C-2 命中处反查了对应 DoD。
