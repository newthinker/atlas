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

	quarters := map[string]*QuarterlyFact{} // key "2026Q1"
	get := func(key string, end, filed time.Time) *QuarterlyFact {
		q, ok := quarters[key]
		if !ok {
			q = &QuarterlyFact{FiscalPeriod: key,
				EPSDiluted: math.NaN(), Revenue: math.NaN(), NetIncome: math.NaN(),
				Equity: math.NaN(), DilutedShares: math.NaN()}
			quarters[key] = q
		}
		if end.After(q.PeriodEnd) {
			q.PeriodEnd = end
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
	firstTag := func(tags ...string) []rawFact {
		for _, tag := range tags {
			if f := unitsOf(tag); len(f) != 0 {
				return f
			}
		}
		return nil
	}

	// duration 型科目:单季直取,FY 存起来供 Q4 推导。
	type fyAgg struct {
		fyVal   float64
		qSum    float64
		qCount  int
		fyEnd   time.Time
		fyFiled time.Time
	}
	applyDuration := func(facts []rawFact, set func(q *QuarterlyFact, v float64)) map[int]*fyAgg {
		aggs := map[int]*fyAgg{}
		latest := map[string]rawFact{} // (fy,fp) → filed 最新的条目(修正重报)
		for _, f := range facts {
			key := fmt.Sprintf("%d%s", f.FY, f.FP)
			if prev, ok := latest[key]; !ok || f.Filed > prev.Filed {
				latest[key] = f
			}
		}
		for _, f := range latest {
			start, e1 := time.Parse("2006-01-02", f.Start)
			end, e2 := time.Parse("2006-01-02", f.End)
			filed, e3 := time.Parse("2006-01-02", f.Filed)
			if e1 != nil || e2 != nil || e3 != nil {
				continue
			}
			days := end.Sub(start).Hours() / 24
			agg := aggs[f.FY]
			if agg == nil {
				agg = &fyAgg{}
				aggs[f.FY] = agg
			}
			switch {
			case f.FP != "FY" && days >= 70 && days <= 100: // 单季
				set(get(fmt.Sprintf("%d%s", f.FY, f.FP), end, filed), f.Val)
				agg.qSum += f.Val
				agg.qCount++
			case f.FP == "FY" && days >= 350 && days <= 380: // 年度
				agg.fyVal, agg.fyEnd, agg.fyFiled = f.Val, end, filed
			}
		}
		return aggs
	}
	deriveQ4 := func(aggs map[int]*fyAgg, set func(q *QuarterlyFact, v float64)) {
		for fy, a := range aggs {
			if a.qCount == 3 && !a.fyEnd.IsZero() {
				// Q4 = FY − (Q1+Q2+Q3)。EPS 用同式为近似(股本变动误差通常 <1%)。
				set(get(fmt.Sprintf("%dQ4", fy), a.fyEnd, a.fyFiled), a.fyVal-a.qSum)
			}
		}
	}
	durationMetric := func(facts []rawFact, set func(q *QuarterlyFact, v float64)) {
		deriveQ4(applyDuration(facts, set), set)
	}

	durationMetric(unitsOf("EarningsPerShareDiluted"), func(q *QuarterlyFact, v float64) { q.EPSDiluted = v })
	durationMetric(firstTag(revenueTags...), func(q *QuarterlyFact, v float64) { q.Revenue = v })
	durationMetric(unitsOf("NetIncomeLoss"), func(q *QuarterlyFact, v float64) { q.NetIncome = v })
	durationMetric(unitsOf("WeightedAverageNumberOfDilutedSharesOutstanding"), func(q *QuarterlyFact, v float64) { q.DilutedShares = v })

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
