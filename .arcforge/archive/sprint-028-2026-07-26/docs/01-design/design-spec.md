# Prism M3 设计规格（接口契约唯一来源）

> 并行任务的**类型一致性基准**。生产方/消费方跨任务时，以本文件签名为准；
> 与 plan 草案冲突处，以本文件 +（`architecture-decisions.md`）为准。
>
> **本文件已含第二轮反审修订**（AD-9~AD-18）：segment_revenue 主键（AD-9）、migrate 并发容错（AD-10）、
> 单遍解析 + 双上限（AD-11）、force 全量重拉（AD-12）、桑基残差节点（AD-13）、Inf 防护（AD-14）、
> 期数上限统一为 10（AD-15）、LoadTemplates 不吞错（AD-16）、FY 行不落库（AD-17）、golden 回归（AD-18）。

## 0. 包与任务映射

| 包 | 任务序列（同包必须串行） |
|---|---|
| `internal/collector/edgar` | TASK-001 → TASK-005 |
| `internal/storage/prism` | TASK-002 |
| `internal/prism/sankey` | TASK-003 → TASK-007 → TASK-009 |
| `internal/prism` | TASK-006 → TASK-008 |
| `cmd/atlas` | TASK-004 → TASK-010 → TASK-011(serve.go) |
| `internal/api/handler/api` + `internal/api`(路由) | TASK-011 |
| `internal/api/handler/web` + 两份模板目录 | TASK-012 → TASK-013 |
| `configs/prism` | TASK-003(msft.yaml) → TASK-014 |
| `docs/` | TASK-015 |

---

## 1. `internal/collector/edgar`

### 1.1 QuarterlyFact 扩字段（TASK-001 产出）

```go
type QuarterlyFact struct {
	FiscalPeriod  string
	PeriodEnd     time.Time
	FilingDate    time.Time
	EPSDiluted    float64
	Revenue       float64
	NetIncome     float64
	Equity        float64
	DilutedShares float64
	// M3 新增（NaN = 缺失）
	GrossProfit     float64
	RnD             float64
	SGnA            float64
	OperatingIncome float64
	IncomeTax       float64
}
```

**tag 链**（提为包级变量，供 segments.go 复用 `revenueTags`）：

```go
var epsTags    = []string{"EarningsPerShareDiluted", "EarningsPerShareBasicAndDiluted"}
var sharesTags = []string{
	"WeightedAverageNumberOfDilutedSharesOutstanding",
	"WeightedAverageNumberOfSharesOutstandingBasic", // basic 近似 diluted，口径注释标注
}
var equityTags = []string{
	"StockholdersEquity",
	"StockholdersEquityIncludingPortionAttributableToNoncontrollingInterest", // AVGO
}
var grossProfitTags = []string{"GrossProfit"}
var costTags        = []string{"CostOfRevenue", "CostOfGoodsAndServicesSold"}
var rndTags         = []string{"ResearchAndDevelopmentExpense"}
var sgnaTags        = []string{"SellingGeneralAndAdministrativeExpense"}
var sgnaSplitTags   = []string{"SellingAndMarketingExpense", "GeneralAndAdministrativeExpense"}
var opIncomeTags    = []string{"OperatingIncomeLoss"}
var taxTags         = []string{"IncomeTaxExpenseBenefit"}
```

**季度化后的推导规则**（顺序执行）：
1. `EPSDiluted` NaN 且 `NetIncome`/`DilutedShares` 有值且 shares≠0 → `EPS = NetIncome / DilutedShares`
   （拆股归一化已在 rawFact 层完成，直接相除）。
2. `GrossProfit` NaN 且 `Revenue`/`Cost` 有值 → `Revenue − Cost`。
3. `SGnA` NaN 且分列（Selling + G&A）有值 → 求和。

duration 型全部经既有 `firstQuarterlyTag(tags...)` + `durationMetric`；`equityTags` 为 instant 型，
按既有 equity 逻辑加同优先级 `firstTag` 回退。

### 1.2 分部解析（TASK-005 产出，TASK-008 消费）

```go
type SegmentPeriod struct {
	PeriodStart, PeriodEnd time.Time
	FilingDate             time.Time
	Form                   string             // "10-Q" | "10-K"
	Values                 map[string]float64 // member local name（去命名空间）→ revenue
}

// NewWithBaseURLs 注入 data host 与 archives host（测试用）；
// 生产 New 默认 https://data.sec.gov + https://www.sec.gov。
func NewWithBaseURLs(userAgent, dataURL, archivesURL string) *Client

// FetchSegmentRevenue 返回 reportDate > since 的报告中、通过期间过滤的各期分部营收。
// axis 为维度轴 local name（模板 segment_axis）。
func (c *Client) FetchSegmentRevenue(cik, axis string, since time.Time) ([]SegmentPeriod, error)
```

**解析规则**（实现注释必须完整保留）：

