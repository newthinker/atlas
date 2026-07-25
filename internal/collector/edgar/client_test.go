package edgar

import (
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Context Checkpoint: done_criteria → test mapping
//   functional[0] CIK 零填充 + UA 头                 → TestFetchCompanyFactsRequest
//   functional[1] 季度判定/Q4 推导/filed/instant     → TestFetchCompanyFactsQuarterization
//   functional[2] 修正重报去重取 filed 最新           → TestFetchCompanyFactsQuarterization (Q1=0.6, 非旧 0.5)
//   boundary[0]   某季科目缺失 → NaN                  → TestFetchCompanyFactsQuarterization (Q2 shares)
//   boundary[1]   Revenue tag 回退分支                → TestFetchCompanyFactsRevenueFallback
//   error[0]      非 us-gaap → ErrNotUSGAAP           → TestFetchCompanyFactsIFRS
//   error[1]      HTTP 非 200 → 含状态码与 CIK 的错误  → TestFetchCompanyFactsHTTPError
//   加强项        季度不全的 FY 不产 Q4 (qCount==3)    → TestFetchCompanyFactsPartialFYNoQ4

// [TASK-008] 拆股归一化:
//   functional[0] 比例跳变检测+每股值归一到最新基准       → TestFetchCompanyFactsSplitNormalization
//   functional[1] 归一后派生 Q4 EPS 无符号矛盾(NI>0→EPS>0) → TestFetchCompanyFactsSplitNormalization
//   functional[2] DilutedShares 反向 ×比例                → TestFetchCompanyFactsSplitNormalization
//   boundary[1]   偏差>5%/非整数比例不误判为拆股           → TestFetchCompanyFactsSplitIgnoresNonIntegerRatio
//   boundary[0]   无拆股公司零影响                        → 既有 mini/rev_*/q4guard/shares_noq4 测试保持通过

// [TASK-001] tag 回退链 + 主干流科目扩展:
//   functional[0] EPS/Equity 首选 tag 缺失时回退命中             → TestFetchCompanyFactsTagFallback
//   functional[1] EPS 两 tag 皆缺 → NetIncome/DilutedShares 推算 → TestFetchCompanyFactsTagFallback
//   functional[2] 主干流五科目直取与推导                         → TestFetchCompanyFactsMainFlow
//   boundary[0]   回退只在首选缺失时生效(负向断言)               → TestFetchCompanyFactsPreferredTagWins
//   boundary[0]   非 NaN 的 EPS 绝不被推算值覆盖                 → TestFetchCompanyFactsTagFallback
//   boundary[0]   Q4 shares 恒 NaN → Q4 EPS 仍走 FY−ΣQ          → TestFetchCompanyFactsQ4EPSNotDerived
//   boundary[1]   shares 为 0/NaN 不推算(不产生 ±Inf)           → TestFetchCompanyFactsNoEPSDerivationWithoutShares
//   boundary[1]   SG&A 单侧不求和 / 毛利负值不钳零               → TestFetchCompanyFactsMainFlow
//   boundary[1]   新增 5 字段无 tag 且无法推导 → 保持 NaN        → TestFetchCompanyFactsQ4EPSNotDerived
//   error[0]      AD-18 golden 回归                              → TestFetchCompanyFactsGolden(golden_test.go)

// factsFileServer serves a fixture file, asserting the requested path.
func factsFileServer(t *testing.T, fixture, wantPath string) (*httptest.Server, *http.Request) {
	t.Helper()
	body, err := os.ReadFile(fixture)
	require.NoError(t, err)
	var captured http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = *r
		assert.Equal(t, wantPath, r.URL.Path)
		w.Write(body)
	}))
	return srv, &captured
}

func TestFetchCompanyFactsRequest(t *testing.T) {
	srv, captured := factsFileServer(t, "testdata/companyfacts_mini.json", "/api/xbrl/companyfacts/CIK0001045810.json")
	defer srv.Close()
	c := NewWithBaseURL("atlas-prism/1.0 (test@example.com)", srv.URL)
	_, err := c.FetchCompanyFacts("1045810")
	require.NoError(t, err)
	assert.Contains(t, captured.Header.Get("User-Agent"), "test@example.com")
}

