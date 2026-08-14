# M1c-1 历史快照抓取 · 定稿设计

本文件是 dev 实现的直接依据。上游 spec（`hestia/docs/superpowers/specs/2026-08-14-…-design.md`）
仍然有效，本文件只**订正**它与实测不符之处、并**补入**人类 2026-08-14 的三处裁决。
冲突时以本文件为准。

## 1. 文件结构

| 文件 | 职责 | 新建/修改 |
|---|---|---|
| `internal/hestia/backfill_scan.go` | index 侧：列表项正则、三种标题识别、翻页与按日期停止 | 新建 |
| `internal/hestia/backfill_scan_test.go` | 用 p146 / p18 / p147 快照驱动 | 新建 |
| `internal/hestia/backfill_search.go` | **搜索侧**：wzdig 查询构造、结果解析、栏目前缀筛（**单页**：取一页 + 解析 + 筛 + 交出总页数；**翻页循环归调用方**，见下） | 新建（本次新增） |
| `internal/hestia/backfill_search_test.go` | 用搜索结果快照驱动 | 新建（本次新增） |
| `internal/hestia/backfill_manifest.go` | `Manifest` / `Article` / `Failed` 类型与逐篇追加写 | 新建 |
| `internal/hestia/backfill_manifest_test.go` | 读写与断点续抓的判据 | 新建 |
| `internal/hestia/backfill_fetch.go` | 限速、落盘、跳过已抓、失败收集、**交叉校验差集** | 新建 |
| `internal/hestia/backfill_fetch_test.go` | 限速、续抓、两类错误的不同处置、差集 | 新建 |
| `cmd/atlas/hestia.go` | `+backfill fetch` 子命令 | 修改 |
| `cmd/atlas/hestia_test.go` | 子命令注册与 flag | 修改 |
| `internal/hestia/testdata/pboc-index-p146-2020.html` | **历史页样本（38KB，进 git）** | 新建 |
| `internal/hestia/testdata/pboc-search-p1.html` | 搜索结果样本（进 git，测试样本） | 新建 |
| `internal/hestia/testdata/README.md` | 登记新增两份样本 | 修改 |
| `internal/hestia/CONTRACTS.md` | 登记本迭代，**更正 H8「同期两篇」→ 三篇** | 修改 |

**为什么分四个文件**：扫描是「index 页里有什么」（纯函数，快照可测）、搜索是「检索服务说有什么」
（纯函数，快照可测）、manifest 是「记了什么」（纯 I/O）、fetch 是「怎么把它们弄下来」（限速与
错误策略）。四者的测试形态完全不同。

**不新建子包**（ADR-0009）：复用的 `parsePaging` / `pageURL` / `resolveURL` / `tagRE` 都是包内
未导出的。文件名加 `backfill_` 前缀。

⚠️ **搜索侧的翻页循环归调用方，不在 `backfill_search.go` 里**（TASK-004 交付时澄清）。
上面职责表原先把「翻页」笼统划给该文件，但**判据是 DoD `functional[3]`**：「总页数供**调用方**
决定翻到第几页」。理由：限速（§7.1 的可注入 sleep）与逐页落盘都在抓取层，翻页循环放进解析层
会和它们打架。`backfill_search.go` 交出 `(hits, totalPages, error)`，循环由抓取层驱动。

## 2. 三种报告标题

```go
// backfillTitleRE 匹配本迭代要抓的三种报告。
//
// 期次段与既有 reportTitleRE 逐字相同（`上半年|前三季度|[一二三四五六七八九十]季度|\d{1,2}月`），
// 扩的只是**报告种类**：加社融存量 / 增量两种。
//
// ⚠️ 期次段必须**紧跟**在报告种类之前 —— 这一条挡住省市分行的报告
// （实测：「2020年11月厦门市金融统计数据报告」「2020年11月份吉林省金融统计数据报告」）。
// 别放宽成「标题里含『金融统计数据报告』」，那会把 30+ 家省市分行的同名报告全收进来。
var backfillTitleRE = regexp.MustCompile(
    `(\d{4})年(上半年|前三季度|[一二三四五六七八九十]季度|\d{1,2}月)?(金融统计数据报告|社会融资规模存量统计数据报告|社会融资规模增量统计数据报告)`)
```

**不解析期次**（spec §7）：manifest 只记标题原文。`parsePeriod` 本迭代不调用——那个「存量说
3 月、增量说一季度」的坑留给 M1c-2 面对数据而不是猜测。

## 3. index 侧扫描（主路径）

### 3.1 列表项正则

实测结构（逐字，同一行）：

```html
<a href="/goutongjiaoliu/113456/113469/2025092212550638999/index.html" onclick="void(0)"
   target="_blank" title="2020年3月社会融资规模存量统计数据报告" istitle="true">…</a></font><span class="hui12">2020-04-10</span>
```

