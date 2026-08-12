# Code Review Round 2 · 消费者位 — 站在 M1b-4b（ingest）实现者的位置回看 Sprint 036

- **审查者**：qa-consumer（QA 第二轮 · 消费者位）
- **日期**：2026-08-12
- **对象**：HEAD `c101d6125d76ce1a8863342072a703c4c206d002`，目标包 `internal/hestia`
- **与之并列**：qa-reviewer 的第一轮在 `docs/05-review/code-review-2026-08-12-round1.md`
- **仓库改动**：零（`git status` 中 `internal/` 干净；`.arcforge/` 的在途改动不是我的）

> **本文件的读法**：正文是 QA 报告；**第 ④ 节（材料读取实况）与第 ⑤ 节（来源标注）是本次实验的控制变量记录**，
> 归档时请与正文一并保留 —— 没有它们，正文里「哪些覆盖是这个位置独有的」无法判断。

---

## 我实际做了什么（方法）

我没有逐条审代码，而是**把 4b 真的写了一遍**：在隔离副本
`<scratchpad>/play`（`cp` 了 `go.mod`/`go.sum`/`internal/hestia`/`internal/macro/bitemporal`，主仓库一个字节没碰）
里写了 15 个探针，其中 ingest 原型就是这段：

```go
raw, _ := f.Get(ctx, c.URL); obs, err := Parse(raw)
obs.Meta.ArticleID = c.ArticleID
rep, err := Validate(ctx, obs, st, cfg.Thresholds); st.Save(ctx, obs, rep)
```

探针源码在 `<scratchpad>/play/probes/zz_qaconsumer_probe*_test.go`（放回 `internal/hestia/` 即可复跑）。

**并且我去线上取了证**：直连（`curl --noproxy '*'`）抓了该栏目 **p1、p3–p24 共 22 页**与 3 篇正文页，全部 HTTP 200。
这一步是本报告一半发现的来源 —— 仓库里的两份 index 快照（p1 无报告条目、p2 只有一条上半年报告）
**恰好不含生产中占 10/12 的那两种形态**，所以从任何一个任务内部都看不见下面的 C1/C2。

**基线**：`go test ./internal/hestia/`（移除我的探针后）→ `ok`。**下面每一条发现都不会让任何现存测试变红。**

---

## ① 覆盖表 —— 检查了 15 条跨任务边界

