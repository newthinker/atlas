// Package edgar fetches SEC EDGAR companyfacts (XBRL) and normalizes them
// into quarterly facts with real filing dates (设计文档 D1/D2, §5.1 防前视).
package edgar

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
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

// 以下 tag 链同样按优先级排列,首选 tag 缺失(或无可用单季条目)时依次回退。全部提为包级
// 变量:除 FetchCompanyFacts 外,分部解析(segments.go)也要按同一口径识别科目。
// 生产实测缺口驱动(COST/V/CRM/WMT/AVGO 曾整列 NaN),回退链只在首选缺失时生效。

var epsTags = []string{"EarningsPerShareDiluted", "EarningsPerShareBasicAndDiluted"}

// sharesTags 的回退项是 **basic** 股本数:不含期权/可转债的稀释效应,口径上略小于 diluted
// (常见偏差 <2%)。宁可用这个近似值也不留 NaN —— 分母缺失会让下游 BVPS/RPS/PE 整条序列消失。
var sharesTags = []string{
	"WeightedAverageNumberOfDilutedSharesOutstanding",
	"WeightedAverageNumberOfSharesOutstandingBasic",
}

// equityTags 的回退项含少数股东权益(AVGO 等只报这一口径),数值略大于母公司股东权益。
var equityTags = []string{
	"StockholdersEquity",
	"StockholdersEquityIncludingPortionAttributableToNoncontrollingInterest",
}

// 主干流科目(收入→毛利→营利→净利)。costTags 与 sgnaSplitTags 不进 QuarterlyFact,
// 只作缺失时的推导输入。
var (
	grossProfitTags = []string{"GrossProfit"}
	costTags        = []string{"CostOfRevenue", "CostOfGoodsAndServicesSold"}
	rndTags         = []string{"ResearchAndDevelopmentExpense"}
	sgnaTags        = []string{"SellingGeneralAndAdministrativeExpense"}
	opIncomeTags    = []string{"OperatingIncomeLoss"}
	taxTags         = []string{"IncomeTaxExpenseBenefit"}
)

// sgnaSplitTags 与上面各变量不同:它**不是回退链而是求和项** —— 部分公司不报合并的 SG&A,
// 只分列销售费用与管理费用。必须两项在同一季都有值才求和(见 FetchCompanyFacts 推导规则 3)。
var sgnaSplitTags = []string{"SellingAndMarketingExpense", "GeneralAndAdministrativeExpense"}

type Client struct {
	client    *http.Client
	baseURL   string
	userAgent string
	// archivesURL 是报告实例文档所在 host,与 baseURL(companyfacts/submissions 的 data host)
	// **不是同一个** —— 生产分别为 data.sec.gov 与 www.sec.gov。
	archivesURL string
	// maxInstanceBytes 是单份实例文档的读取上限(AD-11)。做成字段而非常量,只为让测试能压低
	// 阈值验证截断行为 —— 造 >64MB 的响应在 CI 上不现实。
	maxInstanceBytes int64
}

func New(userAgent string) *Client {
	return NewWithBaseURLs(userAgent, defaultBaseURL, defaultArchivesURL)
}

// NewWithBaseURL 注入单一 host。archivesURL 一并指向该 host:测试构造的 client 若把 archives
// 留成真实 www.sec.gov,一旦调用 FetchSegmentRevenue 就会打到线上。
func NewWithBaseURL(userAgent, baseURL string) *Client {
	return NewWithBaseURLs(userAgent, baseURL, baseURL)
}

