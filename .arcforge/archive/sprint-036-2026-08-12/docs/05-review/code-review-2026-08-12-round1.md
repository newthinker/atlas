# Sprint 036 Code Review — 第一轮（常规 reviewer 位）

审查者 `qa-reviewer` · HEAD `c101d6125d76ce1a8863342072a703c4c206d002` · 目标包 `internal/hestia`
全部探针跑在隔离 worktree `/private/tmp/qa-probe-036`（已 `remove --force` + `prune`，`git worktree list` 无残留；主工作区 `internal/hestia/` 零改动）。

准入规则：每条发现必须附可复现的观察（命令 + 实际输出）。纯推理的一律单列在「未经观察的疑虑」，不计入发现清单。

---

## ① 结论：**PASS（附带结转）**

无 CRITICAL。两条 MAJOR **都不是 DoD 违反** —— 它们是 DoD 从未要求过的健壮性边界，按「DoD 是一切测试的唯一依据」不构成退回返工的依据（G33 同形）。建议**不走 `review_fix`**，把两条 MAJOR 作为 CONTRACTS 补录 + M1b-4b 的 DoD 输入。

若裁决规则是「任何 MAJOR ⇒ 不 PASS」，则本轮为 CONTESTED，决定权在人类。我不认为存在需要人类裁的争议 —— **证据是清楚的，分歧只可能在「现在修还是 4b 修」**。

交付质量本身很高：新增代码 100% 语句覆盖、`go vet` / `gofmt` 干净、93.2%（实跑复核，非转述）、四个主循环变异全部 KILLED 且因果指向正确的用例。

---

## ② 发现清单

### 【MAJOR-1】`reportTitleRE` 无边界锚 ⇒ 同期「伴生文章」顶掉真报告，且真报告被静默丢弃

`internal/hestia/discover.go:100`

正则 `(\d{4})年(上半年|\d{1,2}月)?金融统计数据报告` 两端都没有锚，任何**把报告标题当子串包含**的标题都会被解析成同一期的候选。

**观察 1**（独立复算，`go run`）：

```
关于2026年7月金融统计数据报告的说明               -> [… 2026 7月]
2026年7月金融统计数据报告解读                     -> [… 2026 7月]
国新办就2026年上半年金融统计数据报告举行新闻发布会  -> [… 2026 上半年]
央行有关负责人就2026年7月金融统计数据报告答记者问   -> [… 2026 7月]
[图解]2026年7月金融统计数据报告                   -> [… 2026 7月]
```

**观察 2**（跑在真实的 `scanPage` / `Discover` 上，合成 index 页）：

```
scanPage 提取 3 条：
  [0] 2026-07/monthly  "央行有关负责人就2026年7月金融统计数据报告答记者问"
  [1] 2026-07/monthly  "[图解]2026年7月金融统计数据报告"
  [2] 2026-07/monthly  "2026年7月金融统计数据报告"

Discover 候选数 = 1
  [0] period=2026-07 type=monthly id=2026071512340454800
      title="央行有关负责人就2026年7月金融统计数据报告答记者问"
```

`Discover` 的 `seen` 去重（`discover.go:261-264`）按 `period/periodType` 取**第一条**，于是：**真报告连一次 `HasPeriod` 都没被查就被跳过，下游拿到的是伴生文章的 URL 与 article_id。** 无错误、无日志。

**为什么它会稳定地朝坏的方向倒**：伴生文章（答记者问 / 图解 / 发布会）发布**晚于或同日于**报告，index 按时间倒序 ⇒ 伴生文章**排在前面** ⇒ 赢的恒定是错的那条。不是随机的。

**⚠️ 诚实限定**：仓库的两份真实快照里**没有**观察到这种标题。p2 上确有同期伴生项 `国新办举行新闻发布会 介绍2026年上半年货币政策执行和金融统计数据情况`（article_id `2026071518015458230`，18:01，确实排在报告 `…12340454869`，12:34 **之前**），但它含的是「金融统计数据**情况**」，不含「报告」二字，恰好不触发。**所以这是结构性隐患，不是已观测到的线上缺陷。** 这正是 G8 记的形状：真实语料让过宽的实现通过。

**建议修复**（已实测，不破坏现有真实语料）：给正则加 `^…$`。

```
2026年上半年金融统计数据报告                      现行=true  加锚=true    ← 快照真实标题
2025年金融统计数据报告                            现行=true  加锚=true    ← testdata 真实标题
央行有关负责人就2026年7月金融统计数据报告答记者问   现行=true  加锚=false
[图解]2026年7月金融统计数据报告                   现行=true  加锚=false
2026年7月金融统计数据报告解读                     现行=true  加锚=false
```