| # | 边界 | 结论 | 依据（可复现） | 来源 |
|---|---|---|---|---|
| B1 | **`[]Candidate` 的字段够不够 ingest 用** | ✅ **够，且刚好够** | P6：`Parse` 填齐 Period/PeriodType/PublishedAt/CaliberVersion/Extractor，只缺 `ArticleID`；补上后 `Meta.validate()` 返回 `nil`，54 字段全出。`Candidate.URL` 是 `resolveURL` 后的绝对 URL，直接可 `f.Get` | [起自预置提示]（起手清单 #1） |
| B2 | **`article_id` 一级幂等键该查哪张表 / 查的表含 pending 会怎样** | ⚠️ **CONTRACTS 那条张力实测成立，且它低估了代价** | P5/P8，见 W3 | [起自预置提示 → 自行深入] |
| B3 | **`DiscoverCfg`/`Config` 够不够装配可运行命令** | ❌ **不够**：无 db 路径、无历史下界；`configs/hestia.yaml` 在仓库里**不存在** | P9 + `ls configs/`，见 W4 | [起自预置提示 → 自行深入] |
| B4 | **discover 侧与 parse 侧的标题文法是否同一套** | ❌ **两套，宽严不同**（discover 无锚 / parse `\A…\z`） | P2：8 个标题里 3 个「discover 收、parse 拒」 | [自行发现] |
| B5 | **候选 period 与正文解析出的 period 不一致时** | ❌ **静默永久重发现循环，无任何报错** | P7 | [自行发现] |
| B6 | **生产中真实存在哪些报告形态** | ❌ **12 种/年里 discover 只认 10 种、parse 只认 2 种** | 线上 22 页普查，见 C1/C2 | [自行发现] |
| B7 | **修订版（同期次、更晚 published_at）能否被发现** | ❌ **结构上不可达** | P10：第二轮 `Discover` 返回 **0 条**；绕过 discover 直接喂则 `verdict=Revision` 正常入库 | [自行发现] |
| B8 | **候选的处理顺序会不会改变闸门结论** | ⚠️ **会，且静默** | P4：新→旧两期都 `stock_continuity{no_prior_period}`；旧→新则第二期真跑了该闸。两种顺序都 `Passed=true` | [自行发现] |
| B9 | **同一期反复未过闸时的账怎么记** | ⚠️ pending 每次 +1 行，`HasPeriod` 恒 false，observations 里查不到该 `article_id` | P8：3 次 Save → pending 3 行 | [起自预置提示 → 自行深入] |
| B10 | **`Fetcher` 能不能被 4b 加限速/UA/重试** | ✅ 能，单方法接口，我自己就包了一个 `mapFetcher` | P1 起全部探针 | [自行发现] |
| B11 | **`fetch.go` 对非 200 / 超大响应的处置** | ✅ **到位**：非 200 带状态码报错、10MB 上限、`NewRequestWithContext` | `internal/hestia/fetch.go:57-70` | [自行发现] |
| B12 | **`Store` 并发契约与 4b 的关系** | ✅ 顺序 ingest 无冲突；若 4b 并行抓取，**同业务键的 Save 必须串行**（包注释已写明 TOCTOU，`-race` 看不见） | `go doc` 包注释「并发契约」节 | [自行发现]（依据是源码注释） |
| B13 | **2020（6 节 rule@v1）老模板能否走通全链路** | ✅ **能**：`completeness passed`，落 observations | P5 | [自行发现] |
| B14 | **`Duplicate` 路径对重跑的影响** | ⚠️ 已登记（G3）：重复 ingest 判 `Duplicate` → 只刷 article_id，新抽取值丢弃，**返回 nil** | P5：第 2/3 次 ingest → `verdict=Duplicate table=hestia_observations err=<nil>` | [自行发现，但属已登记问题]（见 ⑤ 节说明） |
| B15 | **`ConfigVersion` 有没有落点** | ⚠️ 全仓库零消费者，4b 也没有地方写它（`Meta` 无该列） | `grep -rn ConfigVersion --include=*.go` → 只有 config.go 与 config_test.go | [起自预置提示 → 自行深入] |

**结论分布：无问题 5 条（B1/B10/B11/B12/B13）、有问题 10 条。**
无问题那 5 条不是「没看」—— B1 是跑通了整条 ingest、B11 是逐行读了 `fetch.go`、B13 是真把 2020 那份喂进库看结果。

---

## ② 发现清单

### 🔴 C1（CRITICAL）月报：discover 产出、Parse 恒拒 —— 每年 8/12 的报告陷入永久重试循环 · [自行发现]

**线上取证**（22 页，覆盖 2025-05-14 ～ 2026-07-15 共 16 个月，15 条报告，一条不漏）：

```
2026-07-15 2026年上半年金融统计数据报告     2026-06-12 2026年5月金融统计数据报告
2026-05-14 2026年4月金融统计数据报告        2026-04-13 2026年一季度金融统计数据报告
2026-03-13 2026年2月金融统计数据报告        2026-02-13 2026年1月金融统计数据报告
2026-01-15 2025年金融统计数据报告           2025-12-12 2025年11月金融统计数据报告
2025-11-13 2025年10月金融统计数据报告       2025-10-15 2025年前三季度金融统计数据报告
2025-09-12 2025年8月金融统计数据报告        2025-08-13 2025年7月金融统计数据报告
2025-07-14 2025年上半年金融统计数据报告     2025-06-13 2025年5月金融统计数据报告
2025-05-14 2025年4月金融统计数据报告
```

⇒ 年度节律固定为 **8 篇月报 + 一季度 + 上半年 + 前三季度 + 年报 = 12 篇**（3/6/9/12 月无月报，被季/半/年报替代）。

**用今天线上的真实字节跑**（P11/P13/P14）：

```
scanPage(p4 真实页) → 1 条: 2026-05/monthly「2026年5月金融统计数据报告」   ← discover 收下了
Parse(真实 2026 年 5 月月报正文) → err: period_type monthly is not supported yet …
第二轮 Discover → 仍返回 [2026-07/monthly]                                  ← P1，循环坐实
```

**对 4b 的影响**：