```go
// backfillItemRE 一条列表项：href / article_id / title 属性 / 发布日期。
//
// 取 title= 属性而非链接文本 —— 回填需要同时拿到标题与发布日期，而 title 属性省掉剥标签。
// article_id **不设任何位数下界**（Sprint 037 教训：`\d{14,}` 让第 15 页起整页命中 0 条、
// 循环体一次都不执行、完全静默）。实测 p146 是 19 位、p18 是 7 位。
var backfillItemRE = regexp.MustCompile(
    `(?s)href="([^"]*?/(\d+)/index\.html)"[^>]*?\stitle="([^"]*)"[^>]*>.*?</a>\s*</font>\s*<span class="hui12">\s*(\d{4}-\d{2}-\d{2})\s*</span>`)
```

🔴 **本文件此处原先写的是 `[^>]*title="`，那是个真 bug**（dev-agent-55 在 TASK-001 实测发现并订正）：
`[^>]*` 会**贪进 `istitle="true"`**，于是捕获组拿到的是字面量 `true` —— p146 上 15 条**全部**如此。

**为什么它特别危险**：条数一条不少、日期一天不差 ⇒ **`boundary[2]` 的计数守卫接不住它**，
表现为整页静默产出 0 篇报告。它藏在一条**为了防静默失效而写的正则**里。

修法是 `[^>]*?\stitle="` —— `\s` 让 `title=` 必须以空白开头，而 `istitle=` 里 `title` 前是 `s`，
结构上落不进去。变异实测：还原成原式后 5 个标题断言红、**计数守卫不红** ⇒ 两条守卫看的是不同失效面，
互补且缺一不可。

```
```

### 3.2 两条必须分开判的守卫

| 情形 | 处置 | 理由 |
|---|---|---|
| 某页匹配 **0 条列表项** | **报错** | 解析失效，不是「这页没有报告」。回填翻 150 页跨越了一次真实站点重建（2026-06-26），静默的后果是回填看起来跑完了而 manifest 少了几十期 |
| 某页有列表项但**其中 0 条是报告** | **正常返回空，不报错** | 大多数页如此。实测 p147 就是这个形态 |

⚠️ 这两条必须各有**独立**用例。只写前者时，把「0 条报告也报错」这个 bug 写进去测试照样绿。

### 3.3 停止条件按日期，不按页码

```go
if oldestOnPage.Before(from) { break }   // 页码随新文章上架而漂移，今天的第 150 页明天就不是
```

`MaxPages`（设 200）仅作兜底，**翻满时报错而不是静默返回**——那说明日期判定没生效。

`--from` 比最新一期还新 ⇒ 第一页就停，**不报错**（正常的「没有要抓的」）。

## 4. 搜索侧扫描（交叉校验路径）

### 4.1 查询构造

```go
const backfillSearchBase = "https://wzdig.pbc.gov.cn/search/pcRender"
const backfillSearchPageID = "c177a85bd02b4114bebebd210809f691"

// backfillSearchKeywords 三个 AND 关键词，各查一遍。
//
// 用 qAll（AND）而非 q（分词 OR）。⚠️ 严格单变量对照实测 2.0–2.5×
// （title+日期窗口 137 vs 276；title 无日期 240 vs 610），**不是曾经写的 25 倍**
// ——那个数混了 searchArea / q-qAll / 日期窗口三个变量，见 requirements-analysis §4。
// advepq / adveq 单用等于空查询（返回全站 549141 条），别用。
var backfillSearchKeywords = []string{
    "金融统计数据报告",
    "社会融资规模存量统计数据报告",
    "社会融资规模增量统计数据报告",
}
```

固定参数：`advSearch=true`、`searchArea=title`、`sr=dateTime desc`、`pNo=<页码>`、
`advtime=5&startTime=<from-01>&endTime=<today>`。

⚠️ **`advtime=5` 是前端已注释掉的未公开参数**（后端 2026-08-14 实测仍接受）。它失效时
**不阻断主路径**：打 WARN、跳过交叉校验、在 final-report 注明。搜索侧是校验不是主路径。

🔴 **「`advtime` 失效时返回全站量级（549141）」是错的**（2026-08-14 由 test-agent-28 实测推翻，
Leader 已逐条复现，四个数字逐字相符）：

| 场景 | 实测 `total-records` |
|---|---|
| 现状 `advtime=5` + 日期范围 | 137 |
| **`advtime` 参数被丢弃** | **240** |
| **`advtime=0`** | **240** |
| `金融统计数据报告` + 无日期过滤 | **1136**（三关键词最大值） |

**`advtime` 失效返回的是「该关键词的全部历史结果」（240 / 1136），不是全站量级。**
`549141` 是**无关键词空查询**的值 —— 那是**另一种**失效模式，且实测那种模式连 `total-records`
字段都取不到，会被「计数字段缺失 ⇒ 报错」拦下。

**后果**：据 549141 设的 5000 上界，距真实失效量级（最大 1136）差 4.4 倍
⇒ **该守卫永远不会因 advtime 失效而触发**，而它的错误文案恰恰指认 advtime。
已转 TASK-004 `review_fix`（FIX-1）：改用**「解析出的每条 `Published` 必须落在 `[from, to]` 内」**
—— 检测**结果的性质**而非**数量**，与语料规模无关。

⚠️ 通用形式：**用「数量异常」当守卫，需要先知道异常时的数量是多少**；而那个数往往没人实测过，
于是阈值订在一个永远够不到的地方。**直接检测「约束有没有被满足」比检测「结果像不像话」可靠。**


### 4.2 结果解析

```go
// backfillSearchItemRE 一条搜索结果：绝对 URL / 标题（含 <font> 高亮）/ 发布日期。
var backfillSearchItemRE = regexp.MustCompile(
    `(?s)<h3>\s*<a href="([^"]+)"[^>]*>(.*?)</a>.*?<span>(\d{4})年(\d{2})月(\d{2})日</span>`)

