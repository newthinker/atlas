# Sprint 037 · 验收标准独立反审报告

**反审者**：独立 reviewer（team-lead spawn，只读，无写通道 token）
**日期**：2026-08-12
**审查对象**：`.arcforge/tasks/TASK-001..009.json`（**9 个任务的版本**）与 `.arcforge/docs/02-plan/requirement-dod-matrix.md`
**需求源**：`hestia/docs/superpowers/plans/2026-08-12-hestia-cli.md`（1882 行）+ 人类 2026-08-12 的两条定案（季报支持 / 修订版暂不支持）
**方法**：先只读需求与现存代码、独立形成验收标准，**读完之前不打开 `.arcforge/tasks/`**；再比对 Leader 的 DoD。

> **落盘时的两点如实说明**
>
> 1. 本报告写于**任务从 9 个变 10 个之前**。Leader 回复称 13 条全部采纳、新增 TASK-010（`parse.go` 标题层）、validator exit 0、wave1 已开工 —— **该处置结果由 Leader 记录，本反审者未复核落盘**。文中凡引用任务编号处均指 9 任务版本。
> 2. 本文所有「实测」均为反审者在只读工作区外（scratchpad）复现所得，未改动仓库任何文件（`git status --porcelain` 全程只有 Leader 自己的两条 untracked）。复现命令附在最后一节，可脱离 scratchpad 重跑。

---

## 一、独立形成的验收标准（在打开 DoD 之前写下，原样保留）

**这批工作算做完的条件：**

1. `atlas hestia ingest` 在真实站点上跑一轮，能把当期报告落进观测表或 pending，二次运行空跑；`atlas hestia status` 能在**空库**和**有数据**两种状态下打印可读输出，且打印的库路径是绝对路径。
2. 三条「看起来正常的失败」各有一道机制挡着，且每道都能指出**破坏它会让哪条断言红**：期次张冠李戴（接缝②）、cwd 错导致的假「0 期」、代理键被照抄。
3. 季报（一季度 / 前三季度）**端到端**可发现、可解析、可入库；不是「标题能匹配」而已。
4. 「单期失败不中断整批」成立，且整批退出码非零 —— 因为 launchd 下退出码是唯一的报警通道。
5. 修订版不支持这件事**写进契约，并写明它为什么不支持**（不是「暂时没做」而是「结构上不可达」）。
6. 配置文件与 plist 不只是「文件存在」，而是**真的会被部署链路带到 runtime 并被 launchd 加载**。

**特别要防住的两类：**

- **做了但没做对**：季报只改一半（发现得了、解析不了），或只改了标题正则而链接层根本没把那条链接交出来。
- **看起来正常的失败**：discover 侧的失败模式是**静默跳过**（`scanPage` 里 `parsePeriod` 不认就 `continue`），parse/extract 侧的失败模式是**响亮报错**。所以任何「季报支持」的验收，**证据必须落在 discover 那一侧的肯定式断言上**，光看 parse 没报错什么都证明不了。

---

## 二、阻断级发现（5 条）

### B1 · `articleLinkRE` 的 `\d{14,}` 是第五处硬卡点，而 TASK-001 把这个缺陷写成了断言

真实的 `2025年前三季度金融统计数据报告` 的 article_id 是 **7 位**，不是 14+ 位。

```
$ curl -s https://www.pbc.gov.cn/goutongjiaoliu/113456/113469/11040-18.html \
  | grep -o '<a href="[^"]*"[^>]*>[^<]*金融统计数据报告[^<]*'
<a href="/goutongjiaoliu/113456/113469/5868082/index.html" onclick="void(0)" target="_blank"
   title="2025年前三季度金融统计数据报告" istitle="true">2025年前三季度金融统计数据报告
```

原文核实（非 WebFetch 摘要）：直接 curl 该文章页得 41178 字节，
`<meta name="Url" content="/goutongjiaoliu/113456/113469/5868082/index.html">`、
`<meta name="PubDate" content="2025-10-15">`。

把 `internal/hestia/discover.go:146` 的 `articleLinkRE` 与 `:155` 的 `tagRE` **逐字复制**出来，跑在真实抓下来的 p7 / p18 上：

```
######## p7.html
  [现状 \d{14,} + 只修标题正则(TASK-001 的设想)]
    id=2026041311133582598(19位) title="2026年一季度金融统计数据报告" 标题正则命中=true
######## p18.html
  [现状 \d{14,} + 只修标题正则(TASK-001 的设想)]
    （零条：整条链接根本没进入 scanPage 的循环体）
  [放宽到 \d{6,} + 修标题正则]
    id=5868082(7位) title="2025年前三季度金融统计数据报告" 标题正则命中=true
```

**范围有多大** —— 逐页统计 article_id 位数（每页 15 条）：

| 页 | id 位数分布 |
|---|---|
| p1、p2（仓库快照） | 15 × 19 位 |
| p7、p8、p9、p10、p11、p12、p13、p14 | 15 × 19 位 |
| **p15、p16、p17、p18** | **15 × 7 位** |

