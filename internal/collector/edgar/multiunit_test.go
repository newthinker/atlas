package edgar

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Context Checkpoint: done_criteria → test mapping  [TASK-001 多 unit tag]
//   functional[0] 多 unit 确定性选择,结果与 unit 在 map 中的顺序无关
//       → TestSelectUnitIsPermutationInvariant        (结构性,p(kill)=1)
//       → TestFetchCompanyFactsMultiUnitSelectsDeterministically (整合层,R 次重复)
//   functional[1] Q4 EPS 经 FY−ΣQ 产出 18.21−12.339=5.871
//       → TestFetchCompanyFactsMultiUnitDerivesQ4EPS
//   functional[2] 首选 tag 全部 unit 无数据 → 回退链生效(且回退 tag 自身也是多 unit)
//       → TestFetchCompanyFactsMultiUnitFallbackChain
//       (首选 tag **整个缺席**的分支由既有 TestFetchCompanyFactsTagFallback 覆盖)
//   functional[3] 单 unit(AVGO 形态)回归:与多 unit 版逐季逐字段一致
//       → TestFetchCompanyFactsMultiUnitMatchesSingleUnit
//   boundary[0]   跨 unit 同期间冲突值:胜出 unit 通吃,落选 unit 整体不参与
//       → TestFetchCompanyFactsMultiUnitConflictingPeriods
//   error[0]      全部 unit 无可用数据 → 视为该 tag 缺失,不 panic、不产出 ±Inf(AD-14)
//       → TestFetchCompanyFactsMultiUnitFallbackChain

// multiUnitRuns 是整合层「确定性」断言的重复次数。
//
// Go 的 map 迭代起点随机,故「取第一个 unit」这类缺陷单次运行只是**概率性**暴露。
// 实测本 fixture 的 EarningsPerShareDiluted(2 万次重新反序列化 + 迭代取首个):
// pure 排首位 0.8764、USD/shares 排首位 0.1235 —— 注意并非 0.5,Go 只保证起点随机,
// 不保证各 key 等概率排首位。故缺陷单次「逃逸」(恰好取到正确 unit)的概率 ≈ 0.124,
// R 次全部逃逸的概率 ≈ 0.124^R;R=32 → ~1e-29,远低于任何 flake 门槛。
// (顺带解释了 live 现象:COST/WMT/CRM 几乎每次都失败,而非半数失败。)
//
// 这只是针对 map 迭代序的**概率性**兜底。选择规则本身的确定性由
// TestSelectUnitIsPermutationInvariant 以显式排列的方式 p=1 地钉死 —— 输出层断言原理上
// 无法必然捕获「取第一个」(正确输出恰好等于「碰巧取到 USD/shares」的输出),
// 故结构性保证必须落在决策函数这一层。
const multiUnitRuns = 32

// COST FY2025 真实值(DoD 指定口径):Q4 = FY − (Q1+Q2+Q3)。
const (
	costQ1  = 4.041
	costQ2  = 4.019
	costQ3  = 4.279
	costFY  = 18.21
	costQ4  = 5.871 // 18.21 − 12.339
	epsTol  = 1e-6
	costCIK = "1045810"
)

// dumpFacts 把 facts 渲染成逐季逐字段的确定性文本(含 M3 五个主干流字段)。
//
// 用文本而非 require.Equal 直接比 []QuarterlyFact:reflect.DeepEqual 下 NaN != NaN,
// 而本包以 NaN 表示缺失、绝大多数字段都是 NaN,直接比较会把两份**完全相同**的结果判为不等。
func dumpFacts(facts []QuarterlyFact) string {
	num := func(v float64) string { return strconv.FormatFloat(v, 'g', 12, 64) }
	var b strings.Builder
	fmt.Fprintf(&b, "quarters=%d\n", len(facts))
	for _, f := range facts {
		fmt.Fprintf(&b, "%s|%s|%s|eps=%s|rev=%s|ni=%s|eq=%s|shares=%s|gp=%s|rnd=%s|sgna=%s|opinc=%s|tax=%s\n",
			f.FiscalPeriod, f.PeriodEnd.Format("2006-01-02"), f.FilingDate.Format("2006-01-02"),
			num(f.EPSDiluted), num(f.Revenue), num(f.NetIncome), num(f.Equity), num(f.DilutedShares),
			num(f.GrossProfit), num(f.RnD), num(f.SGnA), num(f.OperatingIncome), num(f.IncomeTax))
	}
	return b.String()
}