1. `GET {dataURL}/submissions/CIK{10位补零}.json` → `filings.recent` 平行数组
   （`form`/`accessionNumber`/`primaryDocument`/`reportDate`/`filingDate`），
   筛 `form ∈ {10-Q, 10-K}` 且 `reportDate > since`。
2. 实例文档 URL（AD-3）：
   `{archivesURL}/Archives/edgar/data/{cik整数}/{accession去连字符}/{primaryDocument .htm→_htm.xml}`
3. XML **单遍** `Decoder.Token()` 解析（AD-11：HTTP body 不可二次读取，**"两遍"不可实现**；
   实测单文档 9.7~10.5 MB，**禁止 Unmarshal 建全树**）。一次遍历同时收集两类数据，
   **流结束后在内存中关联**（不假设 context 一定先于 fact 出现）：
   - **contexts**：`<context id=…>` 记录 `period/startDate,endDate`（无 startDate 的 instant 型跳过）
     与 `entity/segment/xbrldi:explicitMember`。收录条件：**恰好一个 explicitMember** 且其
     `dimension` local name == axis（排除 segment×geography 等交叉维，也排除单维但轴不匹配的 context）。
     > **干扰类型的真实分布**（TASK-005 全文档统计，校正 Leader 早前基于局部 grep 的结论）：
     > 409 个 context 中 explicitMember 个数分布为 `{0:16, 1:276, 2:73, 3:44}` ——
     > **多维 context 真实存在 117 个**（Leader 早前「MSFT 没出现交叉维」的说法只对分部轴成立）。
     > 而 276 个单维 context 里只有 **18 个**落在 `StatementBusinessSegmentsAxis`，
     > 其余是 DebtInstrumentAxis(73) / StatementEquityComponentsAxis(68) / ProductOrServiceAxis(48) 等。
     > → **「单维但轴不匹配」才是压倒性的主要干扰源，不是交叉维。两条排除规则缺一不可。**
     >
     > 另注：真实根元素用**默认命名空间**，`context`/`unit`/`period` 全无前缀，只有
     > `xbrldi:explicitMember` 带前缀；而 `dimension` 属性值与 member 文本是 QName **字符串**，
     > `encoding/xml` 不解析其中的前缀，**必须自己按最后一个 `:` 截断**。
     member 值去命名空间前缀（`msft:IntelligentCloudMember` → `IntelligentCloudMember`）。
   - **pending facts**：元素 local name ∈ `revenueTags` 的元素，暂存 `(contextRef, tag, value, unitRef)`。
   - **关联与过滤**（遍历结束后）：`contextRef` 命中已收 context 的 fact 才计入 (period, member)；
     `unitRef` 非 USD 的 fact **排除**；同 (period, member) 多条时取 **tag 链优先级最高者**。
     > **AD-23 单位判定必须解析 `<unit>` 定义，不能只看 `unitRef` 字符串**（TASK-005 live 实测校正）：
     > 计划示例写的 `unitRef="usd"` 是失真的，真实文档用的是**引用 id**（`U_USD` / `U_EUR` / `U_shares`）。
     > 更危险的是真实文档含
     > `U_UnitedStatesOfAmericaDollarsShare -> <divide><unitNumerator><measure>iso4217:USD</measure>…`
     > 即 **USD/share（EPS 的单位）**。若按「measure 含 USD 就算营收单位」的天真判定，
     > **每股值会被当成分部营收**（合法数值、语义完全错误——正是「合法值但语义错误」那一类）。
     > 正确规则：只有**恰好一个直接子 `<measure>`** 且其 local name 为 USD 才算合格；
     > `<divide>` 里的 measure 是孙元素，Go 的直接子元素匹配天然排除它。
     > `<unit>` 定义可能出现在引用它的 fact **之后**，故单位关联同样不能依赖文档顺序。
   - **期间过滤**：`endDate − startDate + 1` 落在 70~100 天（单季）或 350~380 天（FY，含 366 闰年）。
4. **每个通过过滤的期间产出一条 `SegmentPeriod`**（AD-2：一份报告可产多条）；
   同 (period, member) 跨报告重复 → 取 `FilingDate` 更晚者。
5. 请求间**无并发**（EDGAR 限频）；**两个 host 的请求都必须携带 UA**（www.sec.gov 无 UA 同样 403）。
6. **双重上限**（AD-11）：
   - 响应体经 `io.LimitReader` 限 **64 MB**，超限该期跳过并返回可观测的 Degraded 信息（不 OOM）；
   - 单次调用最多处理最近 **12** 份符合条件的报告（防 `since` 为零值时数十份 × ~10MB 的首跑风暴）。
7. **降级**：某份报告的实例文档 404（2019 前的老式非 inline XBRL filing，`_htm.xml` 推导不成立）
   或解析失败 → **跳过该期并记录，不中断整家**。

---

## 2. `internal/storage/prism`（TASK-002 产出）

