# 宏观危机监控（Cassandra）实施方案

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**设计文档**：`docs/plans/atlas-macro-crisis-monitor-design.md`（v0.2，本方案的唯一需求来源）
**Goal**：为 Atlas 新增系统性风险监控模块——采集 7 个市场压力指标入 sqlite，按配置驱动的三色规则 + 抑制降噪 + 四态状态机每日评估，经 telegram 推送风险状态，launchd 定时唤起、进程无状态。

**Architecture**：新增 `internal/collector/fred`（FRED API 客户端）与 `internal/crisis`（store / derive / rules / suppress / statemachine / eval / ingest / notify 八个文件的独立包）；CLI 子命令平铺在 `cmd/atlas/crisis.go`；所有阈值进 `configs/crisis-monitor.yaml`；sqlite（新库 `data/crisis.db`）为唯一真相源。规则引擎与状态机是**纯函数**（通过 `SeriesReader` / `EvalHistory` 两个窄接口取数），live 评估与历史回测（replay）共用同一套引擎。

**Tech Stack**：Go 1.24.4、cobra、viper、testify、modernc.org/sqlite v1.38.2（复用 `internal/storage/signal` 的 sqlite 惯例）、复用 `internal/collector/yahoo` 与 `internal/notifier/telegram`。

## Global Constraints

- **sqlite 固定 `modernc.org/sqlite v1.38.2`，不得升级**；所有 `go build` / `go test` 前缀 `GOTOOLCHAIN=local`（仓库既有约束）。
- 时间列一律 TEXT：观测日 `YYYY-MM-DD`；时间戳固定宽度 UTC RFC3339 `"2006-01-02T15:04:05.000000000Z07:00"`（与 `internal/storage/signal/sqlite.go:23` 的 `timeLayout` 一致，字典序 = 时间序）。
- **全部阈值进 `configs/crisis-monitor.yaml`，代码中不出现阈值数字**（设计 §4.1）。
- 通知文案**禁止出现"必然/一定/即将"**，统一概率表述，页脚带边界声明（设计 §3.3/§5）；不做优先级路由，文案前缀 `[P0]/[P1]/[P2]`（设计 §4.4）。
- 交易日一律用"周内日"近似（不含假日日历）；设计 §4.3 已声明不追求唤起时刻精确，靠幂等兜底。
- 每个任务提交前：运行 gitnexus `detect_changes()` 核对影响面；按用户全局规范运行 code-simplifier sub-agent（Task tool, subagent_type: `code-simplifier:code-simplifier`）。修改**已有** symbol（仅 Task 3 涉及 `yahoo.validateSymbol`）前先跑 gitnexus `impact`。
- 分支：`feature/crisis-monitor`（执行时经 superpowers:using-git-worktrees 建隔离工作区）。提交格式 `feat(crisis): ...` / `feat(fred): ...` / `fix(yahoo): ...`。
- 部署前置的人工依赖（设计 §5）：FRED API key（**已配置**在 `configs/config.yaml` 的 `collectors.fred.api_key`，该文件在 .gitignore 中与 telegram/lixinger 凭据同层；env `FRED_API_KEY` 可覆盖，密钥不得写入任何入库文件）；HY OAS 2006 年起历史 CSV 快照（第二阶段回测验收的硬前提）。

## 存储单位约定（canonical units）

`macro_observations.value` 按**阈值原生单位**入库，规则引擎不再换算：

| indicator | 存储单位 | 来源与采集期换算 |
|---|---|---|
| `vix` | 指数点 | FRED `VIXCLS` 原值 |
| `move` | 指数点 | Yahoo `^MOVE` 日收盘 |
| `sofr_effr` | bp | FRED `SOFR`、`EFFR` 按日 join 后 `(SOFR−EFFR)×100`，source=`derived`（设计 §2.2 的派生在 crisis 包完成，不落 collector） |
| `hy_oas` | bp | FRED `BAMLH0A0HYM2` 百分数 ×100 |
| `t10y2y` | bp | FRED `T10Y2Y` 百分数 ×100 |
| `nfci` | 指数 | FRED `NFCI` 原值（周频） |
| `usdjpy` | 汇率 | Yahoo `JPY=X` 日收盘 |

派生量 `usdjpy_wow_pct`（周环比）、`hy_oas` 月度走阔、VIX 单周涨幅在**评估时**由窗口现算，不落库。

## 对设计 v0.2 的三处已核实偏差（执行者不需重新决策）

1. **`crisis_evaluations` 增加 `ts` 列**（数据观测日）。设计 §4.2 的 schema 只有 `eval_at`（评估时刻），但幂等（"当日已评估则跳过"）和回测（按数据日检索 2008 年的评估）都需要按**数据日**查询；回放 2008 年数据时 `eval_at` 是 2026 年的时刻，无法充当数据日。`indicator` 列用 `''` 而非 NULL 表示系统级行（省掉 `sql.NullString` 扫描）。
2. **配置结构用 typed struct 而非设计 §4.1 示意的泛化 threshold DSL**。7 个指标的规则形态各异（持续性、双向黄灯、动量、复陡标记），泛化 DSL 是过度设计；YAML 保留全部数字可调这一设计意图不变。
3. **分位轨最小窗口 60 个观测**：窗口不足 5 年用可得最长窗口（设计 §2.2），但观测数 < 60 时分位无统计意义，跳过分位轨（`window_actual_obs` 仍如实标注）。

## 文件结构总览

```
internal/collector/fred/
├── fred.go                  # FRED API 客户端（key、重试退避、"." 缺失值过滤）
└── fred_test.go
internal/collector/yahoo/
└── yahoo.go                 # 仅改 validSymbol 正则：支持货币符号 JPY=X（Task 3）
internal/crisis/
├── types.go                 # Status/Tag/SystemState/Observation/Evaluation/IndicatorResult
│                            #   + SeriesReader/EvalHistory 接口 + severity/maxStatus/isColor
├── dates.go                 # dateLayout/timeLayout、PrevTradingDay、addYears/addDays、NowStamp
├── store.go                 # 两张表的 sqlite 存取 + Reader/History 适配器
├── config.go                # crisis-monitor.yaml 加载与校验（viper）
├── derive.go                # WowPct/MomChange/SpreadBp/Percentile（复用 valuation.PercentileRank）
├── suppress.go              # InQuarterEndWindow / staleFor / ApplyHysteresis
├── rules.go                 # EvaluateIndicator + 7 个指标的三色规则（绝对阈值轨 + 分位轨）
├── statemachine.go          # NextState + SysDetail + 连续日计数（读 EvalHistory）
├── memhistory.go            # MemHistory：EvalHistory 的内存实现（回测与测试用）
├── eval.go                  # EvalDay 单日编排：规则 → 抑制/防抖 → 状态机 → Evaluation 行
├── ingest.go                # Ingestor：FRED/Yahoo 采集 → 单位换算 → UpsertObservations
├── notify.go                # Sender 接口 + Messages 文案生成（[P0]/[P1]/[P2]、页脚、月报/周报）
└── *_test.go                # 每个文件一份对应测试
cmd/atlas/
├── crisis.go                # crisis 父命令 + backfill/eval/status/replay 子命令（沿用平铺惯例）
└── crisis_test.go
configs/
└── crisis-monitor.yaml      # 全部阈值与调度参数
deploy/launchd/
├── com.newthinker.atlas.crisis-daily.plist
├── com.newthinker.atlas.crisis-nfci.plist
└── com.newthinker.atlas.crisis-intraday-jpy.plist
```

## 核心接口契约（跨任务的公共签名，后续任务以此为准）

```go
// ---- types.go (Task 1) ----
type Status string        // GREEN/AMBER/RED/STALE/SUPPRESSED_SEASONAL/NO_DATA
type Tag string           // ""/COMPLACENCY/STRESS/CROWDED/STEEPENING
type SystemState string   // NORMAL/WATCH/BREWING/CRISIS
const IndVIX, IndMOVE, IndSOFREFFR, IndHYOAS, IndT10Y2Y, IndNFCI, IndUSDJPY = "vix", "move", "sofr_effr", "hy_oas", "t10y2y", "nfci", "usdjpy"
var AllIndicators []string

type Observation struct{ Date, Indicator string; Value float64; Source, FetchedAt string }
type Evaluation struct {
    TS, EvalAt, Indicator string // Indicator=="" 为系统级行
    Status Status; Tag Tag; Value, Pct5y float64
    SystemState SystemState; Detail string // JSON
}
type IndicatorResult struct {
    Indicator string; Status, RawStatus Status; Tag Tag
    Value, Pct5y float64; WindowActualObs int
}
type SeriesReader interface {
    Window(indicator, end string, n int) ([]Observation, error)      // ts<=end 最近 n 条，升序
    WindowSince(indicator, from, end string) ([]Observation, error)  // from<=ts<=end，升序
}
type EvalHistory interface {
    RecentSystem(n int) ([]Evaluation, error)                  // 新→旧
    RecentIndicator(indicator string, n int) ([]Evaluation, error)
}
func severity(s Status) int          // GREEN=0 AMBER=1 RED=2（非色彩 -1）
func maxStatus(a, b Status) Status
func isColor(s Status) bool

// ---- dates.go (Task 1) ----
func PrevTradingDay(t time.Time) time.Time  // 周内日近似
func NowStamp(t time.Time) string           // 固定宽度 UTC RFC3339
func addYears(date string, y int) string
func addDays(date string, d int) string
func daysBetween(from, to string) int

// ---- store.go (Task 1) ----
func NewStore(path string) (*Store, error)  // WAL + busy_timeout + schema，模式同 signal.NewSQLiteStore
func (s *Store) Close() error
func (s *Store) UpsertObservations(ctx context.Context, obs []Observation) error   // INSERT OR REPLACE，事务
func (s *Store) Observation(ctx context.Context, indicator, date string) (*Observation, error) // 无则 nil,nil
func (s *Store) LatestObservation(ctx context.Context, indicator string) (*Observation, error)
func (s *Store) SeriesWindow(ctx context.Context, indicator, end string, n int) ([]Observation, error)
func (s *Store) SeriesSince(ctx context.Context, indicator, from, end string) ([]Observation, error)
func (s *Store) AppendEvaluations(ctx context.Context, evals []Evaluation) error
func (s *Store) RecentSystemEvals(ctx context.Context, n int) ([]Evaluation, error)
func (s *Store) RecentIndicatorEvals(ctx context.Context, indicator string, n int) ([]Evaluation, error)
func (s *Store) LatestSystemEval(ctx context.Context) (*Evaluation, error)          // 无则 nil,nil
func (s *Store) HasSystemEvalForDate(ctx context.Context, date string) (bool, error)
func (s *Store) EvalDates(ctx context.Context, from, to string) ([]string, error)   // vix 观测日 = 回测评估日
func (s *Store) Reader(ctx context.Context) SeriesReader
func (s *Store) History(ctx context.Context) EvalHistory

// ---- internal/collector/fred (Task 2) ----
package fred
type Observation struct{ Date string; Value float64 }
func New(apiKey string) *Client
func NewWithBaseURL(apiKey, baseURL string) *Client   // 测试注入 httptest
func (c *Client) FetchSeries(ctx context.Context, seriesID, start, end string) ([]Observation, error)

// ---- config.go (Task 4) ----
func LoadConfig(path string) (*Config, error)
// Config{Storage{Path}, FRED{APIKeyEnv}, Freshness{DailyMaxLagDays, WeeklyMaxLagDays},
//   Percentile{WindowYears, Amber, Red}, Indicators{VIX, MOVE, SOFREFFR, HYOAS, T10Y2Y, NFCI, USDJPY},
//   StateMachine{WatchAmberCount, CrisisExitDays, WatchExitDays, BrewingExitDays, DemoteHysteresisDays}}

// ---- derive.go (Task 5) ----
func SpreadBp(sofr, effr float64) float64
func WowPct(window []Observation) (float64, bool)          // 需 ≥6 观测：close_t/close_{t-5} − 1
func MomChange(window []Observation, n int) (float64, bool) // window[last] − window[last−n]
func Percentile(window []Observation, current float64) (float64, int) // 0–1 分位, 实际观测数；空窗 (-1, 0)

// ---- suppress.go (Task 8) ----
func InQuarterEndWindow(date string) bool
func staleFor(cfg *Config, indicator, evalDate, latestObsDate string) bool
func ApplyHysteresis(raw Status, prev []Evaluation, days int) Status  // prev 新→旧；升级立即、降级需连续

// ---- rules.go (Task 9) ----
func EvaluateIndicator(cfg *Config, indicator, date string, sr SeriesReader) (IndicatorResult, error)
// 输出 RawStatus/Tag/Pct5y；NO_DATA/STALE 直接定 Status；抑制与防抖由 EvalDay 应用

// ---- statemachine.go + memhistory.go (Task 10) ----
type SysDetail struct{ Date string; AnyTrigger, BrewingPair bool; AmberCount int; Prev SystemState } // JSON 进系统行 detail
func NextState(cfg *Config, prev SystemState, res map[string]IndicatorResult, hist EvalHistory) (SystemState, SysDetail, error)
func NewMemHistory() *MemHistory       // 实现 EvalHistory
func (m *MemHistory) Append(evals []Evaluation)

// ---- eval.go (Task 11) ----
type DayResult struct {
    Date string; Results map[string]IndicatorResult
    PrevState, State SystemState; Detail SysDetail
    Evaluations []Evaluation               // 7 指标行 + 1 系统行，可直接落库
}
func (r *DayResult) Transitioned() bool
func EvalDay(cfg *Config, date string, sr SeriesReader, hist EvalHistory, evalAt time.Time) (*DayResult, error)

// ---- ingest.go (Task 6) ----
type FREDFetcher interface{ FetchSeries(ctx context.Context, seriesID, start, end string) ([]fred.Observation, error) }
type HistoryFetcher interface{ FetchHistory(symbol string, start, end time.Time, interval string) ([]core.OHLCV, error) }
type IngestReport struct{ Counts map[string]int; YahooErrs map[string]error } // yahoo 失败降级不阻断（设计 §2.1 MOVE 缺数降级）
func NewIngestor(f FREDFetcher, y HistoryFetcher, s *Store) *Ingestor
func (ig *Ingestor) IngestAll(ctx context.Context, from, to string) (*IngestReport, error)   // FRED 失败才返回 error
func (ig *Ingestor) IngestNFCI(ctx context.Context, from, to string) (int, error)

// ---- notify.go (Task 14) ----
type Sender interface{ SendText(text string) error }   // telegram.Telegram 已满足
func Messages(res *DayResult, stateDays int, summaryDue bool, staleInds []string) []string
```

## 状态机语义补充（设计 §3.3 的两处解读，写死进实现）

- **"全系统 AMBER ≥ 3"按"AMBER 及以上"计数**（RED 计入）。否则"2 个 RED + 2 个 AMBER"反而不触发 WATCH，违背共振语义。
- 连续日计数（CRISIS 退出 10 日、WATCH 退出 20 日、BREWING 退出 10 日）从 `crisis_evaluations` 历史行重建：系统行 detail 存 `any_trigger`/`brewing_pair` 布尔，指标行存 status——**进程内不持有任何计数器**（设计 §4.3 冷启动：backfill 后初始 NORMAL，防抖计数从首次 eval 起累积 = 历史行数不足时不允许降级/退出，天然满足）。
- STALE / SUPPRESSED_SEASONAL / NO_DATA 一律退出共振计数（设计 §3.2 条 5：共用同一退出机制）。

---

# 任务分解

## 第一阶段：数据流跑通（Task 1–7）

### Task 1: crisis 包基础类型、日期工具与 sqlite Store

**Files:**
- Create: `internal/crisis/types.go`、`internal/crisis/dates.go`、`internal/crisis/store.go`
- Test: `internal/crisis/store_test.go`、`internal/crisis/dates_test.go`

**Interfaces:**
- Consumes: `modernc.org/sqlite`（惯例照抄 `internal/storage/signal/sqlite.go`）
- Produces: 上方"核心接口契约"中 types.go / dates.go / store.go 全部签名（后续所有任务的地基）

Schema（含偏差 1 的 `ts` 列）：

```sql
CREATE TABLE IF NOT EXISTS macro_observations (
    ts          TEXT NOT NULL,     -- 观测日 YYYY-MM-DD
    indicator   TEXT NOT NULL,
    value       REAL,
    source      TEXT,              -- fred / yahoo / manual_backfill / derived
    fetched_at  TEXT,
    PRIMARY KEY (ts, indicator)
);
CREATE TABLE IF NOT EXISTS crisis_evaluations (
    ts            TEXT NOT NULL,             -- 数据观测日（偏差 1）
    eval_at       TEXT NOT NULL,
    indicator     TEXT NOT NULL DEFAULT '',  -- '' = 系统级行（偏差 1）
    status        TEXT,
    tag           TEXT,
    value         REAL,
    pct_5y        REAL,
    system_state  TEXT,
    detail        TEXT
);
CREATE INDEX IF NOT EXISTS idx_macro_obs_ind_ts   ON macro_observations(indicator, ts);
CREATE INDEX IF NOT EXISTS idx_crisis_eval_ind_ts ON crisis_evaluations(indicator, ts);
```

- [ ] **Step 1: 写日期工具的失败测试** — 创建 `internal/crisis/dates_test.go`

```go
package crisis

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestPrevTradingDay(t *testing.T) {
	mon := time.Date(2026, 7, 13, 10, 0, 0, 0, time.UTC) // 周一
	assert.Equal(t, "2026-07-10", PrevTradingDay(mon).Format("2006-01-02"))
	sun := time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC) // 周日
	assert.Equal(t, "2026-07-10", PrevTradingDay(sun).Format("2006-01-02"))
	wed := time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC) // 周三
	assert.Equal(t, "2026-07-07", PrevTradingDay(wed).Format("2006-01-02"))
}

func TestDateHelpers(t *testing.T) {
	assert.Equal(t, 2, daysBetween("2026-07-01", "2026-07-03"))
	assert.Equal(t, 0, daysBetween("2026-07-03", "2026-07-03"))
	assert.Equal(t, "2021-07-13", addYears("2026-07-13", -5))
	assert.Equal(t, "2026-05-29", addDays("2026-07-13", -45))
	assert.Equal(t, "2026-07-13T02:03:04.000000000Z",
		NowStamp(time.Date(2026, 7, 13, 2, 3, 4, 0, time.UTC)))
}
```

- [ ] **Step 2: 运行确认失败**

Run: `GOTOOLCHAIN=local go test ./internal/crisis/ -v`
Expected: 编译失败，`undefined: PrevTradingDay` 等

- [ ] **Step 3: 实现 types.go 与 dates.go**

`internal/crisis/types.go`：

```go
// Package crisis implements the macro crisis monitor (Cassandra): ingestion
// of seven market-stress indicators, config-driven threshold rules with
// suppression, and a system-level state machine. sqlite is the single source
// of truth; the process itself is stateless (design §4.3).
package crisis

type Status string

const (
	StatusGreen      Status = "GREEN"
	StatusAmber      Status = "AMBER"
	StatusRed        Status = "RED"
	StatusStale      Status = "STALE"
	StatusSuppressed Status = "SUPPRESSED_SEASONAL"
	StatusNoData     Status = "NO_DATA"
)

type Tag string

const (
	TagComplacency Tag = "COMPLACENCY"
	TagStress      Tag = "STRESS"
	TagCrowded     Tag = "CROWDED"
	TagSteepening  Tag = "STEEPENING"
)

type SystemState string

const (
	StateNormal  SystemState = "NORMAL"
	StateWatch   SystemState = "WATCH"
	StateBrewing SystemState = "BREWING"
	StateCrisis  SystemState = "CRISIS"
)

const (
	IndVIX      = "vix"
	IndMOVE     = "move"
	IndSOFREFFR = "sofr_effr"
	IndHYOAS    = "hy_oas"
	IndT10Y2Y   = "t10y2y"
	IndNFCI     = "nfci"
	IndUSDJPY   = "usdjpy"
)

var AllIndicators = []string{IndVIX, IndMOVE, IndSOFREFFR, IndHYOAS, IndT10Y2Y, IndNFCI, IndUSDJPY}

// Observation is one dated indicator value in canonical units (see the unit
// table in docs/plans/2026-07-13-macro-crisis-monitor-impl.md).
type Observation struct {
	Date      string // 观测日 YYYY-MM-DD
	Indicator string
	Value     float64
	Source    string // fred / yahoo / manual_backfill / derived
	FetchedAt string // 固定宽度 UTC RFC3339
}

// Evaluation is one audit row of crisis_evaluations. Indicator=="" marks the
// system-level row carrying SystemState and the SysDetail JSON.
type Evaluation struct {
	TS          string // 数据观测日 YYYY-MM-DD
	EvalAt      string
	Indicator   string
	Status      Status
	Tag         Tag
	Value       float64
	Pct5y       float64 // 0–1；-1 = 分位窗口为空
	SystemState SystemState
	Detail      string // JSON
}

// IndicatorResult is the in-memory outcome of evaluating one indicator for
// one day. Status is the effective status (after suppression/hysteresis),
// RawStatus the pure rule outcome (persisted in detail for hysteresis).
type IndicatorResult struct {
	Indicator       string
	Status          Status
	RawStatus       Status
	Tag             Tag
	Value           float64
	Pct5y           float64
	WindowActualObs int
}

// SeriesReader is the rules engine's read-only view of observations; the
// sqlite Store and test fixtures both implement it.
type SeriesReader interface {
	// Window returns最近 n 条 ts<=end 的观测，升序。
	Window(indicator, end string, n int) ([]Observation, error)
	// WindowSince returns from<=ts<=end 的全部观测，升序。
	WindowSince(indicator, from, end string) ([]Observation, error)
}

// EvalHistory is the state machine's view of past evaluations (consecutive-day
// counters and hysteresis are rebuilt from it — no in-process counters).
type EvalHistory interface {
	RecentSystem(n int) ([]Evaluation, error)                     // 新→旧
	RecentIndicator(indicator string, n int) ([]Evaluation, error) // 新→旧
}

// severity orders the three colors; non-color statuses sort below GREEN so
// they never win a maxStatus and are excluded from resonance counting.
func severity(s Status) int {
	switch s {
	case StatusGreen:
		return 0
	case StatusAmber:
		return 1
	case StatusRed:
		return 2
	default:
		return -1
	}
}

func maxStatus(a, b Status) Status {
	if severity(b) > severity(a) {
		return b
	}
	return a
}

func isColor(s Status) bool { return severity(s) >= 0 }
```

`internal/crisis/dates.go`：

```go
package crisis

import "time"

const (
	dateLayout = "2006-01-02"
	// timeLayout matches internal/storage/signal: fixed-width UTC RFC3339 so
	// lexicographic order on TEXT columns equals chronological order.
	timeLayout = "2006-01-02T15:04:05.000000000Z07:00"
)

// NowStamp renders t as the fixed-width UTC timestamp for fetched_at/eval_at.
func NowStamp(t time.Time) string { return t.UTC().Format(timeLayout) }

func isWeekend(t time.Time) bool {
	return t.Weekday() == time.Saturday || t.Weekday() == time.Sunday
}

// PrevTradingDay returns the last weekday strictly before t. Weekday ≈ trading
// day: holidays are accepted noise (design §4.3 relies on idempotency).
func PrevTradingDay(t time.Time) time.Time {
	d := t.AddDate(0, 0, -1)
	for isWeekend(d) {
		d = d.AddDate(0, 0, -1)
	}
	return d
}

func addYears(date string, years int) string {
	t, err := time.Parse(dateLayout, date)
	if err != nil {
		return date
	}
	return t.AddDate(years, 0, 0).Format(dateLayout)
}

func addDays(date string, days int) string {
	t, err := time.Parse(dateLayout, date)
	if err != nil {
		return date
	}
	return t.AddDate(0, 0, days).Format(dateLayout)
}

// daysBetween returns whole calendar days from `from` to `to` (0 when equal
// or unparseable).
func daysBetween(from, to string) int {
	f, errF := time.Parse(dateLayout, from)
	t, errT := time.Parse(dateLayout, to)
	if errF != nil || errT != nil {
		return 0
	}
	return int(t.Sub(f).Hours() / 24)
}
```

