package edgar

import (
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Context Checkpoint: done_criteria → test mapping
//
//	[TASK-002] V(Visa) 的 EPS 走「报告实例文档 + 股份类别维度」回退路径
//
//	functional[0] 可行性:真实 V 实例文档里 EPS **存在**,且 100% 带
//	              StatementClassOfStockAxis 维度、无任何无维度 EPS
//	              (= companyfacts 一条都没有的确切原因)      → TestVInstanceEPSIsAlwaysClassDimensioned
//	functional[0] 对照:同一份文档的 NetIncomeLoss **有**无维度版本
//	              (证明缺的不是文档而是「无维度」这个形态)     → TestVInstanceNetIncomeHasUndimensionedFact
//	functional[1] 真实 10-K 解析出 Class A 三个财年稀释 EPS    → TestParseClassFactsRealTenK
//	functional[1] 主类别选择:选中 Class A,不选 B2 的 0 值陷阱  → TestDominantClassPicksClassA
//	functional[1] 主类别选择与输入排列无关(结构性 p(kill)=1)  → TestDominantClassIsPermutationInvariant
//	functional[1] 接入取数链:V 的 eps_diluted 不再全为 NULL    → TestFetchCompanyFactsClassEPSFallback
//	functional[1] Q4 = FY − ΣQ1..Q3 用真实数据两个财年闭合     → TestFetchCompanyFactsClassEPSDerivesQ4
//	functional[1] 只取 Diluted,不得混入同文档的 Basic          → TestParseClassFactsDoesNotMixBasic
//	functional[2] 回归:companyfacts 有 EPS 时**一次 archives 请求都不发**
//	              (结构性保证,非概率性)                       → TestFetchCompanyFactsWithEPSNeverTouchesArchives
//	functional[2] 回归:四家既有 fixture 逐季逐字段与改动前一致  → TestFetchCompanyFactsUnaffectedByClassEPSPath
//	functional[1] 同期同值跨报告重复 → 取原始申报日(防前视)    → TestClassRawFactsKeepsEarliestFilingForUnchangedValue
//	functional[1] 值变了(真重述)→ 两条都留,由 keepLatest 定夺  → TestClassRawFactsKeepsBothOnRestatement
//	boundary[0]   YTD 累计期(182/183/273/274 天)必须丢弃      → TestClassRawFactsDropsYTDPeriods
//	boundary[0]   合成 rawFact 的 FP 标注与下游过滤器自洽       → TestClassRawFactsFPMatchesDownstreamFilter
//	boundary[0]   回看上限 maxClassEPSFilings=28(AD-11 双重上限)→ TestFetchClassEPSRespectsLookbackCap
//	boundary[0]   与 maxSegmentFilings=12 相互独立,互不牵动     → TestClassEPSAndSegmentCapsAreIndependent
//	boundary[0]   2019 前老式 filing 连续 404 尾部逐份跳过       → TestFetchClassEPSSkipsPre2019NotFoundTail
//	error[0]      实例文档 404 → 跳过该份,不 panic 不整体失败   → TestFetchCompanyFactsClassEPSInstanceNotFound
//	error[0]      submissions 非 200 → 保持 degraded,不返回错误 → TestFetchCompanyFactsClassEPSSubmissionsError
//	error[0]      实例文档 XML 损坏 → 跳过该份继续              → TestFetchCompanyFactsClassEPSMalformedInstance
//	error[0]      响应体超限 → 跳过该份,不 OOM                  → TestFetchCompanyFactsClassEPSBodyLimit
//	error[0]      全部实例文档不可达 → 与当前行为一致(EPS 全 NaN) → TestFetchCompanyFactsClassEPSAllUnavailable
//
//	[TASK-003] 收尾:让两条「看着绿其实没判别力」的地方真正有判别力
//
//	functional[1] 回看上限用例对**取值**有判别力(期望值不再自引用常量)
//	              → TestFetchClassEPSRespectsLookbackCap(改写)
//	functional[2] dominantClass 用 max 而非 sum(真实数据无判别力,构造反例)
//	boundary[0]   该 fixture 只测算法选择,与 V 的真实分布刻意不同
//	              → TestDominantClassUsesMaxSharesNotSum(新增)

// vCIK 是 Visa 的真实 CIK。
const vCIK = "1403161"

// vInstanceDocs 把 submissions_v.json 里五份真实报告的实例文档 basename 映射到 fixture。
// basename 由 primaryDocument 经 AD-3 推导(v-20250930.htm → v-20250930_htm.xml)。
//
// FY2026Q2 那份是刻意加的:它含 2025-01-01~2025-03-31 的**对比期**(值 2.32,与 FY2025Q2
// 报告的本期值完全相同),构成「同一期间跨报告重复出现」的真实样本 —— 点位可用日期的
// 关键用例(TestClassRawFactsKeepsEarliestFilingForUnchangedValue)靠它才成立。
func vInstanceDocs() map[string]string {
	return map[string]string{
		"v-20250930_htm.xml": "testdata/instance_v_fy2025_10k.xml",
		"v-20241231_htm.xml": "testdata/instance_v_fy2025q1_10q.xml",
		"v-20250331_htm.xml": "testdata/instance_v_fy2025q2_10q.xml",
		"v-20250630_htm.xml": "testdata/instance_v_fy2025q3_10q.xml",
		"v-20260331_htm.xml": "testdata/instance_v_fy2026q2_10q.xml",
	}
}

// newClassEPSServers 起 data host(companyfacts + submissions 两类路径)与 archives host
// 两个独立 server。与 newSegmentServers 的区别:data host 这里要同时应答 companyfacts,
// 因为本路径的入口是 FetchCompanyFacts 而非 FetchSegmentRevenue。
//
// factsFixture 为空 → companyfacts 返回 500;submissionsFixture 为空 → submissions 返回 500。
func newClassEPSServers(t *testing.T, factsFixture, submissionsFixture string,
	docs map[string]string) (*Client, *hostRecorder, *hostRecorder) {
	t.Helper()
	dataRec, archRec := &hostRecorder{}, &hostRecorder{}

	// fixture 为空(或读不出)一律 500 —— 让对应路径「不可用」是多条降级用例的前提。
	serve := func(w http.ResponseWriter, fixture string) {
		body, err := os.ReadFile(fixture)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Write(body)
	}

	dataSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dataRec.record(r)
		if strings.Contains(r.URL.Path, "/submissions/") {
			serve(w, submissionsFixture)
			return
		}
		serve(w, factsFixture)
	}))
	t.Cleanup(dataSrv.Close)

	archSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		archRec.record(r)
		fixture, ok := docs[path.Base(r.URL.Path)]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		serve(w, fixture)
	}))
	t.Cleanup(archSrv.Close)

	return NewWithBaseURLs("atlas-prism/1.0 (test@example.com)", dataSrv.URL, archSrv.URL), dataRec, archRec
}