// fetchDeterministic 重复抓取同一 fixture multiUnitRuns 次,断言每次输出都相同
// (出现两种及以上结果即证明 unit 选择依赖 map 迭代序),失败时把各变体打出来便于定位。
// 返回该输出的文本形态与最后一次的 facts。
func fetchDeterministic(t *testing.T, fixture string) (string, []QuarterlyFact) {
	t.Helper()
	srv, _ := factsFileServer(t, "testdata/"+fixture+".json",
		"/api/xbrl/companyfacts/CIK0001045810.json")
	defer srv.Close()
	c := NewWithBaseURL("ua (t@e.com)", srv.URL)

	seen := map[string]bool{}
	var distinct []string
	var last []QuarterlyFact
	for i := 0; i < multiUnitRuns; i++ {
		facts, err := c.FetchCompanyFacts(costCIK)
		require.NoError(t, err)
		if d := dumpFacts(facts); !seen[d] {
			seen[d] = true
			distinct = append(distinct, d)
		}
		last = facts
	}
	if len(distinct) > 1 {
		t.Errorf("%s: %d 次抓取产出了 %d 种不同结果,unit 选择依赖 map 迭代序(非确定性):\n--- %s",
			fixture, multiUnitRuns, len(distinct), strings.Join(distinct, "--- 变体分隔 ---\n"))
	}
	return distinct[0], last
}

// assertNoInf 断言没有任何字段是 ±Inf —— 下游 encoding/json 遇到 Inf 会直接报错(AD-14)。
func assertNoInf(t *testing.T, facts []QuarterlyFact) {
	t.Helper()
	for _, f := range facts {
		for name, v := range map[string]float64{
			"EPSDiluted": f.EPSDiluted, "Revenue": f.Revenue, "NetIncome": f.NetIncome,
			"Equity": f.Equity, "DilutedShares": f.DilutedShares, "GrossProfit": f.GrossProfit,
			"RnD": f.RnD, "SGnA": f.SGnA, "OperatingIncome": f.OperatingIncome,
			"IncomeTax": f.IncomeTax,
		} {
			assert.False(t, math.IsInf(v, 0), "%s 的 %s 为 ±Inf(AD-14)", f.FiscalPeriod, name)
		}
	}
}

// epsByPeriod 把 facts 索引为 FiscalPeriod → EPSDiluted。
func epsByPeriod(facts []QuarterlyFact) map[string]float64 {
	m := map[string]float64{}
	for _, f := range facts {
		m[f.FiscalPeriod] = f.EPSDiluted
	}
	return m
}

// functional[0] 多 unit 时选择必须确定 —— 且选中的是数据完整的那个 unit(USD/shares),
// 而非只剩零星陈旧重述的 pure。
func TestFetchCompanyFactsMultiUnitSelectsDeterministically(t *testing.T) {
	_, facts := fetchDeterministic(t, "companyfacts_multiunit")

	require.Len(t, facts, 4, "选中 USD/shares 应得 Q1..Q3 三个单季 + 推导 Q4;"+
		"若误选 pure(仅 1 个可用单季 + 1 个年度)则只剩 1 季且无法推导 Q4")
	eps := epsByPeriod(facts)
	assert.InDelta(t, costQ1, eps["2025Q1"], epsTol)
	assert.InDelta(t, costQ2, eps["2025Q2"], epsTol)
	assert.InDelta(t, costQ3, eps["2025Q3"], epsTol)
}

// functional[1] 本任务的核心产出:Q4 EPS 从 NaN 变为 FY−ΣQ 的实算值。
func TestFetchCompanyFactsMultiUnitDerivesQ4EPS(t *testing.T) {
	_, facts := fetchDeterministic(t, "companyfacts_multiunit")

	eps := epsByPeriod(facts)
	q4, ok := eps["2025Q4"]
	require.True(t, ok, "Q4 缺失 —— 年度 EPS 未被取到,FY−ΣQ 无输入(正是本缺陷的症状)")
	require.False(t, math.IsNaN(q4), "Q4 EPS 仍为 NaN;Q4 的 DilutedShares 恒 NaN,只能靠 FY−ΣQ")
	assert.InDelta(t, costQ4, q4, epsTol,
		"Q4 EPS 应为 %v − (%v+%v+%v) = %v", costFY, costQ1, costQ2, costQ3, costQ4)
	assertNoInf(t, facts)
}

