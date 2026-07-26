package sankey

import (
	"errors"
	"fmt"
	"math"
	"strings"

	prismstore "github.com/newthinker/atlas/internal/storage/prism"
)

// ErrNoTemplate is returned by Analyze for a symbol that has no sankey
// template configured. Callers match it with errors.Is to answer 404 rather
// than 500.
var ErrNoTemplate = errors.New("sankey: no template for symbol")

// PeriodView is one period as the front end consumes it.
type PeriodView struct {
	Period  string
	Graph   Graph
	Metrics PeriodMetrics
}

// Analysis is the payload behind /api/prism/sankey.
type Analysis struct {
	Symbol, Granularity string
	Periods             []PeriodView
	Matrix              []MatrixRow
	Truncated           bool // more periods existed than MaxPeriods allows
	// Conflicts lists periods the engine refused to aggregate because their
	// fiscal_period label covers several quarters. Surfacing them is the point:
	// the alternative is a page that quietly shows a fiscal year at three times
	// its real size.
	Conflicts []PeriodConflict
	Template  struct {
		Version int
		Lang    map[string]string // segment_key → display name in the requested lang
	}
}

// FundamentalView is the payload behind /api/prism/fundamental.
type FundamentalView struct {
	Symbol  string
	Periods []string // fiscal_period, ascending
	// PeriodEnds is the calendar date each Periods entry closes on, same order
	// and length. It is carried alongside the labels because the labels cannot
	// stand in for it: a fiscal "2026Q1" ends in September for a company whose
	// year is offset, so anything that has to place a report period on a real
	// timeline — the chart page resampling prices onto report periods — needs
	// this column rather than a parsed label.
	PeriodEnds []string
	Metrics    map[string][]float64
	Dates      []string // price_daily
	Closes     []float64
}

// ServiceStore is the subset of *prismstore.Store the analysis needs.
//
// InstrumentID exists because the read paths only receive a symbol while the
// row accessors are keyed by id, and the only other way to obtain one is
// UpsertInstrument — a write whose ON CONFLICT clause would blank the
// instrument's market/name/grp/source (AD-24).
type ServiceStore interface {
	InstrumentID(symbol string) (int64, error)
	QuarterlyFundamentals(instrumentID int64) ([]prismstore.FundamentalRow, error)
	SegmentRows(instrumentID int64) ([]prismstore.SegmentRow, error)
	PriceSeries(symbol, from string) ([]string, []float64, error)
}

// Service answers the sankey and fundamental queries from stored rows.
type Service struct {
	store     ServiceStore
	templates map[string]*Template
}

func NewService(store ServiceStore, templates map[string]*Template) *Service {
	return &Service{store: store, templates: templates}
}

// Analyze builds the earnings bridge for one symbol. from/to are inclusive
// period labels; both empty means DefaultSelection picks the comparison
// context. granularity is "q" or "fy" and, when empty, follows DefaultSelection.
// lang is "zh" or "en".
//
// A symbol without a template yields ErrNoTemplate so the caller can answer
// 404 rather than 500.
func (s *Service) Analyze(symbol, from, to, granularity, lang string) (*Analysis, error) {
	tmpl, ok := s.templates[strings.ToUpper(symbol)]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNoTemplate, symbol)
	}
	funds, segs, err := s.rows(symbol)
	if err != nil {
		return nil, err
	}

	// The fy pass reports every conflict the q pass does, plus the fiscal years
	// it had to refuse, so it is the one worth surfacing.
	quarters, _ := BuildPeriods(funds, segs, "q")
	fys, conflicts := BuildPeriods(funds, segs, "fy")

	var sel []PeriodMetrics
	switch granularity {
	case "q":
		sel = quarters
	case "fy":
		sel = fys
	default:
		sel, granularity = DefaultSelection(quarters, fys)
	}
	sel, truncated := truncate(filterRange(sel, from, to))

	out := &Analysis{
		Symbol: symbol, Granularity: granularity,
		Truncated: truncated, Conflicts: conflicts,
	}
	out.Template.Version = tmpl.Version
	out.Template.Lang = langMap(tmpl, lang)
	for _, p := range sel {
		// The whole Graph is carried over, Notes included: dropping them would
		// leave a negative residual drawn at zero width with nothing to explain
		// it (AD-22).
		out.Periods = append(out.Periods, PeriodView{
			Period:  p.Period,
			Graph:   BuildSankey(p, tmpl, lang),
			Metrics: p,
		})
	}
	out.Matrix = BuildMatrix(sel, tmpl, lang, quarters)
	return out, nil
}