**分界在 p14 / p15 之间。第 15 页起 `scanPage` 一条候选都产不出，且完全静默**（`Discover` 照常翻到 `MaxPages` 后返回空，不报错）。每页约覆盖 20 天 ⇒ 约 2025-11 之前的全部历史对 discover 不可见。

这不只是季报的问题：**M1c 的 80 期回填（2020-01 起）整段落在死区里。**

**对本 Sprint 的直接后果**：

- TASK-001 `functional[2]` 要求断言「ArticleID `^\d{14,}$`」—— **这是把缺陷钉成契约**。用真实前三季度快照时这条永远绿不了；只用一季度快照时它绿，而前三季度的缺口原样留下，DoD 还会被判满足。
- Leader 写的「遍历型断言必须有肯定式前置锚点（`require.NotEmpty`）」（G9）**是唯一会救场的东西** —— 用 p18 快照时 `scanPage` 返回 nil，`require.NotEmpty` 打红。这条防护第一次真正兑现。

**建议**：TASK-001 加一条 functional「`scanPage` 必须能从**真实前三季度快照**（7 位 article_id）提取到该条目」，把 `^\d{14,}$` 改成 `^\d+$` 或直接断言等于样本里的字面量。放宽下界要配一条否定式边界 —— 实测仓库 p1 快照上 `\d{14,}` → 15 条、`\d{6,}` → 57 条，多出的 42 条全是栏目导航页（`/rmyh/105145/index.html` 这类），链接文本过不了 `parsePeriod`，所以放宽本身安全；但**这 42 条产出 0 候选**要有断言钉住，否则下次再放宽就没有网了。

> ⚠️ `discover.go:137` 的注释写着「栏目路径不写死（栏目 ID 变了能自动跟随）」—— 所以修法**不建议**改成锚定 `/113456/113469/`，那会推翻一条既有设计决定。降低位数下限 + 依赖 `parsePeriod` 过滤是代价更小的路。

---

### B2 · `parse.go` 的 `titleRE` / `parseTitle` 不在任何任务的 `writes` 里

- TASK-001 writes：`discover.go` / `discover_test.go` / `types.go` / `testdata`
- TASK-004 writes：`profiles.go` / `profiles_test.go` / `testdata`
- `parse.go:21` 的 `titleRE`（`\A(\d{4})年(上半年|[0-9]{1,2}月)?金融统计数据报告\z`）与 `:28` 的 `parseTitle`：**无人认领**

实测（逐字复制两条正则跑真实标题）：

```
2026年一季度金融统计数据报告      discover=[]  parse=[]
2025年前三季度金融统计数据报告     discover=[]  parse=[]
2026年上半年金融统计数据报告      discover=[… 2026 上半年]  parse=[… 2026 上半年]
2025年金融统计数据报告           discover=[… 2025 ""]      parse=[… 2025 ""]
```

⇒ **TASK-004 的 `functional[1]`（「用真实一季度正文跑通 `Parse` → `extractFields`」）与 `non_functional[0]`（端到端 `scanPage → parsePeriod → Parse → Validate`）结构上不可满足** —— `Parse` 会在 `parseTitle` 处返回 `unrecognized report title`，一步都走不到 `extractFields`。dev 只能中途 `update --json-field writes` 补 `parse.go`，那是拆分时本可避免的返工。

**建议**：把 `./internal/hestia/parse.go` + `parse_test.go` 交给标题层（TASK-001 或独立任务），TASK-004 保持只碰 `profiles.go`。与 wave 1 的 TASK-002 / TASK-003 无文件重叠，不破坏 scope 互斥。

**附带建议（让中间态自解释）**：`parse.go:189` 的 `checkPeriodTypeSupported` 目前只挡 `monthly`，季度类型会直接穿过去。让标题层任务把新的季度类型也加进拒绝列表、由 TASK-004 移除 —— 这样「只做了标题层」的中间态给出的是一句写明理由的拒绝，而不是 extract 深处的 `not found among N candidate sentence(s)`。

---

### B3 · TASK-001 `error_handling` 的「两个 map 互查」照做会毁掉 monthly

DoD 原文：「任何在 `validPeriodTypes` 里的类型，`periodEndMonth` 都要有对应项，**反之亦然**」。

但 `types.go:62-77` 写着：**`monthly` 刻意不在 `periodEndMonth` 表内**（「monthly 不在表内，因为任意月份都合法」），而 `types.go:150` 的校验正是
`if want, ok := periodEndMonth[m.PeriodType]; ok && m.Period[5:] != want` —— 靠「查不到就跳过」实现。

⇒ 这条 DoD 的正向（`validPeriodTypes ⊆ periodEndMonth`）**对现状即为假**。dev 只有两条路：

- (a) 给 `monthly` 编一个期末月 ⇒ **除该月外的每一期月报都会被 `Meta.validate` 拒绝**；
- (b) 悄悄把断言改成单向 ⇒ 这条 DoD 变成空话。

**建议**改成两条可满足且真能防漏改的：

