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

// splitEvent marks a detected stock split: 从 effFiled 起(首个体现拆股后基准的申报)
// 每股值缩小为原来的 1/ratio,股本数放大为 ratio 倍。
type splitEvent struct {
	effFiled time.Time
	ratio    float64
}

// splitRatioTolerance:跨 filed 比例与最近整数的相对偏差在此内才认定为拆股(重述噪声/
// 四舍五入不误判);splitMinVotes 要求至少这么多个 period_end 都观测到同一比例才认定为
// 拆股——真实拆股会重述所有历史期,亏损年个别科目的噪声比例只在单个 period_end 出现。
const (
	splitRatioTolerance = 0.05
	splitMinVotes       = 2
)

// splitRatios 是真实股票拆股的常见比例白名单。只在 2024 被重述、跳过 2021 中间重述的老
// 期间,其相邻跨 filed 比例会是「组合比」(如 4:1×10:1=40),并非一次独立拆股;若把组合比
// 也当拆股会与 4、10 叠乘导致早期值被严重过度归一。故只认白名单内的原子比例,时间线仍由
// 各原子拆股(4、10)自身的投票建立,并按 filed 正确累积应用到全部条目。
var splitRatios = map[float64]bool{
	2: true, 3: true, 4: true, 5: true, 6: true, 7: true, 8: true, 10: true, 15: true, 20: true,
}

// eachRestatementRatio 遍历各 period_end 组内按 filed 升序的相邻条目对,对「值比为近整数
// (>=2,偏差<=splitRatioTolerance)」的对回调 fn(该比例, 新基准条目)。inverse=true 翻转比例
// 方向(股本数拆股后变大,用 newer/older),供股本复用同一扫描。
func eachRestatementRatio(byEnd map[string][]rawFact, inverse bool, fn func(R float64, newer rawFact)) {
	for _, grp := range byEnd {
		sort.Slice(grp, func(i, j int) bool { return grp[i].Filed < grp[j].Filed })
		for i := 1; i < len(grp); i++ {
			older, newer := grp[i-1], grp[i]
			if older.Val == 0 || newer.Val == 0 {
				continue
			}
			r := older.Val / newer.Val
			if inverse {
				r = newer.Val / older.Val
			}
			R := math.Round(r)
			if R < 2 || math.Abs(r-R)/R > splitRatioTolerance {
				continue
			}
			fn(R, newer)
		}
	}
}

// ratioVotes 统计各 period_end 组内「相邻 filed 值比 ≈ 白名单原子拆股比例」的票数。
func ratioVotes(byEnd map[string][]rawFact) map[float64]int {
	votes := map[float64]int{}
	eachRestatementRatio(byEnd, false, func(R float64, _ rawFact) {
		if splitRatios[R] {
			votes[R]++
		}
	})
	return votes
}

// refineEffDates 对每个已验证真实比例 R,从条目里「比例 ≈ R」的相邻 filed 对取最早的新基准
// filed 作为生效日。inverse=false 用于每股值(拆股后变小,older/newer≈R);inverse=true 用于
// 股本数(拆股后变大,newer/older≈R)。股本是数十亿大整数,季度粒度也无舍入噪声,能把生效日
// 收得比每股值(小额季度舍入使 ratio 失真、只能靠年报)更紧,避免误伤拆股后原生季度。
func refineEffDates(byEnd map[string][]rawFact, valid map[float64]bool, inverse bool, effByRatio map[float64]time.Time) {
	eachRestatementRatio(byEnd, inverse, func(R float64, newer rawFact) {
		if !valid[R] {
			return
		}
		eff, err := time.Parse("2006-01-02", newer.Filed)
		if err != nil {
			return
		}
		if cur, ok := effByRatio[R]; !ok || eff.Before(cur) {
			effByRatio[R] = eff
		}
	})
}