- 系统唯一的「已处理」记忆是 observations 表；月报永远进不去 ⇒ **每次唤起都重新发现、重新下载正文、重新报同一个错，无限期**。
- 4b 必须当场决定：**单条候选失败是中止本轮还是继续**。若中止，因为月报总是最新的那条，**整条管线永远走不到后面的 h1/年报**，
  而这个决定不在任何 DoD 或契约里。
- **「拿到月报样本就删掉 `checkPeriodTypeSupported` 即可」这个预期是错的**：真实月报是 **7 节**（比年报少「国家外汇储备余额」那节），
  `detectExtractor` 直接报 `unrecognized layout: 7 sections, tsf_section=true`（P14）。
  支持月报 = **新增第三档模板 + 孪生句消歧 + 新 completeness profile**，不是删一个函数。

### 🔴 C2（CRITICAL）季度报：两侧文法都不认 —— 每年 2/12 **静默**消失 · [自行发现]

```
P12：2026年一季度金融统计数据报告    discover=false  parseTitle=false
     2025年前三季度金融统计数据报告   discover=false  parseTitle=false
P11：scanPage(p7 真实页)  → 0 条      scanPage(p18 真实页) → 0 条
```

`reportTitleRE` 的期次段是 `(上半年|\d{1,2}月)?`，「一季度」「前三季度」都不匹配；可选组吞成空之后又要求
「金融统计数据报告」紧跟「YYYY年」⇒ 整条不命中，**安静跳过、零报错**。

**这正是 `discover.go:84-87` 那段注释亲手描述的失效模式**（「会让每年 1 月的年度数据被静默跳过：不报任何错，
只是看起来『今天没有新文章』」）—— 年报那一半被实测推翻并修好了，**季报这一半原样留着，因为两份 index 快照里没有它**。

**修复成本已量化**（P15）：一季度正文 **8 节、`detectExtractor` 判 `rule@v2`**，结构与年报**完全同构**；
唯一卡点是 `profiles.go:70` 的 `cumulativePeriods = {全年, 上半年}` 不含「一季度」「前三季度」，于是 `extractFields`
响亮失败：`人民币存款期内合计 not found among 0 candidate sentence(s)`。
⇒ 支持季报需动四处：标题正则、`validPeriodTypes`、`periodEndMonth`（q1→03、q3→09）、`cumulativePeriods`。
**这四处全在 M1b-1/M1b-2 的地盘，不是 4b 能单方面决定的。**

> **C1+C2 合起来是本报告的头条**：按当前交付，管线**每年只能真正入库 2 篇**（上半年 + 年报），
> 8 篇陷入永久失败循环，2 篇静默不可见。**而这一条不会让任何测试变红**，因为两份快照恰好只含 h1 一种形态。

### 🟠 W1（WARNING）修订版结构上不可达 —— 双时态修订链没有生产者 · [自行发现]

P10：某期入库后，把「同期次、新 article_id、更晚 PubDate」的修订版挂在列表最前 → **`Discover` 返回 0 条**。
绕过 discover 直接喂 → `verdict=Revision`、observations 变 2 行 ⇒ **Store 侧完全支持，是 discover 够不着**。

判停规则的理由（「index 按时间倒序，再往后只会更旧」）对**期次**成立，对**同一期的新文章**不成立 ——
修订版恰恰出现在最前面。同理 `refreshArticleID`（站点迁移换 id）在正常路径上也永不触发。
两个决定各自正确，空白在交界处。⇒ **4b 必须回答：修订走不走正常路径？不走的话 bitemporal 那套的投入由谁兑现？**

### 🟠 W2（WARNING）候选的处理顺序决定历史闸门跑不跑，而没人说要倒序 · [自行发现]

P4（同 period_type 的两期，同一份真实年报改期次）：

```
新→旧: 2025-12 passed=true skipped=[stock_continuity{no_prior_period} …]
       2024-12 passed=true skipped=[stock_continuity{no_prior_period} …]
旧→新: 2024-12 passed=true skipped=[stock_continuity{no_prior_period} …]
       2025-12 passed=true skipped=[magnitude_sanity{not_calibrated}]   ← 闸门真跑了
```

`Discover` 的文档明写「最近的排在前面」，`Validate` 的 `passed` 只被 `CheckFailed` 拉低（`validate.go:101`），
**skipped 不影响 Passed** ⇒ 直接 `for _, c := range cands` 顺着写，首跑全量回填会让 `stock_continuity` 与
`deposit_sum` 的漂移检测**一次都没真正执行**，而数据照样进权威表、报告照样 `Passed=true`、零告警。
⇒ 4b 必须反转切片（或按 period 升序），**且这条应当进 DoD**，因为它没有任何机制守着。