// functional[3] 单 unit 回归:多 unit fixture 与「删掉 pure 后」的单 unit fixture
// 必须逐季逐字段完全一致。一处断言同时钉死两件事 —— 多 unit 选对了、单 unit 没被改坏。
func TestFetchCompanyFactsMultiUnitMatchesSingleUnit(t *testing.T) {
	multiDump, _ := fetchDeterministic(t, "companyfacts_multiunit")
	singleDump, _ := fetchDeterministic(t, "companyfacts_singleunit")

	assert.Equal(t, singleDump, multiDump,
		"多 unit(pure + USD/shares)与单 unit(仅 USD/shares)的输出必须一致")
}

// boundary[0] 两个 unit 含同一期间的冲突值时的行为:**胜出 unit 通吃,落选 unit 整体不参与**。
//
// fixture 刻意让 pure 的 filed(2026-01-15)晚于 USD/shares 的全部条目 —— 真实 COST 正是
// 这个形态(pure 全部 filed=2010-10-18,晚于对应期间的原始申报)。因此任何「合并两个 unit」
// 的实现都会在 applyDuration 的 keepLatest(取 filed 最新)去重中让陈旧的 pure 值胜出:
// Q1 会变成 99.9,Q4 会变成 88.8−(99.9+4.019+4.279) = −19.398。断言正是要排除这两种结果。
func TestFetchCompanyFactsMultiUnitConflictingPeriods(t *testing.T) {
	_, facts := fetchDeterministic(t, "companyfacts_multiunit")
	eps := epsByPeriod(facts)

	assert.InDelta(t, costQ1, eps["2025Q1"], epsTol,
		"Q1 同期间 pure=99.9(filed 更晚)vs USD/shares=4.041:取胜出 unit 的 4.041,不合并")
	assert.InDelta(t, costQ4, eps["2025Q4"], epsTol,
		"Q4 由胜出 unit 的年度 18.21 推导;若合并则年度取 pure=88.8,得 −19.398")

	for _, f := range facts {
		for _, stale := range []float64{99.9, 88.8, 77.7} {
			assert.Greater(t, math.Abs(f.EPSDiluted-stale), epsTol,
				"%s 取到了落选 unit(pure)的哨兵值 %v", f.FiscalPeriod, stale)
		}
	}
}

// functional[2] + error[0] 首选 tag 的**所有 unit 都无可用数据** → 该 tag 视为缺失,
// 回退链走到 EarningsPerShareBasicAndDiluted;而回退 tag 自身也是多 unit,须同样确定性选中。
// 全程不得 panic、不得产出 ±Inf。
func TestFetchCompanyFactsMultiUnitFallbackChain(t *testing.T) {
	_, facts := fetchDeterministic(t, "companyfacts_multiunit_fallback")

	require.Len(t, facts, 4, "回退 tag 的 USD/shares 有三个单季 + 一个年度 → 应得 4 季")
	eps := epsByPeriod(facts)
	assert.InDelta(t, 1.0, eps["2025Q1"], epsTol)
	assert.InDelta(t, 1.1, eps["2025Q2"], epsTol)
	assert.InDelta(t, 1.2, eps["2025Q3"], epsTol)
	assert.InDelta(t, 5.0-(1.0+1.1+1.2), eps["2025Q4"], epsTol, "回退 tag 上同样推导 Q4")

	// error[0] 无任何 shares tag → DilutedShares 恒 NaN → EPS 推算(NI/shares)不可能触发,
	// 任何 ±Inf 都无处产生。
	for _, f := range facts {
		assert.True(t, math.IsNaN(f.DilutedShares), "%s 无 shares tag → NaN", f.FiscalPeriod)
	}
	assertNoInf(t, facts)
}