// parseFixture 流式解析一份 fixture 实例文档。
func parseFixture(t *testing.T, p, axis string) []classFact {
	t.Helper()
	f, err := os.Open(p)
	require.NoError(t, err)
	defer f.Close()
	facts, err := parseClassFacts(f, axis)
	require.NoError(t, err)
	return facts
}

// 文本层面的对照断言复用 segments_test.go 已有的 readFixture(同包,签名与语义一致)。

// findQuarter 取指定 period_end 的那一季。
func findQuarter(t *testing.T, got []QuarterlyFact, end string) QuarterlyFact {
	t.Helper()
	for _, q := range got {
		if q.PeriodEnd.Format(dateLayout) == end {
			return q
		}
	}
	t.Fatalf("period_end %s 未出现在结果中", end)
	return QuarterlyFact{}
}

// ---------------------------------------------------------------------------
// functional[0] 可行性验证 —— 把「实例文档里有 EPS 且全部带类别维度」这条实测发现
// 钉成可执行断言。fixture 是从真实抓取的 2.47 MB 10-K 里**逐字节原样**截取的子集。
// ---------------------------------------------------------------------------

func TestVInstanceEPSIsAlwaysClassDimensioned(t *testing.T) {
	facts := parseFixture(t, "testdata/instance_v_fy2025_10k.xml", classOfStockAxis)

	var diluted int
	for _, f := range facts {
		if f.tag != "EarningsPerShareDiluted" {
			continue
		}
		diluted++
		assert.NotEmpty(t, f.member,
			"V 的每一条 EPS 事实都必须带类别 member —— 这正是 companyfacts 取不到的原因")
	}
	assert.Positive(t, diluted,
		"可行性前提:真实 10-K 实例文档里必须存在 EarningsPerShareDiluted 事实")

	// 反向:整份文档不存在**无维度**的 EPS 事实。parseClassFacts 只收轴匹配的事实,
	// 故这里换用一个不存在的轴来扫描 —— 若有无维度 EPS,它同样不会被任何轴收录,
	// 因此改用原始文本层面的对照断言(见 TestVInstanceNetIncomeHasUndimensionedFact)。
	assert.NotContains(t, readFixture(t, "testdata/instance_v_fy2025_10k.xml"),
		`<us-gaap:EarningsPerShareDiluted contextRef=`,
		"若出现单行无维度写法说明 fixture 形态已变,须重新核对结论")
}

// 对照组:同一份真实文档里 NetIncomeLoss **有**无维度事实。
// 这条把「companyfacts 缺的是无维度形态,而不是这家公司没申报」钉死 —— 没有它,
// functional[0] 的结论可以被「V 根本没报 EPS」这一竞争解释推翻。
func TestVInstanceNetIncomeHasUndimensionedFact(t *testing.T) {
	// 无维度 context(c-1:只有 entity/identifier,无 segment)必须存在且被 NetIncomeLoss 引用。
	assert.Contains(t, readFixture(t, "testdata/instance_v_fy2025_10k.xml"), "<us-gaap:NetIncomeLoss",
		"对照前提:真实文档含 NetIncomeLoss")

	facts := parseFixture(t, "testdata/instance_v_fy2025_10k.xml", classOfStockAxis)
	for _, f := range facts {
		assert.NotEqual(t, "NetIncomeLoss", f.tag,
			"parseClassFacts 只应收 EPS/股本科目,不收 NetIncomeLoss")
	}
}

// ---------------------------------------------------------------------------
// functional[1] 解析与选类
// ---------------------------------------------------------------------------

// 真实 10-K 的 Class A 稀释 EPS:三个财年逐值锚定(值取自 SEC 原文)。
func TestParseClassFactsRealTenK(t *testing.T) {
	facts := parseFixture(t, "testdata/instance_v_fy2025_10k.xml", classOfStockAxis)

	got := map[string]float64{}
	for _, f := range facts {
		if f.tag == "EarningsPerShareDiluted" && f.member == "CommonClassAMember" {
			got[f.start+"~"+f.end] = f.val
		}
	}
	for _, tc := range []struct {
		period string
		want   float64
	}{
		{"2022-10-01~2023-09-30", 8.28},
		{"2023-10-01~2024-09-30", 9.73},
		{"2024-10-01~2025-09-30", 10.20},
	} {
		assert.InDelta(t, tc.want, got[tc.period], 1e-9, "FY %s 的 Class A 稀释 EPS", tc.period)
	}

	// 同一事实在 inline XBRL 里会被渲染两次(同 contextRef 同值),解析层保留重复是允许的,
	// 但**值必须一致** —— 若出现同期同类别不同值,说明关联逻辑串了 context。
	seen := map[string]float64{}
	for _, f := range facts {
		k := f.tag + "|" + f.member + "|" + f.start + "|" + f.end
		if prev, ok := seen[k]; ok {
			assert.InDelta(t, prev, f.val, 1e-9, "同 (tag,member,period) 的重复事实值必须一致: %s", k)
		}
		seen[k] = f.val
	}
}

// 只取 Diluted:同一份文档里 Basic(10.22/9.74/8.29)与 Diluted(10.20/9.73/8.28)成对出现,
// 混淆会得到系统性偏高的 EPS 且极难察觉。
func TestParseClassFactsDoesNotMixBasic(t *testing.T) {
	facts := parseFixture(t, "testdata/instance_v_fy2025_10k.xml", classOfStockAxis)

	eps, _ := classRawFacts(facts, "CommonClassAMember")
	for _, f := range eps {
		assertNotNear(t, 10.22, f.Val, "10.22 是 Basic FY2025,不得出现在稀释 EPS 里")
		assertNotNear(t, 9.74, f.Val, "9.74 是 Basic FY2024")
		assertNotNear(t, 8.29, f.Val, "8.29 是 Basic FY2023")
	}
	assert.NotEmpty(t, eps)
}