- [ ] **Step 4: 运行确认通过**

Run: `GOTOOLCHAIN=local go test ./internal/crisis/ -v`
Expected: PASS（TestPrevTradingDay、TestDateHelpers）

- [ ] **Step 5: 写 Store 的失败测试** — 创建 `internal/crisis/store_test.go`

```go
package crisis

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := NewStore(filepath.Join(t.TempDir(), "crisis.db"))
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	return s
}

func TestStoreUpsertIdempotentAndWindows(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	obs := []Observation{
		{Date: "2026-07-01", Indicator: IndVIX, Value: 15, Source: "fred", FetchedAt: "2026-07-02T00:00:00.000000000Z"},
		{Date: "2026-07-02", Indicator: IndVIX, Value: 16, Source: "fred", FetchedAt: "2026-07-03T00:00:00.000000000Z"},
		{Date: "2026-07-03", Indicator: IndVIX, Value: 17, Source: "fred", FetchedAt: "2026-07-04T00:00:00.000000000Z"},
		{Date: "2026-07-03", Indicator: IndHYOAS, Value: 267, Source: "fred", FetchedAt: "2026-07-04T00:00:00.000000000Z"},
	}
	require.NoError(t, s.UpsertObservations(ctx, obs))

	// 同 (ts, indicator) 重写为覆盖而非报错（多时点唤起的幂等基础）
	obs[2].Value = 18
	require.NoError(t, s.UpsertObservations(ctx, obs))

	win, err := s.SeriesWindow(ctx, IndVIX, "2026-07-03", 2)
	require.NoError(t, err)
	require.Len(t, win, 2) // 截断到 n，升序
	assert.Equal(t, 16.0, win[0].Value)
	assert.Equal(t, 18.0, win[1].Value)

	since, err := s.SeriesSince(ctx, IndVIX, "2026-07-02", "2026-07-03")
	require.NoError(t, err)
	require.Len(t, since, 2)
	assert.Equal(t, "2026-07-02", since[0].Date)

	got, err := s.Observation(ctx, IndVIX, "2026-07-02")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, 16.0, got.Value)
	missing, err := s.Observation(ctx, IndVIX, "2026-06-30")
	require.NoError(t, err)
	assert.Nil(t, missing)

	latest, err := s.LatestObservation(ctx, IndVIX)
	require.NoError(t, err)
	require.NotNil(t, latest)
	assert.Equal(t, "2026-07-03", latest.Date)

	dates, err := s.EvalDates(ctx, "2026-07-01", "2026-07-03")
	require.NoError(t, err)
	assert.Equal(t, []string{"2026-07-01", "2026-07-02", "2026-07-03"}, dates)
}

func TestStoreEvaluations(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	empty, err := s.LatestSystemEval(ctx)
	require.NoError(t, err)
	assert.Nil(t, empty)

	evals := []Evaluation{
		{TS: "2026-07-01", EvalAt: "2026-07-02T01:00:00.000000000Z", Indicator: IndVIX,
			Status: StatusGreen, Value: 15, Pct5y: 0.12, Detail: `{"raw":"GREEN"}`},
		{TS: "2026-07-01", EvalAt: "2026-07-02T01:00:00.000000000Z", Indicator: "",
			SystemState: StateNormal, Detail: `{"any_trigger":false}`},
		{TS: "2026-07-02", EvalAt: "2026-07-03T01:00:00.000000000Z", Indicator: IndVIX,
			Status: StatusAmber, Tag: TagStress, Value: 26, Pct5y: 0.91, Detail: `{"raw":"AMBER"}`},
		{TS: "2026-07-02", EvalAt: "2026-07-03T01:00:00.000000000Z", Indicator: "",
			SystemState: StateWatch, Detail: `{"any_trigger":true}`},
	}
	require.NoError(t, s.AppendEvaluations(ctx, evals))

	sys, err := s.RecentSystemEvals(ctx, 5)
	require.NoError(t, err)
	require.Len(t, sys, 2)
	assert.Equal(t, "2026-07-02", sys[0].TS) // 新→旧
	assert.Equal(t, StateWatch, sys[0].SystemState)

	ind, err := s.RecentIndicatorEvals(ctx, IndVIX, 1)
	require.NoError(t, err)
	require.Len(t, ind, 1)
	assert.Equal(t, StatusAmber, ind[0].Status)
	assert.Equal(t, TagStress, ind[0].Tag)

	latest, err := s.LatestSystemEval(ctx)
	require.NoError(t, err)
	require.NotNil(t, latest)
	assert.Equal(t, StateWatch, latest.SystemState)

	has, err := s.HasSystemEvalForDate(ctx, "2026-07-02")
	require.NoError(t, err)
	assert.True(t, has)
	has, err = s.HasSystemEvalForDate(ctx, "2026-07-03")
	require.NoError(t, err)
	assert.False(t, has)

	// Reader / History 适配器冒烟
	w, err := s.Reader(ctx).Window(IndVIX, "2026-07-03", 1)
	require.NoError(t, err)
	assert.Len(t, w, 0) // 本测试未写观测
	h, err := s.History(ctx).RecentSystem(1)
	require.NoError(t, err)
	assert.Len(t, h, 1)
}
```

- [ ] **Step 6: 运行确认失败**

Run: `GOTOOLCHAIN=local go test ./internal/crisis/ -v`
Expected: 编译失败，`undefined: NewStore`

- [ ] **Step 7: 实现 store.go**（模式照抄 `internal/storage/signal/sqlite.go`：MkdirAll → WAL DSN → Ping → schema）

```go
package crisis

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS macro_observations (
	ts          TEXT NOT NULL,
	indicator   TEXT NOT NULL,
	value       REAL,
	source      TEXT,
	fetched_at  TEXT,
	PRIMARY KEY (ts, indicator)
);
CREATE TABLE IF NOT EXISTS crisis_evaluations (
	ts            TEXT NOT NULL,
	eval_at       TEXT NOT NULL,
	indicator     TEXT NOT NULL DEFAULT '',
	status        TEXT NOT NULL DEFAULT '',
	tag           TEXT NOT NULL DEFAULT '',
	value         REAL NOT NULL DEFAULT 0,
	pct_5y        REAL NOT NULL DEFAULT 0,
	system_state  TEXT NOT NULL DEFAULT '',
	detail        TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_macro_obs_ind_ts   ON macro_observations(indicator, ts);
CREATE INDEX IF NOT EXISTS idx_crisis_eval_ind_ts ON crisis_evaluations(indicator, ts);`

// Store is the sqlite-backed source of truth for observations and
// evaluations (WAL + busy_timeout, same conventions as storage/signal).
type Store struct {
	db *sql.DB
}

func NewStore(path string) (*Store, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("creating crisis db dir: %w", err)
		}
	}
	dsn := "file:" + path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening crisis db: %w", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("connecting crisis db: %w", err)
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("creating crisis schema: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// UpsertObservations writes obs in one transaction; the (ts, indicator)
// primary key makes rewrites overwrite, so backfill and repeated daily
// wakeups are idempotent by construction.
func (s *Store) UpsertObservations(ctx context.Context, obs []Observation) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning tx: %w", err)
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx,
		`INSERT OR REPLACE INTO macro_observations (ts, indicator, value, source, fetched_at)
		 VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("preparing upsert: %w", err)
	}
	defer stmt.Close()
	for _, o := range obs {
		if _, err := stmt.ExecContext(ctx, o.Date, o.Indicator, o.Value, o.Source, o.FetchedAt); err != nil {
			return fmt.Errorf("upserting %s/%s: %w", o.Indicator, o.Date, err)
		}
	}
	return tx.Commit()
}

const obsSelect = `SELECT ts, indicator, value, source, fetched_at FROM macro_observations`

func (s *Store) Observation(ctx context.Context, indicator, date string) (*Observation, error) {
	row := s.db.QueryRowContext(ctx, obsSelect+` WHERE indicator = ? AND ts = ?`, indicator, date)
	o, err := scanObservation(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &o, nil
}

func (s *Store) LatestObservation(ctx context.Context, indicator string) (*Observation, error) {
	row := s.db.QueryRowContext(ctx,
		obsSelect+` WHERE indicator = ? ORDER BY ts DESC LIMIT 1`, indicator)
	o, err := scanObservation(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &o, nil
}

// SeriesWindow returns最近 n 条 ts<=end 的观测（升序）：DESC LIMIT 取窗再反转。
func (s *Store) SeriesWindow(ctx context.Context, indicator, end string, n int) ([]Observation, error) {
	rows, err := s.db.QueryContext(ctx,
		obsSelect+` WHERE indicator = ? AND ts <= ? ORDER BY ts DESC LIMIT ?`, indicator, end, n)
	if err != nil {
		return nil, fmt.Errorf("querying window: %w", err)
	}
	out, err := collectObservations(rows)
	if err != nil {
		return nil, err
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

func (s *Store) SeriesSince(ctx context.Context, indicator, from, end string) ([]Observation, error) {
	rows, err := s.db.QueryContext(ctx,
		obsSelect+` WHERE indicator = ? AND ts >= ? AND ts <= ? ORDER BY ts ASC`, indicator, from, end)
	if err != nil {
		return nil, fmt.Errorf("querying range: %w", err)
	}
	return collectObservations(rows)
}

// EvalDates returns vix 的观测日序列（回测的评估日历——vix 覆盖全部验收时段）。
func (s *Store) EvalDates(ctx context.Context, from, to string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT ts FROM macro_observations WHERE indicator = ? AND ts >= ? AND ts <= ? ORDER BY ts ASC`,
		IndVIX, from, to)
	if err != nil {
		return nil, fmt.Errorf("querying eval dates: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) AppendEvaluations(ctx context.Context, evals []Evaluation) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning tx: %w", err)
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO crisis_evaluations (ts, eval_at, indicator, status, tag, value, pct_5y, system_state, detail)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("preparing eval insert: %w", err)
	}
	defer stmt.Close()
	for _, e := range evals {
		if _, err := stmt.ExecContext(ctx, e.TS, e.EvalAt, e.Indicator, string(e.Status),
			string(e.Tag), e.Value, e.Pct5y, string(e.SystemState), e.Detail); err != nil {
			return fmt.Errorf("inserting evaluation %s/%s: %w", e.TS, e.Indicator, err)
		}
	}
	return tx.Commit()
}

const evalSelect = `SELECT ts, eval_at, indicator, status, tag, value, pct_5y, system_state, detail
	FROM crisis_evaluations`

func (s *Store) RecentSystemEvals(ctx context.Context, n int) ([]Evaluation, error) {
	rows, err := s.db.QueryContext(ctx,
		evalSelect+` WHERE indicator = '' ORDER BY ts DESC LIMIT ?`, n)
	if err != nil {
		return nil, fmt.Errorf("querying system evals: %w", err)
	}
	return collectEvaluations(rows)
}

func (s *Store) RecentIndicatorEvals(ctx context.Context, indicator string, n int) ([]Evaluation, error) {
	rows, err := s.db.QueryContext(ctx,
		evalSelect+` WHERE indicator = ? ORDER BY ts DESC LIMIT ?`, indicator, n)
	if err != nil {
		return nil, fmt.Errorf("querying indicator evals: %w", err)
	}
	return collectEvaluations(rows)
}

func (s *Store) LatestSystemEval(ctx context.Context) (*Evaluation, error) {
	evals, err := s.RecentSystemEvals(ctx, 1)
	if err != nil || len(evals) == 0 {
		return nil, err
	}
	return &evals[0], nil
}

func (s *Store) HasSystemEvalForDate(ctx context.Context, date string) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM crisis_evaluations WHERE indicator = '' AND ts = ?`, date).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("checking eval for %s: %w", date, err)
	}
	return n > 0, nil
}

// Reader / History bind a context so the pure engine interfaces stay
// context-free (replay and tests use in-memory implementations instead).
func (s *Store) Reader(ctx context.Context) SeriesReader { return storeReader{ctx, s} }
func (s *Store) History(ctx context.Context) EvalHistory { return storeHistory{ctx, s} }

type storeReader struct {
	ctx context.Context
	s   *Store
}

func (r storeReader) Window(indicator, end string, n int) ([]Observation, error) {
	return r.s.SeriesWindow(r.ctx, indicator, end, n)
}

func (r storeReader) WindowSince(indicator, from, end string) ([]Observation, error) {
	return r.s.SeriesSince(r.ctx, indicator, from, end)
}

type storeHistory struct {
	ctx context.Context
	s   *Store
}

func (h storeHistory) RecentSystem(n int) ([]Evaluation, error) {
	return h.s.RecentSystemEvals(h.ctx, n)
}

func (h storeHistory) RecentIndicator(indicator string, n int) ([]Evaluation, error) {
	return h.s.RecentIndicatorEvals(h.ctx, indicator, n)
}

type scanner interface {
	Scan(dest ...any) error
}

func scanObservation(sc scanner) (Observation, error) {
	var o Observation
	if err := sc.Scan(&o.Date, &o.Indicator, &o.Value, &o.Source, &o.FetchedAt); err != nil {
		return Observation{}, err
	}
	return o, nil
}

func collectObservations(rows *sql.Rows) ([]Observation, error) {
	defer rows.Close()
	out := []Observation{}
	for rows.Next() {
		o, err := scanObservation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func collectEvaluations(rows *sql.Rows) ([]Evaluation, error) {
	defer rows.Close()
	out := []Evaluation{}
	for rows.Next() {
		var (
			e      Evaluation
			status string
			tag    string
			state  string
		)
		if err := rows.Scan(&e.TS, &e.EvalAt, &e.Indicator, &status, &tag,
			&e.Value, &e.Pct5y, &state, &e.Detail); err != nil {
			return nil, err
		}
		e.Status, e.Tag, e.SystemState = Status(status), Tag(tag), SystemState(state)
		out = append(out, e)
	}
	return out, rows.Err()
}
```

- [ ] **Step 8: 运行确认通过**

Run: `GOTOOLCHAIN=local go test ./internal/crisis/ -v`
Expected: PASS（4 个测试全绿）

- [ ] **Step 9: 提交**

```bash
git add internal/crisis/
git commit -m "feat(crisis): add types, date helpers and sqlite store"
```

### Task 2: FRED 采集器 `internal/collector/fred`

**Files:**
- Create: `internal/collector/fred/fred.go`
- Test: `internal/collector/fred/fred_test.go`

**Interfaces:**
- Produces: `fred.New / NewWithBaseURL / (*Client).FetchSeries`（Task 6 ingest 消费）

要点：endpoint `GET {base}/series/observations?series_id=&api_key=&file_type=json&observation_start=&observation_end=`；响应中 `value:"."` 表示缺失、跳过；5xx/网络错误指数退避重试 3 次（退避基准 `backoff` 字段可注入，测试设 1ms）；4xx 不重试直接报错。

- [ ] **Step 1: 写失败测试** — 创建 `internal/collector/fred/fred_test.go`

```go
package fred

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchSeriesParsesAndSkipsMissing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/series/observations", r.URL.Path)
		q := r.URL.Query()
		assert.Equal(t, "VIXCLS", q.Get("series_id"))
		assert.Equal(t, "test-key", q.Get("api_key"))
		assert.Equal(t, "json", q.Get("file_type"))
		assert.Equal(t, "2026-07-01", q.Get("observation_start"))
		assert.Equal(t, "2026-07-03", q.Get("observation_end"))
		fmt.Fprint(w, `{"observations":[
			{"date":"2026-07-01","value":"15.0"},
			{"date":"2026-07-02","value":"."},
			{"date":"2026-07-03","value":"17.5"}]}`)
	}))
	defer srv.Close()

	c := NewWithBaseURL("test-key", srv.URL)
	obs, err := c.FetchSeries(context.Background(), "VIXCLS", "2026-07-01", "2026-07-03")
	require.NoError(t, err)
	require.Len(t, obs, 2) // "." 缺失值被过滤（FRED 约定）
	assert.Equal(t, Observation{Date: "2026-07-01", Value: 15.0}, obs[0])
	assert.Equal(t, Observation{Date: "2026-07-03", Value: 17.5}, obs[1])
}

func TestFetchSeriesRetriesOn5xx(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		fmt.Fprint(w, `{"observations":[{"date":"2026-07-01","value":"1"}]}`)
	}))
	defer srv.Close()

	c := NewWithBaseURL("k", srv.URL)
	c.backoff = time.Millisecond
	obs, err := c.FetchSeries(context.Background(), "SOFR", "", "")
	require.NoError(t, err)
	assert.Len(t, obs, 1)
	assert.Equal(t, 2, calls)
}

func TestFetchSeriesNoRetryOn4xx(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	c := NewWithBaseURL("k", srv.URL)
	c.backoff = time.Millisecond
	_, err := c.FetchSeries(context.Background(), "SOFR", "", "")
	require.Error(t, err)
	assert.Equal(t, 1, calls) // 4xx 不重试
}
```

- [ ] **Step 2: 运行确认失败**

Run: `GOTOOLCHAIN=local go test ./internal/collector/fred/ -v`
Expected: 编译失败，`undefined: NewWithBaseURL`

- [ ] **Step 3: 实现 fred.go**

```go
// Package fred fetches observation series from the FRED API
// (https://fred.stlouisfed.org/docs/api/). Free tier allows ~120 req/min —
// far above this module's 6 series/day, so no client-side rate limiting.
package fred

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const defaultBaseURL = "https://api.stlouisfed.org/fred"

// Observation is one dated value of a series. Missing observations
// (value ".") are filtered out by FetchSeries.
type Observation struct {
	Date  string // YYYY-MM-DD
	Value float64
}

type Client struct {
	apiKey  string
	baseURL string
	client  *http.Client
	backoff time.Duration // 重试退避基准；测试注入 1ms
}

func New(apiKey string) *Client { return NewWithBaseURL(apiKey, defaultBaseURL) }

// NewWithBaseURL is for tests injecting an httptest server.
func NewWithBaseURL(apiKey, baseURL string) *Client {
	return &Client{
		apiKey:  apiKey,
		baseURL: baseURL,
		client:  &http.Client{Timeout: 15 * time.Second},
		backoff: 2 * time.Second,
	}
}

// FetchSeries returns the observations of seriesID within [start, end]
// (either may be empty for FRED's defaults). Transport errors and 5xx are
// retried up to 3 attempts with exponential backoff (design §4.3); 4xx fails
// immediately.
func (c *Client) FetchSeries(ctx context.Context, seriesID, start, end string) ([]Observation, error) {
	q := url.Values{}
	q.Set("series_id", seriesID)
	q.Set("api_key", c.apiKey)
	q.Set("file_type", "json")
	if start != "" {
		q.Set("observation_start", start)
	}
	if end != "" {
		q.Set("observation_end", end)
	}
	reqURL := c.baseURL + "/series/observations?" + q.Encode()

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(c.backoff << (attempt - 1)):
			}
		}
		obs, retryable, err := c.fetchOnce(ctx, reqURL)
		if err == nil {
			return obs, nil
		}
		lastErr = err
		if !retryable {
			return nil, fmt.Errorf("fred %s: %w", seriesID, err)
		}
	}
	return nil, fmt.Errorf("fred %s after retries: %w", seriesID, lastErr)
}

func (c *Client) fetchOnce(ctx context.Context, reqURL string) ([]Observation, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, false, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 {
		return nil, true, fmt.Errorf("http %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("http %d", resp.StatusCode)
	}
	var body struct {
		Observations []struct {
			Date  string `json:"date"`
			Value string `json:"value"`
		} `json:"observations"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, false, fmt.Errorf("decoding response: %w", err)
	}
	out := make([]Observation, 0, len(body.Observations))
	for _, o := range body.Observations {
		if o.Value == "." {
			continue
		}
		v, err := strconv.ParseFloat(o.Value, 64)
		if err != nil {
			return nil, false, fmt.Errorf("parsing %s value %q: %w", o.Date, o.Value, err)
		}
		out = append(out, Observation{Date: o.Date, Value: v})
	}
	return out, false, nil
}
```

- [ ] **Step 4: 运行确认通过**

Run: `GOTOOLCHAIN=local go test ./internal/collector/fred/ -v`
Expected: PASS（3 个测试全绿）

- [ ] **Step 5: 提交**

```bash
git add internal/collector/fred/
git commit -m "feat(fred): add FRED API collector client"
```

### Task 3: yahoo collector 支持货币符号 `JPY=X`

**Files:**
- Modify: `internal/collector/yahoo/yahoo.go:25`（`validSymbol` 正则）
- Test: `internal/collector/yahoo/yahoo_test.go`（追加用例）

**Interfaces:**
- Produces: `yahoo.FetchHistory("JPY=X", ...)` 可用（Task 6 消费；`^MOVE` 现有正则已匹配，无需改动）

**已核实的坑**：现有正则 `[A-Za-z]{1,6}=F` 只放行期货后缀，`JPY=X` 会被 `validateSymbol` 拒绝。改为 `=[FX]`：

```go
var validSymbol = regexp.MustCompile(`^(\^[A-Za-z0-9]{1,10}|[A-Za-z0-9]{1,10}(\.[A-Za-z]{1,4})?|[A-Za-z]{1,6}=[FX])$`)
```

步骤中须先跑 `gitnexus impact({target:"validateSymbol", direction:"upstream"})` 报告影响面（项目 MUST 规则），再改。

- [ ] **Step 1: 影响面分析（项目 MUST 规则）**

运行 gitnexus `impact({target: "validateSymbol", direction: "upstream"})`，向执行记录报告直接调用方与风险级别。预期调用方为 yahoo 包内 FetchQuote/FetchHistory/EPS 路径；本次改动只**放宽**匹配（新增 `=X` 后缀），已匹配的符号集合不变，不影响既有调用方。若报告为 HIGH/CRITICAL，先停下向用户说明再继续。

- [ ] **Step 2: 写失败测试** — 在 `internal/collector/yahoo/yahoo_test.go` 追加

```go
func TestValidateSymbolCurrencyPairs(t *testing.T) {
	assert.NoError(t, validateSymbol("JPY=X"))  // 货币符号（设计 §2.1 USD/JPY）
	assert.NoError(t, validateSymbol("GC=F"))   // 期货符号不受影响
	assert.NoError(t, validateSymbol("^MOVE"))  // 指数符号不受影响
	assert.Error(t, validateSymbol("JPY=Z"))    // 未知后缀仍拒绝
}
```

（若该文件未导入 testify 的 `assert`，按文件现有断言风格改写为等价判断。）

- [ ] **Step 3: 运行确认失败**

Run: `GOTOOLCHAIN=local go test ./internal/collector/yahoo/ -run TestValidateSymbolCurrencyPairs -v`
Expected: FAIL — `JPY=X` 被现有正则拒绝

- [ ] **Step 4: 修改正则**（`internal/collector/yahoo/yahoo.go:25` 附近）

```go
// validSymbol matches stock symbols (AAPL, 600519.SH, 0700.HK),
// index symbols (^GSPC), futures symbols (GC=F) and Yahoo currency
// symbols (JPY=X).
// Validation is purely syntactic and intentionally decoupled from the
// phase-1 coverage list (see design §2.1).
var validSymbol = regexp.MustCompile(`^(\^[A-Za-z0-9]{1,10}|[A-Za-z0-9]{1,10}(\.[A-Za-z]{1,4})?|[A-Za-z]{1,6}=[FX])$`)
```

- [ ] **Step 5: 运行确认通过（含全包回归）**

Run: `GOTOOLCHAIN=local go test ./internal/collector/yahoo/ -v`
Expected: PASS，且既有测试无回归

- [ ] **Step 6: 提交**

```bash
git add internal/collector/yahoo/
git commit -m "fix(yahoo): accept =X currency symbols in validateSymbol"
```

### Task 4: 配置文件 `configs/crisis-monitor.yaml` 与加载

**Files:**
- Create: `configs/crisis-monitor.yaml`、`internal/crisis/config.go`
- Test: `internal/crisis/config_test.go`

**Interfaces:**
- Consumes: viper（既有依赖）
- Produces: `LoadConfig(path) (*Config, error)` 与全部 typed 配置结构（偏差 2）；所有后续任务从 `cfg.Indicators.*` / `cfg.StateMachine.*` 取阈值

YAML 完整内容（设计 §3.1 阈值表 + §3.3 状态机参数的逐项落地，执行者原样写入）：

```yaml
storage:
  path: data/crisis.db

