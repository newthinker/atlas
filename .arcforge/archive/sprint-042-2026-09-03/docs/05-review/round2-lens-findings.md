# Sprint M1c-4 · 第二轮补遗 —— 三个 lens 子代理的发现与我的核实状态

> **provenance**：内容由 **`qa-m1c4`** 汇总，落盘实例 `qa-m1c4-r4`（`qa-m1c4` 的 token 与矩阵
> 登记值不符，成因在 Leader 侧且**未查实**，Leader 已如实记「成因未查实，不编」）。
> **同一个实例，身份轮换不是返工。**

**锚** `c6f33b71d9d123a8110c668a75b451a8fd206a49` ⚠️ TASK-014 的 `writes` 含此处 5 个文件,合入后须重采。

## 为什么有这份补遗

`round1-review.md` / `round2-adversarial.md` 落盘时,三个 lens 子代理(Skeptic / Architect /
Minimalist)尚未把清单带回,那两份报告的结论**全部出自我本体的实验**。三份清单随后到达(共 38 条)。
**本文件是补遗,不推翻前两份**;但其中两条**修正了 `round2-adversarial.md` 的措辞**,见下。

🔴 **每一条都标注了我的核实状态。** 判据是我自己的观察,不是转述:

| 标记 | 含义 |
|---|---|
| **已独立核实** | 我自己读代码/跑实验确认过,贴了观察 |
| **机制已核实、数字未核** | 我确认了「这个机制确实这样工作」,但**没有独立复算它给的数字** |
| **仅转述** | 我没有核实,原样转达并**标明来源**,请勿据它下判定 |

---

## 🔴 A. 修正 `round2-adversarial.md` 的两处措辞

### A-1 「族内量级核对」是**告警级**,不阻断 —— 我原文把两道防线并列,是不准确的

**来源** Architect A1 · **状态:已独立核实**

```
$ grep -n 'viol\|famViol' internal/hestia/backfill_load_report.go
187:  viol, st := checkCaliberRouting(res.Groups)          ← renderLoadReport 的局部变量
227:  famViol, famInsuf := checkCaliberFamilyMedians(...)  ← 同上
$ sed -n '317,340p' internal/hestia/backfill_load_report.go
func checkLoadIdentities(res *BackfillLoadResult) error {   ← 只读 6 个计数器
    ... res.MergedRowsCounted / MergedGroups / DBMergedRows / Merged / ToObservations / ToPending
```

⇒ `viol` 与 `famViol` **不经任何路径到达 `checkLoadIdentities`** ⇒ 整族位移时它只多印几行,
**数据照样落进 `hestia_observations`,退出码 0**。

🔴 **我在 `round2-adversarial.md` 的 R2-1 里写「两道实测有效的自动防线」,这个并列不准确。** 精确的说法是:

| 防线 | 性质 | M5/M6/M11 实测 |
|---|---|---|
| **`magnitude_sanity`** | 🔴 **真闸门** —— 观测直接落 pending,进不了权威表 | 拒收 **10 / 10 / 37 个观测** |
| 族内量级核对 | **告警级** —— 只打印,不影响退出码、不阻断入库 | 报 **2 / 4 / 4 违反** |

⚠️ **R2-1 的结论不变**(CONTRACTS §G-a 的「完全无闸区」仍是假的 —— `magnitude_sanity` 真的挡住了),
**但支撑它的只有一道闸,不是两道。** 另一道是给人看的。

⚠️ **Architect 还指出 CONTRACTS §B 把它与四道恒等式并排成「三条断言/0 违反」** ⇒ 读者会默认同等强制。
建议:要么让违反进 `checkLoadIdentities`,要么把「刻意告警级」这条裁决写进 §G-7 开口清单 —— **现在正文和开口表都没有。**

### A-2 我的 R1-2 说 `m[1] != ""` 是「潜伏」,更可能是**不可达**

**来源** Skeptic MINOR-8 · **状态:机制已核实、我未复算其正则实验**

Skeptic 打印 `loanFlowRE.String()` 确认**期次捕获组没有 `?`**(不可选),两种无前缀形态均不匹配,
两个生产调用方传的都是 `flowRE`。⇒ 若成立,那个守卫是**死代码**,而注释 `extract.go:348` 称它
「正是本迭代唯一保留的那条拒绝要防的东西」。