func TestDominantClassPicksClassA(t *testing.T) {
	facts := parseFixture(t, "testdata/instance_v_fy2025_10k.xml", classOfStockAxis)

	assert.Equal(t, "CommonClassAMember", dominantClass(facts),
		"Class A 是 ticker V 对应的可交易类别,稀释股本 ~1.97e9 远大于其它类别")

	// 负向:B2 在 FY2023 的稀释 EPS 是 **0**,选中它会让整条序列出现 0 值而不是缺失。
	eps, _ := classRawFacts(facts, dominantClass(facts))
	for _, f := range eps {
		assert.NotZero(t, f.Val, "主类别的 EPS 不应出现 0 值(B2 陷阱)")
	}
}

// 结构性:dominantClass 的结果与输入切片排列无关。穷举 4 个类别的全部 24 种排列,
// 「返回首个遇到的 member」这类变异在其中至少一个排列上必然返回错误答案 ⇒ p(kill)=1,
// 不依赖 map 迭代序的随机性(AD-27「结构性保证 > 概率性压制」,沿用 TASK-001 的做法)。
func TestDominantClassIsPermutationInvariant(t *testing.T) {
	base := parseFixture(t, "testdata/instance_v_fy2025_10k.xml", classOfStockAxis)

	// 按 member 分组,再对分组做全排列后拼回,保证每个类别都有机会排在最前。
	byMember := map[string][]classFact{}
	for _, f := range base {
		byMember[f.member] = append(byMember[f.member], f)
	}
	members := make([]string, 0, len(byMember))
	for m := range byMember {
		members = append(members, m)
	}
	sort.Strings(members)
	require.GreaterOrEqual(t, len(members), 4, "真实 10-K 至少含 A/B1/B2/C 四个类别")

	for _, perm := range permutations(members) {
		var shuffled []classFact
		for _, m := range perm {
			shuffled = append(shuffled, byMember[m]...)
		}
		assert.Equal(t, "CommonClassAMember", dominantClass(shuffled),
			"排列 %v 下结果必须不变", perm)
	}
}

// permutations 复用 multiunit_test.go 的实现(同包)。

// assertNotNear 断言 got 与 unwanted 不在容差内相等。testify 无 NotInDelta,
// 自己写一个 —— 这类「某个具体错值不得出现」的负向断言是本任务多处的关键防线。
func assertNotNear(t *testing.T, unwanted, got float64, msgAndArgs ...any) {
	t.Helper()
	assert.Greater(t, math.Abs(got-unwanted), 1e-9, msgAndArgs...)
}

// ---------------------------------------------------------------------------
// functional[1] 端到端:接入 FetchCompanyFacts 取数链
// ---------------------------------------------------------------------------

// V 的 companyfacts 无任何 EPS/股本 tag(真实截取),四份真实实例文档补齐 EPS。
func TestFetchCompanyFactsClassEPSFallback(t *testing.T) {
	c, _, archRec := newClassEPSServers(t, "testdata/companyfacts_v_no_eps.json",
		"testdata/submissions_v.json", vInstanceDocs())

	facts, err := c.FetchCompanyFacts(vCIK)
	require.NoError(t, err)
	require.NotEmpty(t, facts)

	// FY2025 三个单季,值取自 SEC 原文(Class A 稀释 EPS)。
	for _, tc := range []struct {
		end string
		eps float64
	}{
		{"2024-12-31", 2.58},
		{"2025-03-31", 2.32},
		{"2025-06-30", 2.69},
	} {
		q := findQuarter(t, facts, tc.end)
		assert.InDelta(t, tc.eps, q.EPSDiluted, 1e-9, "%s 单季 Class A 稀释 EPS", tc.end)
	}

	// 回退路径确实发生了:archives host 收到过请求。
	assert.NotEmpty(t, archRec.paths(), "companyfacts 无 EPS 时必须去取实例文档")

	// 该标的不再「EPS 全为 NULL」。
	var withEPS int
	for _, q := range facts {
		if !math.IsNaN(q.EPSDiluted) {
			withEPS++
		}
	}
	assert.GreaterOrEqual(t, withEPS, 8,
		"至少要有 8 个有 EPS 的季度,下游 ReconstructPESeries 才可能凑够 MinEPSPoints")
}

// Q4 无独立单季申报,靠 FY − ΣQ1..Q3 推导。两个财年各自闭合,全部用真实数据:
//
//	FY2025: 10.20 − (2.58 + 2.32 + 2.69) = 2.61
//	FY2024:  9.73 − (2.39 + 2.29 + 2.40) = 2.65
func TestFetchCompanyFactsClassEPSDerivesQ4(t *testing.T) {
	c, _, _ := newClassEPSServers(t, "testdata/companyfacts_v_no_eps.json",
		"testdata/submissions_v.json", vInstanceDocs())

	facts, err := c.FetchCompanyFacts(vCIK)
	require.NoError(t, err)

	q25 := findQuarter(t, facts, "2025-09-30")
	assert.InDelta(t, 10.20-(2.58+2.32+2.69), q25.EPSDiluted, 1e-9,
		"FY2025 Q4 = FY − ΣQ1..Q3")

	q24 := findQuarter(t, facts, "2024-09-30")
	assert.InDelta(t, 9.73-(2.39+2.29+2.40), q24.EPSDiluted, 1e-9,
		"FY2024 Q4 = FY − ΣQ1..Q3(去年同季来自各 10-Q 的对比期)")

	// 推导值必须落在合理区间,防「FY+ΣQ」之类的符号错误(那会得到 ~17.8)。
	assert.Greater(t, q25.EPSDiluted, 0.0)
	assert.Less(t, q25.EPSDiluted, 5.0)
}

// ---------------------------------------------------------------------------
// functional[2] 回归:既有四家(COST/CRM/WMT/AVGO)走 companyfacts 主路径,不受影响
// ---------------------------------------------------------------------------