1. `periodEndMonth` 的每个键必须在 `validPeriodTypes` 里（单向包含，当前成立）；
2. **除 `monthly` 外**，`validPeriodTypes` 的每个键都要有期末月，并在注释里写明 `monthly` 是唯一豁免及其理由。

**附带发现**：这条真正想防的「改一个忘另一个」还有**两处纯字符串**会静默过期，均无 DoD 覆盖 ——
`types.go:144` 的 `(want monthly|h1|annual)` 与 `thresholds.go:126` 的同一串。
讽刺的是 `types.go:116` 的 `checkEnum` 注释专门警告过「抄一遍的写法会静默过期」，而这两处正是它自己没用 `checkEnum` 的地方。建议加一条：错误信息里的取值列表必须由 `validPeriodTypes` 派生。

---

### B4 · TASK-007 `boundary` 的「`--force` 重跑已观测期次 ⇒ Duplicate」不成立

`--force` 只绕过 `ingestOne` 里的 `HasArticle`，**不绕过 `Discover` 里的 `HasPeriod`**。
`discover.go:270-272` 是 `if has { return out, nil }` —— 已在观测表的期次会让 `Discover` 当场返回空。

不是推理，仓库里有现成的绿测试直接证明：

```
$ go test ./internal/hestia/ -run '^TestDiscoverEmptyStoreExhaustsWhileKnownStopsEarly$' -v -count=1
--- PASS: TestDiscoverEmptyStoreExhaustsWhileKnownStopsEarly (0.00s)
```

其断言 `discover_test.go:565`：`assert.Empty(t, gotB, "唯一的候选已入库，不该产出任何东西")`。

⇒ 计划里的 `TestForceOnObservedPeriodIsDuplicate` 会**红在** `assert.Contains(t, out.String(), "Duplicate")` —— 实际输出是 `no new reports`。

**这与 Leader 已抓到的修订版结构盲区（P10）是同一个形状，换了个载体。** 计划自己在 pending 那半察觉到了同源现象（1878 行的 ⚠️「`Discover` 仍会把这期报出来——因为 `HasPeriod` 不认 pending」），却没把同一句推理用在「`HasPeriod` **认**观测表」这一半上。

**危险的不是它会红，是它会被怎么修好**：dev 最省事的做法是删掉 `Contains("Duplicate")`，只留 `assert.Equal(t, 1, countRows(...))`。那条断言会绿 —— **但绿的原因是根本没走到 `Save`**，与「Duplicate 不产生重复行」毫无关系。

**建议**改成如实的两半：

- 「`--force` 重跑 **pending** 期次 ⇒ `New` 落观测表」保留（这条成立）；
- 「`--force` 对**已在观测表**的期次**不可达**」，测试断言 `no new reports`，并写进 CONTRACTS：**`--force` 的作用域只到 pending**，「调了阈值想重跑一个已入库的期次」目前没有出路（M1c 标定 `MagnitudeRanges` 时会直接撞上，而 CONTRACTS 现在正准备写「`--force` 是为这件事准备的」）。

---

### B5 · TASK-004 标题「`cumulativePeriods`（唯一硬卡点）」不成立 —— `periodAlt` 严格在它上游

`extract.go:187` 的谓词是 `cumulativePeriods[m[1]] && m[2] == currencyRMB`，而 `m[1]` 只可能是 `profiles.go:62` 的 `periodAlt` 产出的分支之一。**往 `cumulativePeriods` 加一个 `periodAlt` 永远产不出的键 = 完全 no-op。**

实测（复刻 `flowRE` 的四种组合，跑在同一段正文上）：

| 改法 | 候选句 | 命中 | 结果 |
|---|---|---|---|
| 现状 | 1 条 `[3月份/人民币]` | 0 | 报错 |
| **只改 `cumulativePeriods`** | 1 条 `[3月份/人民币]` | 0 | 报错 —— **与现状逐字相同** |
| 只改 `periodAlt` | 3 条 `[一季度/人民币 3月份/人民币 一季度/外币]` | 0 | 报错 |
| 两处都改 | 3 条（同上） | 1 | 抽到值 |

⇒ **TASK-004 的消融自证判据有缺陷**：「把季报前缀从 `cumulativePeriods` 删掉，确认转红」—— 删 `periodAlt` 那一半也红，删两边也红，**四格里三格红，这个消融不携带信息**（正是 Leader 自己引的 G31）。

**建议**：标题改为「季报抽取支持：`periodAlt` + `cumulativePeriods`（两处，缺一即 no-op）」；消融做成 **2×2**，要求两次单删的**失败信息不同**（删 `periodAlt` 时候选数掉，删 `cumulativePeriods` 时候选数不变而命中为 0），并把两次的候选数写进 discovery。这样消融才区分得开哪一处承重。

---

## 三、会被错误实现平凡满足的 DoD

> 问法：「有没有一个我想排除的实现，能让这条断言照样绿？」（B1、B4 亦属此类）

### C1 · TASK-005 `non_functional[0]` 的倒序断言