```go
// fundamental_q 扩列（迁移 + 新库 schema 同步）
type FundamentalRow struct {
	FiscalPeriod, PeriodEnd, FilingDate string
	Revenue, NetIncome, EPSDiluted, Equity, DilutedShares float64
	GrossProfit, RnD, SGnA, OperatingIncome, IncomeTax    float64 // M3 新增
	Source string
}

type SegmentRow struct {
	PeriodEnd    string  // "YYYY-MM-DD" —— 真实主键（AD-9）
	FiscalPeriod string  // "2026Q1" —— 展示标签，可为空
	SegmentKey   string  // 模板 key
	Revenue      float64
	Source       string  // "edgar_segment" | "manual"
}
type PriceRow struct {
	D     string  // "YYYY-MM-DD"
	Close float64
}

func (s *Store) UpsertSegments(instrumentID int64, rows []SegmentRow) error
func (s *Store) SegmentRows(instrumentID int64) ([]SegmentRow, error)      // ORDER BY period_end, segment_key
func (s *Store) LatestSegmentPeriodEnd(instrumentID int64) (string, error) // 无数据返回 ""
func (s *Store) UpsertPrices(instrumentID int64, closes []PriceRow) error
func (s *Store) PriceSeries(symbol, from string) (dates []string, closes []float64, err error)
```

**schema 追加**（进 `const schema`，`CREATE TABLE IF NOT EXISTS`）：

```sql
-- AD-9: PK 用 period_end（真实主键，与 fundamental_q 同源），不用 fiscal_period（展示标签，不可靠）
CREATE TABLE IF NOT EXISTS segment_revenue (
  instrument_id INTEGER NOT NULL REFERENCES instrument(id),
  period_end    TEXT NOT NULL,
  fiscal_period TEXT,
  segment_key   TEXT NOT NULL,
  revenue       REAL,
  source        TEXT,
  PRIMARY KEY (instrument_id, period_end, segment_key)
) WITHOUT ROWID;
CREATE TABLE IF NOT EXISTS price_daily (
  instrument_id INTEGER NOT NULL REFERENCES instrument(id),
  d     TEXT NOT NULL,
  close REAL,
  PRIMARY KEY (instrument_id, d)
) WITHOUT ROWID;
```

**迁移**（`Open` 中 `db.Exec(schema)` 之后调用；仓库首次引入 migration 机制）：

```go
// migrate adds columns introduced after M2 to pre-existing databases.
func migrate(db *sql.DB) error {
	for _, col := range []string{"gross_profit", "rnd", "sgna", "operating_income", "income_tax"} {
		var n int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM pragma_table_info('fundamental_q') WHERE name=?`, col).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			// AD-10: serve(常驻) 与 prism-daily(定时) 会并发 Open 同一库,check-then-act 存在竞态。
			// 另一进程抢先加列时 ALTER 报 "duplicate column name" —— 视为已迁移,继续而非失败。
			if _, err := db.Exec(`ALTER TABLE fundamental_q ADD COLUMN ` + col + ` REAL`); err != nil {
				if !strings.Contains(err.Error(), "duplicate column") {
					return err
				}
			}
		}
	}
	return nil
}
```

`LatestSegmentPeriodEnd` 实现（AD-9：不再 JOIN，消除锚点回退风险）：
```sql
SELECT MAX(period_end) FROM segment_revenue WHERE instrument_id = ?
```

新增方法风格与既有一致：`db.Begin()` → `defer tx.Rollback()` → `tx.Prepare(INSERT … ON CONFLICT(pk)
DO UPDATE SET x=excluded.x)` → 循环 `stmt.Exec(…, toNull(v))` → `tx.Commit()`；查询侧 `sql.NullFloat64`
+ `fromNull`，末尾 `return out, rows.Err()`。

---

## 3. `internal/prism/sankey`

### 3.1 模板（TASK-003 产出）

```go
package sankey

type Segment struct {
	Key    string `mapstructure:"key"`         // AD-1: viper，非 yaml tag
	NameZH string `mapstructure:"name_zh"`
	NameEN string `mapstructure:"name_en"`
	Member string `mapstructure:"xbrl_member"` // 实例文档 explicitMember local name
}
type Template struct {
	Company     string    `mapstructure:"company"`      // symbol
	CIK         string    `mapstructure:"cik"`
	SegmentAxis string    `mapstructure:"segment_axis"` // 默认 StatementBusinessSegmentsAxis
	Segments    []Segment `mapstructure:"segments"`
	Version     int       `mapstructure:"version"`      // 重述断点机制（设计 §10）
	SinceFY     int       `mapstructure:"since_fy"`     // 0 = 全部
}