// epsBearingFixtures 是**companyfacts 里已含 EPS** 的 fixture —— 即 COST/CRM/WMT/AVGO
// 四家在生产中的形态。functional[2] 的回归范围精确地就是这一类。
//
// 刻意**不含** mainflow / eps_no_shares / fy_june / fy_december:那四个 fixture 本就没有
// 任何 EPS tag,按设计会进入回退路径,把它们放进回归集是范围错误(见
// TestNoEPSFixturesDoEnterFallback,那条反过来把「它们确实进入回退」钉住)。
func epsBearingFixtures() []string {
	return []string{
		"testdata/companyfacts_mini.json",
		"testdata/companyfacts_multiunit.json",
		"testdata/companyfacts_singleunit.json",
		"testdata/companyfacts_tag_fallback.json",
		"testdata/companyfacts_split.json",
		"testdata/companyfacts_q4guard.json",
	}
}

// requireHasEPS 断言 fixture 前提:含至少一个 epsTags 里的 tag。没有这条,
// 上面的清单一旦被改动(或 fixture 被改瘦)就会静默退化成「什么都没验」。
func requireHasEPS(t *testing.T, fixture string) {
	t.Helper()
	raw := readFixture(t, fixture)
	found := slices.ContainsFunc(epsTags, func(tag string) bool {
		return strings.Contains(raw, `"`+tag+`"`)
	})
	require.True(t, found, "%s 必须含 EPS tag,否则它属于回退路径而非回归范围", fixture)
}

// 结构性保证:companyfacts 里有 EPS ⇒ 回退分支根本不进入 ⇒ archives host **零请求**。
// 这比「结果碰巧一样」强:即便实例文档路径有任意 bug,也影响不到这四家。
func TestFetchCompanyFactsWithEPSNeverTouchesArchives(t *testing.T) {
	for _, fixture := range epsBearingFixtures() {
		t.Run(path.Base(fixture), func(t *testing.T) {
			requireHasEPS(t, fixture)
			// submissions 与实例文档一律不可用:一旦回退分支被误触发,必然暴露为请求记录非空。
			c, dataRec, archRec := newClassEPSServers(t, fixture, "", nil)

			_, err := c.FetchCompanyFacts("1045810")
			require.NoError(t, err)
			assert.Empty(t, archRec.paths(),
				"companyfacts 已含 EPS 时不得发起任何实例文档请求")
			for _, p := range dataRec.paths() {
				assert.NotContains(t, p, "/submissions/",
					"连 submissions 都不该请求 —— 回退分支整个不进入")
			}
		})
	}
}

// 逐季逐字段对照:同一 fixture 在「archives 完全不可达」与「archives 可用」两种环境下
// 结果必须逐字节一致 —— 新增回退路径不得以任何方式改动主路径的取数结果。
func TestFetchCompanyFactsUnaffectedByClassEPSPath(t *testing.T) {
	for _, fixture := range epsBearingFixtures() {
		t.Run(path.Base(fixture), func(t *testing.T) {
			requireHasEPS(t, fixture)
			cDead, _, _ := newClassEPSServers(t, fixture, "", nil)
			cLive, _, _ := newClassEPSServers(t, fixture, "testdata/submissions_v.json", vInstanceDocs())

			gotDead, err := cDead.FetchCompanyFacts("1045810")
			require.NoError(t, err)
			gotLive, err := cLive.FetchCompanyFacts("1045810")
			require.NoError(t, err)

			// 用 goldenDump 而不是直接比结构体:NaN 不能用 assert.Equal 比浮点,且
			// Q4 = FY − Σ三季 的求和顺序取自 singleByEnd 的 **map 迭代序**,末位有 ~1e-16
			// 抖动(实测同一 fixture 两次运行分别得 0.9000000000000004 / 0.8999999999999999)。
			// 这是本改动**之前就存在**的性质,与类别 EPS 路径无关;goldenDump 的 12 位有效
			// 数字正好在该噪声之上,用 %v 则会把它误报成回归。
			assert.Equal(t, goldenDump(gotDead), goldenDump(gotLive),
				"主路径结果不得随 archives 可用性变化")
			assert.NotEmpty(t, gotDead)
		})
	}
}

// 对偶:companyfacts **没有** EPS 的 fixture 确实会进入回退路径。
// 这条与上面两条合起来把触发边界卡死 —— 只证「有 EPS 时不进入」而不证「没 EPS 时进入」,
// 一个「回退永不触发」的实现(比如条件写反或恒 false)照样能全绿。
func TestNoEPSFixturesDoEnterFallback(t *testing.T) {
	for _, fixture := range []string{
		"testdata/companyfacts_mainflow.json",
		"testdata/companyfacts_eps_no_shares.json",
		"testdata/companyfacts_fy_june.json",
		"testdata/companyfacts_v_no_eps.json",
	} {
		t.Run(path.Base(fixture), func(t *testing.T) {
			c, dataRec, _ := newClassEPSServers(t, fixture, "", nil)
			_, err := c.FetchCompanyFacts("1045810")
			require.NoError(t, err, "回退路径失败不得让 FetchCompanyFacts 出错")

			var sawSubmissions bool
			for _, p := range dataRec.paths() {
				if strings.Contains(p, "/submissions/") {
					sawSubmissions = true
				}
			}
			assert.True(t, sawSubmissions,
				"companyfacts 无 EPS 时必须尝试回退路径(否则 V 永远修不好)")
		})
	}
}

// ---------------------------------------------------------------------------
// boundary[0] 期间过滤与合成 rawFact 的自洽
// ---------------------------------------------------------------------------