**不把这个理由写强**：加锚把失效模式从「错的那条静默胜出」换成「带装饰的真标题被静默跳过」—— **两者都是静默的**。真正闭合还需要：`scanPage` 在同一页对同一 `period/periodType` 产出 >1 条时，那本身就是应当被看见的信号（当前是无声吞掉）。

---

### 【MAJOR-2】「命中已入库期次即停」⇒ 其下方的历史空洞永远不会被重新发现

`internal/hestia/discover.go:270-272`

**观察**（真实 `Discover`，库里有 2026-07、缺 2026-06）：

```
候选数=0  实际请求页数=1
calls=[https://www.pbc.gov.cn/goutongjiaoliu/113456/113469/index.html]
```

第 2 页上那份 2026-06 的报告**从未被请求**，且此后每一次运行都会得到同样的结果 —— 空洞是永久的。

**注释的理由不覆盖这个情形**（`discover.go:219`）：

> 为什么碰到已入库的期次就停：index 按时间倒序，再往后只会更旧。

「再往后只会更旧」成立，它证明的是「下方没有更新的东西」；它**没有**证明「下方没有缺的东西」。这是「结论也许对、理由不覆盖」的形状。

**空洞怎么形成**（4a/4b 交界，双方各自都正确）：`Discover` 返回「最近的排在前面」，4b 的 ingest 若按序处理并在中途失败，**较新的 A 已入库、较旧的 B 未入库**。B 既没进 `hestia_observations` 也没进 `hestia_pending` ⇒ `HasPeriod(B)` 为 false 本可救它，**但 Discover 在 A 处就停了，根本走不到 B**。

注意 `HasPeriod` 的文档（`store.go:216`）明确为「pending 期次可重试」做了设计 —— **它把「空洞可能存在」想到了，但只想到了 pending 那一种**。「压根没记录」这一种落在两者之间。

**与 CONTRACTS 已记的 4a/4b 张力不是同一条**：那条讲的是 `article_id` 一级幂等键查的表若含 pending 行会让重试无声失效；本条讲的是 `Discover` 的终止条件本身。两条互不覆盖。

**建议**：至少落进 CONTRACTS + 4b 的 DoD。可选机制化方向（不建议 4a 现在做）：把停止条件从「首个已入库」改成「连续 N 个已入库」，或由 4b 负责显式的空洞探测。

---

### 【MINOR-3】`MaxPages <= 0` 时 `Discover` 静默返回空清单，不报错

**观察**：

```
MaxPages=0 -> got=[] err=<nil> calls=[…/index.html]
```

取回了 index、解析了分页，然后循环 0 次，返回 `nil, nil`。

`Config.validate()`（`config.go:51`）挡了 `max_pages < 1`，但只覆盖 `LoadConfig` 这条路径；`Discover` 是导出函数、`DiscoverCfg` 是导出结构体，4b 完全可能手工装配（现有测试就是这么做的）。而这个失效模式正是 `parsePaging` 的注释立志要避免的那一类 ——「管线看起来在跑却再也发现不了任何东西」，只是更彻底：一页都不扫。

**建议**：`Discover` 开头对 `cfg.MaxPages < 1` 直接返回错误（成本一行）。

---

### 【MINOR-4】`go doc` 守卫的「超范围」理由不成立

dev-agent-50 的理由是「需给 `store.go: Close` 补注释 + 新增 AST 测试，**且整包范围会碰不在它 `writes` 里的 `validate.go`**」。

**观察 A**（只解析 `store.go`）：

```
store.go NewStore             Doc==nil? false
store.go recv.Close           Doc==nil? true     ← 唯一一处
store.go recv.DB              Doc==nil? false
store.go recv.HasPeriod       Doc==nil? false
store.go recv.Preceding       Doc==nil? false
store.go recv.Save            Doc==nil? false
```

**观察 B**（**整包**扫描，套用 `TestPackageExposesNoWriteFunctions` **同一套**「接收者不导出 ⇒ 不构成导出面」的过滤 —— 那段代码就在同一个文件里）：

```
【缺 Doc】store.go -> Close
全包（含导出接收者过滤）缺 Doc 的导出函数/方法总数 = 1
```

`validate.go` 的那个 `Preceding` 是 `func (noHistory) Preceding(...)`（`validate.go:49-51`），**接收者 `noHistory` 不导出**，被同一套过滤排除。

⇒ **即使做整包守卫，也只需要碰 `store.go`（给 `Close` 补一行注释）+ `store_test.go`（加测试）—— 两者都在它的 `writes` 内。** 「会碰到 `validate.go`」这个事实前提是错的（我第一版粗糙探针也得出了「validate.go 会被牵连」，是套上现成过滤后才发现自己错了 —— 故此条用了两个方向的观察）。