func TestFetchCompanyFactsQuarterization(t *testing.T) {
	srv, _ := factsFileServer(t, "testdata/companyfacts_mini.json", "/api/xbrl/companyfacts/CIK0001045810.json")
	defer srv.Close()
	c := NewWithBaseURL("ua (t@e.com)", srv.URL)
	facts, err := c.FetchCompanyFacts("1045810")
	require.NoError(t, err)
	require.Len(t, facts, 4, "3 个 10-Q + 1 个推导 Q4")

	// functional[2] 修正重报去重:Q1 EPS 有两条 (filed 2025-05-01 val 0.5 / filed 2025-05-28 val 0.6),
	// 去重后取 filed 最新那条 0.6,而非旧条目 0.5。
	q1 := facts[0]
	assert.Equal(t, "2026Q1", q1.FiscalPeriod)
	assert.InDelta(t, 0.6, q1.EPSDiluted, 1e-9, "去重取 filed 最新 0.6,非旧条目 0.5")

	q4 := facts[3] // period_end 升序,最后为 Q4
	assert.Equal(t, "2026Q4", q4.FiscalPeriod)
	assert.Equal(t, time.Date(2026, 1, 25, 0, 0, 0, 0, time.UTC), q4.PeriodEnd)
	assert.Equal(t, time.Date(2026, 2, 26, 0, 0, 0, 0, time.UTC), q4.FilingDate, "Q4 生效日=10-K filed")
	assert.InDelta(t, 3.0-(0.6+0.7+0.8), q4.EPSDiluted, 1e-9, "Q4 EPS = FY − 3Q(近似口径)")
	assert.InDelta(t, 170000-(40000+42000+44000), q4.Revenue, 1e-9)
	assert.InDelta(t, 74000-(15000+17000+19000), q4.NetIncome, 1e-9)
	assert.InDelta(t, 95000.0, q4.Equity, 1e-9, "instant 型按 end 对齐")

	// boundary[0] Q2 shares 缺失 → NaN,不影响其他科目。
	// BUG-1 回归:EPS Q2 同 (fy,fp) 同 filed 有两条——累计 6mo(181 天 val 1.3)+ 单季(90 天 val 0.7),
	// 累计条目排在前。必须先按 duration 过滤再去重,断言取到单季 0.7 而非累计 1.3(否则 Q2 会因
	// duration 过滤丢失,facts 长度也不为 4)。Q3 同理(9mo 2.1 vs 单季 0.8)。
	q2 := facts[1]
	assert.Equal(t, "2026Q2", q2.FiscalPeriod)
	assert.True(t, math.IsNaN(q2.DilutedShares), "该季 shares 缺失 → NaN")
	assert.InDelta(t, 0.7, q2.EPSDiluted, 1e-9, "取单季 0.7,非累计 6mo 1.3(BUG-1)")

	q3 := facts[2]
	assert.Equal(t, "2026Q3", q3.FiscalPeriod)
	assert.InDelta(t, 0.8, q3.EPSDiluted, 1e-9, "取单季 0.8,非累计 9mo 2.1(BUG-1)")
}

// BUG-2 回归:首选 revenue tag 存在但其单季只标 fp=FY(不可用),须回退到带 fp=Q1/Q2/Q3 的次选 tag。
func TestFetchCompanyFactsRevenueTagSkipsUnusable(t *testing.T) {
	srv, _ := factsFileServer(t, "testdata/companyfacts_rev_fpfy.json", "/api/xbrl/companyfacts/CIK0001045810.json")
	defer srv.Close()
	c := NewWithBaseURL("ua (t@e.com)", srv.URL)
	facts, err := c.FetchCompanyFacts("1045810")
	require.NoError(t, err)
	require.Len(t, facts, 1)
	assert.Equal(t, "2026Q1", facts[0].FiscalPeriod)
	assert.InDelta(t, 40000.0, facts[0].Revenue, 1e-9,
		"首选 tag 单季仅 fp=FY 不可用 → 回退 Revenues 取到 40000,而非 RevenueFromContract 的 fp=FY 值 99999")
}

// boundary[1] Revenue tag 回退分支:第一优先 tag 缺失 → 回退 Revenues 仍取到 Revenue。
func TestFetchCompanyFactsRevenueFallback(t *testing.T) {
	srv, _ := factsFileServer(t, "testdata/companyfacts_rev_fallback.json", "/api/xbrl/companyfacts/CIK0001045810.json")
	defer srv.Close()
	c := NewWithBaseURL("ua (t@e.com)", srv.URL)
	facts, err := c.FetchCompanyFacts("1045810")
	require.NoError(t, err)
	require.Len(t, facts, 1)
	assert.Equal(t, "2026Q1", facts[0].FiscalPeriod)
	assert.InDelta(t, 40000.0, facts[0].Revenue, 1e-9, "第一优先 tag 缺失 → 回退 Revenues 取到 40000")
}