⇒ **与我 M3 变异 SURVIVED、真语料 diff 0 行的观察完全一致,而且给出了更强的解释**:
不是「今天恰好没触发」,是**结构上触发不了**。⚠️ 我未独立复算那两条正则实验,故标为待核。
**处置不变(登记进开口清单),但理由要换成 Skeptic 那条。**

---

## B. 新增 MAJOR —— 其中一条是代码级

### 🔴 B-1 合计句是「二选一」不是「路由」,当月值被静默丢弃 —— **已独立核实**

**来源** Skeptic MAJOR-1 · **位置** `extract.go:407-412` / `:676-682` / `:730-740`

```go
if hasCum {
    m, err := selectUnique(ms, what+"（累计口径）", keepCum, label)
    return m, true, err            // ← 有累计族就返回它，当月族整个不看
}
```

调用方据这**一个**返回值写**一列**:

```go
flowField := FieldDepositFlowYTD
if !flowCum { flowField = FieldDepositFlowMoM }
setFlow(c, flowField, ...)         // ← 只写一列
```

🔴 **同一个 commit 里的 `extractTSFFlowArticle` 是真双写**(`extract.go:1038-1039`):

```go
{cumulative, caliberCumulative, FieldTSFFlowYTD,  "累计口径"},
{current,    caliberCurrent,    FieldTSFFlowMoM,  "当月口径"},
```

⇒ **同一 sprint、同一个概念、两条路径形状相反。**

**为什么这是 MAJOR**:CONTRACTS §G-2 明写本轮「**把两种口径同时原样存下,一个都不丢**」——
**那句话对合计句路径为假**。当一篇同时有累计句与当月句时,当月值有列(`deposit_flow_mom` /
`loan_flow_mom` 本轮新建)却不写进去。
**后果无人可见**:NULL 与「报告本来就没这句」在库里同形;`completeness` 因 `missingCaliberAware`
的同族放松也不报缺。

⚠️ **不是回归** —— 改动前只保累计句且缺累计即拒整篇,现在严格更多。**是实现不完整 + 契约自述不实。**
⚠️ **Skeptic 报的数字(存款 5 期 / 贷款 8 期原文有句而列为空)我未独立复算**,标为待核;
**上面的机制是我自己读代码确认的。**

**建议** TASK-014 仍 `in_progress` 且 `writes` 含 `extract.go` 与 `CONTRACTS.md` ⇒ **这是它的天然落点**,
不必重开已 `verified` 的任务。最小动作:订正 §G-2 那句「一个都不丢」;双写本身可作结转项。

### 🔴 B-2 口径路由核对只在 `backfill load` 批处理路径,`hestia ingest` 每次写入无覆盖

**来源** Architect A2 · **状态:仅转述**(调用点可穷举这一点我未自己跑 AST)

被替换掉的「拒绝整篇」原在**抽取层**,对 `Ingest` 与 `BackfillLoad` **同时**生效;替代品只在后者。
月度 ingest 若误路由,`magnitude_sanity`/`completeness`/`deposit_sum` 三道都会 passed。
⚠️ Architect 自己不判 CRITICAL,理由是 `sectorCaliberOf` 的结构性判据已在 218 篇上核过、§H-5 有 20 个字段级人工核对。**我同意这个分级。**

### B-3 `PrecedingAll` 的 pending 侧 `LIMIT n` 在去重之前生效

**来源** Skeptic MAJOR-3 · **状态:仅转述**(它给了探针:同期 Save 4 次 → pending 4 行 → `PrecedingAll(n=6)` 只返回 1 期)

主键含 `ingested_at` ⇒ 同一期反复失败会留多行 ⇒ 窗口被同一期占满 ⇒ `len(hist) < 3` ⇒
`drift_skipped` ⇒ passed。**「一期反复入不了库」这件事本身把 drift 保护关掉。**
生产库现 21 行 / 21 个不同期次,**尚未显形**(刚切新库),再跑一次 `backfill load` 即显形。
建议 pending 侧 `GROUP BY period HAVING ingested_at = MAX(ingested_at)`。