**这不改变 dev 的诚实度**（它主动交代了这件事，那是好的），但结论要更正：**这个守卫当时是范围内可实施的**，代价约等于一行注释 + 一条 15 行的 AST 测试。建议开成 4b 的一条任务。

---

### 【MINOR-5】`DiscoverCfg.Timeout` 被装载并强制校验，但全仓无任何消费点

**观察**：

```
internal/hestia/config.go:53:      case c.Discover.Timeout <= 0:                        ← 校验它
internal/hestia/config_test.go:77: assert.Equal(30*time.Second, …)                      ← 断言它被解析
internal/hestia/discover.go:210:   Timeout time.Duration `mapstructure:"timeout"`       ← 定义它
internal/hestia/fetch.go:37:       Timeout: timeout,                                    ← NewPBOCFetcher 自己的入参，与本字段无关
```

`Discover` 函数体一次都没读 `cfg.Timeout`。当前无害（4b 才接线），风险是：4b 装配时**忘了把它传给 `NewPBOCFetcher`，不会有任何东西变红** —— 配置写了 30s、实际生效的是 fetcher 的入参值，而 YAML 看起来完全正常。属「当前正确，正确性依赖一个没被断言钉住的前提」。

**建议**：记进 CONTRACTS，作为 4b 接线的验收项。

---

### 【MINOR-6】`configs/hestia.yaml` 不存在，且 crisis 先例中最要紧的那半没被继承

`config.go:10` 说「Config 是 `configs/hestia.yaml` 的内存形态」，`config.go:13` 说「照 `internal/crisis/config.go` 的先例」。

**观察**：

```
$ ls configs/
config.example.yaml  config.yaml  crisis-monitor.yaml  percentile-watchlist.yaml  prism
$ ls configs/hestia.yaml
ls: configs/hestia.yaml: No such file or directory
```

而 crisis 先例包含这一条（`internal/crisis/config_test.go:19`）：

```go
// 冒烟测试直接加载仓库内的正式配置，保证 yaml 与 struct 永不脱节。
func TestLoadConfigFromRepoFile(t *testing.T) {
	cfg, err := LoadConfig(filepath.Join("..", "..", "configs", "crisis-monitor.yaml"))
```

hestia 的五条 config 测试**全部**用 `t.TempDir()` 里的合成 YAML ⇒ `mapstructure` tag 与真实配置文件之间没有任何比对。文件不存在很可能是有意的（4b 才产出），但**「照先例」这句话目前只兑现了一半，没兑现的正是先例里承重的那半**。

**建议**：4b 落 `configs/hestia.yaml` 时同步补这条冒烟测试。

---

### 【MINOR-7】`spec §4.3` / `方案报告 4.1 / 5.3.1 / 5.3.2 / 4.6.2` 不在仓库内

派单要求核「`Discover` 的翻页终止条件与 spec §4.3 是否一致」。**核不了**，只能核内部一致性。

**观察**：全仓（排除 archive）对 `spec §4.3` 的引用共 5 处，**全部是下游** —— `discover.go:221`、`discover_test.go:500/542/556`、`TASK-005.json` 的 DoD、`TASK-005` 的 discovery、`independent-review.md`。`.arcforge/docs/01-design/requirements-analysis.md` 只有 79 行且不含 `4.3` / `max_pages` / `MaxPages`。**没有任何一份文件是 §4.3 本身。**

⇒ 六处引用互相印证不构成印证 —— 它们全部下游于同一句未落盘的断言。内部一致性已确认（见下），但「与 spec 一致」这句话目前**无法被任何人复核**，且 spec 若改动不会有任何信号。

---

## ③ 已核实为 PASS 的项（每条带证据，非转述）

