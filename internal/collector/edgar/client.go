// Package edgar fetches SEC EDGAR companyfacts (XBRL) and normalizes them
// into quarterly facts with real filing dates (设计文档 D1/D2, §5.1 防前视).
package edgar

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"time"
)

const defaultBaseURL = "https://data.sec.gov"

// ErrNotUSGAAP marks IFRS/foreign filers (20-F) — 首批不支持,M3+ 扩展。
var ErrNotUSGAAP = errors.New("edgar: no us-gaap facts (IFRS filer unsupported)")

// revenueTags in priority order (公司间 tag 使用不一,设计风险表已知项).
var revenueTags = []string{
	"RevenueFromContractWithCustomerExcludingAssessedTax",
	"Revenues",
	"SalesRevenueNet",
}

type Client struct {
	client    *http.Client
	baseURL   string
	userAgent string
}

func New(userAgent string) *Client { return NewWithBaseURL(userAgent, defaultBaseURL) }

func NewWithBaseURL(userAgent, baseURL string) *Client {
	return &Client{
		client:    &http.Client{Timeout: 60 * time.Second},
		baseURL:   baseURL,
		userAgent: userAgent,
	}
}

// QuarterlyFact is one normalized fiscal quarter. NaN marks missing metrics.
type QuarterlyFact struct {
	FiscalPeriod  string
	PeriodEnd     time.Time
	FilingDate    time.Time
	EPSDiluted    float64
	Revenue       float64
	NetIncome     float64
	Equity        float64
	DilutedShares float64
}

// rawFact mirrors one entry of facts[ns][tag].units[unit].
type rawFact struct {
	Start string  `json:"start"`
	End   string  `json:"end"`
	Val   float64 `json:"val"`
	FY    int     `json:"fy"`
	FP    string  `json:"fp"`
	Form  string  `json:"form"`
	Filed string  `json:"filed"`
}

type factsDoc struct {
	Facts map[string]map[string]struct {
		Units map[string][]rawFact `json:"units"`
	} `json:"facts"`
}

// durationDays returns end-start in days; ok=false when either date is unparseable
// or the entry is instant (no start).
func durationDays(f rawFact) (days float64, ok bool) {
	start, e1 := time.Parse("2006-01-02", f.Start)
	end, e2 := time.Parse("2006-01-02", f.End)
	if e1 != nil || e2 != nil {
		return 0, false
	}
	return end.Sub(start).Hours() / 24, true
}

// isSingleQuarter reports whether f is a proper single fiscal quarter: fp∈{Q1,Q2,Q3}
// 且 duration 70~100 天。真实 10-Q 同一 (fy,fp) 还含累计条目(6mo/9mo),10-K 内还有
// fp=FY 的 90 天季度拆分——都不算单季。必须靠此判定在 (fy,fp) 去重「之前」先过滤,否则
// 累计/FY 条目可能胜出去重后又被 duration 丢弃,导致整季消失(BUG-1/BUG-3)。
func isSingleQuarter(f rawFact) bool {
	if f.FP != "Q1" && f.FP != "Q2" && f.FP != "Q3" {
		return false
	}
	d, ok := durationDays(f)
	return ok && d >= 70 && d <= 100
}

// isAnnual reports whether f is a full-year entry (fp=FY 且 350~380 天),供 Q4 推导。
func isAnnual(f rawFact) bool {
	if f.FP != "FY" {
		return false
	}
	d, ok := durationDays(f)
	return ok && d >= 350 && d <= 380
}