// 真实 10-Q 同时含单季(90/91/92 天)与本年/去年累计(182/183/273/274 天)。
// 累计期必须丢弃,否则会被当成单季写进对应 period_end,把该季 EPS 放大一倍以上。
func TestClassRawFactsDropsYTDPeriods(t *testing.T) {
	facts := parseFixture(t, "testdata/instance_v_fy2025q3_10q.xml", classOfStockAxis)

	// 解析层保留全部期间(含累计),过滤发生在 classRawFacts。
	var sawYTD bool
	for _, f := range facts {
		if f.tag == "EarningsPerShareDiluted" && f.member == "CommonClassAMember" &&
			f.start == "2024-10-01" && f.end == "2025-06-30" {
			sawYTD = true
			assert.InDelta(t, 7.59, f.val, 1e-9, "273 天累计期原值")
		}
	}
	require.True(t, sawYTD, "fixture 前提:真实 10-Q 含 273 天本年累计期")

	eps, _ := classRawFacts(facts, "CommonClassAMember")
	for _, f := range eps {
		assertNotNear(t, 7.59, f.Val, "273 天累计期不得进入合成 rawFact")
		assertNotNear(t, 7.08, f.Val, "274 天去年累计期不得进入")
		assertNotNear(t, 4.90, f.Val, "182 天累计期不得进入")
	}
	// 单季必须留下。
	var got []float64
	for _, f := range eps {
		got = append(got, f.Val)
	}
	assert.Contains(t, got, 2.69, "2025-04-01~2025-06-30 单季必须保留")
}

// 合成 rawFact 的 FP 标注必须与下游 isSingleQuarter/isAnnual 的判定一致。
// 两处口径若分叉(例如按 segments.go 的 keepDuration 含首尾 +1 来分类,而下游用
// client.go 的 durationDays 不含 +1),边界上的期间会被标成单季却被过滤器拒收,
// 表现为「数据莫名其妙少一季」且极难定位。
func TestClassRawFactsFPMatchesDownstreamFilter(t *testing.T) {
	var all []classFact
	for _, fx := range []string{
		"testdata/instance_v_fy2025_10k.xml",
		"testdata/instance_v_fy2025q1_10q.xml",
		"testdata/instance_v_fy2025q2_10q.xml",
		"testdata/instance_v_fy2025q3_10q.xml",
	} {
		all = append(all, parseFixture(t, fx, classOfStockAxis)...)
	}

	eps, shares := classRawFacts(all, "CommonClassAMember")
	require.NotEmpty(t, eps)
	for _, f := range append(append([]rawFact{}, eps...), shares...) {
		assert.True(t, isSingleQuarter(f) || isAnnual(f),
			"合成的每条 rawFact 都必须被下游过滤器接受: %+v", f)
		if f.FP == "FY" {
			assert.True(t, isAnnual(f), "标了 FY 就必须通过 isAnnual: %+v", f)
		} else {
			assert.True(t, isSingleQuarter(f), "标了单季就必须通过 isSingleQuarter: %+v", f)
		}
	}
}

// 回看上限:submissions 给 40 份,也只取最近 28 份。
//
// **期望值 28 在本用例里是写死的字面量,刻意不从 maxClassEPSFilings 算出来。**
// 初版用 `nFilings = maxClassEPSFilings + 12` 造份数、又用 `maxClassEPSFilings` 断言长度,
// 两端随常量同步变化 ⇒ 把常量改成任意值它照样绿(实测变异 28→12,本用例 0/5 红),
// **对取值零判别力**。当时它看上去被杀死了,真正杀它的是 TestClassEPSAndSegmentCapsAreIndependent
// 里那句字面量断言 —— 判别力必须长在**用例自己**身上,不能靠别处兜底。
//
// 28 的正当性来自数据侧实测(2026-07-26 逐份探测 V 的 40 份:序号 0–27 全 200、28 起连续 404),
// 不来自这个测试。将来可用份数增长、按 maxClassEPSFilings 的注释合理调大常量时,
// wantCap 需一并更新 —— 强制这次同步正是本用例的作用。
func TestFetchClassEPSRespectsLookbackCap(t *testing.T) {
	const (
		wantCap  = 28 // 与 maxClassEPSFilings 独立写死,故常量变动必然在此暴露
		nFilings = 40 // 探测过的份数,刻意多于上限
	)

	// 请求路径只在这一处成形:登记 fixture 与断言两端共用,改一处漏一处不再可能。
	archPath := func(i int) string {
		return fmt.Sprintf("/Archives/edgar/data/%s/000095017025%06d/doc-%02d_htm.xml", vCIK, i, i)
	}

	var rows [][3]string
	docs := map[string]string{}
	for i := 0; i < nFilings; i++ {
		// reportDate 严格递减(SEC 惯例由新到旧)
		report := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC).
			AddDate(0, -3*i, 0).Format(dateLayout)
		rows = append(rows, [3]string{"10-Q", report, report})
		docs[path.Base(archPath(i))] = "testdata/instance_v_fy2025q2_10q.xml"
	}
	subsPath := path.Join(t.TempDir(), "subs.json")
	require.NoError(t, os.WriteFile(subsPath, []byte(submissionsJSON(t, rows)), 0o644))

	c, _, archRec := newClassEPSServers(t, "testdata/companyfacts_v_no_eps.json", subsPath, docs)
	_, err := c.FetchCompanyFacts(vCIK)
	require.NoError(t, err)

	// 集合相等而非「只断长度」:多拉、少拉、重复拉、拿错哪一份都在这里暴露。
	// 只断长度的话,「取最旧那 28 份」这类变异照样绿。
	want := make([]string, 0, wantCap)
	for i := 0; i < wantCap; i++ {
		want = append(want, archPath(i))
	}
	assert.ElementsMatch(t, want, archRec.paths(),
		"必须且只能请求最近 %d 份(序号 0..%d)", wantCap, wantCap-1)
}

// 类别 EPS 路径与分部营收路径的回看上限**互相独立**。
// 这条把「用了独立常量」钉成可执行断言:若把 maxClassEPSFilings 换回 maxSegmentFilings,
// 或反过来让 FetchSegmentRevenue 用上 28,都会在这里暴露。
func TestClassEPSAndSegmentCapsAreIndependent(t *testing.T) {
	require.NotEqual(t, maxSegmentFilings, maxClassEPSFilings,
		"两条路径的上限刻意不同;相等会让本测试失去判别力")
	assert.Equal(t, 12, maxSegmentFilings, "分部营收上限不得被本任务改动")
	assert.Equal(t, 28, maxClassEPSFilings,
		"类别 EPS 上限当前 = 28(2026-07-26 的可用份数快照);"+
			"若因新增申报按 maxClassEPSFilings 注释合理调大,此处与 TestFetchClassEPSRespectsLookbackCap 一并更新")
}