// --- functional[0] 的结构性保证 ---------------------------------------------
//
// 整合层的输出断言**原理上**无法必然捕获「取第一个 unit」:正确结果恰好等于「碰巧取到
// USD/shares」的结果,单次运行只有 p=0.5 暴露,靠重复运行也只能压到 2^-32,仍是概率性的。
// 把决策提为纯函数 bestUnitName(units, names) 并由测试**显式**传入全部排列后,
// 变异 `return names[0]` 在排列 {pure, USD/shares} 上必然返回 "pure" → p(kill)=1,
// 与 map 迭代随机性无关。这正是 AD-27「结构性保证 > 概率性压制」。

// 构造 rawFact 的三种形态,口径与 isSingleQuarter / isAnnual 一致。
func factSingle(fp string) rawFact { // 83 天,fp∈Q1/Q2/Q3 → 可用单季
	return rawFact{Start: "2024-11-25", End: "2025-02-16", FP: fp, Filed: "2025-03-13"}
}
func factAnnual() rawFact { // 363 天,fp=FY → 可用年度
	return rawFact{Start: "2024-09-02", End: "2025-08-31", FP: "FY", Filed: "2025-10-08"}
}
func factUnusable() rawFact { // 167 天累计条目 → 既非单季也非年度
	return rawFact{Start: "2024-09-02", End: "2025-02-16", FP: "Q2", Filed: "2025-03-13"}
}

// permutations 返回 in 的全部排列(n 小,朴素实现即可)。
func permutations(in []string) [][]string {
	if len(in) <= 1 {
		return [][]string{append([]string(nil), in...)}
	}
	var out [][]string
	for i := range in {
		rest := make([]string, 0, len(in)-1)
		rest = append(rest, in[:i]...)
		rest = append(rest, in[i+1:]...)
		for _, p := range permutations(rest) {
			out = append(out, append([]string{in[i]}, p...))
		}
	}
	return out
}

func TestSelectUnitIsPermutationInvariant(t *testing.T) {
	cases := []struct {
		name  string
		units map[string][]rawFact
		want  string
	}{
		{
			name: "COST 形态:可用条数多者胜(USD/shares 承载全部年度数据)",
			units: map[string][]rawFact{
				"pure":       {factSingle("Q1"), factAnnual(), factUnusable()},
				"USD/shares": {factSingle("Q1"), factSingle("Q2"), factSingle("Q3"), factAnnual()},
			},
			want: "USD/shares",
		},
		{
			name: "落选 unit 总条数更多但可用为 0 —— 打分只看可用条数",
			units: map[string][]rawFact{
				"noise": {factUnusable(), factUnusable(), factUnusable(), factUnusable()},
				"good":  {factSingle("Q1")},
			},
			want: "good",
		},
		{
			name: "可用条数相同 → 总条数多者胜",
			units: map[string][]rawFact{
				"aaa": {factSingle("Q1")},
				"bbb": {factSingle("Q1"), factUnusable()},
			},
			want: "bbb",
		},
		{
			name: "可用与总条数均相同 → unit 名升序,杜绝任何残余抖动",
			units: map[string][]rawFact{
				"bbb": {factSingle("Q1")},
				"aaa": {factSingle("Q1")},
			},
			want: "aaa",
		},
		{
			name: "全部 unit 都无可用数据 → 仍须确定性返回(名升序),交由回退链处理",
			units: map[string][]rawFact{
				"pure":       {},
				"USD/shares": {},
			},
			want: "USD/shares", // 'U'(85) < 'p'(112)
		},
		{
			name: "三个 unit:排列数 6,任一排列都不得改变结果",
			units: map[string][]rawFact{
				"pure":       {factSingle("Q1")},
				"USD/shares": {factSingle("Q1"), factSingle("Q2"), factAnnual()},
				"shares":     {factSingle("Q1"), factUnusable()},
			},
			want: "USD/shares",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			names := make([]string, 0, len(tc.units))
			for n := range tc.units {
				names = append(names, n)
			}
			for _, perm := range permutations(names) {
				assert.Equal(t, tc.want, bestUnitName(tc.units, perm),
					"候选顺序 %v 改变了选择结果 —— 规则不确定", perm)
			}
		})
	}
}

// selectUnit 在 tag 无 unit 时返回 nil,交由回退链判定为「该 tag 缺失」。
func TestSelectUnitEmpty(t *testing.T) {
	assert.Nil(t, selectUnit(nil))
	assert.Nil(t, selectUnit(map[string][]rawFact{}))
}
