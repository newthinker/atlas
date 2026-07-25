package sankey

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	prismstore "github.com/newthinker/atlas/internal/storage/prism"
)

// MaxPeriods caps how many periods a selection may carry (AD-15). The engine,
// the API and the pages all read this one constant so the limit cannot drift
// apart again.
const MaxPeriods = 10

// residualShare is the share of revenue below which a residual node is dropped
// as noise rather than drawn (AD-13).
const residualShare = 0.005

// PeriodMetrics is one column of the analysis: a quarter ("2026Q1") or a
// fiscal year ("FY2026"). A NaN means "missing", never zero.
type PeriodMetrics struct {
	Period   string
	Segments map[string]float64 // segment_key → revenue

	Revenue, GrossProfit, OperatingIncome, NetIncome, RnD, SGnA, IncomeTax float64
	GrossMargin, OpMargin, NetMargin                                       float64
}

// BuildPeriods turns store rows into period columns, ascending by period.
//
// granularity "q" converts quarters one to one; "fy" groups them by the fiscal
// year embedded in the fiscal_period label (so a June-ending issuer's 2026Q1,
// whose period_end is 2025-09-30, still lands in FY2026) and sums the flow
// items. A fiscal year with fewer than four quarters is not emitted.
//
// Segment rows are attributed by fiscal_period. Rows whose label is empty
// (allowed by AD-9 when the period_end reverse lookup failed) cannot be placed
// in a period and are skipped; the revenue they carry then shows up in the
// sankey's other_segment residual rather than being charged to a wrong period.
func BuildPeriods(f []prismstore.FundamentalRow, segs []prismstore.SegmentRow, granularity string) []PeriodMetrics {
	quarters := buildQuarters(f, segs)
	if granularity != "fy" {
		return quarters
	}
	return aggregateFiscalYears(quarters)
}

func buildQuarters(f []prismstore.FundamentalRow, segs []prismstore.SegmentRow) []PeriodMetrics {
	byPeriod := make(map[string]map[string]float64)
	for _, s := range segs {
		if s.FiscalPeriod == "" {
			continue
		}
		if byPeriod[s.FiscalPeriod] == nil {
			byPeriod[s.FiscalPeriod] = make(map[string]float64)
		}
		byPeriod[s.FiscalPeriod][s.SegmentKey] += s.Revenue
	}

	out := make([]PeriodMetrics, 0, len(f))
	for _, r := range f {
		if r.FiscalPeriod == "" {
			continue
		}
		p := PeriodMetrics{
			Period: r.FiscalPeriod, Segments: byPeriod[r.FiscalPeriod],
			Revenue: r.Revenue, GrossProfit: r.GrossProfit, OperatingIncome: r.OperatingIncome,
			NetIncome: r.NetIncome, RnD: r.RnD, SGnA: r.SGnA, IncomeTax: r.IncomeTax,
		}
		p.setMargins()
		out = append(out, p)
	}
	sortByPeriod(out)
	return out
}

func aggregateFiscalYears(quarters []PeriodMetrics) []PeriodMetrics {
	groups := make(map[string][]PeriodMetrics)
	for _, q := range quarters {
		fy := fiscalYearOf(q.Period)
		if fy == "" {
			continue
		}
		groups[fy] = append(groups[fy], q)
	}

	out := make([]PeriodMetrics, 0, len(groups))
	for fy, qs := range groups {
		if len(qs) < 4 {
			continue // an incomplete fiscal year is never extrapolated
		}
		agg := PeriodMetrics{Period: "FY" + fy, Segments: make(map[string]float64)}
		for _, q := range qs {
			agg.Revenue += q.Revenue
			agg.GrossProfit += q.GrossProfit
			agg.OperatingIncome += q.OperatingIncome
			agg.NetIncome += q.NetIncome
			agg.RnD += q.RnD
			agg.SGnA += q.SGnA
			agg.IncomeTax += q.IncomeTax
			for k, v := range q.Segments {
				agg.Segments[k] += v
			}
		}
		// Margins come from the summed values, not from averaging quarterly ratios.
		agg.setMargins()
		out = append(out, agg)
	}
	sortByPeriod(out)
	return out
}

func (p *PeriodMetrics) setMargins() {
	p.GrossMargin = safeDiv(p.GrossProfit, p.Revenue)
	p.OpMargin = safeDiv(p.OperatingIncome, p.Revenue)
	p.NetMargin = safeDiv(p.NetIncome, p.Revenue)
}