### 🟠 W3（WARNING）pending 的账：CONTRACTS 那条张力成立，但它低估了「选项一」的代价 · [起自预置提示 → 自行深入]

P8（同一期连续 3 次未过闸）：`pending 行数=3  HasPeriod=false  observations 中该 article_id 命中数=0`
P5（三种查法的答案，对已过闸的期）：`observations→1  pending→0  v_hestia_current→1`

⇒ 逐条确认：

- 一级键**只查 observations**（CONTRACTS 的选项一）：pending 期次会被重抓 —— 但代价**不止「一次 HTTP」**，
  还有**每轮一行 pending**。6 小时一跑 ⇒ 单个卡住的期次每年 ~1460 行几乎相同的记录，
  而 pending 的唯一消费者是人（`schema.go:61`）⇒ **诊断面被自己灌满**。
- 一级键**连 pending 一起查**：那期永不重试，`HasPeriod` 刻意保留的「允许重试」在 4b 悄悄失效 —— 契约已预言，实测坐实。

**可落地的第三条路（不需要改任何已 verified 的代码）**：4b 用只读 `DB()` 自己做重试记账 ——
`SELECT count(*) FROM hestia_pending WHERE period=? AND period_type=? AND article_id=?`，≥N 次就跳过并记日志。
我已实测该查询经 `Querier` 可用（P5）。

### 🟠 W4（WARNING）`Config` 装配不出一个可运行的命令 · [起自预置提示 → 自行深入]

P9 实测四种 YAML 写法：

```
timeout 写 10s     → Timeout=10s   ✅
timeout 写裸数字 30 → Timeout=30ns  ⚠️ 校验通过（validate 只查 >0）
多写 storage.path / rate_limit → err=nil，**被静默丢弃**
```

- **无 db 路径**：`crisis.Config` 有 `Storage.Path`（`cmd/atlas/crisis.go:95` 就是 `crisis.NewStore(ccfg.Storage.Path)`），
  而 hestia `Config` 只有 `ConfigVersion/Discover/Thresholds`。config.go 的注释明写「照 internal/crisis/config.go 的先例」——
  **先例的关键一半没照到**。4b 要么加字段（回头动 TASK-007 已 verified 的文件），要么走 flag（与先例分叉）。
- **未知键静默丢弃**：谁把 `storage.path` 写进 YAML 会得到**沉默**而不是报错。
- **`configs/hestia.yaml` 在仓库里不存在**（`ls configs/`：只有 config/crisis-monitor/percentile-watchlist/prism），
  而 `LoadConfig` 找不到文件即报错 ⇒ 4b 还要顺手写这份 YAML，且它**不在任何任务的 `writes` 里**。
- **无历史下界**：首跑范围只由 `max_pages`（页数）控制，而页↔日期是漂的（实测每页 21–23 天）。
  空库 + `max_pages: 408` ⇒ 408 次 index 请求 + 二十余年的候选。想表达「只抓 2024 年以后」在当前配置里**表达不出来**。

### 🟠 W5（WARNING）两套标题文法：宽的那套会放进 Parse 拒收的候选，且**去重键会把真报告挤掉** · [自行发现]

P2（同一批标题喂两侧）：

```
解读2026年上半年金融统计数据报告        discover=true(2026-06/h1)     parse=false
2026年上半年金融统计数据报告(修订)       discover=true(2026-06/h1)     parse=false
关于印发2025年金融统计数据报告的通知      discover=true(2025-12/annual)  parse=false
国新办…货币政策执行和金融统计数据情况     discover=false                parse=false   ← 这条挡住了 ✅
```

P3（更要命的后果）：让一条「解读…」排在真报告**前面** → `Discover` 只返回 **1 条，就是那条解读**。
因为 `seen` 的键是 `period+"/"+periodType`，**同期次的第一条赢**，真报告被安静丢掉，而 Parse 又拒收解读
⇒ 那一期**永久拿不到**，零报错。