// LoadTemplates 读取 dir 下 *.yaml，返回 symbol（大写）→ Template。
// 校验：key/xbrl_member 非空且包内唯一；company 非空。
// 目录不存在 → 返回空 map + nil error（未配置模板是合法状态）。
//
// AD-16 错误语义（三种情况必须区分，不可一律吞成空 map）：
//   目录不存在      → 空 map, nil        （未配置，合法）
//   目录存在但为空  → 空 map, nil        （未配置，合法）
//   含非法 YAML     → nil, error         （error 信息**必须含出错的文件名**）
// 调用方（serve.go / cmd prism.go）**禁止写 `templates, _ :=` 丢弃 error**——
// 模板被 rsync --delete 清掉或 YAML 写坏时若静默，表现为 sankey 全站 404 且日志无痕。
func LoadTemplates(dir string) (map[string]*Template, error)

// LoadManualSegments 读 {dir}/{symbol}.yaml：fiscal_period → segment_key → 金额。
// 文件不存在 → 空 map + nil error。
func LoadManualSegments(dir, symbol string) (map[string]map[string]float64, error)
```

`configs/prism/templates/msft.yaml`（首个真实模板，member 名已 live 验证）：

```yaml
# Prism 财报桥模板：只定义分部与 XBRL member 映射；主干流（收入→毛利→营利→净利）
# 由 fundamental_q 自动构建，不在此配置（AD-5）。member 名 = 实例文档 explicitMember 去命名空间。
company: MSFT
cik: "789019"
segment_axis: StatementBusinessSegmentsAxis
version: 1
segments:
  - {key: productivity,       name_zh: 生产力与业务流程, name_en: Productivity and Business Processes, xbrl_member: ProductivityAndBusinessProcessesMember}
  - {key: intelligent_cloud,  name_zh: 智能云,          name_en: Intelligent Cloud,                   xbrl_member: IntelligentCloudMember}
  - {key: personal_computing, name_zh: 更多个人计算,     name_en: More Personal Computing,             xbrl_member: MorePersonalComputingMember}
```

### 3.1.1 ⚠ viper 小写化契约（TASK-003 实测，TASK-007/008/009 必须遵守）

dev-agent-17 在 TASK-003 中实测确认 viper 的一个关键行为，**下游一旦不知情就会产生静默 JOIN 失败**：

- **viper 只小写化 map 的 key，不动 value。** 实测 `AllSettings`：`FY2025` → `fy2025`、
  嵌套 `Cloud` → `cloud`；而 `company: MSFT` 的**值** `MSFT` 原样保留。
- 推论 1：`Template` / `Segment` 的全部字段都是 YAML **值**，无需归一，大小写原样。
- 推论 2：**`LoadManualSegments` 返回的两层 map 的 key（`fiscal_period` 与 `segment_key`）
  已被 viper 小写化**。

**由此产生的硬契约**：
1. **模板 `segments[].key` 必须小写** —— 否则手工分部数据（key 被小写化）与模板 key 永远对不上，
   且不会报错，只会静默丢数据。`Segment.Key` 的注释已写明该约定。
2. `LoadManualSegments` 内部对 `fiscal_period` 先 `strings.ToUpper` 归一再走正则
   `^([0-9]{4}Q[1-4]|FY[0-9]{4})$` 校验（不归一则所有合法手工数据都会被误判非法），
   错误信息回显**原始**键 + 文件路径。
3. TASK-008 用 manual 数据覆盖 auto 行时，`segment_key` 的比较**必须按小写口径**。

**其他已落地的加载层约定**（TASK-003 discovery，下游可直接依赖）：
- 返回 map 的 key 为 `ToUpper(tmpl.Company)`，**取自 company 字段而非文件名**
  （文件名与 company 可能不一致；以 company 为准才能给 TASK-011 的 CIK 串号校验留唯一依据）。
- `SegmentAxis` 为空时由加载层回填导出常量 `DefaultSegmentAxis = "StatementBusinessSegmentsAxis"`，
  **下游不必各自判空**。
- `Segments` **保序**，顺序即 YAML 顺序，桑基左侧渲染顺序可直接沿用。
- 校验比原 DoD 多一条：**`xbrl_member` 包内唯一**（两个分部抢同一份 XBRL 数据是硬故障）。
- 「合法 + 非法模板混放」时返回 `nil` map，**不返回部分结果**（避免部分成功掩盖故障）。

**加载期必须显式失败的两条**（Leader 裁决，归属 TASK-003，2026-07-25 修订）：
1. **`segments[].key` 含大写字符 → error**。仅靠注释约定不够：模板写 `key: Cloud` 不报错，
   但手工数据经 viper 必为小写 `cloud`，结果是**运行期静默空 JOIN**（test-agent-6 探针实测：
   以模板 key 查得 0 条）。
2. **两个模板文件声明同一 `company` → error**。后加载者静默覆盖 map 条目，
   赢家取决于 `os.ReadDir` 顺序（test-agent-6 探针实测：`len(map)=1`，赢家为 `SecondMember`）。

> **修订说明**：本节初稿曾把第 2 条写为「本期不修，归属 TASK-011」，与 Leader 追加给 TASK-003 的
> 验收要求自相矛盾（test-agent-6 在验证报告 §4 指出）。裁决为**两条都归 TASK-003**——
> 二者同属「加载期显式失败 vs 运行期静默空数据」，与 AD-16 同源，合计约 6 行校验 + 2 个负向用例，
> 拆到 5 个 wave 之后的 TASK-011（API 层任务）反而构成 scope 漂移。

### 3.2 多期引擎（TASK-007 产出，TASK-009 消费）

```go
type PeriodMetrics struct {
	Period   string             // "2026Q1" 或 "FY2026"
	PeriodEnd string            // AD-26 补充（TASK-016 返工轮）：该期的 period_end；
	                            // 财年取**最后一季**的 period_end（取首季会把年份标早 9 个月）。
	                            // 用途：① 同比反查的 340~390 天校验载体；② 下游可直接显示期末日期，
	                            // 不必从标签反推——而标签本身正是不可靠的那个东西。
	Segments map[string]float64 // segment_key → revenue
	Revenue, GrossProfit, OperatingIncome, NetIncome, RnD, SGnA, IncomeTax float64
	GrossMargin, OpMargin, NetMargin float64 // NaN = 分母缺失或为 0
}