// FetchCompanyFacts downloads and quarterizes the companyfacts of one CIK.
// Full refetch every call — EDGAR has no incremental API; upserts are
// idempotent downstream. Results are sorted by PeriodEnd ascending.
func (c *Client) FetchCompanyFacts(cik string) ([]QuarterlyFact, error) {
	url := fmt.Sprintf("%s/api/xbrl/companyfacts/CIK%010s.json", c.baseURL, cik)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	// SEC 要求 UA 携带可联系方式,缺失会被 403。
	req.Header.Set("User-Agent", c.userAgent)
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("edgar: request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("edgar: unexpected HTTP status %d for CIK %s", resp.StatusCode, cik)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var doc factsDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("edgar: decode companyfacts: %w", err)
	}
	gaap, ok := doc.Facts["us-gaap"]
	if !ok || len(gaap) == 0 {
		return nil, ErrNotUSGAAP
	}

	// quarters 以实际 period_end 为 key(而非 fy/fp)——EDGAR 的 fy/fp 是报告上下文,同一
	// (fy,fp) 可能对应不同实际季度;按 end 建键才能让各科目正确归并到同一真实季度。label 仅
	// 作 FiscalPeriod 展示标签(取首个写入者的)。
	quarters := map[string]*QuarterlyFact{} // key = period_end "2006-01-02"
	get := func(end, filed time.Time, label string) *QuarterlyFact {
		key := end.Format("2006-01-02")
		q, ok := quarters[key]
		if !ok {
			q = &QuarterlyFact{FiscalPeriod: label, PeriodEnd: end,
				EPSDiluted: math.NaN(), Revenue: math.NaN(), NetIncome: math.NaN(),
				Equity: math.NaN(), DilutedShares: math.NaN()}
			quarters[key] = q
		}
		if filed.After(q.FilingDate) {
			q.FilingDate = filed
		}
		return q
	}

	unitsOf := func(tag string) []rawFact {
		t, ok := gaap[tag]
		if !ok {
			return nil
		}
		for _, facts := range t.Units { // 单 unit tag,取第一个 unit
			return facts
		}
		return nil
	}
	hasQuarterly := func(facts []rawFact) bool {
		for _, f := range facts {
			if isSingleQuarter(f) {
				return true
			}
		}
		return false
	}
	// firstQuarterlyTag 选首个含可用单季(fp∈Q1/Q2/Q3)条目的 tag。某些公司首选 tag
	// (如 RevenueFromContract…)的单季只标 fp=FY 不可用,须回退到带正确季度标签的次选
	// tag(如 Revenues)(BUG-2)。都无可用单季 → 退回首个非空(至少保留年度条目)。
	firstQuarterlyTag := func(tags ...string) []rawFact {
		for _, tag := range tags {
			if f := unitsOf(tag); hasQuarterly(f) {
				return f
			}
		}
		for _, tag := range tags {
			if f := unitsOf(tag); len(f) != 0 {
				return f
			}
		}
		return nil
	}

	// qEntry / annEntry:按实际期间(而非 fy/fp)表示单季与年度条目。
	type qEntry struct {
		end, filed time.Time
		val        float64
		label      string // "%d%s" fy+fp,仅作 FiscalPeriod 展示标签
	}
	type annEntry struct {
		fy                int
		start, end, filed time.Time
		val               float64
	}
	// applyDuration 先按 duration 过滤出「单季(fp∈Q1/Q2/Q3,70~100天)/年度(fp=FY,350~380天)」,
	// 再按实际 period_end 去重(取 filed 最新)。过滤必须在去重之前——真实 10-Q 每季含单季+累计
	// (6mo/9mo)双条目,若先去重累计条目可能胜出后被丢弃(BUG-1)。按 end 去重(而非 fy/fp)可
	// 避免 EDGAR 报告上下文标签不可靠导致的丢季/错配。
	applyDuration := func(facts []rawFact) (singles []qEntry, annuals []annEntry) {
		singleByEnd := map[string]rawFact{} // period_end → filed 最新的单季条目
		annualByEnd := map[string]rawFact{} // period_end → filed 最新的年度条目
		keepLatest := func(m map[string]rawFact, f rawFact) {
			if p, ok := m[f.End]; !ok || f.Filed > p.Filed {
				m[f.End] = f
			}
		}
		for _, f := range facts {
			switch {
			case isSingleQuarter(f):
				keepLatest(singleByEnd, f)
			case isAnnual(f):
				keepLatest(annualByEnd, f)
			}
		}
		for _, f := range singleByEnd {
			end, e1 := time.Parse("2006-01-02", f.End)
			filed, e2 := time.Parse("2006-01-02", f.Filed)
			if e1 != nil || e2 != nil {
				continue
			}
			singles = append(singles, qEntry{end, filed, f.Val, fmt.Sprintf("%d%s", f.FY, f.FP)})
		}
		for _, f := range annualByEnd {
			start, e0 := time.Parse("2006-01-02", f.Start)
			end, e1 := time.Parse("2006-01-02", f.End)
			filed, e2 := time.Parse("2006-01-02", f.Filed)
			if e0 != nil || e1 != nil || e2 != nil {
				continue
			}
			annuals = append(annuals, annEntry{f.FY, start, end, filed, f.Val})
		}
		return singles, annuals
	}
	emitSingles := func(singles []qEntry, set func(q *QuarterlyFact, v float64)) {
		for _, s := range singles {
			set(get(s.end, s.filed, s.label), s.val)
		}
	}
	// 流量型(EPS/Revenue/NetIncome):落单季 + 按实际期间匹配推导 Q4。对每个年度条目取 period_end
	// 落在其区间 (start,end] 内的单季,恰好 3 个才推导 Q4=年度−Σ三季(period_end/filing 用年度条目)。
	// 用实际期间匹配可正确归集同财年三季,既恢复 Q4 又避免跨财年错配得负值。
	durationMetric := func(facts []rawFact, set func(q *QuarterlyFact, v float64)) {
		singles, annuals := applyDuration(facts)
		emitSingles(singles, set)
		for _, a := range annuals {
			var sum float64
			n := 0
			for _, s := range singles {
				if s.end.After(a.start) && !s.end.After(a.end) {
					sum += s.val
					n++
				}
			}
			if n == 3 {
				// Q4 = FY − (Q1+Q2+Q3)。EPS 用同式为近似(股本变动误差通常 <1%)。
				set(get(a.end, a.filed, fmt.Sprintf("%dQ4", a.fy)), a.val-sum)
			}
		}
	}

	durationMetric(unitsOf("EarningsPerShareDiluted"), func(q *QuarterlyFact, v float64) { q.EPSDiluted = v })
	durationMetric(firstQuarterlyTag(revenueTags...), func(q *QuarterlyFact, v float64) { q.Revenue = v })
	durationMetric(unitsOf("NetIncomeLoss"), func(q *QuarterlyFact, v float64) { q.NetIncome = v })
	// 股本为时点/加权平均量(非流量),Q4=FY−ΣQ 减法无意义(FY≈单季 → 得负值),故只落单季,
	// Q4 无独立单季申报 → 该季 DilutedShares 留 NaN。
	sharesSingles, _ := applyDuration(unitsOf("WeightedAverageNumberOfDilutedSharesOutstanding"))
	emitSingles(sharesSingles, func(q *QuarterlyFact, v float64) { q.DilutedShares = v })

	// instant 型(Equity):按 end 匹配同 period_end 的季度。
	for _, f := range unitsOf("StockholdersEquity") {
		end, err := time.Parse("2006-01-02", f.End)
		if err != nil {
			continue
		}
		for _, q := range quarters {
			if q.PeriodEnd.Equal(end) {
				q.Equity = f.Val
			}
		}
	}

	out := make([]QuarterlyFact, 0, len(quarters))
	for _, q := range quarters {
		if q.PeriodEnd.IsZero() || q.FilingDate.IsZero() {
			continue
		}
		out = append(out, *q)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PeriodEnd.Before(out[j].PeriodEnd) })
	return out, nil
}