⚠️ **诚实限定**：这条的**机制**已实测坐实，但**真实语料未证** —— 我普查的 22 页里没有出现这种标题。
危害等级取决于央行会不会发这类标题。**但 C1 已经是同一形状的实证**（discover 收下 Parse 拒收的候选），
所以「宽严不一致」本身不是假想。
⇒ 建议：4b 在 ingest 里做一次 `c.Period == obs.Meta.Period && c.PeriodType == obs.Meta.PeriodType` 的交叉核对（见 W6），
或让两侧共用同一条正则。

### 🟠 W6（WARNING）候选与正文的期次不一致 → 静默永久循环，且当前**无人核对** · [自行发现]

P7（index 链接文本写 2026 上半年、正文 meta 是 2025 年报）：

```
候选宣称 2026-06/h1，入库结果 table=hestia_observations err=<nil>
入库后 HasPeriod(2026-06,h1) = false     ← 判停键永远不会变 true
下一轮 Discover 仍返回 1 条
```

数据按**正文**的键入库（正确），但 discover 的判停键按**候选**的期次问（也正确），两者对不上时
⇒ 每轮重新发现、重新下载、重新写同一行（`Duplicate`），**永不收敛且完全无声**。
全包目前没有任何一处比对这两个来源。⇒ 4b 应当在 `Parse` 之后立刻断言两者相等，不等就响亮失败。

### 🔵 S1–S5（SUGGESTION）

- **S1** · [起自预置提示 → 自行深入] `timeout: 30`（写的人想写 30 秒）→ `30ns`，`validate` 只有下界 `>0` 放行；
  每次请求都会超时，而配置看着没问题。建议加下界（如 ≥1s）。本仓库其余配置刻意用 `_days` 整数
  （`configs/crisis-monitor.yaml`）绕开了这个坑，hestia 是第一个引入 `time.Duration` 的。
- **S2** · [起自预置提示 → 自行深入] `ConfigVersion` 全仓库零消费者，`Meta` 里也没有对应列 ⇒ 4b 拿到它没地方放。
  请明确它由谁在何时写进契约 JSON。
- **S3** · [自行发现] `TASK-003` 的 discovery 写着「央行每年 1 月同时发年报与 12 月月报，正是同页两条的形态……**不是假想风险**」。
  **16 个月的线上数据里没有 12 月月报**（2026-01-15 只有年报，上一条是 2025-12-12 的 11 月报）。
  那条合成用例本身仍有价值（它测的是「返回整页全部候选」），但**支撑它的那个事实未被观察到** —— 属「结论对、理由错」。
- **S4** · [自行发现] `parse.go:186` 猜月报的累计前缀是「1-5月」，**真实文本是「前五个月」**
  （`前五个月人民币存款增加15.77万亿元`）。谁照着这条注释写正则会写错。
- **S5** · [自行发现] `Discover` 对 index 页无限速地连发（首跑可达 408 次）。可由 4b 包一层 `Fetcher` 解决
  （接口形状允许，已验证），但**没有任何地方提醒 4b 要做这件事**。

---

## ③ 未经观察的判断（纯推理，单列）

1. **首跑规模的估算**：22 年 × 12 篇 ≈ 250+ 候选。依据是「6115 条 / 408 页 / 每页实测 21–23 天」的外推，
   **我只普查了最近 22 页**，更早年份的报告标题格式是否一致**没有验证**。
2. **W5 的真实发生率**：机制已证，真实语料**未见**该形态。
3. **央行是否真的会重发/修订同一期**（W1 的现实权重）：我只证明了「若发生，管线看不见」，**没有证据说明它发生过**。
   Store 侧为此投入了整套双时态设计，这个前提值得单独确认。
4. **月报支持的完整工作量**：只证到「7 节 ⇒ `detectExtractor` 报错」与「孪生句前缀是『前五个月』」，
   **没有**评估孪生句消歧的实际难度。
5. **`v_hestia_current` 与 `hestia_observations` 对 article_id 查询的等价性**：P5 显示两者对同一 article_id 都返回 1，
   但**我没有构造修订链场景**去区分它们（有修订时视图只留最新一行，按 article_id 查旧行会 miss）。
   4b 若用视图做幂等键要自己再验一次。

---

## ④ 材料读取实况（控制变量记录 —— 请勿在归档时删去）

> Leader 的派单写了「输入材料（**请全部读**）」并列了五项。以下是**真实**读取量，
> 是在收到「如实报」这条追加要求时的状态；**我没有为此补读任何材料，也没有改动任何分析**。