// 2019 年前的老式 filing(非 inline XBRL,_htm.xml 推导不成立)一律 404。
// 真实形态:最近若干份可用,再往前**连续**一批全 404。必须逐份跳过、不使整批失败,
// 且可用的那些照常产出 —— 否则一家老公司会因为尾部老报告而整条 EPS 归零。
func TestFetchClassEPSSkipsPre2019NotFoundTail(t *testing.T) {
	var rows [][3]string
	docs := map[string]string{}
	for i := 0; i < maxClassEPSFilings; i++ {
		report := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC).
			AddDate(0, -3*i, 0).Format(dateLayout)
		rows = append(rows, [3]string{"10-Q", report, report})
		// 只有最近 4 份登记 fixture;其余 24 份未登记 ⇒ server 返回 404(模拟老式 filing)。
		if i < 4 {
			docs[fmt.Sprintf("doc-%02d_htm.xml", i)] = "testdata/instance_v_fy2025q2_10q.xml"
		}
	}
	subsPath := path.Join(t.TempDir(), "subs.json")
	require.NoError(t, os.WriteFile(subsPath, []byte(submissionsJSON(t, rows)), 0o644))

	logs := captureLogs(t)
	c, _, archRec := newClassEPSServers(t, "testdata/companyfacts_v_no_eps.json", subsPath, docs)

	facts, err := c.FetchCompanyFacts(vCIK)
	require.NoError(t, err, "尾部连续 404 不得让整家失败")

	// 28 份全部尝试过(404 不提前终止循环)。
	assert.Len(t, archRec.paths(), maxClassEPSFilings,
		"404 只跳过该份,不得中断后续尝试")
	assert.Contains(t, logs.String(), "404", "跳过必须可观测")

	// 可用的那 4 份照常产出:2025-01-01~2025-03-31 单季 EPS = 2.32。
	q := findQuarter(t, facts, "2025-03-31")
	assert.InDelta(t, 2.32, q.EPSDiluted, 1e-9,
		"能取到的报告必须照常生效,不受 404 尾部影响")
}

// ---------------------------------------------------------------------------
// error_handling[0] 降级:任何失败都不得 panic、不得让 FetchCompanyFacts 返回错误
// (V 目前的 degraded 是可接受状态,修不成功不能让情况更糟)
// ---------------------------------------------------------------------------

// 部分实例文档 404(2019 前老式 filing 的 _htm.xml 推导不成立):跳过该份,其余照常。
func TestFetchCompanyFactsClassEPSInstanceNotFound(t *testing.T) {
	docs := vInstanceDocs()
	delete(docs, "v-20250630_htm.xml") // 该份 404

	logs := captureLogs(t)
	c, _, _ := newClassEPSServers(t, "testdata/companyfacts_v_no_eps.json",
		"testdata/submissions_v.json", docs)

	facts, err := c.FetchCompanyFacts(vCIK)
	require.NoError(t, err, "单份实例文档 404 不得让整家失败")

	// 缺了 Q3 那份 ⇒ 该季无 EPS,且 FY2025 只剩 2 个单季 ⇒ Q4 不再推导(需恰好 3 个)。
	assert.True(t, math.IsNaN(findQuarter(t, facts, "2025-06-30").EPSDiluted),
		"缺失那份对应的季度 EPS 应为 NaN,而不是错值")
	// 其余季度仍在。
	assert.InDelta(t, 2.58, findQuarter(t, facts, "2024-12-31").EPSDiluted, 1e-9)
	assert.Contains(t, logs.String(), "404",
		"跳过必须可观测")
}

// submissions 不可达:整条回退路径放弃,行为回到「companyfacts 取不到 EPS」的现状。
func TestFetchCompanyFactsClassEPSSubmissionsError(t *testing.T) {
	logs := captureLogs(t)
	c, _, archRec := newClassEPSServers(t, "testdata/companyfacts_v_no_eps.json", "", vInstanceDocs())

	facts, err := c.FetchCompanyFacts(vCIK)
	require.NoError(t, err, "submissions 失败不得让 FetchCompanyFacts 返回错误")
	require.NotEmpty(t, facts, "companyfacts 的其它科目仍应正常产出季度")

	for _, q := range facts {
		assert.True(t, math.IsNaN(q.EPSDiluted), "取不到 EPS 时保持 NaN,不得凭空造值")
	}
	assert.Empty(t, archRec.paths(), "submissions 都拿不到,不应再去请求实例文档")
	assert.NotEmpty(t, logs.String(), "降级必须留下日志")
}

// 实例文档 XML 损坏:跳过该份,不中断其余。
func TestFetchCompanyFactsClassEPSMalformedInstance(t *testing.T) {
	docs := vInstanceDocs()
	docs["v-20250630_htm.xml"] = "testdata/instance_malformed.xml"

	logs := captureLogs(t)
	c, _, _ := newClassEPSServers(t, "testdata/companyfacts_v_no_eps.json",
		"testdata/submissions_v.json", docs)

	facts, err := c.FetchCompanyFacts(vCIK)
	require.NoError(t, err)
	assert.InDelta(t, 2.58, findQuarter(t, facts, "2024-12-31").EPSDiluted, 1e-9,
		"其余报告不受损坏那份影响")
	assert.NotEmpty(t, logs.String())
}

// 响应体超限:跳过该份而不是把截断后的残缺数据当成有效结果。
func TestFetchCompanyFactsClassEPSBodyLimit(t *testing.T) {
	logs := captureLogs(t)
	c, _, _ := newClassEPSServers(t, "testdata/companyfacts_v_no_eps.json",
		"testdata/submissions_v.json", vInstanceDocs())
	c.maxInstanceBytes = 512 // 远小于任何 fixture,必然触顶

	facts, err := c.FetchCompanyFacts(vCIK)
	require.NoError(t, err)
	for _, q := range facts {
		assert.True(t, math.IsNaN(q.EPSDiluted), "全部超限 ⇒ 无 EPS,而不是残缺值")
	}
	assert.Contains(t, logs.String(), "limit")
}