### B-4 `drift_max = 0.03` 对 **ytd 也**过紧,且 11/20 是「向好方向」被拒

**来源** Skeptic MAJOR-4 · **状态:与我 R2-2 独立同向,细分数字仅转述**

CONTRACTS §C 已登记「对 mom 过紧(12/21)」,但**未登记**:① 对 ytd 同样过紧(8 条);
② **11/20 是残差变小(勾稽变好)反而被拒**(判据 `math.Abs(r-mean)` 是双侧)。
⚠️ Skeptic 还质疑 §C 把 `2025-02` 称作「TASK-008 新增的保护」缺乏支撑(该期 residual 0.0458
低于容差 0.17、也低于 ytd 族 p50 0.0885,仅因低于近期均值 0.1086 被拒)。**这条值得 Leader 复核。**

### B-5 恒等式的两个容差是 Go 常量、不进配置

**来源** Architect A3 · **状态:仅转述**

`caliberIdentityTolerance = 5.0` / `caliberIdentityRelTolerance = 0.02` / `caliberMedianMinSamples = 3`
都是常量;同 sprint 形状完全相同的另一组(`deposit_sum_tolerance_mom` 等)走了配置。
⇒ 改这两个常量 `config_version` 一字不变。**§C 那张「四个容差」表读起来像穷举,实际不是。**

### B-6 §H-1 否掉 ALTER 路径的理由被证否

**来源** Architect A4 · **状态:仅转述,但它给了实测**(`create view v as select * from t` → `alter table t add column b` → `pragma table_info(v)` 得 5 列含 b)

原文「列序与 `observationsDDL()` 不保证一致,而 INSERT 是按列序拼的」——实际 `insertSQL` 逐列拼**列名**,
`Preceding` 显式 SELECT 列表,`store.go:139` 自己就写着「INSERT 显式列名」。
⇒ 本次切库结论不受影响(0 行,删+换是对的),但 §H-1 会让将来那次非空库迁移的执行者**做一项无意义的核对**。

---

## C. 与我已落盘发现重合的(证据强度说明)

| 我的编号 | lens | 关系 |
|---|---|---|
| R1-1(§F 老库开口证否) | Architect A5 | **双源独立** —— 我用真备份库实测,它读的是两条测试的源码。**两条路径不同,结论一致。** |
| R1-4(无条件打印「全部成立 ✓」) | Skeptic MINOR-5 同形 | 独立同向 |
| R2-2(drift_max) | Skeptic MAJOR-4 | 独立同向,它的细分更细 |
| `absTol` 对 `gateCorpLoanReconcile` 无效 | Minimalist / simplifier / Leader 转述 | ⚠️ **我是在 Leader 告知后才去读代码确认的,不是独立发现** —— 如实标注 |

---

## D. MINOR / INFO(只列标题,来源与状态)

**Minimalist**(14 条,MAJOR 3 / MINOR 5 / INFO 6,均**仅转述**除注明外):
`profiles.go:576-582` 注释在引入 commit 上即为假 · `extract.go:331-335` 被删函数的孤儿 doc 成了新函数
godoc 首句 · `validate.go:399` 注释称绝对域而代码在比值域 · `pickCaliberBand`/`pickBandFor` 可合并
(它做了实验:改 4 行包装后测试仍绿,净 -4 行) · 5 处注释点名 AST 里不存在的标识符 ·
yaml `:53`/`:68` 两个不同的数都叫 p95(0.1518 vs 0.1663,真因是后者**排除 1 月**,该矛盾已复制到 4 个文件) ·
yaml 内部与跨文件重复且**分叉已开始** · `TestEveryMomFieldHasAYTDTwin` 被另一条完全包含(判定可删但不值得) ·
死代码 AST 可达性闭包 551 个声明中不可达 6 个,**`tsfFlowTotalRE` 契约自述核实成立** ·
`mustMatch`/`selectUnique` 与 `bandLimitRatio`/`caliberIdentityLimit` **判定不合并**(理由已核) ·
本轮新增结构体字段零消费者 **0 个**(296 个字段全扫)。