// BuildPeriods: granularity "q" 直转；"fy" 按 fiscal_period 前 4 位财年分组，
// 流量科目 4 季求和（不足 4 季的财年不产出），Segments 按 key 求和。
func BuildPeriods(f []prismstore.FundamentalRow, segs []prismstore.SegmentRow, granularity string) ([]PeriodMetrics, []PeriodConflict)

// AD-26（TASK-016，2026-07-25）：签名增加 []PeriodConflict 返回值。
// 背景：真实 EDGAR 数据中 **27% 的 FiscalPeriod 标签冲突**（实测 MSFT 7/26，最严重的
// 2018Q4 有 4 个不同季度共用）——根因是 client.go 的 applyDuration 去重时胜出条目携带的是
// 较晚报告的 fy/fp 上下文标签。后果实测：FY2025 productivity 报 392.914B（真值 120.810B，3.25 倍）。
//
// 两层防御，各治一半、不可互相替代：
//  1. buildQuarters 的分部桶改为 (label, period_end) 二级键，主表行只取与自身 period_end
//     相符（±15 天，见 periodEndSlackDays）的桶 → 消除跨季相加；
//  2. aggregateFiscalYears 增 len(qs) > 4 上界拒绝（与既有 < 4 对称）→ 6 个季度不再被聚合成
//     "完整财年"。第 1 层拦不住第 2 层：主表行本身就是 6 行 period_end 各不相同的合法数据。
//
// **为何不保留原签名 + 另加带 report 的变体**（dev-agent-17 的判断，Leader 认可）：
// 那等于留下一个丢弃冲突的便捷入口 —— AD-16 的教训正是「可忽略的错误一定会被忽略」
// （plan 原稿那句 `templates, _ :=`）。全仓生产调用点只有 service.go 两处，代价可控。
//
// **±15 天容差是必需的，不是防御性编程**：TASK-008 反查 fiscal_period 本身带 ±3 天容差，
// 分部行与主表行的 period_end 可以合法地差几天。若要求精确相等，真实数据里每个季度的分部
// 都会对不上、全部落进 other_segment 残差 —— **那是把一个静默错误换成另一个**。
//
// 对不上任何桶时 Segments 为 nil（而非退而取最近的桶）：「本期没有分部数据」会被残差吸收、可见；
// 「错记了别的季度的值」不可见。
//
// 透传链：BuildPeriods → Analysis.Conflicts（service.go）→ TASK-011 API → TASK-012 降级提示。
// 与 AD-22 的 Graph.Notes 同理，都是「本该有但被拒绝的东西」的唯一数据源。

// DefaultSelection 智能对比上下文（设计 §5.6）：
//   最新期为季报 → 该期所属财年内全部已发布季，granularity "q"；
//   最新财年 4 季齐（年报已出）→ 全部 FY（最多 10 个，取最近），granularity "fy"。
// 边界（必须定义并有断言）：quarters 与 fys 皆空 → 返回 (nil, "q")；
//   最新期为财年首季（该财年只有 1 期）→ 返回该 1 期；
//   最新财年只有 3 季但年报已发（Q4 推导失败）→ 走季度分支返回已有季，不伪造 FY。
func DefaultSelection(quarters, fys []PeriodMetrics) ([]PeriodMetrics, string)

type MatrixRow struct {
	Key, Label string
	Values     []float64
	Change     float64
	ChangeKind string // "yoy" | "cagr" | "none"
}