// 加强项:季度不全的 FY(只有 Q1、Q2)→ qCount==2 → 不推导 Q4。
func TestFetchCompanyFactsPartialFYNoQ4(t *testing.T) {
	srv, _ := factsFileServer(t, "testdata/companyfacts_partial_fy.json", "/api/xbrl/companyfacts/CIK0001045810.json")
	defer srv.Close()
	c := NewWithBaseURL("ua (t@e.com)", srv.URL)
	facts, err := c.FetchCompanyFacts("1045810")
	require.NoError(t, err)
	require.Len(t, facts, 2, "只有 Q1、Q2,qCount!=3 不产 Q4")
	for _, f := range facts {
		assert.NotEqual(t, "2026Q4", f.FiscalPeriod, "季度不全不应推导 Q4")
	}
}

// 股本为时点/加权平均量,不做 Q4=FY−ΣQ 减法推导(否则得负值)。EPS 正常推导 Q4,
// 但同一 Q4 的 DilutedShares 必须为 NaN(无独立单季申报),不能是 100−300=−200。
func TestFetchCompanyFactsSharesNoQ4Derivation(t *testing.T) {
	srv, _ := factsFileServer(t, "testdata/companyfacts_shares_noq4.json", "/api/xbrl/companyfacts/CIK0001045810.json")
	defer srv.Close()
	c := NewWithBaseURL("ua (t@e.com)", srv.URL)
	facts, err := c.FetchCompanyFacts("1045810")
	require.NoError(t, err)
	require.Len(t, facts, 4, "EPS 推导出 Q4")
	q4 := facts[3]
	assert.Equal(t, "2026Q4", q4.FiscalPeriod)
	assert.InDelta(t, 3.0-(0.6+0.7+0.8), q4.EPSDiluted, 1e-9, "EPS 正常 Q4 推导")
	assert.True(t, math.IsNaN(q4.DilutedShares), "股本不做 Q4 减法推导 → NaN,不能是负值")
}

// EDGAR 的 fy/fp 不可靠:三个单季实际 period_end 落在年度条目区间外(跨财年凑数)→ 不推导 Q4
// (宁缺勿错),避免 FY−ΣQ 得负值。
func TestFetchCompanyFactsQ4GuardRejectsCrossYear(t *testing.T) {
	srv, _ := factsFileServer(t, "testdata/companyfacts_q4guard.json", "/api/xbrl/companyfacts/CIK0001045810.json")
	defer srv.Close()
	c := NewWithBaseURL("ua (t@e.com)", srv.URL)
	facts, err := c.FetchCompanyFacts("1045810")
	require.NoError(t, err)
	require.Len(t, facts, 3, "三个单季 period_end 在年度区间外 → 不推导 Q4")
	for _, f := range facts {
		assert.NotEqual(t, "2026Q4", f.FiscalPeriod, "跨财年拼凑不应推导 Q4")
	}
}

// [TASK-008 review_fix] 同比例多次拆股:一个年度 period_end 跨两次 2:1 拆股产生两个 ratio-2
// 相邻跳变(filed 2021-02-20 与 2024-02-20,相隔 >1 年)。必须识别为两个独立事件(按生效日聚簇,
// 而非按 ratio 合并),pre-both 季度累计 ÷4(而非只 ÷2);DilutedShares 反向 ×4。
func TestFetchCompanyFactsRepeatedSameRatioSplit(t *testing.T) {
	srv, _ := factsFileServer(t, "testdata/companyfacts_double_split.json", "/api/xbrl/companyfacts/CIK0001045810.json")
	defer srv.Close()
	c := NewWithBaseURL("ua (t@e.com)", srv.URL)
	facts, err := c.FetchCompanyFacts("1045810")
	require.NoError(t, err)
	require.Len(t, facts, 1)
	q1 := facts[0]
	assert.Equal(t, time.Date(2019, 4, 27, 0, 0, 0, 0, time.UTC), q1.PeriodEnd)
	assert.InDelta(t, 2.0, q1.EPSDiluted, 1e-9, "两次 2:1 累计 ÷4(8.0÷4=2.0),而非只 ÷2=4.0")
	assert.InDelta(t, 4000.0, q1.DilutedShares, 1e-9, "股本反向 ×4(1000×4)")
	assert.InDelta(t, 5000.0, q1.NetIncome, 1e-9, "绝对值不受拆股影响")
}