func sortByPeriod(ps []PeriodMetrics) {
	sort.Slice(ps, func(i, j int) bool { return ps[i].Period < ps[j].Period })
}

// fiscalYearOf returns the fiscal year of "2026Q1" or "FY2026" as "2026".
func fiscalYearOf(period string) string {
	p := strings.TrimPrefix(period, "FY")
	if len(p) < 4 {
		return ""
	}
	return p[:4]
}

// safeDiv never returns ±Inf (AD-14): encoding/json refuses to marshal Inf and
// would turn the whole API response into a 500, while jf() only maps NaN.
func safeDiv(num, den float64) float64 {
	if den == 0 || math.IsNaN(den) || math.IsInf(den, 0) || math.IsInf(num, 0) {
		return math.NaN()
	}
	return num / den
}

// DefaultSelection picks the comparison context (design §5.6): the fiscal years
// once the newest one is complete (its annual report is out), otherwise the
// quarters published so far in the newest fiscal year.
//
// Both inputs must be ascending by period, as returned by BuildPeriods, and fys
// only ever holds complete fiscal years — that is what keeps an unfinished year
// (say a failed Q4 derivation) from being passed off as annual data.
func DefaultSelection(quarters, fys []PeriodMetrics) ([]PeriodMetrics, string) {
	switch {
	case len(quarters) == 0 && len(fys) == 0:
		return nil, "q"
	case len(quarters) == 0:
		return lastN(fys, MaxPeriods), "fy"
	}

	latest := fiscalYearOf(quarters[len(quarters)-1].Period)
	if len(fys) > 0 && fiscalYearOf(fys[len(fys)-1].Period) == latest {
		return lastN(fys, MaxPeriods), "fy"
	}

	var sel []PeriodMetrics
	for _, q := range quarters {
		if fiscalYearOf(q.Period) == latest {
			sel = append(sel, q)
		}
	}
	return sel, "q"
}

func lastN(ps []PeriodMetrics, n int) []PeriodMetrics {
	if len(ps) <= n {
		return ps
	}
	return ps[len(ps)-n:]
}

// MatrixRow is one line of the comparison table: one value per selected period
// plus a trailing change against the natural comparison period.
type MatrixRow struct {
	Key, Label string
	Values     []float64
	Change     float64
	ChangeKind string // "yoy" | "cagr" | "none"
}

// trunkRows are the fixed line items every issuer shares. They are hard-coded
// rather than templated (AD-5) because they are identical across companies.
var trunkRows = []struct {
	key    string
	value  func(PeriodMetrics) float64
	zh, en string
}{
	{"revenue", func(p PeriodMetrics) float64 { return p.Revenue }, "收入", "Revenue"},
	{"gross_profit", func(p PeriodMetrics) float64 { return p.GrossProfit }, "毛利", "Gross Profit"},
	{"operating_income", func(p PeriodMetrics) float64 { return p.OperatingIncome }, "营业利润", "Operating Income"},
	{"net_income", func(p PeriodMetrics) float64 { return p.NetIncome }, "净利润", "Net Income"},
	{"rnd", func(p PeriodMetrics) float64 { return p.RnD }, "研发费用", "R&D"},
	{"sganda", func(p PeriodMetrics) float64 { return p.SGnA }, "销售及管理费用", "SG&A"},
	{"income_tax", func(p PeriodMetrics) float64 { return p.IncomeTax }, "所得税", "Income Tax"},
	{"gross_margin", func(p PeriodMetrics) float64 { return p.GrossMargin }, "毛利率", "Gross Margin"},
	{"op_margin", func(p PeriodMetrics) float64 { return p.OpMargin }, "营业利润率", "Operating Margin"},
	{"net_margin", func(p PeriodMetrics) float64 { return p.NetMargin }, "净利率", "Net Margin"},
}