fred:
  api_key_env: FRED_API_KEY

freshness:                     # 设计 §3.2 条 2：超过预期发布时点 48h → STALE
  daily_max_lag_days: 4        # T+1 + 48h + 周末缓冲
  weekly_max_lag_days: 12      # NFCI 周频

percentile:                    # 设计 §3.1 双轨中的分位轨（全指标通用参数）
  window_years: 5
  amber: 0.90
  red: 0.97

indicators:
  vix:
    series: VIXCLS
    amber: 25
    red: 30
    weekly_spike_pct: 0.50     # 单周涨幅 > 50% → AMBER
    percentile_track: true
  move:
    series: ^MOVE
    amber: 100
    red: 120
    percentile_track: true
  sofr_effr:
    amber_bp: 10
    amber_persist_days: 3      # +10~25bp 持续 ≥3 交易日
    red_bp: 25
    red_persist_days: 5        # >+25bp 持续 ≥5 交易日
    percentile_track: true
    suppress_quarter_end: true # 设计 §3.2 条 1
  hy_oas:
    series: BAMLH0A0HYM2
    amber_low_bp: 350          # <350 自满（COMPLACENCY）
    amber_high_bp: 500         # 500–600 压力（STRESS）
    red_bp: 600
    momentum_bp: 100           # 月走阔 >100bp → AMBER(STRESS)
    momentum_window_obs: 21
    percentile_track: true
  t10y2y:
    series: T10Y2Y
    amber_bp: 25               # >25 绿；0~25 黄；<0 红
    steepening_bp: 50          # 倒挂后复陡 >50bp → STEEPENING 标记
    steepening_lookback_obs: 250
    percentile_track: false    # 方向反转（低位才危险），分位轨不适用
  nfci:
    series: NFCI
    green_below: -0.3
    red_above: 0
    percentile_track: true
  usdjpy:
    amber_wow_pct: -0.02       # 周环比 ≤ −2%（日元急升值 = USDJPY 下跌）
    red_wow_pct: -0.03
    crowded_52w_pct: 0.98      # 52 周分位 ≥0.98（USDJPY 极端高位 = 日元空头拥挤）→ CROWDED
    percentile_track: false

state_machine:
  watch_amber_count: 3         # NORMAL→WATCH 的 AMBER 计数阈（AMBER 及以上）
  crisis_exit_days: 10
  watch_exit_days: 20
  brewing_exit_days: 10
  demote_hysteresis_days: 3    # 设计 §3.2 条 3：降级需连续 3 观测日
```

**方向语义（已核实，执行者不需再推导）**：日元"急升值"= USDJPY **下跌**，故 wow 阈值为负数、`≤` 比较；CROWDED 的"52 周极端弱势"指日元弱 = USDJPY 高位，用 52 周窗口的高分位判定。

- [ ] **Step 1: 创建配置文件**

创建 `configs/crisis-monitor.yaml`，内容为本任务上方"YAML 完整内容"代码块的**原样复制**（含注释）。

- [ ] **Step 2: 写失败测试** — 创建 `internal/crisis/config_test.go`

```go
package crisis

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 冒烟测试直接加载仓库内的正式配置，保证 yaml 与 struct 永不脱节。
func TestLoadConfigFromRepoFile(t *testing.T) {
	cfg, err := LoadConfig(filepath.Join("..", "..", "configs", "crisis-monitor.yaml"))
	require.NoError(t, err)

	assert.Equal(t, "data/crisis.db", cfg.Storage.Path)
	assert.Equal(t, "FRED_API_KEY", cfg.FRED.APIKeyEnv)
	assert.Equal(t, 4, cfg.Freshness.DailyMaxLagDays)
	assert.Equal(t, 12, cfg.Freshness.WeeklyMaxLagDays)
	assert.Equal(t, 5, cfg.Percentile.WindowYears)
	assert.Equal(t, 0.90, cfg.Percentile.Amber)
	assert.Equal(t, 0.97, cfg.Percentile.Red)

	assert.Equal(t, 30.0, cfg.Indicators.VIX.Red)
	assert.Equal(t, 0.50, cfg.Indicators.VIX.WeeklySpikePct)
	assert.Equal(t, 120.0, cfg.Indicators.MOVE.Red)
	assert.Equal(t, 3, cfg.Indicators.SOFREFFR.AmberPersistDays)
	assert.Equal(t, 5, cfg.Indicators.SOFREFFR.RedPersistDays)
	assert.True(t, cfg.Indicators.SOFREFFR.SuppressQuarterEnd)
	assert.Equal(t, 350.0, cfg.Indicators.HYOAS.AmberLowBp)
	assert.Equal(t, 600.0, cfg.Indicators.HYOAS.RedBp)
	assert.Equal(t, 21, cfg.Indicators.HYOAS.MomentumWindowObs)
	assert.Equal(t, 250, cfg.Indicators.T10Y2Y.SteepeningLookbackObs)
	assert.False(t, cfg.Indicators.T10Y2Y.PercentileTrack)
	assert.Equal(t, -0.3, cfg.Indicators.NFCI.GreenBelow)
	assert.Equal(t, -0.02, cfg.Indicators.USDJPY.AmberWowPct)
	assert.Equal(t, 0.98, cfg.Indicators.USDJPY.Crowded52wPct)

	assert.Equal(t, 3, cfg.StateMachine.WatchAmberCount)
	assert.Equal(t, 10, cfg.StateMachine.CrisisExitDays)
	assert.Equal(t, 20, cfg.StateMachine.WatchExitDays)
	assert.Equal(t, 10, cfg.StateMachine.BrewingExitDays)
	assert.Equal(t, 3, cfg.StateMachine.DemoteHysteresisDays)
}

func TestLoadConfigValidation(t *testing.T) {
	p := filepath.Join(t.TempDir(), "bad.yaml")
	require.NoError(t, os.WriteFile(p, []byte("fred:\n  api_key_env: X\n"), 0o644))
	_, err := LoadConfig(p)
	require.ErrorContains(t, err, "storage.path")

	_, err = LoadConfig(filepath.Join(t.TempDir(), "absent.yaml"))
	require.Error(t, err)
}
```

- [ ] **Step 3: 运行确认失败**

Run: `GOTOOLCHAIN=local go test ./internal/crisis/ -run TestLoadConfig -v`
Expected: 编译失败，`undefined: LoadConfig`

- [ ] **Step 4: 实现 config.go**

```go
package crisis

import (
	"fmt"

	"github.com/spf13/viper"
)

// Config mirrors configs/crisis-monitor.yaml. Typed per indicator rather than
// a generic threshold DSL because the seven rule shapes differ (persistence,
// two-sided amber, momentum, steepening); every number stays in YAML so
// tuning never needs a release (design §4.1).
type Config struct {
	Storage      StorageCfg      `mapstructure:"storage"`
	FRED         FREDCfg         `mapstructure:"fred"`
	Freshness    FreshnessCfg    `mapstructure:"freshness"`
	Percentile   PercentileCfg   `mapstructure:"percentile"`
	Indicators   IndicatorsCfg   `mapstructure:"indicators"`
	StateMachine StateMachineCfg `mapstructure:"state_machine"`
}

type StorageCfg struct {
	Path string `mapstructure:"path"`
}

type FREDCfg struct {
	APIKeyEnv string `mapstructure:"api_key_env"`
}

type FreshnessCfg struct {
	DailyMaxLagDays  int `mapstructure:"daily_max_lag_days"`
	WeeklyMaxLagDays int `mapstructure:"weekly_max_lag_days"`
}

type PercentileCfg struct {
	WindowYears int     `mapstructure:"window_years"`
	Amber       float64 `mapstructure:"amber"`
	Red         float64 `mapstructure:"red"`
}

type IndicatorsCfg struct {
	VIX      VIXCfg      `mapstructure:"vix"`
	MOVE     MOVECfg     `mapstructure:"move"`
	SOFREFFR SOFREFFRCfg `mapstructure:"sofr_effr"`
	HYOAS    HYOASCfg    `mapstructure:"hy_oas"`
	T10Y2Y   T10Y2YCfg   `mapstructure:"t10y2y"`
	NFCI     NFCICfg     `mapstructure:"nfci"`
	USDJPY   USDJPYCfg   `mapstructure:"usdjpy"`
}

type VIXCfg struct {
	Series          string  `mapstructure:"series"`
	Amber           float64 `mapstructure:"amber"`
	Red             float64 `mapstructure:"red"`
	WeeklySpikePct  float64 `mapstructure:"weekly_spike_pct"`
	PercentileTrack bool    `mapstructure:"percentile_track"`
}

type MOVECfg struct {
	Series          string  `mapstructure:"series"`
	Amber           float64 `mapstructure:"amber"`
	Red             float64 `mapstructure:"red"`
	PercentileTrack bool    `mapstructure:"percentile_track"`
}

type SOFREFFRCfg struct {
	AmberBp            float64 `mapstructure:"amber_bp"`
	AmberPersistDays   int     `mapstructure:"amber_persist_days"`
	RedBp              float64 `mapstructure:"red_bp"`
	RedPersistDays     int     `mapstructure:"red_persist_days"`
	PercentileTrack    bool    `mapstructure:"percentile_track"`
	SuppressQuarterEnd bool    `mapstructure:"suppress_quarter_end"`
}

type HYOASCfg struct {
	Series            string  `mapstructure:"series"`
	AmberLowBp        float64 `mapstructure:"amber_low_bp"`
	AmberHighBp       float64 `mapstructure:"amber_high_bp"`
	RedBp             float64 `mapstructure:"red_bp"`
	MomentumBp        float64 `mapstructure:"momentum_bp"`
	MomentumWindowObs int     `mapstructure:"momentum_window_obs"`
	PercentileTrack   bool    `mapstructure:"percentile_track"`
}

type T10Y2YCfg struct {
	Series                string  `mapstructure:"series"`
	AmberBp               float64 `mapstructure:"amber_bp"`
	SteepeningBp          float64 `mapstructure:"steepening_bp"`
	SteepeningLookbackObs int     `mapstructure:"steepening_lookback_obs"`
	PercentileTrack       bool    `mapstructure:"percentile_track"`
}

type NFCICfg struct {
	Series          string  `mapstructure:"series"`
	GreenBelow      float64 `mapstructure:"green_below"`
	RedAbove        float64 `mapstructure:"red_above"`
	PercentileTrack bool    `mapstructure:"percentile_track"`
}

type USDJPYCfg struct {
	AmberWowPct     float64 `mapstructure:"amber_wow_pct"`
	RedWowPct       float64 `mapstructure:"red_wow_pct"`
	Crowded52wPct   float64 `mapstructure:"crowded_52w_pct"`
	PercentileTrack bool    `mapstructure:"percentile_track"`
}

type StateMachineCfg struct {
	WatchAmberCount      int `mapstructure:"watch_amber_count"`
	CrisisExitDays       int `mapstructure:"crisis_exit_days"`
	WatchExitDays        int `mapstructure:"watch_exit_days"`
	BrewingExitDays      int `mapstructure:"brewing_exit_days"`
	DemoteHysteresisDays int `mapstructure:"demote_hysteresis_days"`
}

func LoadConfig(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("reading crisis config: %w", err)
	}
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("parsing crisis config: %w", err)
	}
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid crisis config %s: %w", path, err)
	}
	return &cfg, nil
}

func (c *Config) validate() error {
	switch {
	case c.Storage.Path == "":
		return fmt.Errorf("storage.path is required")
	case c.FRED.APIKeyEnv == "":
		return fmt.Errorf("fred.api_key_env is required")
	case c.Percentile.WindowYears <= 0:
		return fmt.Errorf("percentile.window_years must be > 0")
	case c.Freshness.DailyMaxLagDays <= 0 || c.Freshness.WeeklyMaxLagDays <= 0:
		return fmt.Errorf("freshness lag days must be > 0")
	case c.StateMachine.WatchAmberCount < 1:
		return fmt.Errorf("state_machine.watch_amber_count must be >= 1")
	case c.StateMachine.DemoteHysteresisDays < 1:
		return fmt.Errorf("state_machine.demote_hysteresis_days must be >= 1")
	case c.StateMachine.CrisisExitDays < 1 || c.StateMachine.WatchExitDays < 1 || c.StateMachine.BrewingExitDays < 1:
		return fmt.Errorf("state_machine exit days must be >= 1")
	}
	return nil
}
```

- [ ] **Step 5: 运行确认通过**

Run: `GOTOOLCHAIN=local go test ./internal/crisis/ -run TestLoadConfig -v`
Expected: PASS（2 个测试全绿）

- [ ] **Step 6: 提交**

```bash
git add configs/crisis-monitor.yaml internal/crisis/config.go internal/crisis/config_test.go
git commit -m "feat(crisis): add config file and typed loader"
```

### Task 5: 派生指标计算 `derive.go`

**Files:**
- Create: `internal/crisis/derive.go`
- Test: `internal/crisis/derive_test.go`

**Interfaces:**
- Consumes: `valuation.PercentileRank`（`internal/valuation/percentile.go:7`，返回 0–100、空序列 -1，除以 100 归一）
- Produces: `SpreadBp / WowPct / MomChange / Percentile`（Task 6 ingest 用 SpreadBp，Task 9 rules 用其余）

- [ ] **Step 1: 写失败测试** — 创建 `internal/crisis/derive_test.go`

```go
package crisis

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// obsSeq 生成从 2026-01-01 起逐日递增的观测序列（多个测试文件复用）。
func obsSeq(vals ...float64) []Observation {
	out := make([]Observation, len(vals))
	for i, v := range vals {
		out[i] = Observation{Date: addDays("2026-01-01", i), Value: v}
	}
	return out
}

func TestSpreadBp(t *testing.T) {
	assert.InDelta(t, 25.0, SpreadBp(4.55, 4.30), 1e-9)
	assert.InDelta(t, -10.0, SpreadBp(4.30, 4.40), 1e-9)
}

func TestWowPct(t *testing.T) {
	w := obsSeq(100, 101, 102, 103, 104, 98) // t-5 观测 = 100 → -2%
	got, ok := WowPct(w)
	require.True(t, ok)
	assert.InDelta(t, -0.02, got, 1e-9)

	_, ok = WowPct(obsSeq(1, 2, 3, 4, 5)) // 不足 6 观测
	assert.False(t, ok)
	_, ok = WowPct(obsSeq(0, 1, 2, 3, 4, 5)) // 基期为 0
	assert.False(t, ok)
}

func TestMomChange(t *testing.T) {
	w := obsSeq(300, 310, 320, 450)
	got, ok := MomChange(w, 3)
	require.True(t, ok)
	assert.InDelta(t, 150.0, got, 1e-9)

	_, ok = MomChange(w, 4) // 观测数不足 n+1
	assert.False(t, ok)
}

func TestPercentile(t *testing.T) {
	p, n := Percentile(obsSeq(1, 2, 3, 4), 3.5)
	assert.Equal(t, 4, n)
	assert.InDelta(t, 0.75, p, 1e-9) // 4 个值中 3 个严格小于 3.5

	p, n = Percentile(nil, 1)
	assert.Equal(t, -1.0, p)
	assert.Equal(t, 0, n)
}
```

- [ ] **Step 2: 运行确认失败**

Run: `GOTOOLCHAIN=local go test ./internal/crisis/ -run 'TestSpreadBp|TestWowPct|TestMomChange|TestPercentile' -v`
Expected: 编译失败，`undefined: SpreadBp` 等

- [ ] **Step 3: 实现 derive.go**

```go
package crisis

import "github.com/newthinker/atlas/internal/valuation"

// SpreadBp converts a SOFR/EFFR pair (both in percent) to a spread in bp
// (design §2.2: sofr_effr_spread_bp).
func SpreadBp(sofr, effr float64) float64 { return (sofr - effr) * 100 }

// WowPct returns close_t/close_{t-5 obs} − 1 over an ascending window
// (design §2.2: usdjpy_wow_pct; also VIX weekly spike). ok=false when fewer
// than 6 observations are available or the base is zero.
func WowPct(window []Observation) (float64, bool) {
	if len(window) < 6 {
		return 0, false
	}
	cur := window[len(window)-1].Value
	base := window[len(window)-6].Value
	if base == 0 {
		return 0, false
	}
	return cur/base - 1, true
}

// MomChange returns window[last] − window[last−n] in the series' own unit
// (design §2.2: hy_oas_mom_bp with n=21). ok=false when fewer than n+1
// observations are available.
func MomChange(window []Observation, n int) (float64, bool) {
	if n <= 0 || len(window) < n+1 {
		return 0, false
	}
	return window[len(window)-1].Value - window[len(window)-1-n].Value, true
}

// Percentile returns current's rank within window as 0–1 plus the actual
// observation count (design §2.2: short windows are used as-is and the
// actual size annotated). Empty window returns (-1, 0).
func Percentile(window []Observation, current float64) (float64, int) {
	if len(window) == 0 {
		return -1, 0
	}
	vals := make([]float64, len(window))
	for i, o := range window {
		vals[i] = o.Value
	}
	return valuation.PercentileRank(vals, current) / 100, len(vals)
}
```

- [ ] **Step 4: 运行确认通过**

Run: `GOTOOLCHAIN=local go test ./internal/crisis/ -run 'TestSpreadBp|TestWowPct|TestMomChange|TestPercentile' -v`
Expected: PASS（4 个测试全绿）

- [ ] **Step 5: 提交**

```bash
git add internal/crisis/derive.go internal/crisis/derive_test.go
git commit -m "feat(crisis): add derived-metric helpers"
```

### Task 6: 采集编排 `ingest.go`

**Files:**
- Create: `internal/crisis/ingest.go`
- Test: `internal/crisis/ingest_test.go`

**Interfaces:**
- Consumes: `fred.Client`（Task 2）、`yahoo.FetchHistory`（Task 3）、`Store.UpsertObservations`（Task 1）、`SpreadBp`（Task 5）
- Produces: `NewIngestor / IngestAll / IngestNFCI / IngestReport`（Task 7 backfill 与 Task 12 eval 消费）

行为要点（设计 §2.1）：
- FRED 日频三序列（vix/hy_oas/t10y2y）按"存储单位约定"表换算入库；`sofr_effr` 由 SOFR、EFFR 两序列**按日期 join**（任一腿缺该日则跳过该日）；NFCI 单独方法（周三 plist 用）。
- Yahoo 两符号（`^MOVE`→move、`JPY=X`→usdjpy）取日收盘；**Yahoo 失败不阻断 FRED**，错误收进 `IngestReport.YahooErrs`（MOVE 缺数降级由 STALE 规则兜底）。
- OHLCV 日期取 `o.Time.UTC().Format("2006-01-02")`。

- [ ] **Step 1: 写失败测试** — 创建 `internal/crisis/ingest_test.go`

```go
package crisis

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/newthinker/atlas/internal/collector/fred"
	"github.com/newthinker/atlas/internal/core"
)

type fakeFRED map[string][]fred.Observation

func (f fakeFRED) FetchSeries(_ context.Context, id, _, _ string) ([]fred.Observation, error) {
	obs, ok := f[id]
	if !ok {
		return nil, fmt.Errorf("no such series %s", id)
	}
	return obs, nil
}

type fakeYahoo struct {
	bars map[string][]core.OHLCV
	err  error
}

func (f fakeYahoo) FetchHistory(symbol string, _, _ time.Time, _ string) ([]core.OHLCV, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.bars[symbol], nil
}

// allFredSeries 提供 6 个 FRED 序列的空实现，测试按需覆盖。
func allFredSeries() fakeFRED {
	return fakeFRED{"VIXCLS": nil, "BAMLH0A0HYM2": nil, "T10Y2Y": nil, "NFCI": nil, "SOFR": nil, "EFFR": nil}
}

func TestIngestAllScalesJoinsAndDegrades(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	ff := allFredSeries()
	ff["VIXCLS"] = []fred.Observation{{Date: "2026-07-01", Value: 15}}
	ff["BAMLH0A0HYM2"] = []fred.Observation{{Date: "2026-07-01", Value: 2.67}}
	ff["T10Y2Y"] = []fred.Observation{{Date: "2026-07-01", Value: 0.35}}
	ff["NFCI"] = []fred.Observation{{Date: "2026-07-01", Value: -0.52}}
	ff["SOFR"] = []fred.Observation{{Date: "2026-07-01", Value: 4.30}, {Date: "2026-07-02", Value: 4.35}}
	ff["EFFR"] = []fred.Observation{{Date: "2026-07-01", Value: 4.40}} // 07-02 缺腿

	ig := NewIngestor(ff, fakeYahoo{err: fmt.Errorf("yahoo down")}, st)
	rep, err := ig.IngestAll(ctx, "2026-07-01", "2026-07-02")
	require.NoError(t, err) // yahoo 失败不阻断（设计 §2.1 缺数降级）

	oas, err := st.Observation(ctx, IndHYOAS, "2026-07-01")
	require.NoError(t, err)
	require.NotNil(t, oas)
	assert.InDelta(t, 267.0, oas.Value, 1e-9) // 百分数 ×100 → bp
	assert.Equal(t, "fred", oas.Source)

	t2, err := st.Observation(ctx, IndT10Y2Y, "2026-07-01")
	require.NoError(t, err)
	require.NotNil(t, t2)
	assert.InDelta(t, 35.0, t2.Value, 1e-9)

	spread, err := st.Observation(ctx, IndSOFREFFR, "2026-07-01")
	require.NoError(t, err)
	require.NotNil(t, spread)
	assert.InDelta(t, -10.0, spread.Value, 1e-9)
	assert.Equal(t, "derived", spread.Source)
	missing, err := st.Observation(ctx, IndSOFREFFR, "2026-07-02")
	require.NoError(t, err)
	assert.Nil(t, missing) // 缺腿日跳过

	assert.Len(t, rep.YahooErrs, 2) // move 与 usdjpy 都失败
	assert.Equal(t, 1, rep.Counts[IndVIX])

	// NFCI 单独刷新
	n, err := ig.IngestNFCI(ctx, "2026-07-01", "2026-07-02")
	require.NoError(t, err)
	assert.Equal(t, 1, n)
}

func TestIngestYahooClose(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	bar := time.Date(2026, 7, 10, 14, 30, 0, 0, time.UTC)
	fy := fakeYahoo{bars: map[string][]core.OHLCV{
		"JPY=X": {{Close: 161.7, Time: bar}},
		"^MOVE": {{Close: 69.6, Time: bar}},
	}}
	ig := NewIngestor(allFredSeries(), fy, st)
	rep, err := ig.IngestAll(ctx, "2026-07-01", "2026-07-10")
	require.NoError(t, err)
	assert.Empty(t, rep.YahooErrs)

	jpy, err := st.Observation(ctx, IndUSDJPY, "2026-07-10")
	require.NoError(t, err)
	require.NotNil(t, jpy)
	assert.InDelta(t, 161.7, jpy.Value, 1e-9)
	assert.Equal(t, "yahoo", jpy.Source)

	mv, err := st.Observation(ctx, IndMOVE, "2026-07-10")
	require.NoError(t, err)
	require.NotNil(t, mv)
	assert.InDelta(t, 69.6, mv.Value, 1e-9)
}
```

- [ ] **Step 2: 运行确认失败**

Run: `GOTOOLCHAIN=local go test ./internal/crisis/ -run TestIngest -v`
Expected: 编译失败，`undefined: NewIngestor`

- [ ] **Step 3: 实现 ingest.go**

```go
package crisis

import (
	"context"
	"fmt"
	"time"

	"github.com/newthinker/atlas/internal/collector/fred"
	"github.com/newthinker/atlas/internal/core"
)

// FREDFetcher / HistoryFetcher are the two upstream dependencies, narrowed to
// interfaces so tests inject fakes (fred.Client and yahoo.Yahoo satisfy them).
type FREDFetcher interface {
	FetchSeries(ctx context.Context, seriesID, start, end string) ([]fred.Observation, error)
}