断言是「较新那期的 `stock_continuity` **不应**是 `no_prior_period`」。纯否定式，而 `gateStockContinuity`（`validate.go:354-372`）有**四种** skip 理由。只要该期缺 `tsf_stock`，返回的是 `absent_field:tsf_stock` —— **断言照样绿，而顺序仍然是错的**。

不是假想：仓库里唯一的两份文章快照是 `pboc-2020-06-h1.html`（rule@v1，无社融板块 ⇒ 无 `tsf_stock`）与 `pboc-2025-12-annual.html`，而 `Preceding` 按 `period_type` 过滤（`validate.go:81`）。想构造「同 `period_type` 的两期」现有 testdata 根本不够，dev 必然要凑 —— 凑出来的十有八九就是上面那个平凡绿。

**建议**换成肯定式：断言该期 `stock_continuity` 的 `Status == CheckPassed` 且 `Value != nil`（即闸门**真的执行了**）；或更直接、也更不依赖数据 —— 断言 `Out` 里输出的期次序列是**升序**的，那是对「顺序」本身的断言，不经过任何闸门。

### C2 · TASK-001 `boundary` 的干扰项对季度分支零鉴别力

`2026年二季度金融机构贷款投向统计报告` 在仓库快照 `pboc-index-p1.html` 里真实存在（article_id `2026072719364116939`）。但实测两种季度改法它**都被拒**：

```
假设修法 A（加 一季度|前三季度）：      2026年二季度金融机构贷款投向统计报告 → []
假设修法 B（写成 前?[一二三四]季度）：  同上 → []
```

拒它的是 `discover.go:87-99` 注释里说的「期次段**紧跟**金融统计数据」那个机制，与季度分支正交。⇒ 这条 boundary 是有价值的回归守卫，但它**不可能红在一个错误的季度实现上**，别把它算成对季报的覆盖。

**真正的季度专属陷阱是交替顺序，且现有守卫抓不到。** 实测：

```
alt=全年|上半年|三季度|一季度|…    对 "前三季度人民币贷款…" 捕获="三季度"    现有前缀守卫=通过
alt=全年|上半年|三季度|前三季度|…   捕获="前三季度"                     现有前缀守卫=通过
alt=全年|上半年|前三季度|三季度|…   捕获="前三季度"                     现有前缀守卫=通过
alt=全年|上半年|前三季度|一季度|…   捕获="前三季度"                     现有前缀守卫=通过
```

`profiles_test.go:163` 的 `TestProfileAlternationsHaveNoPrefixPairs` 只查**前缀**对（`strings.HasPrefix`），而这里是**后缀**关系 —— 四个变体它全放行。写成 `三季度` 时 Go 的 leftmost 规则会从「三」那个位置起匹，捕获到 `三季度`，随后 `cumulativePeriods["前三季度"]` 查不到 ⇒ 报错；若 dev 为了让它绿而把 `三季度` 也登记进 `cumulativePeriods`，就等于默认「三季度」是累计口径，而那个判断没有任何样本支持。

**建议**：TASK-004 加一条 boundary —— 用真实前三季度正文断言 `flowRE` 捕获组 `m[1]` **逐字等于**样本里的那个词（不是「能匹配」）；并把 `TestProfileAlternationsHaveNoPrefixPairs` 的判据从 prefix 扩到 substring（改一行，两个方向都覆盖）。

### C3 · TASK-009 plist 守卫：DoD 比计划强，但 dev 会照抄计划

Leader 写的是「断言 `EnvironmentVariables` 里不存在任何 `*_proxy` / `*_PROXY` 键」+ 肯定式锚点 —— 这是全套 DoD 里质量最高的一条。

但计划 1671 行给的实现只枚举 4 个名字（`http_proxy`/`https_proxy`/`HTTP_PROXY`/`HTTPS_PROXY`），而 `deploy/launchd/com.newthinker.atlas.crisis-daily.plist` 里**还有一个 `no_proxy`**。照抄 crisis 再删掉那两对，`no_proxy` 留下 → 测试全绿，而「一个代理键都不设」这句话是假的。

**建议**在 DoD 里明确「**不得照抄计划 1671 行的四名单**」，否则强版本会在实现时静默退化成弱版本。

---

## 四、DoD 遗漏项（独立想到、原 DoD 未覆盖）