**Architect**(15 条):`caliberBand` 自己立的规则被违反(`drift_max` 仍写死读 `in.cfg`) ·
`bandLimitRatio`/`caliberIdentityLimit` 无绑定断言(15 个 `_test.go` 中 `bandLimitRatio` **0 命中**) ·
`TestTemplateTablesDeclareBothColumns` 只守单向 · `--allow-incomplete` 已成生产常态却不进库 ·
`store.go:135`「约 20 涨到 54」已过期(现 76) · **「caliber」双义**(统计口径版本 vs 累计/当月),
建议新概念改名 `basis`,不动已烧进 schema 的 `caliber_version` ·
🔴 **列数是线性不是翻倍**:`32 + 22k`,k=2→76 ✓ k=3→98,我在派活问题里预设的「152」前提不成立 ·
🔴 **契约给的建模理由不是最强的那条**:`profiles.go:300` 只写「加列要动双时态主键」(成本论证,可被
「成本可以付」推翻),而**决定性的正确性论证在 CONTRACTS 里却从未连回这个决定** ——
§H-5「口径是按段的」5 期「存款 ytd × 贷款 mom」⇒ 行级 caliber 列**装不下**;
§B 可判 13 对-观测同观测两族在场 ⇒ 族级 caliber 枚举列也装不下。**两条都是构造上不可能,不是代价太大。**

**Skeptic**(9 条):`config.go` 显式写 `magnitude_ranges: {}` 会被拒,而错误文案自己的建议正是「请把整张表留空」
(两条被引用的守卫都不覆盖这一格) · `required.go:128`/`validate.go:724` 的「54 篇」**单位错**
(实为 54 **段** = 27 篇 × 2 侧;有分部门段的是 80 篇)⚠️ **这与 `extract.go:496` 的「54 篇…54/54 一致、0 例外」
互斥,若按篇读那条「0 例外」的射程只有 54/80** · `backfill_load_report.go:441` 注释仍写「±1 亿元」
(与 26 行之上的取值块直接矛盾) · `pickCaliberBand` 的「两族都齐取 ytd」在生产**不可达**
(权威表 76 条里 deposit 与 corp_loan 两族同时齐**各 0 条**) ·
🔴 **它查过而未发现问题的五处(带分母)**:UTF-8 切点 436 次判定失败 **0 次** ·
NaN/Inf 三种形态构造不出 · `periodGapMonths` 对四种 period_type 正确且有生产实证 ·
`PrecedingAll` 的 top-n 语义正确、`period_type` 不串档 · `cumulativeFlowTail` 的 nil 有测试守住。
⚠️ **它还独立复算了 TASK-011 那道闸的四类计数与 22 项区间取值公式,与 CONTRACTS 登记逐位相同** ——
**那两节的登记是准确的。**

---

## 结论(汇总两轮 + 本补遗)

**第一轮 PASS · 第二轮 PASS** —— **0 CRITICAL**,无回归,无产出错值的路径。

⚠️ **但新增一条代码级 MAJOR(B-1)**:合计句「二选一」使可得的当月值被静默丢弃,
而 CONTRACTS §G-2 自述「两种口径同时原样存下,**一个都不丢**」**对该路径为假**。
**它不改变 PASS**(不是回归、不产错值、13 个任务各自的 DoD 未要求合计句双写),
但**契约自述必须订正**,且双写应作为结转项。

**落点建议**:TASK-014 仍 `in_progress`,`writes` 含 `extract.go` 与 `CONTRACTS.md`
⇒ **B-1、A-1、A-2 三条都能进它,不必重开任何已 `verified` 的任务。**

🔴 **一条方法论**:三个 lens 找到的 MAJOR 与我本体找到的 **几乎不重叠**(只有 §F 那条双源命中)。
⇒ **不是「多跑几个视角提高覆盖率」,是它们各自看的东西根本不同** ——
我做的是变异与真语料 A/B(**动态**),它们做的是 AST 与调用图可达性(**静态**)。
**B-1 那种「两条路径形状相反」的缺陷,变异测试结构上发现不了**:变异只问「改了会不会红」,
而这里没有东西可改 —— **缺的是一段从未写过的代码。**