type HistoryFetcher interface {
	FetchHistory(symbol string, start, end time.Time, interval string) ([]core.OHLCV, error)
}

// IngestReport carries per-indicator row counts plus non-fatal yahoo errors:
// MOVE/USDJPY failures degrade to the STALE path instead of blocking FRED
// ingestion (design §2.1).
type IngestReport struct {
	Counts    map[string]int
	YahooErrs map[string]error
}

type Ingestor struct {
	fred  FREDFetcher
	yahoo HistoryFetcher
	store *Store
	now   func() time.Time
}

func NewIngestor(f FREDFetcher, y HistoryFetcher, s *Store) *Ingestor {
	return &Ingestor{fred: f, yahoo: y, store: s, now: time.Now}
}

// fredDirect are the FRED series stored as-is (scale converts percent→bp per
// the canonical-unit table in the implementation plan).
var fredDirect = []struct {
	indicator string
	series    string
	scale     float64
}{
	{IndVIX, "VIXCLS", 1},
	{IndHYOAS, "BAMLH0A0HYM2", 100},
	{IndT10Y2Y, "T10Y2Y", 100},
	{IndNFCI, "NFCI", 1},
}

var yahooSymbols = map[string]string{IndMOVE: "^MOVE", IndUSDJPY: "JPY=X"}

// IngestAll fetches every indicator for [from, to]: FRED errors abort (the
// daily eval retries next wakeup), yahoo errors are collected in the report.
func (ig *Ingestor) IngestAll(ctx context.Context, from, to string) (*IngestReport, error) {
	rep := &IngestReport{Counts: map[string]int{}, YahooErrs: map[string]error{}}
	stamp := NowStamp(ig.now())

	for _, fs := range fredDirect {
		n, err := ig.ingestFredSeries(ctx, fs.series, fs.indicator, fs.scale, from, to, stamp)
		if err != nil {
			return nil, err
		}
		rep.Counts[fs.indicator] = n
	}

	n, err := ig.ingestSpread(ctx, from, to, stamp)
	if err != nil {
		return nil, err
	}
	rep.Counts[IndSOFREFFR] = n

	for ind, sym := range yahooSymbols {
		n, err := ig.ingestYahoo(ctx, ind, sym, from, to, stamp)
		if err != nil {
			rep.YahooErrs[ind] = err
			continue
		}
		rep.Counts[ind] = n
	}
	return rep, nil
}

// IngestNFCI refreshes only the weekly NFCI series (Wednesday plist, design §4.3).
func (ig *Ingestor) IngestNFCI(ctx context.Context, from, to string) (int, error) {
	return ig.ingestFredSeries(ctx, "NFCI", IndNFCI, 1, from, to, NowStamp(ig.now()))
}

func (ig *Ingestor) ingestFredSeries(ctx context.Context, seriesID, indicator string, scale float64, from, to, stamp string) (int, error) {
	obs, err := ig.fred.FetchSeries(ctx, seriesID, from, to)
	if err != nil {
		return 0, fmt.Errorf("fetching %s: %w", seriesID, err)
	}
	rows := make([]Observation, 0, len(obs))
	for _, o := range obs {
		rows = append(rows, Observation{
			Date: o.Date, Indicator: indicator, Value: o.Value * scale,
			Source: "fred", FetchedAt: stamp,
		})
	}
	if err := ig.store.UpsertObservations(ctx, rows); err != nil {
		return 0, err
	}
	return len(rows), nil
}

// ingestSpread joins SOFR and EFFR by date and stores the spread in bp
// (design §2.2; days missing either leg are skipped).
func (ig *Ingestor) ingestSpread(ctx context.Context, from, to, stamp string) (int, error) {
	sofr, err := ig.fred.FetchSeries(ctx, "SOFR", from, to)
	if err != nil {
		return 0, fmt.Errorf("fetching SOFR: %w", err)
	}
	effr, err := ig.fred.FetchSeries(ctx, "EFFR", from, to)
	if err != nil {
		return 0, fmt.Errorf("fetching EFFR: %w", err)
	}
	effrByDate := make(map[string]float64, len(effr))
	for _, o := range effr {
		effrByDate[o.Date] = o.Value
	}
	rows := []Observation{}
	for _, o := range sofr {
		e, ok := effrByDate[o.Date]
		if !ok {
			continue
		}
		rows = append(rows, Observation{
			Date: o.Date, Indicator: IndSOFREFFR, Value: SpreadBp(o.Value, e),
			Source: "derived", FetchedAt: stamp,
		})
	}
	if err := ig.store.UpsertObservations(ctx, rows); err != nil {
		return 0, err
	}
	return len(rows), nil
}

func (ig *Ingestor) ingestYahoo(ctx context.Context, indicator, symbol, from, to, stamp string) (int, error) {
	start, err := time.Parse(dateLayout, from)
	if err != nil {
		return 0, fmt.Errorf("parsing from %q: %w", from, err)
	}
	end, err := time.Parse(dateLayout, to)
	if err != nil {
		return 0, fmt.Errorf("parsing to %q: %w", to, err)
	}
	// end+1d：yahoo chart 区间对当日收盘的包含性不稳定，多取一天由 upsert 幂等兜底
	bars, err := ig.yahoo.FetchHistory(symbol, start, end.AddDate(0, 0, 1), "1d")
	if err != nil {
		return 0, err
	}
	rows := make([]Observation, 0, len(bars))
	for _, b := range bars {
		rows = append(rows, Observation{
			Date: b.Time.UTC().Format(dateLayout), Indicator: indicator, Value: b.Close,
			Source: "yahoo", FetchedAt: stamp,
		})
	}
	if err := ig.store.UpsertObservations(ctx, rows); err != nil {
		return 0, err
	}
	return len(rows), nil
}
```

- [ ] **Step 4: 运行确认通过**

Run: `GOTOOLCHAIN=local go test ./internal/crisis/ -run TestIngest -v`
Expected: PASS（2 个测试全绿）

- [ ] **Step 5: 提交**

```bash
git add internal/crisis/ingest.go internal/crisis/ingest_test.go
git commit -m "feat(crisis): add ingest orchestration"
```

### Task 7: CLI 骨架 + `crisis backfill` 子命令（第一阶段收口）

**Files:**
- Create: `cmd/atlas/crisis.go`
- Test: `cmd/atlas/crisis_test.go`

**Interfaces:**
- Consumes: `crisis.LoadConfig / NewStore / NewIngestor`、`fred.New`、`yahoo.New`、cobra 平铺注册惯例（`cmd/atlas/watchlist.go:36` 的 init 模式）
- Produces: `atlas crisis` 父命令 + `backfill` 子命令；`--crisis-config`（默认 `configs/crisis-monitor.yaml`）persistent flag；`importCSV` 助手（Task 12/13 复用 openCrisisStore 等私有助手）

命令面：
```
atlas crisis backfill --from 2006-01-01 [--to YYYY-MM-DD]          # FRED+Yahoo 全量回填
atlas crisis backfill --csv path --indicator hy_oas [--scale 100]  # 人工快照导入（设计 §4.3）
```
CSV 格式 `date,value` 两列、可带表头；`--scale` 用于百分数→bp（ICE 快照通常为百分数，导入 hy_oas 用 `--scale 100`）。

**第一阶段人工验收（设计 §6 第一步）：**

- [ ] `GOTOOLCHAIN=local go build -o bin/atlas ./cmd/atlas`
- [ ] FRED key 已在 `configs/config.yaml` 的 `collectors.fred.api_key`（env `FRED_API_KEY` 可覆盖），运行 `bin/atlas crisis backfill -c configs/config.yaml --from 2006-01-01`
- [ ] `bin/atlas crisis backfill --csv <HY-OAS快照.csv> --indicator hy_oas --scale 100`
- [ ] 用 `sqlite3 data/crisis.db` 抽查 3 个日期的 vix/hy_oas/t10y2y 读数与 FRED 官网一致（hy_oas/t10y2y 注意 ×100）
- [ ] 对照设计附录基线（2026-07-12：VIX 15.0 / MOVE 69.6 / SOFR−EFFR −10bp / HY OAS 267bp / 10Y−2Y +35bp / NFCI −0.52 / USDJPY 161.7）核对最近观测

- [ ] **Step 1: 写失败测试** — 创建 `cmd/atlas/crisis_test.go`

```go
package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/newthinker/atlas/internal/crisis"
)

func newCrisisTestStore(t *testing.T) *crisis.Store {
	t.Helper()
	st, err := crisis.NewStore(filepath.Join(t.TempDir(), "crisis.db"))
	require.NoError(t, err)
	t.Cleanup(func() { st.Close() })
	return st
}

func TestImportCSVFrom(t *testing.T) {
	st := newCrisisTestStore(t)
	ctx := context.Background()

	// 表头 + 两行数据，scale=100（百分数 → bp）
	csvData := "date,value\n2006-01-03,3.55\n2006-01-04,3.60\n"
	n, err := importCSVFrom(ctx, st, strings.NewReader(csvData), "hy_oas", 100)
	require.NoError(t, err)
	assert.Equal(t, 2, n)

	obs, err := st.Observation(ctx, "hy_oas", "2006-01-03")
	require.NoError(t, err)
	require.NotNil(t, obs)
	assert.InDelta(t, 355.0, obs.Value, 1e-9)
	assert.Equal(t, "manual_backfill", obs.Source)

	// 无表头也可导入
	n, err = importCSVFrom(ctx, st, strings.NewReader("2006-01-05,3.70\n"), "hy_oas", 100)
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	// 坏值与坏日期（非首行）报错
	_, err = importCSVFrom(ctx, st, strings.NewReader("2006-01-06,abc\n"), "hy_oas", 1)
	require.Error(t, err)
	_, err = importCSVFrom(ctx, st, strings.NewReader("date,value\nnot-a-date,1\n"), "hy_oas", 1)
	require.Error(t, err)
}

func TestCrisisCommandRegistered(t *testing.T) {
	var crisisUse []string
	for _, c := range rootCmd.Commands() {
		if c.Use == "crisis" {
			for _, sub := range c.Commands() {
				crisisUse = append(crisisUse, sub.Use)
			}
		}
	}
	assert.Contains(t, crisisUse, "backfill")
}
```

- [ ] **Step 2: 运行确认失败**

Run: `GOTOOLCHAIN=local go test ./cmd/atlas/ -run 'TestImportCSVFrom|TestCrisisCommandRegistered' -v`
Expected: 编译失败，`undefined: importCSVFrom`

- [ ] **Step 3: 实现 crisis.go**（子命令平铺注册，模式同 `cmd/atlas/watchlist.go`）

```go
package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/newthinker/atlas/internal/collector/fred"
	"github.com/newthinker/atlas/internal/collector/yahoo"
	"github.com/newthinker/atlas/internal/crisis"
)

var (
	crisisCfgPath     string
	backfillFrom      string
	backfillTo        string
	backfillCSV       string
	backfillIndicator string
	backfillScale     float64
)

var crisisCmd = &cobra.Command{
	Use:   "crisis",
	Short: "Macro crisis monitor (Cassandra)",
	Long: `Systemic-risk monitor: seven market-stress indicators, three-color
rules and a NORMAL/WATCH/BREWING/CRISIS state machine. Risk states only —
never trade signals (see docs/plans/atlas-macro-crisis-monitor-design.md).`,
}

var crisisBackfillCmd = &cobra.Command{
	Use:   "backfill",
	Short: "Backfill indicator history from FRED/Yahoo or a CSV snapshot",
	RunE:  runCrisisBackfill,
}

func init() {
	crisisCmd.PersistentFlags().StringVar(&crisisCfgPath, "crisis-config",
		"configs/crisis-monitor.yaml", "crisis monitor config path")
	crisisBackfillCmd.Flags().StringVar(&backfillFrom, "from", "", "start date YYYY-MM-DD (FRED/Yahoo backfill)")
	crisisBackfillCmd.Flags().StringVar(&backfillTo, "to", "", "end date YYYY-MM-DD (default today)")
	crisisBackfillCmd.Flags().StringVar(&backfillCSV, "csv", "", "CSV snapshot path (date,value)")
	crisisBackfillCmd.Flags().StringVar(&backfillIndicator, "indicator", "", "indicator for --csv import (e.g. hy_oas)")
	crisisBackfillCmd.Flags().Float64Var(&backfillScale, "scale", 1, "value multiplier for --csv (percent→bp: 100)")
	crisisCmd.AddCommand(crisisBackfillCmd)
	rootCmd.AddCommand(crisisCmd)
}

func openCrisisStore() (*crisis.Config, *crisis.Store, error) {
	ccfg, err := crisis.LoadConfig(crisisCfgPath)
	if err != nil {
		return nil, nil, err
	}
	st, err := crisis.NewStore(ccfg.Storage.Path)
	if err != nil {
		return nil, nil, err
	}
	return ccfg, st, nil
}

// resolveFREDKey：环境变量优先（launchd/CI 可临时覆盖），否则回退主配置
// collectors.fred.api_key —— configs/config.yaml 在 .gitignore 中，与
// telegram/lixinger 凭据同层，密钥不入库。回退路径依赖根命令的 -c/--config。
func resolveFREDKey(envName string) string {
	if k := os.Getenv(envName); k != "" {
		return k
	}
	cfg, err := loadConfigOrDefaults()
	if err != nil {
		return ""
	}
	if fc, ok := cfg.Collectors["fred"]; ok {
		return fc.APIKey
	}
	return ""
}

func runCrisisBackfill(cmd *cobra.Command, args []string) error {
	ccfg, st, err := openCrisisStore()
	if err != nil {
		return err
	}
	defer st.Close()
	ctx := cmd.Context()

	if backfillCSV != "" {
		if backfillIndicator == "" {
			return fmt.Errorf("--csv requires --indicator")
		}
		n, err := importCSV(ctx, st, backfillCSV, backfillIndicator, backfillScale)
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "imported %d observations for %s\n", n, backfillIndicator)
		return nil
	}

	if backfillFrom == "" {
		return fmt.Errorf("--from is required (or use --csv)")
	}
	apiKey := resolveFREDKey(ccfg.FRED.APIKeyEnv)
	if apiKey == "" {
		return fmt.Errorf("FRED key missing: set env %s or collectors.fred.api_key in the main config (-c)", ccfg.FRED.APIKeyEnv)
	}
	to := backfillTo
	if to == "" {
		to = time.Now().UTC().Format("2006-01-02")
	}
	ig := crisis.NewIngestor(fred.New(apiKey), yahoo.New(), st)
	rep, err := ig.IngestAll(ctx, backfillFrom, to)
	if err != nil {
		return err
	}
	for ind, n := range rep.Counts {
		fmt.Fprintf(cmd.OutOrStdout(), "%-10s %6d rows\n", ind, n)
	}
	for ind, ferr := range rep.YahooErrs {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: yahoo %s failed: %v (degrades to STALE)\n", ind, ferr)
	}
	return nil
}

func importCSV(ctx context.Context, st *crisis.Store, path, indicator string, scale float64) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	return importCSVFrom(ctx, st, f, indicator, scale)
}

// importCSVFrom reads date,value rows (optional header), multiplies values by
// scale and upserts them as manual_backfill observations (design §4.3: the
// HY OAS snapshot predating FRED's 3-year truncation comes in this way).
func importCSVFrom(ctx context.Context, st *crisis.Store, r io.Reader, indicator string, scale float64) (int, error) {
	rows, err := csv.NewReader(r).ReadAll()
	if err != nil {
		return 0, err
	}
	stamp := crisis.NowStamp(time.Now())
	obs := make([]crisis.Observation, 0, len(rows))
	for i, rec := range rows {
		if len(rec) < 2 {
			return 0, fmt.Errorf("line %d: want 2 columns date,value", i+1)
		}
		date := strings.TrimSpace(rec[0])
		if _, err := time.Parse("2006-01-02", date); err != nil {
			if i == 0 {
				continue // 表头
			}
			return 0, fmt.Errorf("line %d: bad date %q", i+1, date)
		}
		v, err := strconv.ParseFloat(strings.TrimSpace(rec[1]), 64)
		if err != nil {
			return 0, fmt.Errorf("line %d: bad value %q", i+1, rec[1])
		}
		obs = append(obs, crisis.Observation{
			Date: date, Indicator: indicator, Value: v * scale,
			Source: "manual_backfill", FetchedAt: stamp,
		})
	}
	if err := st.UpsertObservations(ctx, obs); err != nil {
		return 0, err
	}
	return len(obs), nil
}
```

- [ ] **Step 4: 运行确认通过（含 cmd 包回归）**

Run: `GOTOOLCHAIN=local go test ./cmd/atlas/ -v`
Expected: PASS，既有 cmd 测试无回归

- [ ] **Step 5: 提交**

```bash
git add cmd/atlas/crisis.go cmd/atlas/crisis_test.go
git commit -m "feat(crisis): add CLI skeleton and backfill command"
```

- [ ] **Step 6: 执行本任务下方的"第一阶段人工验收"清单**（需要 FRED_API_KEY 与 HY OAS 快照 CSV，属人工前置依赖；验收结果记入执行记录）

## 第二阶段：规则、状态机与回测（Task 8–13）

### Task 8: 抑制、防抖与新鲜度 `suppress.go`

**Files:**
- Create: `internal/crisis/suppress.go`
- Test: `internal/crisis/suppress_test.go`

**Interfaces:**
- Consumes: Task 1 types/dates、Task 4 `Config.Freshness`
- Produces: `InQuarterEndWindow / staleFor / ApplyHysteresis`（Task 9 rules 用 staleFor，Task 11 EvalDay 用另两个）

语义（设计 §3.2）：
- `InQuarterEndWindow`：季度最后 3 个交易日 ∪ 下季度前 2 个交易日（周内日近似）。测试锚点（已核实日历）：2026-03-27(五)/03-30(一)/03-31(二) 与 04-01(三)/04-02(四) 为真；03-26、04-03、周末为假。
- `staleFor`：`daysBetween(latestObsDate, evalDate) > maxLag`，nfci 用 weekly、其余 daily。MOVE"连续 3 日拉取失败→STALE"由该新鲜度窗口等效覆盖（不单独计失败次数）。
- `ApplyHysteresis(raw, prev, days)`：升级立即生效；降级要求今日 raw 加上此前 `days-1` 个评估日的 raw（从指标行 detail JSON 的 `raw` 字段读，缺失回退 status 列）severity 全部 ≤ 目标档，否则维持昨日生效状态；历史行数不足 `days-1` 时不降级。

- [ ] **Step 1: 写失败测试** — 创建 `internal/crisis/suppress_test.go`

```go
package crisis

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInQuarterEndWindow(t *testing.T) {
	// 2026-03-31 = 周二（已核实日历）：季末最后 3 交易日 = 3/27(五)、3/30(一)、3/31(二)
	for _, d := range []string{"2026-03-27", "2026-03-30", "2026-03-31", "2026-04-01", "2026-04-02", "2026-12-31"} {
		assert.True(t, InQuarterEndWindow(d), d)
	}
	for _, d := range []string{
		"2026-03-26", // 季末窗口前一交易日
		"2026-04-03", // 季初窗口后一交易日
		"2026-03-28", // 周六
		"2026-05-15", // 季中
		"bad-date",
	} {
		assert.False(t, InQuarterEndWindow(d), d)
	}
}

func TestStaleFor(t *testing.T) {
	cfg := &Config{Freshness: FreshnessCfg{DailyMaxLagDays: 4, WeeklyMaxLagDays: 12}}
	assert.True(t, staleFor(cfg, IndVIX, "2026-07-10", "2026-07-03"))   // 7 天 > 4
	assert.False(t, staleFor(cfg, IndVIX, "2026-07-10", "2026-07-08"))  // 2 天
	assert.False(t, staleFor(cfg, IndNFCI, "2026-07-13", "2026-07-01")) // 周频 12 天上限
	assert.True(t, staleFor(cfg, IndNFCI, "2026-07-14", "2026-07-01"))
}

func evalWithRaw(status, raw Status) Evaluation {
	return Evaluation{Status: status, Detail: `{"raw":"` + string(raw) + `"}`}
}

func TestApplyHysteresis(t *testing.T) {
	// 升级立即生效
	assert.Equal(t, StatusRed,
		ApplyHysteresis(StatusRed, []Evaluation{evalWithRaw(StatusGreen, StatusGreen)}, 3))
	// 无历史 → 原样（冷启动首日）
	assert.Equal(t, StatusGreen, ApplyHysteresis(StatusGreen, nil, 3))
	// 降级被昨日 raw AMBER 挡住 → 维持昨日生效状态
	blocked := []Evaluation{evalWithRaw(StatusAmber, StatusAmber), evalWithRaw(StatusAmber, StatusAmber)}
	assert.Equal(t, StatusAmber, ApplyHysteresis(StatusGreen, blocked, 3))
	// 今日 + 此前 2 日 raw 均 GREEN = 连续 3 观测日 → 放行降级
	clear := []Evaluation{evalWithRaw(StatusAmber, StatusGreen), evalWithRaw(StatusAmber, StatusGreen)}
	assert.Equal(t, StatusGreen, ApplyHysteresis(StatusGreen, clear, 3))
	// 历史不足 days-1 → 维持
	short := []Evaluation{evalWithRaw(StatusAmber, StatusGreen)}
	assert.Equal(t, StatusAmber, ApplyHysteresis(StatusGreen, short, 3))
	// 历史中夹非色彩状态（STALE）→ 无法确认连续低档，维持
	stale := []Evaluation{evalWithRaw(StatusAmber, StatusGreen), evalWithRaw(StatusStale, StatusStale)}
	assert.Equal(t, StatusAmber, ApplyHysteresis(StatusGreen, stale, 3))
}
```

- [ ] **Step 2: 运行确认失败**

Run: `GOTOOLCHAIN=local go test ./internal/crisis/ -run 'TestInQuarterEndWindow|TestStaleFor|TestApplyHysteresis' -v`
Expected: 编译失败，`undefined: InQuarterEndWindow`

- [ ] **Step 3: 实现 suppress.go**

```go
package crisis

import (
	"encoding/json"
	"time"
)

// InQuarterEndWindow reports whether date falls in the quarter-end
// suppression window: the last 3 trading days of a quarter or the first 2 of
// the next (design §3.2 rule 1 — repo-market spikes there are routine).
// Weekday ≈ trading day; weekends and unparseable dates are never in-window.
func InQuarterEndWindow(date string) bool {
	d, err := time.Parse(dateLayout, date)
	if err != nil || isWeekend(d) {
		return false
	}
	cur := lastTradingDayOnOrBefore(quarterEnd(d))
	for i := 0; i < 3; i++ {
		if cur.Equal(d) {
			return true
		}
		cur = PrevTradingDay(cur)
	}
	cur = firstTradingDayOnOrAfter(quarterStart(d))
	for i := 0; i < 2; i++ {
		if cur.Equal(d) {
			return true
		}
		cur = nextTradingDay(cur)
	}
	return false
}

func quarterStart(t time.Time) time.Time {
	m := ((int(t.Month())-1)/3)*3 + 1
	return time.Date(t.Year(), time.Month(m), 1, 0, 0, 0, 0, time.UTC)
}

func quarterEnd(t time.Time) time.Time { return quarterStart(t).AddDate(0, 3, -1) }

func lastTradingDayOnOrBefore(t time.Time) time.Time {
	for isWeekend(t) {
		t = t.AddDate(0, 0, -1)
	}
	return t
}

func firstTradingDayOnOrAfter(t time.Time) time.Time {
	for isWeekend(t) {
		t = t.AddDate(0, 0, 1)
	}
	return t
}

func nextTradingDay(t time.Time) time.Time {
	d := t.AddDate(0, 0, 1)
	for isWeekend(d) {
		d = d.AddDate(0, 0, 1)
	}
	return d
}

// staleFor implements design §3.2 rule 2 (≈48h beyond expected publication,
// widened via config for weekends/holidays). It also covers the MOVE
// "3 consecutive fetch failures" degradation: failed fetches leave the latest
// observation aging past the window. NFCI uses the weekly allowance.
func staleFor(cfg *Config, indicator, evalDate, latestObsDate string) bool {
	maxLag := cfg.Freshness.DailyMaxLagDays
	if indicator == IndNFCI {
		maxLag = cfg.Freshness.WeeklyMaxLagDays
	}
	return daysBetween(latestObsDate, evalDate) > maxLag
}