// BuildMatrix 行 = 各 segment + 主干科目 + 三比率，列 = 各期；
// 末列 Change：季度 vs 去年同期（从 allQuarters 反查）；年度 vs 上年；年度且 ≥3 期 → 区间 CAGR。
// 缺对照期 → Change NaN、ChangeKind "none"。
//
// AD-14（硬故障防护）：对照期存在但值为 0 或 NaN 时，Change **必须返回 NaN 而非 ±Inf**——
// encoding/json 对 Inf 直接报错，会让整个 API 返回 500，而 jf() 只映射 NaN。
// CAGR 同理：起点 ≤0 或跨期数 <2 → NaN。
func BuildMatrix(sel []PeriodMetrics, tmpl *Template, lang string, allQuarters []PeriodMetrics) []MatrixRow

type Node struct{ Name string; Value float64 }   // Value = max(入流, 出流)：源节点取出流、汇节点取入流
type Link struct{ Source, Target string; Value float64 }
type Graph struct {
	Nodes []Node
	Links []Link
	Notes []string   // AD-22：负残差/0 宽节点的显式记账，前端 footnote 数据源
}

// AD-22（Leader 批准 dev-agent-17 的接口偏离，2026-07-25）：
// 本节初稿要求「负残差被显式记录、footnote 记账」，但 Graph 没有任何字段能承载该记录 ——
// 是设计疏漏。加 Notes []string（纯增字段，Node/Link 契约不变，TASK-009/012 尚未开工）。
// 不加的话负 tax_other 只会被画成 0 宽，图上凭空少一块且无人知情 ——
// 恰是 AD-13 想消灭的那类静默失真。
// 被拒方案：把说明塞进节点名 —— 会污染前端渲染文本。

// MaxPeriods = 10 已在包内导出（AD-15）。TASK-009 的 Truncated 判定必须引用该常量，
// 不得再写字面量 —— AD-15 的成因正是三处各写各的上限值。

// BuildSankey 单期桑基（ECharts 格式）。结构：
//   segments + other_segment → revenue          // AD-13 残差
//   revenue → cogs + gross_profit
//   gross_profit → opex + operating_income
//   opex → rnd + sganda + other_opex            // AD-13 残差
//   operating_income → tax_other + net_income   // tax_other = 营利 − 净利（含税与其他，footnote 标注）
//
// AD-13 残差节点（真实数据中 Σsegments ≠ Revenue 是常态：corporate/other/eliminations；
// RnD+SGnA ≠ GrossProfit−OperatingIncome 也是常态：其他营业费用/摊销/减值。
// 没有残差节点则守恒断言只能在人造数据上成立）：
//   other_segment = Revenue − Σsegments
//   other_opex    = (GrossProfit − OperatingIncome) − RnD − SGnA
// 两者 |差| < Revenue × 0.5% 时省略该节点。
//
// 负值处理：残差或 tax_other 为负（如利息收入大的公司 NetIncome > OperatingIncome）时
// **不画负流**，改为图上标注 + footnote 记账；守恒断言只对非负流成立且负残差被显式记录。
// NaN 节点 Value 保持 NaN 由前端渲染 "—"。
func BuildSankey(p PeriodMetrics, tmpl *Template, lang string) Graph
```

### 3.3 装配层（TASK-009 产出，TASK-011 消费）

```go
type ServiceStore interface {
	QuarterlyFundamentals(instrumentID int64) ([]prismstore.FundamentalRow, error)
	SegmentRows(instrumentID int64) ([]prismstore.SegmentRow, error)
	PriceSeries(symbol, from string) ([]string, []float64, error)
	InstrumentID(symbol string) (int64, error)   // AD-24，见下
}
func NewService(store ServiceStore, templates map[string]*Template) *Service

// AD-24（Leader 批准 dev-agent-17 的提案，2026-07-25）：本节初稿写
// 「由 symbol 解析 instrumentID 的既有途径（实现者按 store 现状选取）」——
// **该「既有途径」根本不存在**，是我未核实 Store 现状就留下的洞。实测 Store 的 13 个方法中：
//   - QuarterlyFundamentals / SegmentRows / LatestSegmentPeriodEnd / LatestDate → 都要 id（正是拿不到的东西）
//   - PriceSeries / Series → 收 symbol，但内部自解析、**不返回 id**
//   - Board() → BoardRow 内嵌的 Instrument 也没有 id，且 INNER JOIN valuation_daily（无估值的标的不出现）
//   - UpsertInstrument → 唯一返回 id 的，但它是**写**操作，且
//     `ON CONFLICT DO UPDATE SET market=excluded.market, name=…, grp=…, source=…`
//     以 Instrument{Symbol: s} 调用会把该标的的 market/name/grp/source **全部刷成空串**。绝不可用于读路径。
//
// 决议：给 prismstore 加纯增只读方法（TASK-009 承接，其 packages 已扩含 ./internal/storage/prism）：
//
//	// InstrumentID resolves a symbol to its row id; unknown symbols yield ErrNotFound.
//	func (s *Store) InstrumentID(symbol string) (int64, error)
//
// 实现同 PriceSeries 的解析逻辑（SELECT id FROM instrument WHERE symbol=?，sql.ErrNoRows → ErrNotFound）。
// 理由：symbol→id 本就是 store 层职责；纯增只读、对既有调用者零影响；
// 顺带为 Analyze/Fundamental 的「未知 symbol」提供统一错误来源（DoD 要求返回 prismstore.ErrNotFound）。
// 被拒方案：(B) 窄接口改 symbol 口径 + TASK-011 写适配器 —— 适配器仍需同一个只读方法，
// 等于把洞推给下游且绕过 store 层；(C) NewService 多收 resolve 函数 —— 解析逻辑仍要有人写，
// 只是换个地方写同样的 SQL，且偏离 §3.3 签名。