| 材料 | 实际读了多少 |
|---|---|
| 1. `.arcforge/docs/02-plan/findings-carryover.md` | **部分**。前 **912 行**（G1–G29 起头处）读了全文；再按 `grep -n "^## G3"` 取到 G30–G35 的**标题清单**，只补读 **1032–1131 行**（G31 尾 + G32 全 + G33 前半）。**G30 / G33 后半 / G34 / G35 正文没读**（含 G34「跨视角对抗必须换位置」—— 我是从派单知道自己位置的，不是从 G34） |
| 2. 七份 `.arcforge/docs/04-test/TASK-00*-verification.md` | **一份都没读，零全文、零片段**。只跑过一次 `grep -rl`（只出文件名、不出内容）。命中只到文件名一级：`monthly` 命中 TASK-004、TASK-006，其余关键词零命中 |
| 3. 七份 `.arcforge/discoveries/TASK-00*.json` | **只读了 TASK-003 的一个片段**（打印「月报」前后约 550 字符，即 S3 的依据）。**其余六份零内容**，同样只有上述 `grep -rl` 的文件名级命中 |
| 4. `internal/hestia/CONTRACTS.md` | **部分，约 100/543 行**：`grep -n` 取全部小节标题；**全文读了 455–543 行**（Sprint 036 整节，含「留给 M1b-4b 的一个张力」+「相关文档」）；另经 `grep -B4 -A4 修订版` 读到 174–186 行附近一段。**其余各节全部未读** |
| 5. 七份 `.arcforge/tasks/TASK-00*.json` 的 `done_criteria` | **一条都没读**。只跑过 `jq -r '.id, .title, .writes'` 取标题与范围声明 |

**不在清单上、但读得最多的是源码**：`discover.go`/`store.go`/`types.go`/`config.go`/`parse.go`/`fetch.go` 全文，
`schema.go`/`validate.go`/`profiles.go`/`extract.go` 片段，`go doc -all ./internal/hestia` 全量导出面。
**本报告的发现基本全长在源码与线上语料上，不长在派单列的五份材料上。**

⇒ **直接后果**：`[起自验证报告]` 在本报告中恒为 **0，且是平凡为真的**（一份都没读），
**不能**拿它支持「验证报告漏了什么」的任何结论。

---

## ⑤ 来源标注汇总（控制变量记录 —— 请勿在归档时删去）

标注口径：`[起自预置提示]` = 追溯得到派单里 dev 交代的三处薄弱点或起手清单三问；
`[起自验证报告]` = 追溯得到七份验证报告；`[自行发现]` = 两者都追溯不到；
`[起自预置提示 → 自行深入]` = 方向来自提示、具体形态自己找到（须写明多出来的部分）。

| 条目 | 来源 | 「→ 自行深入」多出来的是什么 |
|---|---|---|
| C1 月报永久重试循环 | **[自行发现]** | — |
| C2 季报静默消失 | **[自行发现]** | — |
| W1 修订版结构不可达 | **[自行发现]** | — |
| W2 处理顺序决定闸门跑不跑 | **[自行发现]** | — |
| W3 pending 记账 / 一级键选表 | **[起自预置提示 → 自行深入]** | ① 量化「选项一的代价不止一次 HTTP，还有每轮一行 pending，6h 一跑 ≈1460 行/年，而 pending 的唯一消费者是人」；② 「用只读 `DB()` 自己做重试记账」这条不需改已 verified 代码的第三路 |
| W4 Config 装配不出可运行命令 | **[起自预置提示 → 自行深入]** | 清单只问「够不够」；本报告给出四个具体形态：db 路径缺失（对照 `cmd/atlas/crisis.go:95` 先例）、未知键静默丢弃、`configs/hestia.yaml` 根本不存在且不在任何 `writes` 里、无历史下界（页数 ≠ 日期） |
| W5 两套文法 + 去重键挤掉真报告 | **[自行发现]** | — |
| W6 候选/正文期次不一致 | **[自行发现]** | — |
| S1 `timeout: 30` → 30ns 过校验 | **[起自预置提示 → 自行深入]** | 同 W4 血统（查 Config 时实测四种 YAML 写法） |
| S2 ConfigVersion 无落点 | **[起自预置提示 → 自行深入]** | 同上 |
| S3 「年报与 12 月月报同页」未被观察到 | **[自行发现]** | 依据 = grep 出的 TASK-003 discovery 片段 + 线上 16 个月数据；无人指出过 |
| S4 注释猜「1-5月」，实为「前五个月」 | **[自行发现]** | — |
| S5 无限速 | **[自行发现]** | — |