// indDetail is the JSON persisted in indicator evaluation rows; Raw feeds the
// hysteresis on later days.
type indDetail struct {
	Raw             Status `json:"raw"`
	WindowActualObs int    `json:"window_actual_obs"`
}

func rawFromDetail(e Evaluation) Status {
	var d indDetail
	if err := json.Unmarshal([]byte(e.Detail), &d); err == nil && d.Raw != "" {
		return d.Raw
	}
	return e.Status
}

// ApplyHysteresis implements design §3.2 rule 3 (asymmetric debounce):
// upgrades take effect immediately; a downgrade needs `days` consecutive
// observation days — today plus days-1 prior raw statuses — at or below the
// target level, otherwise yesterday's effective status is kept. prev is
// newest-first. Insufficient or non-color history blocks the downgrade.
func ApplyHysteresis(raw Status, prev []Evaluation, days int) Status {
	if len(prev) == 0 || !isColor(raw) {
		return raw
	}
	prevEff := prev[0].Status
	if !isColor(prevEff) || severity(raw) >= severity(prevEff) {
		return raw
	}
	need := days - 1
	if len(prev) < need {
		return prevEff
	}
	for i := 0; i < need; i++ {
		r := rawFromDetail(prev[i])
		if !isColor(r) || severity(r) > severity(raw) {
			return prevEff
		}
	}
	return raw
}
```

- [ ] **Step 4: 运行确认通过**

Run: `GOTOOLCHAIN=local go test ./internal/crisis/ -run 'TestInQuarterEndWindow|TestStaleFor|TestApplyHysteresis' -v`
Expected: PASS（3 个测试全绿）

- [ ] **Step 5: 提交**

```bash
git add internal/crisis/suppress.go internal/crisis/suppress_test.go
git commit -m "feat(crisis): add suppression, staleness and hysteresis"
```

### Task 9: 单指标规则引擎 `rules.go`

**Files:**
- Create: `internal/crisis/rules.go`
- Test: `internal/crisis/rules_test.go`（内含 `memSeries` fixture，实现 SeriesReader，供 Task 10/11 测试复用）

**Interfaces:**
- Consumes: Task 4 Config、Task 5 derive、Task 8 staleFor、SeriesReader
- Produces: `EvaluateIndicator(cfg, indicator, date, sr) (IndicatorResult, error)`

评估流水线（对全部 7 指标统一）：
1. `Window(ind, date, 1)` 空 → `NO_DATA`；`staleFor` → `STALE`（两者均直接返回，退出共振）。
2. 指标专属绝对阈值轨（设计 §3.1 表逐行）：
   - **vix**：`>red` 红；`[amber,red]` 黄；6 观测窗 `WowPct > weekly_spike_pct` → 至少黄。
   - **move**：`>red` 红；`[amber,red]` 黄。
   - **sofr_effr**：最近 `red_persist_days` 观测全 `>red_bp` → 红；最近 `amber_persist_days` 观测全 `>amber_bp` → 黄（持续性条件，设计 §3.1 说明三）。
   - **hy_oas**：`>red_bp` 红；`<amber_low_bp` 黄+`COMPLACENCY`；`[amber_high_bp,red_bp]` 黄+`STRESS`；`MomChange(win, momentum_window_obs) > momentum_bp` → 至少黄、无 tag 时补 `STRESS`（双向黄灯，设计 §3.1 说明一）。
   - **t10y2y**：`<0` 红；`[0,amber_bp]` 黄；否则绿。`steepening_lookback_obs` 窗口内最小值 `<0` 且 `当前−最小值 > steepening_bp` → 附加 `STEEPENING` tag（不改色，设计 §3.1 说明二）。
   - **nfci**：`>red_above` 红；`[green_below,red_above]` 黄；否则绿。
   - **usdjpy**：`WowPct ≤ red_wow_pct` 红；`≤ amber_wow_pct` 黄；52 周窗口 `Percentile ≥ crowded_52w_pct`（观测数 ≥60）→ 至少黄+`CROWDED`。
3. 分位轨（`percentile_track: true` 且 5 年窗口观测数 ≥60，偏差 3）：`Pct5y ≥ percentile.red` → 升红；`≥ percentile.amber` → 升黄。**两轨任一触发即升级**（maxStatus 合成）。
4. `Pct5y`/`WindowActualObs` 恒填充（含 track=false 指标，供通知展示）。

**验收锚点测试**：用设计附录基线值构造 fixture，断言 vix/move/sofr_effr/t10y2y/nfci 绿、hy_oas 黄+COMPLACENCY、usdjpy 黄+CROWDED（AMBER 计数 = 2）。

- [ ] **Step 1: 写失败测试** — 创建 `internal/crisis/rules_test.go`（含 `memSeries`、`seriesEnding`、`baselineSeries`、`testConfig` 四个 fixture，Task 10/11 的测试复用）

```go
package crisis

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// memSeries 实现 SeriesReader（观测按升序存放）。
type memSeries map[string][]Observation

func (m memSeries) Window(indicator, end string, n int) ([]Observation, error) {
	all := m.upTo(indicator, end)
	if len(all) > n {
		all = all[len(all)-n:]
	}
	return all, nil
}

func (m memSeries) WindowSince(indicator, from, end string) ([]Observation, error) {
	var out []Observation
	for _, o := range m[indicator] {
		if o.Date >= from && o.Date <= end {
			out = append(out, o)
		}
	}
	return out, nil
}

func (m memSeries) upTo(indicator, end string) []Observation {
	var out []Observation
	for _, o := range m[indicator] {
		if o.Date <= end {
			out = append(out, o)
		}
	}
	return out
}

// seriesEnding 生成 n 个逐日观测、末日为 end：前 n-1 个取 base，最后一个取 last。
func seriesEnding(end string, n int, base, last float64) []Observation {
	out := make([]Observation, n)
	for i := 0; i < n; i++ {
		v := base
		if i == n-1 {
			v = last
		}
		out[i] = Observation{Date: addDays(end, i-n+1), Value: v}
	}
	return out
}

// baselineSeries 按设计附录基线读数（2026-07-12 核实）构造 7 指标序列。
func baselineSeries(end string) memSeries {
	return memSeries{
		IndVIX:      seriesEnding(end, 80, 15, 15.0),
		IndMOVE:     seriesEnding(end, 80, 70, 69.6),
		IndSOFREFFR: seriesEnding(end, 80, -10, -10),
		IndHYOAS:    seriesEnding(end, 80, 270, 267),
		IndT10Y2Y:   seriesEnding(end, 80, 35, 35),
		IndNFCI:     seriesEnding(end, 80, -0.5, -0.52),
		IndUSDJPY:   seriesEnding(end, 80, 150, 161.7), // 52 周内最高 → CROWDED
	}
}

// testConfig 与 configs/crisis-monitor.yaml 数值一致（引擎测试不读文件）。
func testConfig() *Config {
	return &Config{
		Storage:    StorageCfg{Path: "unused"},
		FRED:       FREDCfg{APIKeyEnv: "FRED_API_KEY"},
		Freshness:  FreshnessCfg{DailyMaxLagDays: 4, WeeklyMaxLagDays: 12},
		Percentile: PercentileCfg{WindowYears: 5, Amber: 0.90, Red: 0.97},
		Indicators: IndicatorsCfg{
			VIX:      VIXCfg{Amber: 25, Red: 30, WeeklySpikePct: 0.50, PercentileTrack: true},
			MOVE:     MOVECfg{Amber: 100, Red: 120, PercentileTrack: true},
			SOFREFFR: SOFREFFRCfg{AmberBp: 10, AmberPersistDays: 3, RedBp: 25, RedPersistDays: 5, PercentileTrack: true, SuppressQuarterEnd: true},
			HYOAS:    HYOASCfg{AmberLowBp: 350, AmberHighBp: 500, RedBp: 600, MomentumBp: 100, MomentumWindowObs: 21, PercentileTrack: true},
			T10Y2Y:   T10Y2YCfg{AmberBp: 25, SteepeningBp: 50, SteepeningLookbackObs: 250},
			NFCI:     NFCICfg{GreenBelow: -0.3, RedAbove: 0, PercentileTrack: true},
			USDJPY:   USDJPYCfg{AmberWowPct: -0.02, RedWowPct: -0.03, Crowded52wPct: 0.98},
		},
		StateMachine: StateMachineCfg{WatchAmberCount: 3, CrisisExitDays: 10, WatchExitDays: 20, BrewingExitDays: 10, DemoteHysteresisDays: 3},
	}
}

// 验收锚点：设计附录基线 → AMBER 计数恰为 2（hy_oas 自满 + usdjpy 拥挤）。
func TestEvaluateIndicatorBaseline(t *testing.T) {
	const d = "2026-07-10"
	cfg, sr := testConfig(), baselineSeries(d)

	want := map[string]struct {
		status Status
		tag    Tag
	}{
		IndVIX:      {StatusGreen, ""},
		IndMOVE:     {StatusGreen, ""},
		IndSOFREFFR: {StatusGreen, ""},
		IndHYOAS:    {StatusAmber, TagComplacency},
		IndT10Y2Y:   {StatusGreen, ""},
		IndNFCI:     {StatusGreen, ""},
		IndUSDJPY:   {StatusAmber, TagCrowded},
	}
	for ind, w := range want {
		res, err := EvaluateIndicator(cfg, ind, d, sr)
		require.NoError(t, err, ind)
		assert.Equal(t, w.status, res.RawStatus, ind)
		assert.Equal(t, w.tag, res.Tag, ind)
		assert.Equal(t, 80, res.WindowActualObs, ind)
	}
}

func TestEvaluateIndicatorStatusPaths(t *testing.T) {
	cfg := testConfig()
	const d = "2026-07-10"

	// NO_DATA：序列不存在（2018 前 SOFR、早期 MOVE 依赖此路径）
	res, err := EvaluateIndicator(cfg, IndSOFREFFR, d, memSeries{})
	require.NoError(t, err)
	assert.Equal(t, StatusNoData, res.Status)

	// STALE：最新观测滞后 7 天 > daily_max_lag_days=4
	res, err = EvaluateIndicator(cfg, IndVIX, d,
		memSeries{IndVIX: seriesEnding("2026-07-03", 10, 15, 15)})
	require.NoError(t, err)
	assert.Equal(t, StatusStale, res.Status)

	// VIX 单周涨幅 >50% → 至少 AMBER（绝对值 22 仍 < 25）
	res, err = EvaluateIndicator(cfg, IndVIX, d,
		memSeries{IndVIX: seriesEnding(d, 10, 14, 22)}) // 22/14−1 ≈ +57%
	require.NoError(t, err)
	assert.Equal(t, StatusAmber, res.RawStatus)

	// SOFR−EFFR 持续性：5 观测全 >25bp → RED
	res, err = EvaluateIndicator(cfg, IndSOFREFFR, d,
		memSeries{IndSOFREFFR: seriesEnding(d, 5, 30, 30)})
	require.NoError(t, err)
	assert.Equal(t, StatusRed, res.RawStatus)
	// 仅最近 3 观测 >10bp → AMBER
	amber := append(seriesEnding(addDays(d, -3), 2, 5, 5), seriesEnding(d, 3, 15, 15)...)
	res, err = EvaluateIndicator(cfg, IndSOFREFFR, d, memSeries{IndSOFREFFR: amber})
	require.NoError(t, err)
	assert.Equal(t, StatusAmber, res.RawStatus)

	// HY OAS 月走阔 110bp > 100bp → AMBER+STRESS（水平 470 在绿区）
	res, err = EvaluateIndicator(cfg, IndHYOAS, d,
		memSeries{IndHYOAS: seriesEnding(d, 25, 360, 470)})
	require.NoError(t, err)
	assert.Equal(t, StatusAmber, res.RawStatus)
	assert.Equal(t, TagStress, res.Tag)

	// 10Y−2Y 倒挂 → RED
	res, err = EvaluateIndicator(cfg, IndT10Y2Y, d,
		memSeries{IndT10Y2Y: seriesEnding(d, 10, 10, -5)})
	require.NoError(t, err)
	assert.Equal(t, StatusRed, res.RawStatus)
	// 倒挂后复陡 60bp > 50bp → GREEN 但带 STEEPENING 标记（设计 §3.1 说明二）
	steep := append(seriesEnding(addDays(d, -5), 20, -30, -30), seriesEnding(d, 5, 20, 30)...)
	res, err = EvaluateIndicator(cfg, IndT10Y2Y, d, memSeries{IndT10Y2Y: steep})
	require.NoError(t, err)
	assert.Equal(t, StatusGreen, res.RawStatus)
	assert.Equal(t, TagSteepening, res.Tag)

	// NFCI：>0 红、−0.3~0 黄
	res, err = EvaluateIndicator(cfg, IndNFCI, d,
		memSeries{IndNFCI: seriesEnding(d, 10, -0.5, 0.1)})
	require.NoError(t, err)
	assert.Equal(t, StatusRed, res.RawStatus)
	res, err = EvaluateIndicator(cfg, IndNFCI, d,
		memSeries{IndNFCI: seriesEnding(d, 10, -0.5, -0.1)})
	require.NoError(t, err)
	assert.Equal(t, StatusAmber, res.RawStatus)

	// USD/JPY 周跌 3.5% ≤ −3%（日元急升值）→ RED；6 观测 <60 → 不做 CROWDED
	res, err = EvaluateIndicator(cfg, IndUSDJPY, d,
		memSeries{IndUSDJPY: seriesEnding(d, 6, 100, 96.5)})
	require.NoError(t, err)
	assert.Equal(t, StatusRed, res.RawStatus)
	assert.Equal(t, Tag(""), res.Tag)

	// 分位轨：绝对值全绿但当前值处 80 观测的 0.9875 分位 ≥0.97 → RED（双轨任一触发）
	ramp := make([]Observation, 80)
	for i := range ramp {
		ramp[i] = Observation{Date: addDays(d, i-79), Value: 10 + float64(i)*0.1}
	}
	res, err = EvaluateIndicator(cfg, IndVIX, d, memSeries{IndVIX: ramp})
	require.NoError(t, err)
	assert.Equal(t, StatusRed, res.RawStatus)
}
```

- [ ] **Step 2: 运行确认失败**

Run: `GOTOOLCHAIN=local go test ./internal/crisis/ -run TestEvaluateIndicator -v`
Expected: 编译失败，`undefined: EvaluateIndicator`

- [ ] **Step 3: 实现 rules.go**

```go
package crisis

// minPercentileObs is the minimum window size for the percentile track and
// the CROWDED check (plan deviation 3: short windows are annotated in
// WindowActualObs, but a rank over a handful of points is meaningless).
const minPercentileObs = 60

// EvaluateIndicator runs the shared pipeline for one indicator (design §3.1):
// data presence → freshness → absolute-threshold track → percentile track,
// either track escalating (maxStatus). NO_DATA and STALE short-circuit and
// leave resonance counting; seasonal suppression and hysteresis are applied
// later by EvalDay.
func EvaluateIndicator(cfg *Config, indicator, date string, sr SeriesReader) (IndicatorResult, error) {
	res := IndicatorResult{Indicator: indicator, RawStatus: StatusGreen, Pct5y: -1}

	latest, err := sr.Window(indicator, date, 1)
	if err != nil {
		return res, err
	}
	if len(latest) == 0 {
		res.Status, res.RawStatus = StatusNoData, StatusNoData
		return res, nil
	}
	res.Value = latest[0].Value
	if staleFor(cfg, indicator, date, latest[0].Date) {
		res.Status, res.RawStatus = StatusStale, StatusStale
		return res, nil
	}

	pctWin, err := sr.WindowSince(indicator, addYears(date, -cfg.Percentile.WindowYears), date)
	if err != nil {
		return res, err
	}
	res.Pct5y, res.WindowActualObs = Percentile(pctWin, res.Value)

	if err := evalAbsolute(cfg, indicator, date, sr, &res); err != nil {
		return res, err
	}

	if percentileTrack(cfg, indicator) && res.WindowActualObs >= minPercentileObs {
		switch {
		case res.Pct5y >= cfg.Percentile.Red:
			res.RawStatus = maxStatus(res.RawStatus, StatusRed)
		case res.Pct5y >= cfg.Percentile.Amber:
			res.RawStatus = maxStatus(res.RawStatus, StatusAmber)
		}
	}

	res.Status = res.RawStatus
	return res, nil
}

func percentileTrack(cfg *Config, indicator string) bool {
	switch indicator {
	case IndVIX:
		return cfg.Indicators.VIX.PercentileTrack
	case IndMOVE:
		return cfg.Indicators.MOVE.PercentileTrack
	case IndSOFREFFR:
		return cfg.Indicators.SOFREFFR.PercentileTrack
	case IndHYOAS:
		return cfg.Indicators.HYOAS.PercentileTrack
	case IndT10Y2Y:
		return cfg.Indicators.T10Y2Y.PercentileTrack
	case IndNFCI:
		return cfg.Indicators.NFCI.PercentileTrack
	case IndUSDJPY:
		return cfg.Indicators.USDJPY.PercentileTrack
	}
	return false
}

func evalAbsolute(cfg *Config, indicator, date string, sr SeriesReader, res *IndicatorResult) error {
	switch indicator {
	case IndVIX:
		return evalVIX(cfg, date, sr, res)
	case IndMOVE:
		evalMOVE(cfg, res)
	case IndSOFREFFR:
		return evalSOFREFFR(cfg, date, sr, res)
	case IndHYOAS:
		return evalHYOAS(cfg, date, sr, res)
	case IndT10Y2Y:
		return evalT10Y2Y(cfg, date, sr, res)
	case IndNFCI:
		evalNFCI(cfg, res)
	case IndUSDJPY:
		return evalUSDJPY(cfg, date, sr, res)
	}
	return nil
}

func evalVIX(cfg *Config, date string, sr SeriesReader, res *IndicatorResult) error {
	c := cfg.Indicators.VIX
	switch {
	case res.Value > c.Red:
		res.RawStatus = StatusRed
	case res.Value >= c.Amber:
		res.RawStatus = StatusAmber
	}
	win, err := sr.Window(IndVIX, date, 6)
	if err != nil {
		return err
	}
	if wow, ok := WowPct(win); ok && wow > c.WeeklySpikePct {
		res.RawStatus = maxStatus(res.RawStatus, StatusAmber)
	}
	return nil
}

func evalMOVE(cfg *Config, res *IndicatorResult) {
	c := cfg.Indicators.MOVE
	switch {
	case res.Value > c.Red:
		res.RawStatus = StatusRed
	case res.Value >= c.Amber:
		res.RawStatus = StatusAmber
	}
}

// evalSOFREFFR: persistence conditions are the core noise filter (design
// §3.1 note 3) — red needs red_persist_days consecutive observations above
// red_bp, amber likewise over its own window.
func evalSOFREFFR(cfg *Config, date string, sr SeriesReader, res *IndicatorResult) error {
	c := cfg.Indicators.SOFREFFR
	win, err := sr.Window(IndSOFREFFR, date, c.RedPersistDays)
	if err != nil {
		return err
	}
	if len(win) >= c.RedPersistDays && allAbove(win, c.RedBp) {
		res.RawStatus = StatusRed
		return nil
	}
	if len(win) >= c.AmberPersistDays && allAbove(lastN(win, c.AmberPersistDays), c.AmberBp) {
		res.RawStatus = StatusAmber
	}
	return nil
}

// evalHYOAS: two-sided amber (design §3.1 note 1) — too tight is complacency,
// moderately wide is stress; the momentum condition catches fast widening
// before the level does.
func evalHYOAS(cfg *Config, date string, sr SeriesReader, res *IndicatorResult) error {
	c := cfg.Indicators.HYOAS
	switch {
	case res.Value > c.RedBp:
		res.RawStatus = StatusRed
	case res.Value < c.AmberLowBp:
		res.RawStatus, res.Tag = StatusAmber, TagComplacency
	case res.Value >= c.AmberHighBp:
		res.RawStatus, res.Tag = StatusAmber, TagStress
	}
	win, err := sr.Window(IndHYOAS, date, c.MomentumWindowObs+1)
	if err != nil {
		return err
	}
	if mom, ok := MomChange(win, c.MomentumWindowObs); ok && mom > c.MomentumBp {
		res.RawStatus = maxStatus(res.RawStatus, StatusAmber)
		if res.Tag == "" {
			res.Tag = TagStress
		}
	}
	return nil
}

// evalT10Y2Y: inversion is red, but STEEPENING marks the historically most
// dangerous window — fast re-steepening after an inversion (design §3.1
// note 2). The tag never changes the color by itself.
func evalT10Y2Y(cfg *Config, date string, sr SeriesReader, res *IndicatorResult) error {
	c := cfg.Indicators.T10Y2Y
	switch {
	case res.Value < 0:
		res.RawStatus = StatusRed
	case res.Value <= c.AmberBp:
		res.RawStatus = StatusAmber
	}
	win, err := sr.Window(IndT10Y2Y, date, c.SteepeningLookbackObs)
	if err != nil {
		return err
	}
	lowest := res.Value
	for _, o := range win {
		if o.Value < lowest {
			lowest = o.Value
		}
	}
	if lowest < 0 && res.Value-lowest > c.SteepeningBp {
		res.Tag = TagSteepening
	}
	return nil
}

func evalNFCI(cfg *Config, res *IndicatorResult) {
	c := cfg.Indicators.NFCI
	switch {
	case res.Value > c.RedAbove:
		res.RawStatus = StatusRed
	case res.Value >= c.GreenBelow:
		res.RawStatus = StatusAmber
	}
}

// evalUSDJPY: JPY 急升值 = USDJPY 下跌，故 wow 阈值为负、≤ 比较（carry trade
// 急平仓方向）；CROWDED = USDJPY 处 52 周高分位（日元极端弱势 = 空头拥挤）。
func evalUSDJPY(cfg *Config, date string, sr SeriesReader, res *IndicatorResult) error {
	c := cfg.Indicators.USDJPY
	win, err := sr.Window(IndUSDJPY, date, 6)
	if err != nil {
		return err
	}
	if wow, ok := WowPct(win); ok {
		switch {
		case wow <= c.RedWowPct:
			res.RawStatus = StatusRed
		case wow <= c.AmberWowPct:
			res.RawStatus = maxStatus(res.RawStatus, StatusAmber)
		}
	}
	yr, err := sr.WindowSince(IndUSDJPY, addYears(date, -1), date)
	if err != nil {
		return err
	}
	if p, n := Percentile(yr, res.Value); n >= minPercentileObs && p >= c.Crowded52wPct {
		res.RawStatus = maxStatus(res.RawStatus, StatusAmber)
		res.Tag = TagCrowded
	}
	return nil
}

func allAbove(obs []Observation, threshold float64) bool {
	for _, o := range obs {
		if o.Value <= threshold {
			return false
		}
	}
	return true
}