| # | 该加到哪 | 加什么 | 为什么 |
|---|---|---|---|
| D1 | **TASK-009**（`writes` 需加 `scripts/ops/install-services.sh`） | plist 必须登记进该脚本的 label 列表 | 脚本第 29-31 行是**硬编码枚举**（10 个 label），新 plist 不加进去就永远不会被 `cp` 到 `~/Library/LaunchAgents`。计划 Step 9 的 `launchctl load ~/Library/LaunchAgents/com.newthinker.atlas.hestia-ingest.plist` 会直接 `no such file`。**`deploy.sh` 那边已核实无问题** —— 它是整目录 `rsync`，`configs/hestia.yaml` 与 `deploy/launchd/*.plist` 都会自动带过去。 |
| D2 | **TASK-009** | plist 必须通过 `plutil -lint` | `install-services.sh:34` 会跑 `plutil -lint`，而计划的 Go 守卫是纯字符串匹配 —— XML 写坏了 Go 测试照样绿，安装时才炸。 |
| D3 | **TASK-009** | 一条测试：`hestia.LoadConfig("../../configs/hestia.yaml")` 必须成功 | 有现成先例：`cmd/atlas/crisis_test.go:487` 读的就是真实的 `../../configs/crisis-monitor.yaml`。目前「配置文件本身能不能装载」的唯一验证是被刻意排除在 DoD 之外的 Step 9 手工验收。 |
| D4 | **TASK-008** | `rootCmd` 需要 `SilenceUsage` | 实测 `go run ./cmd/atlas crisis status --crisis-config /nonexistent/nope.yaml`，`Error: …` 之后跟了 9 行完整 usage；`main.go:26-30` 没有 `SilenceUsage`/`SilenceErrors`。「单期失败不中断整批 + 汇总非零」的设计意图是让 `hestia-ingest.err.log` 成为报警通道，而每次失败灌一屏 usage 会把真正那行错误埋掉 —— 且这个管线预期会有**连续两个月每天三次**的稳定失败态（见 D6）。 |
| D5 | **TASK-006** | `n ≤ 0` 的判据不对称 | SQLite 的 `LIMIT -1` 是**不限行数**、`LIMIT 0` 是零行。DoD 已要求「钉住」，需点名免得 dev 只测 0 不测负。 |
| D6 | **TASK-001 / Sprint 层面** | 标题层任务**不得单独合进 master** | 标题层落地而抽取层未落地时，季报会从「静默消失」变成「被发现→`Parse`/extract 报错→整批非零」。`Discover` 只在命中已入库期次时提前返回，失败的期次永远入不了库 ⇒ 该期会在 `MaxPages` 窗口内（约 2 个月）**每次唤起都失败**，一天三次。比静默好，但会让退出码这个唯一报警通道在一年里失效约 4 个月。建议两任务同一分支、一次合入。 |
| D7 | **TASK-006 / TASK-008 择一** | 绝对路径解析职责重复 | TASK-008 `functional[1]` 说「`db_path` 在这一层解析成绝对路径，**这是相对路径唯一被解析的地方**」，而 TASK-006 `non_functional[0]` 要求 `RenderStatus` 内部做 `filepath.Abs`（计划 1260 行亦然）。都实现的话解析发生两次，「唯一」是假的。行为上无害，但下一个人读到「唯一」会去找另一处并删掉它。请指定一处。 |
| D8 | **TASK-004** | 「一季度」与「前三季度」共用一条还是各一条 —— 答案已实测 | 两者形态相同（都是 `periodAlt` 的独立分支 + `cumulativePeriods` 的独立键），**共用同一条实现**；但**必须各有一条断言**，因为 `前三季度` 有 C2 的后缀陷阱而 `一季度` 没有。 |

---

## 五、季报两任务专项（第 4 问）

**拆成两个：对，但边界划错了。** 现在的边界是「discover 侧 vs extract 侧」，真实依赖链是：

```
articleLinkRE(\d{14,})  →  reportTitleRE/parsePeriod  →  titleRE/parseTitle  →  periodAlt  →  cumulativePeriods
   [B1 无人认领]            [TASK-001]                   [B2 无人认领]         [TASK-004]     [TASK-004]
   ←──────────────── 静默失败 ────────────────→         ←──────────── 响亮失败 ────────────→
```

**`cumulativePeriods` 不是唯一硬卡点，甚至不是主要那个** —— 它在链条最末端且失败最响亮。真正会静默吃掉季报的是最左边那两个。建议按「静默 / 响亮」重划边界：标题层任务收下 `articleLinkRE` + 两个标题正则 + 两个 map（全部是静默失败点），抽取层任务收下 `periodAlt` + `cumulativePeriods`。

**没有第六处。** 把 `period_type` 在包内的全部消费点扫过并逐个排除：

- `schema.go` 对 `period_type` **没有 CHECK 约束**（只有 `PRIMARY KEY (period, period_type, published_at)`）⇒ 新期次类型**不需要 schema 迁移**，`NewStore` 的 `verifyObservationsSchema` 不会拦。**这条原本怀疑是阻断项，实测排除。**
- `thresholds.go:124` 走的是 `validPeriodTypes` 这个 map 本身，自动跟随。
- 「月均折算除数 1/6/12」在本包**没有实现**（`types.go:38` 的注释只是声明）⇒ 本 Sprint 不需要为季度补除数，但那句注释会因此变成假的，请顺手改。
- `bitemporal` 的业务键含 `period_type`，季度期次（如 `2026-03/q1`）与同月月报（`2026-03/monthly`）是不同的键，不会互相吞 —— 与 `types.go:71-73` 对 `YYYY-12/monthly` 的既有论证同构。
- `findSection` 只认标题，正文里的期次词不影响板块定位（`sections.go:70-100`）。

