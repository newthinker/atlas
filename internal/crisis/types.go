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
	RecentSystem(n int) ([]Evaluation, error)                      // 新→旧
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