**覆盖表 15 条的来源已逐行标在 ① 节表格最后一列。**

**关于 B14 的认识边界**：我从 `store.go:403` `refreshArticleID` 的注释读到它自称 G3，
**但我没读过任何登记它的文档**，因此**无法判断**它在派单材料里是否已被指出 —— 请按「已知」处理。

**关于派单里「dev 主动交代的三处」**：**一条都没进本报告的发现**。
`go doc` 无守卫、`parsePeriod` 的 `err` 分支不可达、G32 缺失约定，三条都没报；
其中第 2 条在读 `discover.go:119-124` 时确实读到了，**读了但没当发现**，因为它对 4b 无影响。

⇒ **净结果：12 条发现里 4 条（W3/W4/S1/S2）血统来自起手清单三问，8 条与任何预置提示都追溯不上；
「dev 交代的三处」与「七份验证报告」对本报告的产出贡献均为 0** —— 后者是因为没读。

### 对本次对照实验效力的限定（Leader 已采纳）

qa-reviewer 那侧的材料基线破了（自述只读了五份里的一份半），**本侧同样破了，但破法不同**：
它读了 `findings-carryover` 全文而验证报告没读；本侧 `findings-carryover` 读了约 2/3、验证报告同样没读、
**但把源码读了个遍，还去抓了线上语料**。

⇒ 两位的差异里**至少有一部分来自「读的东西不同」**，不只是位置不同。可留的方法论结论只能收窄到：

> **「消费者位 + 直接跑源码与线上语料」这个组合，产出了 8 条与全部预置提示无关的发现。**
> **「位置」单独的贡献，本次实验分不出来** —— 因为两位的材料基线都不是设计的那一份。

---

## ⑥ 结论：作为 4b 的实现者，我现在能不能开工？

**能开工，但不能照着现状开工 —— 因为按当前交付把 4b 写完，管线每年只能真正入库 2 篇报告（上半年 + 年报）：
8 篇月报陷入永久重试循环、2 篇季报静默消失，而这一切不会让任何测试变红、不会产生任何告警。**

**需 Leader / 人类定案的三件**（都超出 4b 自身权限）：

1. **季报（C2）**：要不要支持？支持则需动 `validPeriodTypes` / `periodEndMonth` / 标题正则 / `cumulativePeriods` 四处
   （M1b-1/2 的地盘）。**成本低、收益立竿见影** —— 正文结构已被 `rule@v2` 完整覆盖。
2. **月报（C1）**：短期内不支持是合理的（缺样本），但必须回答「**发现了却抓不了的候选怎么记账**」——
   否则 4b 只能在「每轮报一次硬错」与「静默吞掉」之间选，而两个选项都不该由 4b 独自决定。
3. **修订（W1）**：正常路径要不要能拿到修订版？答「不要」也行，但得写进契约，否则 Store 侧那套双时态是空转的。

**4b 自己就能闭合的**（建议进 DoD）：**倒序处理候选**（W2）、**pending 重试记账走只读 `DB()`**（W3）、
**候选期次 vs 正文期次交叉核对**（W6）、**包一层限速 Fetcher**（S5）。

**配置侧（W4）需要一个决定**：db 路径进 `Config`（照 crisis 先例，但要动已 verified 的 `config.go`），还是走 flag。
另外**得有人认领 `configs/hestia.yaml`** —— 它现在不存在，也不在任何任务的 `writes` 里。

---

## 附：复现入口

- 探针：`<scratchpad>/play/probes/zz_qaconsumer_probe*_test.go`
  （移回副本的 `internal/hestia/` 后 `GOTOOLCHAIN=local GOFLAGS=-mod=mod go test ./internal/hestia/ -run TestProbe -v`）
- 线上素材：`pboc_p4/p7/p18.html`、`pboc_may2026.html`、`pboc_q1.html`，放在同副本的 `testdata/` 下 —— **均未进主仓库**
- 隔离副本是 `cp` 出来的临时目录，随 session 清理；素材可用报告中的 URL 重抓
  （栏目 `https://www.pbc.gov.cn/goutongjiaoliu/113456/113469/`，分页模板 `11040-N.html`）