---

## 六、已取到的真实样本事实（可直接进 TASK-001/004 的 context）

**均为 curl 取原文核实，不是 WebFetch 摘要。**

| | 一季度 | 前三季度 |
|---|---|---|
| `<meta ArticleTitle>` | `2026年一季度金融统计数据报告` | `2025年前三季度金融统计数据报告` |
| URL | `https://www.pbc.gov.cn/goutongjiaoliu/113456/113469/2026041311133582598/index.html` | `https://www.pbc.gov.cn/goutongjiaoliu/113456/113469/5868082/index.html` |
| article_id | `2026041311133582598`（19 位） | `5868082`（**7 位**） |
| `<meta PubDate>` | 2026-04-13（列表页日期） | `2025-10-15`（原文核实） |
| 所在页（2026-08-12） | p7 | p18 |
| **正文累计前缀** | **`一季度`** | **`前三季度`** |

**一季度正文的 8 个板块标题（逐字）：**

```
一、社会融资规模存量同比增长7.9%
二、一季度社会融资规模增量累计为14.83万亿元
三、广义货币增长8.5%
四、一季度人民币存款增加13.73万亿元
五、一季度人民币贷款增加8.6万亿元
六、3月份银行间人民币市场同业拆借月加权平均利率为1.38%，质押式债券回购月加权平均利率为1.4%
七、国家外汇储备余额3.34万亿美元
八、一季度经常项下跨境人民币结算金额为4.52万亿元，直接投资跨境人民币结算金额为2.11万亿元
```

`sectionRules` 的 7 个 keyword 全部命中标题，板块数 = 8 = `sectionsV2` ⇒ **`detectExtractor` 判 `rule@v2`，与 Sprint 036 消费者位的说法一致**，结构与年报同构。

**正文关键句（逐字）：**

```
一季度人民币存款增加13.73万亿元。其中，住户存款增加7.68万亿元，非金融企业存款增加2.68万亿元，
财政性存款增加4606亿元，非银行业金融机构存款增加2.03万亿元。
一季度外币存款增加703亿美元。
一季度人民币贷款增加8.6万亿元。分部门看，住户贷款增加2967亿元，其中，短期贷款减少1640亿元，
中长期贷款增加4607亿元；企（事）业单位贷款增加8.6万亿元，其中，短期贷款增加4.13万亿元，
中长期贷款增加5.42万亿元，票据融资减少1.1万亿元；非银行业金融机构贷款减少3680亿元。
一季度外币贷款增加329亿美元。
3月末，本外币存款余额350.23万亿元，同比增长8.7%
月末人民币存款余额342.41万亿元，同比增长8.6%
2026年一季度社会融资规模增量累计为14.83万亿元
初步统计，2026年3月末社会融资规模存量为456.46万亿元
```

前三季度正文（同样 curl 核实）：`前三季度人民币贷款增加14.75万亿元。` / `前三季度人民币存款增加22.71万亿元。`

**三条对 TASK-004 直接有用的结论：**

1. **外币孪生句实测存在**（`一季度外币存款增加703亿美元`、`一季度外币贷款增加329亿美元`）⇒ boundary 条不会落到「未观察到」那个分支，dev 必须真写这条用例。
2. **存 / 贷两节里没有单月孪生句**（`3月份` / `3月末` 只出现在利率与外汇两节）⇒ **DoD 钉的失败串 `not found among 0 candidate sentence(s)` 是对的**。
   （反审者一开始按 h1 样本体例仿写了一段正文，推出应是 `among 1`；取到真实正文后该推测被推翻，**以真实样本为准**。这条记下来是因为它示范了「仿写语料会得出错误的预期串」。）
3. 板块标题 `四、一季度人民币存款增加13.73万亿元` 与正文首句同形，但 `splitSections`（`sections.go:57-68`）把标题放 `Title`、正文放 `Body`，`flowRE` 只跑在 `Body` 上 ⇒ 不会造成 `selectUnique` 的双命中。年报样本是同样的形状且现在是绿的，可放心。

**覆盖率基线实测**（`go test -count=1 -cover`）：`internal/hestia` **93.2%**、`cmd/atlas` **75.2%**（低于 80 门禁，TASK-008 引的 AD-6 提醒成立；记忆里记的是 sprint-023 的 75.9%，当前值是 75.2%）。

---

## 七、未经观察的判断（纯推理，未验证）

1. **B1 对生产链路的影响判断为「有限」**：`MaxPages: 3` 下永远够不到 p15；新发布的文章看起来都拿 19 位 id（p7 的一季度报是 2026-04 发布的，19 位），7 位 id 更像是 2026-06-26 站点重建**没覆盖到的历史存量**。但**没有观察能证明「下一篇前三季度报（2026-10）一定是 19 位」** —— 它尚未发布。⇒ 对本 Sprint 是「测试样本不可用」的**确定**问题，对 M1c 回填是**确定**问题，对生产是**可能**问题。
2. TASK-004 端到端跑通后 `completeness` 闸能否过（54 字段是否齐全），只核对了板块结构，**未逐字段验证**。
3. TASK-009 要求「解析 plist 的 `EnvironmentVariables` 字典」在「无新增依赖」约束下是否好写，**未写原型**；`encoding/xml` 与 `exec plutil -convert json` 两条路都认为可行，未验证。
4. D4 验证了 cobra 会打印 usage，但**未验证** `SilenceUsage` 加上之后 crisis 现有测试是否仍绿。