// 全部实例文档不可达 ⇒ 与本改动前的行为完全一致(EPS 全 NaN,该标的 degraded 走 engine)。
func TestFetchCompanyFactsClassEPSAllUnavailable(t *testing.T) {
	cNone, _, _ := newClassEPSServers(t, "testdata/companyfacts_v_no_eps.json",
		"testdata/submissions_v.json", nil) // 所有 basename 都 404

	got, err := cNone.FetchCompanyFacts(vCIK)
	require.NoError(t, err)
	require.NotEmpty(t, got)
	for _, q := range got {
		assert.True(t, math.IsNaN(q.EPSDiluted))
	}
}

// ---------------------------------------------------------------------------
// boundary[0] 点位可用日期(防前视)—— 本路径特有的一类失真
//
// companyfacts 的每条 entry 自带**它自己的** filed;而实例文档路径只能拿到「装着这条事实
// 的那份报告」的 filingDate。同一期间会在**次年同期报告的对比期**里再次出现,于是同一个
// 值被打上晚一年的日期,applyDuration 的 keepLatest(取 filed 最新)让晚的那条胜出
// ⇒ 每个季度的可用日期被系统性推迟约一年 ⇒ 估值序列前端被防前视门禁整段砍掉。
//
// 实测(修复前):V 的 2025Q1(period_end 2024-12-31)filing_date 落在 2026-01-30 而非真实的
// 2025-01-31;valuation_daily 仅剩 371 天 / pctl_5y 119 点,**比修复前走 engine 兜底的
// 705 天 / 453 点还短**。
// ---------------------------------------------------------------------------

// 值未变的重复披露:保留**最早**那次的 filed。
func TestClassRawFactsKeepsEarliestFilingForUnchangedValue(t *testing.T) {
	facts := []classFact{
		// 次年同期报告里的对比期(晚)——真实顺序:取数循环按 reportDate 降序,晚的先来。
		{tag: "EarningsPerShareDiluted", member: "A",
			start: "2025-01-01", end: "2025-03-31", filed: "2026-04-29", val: 2.32},
		// 本期报告(早)
		{tag: "EarningsPerShareDiluted", member: "A",
			start: "2025-01-01", end: "2025-03-31", filed: "2025-04-30", val: 2.32},
	}
	eps, _ := classRawFacts(facts, "A")
	require.Len(t, eps, 1, "同期同值只应留一条")
	assert.Equal(t, "2025-04-30", eps[0].Filed,
		"值没变就不该把可用日期推迟到对比期那份报告的日期")
}

// 值变了(真重述):两条都要保留,交给下游 keepLatest 取 filed 最新的重述值。
// 没有这条,上一条测试可以被「无脑取最早」的实现满足,而那会让重述永远生效不了。
func TestClassRawFactsKeepsBothOnRestatement(t *testing.T) {
	facts := []classFact{
		{tag: "EarningsPerShareDiluted", member: "A",
			start: "2025-01-01", end: "2025-03-31", filed: "2025-04-30", val: 2.32},
		{tag: "EarningsPerShareDiluted", member: "A",
			start: "2025-01-01", end: "2025-03-31", filed: "2026-04-29", val: 2.50}, // 重述
	}
	eps, _ := classRawFacts(facts, "A")
	require.Len(t, eps, 2, "值不同 ⇒ 两条都保留,由 keepLatest 选重述值")

	byFiled := map[string]float64{}
	for _, f := range eps {
		byFiled[f.Filed] = f.Val
	}
	assert.InDelta(t, 2.32, byFiled["2025-04-30"], 1e-9)
	assert.InDelta(t, 2.50, byFiled["2026-04-29"], 1e-9)
}

// 端到端(取数层):用五份真实报告跑 submissions → 实例文档 → 合成 rawFact 全链,
// 断言重复披露的那一期取到**原始**申报日。
//
// 刻意断言在 rawFact 层而不是 QuarterlyFact.FilingDate 上:后者是**全部科目取 max**
// (client.go 的 get()),而 companyfacts 自己的 NetIncomeLoss 同样含 filed=2026-04-29 的
// 对比期条目 —— 那个约一年的滞后是**本改动之前就存在、且对所有公司一视同仁**的既有语义
// (keepLatest:重述以最新披露为准),不在本任务范围内。把它写进断言只会得到一个
// 「测了别人的行为」的假阳性用例。
func TestFetchClassDimensionedEPSUsesOriginalFilingDate(t *testing.T) {
	c, _, _ := newClassEPSServers(t, "testdata/companyfacts_v_no_eps.json",
		"testdata/submissions_v.json", vInstanceDocs())

	eps, _, err := c.fetchClassDimensionedEPS(vCIK, classOfStockAxis)
	require.NoError(t, err)
	require.NotEmpty(t, eps)

	// 2025-01-01~2025-03-31 同时出现在 FY2025Q2 10-Q(filed 2025-04-30,本期)与
	// FY2026Q2 10-Q(filed 2026-04-29,对比期),两处值都是 2.32 ⇒ 只留 2025-04-30 那条。
	var got []rawFact
	for _, f := range eps {
		if f.Start == "2025-01-01" && f.End == "2025-03-31" {
			got = append(got, f)
		}
	}
	require.Len(t, got, 1, "同期同值跨两份报告重复,只应留一条")
	assert.InDelta(t, 2.32, got[0].Val, 1e-9)
	assert.Equal(t, "2025-04-30", got[0].Filed,
		"值没变就不该把可用日期推迟到对比期那份报告的日期")
}

// ---------------------------------------------------------------------------
// 变异测试补漏(AD-27:存活变异不得默认归类为等价)
//
// 首轮 11 个变异中 M1/M3/M4 存活。逐个查证后确认**都不是等价变异**,而是
// 「V 的真实数据恰好覆盖不到该判定」—— 实测 V 三份真实文档里 EPS/股本事实引用的
// 12~16 个 context **全部**是「单维且落在类别轴」,于是轴匹配、单维、候选过滤这三处
// 判定在真实数据上从不被触发。下面三条用例针对性补上。
// ---------------------------------------------------------------------------