// detectSplits 从每股科目(EPSDiluted)与股本数(DilutedShares)的跨 filed 重述推断拆股时间线,
// 分两步以兼顾「比例干净」与「生效日精确」:
//  1. 比例只从 EPS **年度(FY)条目**投票检测——年度每股值量级大、无低值四舍五入噪声,只会给出
//     真实拆股比(如 4、10);季度低值(如 0.06→0.02=3×)的舍入噪声不会污染。需 >=splitMinVotes
//     个不同财年年度确认,进一步排除偶发。
//  2. 生效日从 EPS 与**股本数**里「比例 ∈ 已验证真实比例」的最早新基准 filed 定位——股本是大整数
//     无舍入噪声,季度重述能给出紧边界(拆股实际生效附近),避免用年报 filed(偏晚)误伤拆股后
//     原生季度。
//
// 早期只在后期被一次性重述(跳过中间拆股)的期间,其相邻比是组合比(如 40),不在白名单、也不是
// 已验证真实比例,被忽略;但时间线由各原子拆股(4、10)自身确立,按 filed 累积应用仍正确归一。
func detectSplits(eps, shares []rawFact) []splitEvent {
	annualByEnd := map[string][]rawFact{}
	epsByEnd := map[string][]rawFact{}
	for _, f := range eps {
		epsByEnd[f.End] = append(epsByEnd[f.End], f)
		if isAnnual(f) {
			annualByEnd[f.End] = append(annualByEnd[f.End], f)
		}
	}
	valid := map[float64]bool{}
	for R, v := range ratioVotes(annualByEnd) {
		if v >= splitMinVotes {
			valid[R] = true
		}
	}
	if len(valid) == 0 {
		return nil
	}
	sharesByEnd := map[string][]rawFact{}
	for _, f := range shares {
		sharesByEnd[f.End] = append(sharesByEnd[f.End], f)
	}
	effByRatio := map[float64]time.Time{}
	refineEffDates(epsByEnd, valid, false, effByRatio)
	refineEffDates(sharesByEnd, valid, true, effByRatio)

	var events []splitEvent
	for R, eff := range effByRatio {
		events = append(events, splitEvent{eff, R})
	}
	return events
}

// splitFactor 返回 filed 这条记录到「最新基准」需累乘的拆股比例(生效日晚于 filed 的拆股
// 全部计入):每股值 ÷factor、股本数 ×factor 即归一到最新基准。filed 不可解析 → 1(不动)。
func splitFactor(events []splitEvent, filedStr string) float64 {
	filed, err := time.Parse("2006-01-02", filedStr)
	if err != nil {
		return 1
	}
	f := 1.0
	for _, e := range events {
		if filed.Before(e.effFiled) {
			f *= e.ratio
		}
	}
	return f
}

// normalizeSplits 把某科目各条目按拆股时间线归一到最新基准的**副本**(不改原切片)。
// invert=false 每股值 ÷factor;invert=true 股本数 ×factor。
func normalizeSplits(facts []rawFact, events []splitEvent, invert bool) []rawFact {
	if len(events) == 0 {
		return facts
	}
	out := make([]rawFact, len(facts))
	copy(out, facts)
	for i := range out {
		f := splitFactor(events, out[i].Filed)
		if f == 1 {
			continue
		}
		if invert {
			out[i].Val *= f
		} else {
			out[i].Val /= f
		}
	}
	return out
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

	// 拆股归一化:从每股科目 EPSDiluted 推断拆股时间线,把每股值(÷)与股本数(×)统一到
	// 最新基准,再喂入季度化。避免年度(拆股后重述)−单季(拆股前基准)得垃圾派生 Q4(TASK-008)。
	// Revenue/NetIncome/Equity 是绝对值,不随拆股变动,不归一。
	epsFacts := unitsOf("EarningsPerShareDiluted")
	sharesFacts := unitsOf("WeightedAverageNumberOfDilutedSharesOutstanding")
	splits := detectSplits(epsFacts, sharesFacts)

	durationMetric(normalizeSplits(epsFacts, splits, false), func(q *QuarterlyFact, v float64) { q.EPSDiluted = v })
	durationMetric(firstQuarterlyTag(revenueTags...), func(q *QuarterlyFact, v float64) { q.Revenue = v })
	durationMetric(unitsOf("NetIncomeLoss"), func(q *QuarterlyFact, v float64) { q.NetIncome = v })
	// 股本为时点/加权平均量(非流量),Q4=FY−ΣQ 减法无意义(FY≈单季 → 得负值),故只落单季,
	// Q4 无独立单季申报 → 该季 DilutedShares 留 NaN。股本随拆股反向放大(×比例)。
	sharesSingles, _ := applyDuration(normalizeSplits(sharesFacts, splits, true))
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