func lastN(obs []Observation, n int) []Observation {
	if len(obs) <= n {
		return obs
	}
	return obs[len(obs)-n:]
}
```

- [ ] **Step 4: 运行确认通过（含全包回归）**

Run: `GOTOOLCHAIN=local go test ./internal/crisis/ -v`
Expected: PASS（基线锚点 + 各状态路径全绿，此前任务的测试无回归）

- [ ] **Step 5: 提交**

```bash
git add internal/crisis/rules.go internal/crisis/rules_test.go
git commit -m "feat(crisis): add per-indicator three-color rules engine"
```

### Task 10: 系统状态机 `statemachine.go` + `memhistory.go`

**Files:**
- Create: `internal/crisis/statemachine.go`、`internal/crisis/memhistory.go`
- Test: `internal/crisis/statemachine_test.go`

**Interfaces:**
- Consumes: Task 1 types、Task 4 `Config.StateMachine`、EvalHistory
- Produces: `NextState / SysDetail / MemHistory`（Task 11 EvalDay、Task 13 replay 消费）

转移规则 = 设计 §3.3 原文 + 本文"状态机语义补充"：
- 任意态：情绪层双红（vix RED ∧ move RED，coloredStatus 计）→ CRISIS（最高优先级）。
- NORMAL → WATCH：领先层（t10y2y/nfci）任一 RED，或 AMBER 及以上计数 ≥ `watch_amber_count`。
- WATCH → BREWING：hy_oas RED ∧ sofr_effr RED。
- CRISIS → WATCH：vix、move 均 GREEN 持续 `crisis_exit_days`（今日 + 历史指标行）。
- BREWING → WATCH：pair 非双红持续 `brewing_exit_days`（今日 + 历史系统行 detail.brewing_pair）。
- WATCH → NORMAL：`any_trigger` 全解除持续 `watch_exit_days`（今日 + 历史系统行 detail.any_trigger）。
- 历史行数不足所需天数 → 不退出（冷启动安全）。

- [ ] **Step 1: 写失败测试** — 创建 `internal/crisis/statemachine_test.go`

```go
package crisis

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// colorResults 构造 7 指标结果：未指定的默认 GREEN。
func colorResults(m map[string]Status) map[string]IndicatorResult {
	out := map[string]IndicatorResult{}
	for _, ind := range AllIndicators {
		s, ok := m[ind]
		if !ok {
			s = StatusGreen
		}
		out[ind] = IndicatorResult{Indicator: ind, Status: s, RawStatus: s}
	}
	return out
}

// histWithSystem 预填 days 条 detail 相同的系统评估行。
func histWithSystem(days int, det SysDetail) *MemHistory {
	h := NewMemHistory()
	b, _ := json.Marshal(det)
	for i := 0; i < days; i++ {
		h.Append([]Evaluation{{Indicator: "", SystemState: det.Prev, Detail: string(b)}})
	}
	return h
}

func TestNextStateTransitions(t *testing.T) {
	cfg := testConfig()
	cases := []struct {
		name string
		prev SystemState
		res  map[string]Status
		want SystemState
	}{
		{"normal stays normal", StateNormal, nil, StateNormal},
		{"leading red → WATCH", StateNormal, map[string]Status{IndNFCI: StatusRed}, StateWatch},
		{"amber-or-worse ≥3 → WATCH", StateNormal,
			map[string]Status{IndVIX: StatusAmber, IndHYOAS: StatusAmber, IndUSDJPY: StatusAmber}, StateWatch},
		{"NO_DATA 退出共振（计数只剩 2）", StateNormal,
			map[string]Status{IndVIX: StatusAmber, IndHYOAS: StatusAmber, IndUSDJPY: StatusNoData}, StateNormal},
		{"SUPPRESSED 退出共振", StateNormal,
			map[string]Status{IndVIX: StatusAmber, IndHYOAS: StatusAmber, IndSOFREFFR: StatusSuppressed}, StateNormal},
		{"watch + credit∧liquidity 双红 → BREWING", StateWatch,
			map[string]Status{IndHYOAS: StatusRed, IndSOFREFFR: StatusRed}, StateBrewing},
		{"normal + pair 不直接 BREWING（设计 §3.3 原文只从 WATCH 转入）", StateNormal,
			map[string]Status{IndHYOAS: StatusRed, IndSOFREFFR: StatusRed}, StateNormal},
		{"情绪双红从 NORMAL → CRISIS", StateNormal,
			map[string]Status{IndVIX: StatusRed, IndMOVE: StatusRed}, StateCrisis},
		{"情绪双红从 BREWING → CRISIS", StateBrewing,
			map[string]Status{IndVIX: StatusRed, IndMOVE: StatusRed}, StateCrisis},
		{"MOVE STALE 时单 VIX 红不触发 CRISIS", StateWatch,
			map[string]Status{IndVIX: StatusRed, IndMOVE: StatusStale}, StateWatch},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			next, det, err := NextState(cfg, tc.prev, colorResults(tc.res), NewMemHistory())
			require.NoError(t, err)
			assert.Equal(t, tc.want, next)
			assert.Equal(t, tc.prev, det.Prev)
		})
	}
}

func TestNextStateExits(t *testing.T) {
	cfg := testConfig()
	greens := colorResults(nil)

	// CRISIS 退出：今日双绿 + 9 日历史双绿 = 持续 10 日 → WATCH
	h := NewMemHistory()
	for i := 0; i < 9; i++ {
		h.Append([]Evaluation{
			{Indicator: IndVIX, Status: StatusGreen},
			{Indicator: IndMOVE, Status: StatusGreen},
		})
	}
	next, _, err := NextState(cfg, StateCrisis, greens, h)
	require.NoError(t, err)
	assert.Equal(t, StateWatch, next)
	// 历史不足 → 维持 CRISIS（冷启动安全）
	next, _, err = NextState(cfg, StateCrisis, greens, NewMemHistory())
	require.NoError(t, err)
	assert.Equal(t, StateCrisis, next)
	// 历史中夹一日 AMBER → 维持
	h.Append([]Evaluation{{Indicator: IndVIX, Status: StatusAmber}, {Indicator: IndMOVE, Status: StatusGreen}})
	next, _, err = NextState(cfg, StateCrisis, greens, h)
	require.NoError(t, err)
	assert.Equal(t, StateCrisis, next)

	// WATCH 退出：今日无触发 + 19 日 any_trigger=false = 持续 20 日 → NORMAL
	next, _, err = NextState(cfg, StateWatch, greens, histWithSystem(19, SysDetail{AnyTrigger: false, Prev: StateWatch}))
	require.NoError(t, err)
	assert.Equal(t, StateNormal, next)
	next, _, err = NextState(cfg, StateWatch, greens, histWithSystem(5, SysDetail{AnyTrigger: false, Prev: StateWatch}))
	require.NoError(t, err)
	assert.Equal(t, StateWatch, next)

	// BREWING 退出：今日非双红 + 9 日 brewing_pair=false = 持续 10 日 → WATCH
	next, _, err = NextState(cfg, StateBrewing, greens, histWithSystem(9, SysDetail{BrewingPair: false, Prev: StateBrewing}))
	require.NoError(t, err)
	assert.Equal(t, StateWatch, next)
	next, _, err = NextState(cfg, StateBrewing, greens, histWithSystem(3, SysDetail{BrewingPair: false, Prev: StateBrewing}))
	require.NoError(t, err)
	assert.Equal(t, StateBrewing, next)
}

func TestMemHistoryOrder(t *testing.T) {
	h := NewMemHistory()
	h.Append([]Evaluation{{Indicator: "", TS: "2026-07-01"}, {Indicator: IndVIX, TS: "2026-07-01"}})
	h.Append([]Evaluation{{Indicator: "", TS: "2026-07-02"}, {Indicator: IndVIX, TS: "2026-07-02"}})

	sys, err := h.RecentSystem(5)
	require.NoError(t, err)
	require.Len(t, sys, 2)
	assert.Equal(t, "2026-07-02", sys[0].TS) // 新→旧

	ind, err := h.RecentIndicator(IndVIX, 1)
	require.NoError(t, err)
	require.Len(t, ind, 1)
	assert.Equal(t, "2026-07-02", ind[0].TS)
}
```

- [ ] **Step 2: 运行确认失败**

Run: `GOTOOLCHAIN=local go test ./internal/crisis/ -run 'TestNextState|TestMemHistory' -v`
Expected: 编译失败，`undefined: NextState`

- [ ] **Step 3: 实现 statemachine.go 与 memhistory.go**

`internal/crisis/statemachine.go`：

```go
package crisis

import "encoding/json"

// SysDetail is the JSON payload of system-level evaluation rows. The
// consecutive-day exit counters are rebuilt from these historical flags, so
// the process holds no in-memory counters (design §4.3: stateless process,
// sqlite as the single source of truth).
type SysDetail struct {
	Date        string      `json:"date"`
	AnyTrigger  bool        `json:"any_trigger"`
	BrewingPair bool        `json:"brewing_pair"`
	AmberCount  int         `json:"amber_count"`
	Prev        SystemState `json:"prev"`
}

// coloredStatus returns the resonance-eligible status of ind; non-color
// statuses (NO_DATA / STALE / SUPPRESSED_SEASONAL) leave the count entirely
// (design §3.2 rules 2 and 5 share this exit).
func coloredStatus(res map[string]IndicatorResult, ind string) Status {
	if s := res[ind].Status; isColor(s) {
		return s
	}
	return ""
}

// amberOrWorseCount counts indicators at AMBER or above — "全系统 AMBER ≥ 3"
// read as at-least-amber so reds are not perversely excluded (plan's
// state-machine semantics note).
func amberOrWorseCount(res map[string]IndicatorResult) int {
	n := 0
	for _, ind := range AllIndicators {
		if severity(coloredStatus(res, ind)) >= severity(StatusAmber) {
			n++
		}
	}
	return n
}

// NextState implements design §3.3. Precedence: the sentiment double-red
// CRISIS trigger fires from any state; all exits require the full
// consecutive-day streak and default to staying put when history is short.
func NextState(cfg *Config, prev SystemState, res map[string]IndicatorResult, hist EvalHistory) (SystemState, SysDetail, error) {
	sm := cfg.StateMachine
	sentimentDoubleRed := coloredStatus(res, IndVIX) == StatusRed && coloredStatus(res, IndMOVE) == StatusRed
	leadingRed := coloredStatus(res, IndT10Y2Y) == StatusRed || coloredStatus(res, IndNFCI) == StatusRed
	brewingPair := coloredStatus(res, IndHYOAS) == StatusRed && coloredStatus(res, IndSOFREFFR) == StatusRed
	amberCount := amberOrWorseCount(res)

	det := SysDetail{
		AnyTrigger:  leadingRed || amberCount >= sm.WatchAmberCount || brewingPair || sentimentDoubleRed,
		BrewingPair: brewingPair,
		AmberCount:  amberCount,
		Prev:        prev,
	}

	if sentimentDoubleRed {
		return StateCrisis, det, nil
	}

	switch prev {
	case StateCrisis:
		ok, err := sentimentGreenStreak(res, hist, sm.CrisisExitDays)
		if err != nil || !ok {
			return StateCrisis, det, err
		}
		return StateWatch, det, nil

	case StateBrewing:
		if brewingPair {
			return StateBrewing, det, nil
		}
		ok, err := systemDetailStreak(hist, sm.BrewingExitDays, func(d SysDetail) bool { return !d.BrewingPair })
		if err != nil || !ok {
			return StateBrewing, det, err
		}
		return StateWatch, det, nil

	case StateWatch:
		if brewingPair {
			return StateBrewing, det, nil
		}
		if det.AnyTrigger {
			return StateWatch, det, nil
		}
		ok, err := systemDetailStreak(hist, sm.WatchExitDays, func(d SysDetail) bool { return !d.AnyTrigger })
		if err != nil || !ok {
			return StateWatch, det, err
		}
		return StateNormal, det, nil

	default: // NORMAL（含冷启动，设计 §4.3）
		if leadingRed || amberCount >= sm.WatchAmberCount {
			return StateWatch, det, nil
		}
		return StateNormal, det, nil
	}
}

// sentimentGreenStreak: 今日 vix/move 双绿，且此前 days-1 个评估日的指标行均为
// GREEN（历史不足 = 不允许退出）。
func sentimentGreenStreak(res map[string]IndicatorResult, hist EvalHistory, days int) (bool, error) {
	if coloredStatus(res, IndVIX) != StatusGreen || coloredStatus(res, IndMOVE) != StatusGreen {
		return false, nil
	}
	for _, ind := range []string{IndVIX, IndMOVE} {
		prev, err := hist.RecentIndicator(ind, days-1)
		if err != nil {
			return false, err
		}
		if len(prev) < days-1 {
			return false, nil
		}
		for _, e := range prev {
			if e.Status != StatusGreen {
				return false, nil
			}
		}
	}
	return true, nil
}

// systemDetailStreak: 此前 days-1 个系统评估行的 detail 均满足 pred（今日条件
// 由调用方先判）。
func systemDetailStreak(hist EvalHistory, days int, pred func(SysDetail) bool) (bool, error) {
	prev, err := hist.RecentSystem(days - 1)
	if err != nil {
		return false, err
	}
	if len(prev) < days-1 {
		return false, nil
	}
	for _, e := range prev {
		var d SysDetail
		if err := json.Unmarshal([]byte(e.Detail), &d); err != nil {
			return false, err
		}
		if !pred(d) {
			return false, nil
		}
	}
	return true, nil
}
```

`internal/crisis/memhistory.go`：

```go
package crisis

// MemHistory is the in-memory EvalHistory used by replay and tests; live
// evaluation uses Store.History instead. Entries are kept newest-first.
type MemHistory struct {
	sys []Evaluation
	ind map[string][]Evaluation
}

func NewMemHistory() *MemHistory {
	return &MemHistory{ind: map[string][]Evaluation{}}
}

// Append prepends one evaluation day's rows.
func (m *MemHistory) Append(evals []Evaluation) {
	for _, e := range evals {
		if e.Indicator == "" {
			m.sys = append([]Evaluation{e}, m.sys...)
		} else {
			m.ind[e.Indicator] = append([]Evaluation{e}, m.ind[e.Indicator]...)
		}
	}
}

func (m *MemHistory) RecentSystem(n int) ([]Evaluation, error) {
	return headN(m.sys, n), nil
}

func (m *MemHistory) RecentIndicator(indicator string, n int) ([]Evaluation, error) {
	return headN(m.ind[indicator], n), nil
}

func headN(evals []Evaluation, n int) []Evaluation {
	if len(evals) > n {
		return evals[:n]
	}
	return evals
}
```

- [ ] **Step 4: 运行确认通过**

Run: `GOTOOLCHAIN=local go test ./internal/crisis/ -run 'TestNextState|TestMemHistory' -v`
Expected: PASS（转移表 + 退出计数 + MemHistory 序全绿）

- [ ] **Step 5: 提交**

```bash
git add internal/crisis/statemachine.go internal/crisis/memhistory.go internal/crisis/statemachine_test.go
git commit -m "feat(crisis): add system state machine and in-memory history"
```

### Task 11: 单日评估编排 `eval.go`

**Files:**
- Create: `internal/crisis/eval.go`
- Test: `internal/crisis/eval_test.go`

**Interfaces:**
- Consumes: Task 9 EvaluateIndicator、Task 8 InQuarterEndWindow/ApplyHysteresis、Task 10 NextState
- Produces: `EvalDay / DayResult`（Task 12 eval CLI、Task 13 replay、Task 14 notify 的唯一输入）

编排顺序：7 指标逐个 `EvaluateIndicator` → sofr_effr 且 raw≥AMBER 且季末窗 → `SUPPRESSED_SEASONAL`；否则色彩状态过 `ApplyHysteresis` → `NextState`（prev 取 `hist.RecentSystem(1)`，空历史 = NORMAL 冷启动）→ 组装 8 行 `Evaluation`（指标行 detail 存 `{"raw":..,"window_actual_obs":..}`，系统行 detail 存 SysDetail JSON）。

- [ ] **Step 1: 写失败测试** — 创建 `internal/crisis/eval_test.go`（fixture 复用 rules_test.go 的 `baselineSeries`/`testConfig`/`seriesEnding`）

```go
package crisis

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var evalAt = time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC)

func TestEvalDayBaselineStaysNormal(t *testing.T) {
	const d = "2026-07-10"
	res, err := EvalDay(testConfig(), d, baselineSeries(d), NewMemHistory(), evalAt)
	require.NoError(t, err)

	assert.Equal(t, StateNormal, res.PrevState) // 空历史 = 冷启动 NORMAL
	assert.Equal(t, StateNormal, res.State)
	assert.False(t, res.Transitioned())
	assert.Equal(t, 2, res.Detail.AmberCount) // hy_oas + usdjpy（附录基线）

	require.Len(t, res.Evaluations, 8) // 7 指标行 + 1 系统行
	sys := res.Evaluations[7]
	assert.Equal(t, "", sys.Indicator)
	assert.Equal(t, StateNormal, sys.SystemState)
	assert.Equal(t, d, sys.TS)
	assert.Contains(t, sys.Detail, `"amber_count":2`)
	vix := res.Evaluations[0]
	assert.Equal(t, IndVIX, vix.Indicator)
	assert.Contains(t, vix.Detail, `"raw":"GREEN"`)
}

func TestEvalDayLeadingRedTransitionsToWatch(t *testing.T) {
	const d = "2026-07-10"
	sr := baselineSeries(d)
	sr[IndNFCI] = seriesEnding(d, 80, 0.1, 0.2) // NFCI > 0 → 领先层红

	res, err := EvalDay(testConfig(), d, sr, NewMemHistory(), evalAt)
	require.NoError(t, err)
	assert.Equal(t, StateWatch, res.State)
	assert.True(t, res.Transitioned())
	assert.True(t, res.Detail.AnyTrigger)
}

func TestEvalDayQuarterEndSuppression(t *testing.T) {
	const d = "2026-03-31" // 季末窗口内（周二，已核实）
	sr := baselineSeries(d)
	sr[IndSOFREFFR] = seriesEnding(d, 80, 15, 15) // 持续 >10bp → raw AMBER

	res, err := EvalDay(testConfig(), d, sr, NewMemHistory(), evalAt)
	require.NoError(t, err)
	r := res.Results[IndSOFREFFR]
	assert.Equal(t, StatusSuppressed, r.Status) // 生效状态被季末抑制
	assert.Equal(t, StatusAmber, r.RawStatus)   // raw 保留审计
}

func TestEvalDayNoDataLeavesResonance(t *testing.T) {
	const d = "2026-07-10"
	sr := baselineSeries(d)
	delete(sr, IndSOFREFFR) // 模拟 2018 前 SOFR 序列不存在（回测早期段）

	res, err := EvalDay(testConfig(), d, sr, NewMemHistory(), evalAt)
	require.NoError(t, err)
	assert.Equal(t, StatusNoData, res.Results[IndSOFREFFR].Status)
	assert.Equal(t, StateNormal, res.State)
	assert.Equal(t, 2, res.Detail.AmberCount) // NO_DATA 不入计数
}

func TestEvalDayHysteresisHoldsDemotion(t *testing.T) {
	const d = "2026-07-10"
	sr := baselineSeries(d) // 今日 raw 全绿基线（move 绿）
	hist := NewMemHistory()
	hist.Append([]Evaluation{{Indicator: IndMOVE, Status: StatusAmber, Detail: `{"raw":"AMBER"}`}})

	res, err := EvalDay(testConfig(), d, sr, hist, evalAt)
	require.NoError(t, err)
	r := res.Results[IndMOVE]
	assert.Equal(t, StatusGreen, r.RawStatus)
	assert.Equal(t, StatusAmber, r.Status) // 降级被防抖挡住（昨日 raw AMBER）
}
```

- [ ] **Step 2: 运行确认失败**

Run: `GOTOOLCHAIN=local go test ./internal/crisis/ -run TestEvalDay -v`
Expected: 编译失败，`undefined: EvalDay`

- [ ] **Step 3: 实现 eval.go**

```go
package crisis

import (
	"encoding/json"
	"time"
)

// DayResult is one day's full evaluation: per-indicator results, the state
// transition and the ready-to-persist audit rows (7 indicator rows + 1
// system row, in AllIndicators order then system last).
type DayResult struct {
	Date        string
	Results     map[string]IndicatorResult
	PrevState   SystemState
	State       SystemState
	Detail      SysDetail
	Evaluations []Evaluation
}

func (r *DayResult) Transitioned() bool { return r.State != r.PrevState }

// EvalDay runs the full pipeline for one observation date: rules → seasonal
// suppression → hysteresis → state machine → audit rows. It is pure over
// (SeriesReader, EvalHistory), so live eval and replay share it.
func EvalDay(cfg *Config, date string, sr SeriesReader, hist EvalHistory, evalAt time.Time) (*DayResult, error) {
	results := make(map[string]IndicatorResult, len(AllIndicators))
	for _, ind := range AllIndicators {
		r, err := EvaluateIndicator(cfg, ind, date, sr)
		if err != nil {
			return nil, err
		}
		switch {
		case ind == IndSOFREFFR && cfg.Indicators.SOFREFFR.SuppressQuarterEnd &&
			severity(r.RawStatus) >= severity(StatusAmber) && InQuarterEndWindow(date):
			r.Status = StatusSuppressed // 设计 §3.2 条 1：仅记录不告警、退出共振
		case isColor(r.RawStatus):
			prev, err := hist.RecentIndicator(ind, cfg.StateMachine.DemoteHysteresisDays-1)
			if err != nil {
				return nil, err
			}
			r.Status = ApplyHysteresis(r.RawStatus, prev, cfg.StateMachine.DemoteHysteresisDays)
		}
		results[ind] = r
	}

	prevState := StateNormal // 冷启动基线（设计 §4.3：backfill 后直接初始化 NORMAL）
	if sys, err := hist.RecentSystem(1); err != nil {
		return nil, err
	} else if len(sys) > 0 && sys[0].SystemState != "" {
		prevState = sys[0].SystemState
	}

	next, det, err := NextState(cfg, prevState, results, hist)
	if err != nil {
		return nil, err
	}
	det.Date = date

	res := &DayResult{Date: date, Results: results, PrevState: prevState, State: next, Detail: det}
	if res.Evaluations, err = buildEvaluations(res, evalAt); err != nil {
		return nil, err
	}
	return res, nil
}

func buildEvaluations(r *DayResult, evalAt time.Time) ([]Evaluation, error) {
	stamp := NowStamp(evalAt)
	out := make([]Evaluation, 0, len(AllIndicators)+1)
	for _, ind := range AllIndicators {
		ir := r.Results[ind]
		d, err := json.Marshal(indDetail{Raw: ir.RawStatus, WindowActualObs: ir.WindowActualObs})
		if err != nil {
			return nil, err
		}
		out = append(out, Evaluation{
			TS: r.Date, EvalAt: stamp, Indicator: ind,
			Status: ir.Status, Tag: ir.Tag, Value: ir.Value, Pct5y: ir.Pct5y,
			Detail: string(d),
		})
	}
	d, err := json.Marshal(r.Detail)
	if err != nil {
		return nil, err
	}
	out = append(out, Evaluation{
		TS: r.Date, EvalAt: stamp, Indicator: "",
		SystemState: r.State, Detail: string(d),
	})
	return out, nil
}
```

- [ ] **Step 4: 运行确认通过（含全包回归）**

Run: `GOTOOLCHAIN=local go test ./internal/crisis/ -v`
Expected: PASS（5 个 EvalDay 测试 + 此前全部测试）

- [ ] **Step 5: 提交**

```bash
git add internal/crisis/eval.go internal/crisis/eval_test.go
git commit -m "feat(crisis): add single-day evaluation orchestration"
```

### Task 12: `crisis eval` / `crisis status` 子命令

**Files:**
- Modify: `cmd/atlas/crisis.go`
- Test: `cmd/atlas/crisis_test.go`（追加）

**Interfaces:**
- Consumes: Task 6/7/11 全部；`config.Load`（主配置，Task 14 通知才用到）
- Produces: `atlas crisis eval [--date YYYY-MM-DD] [--mode daily|nfci]`、`atlas crisis status`；`executeCrisisEvalDaily(ctx, deps, dateOverride)`（deps 注入模式沿用 `watchlistDeps`，Task 14/15 扩展）

daily 模式流程（设计 §4.3 daily_fetch_eval 行）：
1. 目标评估日 = `--date` 或 `PrevTradingDay(now)`。
2. **幂等**：`HasSystemEvalForDate` 为真 → 打印 already evaluated，exit 0（多时点唤起的第 2+ 次空跑）。
3. 增量采集 `IngestAll(target−45d, today)`（45d 覆盖 NFCI 周频与假日空洞；upsert 幂等）。
4. **数据齐备性**：required = vix/hy_oas/t10y2y/sofr_effr 四个 FRED 序列在 target 日有观测，缺任一 → 打印 not ready，exit 0 等下次唤起；move/usdjpy 缺失走 STALE/NO_DATA 正常评估。
5. `EvalDay` → `AppendEvaluations` → 打印摘要（通知在 Task 14 接入）。

nfci 模式：仅 `IngestNFCI(now−30d, today)`，不评估（NFCI 更新后参与后续 daily 评估，设计 §3.2 条 4）。
status：最新系统行 + 当日各指标行，打印状态、持续天数（系统行同态连续段计数）、各指标 status/value/5y 分位。

- [ ] **Step 1: 写失败测试** — 在 `cmd/atlas/crisis_test.go` 追加（imports 增加 `io`、`time`）

```go
// crisisTestConfig 与 configs/crisis-monitor.yaml 数值一致（cmd 测试不读文件）。
func crisisTestConfig() *crisis.Config {
	return &crisis.Config{
		Storage:    crisis.StorageCfg{Path: "unused"},
		FRED:       crisis.FREDCfg{APIKeyEnv: "FRED_API_KEY"},
		Freshness:  crisis.FreshnessCfg{DailyMaxLagDays: 4, WeeklyMaxLagDays: 12},
		Percentile: crisis.PercentileCfg{WindowYears: 5, Amber: 0.90, Red: 0.97},
		Indicators: crisis.IndicatorsCfg{
			VIX:      crisis.VIXCfg{Amber: 25, Red: 30, WeeklySpikePct: 0.50, PercentileTrack: true},
			MOVE:     crisis.MOVECfg{Amber: 100, Red: 120, PercentileTrack: true},
			SOFREFFR: crisis.SOFREFFRCfg{AmberBp: 10, AmberPersistDays: 3, RedBp: 25, RedPersistDays: 5, PercentileTrack: true, SuppressQuarterEnd: true},
			HYOAS:    crisis.HYOASCfg{AmberLowBp: 350, AmberHighBp: 500, RedBp: 600, MomentumBp: 100, MomentumWindowObs: 21, PercentileTrack: true},
			T10Y2Y:   crisis.T10Y2YCfg{AmberBp: 25, SteepeningBp: 50, SteepeningLookbackObs: 250},
			NFCI:     crisis.NFCICfg{GreenBelow: -0.3, RedAbove: 0},
			USDJPY:   crisis.USDJPYCfg{AmberWowPct: -0.02, RedWowPct: -0.03, Crowded52wPct: 0.98},
		},
		StateMachine: crisis.StateMachineCfg{WatchAmberCount: 3, CrisisExitDays: 10, WatchExitDays: 20, BrewingExitDays: 10, DemoteHysteresisDays: 3},
	}
}