// NewWithBaseURLs 分别注入 data host 与 archives host。
func NewWithBaseURLs(userAgent, baseURL, archivesURL string) *Client {
	return &Client{
		client:           &http.Client{Timeout: 60 * time.Second},
		baseURL:          baseURL,
		userAgent:        userAgent,
		archivesURL:      archivesURL,
		maxInstanceBytes: defaultMaxInstanceBytes,
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
	// M3 主干流科目(收入→毛利→营利→净利),NaN = 缺失或无法推导。
	GrossProfit     float64
	RnD             float64
	SGnA            float64
	OperatingIncome float64
	IncomeTax       float64
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

// tagFacts is facts[ns][tag]; named so helpers can take it as a parameter.
type tagFacts struct {
	Units map[string][]rawFact `json:"units"`
}

type factsDoc struct {
	Facts map[string]map[string]tagFacts `json:"facts"`
}

// fiscalYearEndMonth 从年度条目(isAnnual,350~380 天)的 period_end 推断公司的财年结束月份。
// 取**众数**而非首个:52/53 周财年的结束日会在月内漂移(实测 NVDA 1/25~1/28、AAPL 9/27~9/30),
// 个别年份甚至可能跨月,众数对这种漂移稳健。ok=false 表示没有任何年度条目,无从推断。
func fiscalYearEndMonth(gaap map[string]tagFacts) (month int, ok bool) {
	votes := map[int]int{}
	for _, t := range gaap {
		for _, facts := range t.Units {
			for _, f := range facts {
				if !isAnnual(f) {
					continue
				}
				if end, err := time.Parse(dateLayout, f.End); err == nil {
					_, m := anchorYearMonth(end)
					votes[m]++
				}
			}
		}
	}
	best, bestN := 0, 0
	for m, n := range votes {
		// 票数相同时取较小月份,避免结果随 map 迭代序抖动。
		if n > bestN || (n == bestN && m < best) {
			best, bestN = m, n
		}
	}
	return best, bestN > 0
}

// fiscalLabel 由 period_end 与财年结束月份推导 "2026Q1" 形式的展示标签。
//
// 它是 **period_end 的纯函数** —— 这正是它替代 EDGAR fy/fp 的理由:后者是**报告的上下文
// 标签**,而 applyDuration 按 period_end 去重取 filed 最新者,于是较早的期间会继承较晚那份
// 报告的 fy/fp。实测真实 MSFT 因此产生 4 组标签冲突(最严重的 2025Q4 有 3 个不同季度共用),
// 下游 sankey 又把该标签当聚合分组键,导致财年营收被跨季相加(FY2025 报 377.752B,真值 281.7B)。
//
// 财年归属**含端点**:财年结束当天属于**上一**财年的 Q4,而非新财年的 Q1。少一个等号会让所有
// Q4 整体后移一年,**而标签依然互不重复** —— 只断言唯一性的测试查不出来,故测试锚定具体值。
func fiscalLabel(end time.Time, fyEndMonth int) string {
	fy, m := anchorYearMonth(end)
	if m > fyEndMonth {
		fy++
	}
	// 财年自 fyEndMonth 的次月开始;月差取模后除以 3 即季序(0..3)。
	offset := ((m-fyEndMonth-1)%12 + 12) % 12
	return fmt.Sprintf("%dQ%d", fy, offset/3+1)
}

// anchorYearMonth 返回期间**实际归属**的年月。52/53 周财年的期末会在月界附近漂移,可能落到
// 次月头几天(实测形态:NVDA 1/25~1/28、AAPL 9/27~9/30,而一个覆盖 2~4 月的季度可能记为
// 结束于 5/1)。直接取 end.Month() 会把这种期间算到下一个季度去 —— 实测 q4guard fixture 里
// 2022-05-01(覆盖 2~4 月)与 2022-07-31(覆盖 5~7 月)会因此**共用同一个标签**,
// 正是本任务要消除的那类冲突。故月初几天一律回归上一个月,期末落在 1 月初时年份同步减一。
//
// 阈值取 15:锚定月末的财报日历漂移至多数天,不会到半月;而真正在月中结束的财季极为罕见。
func anchorYearMonth(end time.Time) (year, month int) {
	y, m := end.Year(), int(end.Month())
	if end.Day() > 15 {
		return y, m
	}
	if m == 1 {
		return y - 1, 12
	}
	return y, m - 1
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

// selectUnit 在一个 tag 的多个 unit 中**确定性地选中一个**(不合并),返回其条目切片。
//
// 「一个 tag 只有一个 unit」曾是本文件的隐含假设,实测不成立:COST/WMT/CRM 的
// EarningsPerShareDiluted 都有 `pure` + `USD/shares` 两个 unit —— `pure` 是 SEC 早期的
// 无量纲单位,只剩零星陈旧重述(COST 3 条 / WMT 1 条可用),`USD/shares` 才承载全部年度数据
// (144~155 条可用)。原实现 `for _, facts := range t.Units { return facts }` 取「第一个」,
// 而 Go 的 map 迭代序随机 ⇒ 有约一半概率取到 pure,年度 EPS 随之丢失,Q4 只能靠 FY−ΣQ 却
// 无输入 → Q4 EPS 全 NaN → TTM 窗口(必含一个 Q4)全部作废 → PE 序列为空。
//
// **选单个而不合并**,三条理由:
//  1. 保持既有「口径不混用」不变量 —— tag 回退链也是选中单一 tag(见 firstQuarterlyTag)。
//  2. 合并会污染 detectSplits 的相邻 filed 比例投票(它假设输入是单一科目的完整历史)。
//  3. 决定性的一条:COST 的 pure 条目 filed=2010-10-18,**晚于**对应期间的原始申报,
//     合并后会在 applyDuration 的 keepLatest(取 filed 最新)去重中胜出,用陈旧重述值
//     覆盖正确值 —— 比丢数据更糟,是静默的错数据。
func selectUnit(units map[string][]rawFact) []rawFact {
	if len(units) == 0 {
		return nil
	}
	names := make([]string, 0, len(units))
	for name := range units {
		names = append(names, name)
	}
	return units[bestUnitName(units, names)]
}

// bestUnitName 在 names 给出的候选 unit 中选优,打分依次为:
// 可用条数(isSingleQuarter 或 isAnnual,即真正会被 applyDuration 采纳的条目)降序 →
// 总条数降序 → unit 名升序。前者是数据驱动的(比写死 "USD/shares" 之类的名字优先级健壮),
// 后两级只为消除并列,保证全序。
//
// 返回值与 names 的**排列无关** —— 打分是全序(unit 名互不相同,末级必然分出胜负),
// 对全序取最大值与折叠顺序无关。这一点是确定性的**结构性保证**,而非「靠 map 恰好稳定」:
// 把候选顺序显式交给调用方,测试才能穷举全部排列断言同一结果(见
// TestSelectUnitIsPermutationInvariant);若把 map 迭代内联在此处,任何测试都只能概率性地
// 捕获顺序依赖。
//
// 注:可用性判定必须与 applyDuration 的过滤口径保持一致,两处一起改。
func bestUnitName(units map[string][]rawFact, names []string) string {
	type candidate struct {
		name          string
		usable, total int
	}
	cands := make([]candidate, 0, len(names))
	for _, name := range names {
		facts := units[name]
		usable := 0
		for _, f := range facts {
			if isSingleQuarter(f) || isAnnual(f) {
				usable++
			}
		}
		cands = append(cands, candidate{name, usable, len(facts)})
	}
	if len(cands) == 0 {
		return ""
	}
	sort.Slice(cands, func(i, j int) bool {
		a, b := cands[i], cands[j]
		if a.usable != b.usable {
			return a.usable > b.usable
		}
		if a.total != b.total {
			return a.total > b.total
		}
		return a.name < b.name
	})
	return cands[0].name
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
	// splitClusterGap:同一比例的生效日观测相隔超过此值即视为**不同次**拆股。单次拆股的
	// 观测(各 period_end 的首个拆股后重述 + 股本重述)通常落在 ~1 年内;公司先后两次同比例
	// 拆股一般相隔数年。
	splitClusterGap = 365 * 24 * time.Hour
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

// collectEffObs 把已验证真实比例 R 的「新基准 filed」观测收集进 obsByRatio[R]。inverse=false
// 用于每股值(拆股后变小,older/newer≈R);inverse=true 用于股本数(拆股后变大,newer/older≈R)。
func collectEffObs(byEnd map[string][]rawFact, valid map[float64]bool, inverse bool, obsByRatio map[float64][]time.Time) {
	eachRestatementRatio(byEnd, inverse, func(R float64, newer rawFact) {
		if !valid[R] {
			return
		}
		if eff, err := time.Parse("2006-01-02", newer.Filed); err == nil {
			obsByRatio[R] = append(obsByRatio[R], eff)
		}
	})
}

// clusterEvents 把每个比例 R 的生效日观测按时间聚簇为独立事件——同一次拆股会被多个 period_end
// 在相近时间观测到(合成一个事件,生效日取簇内最早);相隔 >splitClusterGap 的观测属**不同次**
// 拆股(如公司先后两次 2:1),各成一个事件。这样同比例多次拆股不会被按 ratio 合并丢失
// (QA WARNING-1)。
func clusterEvents(obsByRatio map[float64][]time.Time) []splitEvent {
	var events []splitEvent
	for R, dates := range obsByRatio {
		sort.Slice(dates, func(i, j int) bool { return dates[i].Before(dates[j]) })
		start, prev := dates[0], dates[0]
		for _, d := range dates[1:] {
			if d.Sub(prev) > splitClusterGap {
				events = append(events, splitEvent{start, R})
				start = d
			}
			prev = d
		}
		events = append(events, splitEvent{start, R})
	}
	return events
}

// detectSplits 从每股科目(EPSDiluted)与股本数(DilutedShares)的跨 filed 重述推断拆股时间线,
// 分两步以兼顾「比例干净」与「生效日精确」:
//  1. 比例只从 EPS **年度(FY)条目**投票检测——年度每股值量级大、无低值四舍五入噪声,只会给出
//     真实拆股比(如 4、10);季度低值(如 0.06→0.02=3×)的舍入噪声不会污染。需 >=splitMinVotes
//     个不同财年年度确认,进一步排除偶发。
//  2. 生效日从**年度 EPS 与股本数**里「比例 ∈ 已验证真实比例」的新基准 filed 观测聚簇定位——
//     两者量级大、无低值舍入噪声(季度低值 EPS 的舍入会造出偶发同比例假观测,故 eff 不扫季度
//     EPS);股本季度粒度亦干净,能把生效日收得紧(拆股实际生效附近),避免用年报 filed(偏晚)
//     误伤拆股后原生季度。同比例多次拆股按生效日聚簇为多个事件(见 clusterEvents)。
//
// 早期只在后期被一次性重述(跳过中间拆股)的期间,其相邻比是组合比(如 40),不在白名单、也不是
// 已验证真实比例,被忽略;但时间线由各原子拆股(4、10)自身确立,按 filed 累积应用仍正确归一。
func detectSplits(eps, shares []rawFact) []splitEvent {
	annualByEnd := map[string][]rawFact{}
	for _, f := range eps {
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
	obsByRatio := map[float64][]time.Time{}
	collectEffObs(annualByEnd, valid, false, obsByRatio)
	collectEffObs(sharesByEnd, valid, true, obsByRatio)
	return clusterEvents(obsByRatio)
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

	// FiscalPeriod 一律由 period_end 推导(见 fiscalLabel)。降级:没有任何年度条目时推不出
	// 财年结束月份,回退 EDGAR 原始 fy/fp —— 那是有冲突风险的口径,故必须记录、不得静默。
	fyEndMonth, hasFYEnd := fiscalYearEndMonth(gaap)
	if !hasFYEnd {
		log.Printf("edgar: CIK %s has no annual entries; falling back to EDGAR fy/fp labels, which may collide across quarters", cik)
	}
	label := func(end time.Time, rawFY int, rawFP string) string {
		if !hasFYEnd {
			return fmt.Sprintf("%d%s", rawFY, rawFP)
		}
		return fiscalLabel(end, fyEndMonth)
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
				Equity: math.NaN(), DilutedShares: math.NaN(),
				GrossProfit: math.NaN(), RnD: math.NaN(), SGnA: math.NaN(),
				OperatingIncome: math.NaN(), IncomeTax: math.NaN()}
			quarters[key] = q
		}
		if filed.After(q.FilingDate) {
			q.FilingDate = filed
		}
		return q
	}

	// 多 unit tag 由 selectUnit 确定性地选中一个(不合并),见其文档注释。
	unitsOf := func(tag string) []rawFact {
		t, ok := gaap[tag]
		if !ok {
			return nil
		}
		return selectUnit(t.Units)
	}
	hasQuarterly := func(facts []rawFact) bool {
		for _, f := range facts {
			if isSingleQuarter(f) {
				return true
			}
		}
		return false
	}
	// firstTag 选首个非空 tag。instant 型科目(equity)没有「含可用单季」的概念,只按优先级
	// 取第一个有数据的 tag —— 首选存在时回退 tag 一律不参与,口径不混用。
	firstTag := func(tags ...string) []rawFact {
		for _, tag := range tags {
			if f := unitsOf(tag); len(f) != 0 {
				return f
			}
		}
		return nil
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
		return firstTag(tags...)
	}

	// qEntry / annEntry:按实际期间(而非 fy/fp)表示单季与年度条目。
	type qEntry struct {
		end, filed time.Time
		val        float64
		label      string // FiscalPeriod 展示标签,由 period_end 推导(见 fiscalLabel)
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
			singles = append(singles, qEntry{end, filed, f.Val, label(end, f.FY, f.FP)})
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
	// emitEntries 把展开后的条目写入对应季度(季度不存在则由 get 创建)。
	emitEntries := func(entries []qEntry, set func(q *QuarterlyFact, v float64)) {
		for _, e := range entries {
			set(get(e.end, e.filed, e.label), e.val)
		}
	}
	// durationEntries 把流量型科目展开为「各单季 + 按实际期间匹配推导的 Q4」。对每个年度条目取
	// period_end 落在其区间 (start,end] 内的单季,恰好 3 个才推导 Q4=年度−Σ三季(period_end/filing
	// 用年度条目)。用实际期间匹配可正确归集同财年三季,既恢复 Q4 又避免跨财年错配得负值。
	durationEntries := func(facts []rawFact) []qEntry {
		singles, annuals := applyDuration(facts)
		out := make([]qEntry, len(singles), len(singles)+len(annuals))
		copy(out, singles)
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
				out = append(out, qEntry{a.end, a.filed, a.val - sum, label(a.end, a.fy, "Q4")})
			}
		}
		return out
	}
	// 流量型(EPS/Revenue/NetIncome/主干流五科目):把展开结果写入对应季度。
	durationMetric := func(facts []rawFact, set func(q *QuarterlyFact, v float64)) {
		emitEntries(durationEntries(facts), set)
	}
	// quarterValues 与 durationMetric 同口径,但只返回 period_end→值 的映射,**不**触碰 quarters。
	// 供 Cost / SG&A 分列这类「仅作推导输入」的辅助科目使用 —— 它们不是 QuarterlyFact 字段,
	// 若走 durationMetric 会经 get() 凭空造出只有辅助科目、其余全 NaN 的幽灵季度。
	quarterValues := func(facts []rawFact) map[string]float64 {
		m := map[string]float64{}
		for _, e := range durationEntries(facts) {
			m[e.end.Format("2006-01-02")] = e.val
		}
		return m
	}

	// 拆股归一化:从每股科目 EPSDiluted 推断拆股时间线,把每股值(÷)与股本数(×)统一到
	// 最新基准,再喂入季度化。避免年度(拆股后重述)−单季(拆股前基准)得垃圾派生 Q4(TASK-008)。
	// Revenue/NetIncome/Equity 是绝对值,不随拆股变动,不归一。
	// 注意 tag 链在此**先于** detectSplits 生效:链只选中一个 tag(不做跨 tag 合并),故拆股
	// 检测的输入仍是单一科目的完整历史,比例投票与生效日聚簇的口径不变(AD-18 golden 回归守护)。
	epsFacts := firstQuarterlyTag(epsTags...)
	sharesFacts := firstQuarterlyTag(sharesTags...)

	// companyfacts 只收录**无维度**事实。某些多类别股权结构的公司(实测 V/Visa)按股份
	// 类别分别披露 EPS,一条无维度 EPS 都没有 ⇒ companyfacts 里整个科目缺席。此时回退到
	// 报告实例文档按类别取(classeps.go)。**只在完全取不到 EPS 时进入**,因此 EPS 正常的
	// 公司连一次实例文档请求都不会发出,主路径行为逐字节不变。
	// 失败一律只记日志:V 目前走 engine fallback 的 degraded 是可接受状态,这条回退路径
	// 修不成功也不能让整个 refresh 失败。
	if len(epsFacts) == 0 {
		classEPS, classShares, err := c.fetchClassDimensionedEPS(cik, classOfStockAxis)
		switch {
		case err != nil:
			log.Printf("edgar: CIK %s class-dimensioned EPS fallback failed: %v", cik, err)
		case len(classEPS) == 0:
			log.Printf("edgar: CIK %s has no class-dimensioned EPS either; EPS stays missing", cik)
		default:
			epsFacts = classEPS
			// 同源同口径:类别 EPS 的分母必须是同一类别的股本。回退路径没取到股本时保留
			// companyfacts 的(总量口径)—— 此时它只参与 detectSplits,不参与 EPS 推算。
			if len(classShares) != 0 {
				sharesFacts = classShares
			}
		}
	}

	splits := detectSplits(epsFacts, sharesFacts)

	durationMetric(normalizeSplits(epsFacts, splits, false), func(q *QuarterlyFact, v float64) { q.EPSDiluted = v })
	durationMetric(firstQuarterlyTag(revenueTags...), func(q *QuarterlyFact, v float64) { q.Revenue = v })
	durationMetric(unitsOf("NetIncomeLoss"), func(q *QuarterlyFact, v float64) { q.NetIncome = v })
	durationMetric(firstQuarterlyTag(grossProfitTags...), func(q *QuarterlyFact, v float64) { q.GrossProfit = v })
	durationMetric(firstQuarterlyTag(rndTags...), func(q *QuarterlyFact, v float64) { q.RnD = v })
	durationMetric(firstQuarterlyTag(sgnaTags...), func(q *QuarterlyFact, v float64) { q.SGnA = v })
	durationMetric(firstQuarterlyTag(opIncomeTags...), func(q *QuarterlyFact, v float64) { q.OperatingIncome = v })
	durationMetric(firstQuarterlyTag(taxTags...), func(q *QuarterlyFact, v float64) { q.IncomeTax = v })

	// 推导输入(不落 QuarterlyFact):成本 与 SG&A 分列。
	costs := quarterValues(firstQuarterlyTag(costTags...))
	sgnaParts := make([]map[string]float64, len(sgnaSplitTags))
	for i, tag := range sgnaSplitTags {
		sgnaParts[i] = quarterValues(firstQuarterlyTag(tag))
	}
	// 股本为时点/加权平均量(非流量),Q4=FY−ΣQ 减法无意义(FY≈单季 → 得负值),故只落单季,
	// Q4 无独立单季申报 → 该季 DilutedShares 留 NaN。股本随拆股反向放大(×比例)。
	sharesSingles, _ := applyDuration(normalizeSplits(sharesFacts, splits, true))
	emitEntries(sharesSingles, func(q *QuarterlyFact, v float64) { q.DilutedShares = v })

	// instant 型(Equity):按 end 匹配同 period_end 的季度。同 end 多条时切片顺序最后一个赢
	// (既有行为,回退链只改「取哪个 tag」,不改这条选取规则)。
	for _, f := range firstTag(equityTags...) {
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

	// 季度化后的推导回填(顺序固定)。三条规则一律只在目标字段为 NaN 时生效 —— 已由 tag 直接
	// 取到值的季度绝不被推算值覆盖。
	for key, q := range quarters {
		// 1. EPS = 净利 / 稀释股本。拆股归一化已在 rawFact 层完成,此处直接相除。
		//    分母为 0 或缺失时**不推算**:产出 ±Inf 会让下游 encoding/json 直接报错(AD-14),
		//    且 Q4 的 DilutedShares 恒为 NaN,该季因此仍走 FY−ΣQ 而非推算。
		if math.IsNaN(q.EPSDiluted) && !math.IsNaN(q.NetIncome) &&
			!math.IsNaN(q.DilutedShares) && q.DilutedShares != 0 {
			q.EPSDiluted = q.NetIncome / q.DilutedShares
		}
		// 2. 毛利 = 收入 − 成本。推导得负值(成本倒挂)照实保留,不钳零 —— 钳零会把真实的
		//    负毛利伪装成盈亏平衡。
		if cost, ok := costs[key]; ok && math.IsNaN(q.GrossProfit) &&
			!math.IsNaN(q.Revenue) && !math.IsNaN(cost) {
			q.GrossProfit = q.Revenue - cost
		}
		// 3. SG&A = 销售费用 + 管理费用。必须**全部**分列在该季都有值才求和:只有单侧时求出的
		//    「半个和」比缺失更危险,会让 opex 结构被系统性低估且无从察觉。
		if math.IsNaN(q.SGnA) {
			sum, complete := 0.0, true
			for _, part := range sgnaParts {
				v, ok := part[key]
				if !ok || math.IsNaN(v) {
					complete = false
					break
				}
				sum += v
			}
			if complete {
				q.SGnA = sum
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