---

## 八、阅读基线（如实申报）

| 材料 | 读到什么程度 |
|---|---|
| `2026-08-12-hestia-cli.md`（1882 行） | **全文**，分 4 次读完 1–500 / 500–1020 / 1020–1500 / 1499–1882 |
| `internal/hestia/discover.go`（279 行） | **全文** |
| `internal/hestia/types.go`（210 行） | **全文** |
| `internal/hestia/profiles.go`（329 行） | **全文** |
| `internal/hestia/parse.go`（199 行） | **全文** |
| `internal/hestia/extract.go`（453 行） | **全文** |
| `internal/hestia/validate.go`（498 行） | **全文** |
| `internal/hestia/config.go`（79 行） | **全文** |
| `internal/hestia/fetch.go` | **全文** |
| `internal/hestia/store.go` | **片段**：1–120 行 + 全文 grep `period_type`/`HasPeriod`/`Preceding` 的命中行。**未通读** Save / savePending / bitemporal 分类逻辑 |
| `internal/hestia/sections.go` | **片段**：1–120 行（含 `splitSections`/`findSection`/`sectionsV1/V2`），后半段未读 |
| `internal/hestia/required.go` | **片段**：1–120 行（实际全文约 50 行，已覆盖 `requiredFields`） |
| `internal/hestia/schema.go` | **仅 grep**（`CHECK`/`period_type`/`PRIMARY KEY`/`UNIQUE`），未通读 |
| `internal/hestia/thresholds.go` | **仅 grep**（`PeriodTypes` 相关行），未通读 |
| `internal/hestia/*_test.go` | **片段**：`profiles_test.go` 160–200 / 316–348 通读；`discover_test.go` 仅 grep + 461–590 相关行；`store_test.go` 仅 grep 导出面守卫注释行；其余仅函数名清单 |
| `internal/hestia/amount.go`、`fields.go`、`strip.go`、`golden_test.go` | **未读** |
| `cmd/atlas/crisis.go`（604 行） | **仅 grep** cobra 装配相关行（37–90），未通读主体 |
| `cmd/atlas/main.go`（30 行） | **全文** |
| `cmd/atlas/crisis_test.go` | **片段**：470–515 |
| `scripts/ops/deploy.sh` | **1–70 行**（含全部 rsync 规则） |
| `scripts/ops/install-services.sh` | **全文**（1–60 行） |
| `deploy/launchd/com.newthinker.atlas.crisis-daily.plist` | **全文** |
| `.arcforge/tasks/TASK-001..009.json` | **全文 9 份** |
| `requirement-dod-matrix.md`（109 行） | **全文** |
| hestia spec `2026-08-12-hestia-cli-design.md` | **未读** —— 计划引用它 12 次，仅通过计划的转述了解 |

**两处复现代码是近似而非逐字复制，请据此打折：**

- `flowRE` 的 sim 里 `directionAlt` / `unitAlt` 是按注释与样本正文**近似**写的（真词表在 `amount.go`，未读）。这只影响候选句能否被匹配，**不影响 B5 的结论** —— 该结论只取决于 `periodPat` 与 `cumulativePeriods` 的先后关系，与词表内容无关。
- `articleLinkRE` / `tagRE` / `reportTitleRE` / `titleRE` / `TestProfileAlternationsHaveNoPrefixPairs` 的判据**是逐字复制**的，**B1、C2 的结论建立在逐字复制之上**。

---

## 九、复现命令（脱离 scratchpad 可重跑）