type PeriodView struct {
	Period  string
	Graph   Graph
	Metrics PeriodMetrics
}
type Analysis struct {
	Symbol, Granularity string
	Periods   []PeriodView
	Matrix    []MatrixRow
	Truncated bool             // 期数 > MaxPeriods(10) 被截断 —— 生产代码必须引用常量,不得写字面量
	Conflicts []PeriodConflict // AD-26：BuildPeriods 拒绝聚合的期与原因（标签冲突、>4 季等）。
	                           // 与 Graph.Notes 同为「本该有但被拒绝的东西」的唯一数据源，
	                           // 断链则前端无法给出降级提示，且不会有任何测试变红。
	Template  struct {
		Version int
		Lang    map[string]string // segment_key → 当前 lang 的显示名
	}
}
// from/to 为 "" → 走 DefaultSelection；lang "zh"|"en"；
// 期数上限 **10**（AD-15，与 DefaultSelection 年度分支一致；超出取最近 10 并置 Truncated=true）。
// 未配置模板的 symbol → 返回 ErrNoTemplate（哨兵错误，可被 errors.Is 判定）。
func (s *Service) Analyze(symbol, from, to, granularity, lang string) (*Analysis, error)

type FundamentalView struct {
	Symbol   string
	Periods  []string            // fiscal_period 升序
	Metrics  map[string][]float64 // revenue/net_income/gross_profit/... + roe_ttm
	Dates    []string             // price_daily
	Closes   []float64
}
// 无 fundamental_q 数据 → prismstore.ErrNotFound
func (s *Service) Fundamental(symbol string) (*FundamentalView, error)
```

ROE(TTM) 推导：`TTM(NetIncome) / Equity`（Equity 取该季末值），分母 NaN 或 0 → NaN。

---

## 4. `internal/prism`

### 4.1 Store 接口扩展（TASK-006 产出）

```go
type Store interface {
	// 既有 5 个方法不变
	UpsertInstrument(inst prismstore.Instrument) (int64, error)
	LatestDate(instrumentID int64) (string, error)
	UpsertValuations(instrumentID int64, rows []prismstore.ValuationRow) error
	UpsertFundamentals(instrumentID int64, rows []prismstore.FundamentalRow) error
	Series(symbol, from string) (*prismstore.SeriesData, error)
	// M3 新增
	UpsertPrices(instrumentID int64, closes []prismstore.PriceRow) error
	UpsertSegments(instrumentID int64, rows []prismstore.SegmentRow) error
	SegmentRows(instrumentID int64) ([]prismstore.SegmentRow, error)
	LatestSegmentPeriodEnd(instrumentID int64) (string, error)
	QuarterlyFundamentals(instrumentID int64) ([]prismstore.FundamentalRow, error)
}
```

`refreshEdgar`（refresh.go:406）与 `refreshEngine`（refresh.go:190）取得 closes 后，
各自 `store.UpsertPrices(id, rows)`；**失败不阻断估值主流程**，记入 Degraded。

### 4.2 分部刷新编排（TASK-008 产出）

```go
type SegmentClient interface {
	FetchSegmentRevenue(cik, axis string, since time.Time) ([]edgar.SegmentPeriod, error)
}

// RefreshSegments 对配置了模板的标的增量拉取分部并落库：
//  1) since = LatestSegmentPeriodEnd；**force=true 时忽略锚点全量重拉**（AD-12：
//     模板迭代「跑一次看 member → 改模板 → 再跑」在纯增量下第二次拉不到任何数据，流程走不通）
//  2) member → segment_key 映射（模板）；未映射 member 记入 Report.Degraded 文本（不失败）
//  3) 落库主键为 period_end（AD-9）。fiscal_period 展示标签由 PeriodEnd 在 fundamental_q 的
//     period_end 中匹配（±3 天容差）反查；**匹配不到不再跳过**——落 period_end + 空 fiscal_period
//     并记 Degraded（数据不丢且可观测）。命中 ≥2 条时报错而非取首个。
//  4) Q4 推导：同财年的 FY 期 Values − 已存 3 季 → Q4（逐 segment；凑不齐则跳过该 segment；
//     推导得负值 → 不落库并记 Degraded）。**FY 期本身不落库**（AD-17：年度值一律由引擎从
//     4 季聚合，避免与 fy 聚合重复计算）。
//  5) manual 数据（LoadManualSegments）最后 upsert，source="manual"，覆盖同键自动行。
//     注意：下一次 auto 刷新会再次覆盖 manual 行 —— 该语义须在 DoD 中定死并有测试。
// 任一标的失败只记入 Report.Failed，不影响其余标的。
// Report.Refreshed 语义：分部刷新与估值刷新的计数**分开表述**，合并输出时不得让 "N ok" 超过标的数。
func RefreshSegments(cfg config.PrismConfig, store Store, seg SegmentClient,
	templates map[string]*sankey.Template, manualDir string, force bool) Report