// [TASK-001] functional[0]/[1] + boundary[0]:EPS/shares/equity 三条链的首选 tag 全缺 →
// 各自回退命中;Q2 连回退 EPS tag 也无条目 → 走 NetIncome/DilutedShares 推算;
// 而 Q1 的 EPS 已由回退 tag 取到 0.6,不得被推算值 15000/24700≈0.607 覆盖。
func TestFetchCompanyFactsTagFallback(t *testing.T) {
	srv, _ := factsFileServer(t, "testdata/companyfacts_tag_fallback.json", "/api/xbrl/companyfacts/CIK0001045810.json")
	defer srv.Close()
	c := NewWithBaseURL("ua (t@e.com)", srv.URL)
	facts, err := c.FetchCompanyFacts("1045810")
	require.NoError(t, err)
	require.Len(t, facts, 2)

	q1 := facts[0]
	assert.False(t, math.IsNaN(q1.EPSDiluted), "EarningsPerShareBasicAndDiluted 回退命中")
	assert.InDelta(t, 0.6, q1.EPSDiluted, 1e-9, "取回退 tag 原值,不被推算值 15000/24700 覆盖")
	assert.False(t, math.IsNaN(q1.Equity), "equity 含少数股东权益的 tag 回退命中")
	assert.InDelta(t, 80000.0, q1.Equity, 1e-9)
	assert.InDelta(t, 24700.0, q1.DilutedShares, 1e-9, "shares 回退到 …NumberOfSharesOutstandingBasic")

	q2 := facts[1]
	require.False(t, math.IsNaN(q2.NetIncome))
	require.False(t, math.IsNaN(q2.DilutedShares))
	assert.InDelta(t, q2.NetIncome/q2.DilutedShares, q2.EPSDiluted, 1e-9, "EPS 两 tag 该季均无条目 → 推算")
	assert.InDelta(t, 0.68, q2.EPSDiluted, 1e-9)
}

// [TASK-001] boundary[0] 负向断言:首选与回退 tag **同时存在**时,结果必须只来自首选 tag
// (回退值全为哨兵 99999,一旦生效即被捕获)。
func TestFetchCompanyFactsPreferredTagWins(t *testing.T) {
	srv, _ := factsFileServer(t, "testdata/companyfacts_tag_preference.json", "/api/xbrl/companyfacts/CIK0001045810.json")
	defer srv.Close()
	c := NewWithBaseURL("ua (t@e.com)", srv.URL)
	facts, err := c.FetchCompanyFacts("1045810")
	require.NoError(t, err)
	require.Len(t, facts, 1)

	q := facts[0]
	assert.InDelta(t, 0.6, q.EPSDiluted, 1e-9, "EarningsPerShareDiluted 优先于 BasicAndDiluted")
	assert.InDelta(t, 24700.0, q.DilutedShares, 1e-9, "…NumberOfDilutedSharesOutstanding 优先于 …OutstandingBasic")
	assert.InDelta(t, 80000.0, q.Equity, 1e-9, "StockholdersEquity 优先于 …IncludingPortionAttributable…")
	assert.InDelta(t, 40000.0-24000.0, q.GrossProfit, 1e-9,
		"毛利推导取 CostOfRevenue(24000),而非 CostOfGoodsAndServicesSold(99999)")
}