// 总条数 / 总页数
var backfillSearchTotalRE = regexp.MustCompile(`id="default-result-total-pages" value="(\d+)"`)
```

标题里嵌 `<font color='#FF0000'>` 高亮标签，**用既有 `tagRE` 剥**。

### 4.3 栏目前缀筛（去掉调查统计司的重复份）

实测同一篇报告在两个栏目各有一份：

```
/goutongjiaoliu/113456/113469/5837468/index.html                           ← 保留
/diaochatongjisi/116219/116225/35ec0aa27604417888826e7ff128cc4a/index.html ← 丢弃
```

**只保留 `/goutongjiaoliu/113456/113469/` 前缀**，使搜索侧与 index 侧的 article_id 可直接比对
（第 5 节的前提）。

🔴 **判据必须是栏目前缀，不能是 article_id 形态**（dev-agent-57 在 TASK-004 实测纠正 Leader 的错误描述）。
本文件原先写「调统司那份 article_id 是 32 位 hex」——**那只对 1/3 的样本**：实测 6 条 `/diaochatongjisi/`
里只有 **2 条**是 32 位 hex（`35ec0aa2…`），另 **4 条是 19 位纯数字**（`2025080618505078072`），
**与 goutongjiaoliu 侧的 id 形态完全撞形**。

Leader 的错因：只看了搜索结果第 1 页的前 4 条，前两对恰好都是 hex，就把它概括成了 id 形态规律。
**这是「用样本形状当契约」** —— 与 `discover.go` 注释里称为「本包最贵的一次」的那个错
（`\d{14,}`，实测 19 位就写死下界）**同一形状**。

**后果**：按 id 形态筛会放进那 4 条，而它们的 id 在 index 侧根本不存在 ⇒ 第 5 节交叉校验的差集
**凭空多出 4 条假信号**。而差集正是本 sprint 用来发现「谁漏了」的那个机制 —— **污染判据比污染数据更糟**。

🔬 **成因是结构性的，不是「我不够谨慎」**（dev-agent-57 在交付中指出，比「个人失误」这个定性有用得多）：

央行两个栏目的 article_id 形态**按站点重建时间分层**（2026-06-26 重建前是 19 位时间戳，重建后是短数字/hex），
而搜索结果**按发布时间倒序** ⇒ **第 1 页前几条必然全是重建后的新形态**。

⇒ 「翻第 1 页看几眼」这个动作，在这个语料上**系统性地**采不到旧形态。它会打到任何这么做的人身上——
dev-agent-57 自己也说，它撞见那 4 条 19 位纯数字，是因为要写 `functional[2]` 的断言、**必须逐条列出全部 12 条**，
不是因为它更谨慎。

⇒ **判据不是「这个样本是真的吗」，而是「这个样本占生产语料的几分之几」。**
前者恒为真（我看到的那 4 条确实都是 hex），后者才能暴露分层。


### 4.4 同守「0 条 ⇒ 报错」

搜索返回 0 条结果 ⇒ 报错，与 index 侧同则（检索服务改版同样是静默失效）。

## 5. 交叉校验（人类裁决 2）

筛完前缀后两侧 URL 同形态 ⇒ **以 article_id 为键**直接比对（index 侧的站内路径先经
`resolveURL` 归一）。

```
A = index 侧候选集      B = 搜索侧候选集
抓取集 = A ∪ B          ← 宁可多抓，不漏
A \ B  → 搜索没索到的（已知会发生：实测 137 vs 158 差 21 条）
B \ A  → index 没翻到的 ← 这才是真风险信号
```

两个差集**都写进 manifest**（`only_in_index[]` / `only_in_search[]`），并在 final-report 报告。
`B \ A` 非空是**告警而非失败**——它意味着 index 翻页漏了，但抓取集已包含它们（∪），交付物不缺。

## 6. manifest

```json
{
  "from": "2020-01",
  "scanned_at": "2026-08-14T10:00:00Z",
  "pages_scanned": 152,
  "search_pages_scanned": 82,
  "articles": [
    {"id": "2025092212551391606", "title": "2021年2月金融统计数据报告",
     "published": "2021-03-10", "url": "https://www.pbc.gov.cn/…",
     "file": "articles/2025092212551391606.html", "sha256": "…",
     "fetched_at": "2026-08-14T10:03:22Z", "source": "index|search|both"}
  ],
  "failed": [{"id": "…", "url": "…", "error": "HTTP 404"}],
  "only_in_index": ["…"],
  "only_in_search": ["…"],
  "search_skipped_reason": ""
}
```

`sha256` 用于重抓时检测内容是否变了（央行改版过一次，再改一次时这是唯一能发现的途径）。

🔴 **`search_skipped_reason` 是 2026-08-14 补上的**：ADR-M1c1-02 从一开始就要求
「跳过必须打 WARN 并**写进 manifest**」，但**本节的 JSON 形状漏了它**，TASK-003 的 DoD
列举新增字段时也漏了 ⇒ 实现方按较具体的那个（JSON 形状）做了，字段不存在。
由 dev-agent-57 在 TASK-005 交付时发现：**它返回了这个值，而没人能存**。
⇒ **同一份设计的两个载体不一致时，实现方会按较具体的那个做，而缺口要到下游消费时才暴露。**
该字段的全部意义是让「**这次没做校验**」与「**校验通过**」在读者看来不一样 —— 两头落空等于机制不存在。

**逐篇追加落盘**（`non_functional[1]`）：每抓完一篇立刻重写整份 manifest，**不是跑完才写**。
400+ 请求中途断掉（网络、Ctrl-C）不该让前面的白抓。

**断点续抓**：重跑时「已在 manifest 且文件存在」的直接跳过，**不重发请求**（`functional[5]`）。

## 7. 抓取

### 7.1 限速

```go
// 央行站点没有公开的速率限制。1 req/s 是自我约束：
// 回填是一次性动作，跑 9 分钟完全可以接受，没有理由更快。
const backfillInterval = time.Second
```

约 150 index + 82 搜索 + 约 250 文章 ≈ **482 次请求 ≈ 9 分钟**。

**可注入的 sleep**（`func(time.Duration)`），测试注入记录器断言「两次请求之间调用了 sleep」
（`non_functional[0]`）——不引入 fake clock。

### 7.2 两类错误处置相反（刻意的）

| 错误 | 处置 | 理由 |
|---|---|---|
| **单篇抓取失败**（网络抖动、404） | 记入 manifest 的 `failed[]` 并**继续**，最后返回**非零** | 外部世界的事 |
| **落盘失败**（磁盘满、权限） | **立刻中止**，不继续抓 | 本机的事——继续抓只会浪费请求，而且后面每一篇都会失败 |

### 7.3 index 页与搜索页也存盘

它们同样不可再生，存了之后重跑与 M1c-2 的核对都能离线做。

## 8. 产物位置（人类裁决 3）

```
~/hestia-backfill-2026-08-14/        ← 仓库外绝对路径
  manifest.json
  index/11040-<N>.html
  search/<keyword-slug>-p<N>.html
  articles/<article_id>.html