// M1 杀手:axis 参数必须真的起门禁作用。
// 用真实 10-K 但传入**另一个轴**:轴匹配判定若被去掉,类别轴的 context 会照收不误。
func TestParseClassFactsRespectsAxis(t *testing.T) {
	assert.Empty(t, parseFixture(t, "testdata/instance_v_fy2025_10k.xml", "StatementEquityComponentsAxis"),
		"轴不匹配时不得收录任何事实;否则任意单维 context 都会被当成股份类别")

	// 对照:传对轴时确实收得到(排除「函数恒返回空」这种平凡通过)。
	assert.NotEmpty(t, parseFixture(t, "testdata/instance_v_fy2025_10k.xml", classOfStockAxis))
}

// M3 杀手:候选类别必须限定为**有 EPS 披露**的那些。
// 只有股本、没有 EPS 的"类别"(如 ParticipatingSecuritiesMember)即便股本数最大,
// 也不能被选中 —— 选中它会让 EPS 整列取不到值。
func TestDominantClassIgnoresMembersWithoutEPS(t *testing.T) {
	facts := []classFact{
		{tag: "EarningsPerShareDiluted", member: "CommonClassAMember",
			start: "2024-10-01", end: "2025-09-30", val: 10.20},
		{tag: "WeightedAverageNumberOfDilutedSharesOutstanding", member: "CommonClassAMember",
			start: "2024-10-01", end: "2025-09-30", val: 1.966e9},
		// 股本数远大于 Class A,但**没有任何 EPS 事实**
		{tag: "WeightedAverageNumberOfDilutedSharesOutstanding", member: "ParticipatingSecuritiesMember",
			start: "2024-10-01", end: "2025-09-30", val: 9.9e9},
	}
	assert.Equal(t, "CommonClassAMember", dominantClass(facts),
		"没有 EPS 披露的 member 不是候选,哪怕它股本最大")
}

// M4 杀手:主类别由**股本大小**决定,不是「排序后的第一个」。
//
// 这一条补的是 TASK-001 同款陷阱:V 的正确答案 CommonClassAMember 恰好也是字母序最前的,
// 于是「return members[0]」这个变异在真实数据上蒙对、排列不变性用例也杀不掉它
// (排序后首个与排列无关,同样满足不变性)。必须构造「正确答案不在字母序首位」的场景。
func TestDominantClassIgnoresAlphabeticalOrder(t *testing.T) {
	mk := func(member string, shares float64) []classFact {
		return []classFact{
			{tag: "EarningsPerShareDiluted", member: member,
				start: "2024-10-01", end: "2025-09-30", val: 1},
			{tag: "WeightedAverageNumberOfDilutedSharesOutstanding", member: member,
				start: "2024-10-01", end: "2025-09-30", val: shares},
		}
	}
	// 字母序:AMember < ZMember;但股本 Z 远大于 A。
	facts := append(mk("CommonClassAMember", 1e6), mk("CommonClassZMember", 9e8)...)
	assert.Equal(t, "CommonClassZMember", dominantClass(facts),
		"应按股本选 Z,而不是按字母序选 A")

	// 反向摆放输入顺序,结果不变(与排列无关的性质在此场景下同样成立)。
	reversed := append(mk("CommonClassZMember", 9e8), mk("CommonClassAMember", 1e6)...)
	assert.Equal(t, "CommonClassZMember", dominantClass(reversed))
}

// M-sum 杀手:主类别按该类别申报过的**最大**股本决定,而不是按求和。
//
// 为什么必须构造 fixture:**V 的真实数据对 max/sum 这个选择没有判别力** —— Class A 的
// max=2.085e9、sum=1.245e11,两种算法都遥遥领先其它类别,把 max 换成 sum 照样选中 Class A
// (实测该变异在本用例存在之前 0/5 红,即完全存活)。TASK-002 的 discovery 把「用 max 不用 sum」
// 写成了明确决策,却没有任何测试钉住它。要钉住,只能造一个两种算法**给出不同答案**的场景。
//
// 场景复刻的正是 max 要防的真实机制:同一条股本事实在 inline XBRL 里会被重复渲染,
// 且 diluted/basic 两条 tag 都落在 sharesTags 里 —— 于是「条数多」会把求和吹大。
// 这里 A 类每条只有 Z 类的 1/6,但有 12 条 ⇒ sum 选 A;max 选 Z。
// 把 max 的正确答案放在字母序**末位**,顺带让「return members[0]」那类变异在这里也红。
//
// **该 fixture 不与生产形态冲突**:它测的是**算法选择**,不是 V 的实际数据分布。
// 真实数据里 max 与 sum 同答案(都选 Class A),正因如此才需要一个刻意构造的反例 ——
// 用真实形态永远问不出「实现选的是哪一种」。
func TestDominantClassUsesMaxSharesNotSum(t *testing.T) {
	const (
		bigOnce   = 9.0e8 // Z 类:一条大额
		smallEach = 1.5e8 // A 类:12 条小额,合计 1.8e9
		nSmall    = 12
	)
	// 判别力自检:fixture 数值若被改成 sum 与 max 同答案,本用例会退化成又一条无判别力的
	// 测试并**静默变绿**。这两行让那种退化立刻失败。
	require.Greater(t, smallEach*nSmall, bigOnce, "sum 口径下必须是 A 类胜出")
	require.Greater(t, bigOnce, smallEach, "max 口径下必须是 Z 类胜出")

	facts := []classFact{
		{tag: "EarningsPerShareDiluted", member: "CommonClassZMember",
			start: "2024-10-01", end: "2025-09-30", val: 10.20},
		{tag: "WeightedAverageNumberOfDilutedSharesOutstanding", member: "CommonClassZMember",
			start: "2024-10-01", end: "2025-09-30", val: bigOnce},
		{tag: "EarningsPerShareDiluted", member: "CommonClassAMember",
			start: "2024-10-01", end: "2025-09-30", val: 0.50},
	}
	for i := 0; i < nSmall; i++ {
		// diluted / basic 交替:两条 tag 同属 sharesTags,是求和被吹大的真实来源之一。
		tag := sharesTags[i%len(sharesTags)]
		facts = append(facts, classFact{tag: tag, member: "CommonClassAMember",
			start: "2024-10-01", end: "2025-09-30", val: smallEach})
	}

	assert.Equal(t, "CommonClassZMember", dominantClass(facts),
		"按最大股本选 Z;实现若改成求和会选中 A(sum(A)=%.3g > max(Z)=%.3g)",
		smallEach*nSmall, bigOnce)
}