// AD-25（Leader 认可 dev-agent-16 的实现偏离，2026-07-25）：本节初稿第 5 参写的是
// `now time.Time`，实测**用不到**——RefreshSegments 的 `since` 来自 LatestSegmentPeriodEnd 锚点
// （或 force 时的零值），全函数无任何 time.Now() 依赖，与需要算 lookback 范围的 Refresh 不同。
// 反之第 5 步「manual 数据最后 upsert 覆盖同键自动行」需要 LoadManualSegments(dir, symbol) 的目录，
// 而初稿**描述了该步骤却没在签名里给出获取目录的途径** —— 与 AD-24（symbol→id 的「既有途径」
// 根本不存在）同一类洞：**描述了要做什么，但没给做这件事所需的参数**。
// 故第 5 参改为 `manualDir string`。TASK-010 的 cmd 接线按此签名调用。
```

---

## 5. API 与页面

### 5.1 路由（TASK-011 注册 API，TASK-012/013 注册页面）

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/prism/sankey?symbol=&from=&to=&granularity=&lang=` | `Analysis` JSON；NaN→null（沿用 `jf`）；无模板 404 `{"error":"no sankey template"}` |
| GET | `/api/prism/fundamental?symbol=` | `FundamentalView` JSON；无数据 404 |
| GET | `/prism/sankey/{symbol}` | 页面（server.go 侧 TrimPrefix 剥参，传第三形参） |
| GET | `/prism/fundamental/{symbol}` | 页面，同上 |

`api.Dependencies` 追加 `PrismSankey *sankey.Service`（nil → 不注册对应路由）；
`cmd/atlas/serve.go` 在 `prismCfg.Enabled` 分支内 `LoadTemplates("configs/prism/templates")` +
`sankey.NewService(prismStore, templates)` 注入。
**模板加载 error 必须记录到日志（AD-16），不得丢弃**；加载失败时 serve 仍启动但不注册 sankey 路由
（降级可观测）。

响应包裹沿用 `response.JSON` → `{"data": …, "meta": {...}}`；**前端 JS 必须用 `resp.data`**（AD-8）。

**AD-14 `jf` 扩展**：既有 `jf(v float64) any` 只把 NaN 映射为 null。本期扩展为同时把
`math.IsInf(v, 0)` 映射为 null —— 作为引擎侧「除零返回 NaN」之外的第二道防线，
防止后续新增计算路径漏出 ±Inf 导致 `json.Marshal` 报错、整个 API 500。

**参数校验**：`granularity` 非法值（非 `q`/`fy`/空）返回 400，不得静默当季度处理；
`symbol` 缺失返回 400；未知 symbol / 无模板 / prism 未启用三种 404 分支的错误文案互不混淆。

### 5.2 页面交互（TASK-012 / TASK-013）

**`/prism/sankey`**（`id="sankey-grid"`）：
- 顶部：报告期范围（两个 select，选项由 API 返回期列表填充）+ 粒度（季/年）+ 语言（中/英）
  + 视图切换（网格 ↔ 单期大图 ↔ 堆叠柱）。
- 网格：每期一个 `.sankey-cell`，ECharts sankey 实例；统一比例尺 —— JS 求各期 max revenue，
  容器高度 × `clamp(rev_i/rev_max, 0.5, 1)` 近似（ECharts 无跨实例比例尺，footnote 说明）。
- 单期大图：节点标注 YoY/QoQ（取自 matrix）。堆叠柱：segments × periods（同一份数据，无额外请求）。
- 矩阵 HTML table：Change 列上色（正绿负红），模板 version 变化处加「口径变更」角标。
- PNG 导出：ECharts toolbox（单期大图与堆叠柱开启）。

**`/prism/fundamental`**（`id="fund-chart"`）：
- 指标切换：营收/净利/毛利率/营业利润率/ROE(TTM)/营收 YoY/净利 YoY（增速 JS 现算）。
- 折线↔柱状切换 + 股价叠加（双轴，price_daily）+ 季度↔年度（JS 端 4 季聚合）+ dataZoom 双端滑块。
- 仅对有 fundamental_q 数据的标的开放；board 卡片加入口链接。

JS 风格沿用 `prism_compare.html`：IIFE、ES5（`var` + `function`）、Tailwind 原子类、元素靠 `id` 定位。