// seedObservations 写入 7 指标、逐日回推 days 天的全绿观测。
func seedObservations(t *testing.T, st *crisis.Store, target string, days int) {
	t.Helper()
	vals := map[string]float64{
		crisis.IndVIX: 15, crisis.IndMOVE: 70, crisis.IndSOFREFFR: -10, crisis.IndHYOAS: 400,
		crisis.IndT10Y2Y: 35, crisis.IndNFCI: -0.5, crisis.IndUSDJPY: 150,
	}
	tt, err := time.Parse("2006-01-02", target)
	require.NoError(t, err)
	var obs []crisis.Observation
	for i := 0; i < days; i++ {
		d := tt.AddDate(0, 0, -i).Format("2006-01-02")
		for ind, v := range vals {
			obs = append(obs, crisis.Observation{Date: d, Indicator: ind, Value: v,
				Source: "test", FetchedAt: "2026-07-11T00:00:00.000000000Z"})
		}
	}
	require.NoError(t, st.UpsertObservations(context.Background(), obs))
}

func noopIngest(calls *int) func(context.Context, string, string) (*crisis.IngestReport, error) {
	return func(context.Context, string, string) (*crisis.IngestReport, error) {
		*calls++
		return &crisis.IngestReport{Counts: map[string]int{}, YahooErrs: map[string]error{}}, nil
	}
}

func TestExecuteCrisisEvalDaily(t *testing.T) {
	st := newCrisisTestStore(t)
	ctx := context.Background()
	const target = "2026-07-10" // 周五
	seedObservations(t, st, target, 80)

	var calls int
	deps := crisisEvalDeps{
		cfg: crisisTestConfig(), store: st, ingest: noopIngest(&calls),
		// 周六上午唤起 → 目标评估日 = 前一交易日 7/10
		now:    func() time.Time { return time.Date(2026, 7, 11, 8, 0, 0, 0, time.UTC) },
		out:    io.Discard, errOut: io.Discard,
	}

	require.NoError(t, executeCrisisEvalDaily(ctx, deps, ""))
	assert.Equal(t, 1, calls)
	sys, err := st.LatestSystemEval(ctx)
	require.NoError(t, err)
	require.NotNil(t, sys)
	assert.Equal(t, crisis.StateNormal, sys.SystemState)
	assert.Equal(t, target, sys.TS)

	// 幂等：同日第二次唤起既不重复评估也不再采集
	require.NoError(t, executeCrisisEvalDaily(ctx, deps, ""))
	assert.Equal(t, 1, calls)
	evals, err := st.RecentSystemEvals(ctx, 10)
	require.NoError(t, err)
	assert.Len(t, evals, 1)
}

func TestExecuteCrisisEvalDailyDataNotReady(t *testing.T) {
	st := newCrisisTestStore(t)
	ctx := context.Background()
	seedObservations(t, st, "2026-07-09", 80) // 数据只到 7/9，目标日 7/10 未齐

	var calls int
	deps := crisisEvalDeps{
		cfg: crisisTestConfig(), store: st, ingest: noopIngest(&calls),
		now:    func() time.Time { return time.Date(2026, 7, 11, 8, 0, 0, 0, time.UTC) },
		out:    io.Discard, errOut: io.Discard,
	}
	require.NoError(t, executeCrisisEvalDaily(ctx, deps, "")) // 静默 exit 0 等下次唤起
	sys, err := st.LatestSystemEval(ctx)
	require.NoError(t, err)
	assert.Nil(t, sys) // 未落任何评估行
}

func TestExecuteCrisisStatus(t *testing.T) {
	st := newCrisisTestStore(t)
	ctx := context.Background()
	var buf strings.Builder
	require.NoError(t, executeCrisisStatus(ctx, st, &buf))
	assert.Contains(t, buf.String(), "no evaluations yet")

	seedObservations(t, st, "2026-07-10", 80)
	var calls int
	deps := crisisEvalDeps{
		cfg: crisisTestConfig(), store: st, ingest: noopIngest(&calls),
		now: func() time.Time { return time.Date(2026, 7, 11, 8, 0, 0, 0, time.UTC) },
		out: io.Discard, errOut: io.Discard,
	}
	require.NoError(t, executeCrisisEvalDaily(ctx, deps, ""))

	buf.Reset()
	require.NoError(t, executeCrisisStatus(ctx, st, &buf))
	assert.Contains(t, buf.String(), "NORMAL")
	assert.Contains(t, buf.String(), "vix")
}
```

- [ ] **Step 2: 运行确认失败**

Run: `GOTOOLCHAIN=local go test ./cmd/atlas/ -run 'TestExecuteCrisis' -v`
Expected: 编译失败，`undefined: crisisEvalDeps`

- [ ] **Step 3: 在 `cmd/atlas/crisis.go` 追加 eval/status 子命令**

flags 与注册（追加到 Task 7 已有的 `init()`）：

```go
var (
	evalDate string
	evalMode string
)

var crisisEvalCmd = &cobra.Command{
	Use:   "eval",
	Short: "Fetch latest data and run one evaluation (launchd entrypoint)",
	RunE:  runCrisisEval,
}

var crisisStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Print current system state and latest indicator readings",
	RunE:  runCrisisStatus,
}

// 追加进已有 init()：
	crisisEvalCmd.Flags().StringVar(&evalDate, "date", "", "override evaluation date YYYY-MM-DD (default: previous trading day)")
	crisisEvalCmd.Flags().StringVar(&evalMode, "mode", "daily", "daily | nfci")
	crisisCmd.AddCommand(crisisEvalCmd, crisisStatusCmd)
```

实现（同文件追加）：

```go
// crisisEvalDeps 注入依赖使流程可单测（模式同 watchlistDeps）。
type crisisEvalDeps struct {
	cfg    *crisis.Config
	store  *crisis.Store
	ingest func(ctx context.Context, from, to string) (*crisis.IngestReport, error)
	now    func() time.Time
	out    io.Writer
	errOut io.Writer
}

// requiredDaily 是齐备性校验的必要集：FRED 日频序列（设计 §4.3——T+1 未齐则
// 退出等下次唤起）；move/usdjpy 缺失走 STALE/NO_DATA 正常评估，nfci 为周频。
var requiredDaily = []string{crisis.IndVIX, crisis.IndHYOAS, crisis.IndT10Y2Y, crisis.IndSOFREFFR}

func runCrisisEval(cmd *cobra.Command, args []string) error {
	ccfg, st, err := openCrisisStore()
	if err != nil {
		return err
	}
	defer st.Close()

	apiKey := resolveFREDKey(ccfg.FRED.APIKeyEnv)
	if apiKey == "" {
		return fmt.Errorf("FRED key missing: set env %s or collectors.fred.api_key in the main config (-c)", ccfg.FRED.APIKeyEnv)
	}
	ig := crisis.NewIngestor(fred.New(apiKey), yahoo.New(), st)

	switch evalMode {
	case "daily":
		deps := crisisEvalDeps{
			cfg: ccfg, store: st, ingest: ig.IngestAll,
			now: time.Now, out: cmd.OutOrStdout(), errOut: cmd.ErrOrStderr(),
		}
		return executeCrisisEvalDaily(cmd.Context(), deps, evalDate)
	case "nfci":
		now := time.Now().UTC()
		n, err := ig.IngestNFCI(cmd.Context(),
			now.AddDate(0, 0, -30).Format("2006-01-02"), now.Format("2006-01-02"))
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "nfci refreshed: %d rows\n", n)
		return nil
	default:
		return fmt.Errorf("unknown --mode %q", evalMode)
	}
}

func executeCrisisEvalDaily(ctx context.Context, d crisisEvalDeps, dateOverride string) error {
	target := dateOverride
	if target == "" {
		target = crisis.PrevTradingDay(d.now().UTC()).Format("2006-01-02")
	}

	// 幂等：多时点唤起的第 2+ 次直接空跑（设计 §4.3，幂等由库保证）
	done, err := d.store.HasSystemEvalForDate(ctx, target)
	if err != nil {
		return err
	}
	if done {
		fmt.Fprintf(d.out, "already evaluated %s, nothing to do\n", target)
		return nil
	}

	// 增量采集：45 天回看覆盖 NFCI 周频与假日空洞，upsert 幂等
	from := mustAddDays(target, -45)
	rep, err := d.ingest(ctx, from, d.now().UTC().Format("2006-01-02"))
	if err != nil {
		return err
	}
	for ind, ferr := range rep.YahooErrs {
		fmt.Fprintf(d.errOut, "warning: yahoo %s failed: %v\n", ind, ferr)
	}

	// 数据齐备性：required 序列在 target 日必须有观测（T+1 校验）
	for _, ind := range requiredDaily {
		obs, err := d.store.Observation(ctx, ind, target)
		if err != nil {
			return err
		}
		if obs == nil {
			fmt.Fprintf(d.out, "data not ready for %s (%s missing), waiting for next wakeup\n", target, ind)
			return nil
		}
	}

	res, err := crisis.EvalDay(d.cfg, target, d.store.Reader(ctx), d.store.History(ctx), d.now())
	if err != nil {
		return err
	}
	if err := d.store.AppendEvaluations(ctx, res.Evaluations); err != nil {
		return err
	}
	printDayResult(d.out, res)
	return nil
}

func mustAddDays(date string, n int) string {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return date
	}
	return t.AddDate(0, 0, n).Format("2006-01-02")
}

func printDayResult(w io.Writer, res *crisis.DayResult) {
	if res.Transitioned() {
		fmt.Fprintf(w, "%s: %s → %s\n", res.Date, res.PrevState, res.State)
	} else {
		fmt.Fprintf(w, "%s: %s\n", res.Date, res.State)
	}
	for _, ind := range crisis.AllIndicators {
		r := res.Results[ind]
		fmt.Fprintf(w, "  %-10s %-20s %10.2f  p5y=%.2f  %s\n", ind, r.Status, r.Value, r.Pct5y, r.Tag)
	}
}

func runCrisisStatus(cmd *cobra.Command, args []string) error {
	_, st, err := openCrisisStore()
	if err != nil {
		return err
	}
	defer st.Close()
	return executeCrisisStatus(cmd.Context(), st, cmd.OutOrStdout())
}

func executeCrisisStatus(ctx context.Context, st *crisis.Store, out io.Writer) error {
	sys, err := st.LatestSystemEval(ctx)
	if err != nil {
		return err
	}
	if sys == nil {
		fmt.Fprintln(out, "no evaluations yet — run `atlas crisis eval` after backfill")
		return nil
	}
	days, err := stateStreakDays(ctx, st, sys.SystemState)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "system state: %s (as of %s, %d eval days)\n", sys.SystemState, sys.TS, days)
	for _, ind := range crisis.AllIndicators {
		evals, err := st.RecentIndicatorEvals(ctx, ind, 1)
		if err != nil {
			return err
		}
		if len(evals) == 0 {
			continue
		}
		e := evals[0]
		fmt.Fprintf(out, "  %-10s %-20s %10.2f  p5y=%.2f  %s\n", ind, e.Status, e.Value, e.Pct5y, e.Tag)
	}
	return nil
}

// stateStreakDays 统计与当前状态相同的连续系统评估行数 = 状态持续评估日数。
func stateStreakDays(ctx context.Context, st *crisis.Store, state crisis.SystemState) (int, error) {
	evals, err := st.RecentSystemEvals(ctx, 500)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, e := range evals {
		if e.SystemState != state {
			break
		}
		n++
	}
	return n, nil
}
```

- [ ] **Step 4: 运行确认通过（含 cmd 包回归）**

Run: `GOTOOLCHAIN=local go test ./cmd/atlas/ -v`
Expected: PASS（3 个新测试 + 既有测试无回归）

- [ ] **Step 5: 提交**

```bash
git add cmd/atlas/crisis.go cmd/atlas/crisis_test.go
git commit -m "feat(crisis): add eval and status commands"
```

### Task 13: `crisis replay` 回测 + 三段历史验收（第二阶段收口）

**Files:**
- Modify: `cmd/atlas/crisis.go`
- Test: `cmd/atlas/crisis_test.go`（追加）

**Interfaces:**
- Consumes: `Store.EvalDates`、`EvalDay`、`MemHistory`
- Produces: `atlas crisis replay --from YYYY-MM-DD --to YYYY-MM-DD [--json]`

replay 循环：`EvalDates(from,to)`（vix 观测日）逐日 `EvalDay(cfg, d, store.Reader, mem)` → `mem.Append`，**不写库**；输出状态转移时间线与各态进入次数汇总。

**第二阶段人工验收（设计 §6 分段降级标准，逐条执行）：**

- [ ] `bin/atlas crisis replay --from 2019-06-01 --to 2020-06-30`：2020-03 段在情绪层双红（首个 vix>30∧move>120 日）**之前**已处于 WATCH 或 BREWING
- [ ] `bin/atlas crisis replay --from 2024-01-01 --to 2024-12-31`：2024-08 段同上标准
- [ ] `bin/atlas crisis replay --from 2007-01-01 --to 2009-12-31`：VIX/HY OAS/10Y−2Y/NFCI 四指标（sofr_effr、move 按 NO_DATA 退出）在 vix 首红**之前**把系统推入 WATCH；BREWING 不作要求
- [ ] `bin/atlas crisis replay --from 2015-01-01 --to 2019-12-31`：进入 BREWING 次数 ≤ 1
- [ ] 任一条不达标 → 只调 `configs/crisis-monitor.yaml` 阈值重跑（不改代码），并把最终参数与四段结果记入本文件附注

- [ ] **Step 1: 写失败测试** — 在 `cmd/atlas/crisis_test.go` 追加（imports 增加 `bytes`）

```go
func TestExecuteCrisisReplayTransitions(t *testing.T) {
	st := newCrisisTestStore(t)
	ctx := context.Background()

	// 80 日全绿基线，最后 3 日 NFCI 转正（领先层红）→ 应出现一次 NORMAL→WATCH
	seedObservations(t, st, "2026-07-10", 80)
	var red []crisis.Observation
	for _, d := range []string{"2026-07-08", "2026-07-09", "2026-07-10"} {
		red = append(red, crisis.Observation{Date: d, Indicator: crisis.IndNFCI, Value: 0.2,
			Source: "test", FetchedAt: "2026-07-11T00:00:00.000000000Z"})
	}
	require.NoError(t, st.UpsertObservations(ctx, red))

	var buf bytes.Buffer
	require.NoError(t, executeCrisisReplay(ctx, crisisTestConfig(), st, "2026-06-25", "2026-07-10", false, &buf))
	out := buf.String()
	assert.Contains(t, out, "NORMAL → WATCH")
	assert.Contains(t, out, "final state: WATCH")
	assert.Contains(t, out, "entered WATCH")

	// 回放零落库：真相源不被回测污染
	sys, err := st.LatestSystemEval(ctx)
	require.NoError(t, err)
	assert.Nil(t, sys)
}

func TestExecuteCrisisReplayNoData(t *testing.T) {
	st := newCrisisTestStore(t)
	var buf bytes.Buffer
	err := executeCrisisReplay(context.Background(), crisisTestConfig(), st, "2008-01-01", "2008-12-31", false, &buf)
	require.ErrorContains(t, err, "run backfill first")
}
```

- [ ] **Step 2: 运行确认失败**

Run: `GOTOOLCHAIN=local go test ./cmd/atlas/ -run TestExecuteCrisisReplay -v`
Expected: 编译失败，`undefined: executeCrisisReplay`

- [ ] **Step 3: 在 `cmd/atlas/crisis.go` 追加 replay 子命令**（imports 增加 `encoding/json`）

```go
var (
	replayFrom string
	replayTo   string
	replayJSON bool
)

var crisisReplayCmd = &cobra.Command{
	Use:   "replay",
	Short: "Replay rules and state machine over backfilled history (no writes)",
	Long: `Re-runs the full evaluation pipeline day by day over macro_observations,
keeping evaluations in memory only. Used for the design §6 historical
acceptance (2008-09 / 2020-03 / 2024-08 / 2015-19 false-positive check) and
for threshold tuning: edit configs/crisis-monitor.yaml and re-run.`,
	RunE: runCrisisReplay,
}

// 追加进已有 init()：
	crisisReplayCmd.Flags().StringVar(&replayFrom, "from", "", "start date YYYY-MM-DD (required)")
	crisisReplayCmd.Flags().StringVar(&replayTo, "to", "", "end date YYYY-MM-DD (required)")
	crisisReplayCmd.Flags().BoolVar(&replayJSON, "json", false, "emit transitions as JSON lines")
	crisisCmd.AddCommand(crisisReplayCmd)
```

```go
func runCrisisReplay(cmd *cobra.Command, args []string) error {
	if replayFrom == "" || replayTo == "" {
		return fmt.Errorf("--from and --to are required")
	}
	ccfg, st, err := openCrisisStore()
	if err != nil {
		return err
	}
	defer st.Close()
	return executeCrisisReplay(cmd.Context(), ccfg, st, replayFrom, replayTo, replayJSON, cmd.OutOrStdout())
}

// executeCrisisReplay 逐日重放：观测来自 sqlite，评估历史只进 MemHistory，
// 不写 crisis_evaluations（审计表只属于 live eval）。
func executeCrisisReplay(ctx context.Context, cfg *crisis.Config, st *crisis.Store, from, to string, jsonOut bool, out io.Writer) error {
	dates, err := st.EvalDates(ctx, from, to)
	if err != nil {
		return err
	}
	if len(dates) == 0 {
		return fmt.Errorf("no observations between %s and %s — run backfill first", from, to)
	}

	mem := crisis.NewMemHistory()
	reader := st.Reader(ctx)
	entered := map[crisis.SystemState]int{}
	cur := crisis.StateNormal
	for _, d := range dates {
		res, err := crisis.EvalDay(cfg, d, reader, mem, time.Now())
		if err != nil {
			return fmt.Errorf("evaluating %s: %w", d, err)
		}
		mem.Append(res.Evaluations)
		if res.Transitioned() {
			entered[res.State]++
			if jsonOut {
				b, _ := json.Marshal(map[string]any{
					"date": d, "from": res.PrevState, "to": res.State, "amber_count": res.Detail.AmberCount,
				})
				fmt.Fprintln(out, string(b))
			} else {
				fmt.Fprintf(out, "%s  %s → %s (amber=%d)\n", d, res.PrevState, res.State, res.Detail.AmberCount)
			}
		}
		cur = res.State
	}

	fmt.Fprintf(out, "\nfinal state: %s over %d eval days\n", cur, len(dates))
	for _, s := range []crisis.SystemState{crisis.StateWatch, crisis.StateBrewing, crisis.StateCrisis} {
		fmt.Fprintf(out, "entered %-8s %d times\n", s, entered[s])
	}
	return nil
}
```

- [ ] **Step 4: 运行确认通过**

Run: `GOTOOLCHAIN=local go test ./cmd/atlas/ -run TestExecuteCrisisReplay -v`
Expected: PASS（2 个测试全绿）

- [ ] **Step 5: 提交**

```bash
git add cmd/atlas/crisis.go cmd/atlas/crisis_test.go
git commit -m "feat(crisis): add replay backtest command"
```

- [ ] **Step 6: 执行本任务上方的"第二阶段人工验收"四段回放**（前置：Task 7 的 backfill 已完成、HY OAS 快照已导入）。判定"情绪层双红之前"用 `sqlite3 data/crisis.db "SELECT MIN(ts) FROM macro_observations WHERE indicator='vix' AND value>30 AND ts BETWEEN '<段起>' AND '<段止>'"` 找 vix 首红日，与 replay 输出的 WATCH/BREWING 进入日比较。不达标只调 YAML 重跑，最终参数与结果记入本文件附注。

## 第三阶段：通知与部署（Task 14–15）

### Task 14: 通知模板与 telegram 接入 `notify.go`

**Files:**
- Create: `internal/crisis/notify.go`
- Modify: `cmd/atlas/crisis.go`（eval 流程末尾接 Sender）
- Test: `internal/crisis/notify_test.go`

**Interfaces:**
- Consumes: `DayResult`；`telegram.New(cfg.Notifiers["telegram"].BotToken, .ChatID, telegram.WithProxy(.Proxy))` + `SendText`（`cmd/atlas/serve.go:330` 同款构造，零改动复用）
- Produces: `Sender` 接口、`Messages(res, stateDays, summaryDue, staleInds) []string`

通知策略（设计 §3.3 表 + §4.4）：
- 状态变更：`[P1]`，进入 BREWING/CRISIS 时 `[P0]`；BREWING/CRISIS 态无变更日 → `[P1]` 日报。
- summaryDue 由 cmd 计算：NORMAL ∧ 当月首个交易日 → 月报（附 7 指标读数+分位）；WATCH ∧ 周一 → 周报。
- STALE 指标 → `[P2]` 运维告警。
- 模板含：当前状态、触发指标及读数、5 年分位、状态持续天数、下一评估提示；页脚固定边界声明。
- **单测断言所有文案不含"必然/一定/即将"**（Global Constraints）。
- 发送失败仅记 stderr 不失败退出（评估已落库，通知丢失可由下次状态查询自愈——文件真相源原则）。

- [ ] **Step 1: 写失败测试** — 创建 `internal/crisis/notify_test.go`

```go
package crisis

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func dayResult(prev, cur SystemState) *DayResult {
	results := map[string]IndicatorResult{}
	for _, ind := range AllIndicators {
		results[ind] = IndicatorResult{Indicator: ind, Status: StatusGreen, RawStatus: StatusGreen, Value: 1, Pct5y: 0.5}
	}
	return &DayResult{Date: "2026-07-10", Results: results, PrevState: prev, State: cur}
}