| 检查项 | 证据 |
|---|---|
| **导出面守卫历次是追加而非放宽** | `git log -p` 跨 **8 个 commit** 逐条比对两条断言：`4f84965 → 234baea → c693177 → 0918324 → bf0ddf1 → 2b93ccd → 7b49b13 → c101d61`，每次都是 `+` 一项、`assert.Equal` 全集合精确相等原样保留，**从未出现 `Subset`/`Contains`**。当前 reflect 版 5 项、AST 版 12 项 |
| **`LoadConfig` 默认值保持是真守卫，不是 `validate()` 的副作用** | 做了因果下钻。变异 M-A（去掉预填 → `Config{}`）红在 `config_test.go:38` 的 `require.NoError` —— **那是 `validate()` 打的，不是值断言**。追加变异 M-E（预填换成**非零但错误**的值 0.99/0.99/0.99/0.99/99，`validate()` 照过）→ 红在 **`config_test.go:42/44/45/46`**，正是那四条 `assert.Equal(def.X, cfg.X)`。⇒ 值断言确实承重。若只跑 M-A 就会得出「守住了」这个**对的结论加错的理由**（G31 形状） |
| **翻页终止条件内部自洽** | M-B（`return out,nil` → `continue`）→ KILLED，红在 `discover_test.go:564/568`（`calls` 应 2 项却有 3 项）；M-C（`limit > totalPages` → `<`）→ 6 条测试红；M-D（去掉 `seen` 去重）→ KILLED 且**只有** `TestDiscoverDeduplicatesAcrossPages` 红（精准，无外溢）。三条的因果都指向名字相符的用例 |
| **`parsePeriod` 的 `err != nil` 分支确实不可达，dev 拒绝编假用例是对的** | 实测：`2026年１２月…`（全角）不匹配、`2026年٣月…`（阿拉伯-印度）不匹配、`2026年100月…` 不匹配（`{1,2}` 上限）、`2026年99月…` 匹配且 `Atoi=(99, <nil>)` 后被 `n>12` 拒。⇒ `\d{1,2}` 下 `Atoi` 必成功，注释里「放宽长度上限它立刻可达」的说法成立。**编一个「看起来在测 Atoi 失败」的用例只会制造假守卫**，该取舍完全支持 |
| **`go doc` 那处已修好** | `go doc ./internal/hestia Store.HasPeriod` 实跑输出完整六段注释（含两处 ⚠️），不再是裸签名 |
| **F8 存量缺陷已修且防了回退** | `store_test.go:1751` 与 `:1862` 均为 `require.NotNil(t, errors.Unwrap(err), …)`；`:1747` 与 `:1842-1848` 留了「别写回 `NotErrorIs`」的说明。全文件已无 `NotErrorIs` 的实际调用 |
| **新增代码覆盖率** | `go tool cover -func`：`LoadConfig` / `validate` / `parsePaging` / `pageURL` / `resolveURL` / `parsePeriod` / `scanPage` / `Discover` / `HasPeriod` **全部 100.0%**；total 93.2% |
| **静态检查** | `go vet ./internal/hestia/...` exit 0；`gofmt -l internal/hestia/` 无输出 |
| **并发安全** | `Discover` 无共享可变状态（`seen` 为局部）；包级正则编译一次、并发只读安全；`LoadConfig` 用 `viper.New()` 每次新建实例而非包级全局（`config.go:30`），无跨调用状态；`Store` 走 `database/sql`，本身 goroutine-safe。**无发现** |

关于「116+ 次 KILLED 是否真守住了它声称的东西」—— 抽查了 5 个变异（M-A/B/C/D/E），全部下钻到「是哪一条断言红的」而不看聚合数。**5 个里有 1 个（M-A）的红来自兄弟断言而非被声称的那条**，是追加 M-E 才把它坐实的。按这个比例，报告里的 KILLED 计数可信；但「每一条 KILLED 都由它声称的那条断言产生」这个更强的命题，只验证了 5/116。

---

## ④ 未经观察的疑虑（不计入发现）

1. **`articleLinkRE` 的 `(?s)(.*?)</a>` 在缺失 `</a>` 的畸形 HTML 上会跨到下一条链接**，让标题串进下一篇的文本。没有构造出能让它产出**错误候选**（而非解析失败）的输入，故不列为发现。
2. **`pagingRE.FindSubmatch` 只取页面上第一个 `jumpTo(...)`**。分页控件通常有多个（首页/上页/下页/末页），若某个的总页数不同就会取错。两份快照里没有观察到不一致的 `jumpTo`，**也没有验证过它们是否一致**。
3. **`scanPage(html, cfg.IndexURL)` 对每一页都用 index 页当 base**。当前所有链接都是站内绝对路径 ⇒ `ResolveReference` 结果与用本页 URL 当 base 相同。若央行改成相对路径，第 2 页起会解析出错误的绝对 URL。**没有观察到相对路径的样本**，纯推理。
4. **AST 版导出面守卫只看 `FuncDecl`**，导出的**类型**（`Config` / `DiscoverCfg` / `Candidate` / `PeriodChecker` / `Fetcher` 本 Sprint 全是新增）不在其视野内。注释里已声明为已知限制，没有实验证明它会造成实际危害，仅记于此。

---

## ⑤ 给消费者位（第 2 轮）的编排信息

按 G34 漏洞一，**没有**也不应把上面的发现给它。但有一条是编排信息而非发现：`hestia.Discover` / `hestia.LoadConfig` / `hestia.NewPBOCFetcher` 在**全仓无任何包外调用点**（grep 实测），即 cmd 层完全未接线 —— 消费者位若要「真的走一遍下一步」，需要知道它是在纸面上走，没有现成装配可读。给不给由 Leader 定。