// BuildMatrix lays out segments (in template order) then the trunk line items
// and the three margins, one column per selected period.
//
// The trailing change is the year-on-year growth for quarters (against the same
// quarter of the previous fiscal year, looked up in allQuarters) and for two
// annual periods, or the interval CAGR once three or more annual periods are
// selected. A missing comparison period yields NaN with kind "none"; a
// comparison that exists but cannot be divided by (zero or NaN) yields NaN with
// the kind kept, so the caller can still tell the two apart.
func BuildMatrix(sel []PeriodMetrics, tmpl *Template, lang string, allQuarters []PeriodMetrics) []MatrixRow {
	if len(sel) == 0 {
		return nil
	}
	annual := strings.HasPrefix(sel[len(sel)-1].Period, "FY")

	rows := make([]MatrixRow, 0, len(trunkRows)+4)
	if tmpl != nil {
		for _, s := range tmpl.Segments {
			rows = append(rows, buildRow(s.Key, displayName(s, lang), sel, annual, allQuarters,
				func(p PeriodMetrics) float64 { return segmentValue(p, s.Key) }))
		}
	}
	for _, r := range trunkRows {
		rows = append(rows, buildRow(r.key, pick(lang, r.zh, r.en), sel, annual, allQuarters, r.value))
	}
	return rows
}

// segmentValue reports NaN for a segment the period does not carry: a zero
// would claim the segment earned nothing, which is a different statement.
func segmentValue(p PeriodMetrics, key string) float64 {
	if v, ok := p.Segments[key]; ok {
		return v
	}
	return math.NaN()
}

func buildRow(key, label string, sel []PeriodMetrics, annual bool,
	allQuarters []PeriodMetrics, value func(PeriodMetrics) float64) MatrixRow {
	row := MatrixRow{Key: key, Label: label, Values: make([]float64, 0, len(sel))}
	for _, p := range sel {
		row.Values = append(row.Values, value(p))
	}
	row.Change, row.ChangeKind = changeOf(row.Values, sel, annual, allQuarters, value)
	return row
}

func changeOf(values []float64, sel []PeriodMetrics, annual bool,
	allQuarters []PeriodMetrics, value func(PeriodMetrics) float64) (float64, string) {
	last := values[len(values)-1]

	if annual {
		switch {
		case len(values) >= 3:
			return cagr(values[0], last, len(values)-1), "cagr"
		case len(values) == 2:
			return growth(last, values[0]), "yoy"
		default:
			return math.NaN(), "none"
		}
	}

	prev, ok := findPeriod(allQuarters, previousYearLabel(sel[len(sel)-1].Period))
	if !ok {
		return math.NaN(), "none"
	}
	return growth(last, value(prev)), "yoy"
}

// growth returns NaN rather than ±Inf when the base is unusable (AD-14).
func growth(now, base float64) float64 {
	return safeDiv(now-base, base)
}

// cagr returns NaN when the interval cannot carry a compound rate: a start at
// or below zero has no meaningful growth rate, and one period spans nothing.
func cagr(first, last float64, spans int) float64 {
	if spans < 1 || math.IsNaN(first) || math.IsNaN(last) || first <= 0 {
		return math.NaN()
	}
	r := math.Pow(last/first, 1/float64(spans)) - 1
	if math.IsInf(r, 0) {
		return math.NaN()
	}
	return r
}

// previousYearLabel maps "2026Q2" to "2025Q2"; the quarter within the fiscal
// year is preserved, which is what makes this work for non-calendar years.
func previousYearLabel(period string) string {
	if len(period) < 5 {
		return ""
	}
	year, err := strconv.Atoi(period[:4])
	if err != nil {
		return ""
	}
	return strconv.Itoa(year-1) + period[4:]
}

func findPeriod(ps []PeriodMetrics, name string) (PeriodMetrics, bool) {
	if name == "" {
		return PeriodMetrics{}, false
	}
	for _, p := range ps {
		if p.Period == name {
			return p, true
		}
	}
	return PeriodMetrics{}, false
}

func displayName(s Segment, lang string) string {
	return pick(lang, s.NameZH, s.NameEN)
}

func pick(lang, zh, en string) string {
	if lang == "en" {
		return en
	}
	return zh
}

// Node and Link are the ECharts sankey payload; Notes carries what the diagram
// itself cannot show.
type Node struct {
	Name  string
	Value float64
}

type Link struct {
	Source, Target string
	Value          float64
}

type Graph struct {
	Nodes []Node
	Links []Link
	// Notes records flows that were drawn at zero width because their value was
	// negative (AD-13). Without it a negative residual would simply vanish from
	// the diagram with nothing to show for it.
	Notes []string
}