func TestMessagesTransitionPriorities(t *testing.T) {
	msgs := Messages(dayResult(StateWatch, StateBrewing), 1, false, nil)
	require.Len(t, msgs, 1)
	assert.True(t, strings.HasPrefix(msgs[0], "[P0]")) // 进入 BREWING = P0 一次（设计 §3.3）
	assert.Contains(t, msgs[0], "WATCH → BREWING")

	msgs = Messages(dayResult(StateNormal, StateWatch), 1, false, nil)
	require.Len(t, msgs, 1)
	assert.True(t, strings.HasPrefix(msgs[0], "[P1]")) // 一般状态变更 = P1
}

func TestMessagesDigestsAndSummaries(t *testing.T) {
	// BREWING 无变更日 → 每日推送
	msgs := Messages(dayResult(StateBrewing, StateBrewing), 5, false, nil)
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0], "日报")

	// NORMAL：仅 summaryDue 时发月报，否则静默（设计 §3.3 表）
	msgs = Messages(dayResult(StateNormal, StateNormal), 30, true, nil)
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0], "月度摘要")
	assert.Empty(t, Messages(dayResult(StateNormal, StateNormal), 30, false, nil))

	// WATCH + summaryDue → 周度摘要
	msgs = Messages(dayResult(StateWatch, StateWatch), 3, true, nil)
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0], "周度摘要")

	// STALE 指标 → P2 运维告警
	msgs = Messages(dayResult(StateNormal, StateNormal), 1, false, []string{IndMOVE})
	require.Len(t, msgs, 1)
	assert.True(t, strings.HasPrefix(msgs[0], "[P2]"))
	assert.Contains(t, msgs[0], "move")
}

// Global Constraints：所有文案禁止确定性字样，状态类通知必须带边界声明页脚。
func TestMessagesForbiddenWords(t *testing.T) {
	all := [][]string{
		Messages(dayResult(StateWatch, StateBrewing), 1, false, []string{IndMOVE}),
		Messages(dayResult(StateBrewing, StateBrewing), 5, true, nil),
		Messages(dayResult(StateNormal, StateNormal), 30, true, nil),
		Messages(dayResult(StateNormal, StateCrisis), 1, false, nil),
	}
	for _, msgs := range all {
		for _, m := range msgs {
			for _, banned := range []string{"必然", "一定", "即将"} {
				assert.NotContains(t, m, banned)
			}
			if strings.HasPrefix(m, "[P0]") || strings.HasPrefix(m, "[P1]") {
				assert.Contains(t, m, "非交易信号")
			}
		}
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `GOTOOLCHAIN=local go test ./internal/crisis/ -run TestMessages -v`
Expected: 编译失败，`undefined: Messages`

- [ ] **Step 3: 实现 notify.go**

```go
package crisis

import (
	"fmt"
	"strings"
)

// Sender is the outbound channel; telegram.Telegram's SendText satisfies it.
// Single channel, no priority routing — urgency rides in the [P0]/[P1]/[P2]
// text prefix (design §4.4).
type Sender interface {
	SendText(text string) error
}

// footer is the fixed boundary disclaimer (design §5). Wording stays
// probabilistic everywhere — deterministic claims are banned by test.
const footer = "\n—\n本通知为风险状态提示（概率语言），非交易信号；指标组合基于有限历史危机样本，存在失效可能；资产操作决策（降杠杆、网格暂停等）不在本模块范围。"

// Messages renders the day's outbound notifications per the design §3.3
// state/notification table: transition alerts ([P0] on entering
// BREWING/CRISIS, else [P1]), daily digests while in BREWING/CRISIS, periodic
// summaries (monthly in NORMAL, weekly in WATCH — summaryDue computed by the
// caller), plus [P2] ops alerts for stale indicators.
func Messages(res *DayResult, stateDays int, summaryDue bool, staleInds []string) []string {
	var msgs []string

	switch {
	case res.Transitioned():
		prefix := "[P1]"
		if res.State == StateBrewing || res.State == StateCrisis {
			prefix = "[P0]"
		}
		msgs = append(msgs, fmt.Sprintf("%s 危机监控状态变更：%s → %s（%s）\n%s%s",
			prefix, res.PrevState, res.State, res.Date, indicatorLines(res), footer))
	case res.State == StateBrewing || res.State == StateCrisis:
		msgs = append(msgs, fmt.Sprintf("[P1] 危机监控日报（%s 第 %d 个评估日，%s）\n%s%s",
			res.State, stateDays, res.Date, indicatorLines(res), footer))
	case summaryDue:
		kind := "月度摘要"
		if res.State == StateWatch {
			kind = "周度摘要"
		}
		msgs = append(msgs, fmt.Sprintf("[P1] 危机监控%s（%s，%s 已持续 %d 个评估日）\n%s%s",
			kind, res.Date, res.State, stateDays, indicatorLines(res), footer))
	}

	for _, ind := range staleInds {
		msgs = append(msgs, fmt.Sprintf(
			"[P2] 运维告警：%s 数据超过新鲜度窗口，标记 STALE，已退出共振计数（%s）", ind, res.Date))
	}
	return msgs
}

// indicatorLines renders one line per indicator: status, reading, 5y
// percentile and tag（设计 §4.4 模板要素），加下一评估提示。
func indicatorLines(res *DayResult) string {
	var b strings.Builder
	for _, ind := range AllIndicators {
		r := res.Results[ind]
		fmt.Fprintf(&b, "%-10s %-20s %10.2f", ind, r.Status, r.Value)
		if r.Pct5y >= 0 {
			fmt.Fprintf(&b, "  5y分位 %2.0f%%", r.Pct5y*100)
		}
		if r.Tag != "" {
			fmt.Fprintf(&b, "  [%s]", r.Tag)
		}
		b.WriteString("\n")
	}
	b.WriteString("下一评估：下一交易日（launchd 多时点唤起）")
	return b.String()
}
```

- [ ] **Step 4: 运行确认通过**

Run: `GOTOOLCHAIN=local go test ./internal/crisis/ -run TestMessages -v`
Expected: PASS（3 个测试全绿）

- [ ] **Step 5: 写 cmd 接线的失败测试** — 在 `cmd/atlas/crisis_test.go` 追加

```go
func TestSummaryDue(t *testing.T) {
	assert.True(t, summaryDue("2026-07-01", crisis.StateNormal))  // 周三 = 当月首交易日 → 月报
	assert.False(t, summaryDue("2026-07-02", crisis.StateNormal))
	assert.True(t, summaryDue("2026-08-03", crisis.StateNormal))  // 8/1 周六 → 首交易日 = 8/3 周一
	assert.True(t, summaryDue("2026-07-13", crisis.StateWatch))   // 周一 → 周报
	assert.False(t, summaryDue("2026-07-14", crisis.StateWatch))
	assert.False(t, summaryDue("2026-07-13", crisis.StateBrewing)) // BREWING 走日报，不走摘要
}
```

Run: `GOTOOLCHAIN=local go test ./cmd/atlas/ -run TestSummaryDue -v` → 编译失败 `undefined: summaryDue`

- [ ] **Step 6: 在 `cmd/atlas/crisis.go` 接入通知**（imports 增加 `github.com/newthinker/atlas/internal/notifier/telegram`）

1. `crisisEvalDeps` 增加字段 `sender crisis.Sender`；`runCrisisEval` 的 daily 分支构造 deps 时补 `sender: buildCrisisSender()`。
2. `executeCrisisEvalDaily` 在 `printDayResult(d.out, res)` 之后追加：

```go
	days, err := stateStreakDays(ctx, d.store, res.State)
	if err != nil {
		return err
	}
	for _, msg := range crisis.Messages(res, days, summaryDue(target, res.State), staleIndicators(res)) {
		if d.sender == nil {
			fmt.Fprintln(d.out, msg) // 未配置 telegram：打印便于本地试运行
			continue
		}
		if err := d.sender.SendText(msg); err != nil {
			// 通知失败不失败退出：评估已落库，状态可由 status 自愈获取（文件真相源）
			fmt.Fprintf(d.errOut, "warning: notify failed: %v\n", err)
		}
	}
	return nil
```

3. 同文件追加助手：

```go
// buildCrisisSender 复用主配置 notifiers.telegram 凭据（serve.go:330 同款构
// 造，notifier 零改动）。未配置 → nil。
func buildCrisisSender() crisis.Sender {
	cfg, err := loadConfigOrDefaults()
	if err != nil {
		return nil
	}
	nc, ok := cfg.Notifiers["telegram"]
	if !ok || !nc.Enabled || nc.BotToken == "" || nc.ChatID == "" {
		return nil
	}
	return telegram.New(nc.BotToken, nc.ChatID, telegram.WithProxy(nc.Proxy))
}

// summaryDue：NORMAL → 当月首个交易日发月报（设计 §4.3：不加第 4 个 plist，
// daily eval 内判断）；WATCH → 周一发周报。
func summaryDue(date string, state crisis.SystemState) bool {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return false
	}
	switch state {
	case crisis.StateNormal:
		return isFirstTradingDayOfMonth(t)
	case crisis.StateWatch:
		return t.Weekday() == time.Monday
	}
	return false
}

func isFirstTradingDayOfMonth(t time.Time) bool {
	if t.Weekday() == time.Saturday || t.Weekday() == time.Sunday {
		return false
	}
	first := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
	for first.Weekday() == time.Saturday || first.Weekday() == time.Sunday {
		first = first.AddDate(0, 0, 1)
	}
	return t.Equal(first)
}

func staleIndicators(res *crisis.DayResult) []string {
	var out []string
	for _, ind := range crisis.AllIndicators {
		if res.Results[ind].Status == crisis.StatusStale {
			out = append(out, ind)
		}
	}
	return out
}
```

- [ ] **Step 7: 运行确认通过（cmd + crisis 双包回归）**

Run: `GOTOOLCHAIN=local go test ./cmd/atlas/ ./internal/crisis/ -v`
Expected: PASS（既有 eval 测试因 sender=nil 走打印路径，无回归）

- [ ] **Step 8: 提交**

```bash
git add internal/crisis/notify.go internal/crisis/notify_test.go cmd/atlas/crisis.go cmd/atlas/crisis_test.go
git commit -m "feat(crisis): add notification templates and telegram wiring"
```

### Task 15: 盘中 JPY 模式 + 3 个 launchd plist + 部署清单（第三阶段收口）

**Files:**
- Modify: `cmd/atlas/crisis.go`（`eval --mode intraday`）、`internal/crisis/store.go`（+`HasIndicatorEvalForDate`）
- Create: `deploy/launchd/com.newthinker.atlas.crisis-daily.plist`、`...crisis-nfci.plist`、`...crisis-intraday-jpy.plist`
- Test: `cmd/atlas/crisis_test.go`（追加）

intraday 模式（设计 §4.3 第三行）：启动即读 `LatestSystemEval`，非 BREWING/CRISIS → exit 0（空跑近零）；否则 `yahoo.FetchQuote("JPY=X")` 取实时价，与库中 5 观测前收盘比 wow，`≤ red_wow_pct` → `[P0]` 告警；以 `indicator='usdjpy_intraday'` 评估行做**每日一次**去重。

plist 要点（模板照 `deploy/launchd/com.newthinker.atlas.refresh-us.plist`：WorkingDirectory `/Users/zuowei/workspace/runtime/atlas`、日志到 runtime logs、PATH 含 go）：
- **crisis-daily**：`StartCalendarInterval` 数组多时点 22:45 / 23:45 / 次日 07:30（+0800 覆盖 ET 上午发布 ± 夏令时漂移，设计 §4.3）；每天触发，周末由"评估日=前一交易日+幂等"自然空跑。
- **crisis-nfci**：周三 21:00 与 22:00 两时点（ET 8:30 发布后）。
- **crisis-intraday-jpy**：`StartInterval 1800`。

**部署与试运行清单：**

- [ ] `GOTOOLCHAIN=local go test ./... && GOTOOLCHAIN=local go build -o bin/atlas ./cmd/atlas`
- [ ] 同步二进制/配置到 runtime 目录（沿用 refresh-market 部署方式），`data/crisis.db` 已含 backfill 数据
- [ ] `cp deploy/launchd/com.newthinker.atlas.crisis-*.plist ~/Library/LaunchAgents/` + 逐个 `launchctl bootstrap gui/$(id -u) ...`
- [ ] 手动触发一次 `launchctl kickstart` 验证日志输出与幂等空跑
- [ ] 试运行两周（设计 §6）；Grafana 面板届时再评估，**不在本期**

- [ ] **Step 1: 写失败测试** — 在 `cmd/atlas/crisis_test.go` 追加（imports 增加 `github.com/newthinker/atlas/internal/core`）

```go
func TestExecuteCrisisIntraday(t *testing.T) {
	st := newCrisisTestStore(t)
	ctx := context.Background()
	deps := crisisEvalDeps{
		cfg: crisisTestConfig(), store: st,
		now: func() time.Time { return time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC) },
		out: io.Discard, errOut: io.Discard,
	}
	var quoteCalls int
	quote := func(string) (*core.Quote, error) {
		quoteCalls++
		return &core.Quote{Price: 145}, nil
	}

	// 非 BREWING/CRISIS → 空跑退出，连行情都不取（设计 §4.3：空跑成本近零）
	require.NoError(t, executeCrisisIntraday(ctx, deps, quote))
	assert.Equal(t, 0, quoteCalls)

	// BREWING 态 + 5 观测前收盘 150、现价 145 → wow=−3.3% ≤ −3% → 告警一次
	seedObservations(t, st, "2026-07-09", 10)
	require.NoError(t, st.AppendEvaluations(ctx, []crisis.Evaluation{{
		TS: "2026-07-09", EvalAt: "2026-07-10T00:00:00.000000000Z", Indicator: "",
		SystemState: crisis.StateBrewing, Detail: "{}",
	}}))
	require.NoError(t, executeCrisisIntraday(ctx, deps, quote))
	assert.Equal(t, 1, quoteCalls)
	sent, err := st.HasIndicatorEvalForDate(ctx, "usdjpy_intraday", "2026-07-10")
	require.NoError(t, err)
	assert.True(t, sent)

	// 同日第二次唤起 → 每日一次去重，不再取行情
	require.NoError(t, executeCrisisIntraday(ctx, deps, quote))
	assert.Equal(t, 1, quoteCalls)
}
```

- [ ] **Step 2: 运行确认失败**

Run: `GOTOOLCHAIN=local go test ./cmd/atlas/ -run TestExecuteCrisisIntraday -v`
Expected: 编译失败，`undefined: executeCrisisIntraday`（以及 `HasIndicatorEvalForDate`）

- [ ] **Step 3: store 增加去重查询 + 实现 intraday 模式**

`internal/crisis/store.go` 追加：

```go
// HasIndicatorEvalForDate dedupes per-day one-shot alerts (intraday JPY).
func (s *Store) HasIndicatorEvalForDate(ctx context.Context, indicator, date string) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM crisis_evaluations WHERE indicator = ? AND ts = ?`, indicator, date).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("checking eval for %s/%s: %w", indicator, date, err)
	}
	return n > 0, nil
}
```

`cmd/atlas/crisis.go` 追加（imports 增加 `github.com/newthinker/atlas/internal/core`；`--mode` 帮助文案改为 `daily | nfci | intraday`）：

```go
// intradayIndicator 是盘中告警的去重行标识（不属于 7 个正式指标）。
const intradayIndicator = "usdjpy_intraday"

// runCrisisEval 的 switch 追加分支：
	case "intraday":
		deps := crisisEvalDeps{
			cfg: ccfg, store: st, now: time.Now,
			out: cmd.OutOrStdout(), errOut: cmd.ErrOrStderr(), sender: buildCrisisSender(),
		}
		return executeCrisisIntraday(cmd.Context(), deps, yahoo.New().FetchQuote)
```

```go
// executeCrisisIntraday（设计 §4.3 intraday_jpy 行）：先读库中系统状态，非
// BREWING/CRISIS 立即退出；否则用 JPY=X 实时价对库中 5 观测前收盘算周环比，
// 触红即发 [P0]（捕捉 carry trade 急平仓），以评估行做每日一次去重。
func executeCrisisIntraday(ctx context.Context, d crisisEvalDeps, quote func(string) (*core.Quote, error)) error {
	sys, err := d.store.LatestSystemEval(ctx)
	if err != nil {
		return err
	}
	if sys == nil || (sys.SystemState != crisis.StateBrewing && sys.SystemState != crisis.StateCrisis) {
		return nil
	}

	today := d.now().UTC().Format("2006-01-02")
	sent, err := d.store.HasIndicatorEvalForDate(ctx, intradayIndicator, today)
	if err != nil {
		return err
	}
	if sent {
		return nil
	}

	q, err := quote("JPY=X")
	if err != nil {
		return err
	}
	win, err := d.store.SeriesWindow(ctx, crisis.IndUSDJPY, today, 5)
	if err != nil {
		return err
	}
	if len(win) < 5 || win[0].Value == 0 {
		return nil // 历史不足，无法算周环比
	}
	wow := q.Price/win[0].Value - 1
	if wow > d.cfg.Indicators.USDJPY.RedWowPct {
		return nil
	}

	// 先落去重行再发送（文件真相源先行，通知丢失不重复告警）
	if err := d.store.AppendEvaluations(ctx, []crisis.Evaluation{{
		TS: today, EvalAt: crisis.NowStamp(d.now()), Indicator: intradayIndicator,
		Status: crisis.StatusRed, Value: q.Price,
		Detail: fmt.Sprintf(`{"wow":%.4f}`, wow),
	}}); err != nil {
		return err
	}
	msg := fmt.Sprintf("[P0] 盘中告警：USD/JPY 周环比 %.1f%%（现价 %.2f），疑似 carry trade 急平仓（%s，系统状态 %s）",
		wow*100, q.Price, today, sys.SystemState)
	if d.sender == nil {
		fmt.Fprintln(d.out, msg)
		return nil
	}
	if err := d.sender.SendText(msg); err != nil {
		fmt.Fprintf(d.errOut, "warning: notify failed: %v\n", err)
	}
	return nil
}
```

- [ ] **Step 4: 运行确认通过（全量回归）**

Run: `GOTOOLCHAIN=local go test ./...`
Expected: PASS（全仓库无回归）

- [ ] **Step 5: 创建 3 个 launchd plist**

`deploy/launchd/com.newthinker.atlas.crisis-daily.plist`（FRED key 由 `--config` 指向的 runtime `configs/config.yaml` 提供——`resolveFREDKey` 回退路径，plist 内不放密钥）：

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>com.newthinker.atlas.crisis-daily</string>

  <key>ProgramArguments</key>
  <array>
    <string>/Users/zuowei/workspace/runtime/atlas/bin/atlas</string>
    <string>crisis</string>
    <string>eval</string>
    <string>--mode</string>
    <string>daily</string>
    <string>--config</string>
    <string>/Users/zuowei/workspace/runtime/atlas/configs/config.yaml</string>
    <string>--crisis-config</string>
    <string>/Users/zuowei/workspace/runtime/atlas/configs/crisis-monitor.yaml</string>
  </array>

  <key>WorkingDirectory</key>
  <string>/Users/zuowei/workspace/runtime/atlas</string>

  <key>EnvironmentVariables</key>
  <dict>
    <key>PATH</key>
    <string>/usr/local/go/bin:/usr/bin:/bin:/usr/sbin:/sbin</string>
  </dict>

  <!-- 多时点唤起（本地 +0800）覆盖 ET 上午发布 ± 夏令时漂移（设计 §4.3）：
       当晚 22:45 / 23:45，次日 07:30 兜底；周末与数据未齐由幂等+齐备性校验空跑。 -->
  <key>StartCalendarInterval</key>
  <array>
    <dict><key>Hour</key><integer>22</integer><key>Minute</key><integer>45</integer></dict>
    <dict><key>Hour</key><integer>23</integer><key>Minute</key><integer>45</integer></dict>
    <dict><key>Hour</key><integer>7</integer><key>Minute</key><integer>30</integer></dict>
  </array>

  <key>RunAtLoad</key>
  <false/>

  <key>StandardOutPath</key>
  <string>/Users/zuowei/workspace/runtime/atlas/logs/crisis-daily.out.log</string>
  <key>StandardErrorPath</key>
  <string>/Users/zuowei/workspace/runtime/atlas/logs/crisis-daily.err.log</string>
</dict>
</plist>
```

`deploy/launchd/com.newthinker.atlas.crisis-nfci.plist`：同上模板，改 `Label` 为 `...crisis-nfci`、`--mode` 为 `nfci`、日志名为 `crisis-nfci.*.log`，`StartCalendarInterval` 换为（NFCI 周三 ET 8:30 发布后，+0800 晚间两时点覆盖夏令时漂移）：

```xml
  <key>StartCalendarInterval</key>
  <array>
    <dict><key>Weekday</key><integer>3</integer><key>Hour</key><integer>21</integer><key>Minute</key><integer>0</integer></dict>
    <dict><key>Weekday</key><integer>3</integer><key>Hour</key><integer>22</integer><key>Minute</key><integer>0</integer></dict>
  </array>
```

`deploy/launchd/com.newthinker.atlas.crisis-intraday-jpy.plist`：同上模板，改 `Label` 为 `...crisis-intraday-jpy`、`--mode` 为 `intraday`、日志名为 `crisis-intraday-jpy.*.log`，定时改为每 30 分钟（设计 §4.3：非 BREWING/CRISIS 空跑立即退出）：

```xml
  <key>StartInterval</key>
  <integer>1800</integer>
```

- [ ] **Step 6: 提交**

```bash
git add internal/crisis/store.go cmd/atlas/crisis.go cmd/atlas/crisis_test.go deploy/launchd/com.newthinker.atlas.crisis-*.plist
git commit -m "feat(crisis): add intraday mode, launchd plists and deployment"
```

- [ ] **Step 7: 执行本任务下方的"部署与试运行清单"**，完成后按 superpowers:finishing-a-development-branch 决定合并方式（merge / PR）

---

## 范围外（设计 §5，执行者不得顺手实现）

Grafana 面板、strategy 层联动、notifier 优先级路由公共化、假日日历、FRED collector 注册进 collector.Registry（crisis 直连使用，无注册需求）。

---

## 附注：第二阶段历史验收结果（2026-07-14，阈值未调整）

四段回测全部达标（configs/crisis-monitor.yaml 初始参数原样通过）：
- 2019-06~2020-06：2020-02-24 WATCH → 02-27 双红 CRISIS（提前 3 交易日）✓
- 2024 全年：2024-01-02 WATCH（t10y2y 倒挂领先红）→ 08-05 双红 CRISIS ✓
- 2007~2009：2007-01-03 WATCH ≪ vix 首红；2009-12-01 CRISIS 退出后 WATCH 恰满 20 交易日（12-29）降 NORMAL（态内计数 7d84524 实证）✓
- 2015~2019：BREWING 0 次（≤1）✓
限定条件：hy_oas 仅回填到 2023-07-14（FRED 三年截断），2008 段与 2015-19 段的 BREWING 语义待人工导入 2006 起 HY OAS CSV 快照后复跑确认（`bin/atlas crisis backfill --csv <快照> --indicator hy_oas --scale 100`）。

### 附注补充（2026-07-14 下午）：HY OAS 历史快照已导入，全量复验通过

- **数据源**：GitHub 公开镜像 `Duzzuti/fear-and-greed`（FRED BAMLH0A0HYM2 存档 2000-01-03~2024-12-19）。可信度验证：与 FRED 官方现存段 378 个重叠日**零不一致**（浮点精确相等），2008-12-15 峰值 21.82% 与史实吻合。裁剪 ≤2023-07-13 段（6,142 行）经 `crisis backfill --csv --scale 100` 导入，source=manual_backfill，与官方段无缝衔接（07-13 397bp → 07-14 390bp）。快照留存 `data/hyoas-2000-2023-snapshot.csv`（dev 与 runtime；ICE 授权限制，**不入公开仓库**）。
- **全量复跑四段回测**：仍全部达标。2008 段带真实 HY 后 2009-12 退出更保守（HY 高位使 WATCH 持续至段末，符合语义）；2015-19 段 BREWING 仍 0 次，新增 WATCH 进入对应 2015-16 能源高收益债危机等真实压力期。阈值仍未调整。
- **launchd Yahoo 403 已修**：本网络直连被边缘封锁，crisis-daily/crisis-intraday-jpy plist 增加本地代理 env（e3f1496）。