// Fundamental returns the quarterly series behind the fundamentals page.
func (s *Service) Fundamental(symbol string) (*FundamentalView, error) {
	id, err := s.store.InstrumentID(symbol)
	if err != nil {
		return nil, err
	}
	rows, err := s.store.QuarterlyFundamentals(id)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("%w: %s has no fundamentals", prismstore.ErrNotFound, symbol)
	}
	dates, closes, err := s.store.PriceSeries(symbol, "")
	if err != nil {
		return nil, err
	}

	out := &FundamentalView{
		Symbol: symbol, Dates: dates, Closes: closes,
		Metrics: map[string][]float64{"roe_ttm": roeTTM(rows)},
	}
	for _, r := range rows {
		out.Periods = append(out.Periods, r.FiscalPeriod)
		out.PeriodEnds = append(out.PeriodEnds, r.PeriodEnd)
	}
	for _, m := range fundamentalMetrics {
		series := make([]float64, 0, len(rows))
		for _, r := range rows {
			series = append(series, m.value(r))
		}
		out.Metrics[m.key] = series
	}
	return out, nil
}

var fundamentalMetrics = []struct {
	key   string
	value func(prismstore.FundamentalRow) float64
}{
	{"revenue", func(r prismstore.FundamentalRow) float64 { return r.Revenue }},
	{"gross_profit", func(r prismstore.FundamentalRow) float64 { return r.GrossProfit }},
	{"operating_income", func(r prismstore.FundamentalRow) float64 { return r.OperatingIncome }},
	{"net_income", func(r prismstore.FundamentalRow) float64 { return r.NetIncome }},
	{"rnd", func(r prismstore.FundamentalRow) float64 { return r.RnD }},
	{"sganda", func(r prismstore.FundamentalRow) float64 { return r.SGnA }},
	{"income_tax", func(r prismstore.FundamentalRow) float64 { return r.IncomeTax }},
	{"eps_diluted", func(r prismstore.FundamentalRow) float64 { return r.EPSDiluted }},
	{"equity", func(r prismstore.FundamentalRow) float64 { return r.Equity }},
}

func (s *Service) rows(symbol string) ([]prismstore.FundamentalRow, []prismstore.SegmentRow, error) {
	id, err := s.store.InstrumentID(symbol)
	if err != nil {
		return nil, nil, err
	}
	funds, err := s.store.QuarterlyFundamentals(id)
	if err != nil {
		return nil, nil, err
	}
	segs, err := s.store.SegmentRows(id)
	if err != nil {
		return nil, nil, err
	}
	return funds, segs, nil
}

// filterRange keeps the periods within [from, to]; either bound may be empty
// to leave that side open. Period labels sort lexicographically within one
// granularity ("2025Q4" < "2026Q1", "FY2025" < "FY2026"), so a string compare
// is the whole of it.
func filterRange(ps []PeriodMetrics, from, to string) []PeriodMetrics {
	if from == "" && to == "" {
		return ps
	}
	var out []PeriodMetrics
	for _, p := range ps {
		if from != "" && p.Period < from {
			continue
		}
		if to != "" && p.Period > to {
			continue
		}
		out = append(out, p)
	}
	return out
}

// truncate keeps the most recent MaxPeriods periods (AD-15) and reports
// whether anything was dropped.
func truncate(ps []PeriodMetrics) ([]PeriodMetrics, bool) {
	return lastN(ps, MaxPeriods), len(ps) > MaxPeriods
}

// roeTTM returns one ROE per row, aligned with rows: the trailing four
// quarters of net income over the equity at that quarter's end. The first
// three quarters have no full window and are NaN, as is any window holding a
// missing value — treating a gap as zero would understate the numerator and
// quietly report a plausible-looking ROE.
func roeTTM(rows []prismstore.FundamentalRow) []float64 {
	out := make([]float64, len(rows))
	for i := range rows {
		if i < 3 {
			out[i] = math.NaN()
			continue
		}
		var ttm float64
		for _, r := range rows[i-3 : i+1] {
			ttm += r.NetIncome
		}
		out[i] = safeDiv(ttm, rows[i].Equity)
	}
	return out
}

// langMap exposes the segment display names for the requested language so the
// front end can label a series it only knows by key.
func langMap(tmpl *Template, lang string) map[string]string {
	if tmpl == nil {
		return nil
	}
	out := make(map[string]string, len(tmpl.Segments))
	for _, s := range tmpl.Segments {
		out[s.Key] = displayName(s, lang)
	}
	return out
}