var sankeyLabels = map[string][2]string{
	"revenue":          {"收入", "Revenue"},
	"cogs":             {"营业成本", "Cost of Revenue"},
	"gross_profit":     {"毛利", "Gross Profit"},
	"opex":             {"营业费用", "Operating Expenses"},
	"rnd":              {"研发费用", "R&D"},
	"sganda":           {"销售及管理费用", "SG&A"},
	"other_opex":       {"其他营业费用", "Other OpEx"},
	"operating_income": {"营业利润", "Operating Income"},
	"tax_other":        {"税项及其他", "Tax & Other"},
	"net_income":       {"净利润", "Net Income"},
	"other_segment":    {"其他分部", "Other Segments"},
}

// BuildSankey draws one period as an ECharts sankey. The trunk is hard-coded
// (AD-5); the template only supplies the segments feeding revenue.
//
// Two residual nodes keep the diagram honest on real filings (AD-13), where
// Σsegments rarely equals revenue (corporate, eliminations) and R&D + SG&A
// rarely equals operating expenses (amortisation, impairments):
//
//	other_segment = Revenue − Σsegments
//	other_opex    = (GrossProfit − OperatingIncome) − RnD − SGnA
//
// Either is dropped when it is smaller than 0.5% of revenue. A negative flow
// (an issuer with large interest income has NetIncome above OperatingIncome) is
// drawn at zero width and recorded in Notes instead — the conservation only
// holds over non-negative flows, so the negative part must stay visible
// somewhere. NaN values stay NaN for the front end to render as "—".
func BuildSankey(p PeriodMetrics, tmpl *Template, lang string) Graph {
	var g graphBuilder
	name := func(key string) string { return pick(lang, sankeyLabels[key][0], sankeyLabels[key][1]) }

	var segmentSum float64
	if tmpl != nil {
		for _, s := range tmpl.Segments {
			v, ok := p.Segments[s.Key]
			if !ok {
				continue
			}
			segmentSum += v
			g.link(displayName(s, lang), name("revenue"), v)
		}
	}
	if residual := p.Revenue - segmentSum; significant(residual, p.Revenue) {
		g.link(name("other_segment"), name("revenue"), residual)
	}

	g.link(name("revenue"), name("cogs"), p.Revenue-p.GrossProfit)
	g.link(name("revenue"), name("gross_profit"), p.GrossProfit)

	opex := p.GrossProfit - p.OperatingIncome
	g.link(name("gross_profit"), name("opex"), opex)
	g.link(name("gross_profit"), name("operating_income"), p.OperatingIncome)

	g.link(name("opex"), name("rnd"), p.RnD)
	g.link(name("opex"), name("sganda"), p.SGnA)
	if residual := opex - p.RnD - p.SGnA; significant(residual, p.Revenue) {
		g.link(name("opex"), name("other_opex"), residual)
	}

	g.link(name("operating_income"), name("tax_other"), p.OperatingIncome-p.NetIncome)
	g.link(name("operating_income"), name("net_income"), p.NetIncome)

	return g.build()
}

type graphBuilder struct {
	links []Link
	notes []string
}

// significant reports whether a residual is worth its own node: anything under
// 0.5% of revenue is rounding noise on a diagram (AD-13). A NaN residual or
// revenue compares false, which drops the node.
func significant(residual, revenue float64) bool {
	return math.Abs(residual) >= math.Abs(revenue)*residualShare
}

func (g *graphBuilder) link(source, target string, value float64) {
	if value < 0 {
		g.notes = append(g.notes,
			fmt.Sprintf("%s→%s 为负（%.0f），按 0 宽绘制，不计入守恒", source, target, value))
		value = 0
	}
	g.links = append(g.links, Link{Source: source, Target: target, Value: value})
}

func (g *graphBuilder) build() Graph {
	in := make(map[string]float64)
	out := make(map[string]float64)
	var order []string
	seen := make(map[string]bool)
	add := func(n string) {
		if !seen[n] {
			seen[n] = true
			order = append(order, n)
		}
	}
	for _, l := range g.links {
		add(l.Source)
		add(l.Target)
		out[l.Source] += l.Value
		in[l.Target] += l.Value
	}

	nodes := make([]Node, 0, len(order))
	for _, n := range order {
		nodes = append(nodes, Node{Name: n, Value: math.Max(in[n], out[n])})
	}
	return Graph{Nodes: nodes, Links: g.links, Notes: g.notes}
}