// [TASK-001] functional[2] + boundary[1]:主干流五科目。
// Q1 毛利/SG&A 走推导;Q2 成本>收入 → 毛利为负不钳零、只有 Selling 单侧 → SG&A 保持 NaN;
// Q3 两个 tag 本季有条目 → 直取,不被推导覆盖。
func TestFetchCompanyFactsMainFlow(t *testing.T) {
	srv, _ := factsFileServer(t, "testdata/companyfacts_mainflow.json", "/api/xbrl/companyfacts/CIK0001045810.json")
	defer srv.Close()
	c := NewWithBaseURL("ua (t@e.com)", srv.URL)
	facts, err := c.FetchCompanyFacts("1045810")
	require.NoError(t, err)
	require.Len(t, facts, 3)
	q1, q2, q3 := facts[0], facts[1], facts[2]

	assert.InDelta(t, 40000.0-24000.0, q1.GrossProfit, 1e-9, "GrossProfit tag 无该季条目 → Revenue−Cost")
	assert.InDelta(t, 5000.0+3000.0, q1.SGnA, 1e-9, "SG&A 由 Selling + G&A 分列求和")
	assert.InDelta(t, 2000.0, q1.RnD, 1e-9, "RnD 由 ResearchAndDevelopmentExpense 直取")
	assert.InDelta(t, 10000.0, q1.OperatingIncome, 1e-9, "OperatingIncome 由 OperatingIncomeLoss 直取")
	assert.InDelta(t, 1500.0, q1.IncomeTax, 1e-9, "IncomeTax 由 IncomeTaxExpenseBenefit 直取")

	assert.InDelta(t, 42000.0-45000.0, q2.GrossProfit, 1e-9, "毛利推导为负时保留负值")
	assert.Less(t, q2.GrossProfit, 0.0, "不钳零")
	assert.True(t, math.IsNaN(q2.SGnA), "只有 Selling 单侧 → 半个和比缺失更危险,保持 NaN")

	assert.InDelta(t, 99999.0, q3.GrossProfit, 1e-9, "GrossProfit tag 该季有条目 → 不被 Revenue−Cost 覆盖")
	assert.InDelta(t, 12345.0, q3.SGnA, 1e-9, "SG&A tag 该季有条目 → 不被分列求和覆盖")
}

// [TASK-001] boundary[0]/[1]:Q4 无独立单季股本申报 → DilutedShares 恒 NaN,
// 故 Q4 EPS 仍走 FY−ΣQ 而非推算;mini 无 Cost/Selling/G&A/RnD/营利/税 tag,
// 新增 5 字段无 tag 且无法推导 → 全季保持 NaN。
func TestFetchCompanyFactsQ4EPSNotDerived(t *testing.T) {
	srv, _ := factsFileServer(t, "testdata/companyfacts_mini.json", "/api/xbrl/companyfacts/CIK0001045810.json")
	defer srv.Close()
	c := NewWithBaseURL("ua (t@e.com)", srv.URL)
	facts, err := c.FetchCompanyFacts("1045810")
	require.NoError(t, err)
	require.Len(t, facts, 4)

	q4 := facts[3]
	assert.True(t, math.IsNaN(q4.DilutedShares), "Q4 无独立单季股本申报 → 恒 NaN")
	assert.InDelta(t, 3.0-(0.6+0.7+0.8), q4.EPSDiluted, 1e-9, "Q4 EPS 仍走 FY−ΣQ,不改走推算")

	for _, f := range facts {
		assert.True(t, math.IsNaN(f.GrossProfit), "%s 无 Cost tag,毛利无法推导", f.FiscalPeriod)
		assert.True(t, math.IsNaN(f.RnD), "%s", f.FiscalPeriod)
		assert.True(t, math.IsNaN(f.SGnA), "%s 无 SG&A 及其分列 tag", f.FiscalPeriod)
		assert.True(t, math.IsNaN(f.OperatingIncome), "%s", f.FiscalPeriod)
		assert.True(t, math.IsNaN(f.IncomeTax), "%s", f.FiscalPeriod)
	}
}

// [TASK-001] boundary[1]:EPS 两 tag 皆缺时,分母为 0(Q1)或缺失(Q2)一律不推算 ——
// 必须保持 NaN,绝不产生 ±Inf(下游 encoding/json 对 Inf 直接报错,AD-14)。
func TestFetchCompanyFactsNoEPSDerivationWithoutShares(t *testing.T) {
	srv, _ := factsFileServer(t, "testdata/companyfacts_eps_no_shares.json", "/api/xbrl/companyfacts/CIK0001045810.json")
	defer srv.Close()
	c := NewWithBaseURL("ua (t@e.com)", srv.URL)
	facts, err := c.FetchCompanyFacts("1045810")
	require.NoError(t, err)
	require.Len(t, facts, 2)

	assert.InDelta(t, 0.0, facts[0].DilutedShares, 1e-9, "该季股本申报为 0")
	assert.True(t, math.IsNaN(facts[0].EPSDiluted), "分母为 0 → 不推算,保持 NaN")
	assert.False(t, math.IsInf(facts[0].EPSDiluted, 0), "绝不产生 ±Inf")

	assert.True(t, math.IsNaN(facts[1].DilutedShares), "该季无股本条目")
	assert.True(t, math.IsNaN(facts[1].EPSDiluted), "分母缺失 → 不推算,保持 NaN")
	assert.False(t, math.IsInf(facts[1].EPSDiluted, 0), "绝不产生 ±Inf")
}