```

约 15MB。**仓库外**的三个理由：worktree 拆掉产物仍在；不可能误提交；会话外备份直接 `cp -r`。

只有那两份测试样本（p146 约 38KB + 搜索结果 1 页）进 git。

## 9. 命令

```
atlas hestia backfill fetch --from 2020-01 --out ~/hestia-backfill-2026-08-14
```

`--from` 格式 `YYYY-MM`。`--out` 必填（无默认值，避免误落进仓库）。

## 10. 交付验收（真跑一次）

回答三个问题——**它们是 M1c-2 的直接输入**：

1. **一共抓到多少期、多少篇？** 期数应接近 80（2020-01 至今）
2. **`rule@v1` → `rule@v2` 的切换点在哪一期？** 由「某期开始只有一篇」自然呈现，不需预先知道
3. **哪些期缺篇？** v1 期次三篇不齐的，M1c-3 入库时会缺字段；这份清单决定 M1c-4 的工作量

外加交叉校验的两个差集。抓完由人类在会话外**立刻单独备份**。

## 11. 每个任务收尾的硬性检查

`gofmt -l`、`go vet ./...`、`go test ./...` 必须干净。注释里引用任务编号**带 milestone 前缀**
（写 `M1c-1 的 TASK-003`）。测试文件的 `import` 按需增补，别把后面 Step 才用到的包提前写进去。