```bash
# 1) 取三份真实页面
curl -sS -o p7.html  "https://www.pbc.gov.cn/goutongjiaoliu/113456/113469/11040-7.html"
curl -sS -o p18.html "https://www.pbc.gov.cn/goutongjiaoliu/113456/113469/11040-18.html"
curl -sS -o q1.html  "https://www.pbc.gov.cn/goutongjiaoliu/113456/113469/2026041311133582598/index.html"
curl -sS -o q3.html  "https://www.pbc.gov.cn/goutongjiaoliu/113456/113469/5868082/index.html"

# 2) 季报链接与 article_id
grep -o '<a href="[^"]*"[^>]*>[^<]*金融统计数据报告[^<]*' p7.html p18.html
grep -o '<meta name="\(ArticleTitle\|PubDate\|Url\)" content="[^"]*"' q1.html q3.html

# 3) 逐页 id 位数分布（把 N 换成 1..18）
for n in 7 8 9 10 11 12 13 14 15 16 17 18; do
  curl -sS -o "pp$n.html" "https://www.pbc.gov.cn/goutongjiaoliu/113456/113469/11040-$n.html"
  echo -n "p$n: "
  grep -oE 'href="/goutongjiaoliu/113456/113469/([0-9]+)/index\.html"' "pp$n.html" \
    | grep -oE '/[0-9]+/index' | tr -d '/index' | awk '{print length($0)}' | sort -n | uniq -c | tr '\n' ' '
  echo
done

# 4) 放宽下界会多带进来什么（在仓库快照上跑）
cd internal/hestia/testdata
grep -coE 'href="[^"]*/[0-9]{14,}/index\.html"' pboc-index-p1.html   # 15
grep -coE 'href="[^"]*/[0-9]{6,}/index\.html"'  pboc-index-p1.html   # 57
grep -oE  'href="[^"]*/[0-9]{6,13}/index\.html"' pboc-index-p1.html | sort -u   # 42 条导航页

# 5) --force 已观测期次不可达（仓库现成的绿测试）
go test ./internal/hestia/ -run '^TestDiscoverEmptyStoreExhaustsWhileKnownStopsEarly$' -v -count=1
# 断言在 internal/hestia/discover_test.go:565

# 6) cobra 会打印整屏 usage
go run ./cmd/atlas crisis status --crisis-config /nonexistent/nope.yaml

# 7) 覆盖率基线
go test ./internal/hestia/ ./cmd/atlas/ -count=1 -cover
```

**B1 / B5 / C2 的 Go sim**（`main.go`，逐字复制处已标注）：

```go
package main

import ("fmt"; "os"; "regexp"; "strings")

// —— 逐字复制自 internal/hestia/discover.go:146 / 155 / 100，parse.go:21 ——
var articleLinkRE = regexp.MustCompile(`(?s)href="([^"]*?/(\d{14,})/index\.html)"[^>]*>(.*?)</a>`)
var tagRE         = regexp.MustCompile(`<[^>]+>`)
var reportTitleRE = regexp.MustCompile(`(\d{4})年(上半年|\d{1,2}月)?金融统计数据报告`)
var titleRE       = regexp.MustCompile(`\A([0-9]{4})年(上半年|[0-9]{1,2}月)?金融统计数据报告\z`)

// —— 假设修法 ——
var fixedTitleRE   = regexp.MustCompile(`(\d{4})年(上半年|前三季度|一季度|\d{1,2}月)?金融统计数据报告`)
var loosenedLinkRE = regexp.MustCompile(`(?s)href="([^"]*?/(\d{6,})/index\.html)"[^>]*>(.*?)</a>`)

func scan(html []byte, linkRE, tRE *regexp.Regexp) (out []string) {
	for _, m := range linkRE.FindAllSubmatch(html, -1) {
		title := strings.TrimSpace(tagRE.ReplaceAllString(string(m[3]), ""))
		if !strings.Contains(title, "金融统计数据报告") { continue }
		out = append(out, fmt.Sprintf("id=%s(%d位) title=%q 标题正则命中=%v",
			m[2], len(m[2]), title, tRE.FindStringSubmatch(title) != nil))
	}
	return
}

func main() {
	for _, f := range []string{"p7.html", "p18.html"} {
		b, _ := os.ReadFile(f)
		fmt.Printf("######## %s\n", f)
		for _, c := range []struct{ name string; l, t *regexp.Regexp }{
			{"现状 \\d{14,} + 现状标题正则", articleLinkRE, reportTitleRE},
			{"现状 \\d{14,} + 只修标题正则", articleLinkRE, fixedTitleRE},
			{"放宽 \\d{6,} + 修标题正则", loosenedLinkRE, fixedTitleRE},
		} {
			rows := scan(b, c.l, c.t)
			fmt.Printf("  [%s]\n", c.name)
			if len(rows) == 0 { fmt.Println("    （零条：整条链接根本没进入 scanPage 的循环体）") }
			for _, r := range rows { fmt.Println("    " + r) }
		}
	}

	// C2：交替顺序 × 现有前缀守卫（判据逐字复制自 profiles_test.go:163）
	body := "前三季度人民币贷款增加19.59万亿元"
	for _, alt := range []string{
		`全年|上半年|三季度|一季度|[0-9]{1,2}月份`,
		`全年|上半年|三季度|前三季度|[0-9]{1,2}月份`,
		`全年|上半年|前三季度|三季度|[0-9]{1,2}月份`,
		`全年|上半年|前三季度|一季度|[0-9]{1,2}月份`,
	} {
		m := regexp.MustCompile(`(` + alt + `)`).FindStringSubmatch(body)
		words, pair := strings.Split(alt, "|"), ""
		for _, s := range words {
			for _, l := range words {
				if s != l && strings.HasPrefix(l, s) { pair = fmt.Sprintf("%q 是 %q 的前缀", s, l) }
			}
		}
		guard := "通过"; if pair != "" { guard = "打红(" + pair + ")" }
		fmt.Printf("alt=%-40s 捕获=%-10q 现有前缀守卫=%s\n", alt, m[1], guard)
	}
}
```