func TestFetchCompanyFactsIFRS(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"cik": 1, "facts": {"ifrs-full": {}}}`))
	}))
	defer srv.Close()
	c := NewWithBaseURL("ua (t@e.com)", srv.URL)
	_, err := c.FetchCompanyFacts("1")
	assert.ErrorIs(t, err, ErrNotUSGAAP)
}

// error[1] HTTP 非 200 → 返回含状态码与 CIK 的错误。
func TestFetchCompanyFactsHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	c := NewWithBaseURL("ua (t@e.com)", srv.URL)
	_, err := c.FetchCompanyFacts("1045810")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "403")
	assert.Contains(t, err.Error(), "1045810")
}

// [TASK-008] 拆股归一化:fixture 模拟 10:1——年度 EPS 被重述(12.0 拆股前→1.2 拆股后),
// 单季只有拆股前基准(3.0/3.5/4.0)。归一化后:单季每股值 ÷10,派生 Q4 = 归一化 FY−Σ三季 > 0
// (与 NI>0 一致,无符号矛盾);DilutedShares 反向 ×10 保持 BVPS/RPS 分母基准一致。
func TestFetchCompanyFactsSplitNormalization(t *testing.T) {
	srv, _ := factsFileServer(t, "testdata/companyfacts_split.json", "/api/xbrl/companyfacts/CIK0001045810.json")
	defer srv.Close()
	c := NewWithBaseURL("ua (t@e.com)", srv.URL)
	facts, err := c.FetchCompanyFacts("1045810")
	require.NoError(t, err)
	require.Len(t, facts, 4, "3 单季 + 推导 Q4")

	q1 := facts[0]
	assert.Equal(t, time.Date(2023, 4, 27, 0, 0, 0, 0, time.UTC), q1.PeriodEnd)
	assert.InDelta(t, 0.30, q1.EPSDiluted, 1e-9, "Q1 EPS 归一化到拆股后基准(3.0÷10)")
	assert.InDelta(t, 25000.0, q1.DilutedShares, 1e-9, "DilutedShares 反向 ×10(2500×10)")
	assert.InDelta(t, 15000.0, q1.NetIncome, 1e-9, "NetIncome 绝对值不受拆股影响")

	q4 := facts[3]
	assert.Equal(t, time.Date(2024, 1, 25, 0, 0, 0, 0, time.UTC), q4.PeriodEnd)
	assert.InDelta(t, 1.2-(0.30+0.35+0.40), q4.EPSDiluted, 1e-9, "派生 Q4 EPS = 归一化 FY−Σ三季 = 0.15")
	assert.Greater(t, q4.EPSDiluted, 0.0, "NI>0 的年度,派生 Q4 EPS 必须 >0(无符号矛盾)")
	assert.InDelta(t, 23000.0, q4.NetIncome, 1e-9, "派生 Q4 NI = 74000−51000")
	assert.Greater(t, q4.NetIncome, 0.0)
}

// [TASK-008] boundary[1]/error[0]:跨 filed 跳变但比例非近似整数(6.9→3.0=2.3,离最近整数 2 偏差 15%>5%)
// → 无法确定拆股比,不归一化,单季每股值保持原值(3.0,而非误 ÷2=1.5)。
func TestFetchCompanyFactsSplitIgnoresNonIntegerRatio(t *testing.T) {
	srv, _ := factsFileServer(t, "testdata/companyfacts_nonsplit_jump.json", "/api/xbrl/companyfacts/CIK0001045810.json")
	defer srv.Close()
	c := NewWithBaseURL("ua (t@e.com)", srv.URL)
	facts, err := c.FetchCompanyFacts("1045810")
	require.NoError(t, err)
	require.Len(t, facts, 1, "只有 Q1 单季,年度非整数跳变不产生归一化副作用")
	assert.InDelta(t, 3.0, facts[0].EPSDiluted, 1e-9, "非整数比例不误判为拆股 → EPS 保持原值 3.0")
}
